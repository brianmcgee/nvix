package store

import (
	"bytes"
	pb "code.tvl.fyi/tvix/store/protos"
	"context"
	"encoding/base64"
	"github.com/charmbracelet/log"
	"github.com/jotfs/fastcdc-go"
	"github.com/juju/errors"
	"github.com/nats-io/nats.go"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"io"
	"lukechampine.com/blake3"
)

var (
	ChunkOptions = fastcdc.Options{
		MinSize:     128 * 1024,
		AverageSize: 512 * 1024,
		MaxSize:     1023 * 1024, // we allow 1kb for headers to avoid the default 1MB max message size
	}
)

func NewBlobService(conn *nats.Conn) (pb.BlobServiceServer, error) {

	js, err := conn.JetStream()
	if err != nil {
		return nil, errors.Annotate(err, "failed to create a JetStream context")
	}

	if _, err := js.AddStream(&nats.StreamConfig{
		Name:        "chunks",
		Subjects:    []string{"TVIX.STORE.CHUNK.>"},
		AllowRollup: true,
		AllowDirect: true,
	}); err != nil {
		return nil, errors.Annotate(err, "failed to create chunks stream")
	}

	if _, err := js.AddStream(&nats.StreamConfig{
		Name:        "blobs",
		Subjects:    []string{"TVIX.STORE.BLOB.>"},
		AllowRollup: true,
		AllowDirect: true,
	}); err != nil {
		return nil, errors.Annotate(err, "failed to create chunks stream")
	}

	return &blobService{
		conn: conn,
	}, nil
}

type blobService struct {
	pb.UnimplementedBlobServiceServer
	conn *nats.Conn
}

func (b *blobService) Stat(ctx context.Context, request *pb.StatBlobRequest) (*pb.BlobMeta, error) {
	// TODO implement me
	panic("implement me")
}

func (b *blobService) Read(request *pb.ReadBlobRequest, server pb.BlobService_ReadServer) error {
	js, err := b.conn.JetStream()
	if err != nil {
		log.Errorf("failed to create a JetStream context: %v", err)
		return status.Error(codes.Internal, "internal error")
	}

	subject := BlobSubjectForDigest(request.Digest)

	blobMsg, err := js.GetLastMsg("blobs", subject)
	if err == nats.ErrMsgNotFound {
		return status.Error(codes.NotFound, "blob not found")
	} else if err != nil {
		log.Errorf("failed to retrieve blob: %v", subject)
		return status.Error(codes.Internal, "internal error")
	}

	chunkDigest := make([]byte, 32)
	chunkDigestReader := bytes.NewReader(blobMsg.Data)

	for {
		n, err := chunkDigestReader.Read(chunkDigest)
		if err == io.EOF {
			break
		} else if n != 32 {
			log.Errorf("malformed blob data, expected 32 bytes but read %v", n)
			return status.Error(codes.Internal, "internal error")
		}

		chunkMsg, err := js.GetLastMsg("chunks", ChunkSubjectForDigest(chunkDigest))
		if err == nats.ErrMsgNotFound {
			return status.Errorf(codes.NotFound, "chunk not found: %v", base64.StdEncoding.EncodeToString(chunkDigest))
		}
		if err = server.Send(&pb.BlobChunk{
			Data: chunkMsg.Data,
		}); err != nil {
			log.Errorf("failed to send blob chunk to client: %v", err)
			return err
		}
	}

	return nil
}

func (b *blobService) Put(server pb.BlobService_PutServer) (err error) {
	allReader, allWriter := io.Pipe()
	chunkReader, chunkWriter := io.Pipe()

	var chunkDigests []byte
	var blobDigest []byte

	multiWriter := io.MultiWriter(allWriter, chunkWriter)

	eg, ctx := errgroup.WithContext(server.Context())

	// pull chunks from the server and insert into the pipeline
	eg.Go(func() error {
		defer func() {
			_ = allWriter.CloseWithError(ctx.Err())
			_ = chunkWriter.CloseWithError(ctx.Err())
		}()

		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				chunk, err := server.Recv()
				if err == io.EOF {
					// finished receiving chunks
					return nil
				} else if err != nil {
					return errors.Annotate(err, "failed to receive next next chunk")
				}

				n, err := io.Copy(multiWriter, bytes.NewReader(chunk.Data))
				if err != nil {
					return errors.Annotate(err, "failed to write next chunk into processing pipe")
				}

				log.Debugf("%d bytes add to the pipeline", n)
			}
		}
	})

	// chunk the input
	eg.Go(func() error {
		chunker, err := fastcdc.NewChunker(chunkReader, ChunkOptions)
		if err != nil {
			log.Error("failed to create a chunker", "error", err)
			return status.Error(codes.Internal, "internal error")
		}

		js, err := b.conn.JetStream()
		if err != nil {
			return errors.Annotate(err, "failed to create a JetStream context")
		}

		hasher := blake3.New(32, nil)

		for {
			chunk, err := chunker.Next()
			if err == io.EOF {
				// no more chunks
				return nil
			} else if err != nil {
				return errors.Annotate(err, "failed to read next chunk")
			}

			n, err := io.Copy(hasher, bytes.NewReader(chunk.Data))
			if err != nil {
				return errors.Annotate(err, "failed to write into chunk hasher")
			} else if n == 0 {
				// finished reading
				return nil
			}

			chunkDigest := hasher.Sum(nil)
			chunkId := base64.StdEncoding.EncodeToString(chunkDigest)

			subject := ChunkSubject(chunkId)
			msg := nats.NewMsg(subject)
			msg.Header.Set(nats.MsgRollup, nats.MsgRollupSubject)
			msg.Data = chunk.Data

			if _, err = js.PublishMsg(msg); err != nil {
				return errors.Annotate(err, "failed to publish chunk into NATS")
			}

			chunkDigests = append(chunkDigests, chunkDigest...)
			hasher.Reset()
		}
	})

	eg.Go(func() error {
		hasher := blake3.New(32, nil)
		for {
			n, err := io.Copy(hasher, allReader)
			if err != nil {
				return errors.Annotate(err, "failed to write next bytes for calculating overall hash")
			} else if n == 0 {
				// finished reading
				blobDigest = hasher.Sum(nil)
				return nil
			}
		}
	})

	if err = eg.Wait(); err != nil {
		log.Errorf("failed to process input: %v", err)
		return status.Error(codes.Internal, "failed to process input")
	}

	js, err := b.conn.JetStream()
	if err != nil {
		log.Error("failed to create a JetStream context", "error", err)
		return status.Error(codes.Internal, "internal error")
	}

	id := base64.StdEncoding.EncodeToString(blobDigest)

	msg := nats.NewMsg(BlobSubject(id))
	msg.Header.Set(nats.MsgRollup, nats.MsgRollupSubject)
	msg.Data = chunkDigests

	_, err = js.PublishMsg(msg)
	if err != nil {
		log.Errorf("failed to publish chunk, id: %v", err)
		return status.Error(codes.Internal, "internal error")
	}

	log.Debug("put complete", "id", msg.Subject, "chunks", len(chunkDigests))

	return server.SendAndClose(&pb.PutBlobResponse{
		Digest: blobDigest,
	})
}
