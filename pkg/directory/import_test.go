package directory

import (
	"context"
	"io"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	pb "code.tvl.fyi/tvix/castore-go"

	"github.com/brianmcgee/nvix/pkg/blob"
	"github.com/brianmcgee/nvix/pkg/test"
	"github.com/nats-io/nats-server/v2/server"
	"google.golang.org/grpc"

	"github.com/charmbracelet/log"
	"github.com/stretchr/testify/require"
)

var (
	canonicalEntries = []struct {
		path  string
		depth int
		isDir bool
	}{
		{path: "a/c.txt", depth: 2, isDir: false},
		{path: "a/d.txt", depth: 2, isDir: false},
		{path: "a/e/f.txt", depth: 3, isDir: false},
		{path: "a/e", depth: 2, isDir: true},
		{path: "a", depth: 1, isDir: true},
		{path: "b/g.txt", depth: 2, isDir: false},
		{path: "b/h.txt", depth: 2, isDir: false},
		{path: "b", depth: 1, isDir: true},
		{path: "", depth: 0, isDir: true},
	}

	canonicalFilePaths = []string{
		"a/c.txt",
		"a/d.txt",
		"a/e/f.txt",
		"b/g.txt",
	}
)

func blobServer(s *server.Server, t test.TestingT) (*grpc.Server, net.Listener) {
	t.Helper()

	conn := test.NatsConn(t, s)

	ctx := context.Background()
	if err := blob.NewMetaStore(conn).Init(ctx); err != nil {
		t.Fatal(err)
	} else if err := blob.NewChunkStore(conn).Init(ctx); err != nil {
		t.Fatal(err)
	}

	blobService, err := blob.NewServer(conn)
	if err != nil {
		t.Fatalf("failed to create blob service: %v", err)
	}

	srv := grpc.NewServer(grpc.MaxRecvMsgSize(16 * 1024 * 1024))
	pb.RegisterBlobServiceServer(srv, blobService)

	lis, err := net.Listen("tcp", ":0")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	go func() {
		if err := srv.Serve(lis); err != nil {
			t.Errorf("error serving server: %v", err)
		}
	}()

	return srv, lis
}

func TestDepthFirstIterator_Canonical(t *testing.T) {
	r := require.New(t)

	iterator, err := NewDepthFirstIterator("../../test/testdata/dfi/canonical")
	r.Nil(err)

	idx := 0

	for {
		info, err := iterator.Next()
		if err == io.EOF {
			return
		}

		r.Nil(err)

		entry := canonicalEntries[idx]
		absPath := iterator.Dir() + "/" + info.Name()

		r.True(strings.HasSuffix(absPath, entry.path))
		r.Equal(entry.isDir, info.IsDir())

		idx += 1
	}
}

func TestUploadFiles(t *testing.T) {
	r := require.New(t)

	s := test.RunBasicJetStreamServer(t)
	defer test.ShutdownJSServerAndRemoveStorage(t, s)

	srv, lis := blobServer(s, t)
	defer srv.Stop()

	conn := test.GrpcConn(lis, t)
	client := pb.NewBlobServiceClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rootPath := "../../test/testdata/dfi/canonical"
	digests, err := UploadFiles(ctx, rootPath, client)

	r.Nil(err)
	r.NotNil(digests)

	r.Equal(len(canonicalFilePaths), len(digests))

	for _, relPath := range canonicalFilePaths {
		absPath, err := filepath.Abs(rootPath + "/" + relPath)
		r.Nil(err)

		future := digests[absPath]
		r.NotNil(future)

		select {
		case <-ctx.Done():
			t.Fatal("ctx cancelled", "err", ctx.Err())
		case digest := <-future.Get():
			// todo generate a blake3 hash of the local file and compare
			r.Len(digest, 32)
		}
	}
}
