package blob

import (
	"context"
	"io"

	capb "code.tvl.fyi/tvix/castore/protos"

	"github.com/brianmcgee/nvix/pkg/store"
	"github.com/charmbracelet/log"
	"github.com/juju/errors"
	"github.com/nats-io/nats.go"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func NewService(conn *nats.Conn) (capb.BlobServiceServer, error) {
	js, err := conn.JetStream()
	if err != nil {
		return nil, errors.Annotate(err, "failed to create a JetStream context")
	}

	if _, err := js.AddStream(&store.DiskBasedStreamConfig); err != nil {
		return nil, errors.Annotate(err, "failed to create disk based stream")
	}

	if _, err := js.AddStream(&store.MemoryBasedStreamConfig); err != nil {
		return nil, errors.Annotate(err, "failed to create memory based stream")
	}

	return &service{
		conn: conn,
		store: &store.CdcStore{
			Meta:   store.NewMetaStore(conn),
			Chunks: store.NewChunkStore(conn),
		},
	}, nil
}

type service struct {
	capb.UnimplementedBlobServiceServer
	conn *nats.Conn

	store *store.CdcStore
}

func (s *service) Stat(ctx context.Context, request *capb.StatBlobRequest) (*capb.BlobMeta, error) {
	digest := store.Digest(request.Digest)
	ok, err := s.store.Stat(digest.String(), ctx)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, status.Errorf(codes.NotFound, "blob not found: %v", digest)
	}
	// castore blob meta is empty for now
	return &capb.BlobMeta{}, nil
}

func (s *service) Read(request *capb.ReadBlobRequest, server capb.BlobService_ReadServer) error {
	ctx, cancel := context.WithCancel(server.Context())
	defer cancel()

	digest := store.Digest(request.Digest)
	reader, err := s.store.Get(digest.String(), ctx)

	if err == store.ErrKeyNotFound {
		return status.Errorf(codes.NotFound, "blob not found: %v", digest)
	} else if err != nil {
		log.Error("failed to get blob", "digest", digest, "error", err)
		return status.Error(codes.Internal, "internal error")
	}

	// we want to stay just under the 4MB max size restriction in gRPC
	sendBuf := make([]byte, (4*1024*1024)-1024)

	for {
		n, err := reader.Read(sendBuf)
		if err == io.EOF {
			_ = reader.Close()
			break
		} else if err != nil {
			log.Errorf("failed to read next chunk: %v", err)
			return status.Error(codes.Internal, "internal error")
		}

		if err = server.Send(&capb.BlobChunk{
			Data: sendBuf[:n],
		}); err != nil {
			log.Errorf("failed to send blob chunk to client: %v", err)
			return err
		}
	}

	return nil
}

func (s *service) Put(server capb.BlobService_PutServer) (err error) {
	ctx, cancel := context.WithCancel(server.Context())
	defer cancel()

	reader := blobReader{server: server}

	digest, err := s.store.Put(&reader, ctx)
	if err != nil {
		log.Error("failed to put blob", "error", err)
		return status.Error(codes.Internal, "internal error")
	}

	return server.SendAndClose(&capb.PutBlobResponse{
		Digest: digest[:],
	})
}

type blobReader struct {
	server capb.BlobService_PutServer
	chunk  []byte
}

func (b *blobReader) Read(p []byte) (n int, err error) {
	if b.chunk == nil {
		var chunk *capb.BlobChunk
		if chunk, err = b.server.Recv(); err != nil {
			return
		}
		b.chunk = chunk.Data
	}

	n = copy(p, b.chunk)
	if n == len(b.chunk) {
		b.chunk = nil
	} else {
		b.chunk = b.chunk[n:]
	}

	return
}

func (b *blobReader) Close() error {
	return nil
}
