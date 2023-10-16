package store

import (
	"net"
	"net/http"
	"runtime/debug"
	"syscall"

	"google.golang.org/grpc/reflection"

	"github.com/brianmcgee/nvix/pkg/cli"

	tvpb "code.tvl.fyi/tvix/store/protos"
	"github.com/brianmcgee/nvix/pkg/pathinfo"

	"github.com/brianmcgee/nvix/pkg/directory"

	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/recovery"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/brianmcgee/nvix/pkg/blob"

	pb "code.tvl.fyi/tvix/castore/protos"
	grpcprom "github.com/grpc-ecosystem/go-grpc-middleware/providers/prometheus"

	"github.com/charmbracelet/log"
	"github.com/ztrue/shutdown"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
)

type Run struct {
	Log  cli.LogOptions  `embed:""`
	Nats cli.NatsOptions `embed:""`

	ListenAddr  string `short:"l" env:"LISTEN_ADDR" default:"localhost:5000"`
	MetricsAddr string `short:"m" env:"METRICS_ADDR" default:"localhost:5050"`
}

func (r *Run) Run() error {
	r.Log.ConfigureLogger()

	conn := r.Nats.Connect()

	blobServer, err := blob.NewServer(conn)
	if err != nil {
		log.Fatalf("failed to create blob service: %v", err)
	}

	directoryServer, err := directory.NewServer(conn)
	if err != nil {
		log.Fatalf("failed to create directory service: %v", err)
	}

	pathInfoServer, err := pathinfo.NewServer(conn, blobServer, directoryServer)
	if err != nil {
		log.Fatalf("failed to create path info service: %v", err)
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
		rpcLogger.Error("recovered from panic", "panic", p, "stack", string(debug.Stack()))
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
	pb.RegisterBlobServiceServer(grpcServer, blobServer)
	pb.RegisterDirectoryServiceServer(grpcServer, directoryServer)
	tvpb.RegisterPathInfoServiceServer(grpcServer, pathInfoServer)

	// register for reflection to help with cli tools
	reflection.Register(grpcServer)

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
