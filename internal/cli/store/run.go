package store

import (
	"net"
	"net/http"
	"runtime/debug"
	"syscall"

	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/recovery"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/brianmcgee/nvix/pkg/blob"

	grpcprom "github.com/grpc-ecosystem/go-grpc-middleware/providers/prometheus"
	"github.com/nats-io/nats.go"

	pb "code.tvl.fyi/tvix/castore/protos"

	"github.com/charmbracelet/log"
	"github.com/ztrue/shutdown"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
)

type Run struct {
	NatsUrl         string `short:"n" env:"NVIX_STORE_NATS_URL" default:"nats://localhost:4222"`
	NatsCredentials string `short:"c" env:"NVIX_STORE_NATS_CREDENTIALS_FILE" required:""`

	ListenAddr  string `short:"l" env:"NVIX_STORE_LISTEN_ADDR" default:"localhost:5000"`
	MetricsAddr string `short:"m" env:"NVIX_STORE_METRICS_ADDR" default:"localhost:5050"`
}

func (r *Run) Run() error {
	log.Debug("connecting to NATS", "url", r.NatsUrl, "creds", r.NatsCredentials)

	conn, err := nats.Connect(r.NatsUrl, nats.UserCredentials(r.NatsCredentials))
	if err != nil {
		log.Fatalf("failed to connect to nats: %v", err)
	}

	service, err := blob.NewService(conn)
	if err != nil {
		log.Fatalf("failed to create blob service")
	}

	// setup metrics
	srvMetrics := grpcprom.NewServerMetrics(
		grpcprom.WithServerHandlingTimeHistogram(
			grpcprom.WithHistogramBuckets([]float64{0.001, 0.01, 0.1, 0.3, 0.6, 1, 3, 6, 9, 20, 30, 60, 90, 120}),
		),
	)
	reg := prometheus.NewRegistry()
	reg.MustRegister(srvMetrics)

	// setup logging
	rpcLogger := log.With("service", "gRPC/server")

	// Setup metric for panic recoveries.
	panicsTotal := promauto.With(reg).NewCounter(prometheus.CounterOpts{
		Name: "grpc_req_panics_recovered_total",
		Help: "Total number of gRPC requests recovered from internal panic.",
	})
	grpcPanicRecoveryHandler := func(p any) (err error) {
		panicsTotal.Inc()
		rpcLogger.Error("recovered from panic", "panic", p, "stack", debug.Stack())
		return status.Errorf(codes.Internal, "%s", p)
	}

	//
	opts := []grpc.ServerOption{
		grpc.MaxRecvMsgSize(16 * 1024 * 1024),
		grpc.ChainUnaryInterceptor(
			srvMetrics.UnaryServerInterceptor(),
			// todo add logging
			recovery.UnaryServerInterceptor(recovery.WithRecoveryHandler(grpcPanicRecoveryHandler)),
		),
		grpc.ChainStreamInterceptor(
			srvMetrics.StreamServerInterceptor(),
			// todo add logging
			recovery.StreamServerInterceptor(recovery.WithRecoveryHandler(grpcPanicRecoveryHandler)),
		),
	}

	grpcServer := grpc.NewServer(opts...)
	pb.RegisterBlobServiceServer(grpcServer, service)

	srvMetrics.InitializeMetrics(grpcServer)

	rpcListener, err := net.Listen("tcp", r.ListenAddr)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	shutdown.Add(func() {
		grpcServer.GracefulStop()
		grpcServer.Stop()
	})

	eg := errgroup.Group{}
	eg.Go(func() error {
		return grpcServer.Serve(rpcListener)
	})

	metricsListener, err := net.Listen("tcp", r.MetricsAddr)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	shutdown.Add(func() {
		_ = metricsListener.Close()
	})

	httpSrv := &http.Server{}

	eg.Go(func() error {
		m := http.NewServeMux()
		// Create HTTP handler for Prometheus metrics.
		m.Handle("/metrics", promhttp.HandlerFor(
			reg,
			promhttp.HandlerOpts{
				// Opt into OpenMetrics e.g. to support exemplars.
				EnableOpenMetrics: true,
			},
		))
		httpSrv.Handler = m
		return httpSrv.Serve(metricsListener)
	})

	log.Info("listening", "type", "rpc", "addr", rpcListener.Addr())
	log.Info("listening", "type", "metrics", "addr", metricsListener.Addr())

	shutdown.Listen(syscall.SIGINT, syscall.SIGTERM)

	return eg.Wait()
}
