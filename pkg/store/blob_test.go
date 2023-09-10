package store

import (
	"bytes"
	pb "code.tvl.fyi/tvix/store/protos"
	"context"
	"crypto/rand"
	"github.com/charmbracelet/log"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"io"
	"net"
	"testing"
)

func blobServer(s *server.Server, t *testing.T) (*grpc.Server, *bufconn.Listener) {
	t.Helper()

	blobService, err := NewBlobService(natsConn(t, s))
	if err != nil {
		t.Fatalf("failed to create blob service: %v", err)
	}

	srv := grpc.NewServer()
	pb.RegisterBlobServiceServer(srv, blobService)

	buffer := 101024 * 1024
	lis := bufconn.Listen(buffer)

	go func() {
		if err := srv.Serve(lis); err != nil {
			t.Errorf("error serving server: %v", err)
		}
	}()

	return srv, lis
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

func TestBlobService_Put(t *testing.T) {

	log.SetLevel(log.DebugLevel)

	s := runBasicJetStreamServer(t)
	defer shutdownJSServerAndRemoveStorage(t, s)

	srv, lis := blobServer(s, t)
	defer srv.Stop()

	conn := grpcConn(lis, t)
	blobClient := pb.NewBlobServiceClient(conn)

	ctx := context.Background()
	put, err := blobClient.Put(ctx)
	if err != nil {
		t.Fatalf("failed to create put server: %v", err)
	}

	payload := make([]byte, 100*1024*1024)
	_, err = rand.Read(payload)
	if err != nil {
		t.Fatalf("failed to generate random bytes: %v", err)
	}

	chunk := make([]byte, 1*1024*1024)
	reader := bytes.NewReader(payload)

	for {
		n, err := reader.Read(chunk)
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

	resp, err := put.CloseAndRecv()
	if err != nil {
		t.Fatalf("failed to received a response: %v", err)
	}

	get, err := blobClient.Read(ctx, &pb.ReadBlobRequest{Digest: resp.Digest})
	if err != nil {
		t.Fatalf("failed to open read request: %v", err)
	}

	buf := bytes.NewBuffer(nil)
	for {
		chunk, err := get.Recv()
		if err == io.EOF {
			assert.Equal(t, payload, buf.Bytes())
			break
		} else if err != nil {
			t.Fatalf("failed to get next chunk: %v", err)
		}
		if _, err = io.Copy(buf, bytes.NewReader(chunk.Data)); err != nil {
			t.Fatalf("failed to write chunk into read buffer: %v", err)
		}
	}
}
