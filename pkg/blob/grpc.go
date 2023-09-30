package blob

import (
	"bytes"
	"context"
	"io"

	"github.com/brianmcgee/nvix/pkg/store"

	capb "code.tvl.fyi/tvix/castore/protos"
	pb "github.com/brianmcgee/nvix/protos"

	"github.com/SaveTheRbtz/fastcdc-go"
	"github.com/charmbracelet/log"
	"github.com/juju/errors"
	"github.com/nats-io/nats.go"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"lukechampine.com/blake3"
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
		conn:   conn,
		blobs:  store.NewBlobStore(conn),
		chunks: store.NewChunkStore(conn),
	}, nil
}

type service struct {
	capb.UnimplementedBlobServiceServer
	conn *nats.Conn

	blobs  *store.Blobs
	chunks *store.Chunks
}

func (b *service) getBlobMeta(digest []byte, ctx context.Context) (*pb.BlobMeta, error) {
	blob, err := b.blobs.Get(store.Digest(digest), ctx)
	if err == store.ErrKeyNotFound {
		return nil, status.Error(codes.NotFound, "blob not found")
	} else if err != nil {
		log.Debugf("failed to retrieve blob: %v", err)
		return nil, status.Error(codes.Internal, "internal error")
	}
	return blob, nil
}

func (b *service) Stat(ctx context.Context, request *capb.StatBlobRequest) (*capb.BlobMeta, error) {
	_, err := b.getBlobMeta(request.Digest, ctx)
	if err != nil {
		return nil, err
	}
	// castore blob meta is empty for now
	return &capb.BlobMeta{}, nil
}

func (b *service) Read(request *capb.ReadBlobRequest, server capb.BlobService_ReadServer) error {
	ctx, cancel := context.WithCancel(server.Context())
	defer cancel()

	meta, err := b.getBlobMeta(request.Digest, server.Context())
	if err != nil {
		return err
	}

	// we want to stay just under the 4MB max size restriction in gRPC
	sendBuf := make([]byte, (4*1024*1024)-1024)

	for _, c := range meta.Chunks {
		digest := store.Digest(c.Digest)
		reader, err := b.chunks.Get(digest, ctx)

		if err == store.ErrKeyNotFound {
			return status.Errorf(codes.NotFound, "chunk not found: %v", digest)
		}

		for {
			n, err := reader.Read(sendBuf)
			if err == io.EOF {
				_ = reader.Close()
				break
			} else if err != nil {
				log.Errorf("failed to read next send chunk: %v", err)
				return status.Error(codes.Internal, "internal error")
			}

			if err = server.Send(&capb.BlobChunk{
				Data: sendBuf[:n],
			}); err != nil {
				log.Errorf("failed to send blob chunk to client: %v", err)
				return err
			}
		}
	}

	return nil
}

func (b *service) Put(server capb.BlobService_PutServer) (err error) {
	ctx := server.Context()

	hasher := blake3.New(32, nil)
	reader, writer := io.Pipe()

	chunker, err := fastcdc.NewChunker(io.TeeReader(reader, hasher), ChunkOptions)
	if err != nil {
		log.Error("failed to create a chunker", "error", err)
		return status.Error(codes.Internal, "internal error")
	}

	var blobDigest store.Digest
	blobMeta := pb.BlobMeta{}

	eg, ctx := errgroup.WithContext(server.Context())

	// pull chunks from the server and insert into the pipeline
	eg.Go(func() error {
		defer func() {
			_ = writer.CloseWithError(ctx.Err())
		}()

		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				c, err := server.Recv()
				if err == io.EOF {
					// finished receiving chunks
					return nil
				} else if err != nil {
					return errors.Annotate(err, "failed to receive next next chunk")
				}

				n, err := io.Copy(writer, bytes.NewReader(c.Data))
				if err != nil {
					return errors.Annotate(err, "failed to write next chunk into processing pipe")
				}

				log.Debugf("%d bytes added to the pipeline", n)
			}
		}
	})

	// chunk the input
	eg.Go(func() error {
		chunkHasher := blake3.New(32, nil)

		for {
			c, err := chunker.Next()
			if err == io.EOF {
				// no more chunks
				blobDigest = store.Digest(hasher.Sum(nil))
				return nil
			} else if err != nil {
				return errors.Annotate(err, "failed to read next chunk")
			}

			n, err := io.Copy(chunkHasher, bytes.NewReader(c.Data))
			if err != nil {
				return errors.Annotate(err, "failed to write into chunk hasher")
			} else if n == 0 {
				// finished reading
				return nil
			}

			chunkDigest := store.Digest(chunkHasher.Sum(nil))

			err = b.chunks.Put(chunkDigest, io.NopCloser(bytes.NewReader(c.Data)), ctx)
			if err != nil {
				return errors.Annotate(err, "failed to put chunk")
			}

			blobMeta.Chunks = append(blobMeta.Chunks, &pb.BlobMeta_ChunkMeta{
				Digest: chunkDigest[:],
				Size:   uint32(len(c.Data)),
			})

			chunkHasher.Reset()
		}
	})

	if err = eg.Wait(); err != nil {
		log.Errorf("failed to process input: %v", err)
		return status.Error(codes.Internal, "failed to process input")
	}

	if err = b.blobs.Put(blobDigest, &blobMeta, ctx); err != nil {
		log.Errorf("failed to put blob meta: %v", err)
		return status.Error(codes.Internal, "internal error")
	}

	log.Debug("put complete", "digest", blobDigest, "chunks", len(blobMeta.Chunks))

	return server.SendAndClose(&capb.PutBlobResponse{
		Digest: blobDigest[:],
	})
}
