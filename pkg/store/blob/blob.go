package blob

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"

	"github.com/brianmcgee/nvix/pkg/store/subject"

	capb "code.tvl.fyi/tvix/castore/protos"
	pb "github.com/brianmcgee/nvix/protos"

	"github.com/charmbracelet/log"
	"github.com/golang/protobuf/proto"
	"github.com/jotfs/fastcdc-go"
	"github.com/juju/errors"
	"github.com/nats-io/nats.go"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"lukechampine.com/blake3"
)

var ChunkOptions = fastcdc.Options{
	MinSize:     4 * 1024 * 1024,
	AverageSize: 6 * 1024 * 1024,
	MaxSize:     (8 * 1024 * 1024) - 1024, // we allow 1kb for headers to avoid max message size
}

func NewService(conn *nats.Conn) (capb.BlobServiceServer, error) {
	js, err := conn.JetStream()
	if err != nil {
		return nil, errors.Annotate(err, "failed to create a JetStream context")
	}

	if _, err := js.AddStream(&nats.StreamConfig{
		Name:        "chunks",
		Subjects:    []string{subject.ChunkPrefix() + ".>"},
		AllowRollup: true,
		AllowDirect: true,
	}); err != nil {
		return nil, errors.Annotate(err, "failed to create chunks stream")
	}

	if _, err := js.AddStream(&nats.StreamConfig{
		Name:        "blobs",
		Subjects:    []string{subject.BlobPrefix() + ".>"},
		AllowRollup: true,
		AllowDirect: true,
	}); err != nil {
		return nil, errors.Annotate(err, "failed to create chunks stream")
	}

	return &service{
		conn: conn,
	}, nil
}

type service struct {
	capb.UnimplementedBlobServiceServer
	conn *nats.Conn
}

func (b *service) getBlobMeta(_ context.Context, js nats.JetStreamContext, digest []byte) (*pb.BlobMeta, error) {
	subj := subject.BlobByDigest(digest)
	blobMsg, err := js.GetLastMsg("blobs", subj)
	if err == nats.ErrMsgNotFound {
		return nil, status.Error(codes.NotFound, "blob not found")
	} else if err != nil {
		log.Debugf("failed to retrieve blob: %v", subj)
		return nil, status.Error(codes.Internal, "internal error")
	}

	blobMeta := pb.BlobMeta{}
	if err = proto.Unmarshal(blobMsg.Data, &blobMeta); err != nil {
		log.Errorf("failed to unmarshal blob meta: %v", err)
		return nil, status.Error(codes.Internal, "internal error")
	}

	return &blobMeta, nil
}

func (b *service) Stat(ctx context.Context, request *capb.StatBlobRequest) (*capb.BlobMeta, error) {
	js, err := b.conn.JetStream()
	if err != nil {
		log.Errorf("failed to create a JetStream context: %v", err)
		return nil, status.Error(codes.Internal, "internal error")
	}

	_, err = b.getBlobMeta(ctx, js, request.Digest)
	if err != nil {
		return nil, err
	}

	// castore blob meta is now empty
	return &capb.BlobMeta{}, nil
}

func (b *service) Read(request *capb.ReadBlobRequest, server capb.BlobService_ReadServer) error {
	js, err := b.conn.JetStream()
	if err != nil {
		log.Errorf("failed to create a JetStream context: %v", err)
		return status.Error(codes.Internal, "internal error")
	}

	meta, err := b.getBlobMeta(server.Context(), js, request.Digest)
	if err != nil {
		return err
	}

	// we want to stay just under the 4MB max size restriction in gRPC
	sendBuf := make([]byte, (4*1024*1024)-1024)

	for _, chunk := range meta.Chunks {
		chunkMsg, err := js.GetLastMsg("chunks", subject.ChunkByDigest(chunk.Digest))
		if err == nats.ErrMsgNotFound {
			return status.Errorf(codes.NotFound, "chunk not found: %v", base64.StdEncoding.EncodeToString(chunk.Digest))
		}

		reader := bytes.NewReader(chunkMsg.Data)
		for {
			n, err := reader.Read(sendBuf)
			if err == io.EOF {
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

	var blobDigest []byte
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
				chunk, err := server.Recv()
				if err == io.EOF {
					// finished receiving chunks
					return nil
				} else if err != nil {
					return errors.Annotate(err, "failed to receive next next chunk")
				}

				n, err := io.Copy(writer, bytes.NewReader(chunk.Data))
				if err != nil {
					return errors.Annotate(err, "failed to write next chunk into processing pipe")
				}

				log.Debugf("%d bytes add to the pipeline", n)
			}
		}
	})

	// chunk the input
	eg.Go(func() error {
		js, err := b.conn.JetStream()
		if err != nil {
			return errors.Annotate(err, "failed to create a JetStream context")
		}

		chunkHasher := blake3.New(32, nil)

		for {
			chunk, err := chunker.Next()
			if err == io.EOF {
				// no more chunks
				blobDigest = hasher.Sum(nil)
				return nil
			} else if err != nil {
				return errors.Annotate(err, "failed to read next chunk")
			}

			n, err := io.Copy(chunkHasher, bytes.NewReader(chunk.Data))
			if err != nil {
				return errors.Annotate(err, "failed to write into chunk hasher")
			} else if n == 0 {
				// finished reading
				return nil
			}

			chunkDigest := chunkHasher.Sum(nil)
			chunkId := base64.StdEncoding.EncodeToString(chunkDigest)

			msg := nats.NewMsg(subject.ChunkById(chunkId))
			msg.Header.Set(nats.MsgRollup, nats.MsgRollupSubject)
			msg.Data = chunk.Data

			if _, err = js.PublishMsg(msg); err != nil {
				return errors.Annotate(err, "failed to publish chunk into NATS")
			}

			blobMeta.Chunks = append(blobMeta.Chunks, &pb.BlobMeta_ChunkMeta{
				Digest: chunkDigest,
				Size:   uint32(len(chunk.Data)),
			})

			chunkHasher.Reset()
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

	msg := nats.NewMsg(subject.BlobById(id))
	msg.Header.Set(nats.MsgRollup, nats.MsgRollupSubject)
	msg.Data, err = proto.Marshal(&blobMeta)

	if err != nil {
		log.Errorf("failed to marshal blob meta: %v", err)
		return status.Error(codes.Internal, "internal error")
	}

	_, err = js.PublishMsg(msg)
	if err != nil {
		log.Errorf("failed to publish chunk, id: %v", err)
		return status.Error(codes.Internal, "internal error")
	}

	log.Debug("put complete", "id", msg.Subject, "chunks", len(blobMeta.Chunks))

	return server.SendAndClose(&capb.PutBlobResponse{
		Digest: blobDigest,
	})
}
