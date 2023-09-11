package store

import (
	"bytes"
	"context"
	"io"
	"net"
	"os"
	"testing"
	"time"

	pb "code.tvl.fyi/tvix/store/protos"

	"github.com/charmbracelet/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats-server/v2/test"
	"github.com/nats-io/nats.go"
)

const (
	_EMPTY_ = ""
)

type logAdapter struct{}

func (l *logAdapter) Noticef(format string, v ...interface{}) {
	log.Infof(format, v...)
}

func (l *logAdapter) Warnf(format string, v ...interface{}) {
	log.Warnf(format, v...)
}

func (l *logAdapter) Fatalf(format string, v ...interface{}) {
	log.Fatalf(format, v...)
}

func (l *logAdapter) Errorf(format string, v ...interface{}) {
	log.Errorf(format, v...)
}

func (l *logAdapter) Debugf(format string, v ...interface{}) {
	log.Debugf(format, v...)
}

func (l *logAdapter) Tracef(format string, v ...interface{}) {
	log.Debugf(format, v...)
}

func runBasicJetStreamServer(t *testing.T) *server.Server {
	t.Helper()
	opts := test.DefaultTestOptions
	opts.Port = -1
	opts.JetStream = true
	opts.Debug = true
	opts.MaxPayload = 8 * 1024 * 1024
	srv := test.RunServer(&opts)
	srv.SetLoggerV2(&logAdapter{}, opts.Debug, opts.Trace, false)
	return srv
}

func natsConn(t *testing.T, s *server.Server, opts ...nats.Option) *nats.Conn {
	t.Helper()
	nc, err := nats.Connect(s.ClientURL(), opts...)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	return nc
}

func jsClient(t *testing.T, s *server.Server, opts ...nats.Option) (*nats.Conn, nats.JetStreamContext) {
	t.Helper()
	nc := natsConn(t, s, opts...)
	js, err := nc.JetStream(nats.MaxWait(10 * time.Second))
	if err != nil {
		t.Fatalf("Unexpected error getting JetStream context: %v", err)
	}
	return nc, js
}

func shutdownJSServerAndRemoveStorage(t *testing.T, s *server.Server) {
	t.Helper()
	var sd string
	if config := s.JetStreamConfig(); config != nil {
		sd = config.StoreDir
	}
	s.Shutdown()
	if sd != _EMPTY_ {
		if err := os.RemoveAll(sd); err != nil {
			t.Fatalf("Unable to remove storage %q: %v", sd, err)
		}
	}
	s.WaitForShutdown()
}

func grpcConn(lis *bufconn.Listener, t *testing.T) *grpc.ClientConn {
	t.Helper()
	conn, err := grpc.Dial("",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(ctx context.Context, s string) (net.Conn, error) {
			return lis.Dial()
		}))
	if err != nil {
		t.Fatalf("failed to create a grpc server connection: %v", err)
	}
	return conn
}

func putBlob(c pb.BlobServiceClient, r io.Reader, chunkSize int, t *testing.T) (*pb.PutBlobResponse, error) {
	t.Helper()

	put, err := c.Put(context.Background())
	if err != nil {
		t.Fatalf("failed to create put: %v", err)
	}

	chunk := make([]byte, chunkSize)

	for {
		n, err := r.Read(chunk)
		if err == io.EOF {
			break
		} else if err != nil {
			t.Fatalf("failed to read chunk: %v", err)
		}

		if err = put.Send(&pb.BlobChunk{
			Data: chunk[:n],
		}); err != nil {
			t.Fatalf("failed to send blob chunk: %v", err)
		}
	}

	return put.CloseAndRecv()
}

func getBlob(c pb.BlobServiceClient, digest []byte, writer io.WriteCloser, t *testing.T) {
	get, err := c.Read(context.Background(), &pb.ReadBlobRequest{Digest: digest})
	if err != nil {
		t.Fatalf("failed to open read request: %v", err)
	}

	for {
		chunk, err := get.Recv()
		if err == io.EOF {
			if err = writer.Close(); err != nil {
				log.Fatalf("failed to close writer: %v", err)
			}
			return
		} else if err != nil {
			t.Fatalf("failed to get next chunk: %v", err)
		}
		if _, err = io.Copy(writer, bytes.NewReader(chunk.Data)); err != nil {
			t.Fatalf("failed to write chunk into read buffer: %v", err)
		}
	}
}
