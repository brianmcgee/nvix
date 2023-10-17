package directory

import (
	"context"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	pb "code.tvl.fyi/tvix/castore/protos"

	"github.com/brianmcgee/nvix/pkg/blob"
	"github.com/brianmcgee/nvix/pkg/test"
	"github.com/nats-io/nats-server/v2/server"
	"google.golang.org/grpc"

	"github.com/charmbracelet/log"
	"github.com/stretchr/testify/require"
)

var (
	canonicalEntries = []struct {
		dir   string
		name  string
		depth int
		isDir bool
	}{
		{dir: "./a", name: "c.txt", depth: 2, isDir: false},
		{dir: "./a", name: "d.txt", depth: 2, isDir: false},
		{dir: "./a/e", name: "f.txt", depth: 3, isDir: false},
		{dir: "./a", name: "e", depth: 2, isDir: true},
		{dir: ".", name: "a", depth: 1, isDir: true},
		{dir: "./b", name: "g.txt", depth: 2, isDir: false},
		{dir: "./b", name: "h.txt", depth: 2, isDir: false},
		{dir: ".", name: "b", depth: 1, isDir: true},
		{dir: "", name: ".", depth: 0, isDir: true},
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

	err := os.Chdir("../../test/testdata/dfi/canonical")
	r.Nil(err)

	iterator, err := NewDepthFirstIterator(".")
	r.Nil(err)

	idx := 0

	for {
		info, err := iterator.Next()
		if err == io.EOF {
			return
		}

		r.Nil(err)

		entry := canonicalEntries[idx]

		r.Equal(entry.dir, iterator.Dir())
		r.Equal(entry.name, info.Name())
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

	digests, err := UploadFiles(
		ctx,
		"../../test/testdata/dfi/canonical",
		func(path string) ([]byte, error) {
			file, err := os.Open(path)
			if err != nil {
				return nil, err
			}

			// 1Mb chunks
			chunk := make([]byte, 1024*1024)

			put, err := client.Put(ctx)
			if err != nil {
				return nil, err
			}

			for {
				n, err := file.Read(chunk)
				if err == io.EOF {
					break
				} else if err != nil {
					return nil, err
				}

				if err = put.Send(&pb.BlobChunk{
					Data: chunk[:n],
				}); err != nil {
					return nil, err
				}
			}

			resp, err := put.CloseAndRecv()
			if err != nil {
				return nil, err
			}

			return resp.Digest, nil
		})

	r.Nil(err)
	r.NotNil(digests)

	r.Equal(len(canonicalFilePaths), len(digests))

	for _, relPath := range canonicalFilePaths {
		absPath, err := filepath.Abs("../../test/testdata/dfi/canonical/" + relPath)
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
