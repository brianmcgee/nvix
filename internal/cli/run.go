package cli

import (
	"net"
	"syscall"

	"github.com/brianmcgee/nvix/pkg/store/blob"

	pb "code.tvl.fyi/tvix/store/protos"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go"
	"github.com/ztrue/shutdown"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
)

type Run struct {
	NatsUrl         string `short:"n" env:"NVIX_STORE_NATS_URL" default:"nats://localhost:4222"`
	NatsCredentials string `short:"c" env:"NVIX_STORE_NATS_CREDENTIALS_FILE" required:""`

	ListenAddr string `short:"l" env:"NVIX_STORE_LISTEN_ADDR" default:"localhost:5000"`
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

	var opts []grpc.ServerOption
	grpcServer := grpc.NewServer(opts...)
	pb.RegisterBlobServiceServer(grpcServer, service)

	listener, err := net.Listen("tcp", r.ListenAddr)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	shutdown.Add(func() {
		grpcServer.GracefulStop()
	})

	eg := errgroup.Group{}
	eg.Go(func() error {
		return grpcServer.Serve(listener)
	})

	log.Info("listening", "listener", listener.Addr())

	shutdown.Listen(syscall.SIGINT, syscall.SIGTERM)

	return eg.Wait()
}
