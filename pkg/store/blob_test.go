package store

import (
	"bytes"
	"crypto/rand"
	"io"
	"testing"

	pb "code.tvl.fyi/tvix/store/protos"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/test/bufconn"
)

func blobServer(s *server.Server, t *testing.T) (*grpc.Server, *bufconn.Listener) {
	t.Helper()

	blobService, err := NewBlobService(natsConn(t, s))
	if err != nil {
		t.Fatalf("failed to create blob service: %v", err)
	}

	srv := grpc.NewServer()
	pb.RegisterBlobServiceServer(srv, blobService)

	buffer := 10 * 1024 * 1024
	lis := bufconn.Listen(buffer)

	go func() {
		if err := srv.Serve(lis); err != nil {
			t.Errorf("error serving server: %v", err)
		}
	}()

	return srv, lis
}

func TestBlobService_Put(t *testing.T) {
	s := runBasicJetStreamServer(t)
	defer shutdownJSServerAndRemoveStorage(t, s)

	srv, lis := blobServer(s, t)
	defer srv.Stop()

	conn := grpcConn(lis, t)
	blobClient := pb.NewBlobServiceClient(conn)

	payload := make([]byte, 100*1024*1024)
	_, err := rand.Read(payload)
	if err != nil {
		t.Fatalf("failed to generate random bytes: %v", err)
	}

	chunkSize := (4 * 1024 * 1024) - 1024 // stay just under 4MB grpc limit
	resp, err := putBlob(blobClient, bytes.NewReader(payload), chunkSize, t)
	if err != nil {
		t.Fatalf("failed to received a response: %v", err)
	}

	reader, writer := io.Pipe()
	go func() {
		getBlob(blobClient, resp.Digest, writer, t)
	}()

	payload2, err := io.ReadAll(reader)
	if err != nil {
		log.Fatalf("failed to read blob: %v", err)
	}

	assert.Equal(t, payload, payload2)
}
