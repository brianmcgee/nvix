package store

import (
	"bytes"
	"context"
	"io"

	"github.com/brianmcgee/nvix/pkg/util"
	"golang.org/x/sync/errgroup"

	"github.com/nats-io/nats.go"

	"github.com/SaveTheRbtz/fastcdc-go"
	pb "github.com/brianmcgee/nvix/protos"
	"github.com/charmbracelet/log"
	"github.com/golang/protobuf/proto"
	"github.com/juju/errors"

	"lukechampine.com/blake3"
)

var ChunkOptions = fastcdc.Options{
	MinSize:     1 * 1024 * 1024,
	AverageSize: 4 * 1024 * 1024,
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

	if err == ErrKeyNotFound {
		return nil, err
	} else if err != nil {
		return nil, errors.Annotate(err, "failed to read bytes")
	}

	var meta pb.BlobMeta
	if err = proto.Unmarshal(b, &meta); err != nil {
		return nil, errors.Annotate(err, "failed to unmarshal blob metadata")
	}
	return &meta, nil
}

func (c *CdcStore) List(ctx context.Context) (util.Iterator[io.ReadCloser], error) {
	metaIterator, err := c.Meta.List(ctx)
	if err != nil {
		return nil, err
	}
	return &blobIterator{
		ctx:          ctx,
		chunks:       c.Chunks,
		metaIterator: metaIterator,
	}, nil
}

func (c *CdcStore) Stat(digest Digest, ctx context.Context) (ok bool, err error) {
	return c.Meta.Stat(digest.String(), ctx)
}

func (c *CdcStore) Get(digest Digest, ctx context.Context) (io.ReadCloser, error) {
	meta, err := c.getMeta(digest.String(), ctx)
	if err != nil {
		return nil, err
	}

	return &blobReader{blob: meta, store: c.Chunks, ctx: ctx}, nil
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

	var futures []nats.PubAckFuture

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

		future, err := c.Chunks.PutAsync(chunkDigest.String(), io.NopCloser(bytes.NewReader(chunk.Data)), ctx)
		if err != nil {
			return nil, errors.Annotate(err, "failed to put chunk")
		}

		futures = append(futures, future)

		blobMeta.Chunks = append(blobMeta.Chunks, &pb.BlobMeta_ChunkMeta{
			Digest: chunkDigest[:],
			Size:   uint32(len(chunk.Data)),
		})

		chunkHasher.Reset()
	}

	for _, future := range futures {
		select {
		case <-future.Ok():
		// do nothing
		case err := <-future.Err():
			return nil, errors.Annotate(err, "failed to put chunk")
		}
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

func (c *CdcStore) Delete(digest Digest, ctx context.Context) error {
	meta, err := c.getMeta(digest.String(), ctx)
	if err != nil {
		return err
	}

	if err = c.Meta.Delete(digest.String(), ctx); err != nil {
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

type blobIterator struct {
	ctx          context.Context
	chunks       Store
	metaIterator util.Iterator[io.ReadCloser]
}

func (b *blobIterator) Next() (io.ReadCloser, error) {
	metaReader, err := b.metaIterator.Next()
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = metaReader.Close()
	}()

	metaBytes, err := io.ReadAll(metaReader)
	if err != nil {
		return nil, errors.Annotate(err, "failed to read blob metadata")
	}

	var meta pb.BlobMeta
	if err = proto.Unmarshal(metaBytes, &meta); err != nil {
		return nil, err
	}

	return &blobReader{
		blob:  &meta,
		store: b.chunks,
		ctx:   b.ctx,
	}, nil
}

func (b *blobIterator) Close() error {
	return b.metaIterator.Close()
}

type blobReader struct {
	blob  *pb.BlobMeta
	store Store

	eg      *errgroup.Group
	ctx     context.Context
	readers chan io.ReadCloser

	reader io.ReadCloser
}

func (c *blobReader) Read(p []byte) (n int, err error) {
	if c.eg == nil {
		var ctx context.Context
		c.eg, ctx = errgroup.WithContext(c.ctx)

		c.readers = make(chan io.ReadCloser, 2)

		c.eg.Go(func() error {
			// close channel on return
			defer close(c.readers)

			b := make([]byte, 0)

			for _, chunk := range c.blob.Chunks {
				r, err := c.store.Get(Digest(chunk.Digest).String(), ctx)
				if err != nil {
					return err
				}

				// tickle the reader to force it to fetch the underlying message and is ready for reading
				_, err = r.Read(b)
				if err != nil {
					return err
				}
				c.readers <- r
			}
			return nil
		})
	}

	for {
		if c.reader == nil {
			var ok bool
			c.reader, ok = <-c.readers
			if !ok {
				// channel has been closed
				err = c.eg.Wait()
				if err == nil {
					err = io.EOF
				}
				return
			}
		}

		n, err = c.reader.Read(p)
		if err == io.EOF {
			if err = c.Close(); err != nil {
				return
			}
			c.reader = nil
		} else {
			return
		}
	}
}

func (c *blobReader) Close() error {
	// do nothing
	return nil
}
