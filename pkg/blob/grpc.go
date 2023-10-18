package blob

import (
	"context"
	"io"

	capb "code.tvl.fyi/tvix/castore-go"

	"github.com/brianmcgee/nvix/pkg/store"
	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func NewServer(conn *nats.Conn) (*Server, error) {
	return &Server{
		conn: conn,
		store: &store.CdcStore{
			Meta:   NewMetaStore(conn),
			Chunks: NewChunkStore(conn),
		},
	}, nil
}

type Server struct {
	capb.UnimplementedBlobServiceServer
	conn *nats.Conn

	store *store.CdcStore
}

func (s *Server) Stat(ctx context.Context, request *capb.StatBlobRequest) (*capb.BlobMeta, error) {
	l := log.WithPrefix("blob.stat")
	l.Debug("executing", "digest", store.Digest(request.GetDigest()))

	digest := store.Digest(request.Digest)
	ok, err := s.store.Stat(digest, ctx)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, status.Errorf(codes.NotFound, "blob not found: %v", digest)
	}
	// castore blob meta is empty for now
	return &capb.BlobMeta{}, nil
}

func (s *Server) Read(request *capb.ReadBlobRequest, server capb.BlobService_ReadServer) error {
	l := log.WithPrefix("blob.read")

	ctx, cancel := context.WithCancel(server.Context())
	defer cancel()

	digest := store.Digest(request.Digest)
	reader, err := s.store.Get(digest, ctx)

	if err == store.ErrKeyNotFound {
		return status.Errorf(codes.NotFound, "blob not found: %v", digest)
	} else if err != nil {
		l.Error("failed to get blob", "digest", digest, "error", err)
		return status.Error(codes.Internal, "internal error")
	}

	// we want to stay just under the 4MB max size restriction in gRPC
	sendBuf := make([]byte, (4*1024*1024)-5)

	for {
		n, err := reader.Read(sendBuf)
		if err == io.EOF {
			_ = reader.Close()
			break
		} else if err != nil {
			l.Errorf("failed to read next chunk: %v", err)
			return status.Error(codes.Internal, "internal error")
		}

		if err = server.Send(&capb.BlobChunk{
			Data: sendBuf[:n],
		}); err != nil {
			l.Errorf("failed to send blob chunk to client: %v", err)
			return err
		}
	}

	return nil
}

func (s *Server) GetByDigest(digest store.Digest, ctx context.Context) (io.ReadCloser, error) {
	return s.store.Get(digest, ctx)
}

func (s *Server) Put(server capb.BlobService_PutServer) (err error) {
	l := log.WithPrefix("blob.put")

	ctx, cancel := context.WithCancel(server.Context())
	defer cancel()

	reader := blobReader{server: server}

	digest, err := s.store.Put(&reader, ctx)
	if err != nil {
		l.Error("failed to put blob", "error", err)
		return status.Error(codes.Internal, "internal error")
	}

	return server.SendAndClose(&capb.PutBlobResponse{
		Digest: digest[:],
	})
}

type blobReader struct {
	server capb.BlobService_PutServer
	chunk  *capb.BlobChunk
}

func (b *blobReader) Read(p []byte) (n int, err error) {
	if b.chunk == nil {
		if b.chunk, err = b.server.Recv(); err != nil {
			return
		}
	}

	n = copy(p, b.chunk.Data)
	if n == len(b.chunk.Data) {
		b.chunk = nil
	} else {
		b.chunk.Data = b.chunk.Data[n:]
	}

	return
}

func (b *blobReader) Close() error {
	return nil
}
