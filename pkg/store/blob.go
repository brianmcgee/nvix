package store

import (
	"bytes"
	pb "code.tvl.fyi/tvix/store/protos"
	"context"
	"encoding/base64"
	"fmt"
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
		AverageSize: 256 * 1024,
		MaxSize:     512 * 1024,
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

	id := base64.StdEncoding.EncodeToString(request.Digest)
	subject := fmt.Sprintf("TVIX.STORE.BLOB.%s", id)

	sub, err := js.SubscribeSync(subject, nats.DeliverAll())
	if err != nil {
		return status.Error(codes.Internal, "internal error")
	}

	for {
		msg, err := sub.NextMsgWithContext(server.Context())
		if err != nil {
			log.Errorf("failed to get next chunk: %v", err)
		}

		chunkId := string(msg.Data)
		chunkMsg, err := js.GetLastMsg("chunks", fmt.Sprintf("TVIX.STORE.CHUNK.%s", chunkId))
		if err == nats.ErrMsgNotFound {
			return status.Errorf(codes.NotFound, "chunk not found: %v", chunkId)
		}
		if err = server.Send(&pb.BlobChunk{
			Data: chunkMsg.Data,
		}); err != nil {
			log.Errorf("failed to send blob chunk to client: %v", err)
			return err
		}

		meta, err := msg.Metadata()
		if err != nil {
			log.Errorf("failed to retrieve message metadata: %v", err)
			return status.Error(codes.Internal, "internal error")
		} else if meta.NumPending == 0 {
			// we have finished reading all the chunks
			break
		}
	}

	return nil
}

func (b *blobService) Put(server pb.BlobService_PutServer) (err error) {
	allReader, allWriter := io.Pipe()
	chunkReader, chunkWriter := io.Pipe()

	var chunkIds []string
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

			digest := hasher.Sum(nil)
			chunkId := base64.StdEncoding.EncodeToString(digest)

			subject := fmt.Sprintf("TVIX.STORE.CHUNK.%s", chunkId)
			msg := nats.NewMsg(subject)
			msg.Header.Set(nats.MsgRollup, nats.MsgRollupSubject)
			msg.Data = chunk.Data

			if _, err = js.PublishMsg(msg); err != nil {
				return errors.Annotate(err, "failed to publish chunk into NATS")
			}

			chunkIds = append(chunkIds, chunkId)
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
	subject := fmt.Sprintf("TVIX.STORE.BLOB.%s", id)

	for _, chunkId := range chunkIds {
		_, err := js.PublishAsync(subject, []byte(chunkId))
		if err != nil {
			log.Errorf("failed to publish chunk id: %v", err)
			return status.Error(codes.Internal, "internal error")
		}
	}

	<-js.PublishAsyncComplete()

	log.Debug("put complete", "id", id, "chunks", len(chunkIds))

	return server.SendAndClose(&pb.PutBlobResponse{
		Digest: blobDigest,
	})
}
