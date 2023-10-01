package store

import (
	"bytes"
	"context"
	"io"

	"github.com/SaveTheRbtz/fastcdc-go"
	pb "github.com/brianmcgee/nvix/protos"
	"github.com/charmbracelet/log"
	"github.com/djherbis/buffer"
	"github.com/djherbis/nio/v3"
	"github.com/golang/protobuf/proto"
	"github.com/juju/errors"

	"lukechampine.com/blake3"
)

var ChunkOptions = fastcdc.Options{
	MinSize:     4 * 1024 * 1024,
	AverageSize: 6 * 1024 * 1024,
	MaxSize:     (8 * 1024 * 1024) - 1024, // we allow 1kb for headers to avoid max message size
}

type CdcStore struct {
	Meta   Store
	Chunks Store
}

func (c *CdcStore) getMeta(key string, ctx context.Context) (*pb.BlobMeta, error) {
	reader, err := c.Meta.Get(key, ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = reader.Close()
	}()

	b, err := io.ReadAll(reader)
	if err != nil {
		return nil, errors.Annotate(err, "failed to read bytes")
	}

	var meta pb.BlobMeta
	if err = proto.Unmarshal(b, &meta); err != nil {
		return nil, errors.Annotate(err, "failed to unmarshal blob metadata")
	}
	return &meta, nil
}

func (c *CdcStore) Get(key string, ctx context.Context) (io.ReadCloser, error) {
	meta, err := c.getMeta(key, ctx)
	if err != nil {
		return nil, err
	}

	// create a buffer with an average of 2 chunks
	buf := buffer.New(int64(ChunkOptions.AverageSize))
	r, w := nio.Pipe(buf)

	go c.readChunks(meta, w, ctx)

	return r, nil
}

func (c *CdcStore) readChunks(meta *pb.BlobMeta, writer *nio.PipeWriter, ctx context.Context) {
	var err error
	var reader io.Reader

	defer func() {
		_ = writer.CloseWithError(err)
	}()

	for _, chunk := range meta.Chunks {
		digest := Digest(chunk.Digest)
		if reader, err = c.Chunks.Get(digest.String(), ctx); err != nil {
			log.Error("failed to retrieve chunk", "digest", digest, "error", err)
			return
		} else if _, err = io.Copy(writer, reader); err != nil {
			log.Error("failed to copy chunk", "digest", digest, "error", err)
			return
		}
	}

	return
}

func (c *CdcStore) Put(reader io.ReadCloser, ctx context.Context) (*Digest, error) {
	hasher := blake3.New(32, nil)
	chunkHasher := blake3.New(32, nil)

	chunker, err := fastcdc.NewChunker(io.TeeReader(reader, hasher), ChunkOptions)
	if err != nil {
		return nil, errors.Annotate(err, "failed to create a chunker")
	}

	var blobDigest Digest
	blobMeta := pb.BlobMeta{}

	for {
		chunk, err := chunker.Next()
		if err == io.EOF {
			// no more chunks
			blobDigest = Digest(hasher.Sum(nil))
			break
		} else if err != nil {
			return nil, errors.Annotate(err, "failed to read next chunk")
		}

		_, err = io.Copy(chunkHasher, bytes.NewReader(chunk.Data))
		if err != nil {
			return nil, errors.Annotate(err, "failed to write into chunk hasher")
		}

		chunkDigest := Digest(chunkHasher.Sum(nil))
		err = c.Chunks.Put(chunkDigest.String(), io.NopCloser(bytes.NewReader(chunk.Data)), ctx)
		if err != nil {
			return nil, errors.Annotate(err, "failed to put chunk")
		}

		blobMeta.Chunks = append(blobMeta.Chunks, &pb.BlobMeta_ChunkMeta{
			Digest: chunkDigest[:],
			Size:   uint32(len(chunk.Data)),
		})

		chunkHasher.Reset()
	}

	b, err := proto.Marshal(&blobMeta)
	if err != nil {
		return nil, errors.Annotate(err, "failed to marshal blob meta")
	}

	if err = c.Meta.Put(blobDigest.String(), io.NopCloser(bytes.NewReader(b)), ctx); err != nil {
		return nil, errors.Annotate(err, "failed to put blob meta")
	}

	log.Debug("put complete", "digest", blobDigest, "chunks", len(blobMeta.Chunks))

	return &blobDigest, nil
}

func (c *CdcStore) Delete(key string, ctx context.Context) error {
	meta, err := c.getMeta(key, ctx)
	if err != nil {
		return err
	}

	if err = c.Meta.Delete(key, ctx); err != nil {
		return errors.Annotate(err, "failed to delete metadata entry")
	}

	// delete all the chunks
	for _, chunk := range meta.Chunks {
		digest := Digest(chunk.Digest)
		if err = c.Chunks.Delete(digest.String(), ctx); err != nil {
			return errors.Annotatef(err, "failed to delete chunk: %v", digest)
		}
	}

	return nil
}
