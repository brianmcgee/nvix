package store

import (
	"bytes"
	"context"
	"io"

	pb "github.com/brianmcgee/nvix/protos"
	"github.com/golang/protobuf/proto"
	"github.com/juju/errors"
	"github.com/nats-io/nats.go"
)

type Blobs struct {
	store Store
}

func (s *Blobs) Get(digest Digest, ctx context.Context) (*pb.BlobMeta, error) {
	reader, err := s.GetRaw(digest, ctx)
	if err != nil {
		return nil, err
	}
	b, err := io.ReadAll(reader)
	if err != nil {
		return nil, errors.Annotate(err, "failed to read blob meta")
	}
	defer func() {
		_ = reader.Close()
	}()
	var meta pb.BlobMeta
	if err = proto.Unmarshal(b, &meta); err != nil {
		return nil, errors.Annotate(err, "failed to unmarshal blob meta")
	}

	return &meta, nil
}

func (s *Blobs) GetRaw(digest Digest, ctx context.Context) (io.ReadCloser, error) {
	return s.store.Get(digest.String(), ctx)
}

func (s *Blobs) Put(digest Digest, blob *pb.BlobMeta, ctx context.Context) error {
	b, err := proto.Marshal(blob)
	if err != nil {
		return errors.Annotate(err, "failed to marshal blob meta")
	}
	return s.store.Put(digest.String(), io.NopCloser(bytes.NewReader(b)), ctx)
}

func (s *Blobs) PutRaw(digest Digest, reader io.ReadCloser, ctx context.Context) error {
	return s.store.Put(digest.String(), reader, ctx)
}

func (s *Blobs) Delete(digest Digest, ctx context.Context) error {
	return s.store.Delete(digest.String(), ctx)
}

func NewBlobStore(conn *nats.Conn) *Blobs {
	diskPrefix := DiskBasedStreamConfig.Subjects[0]
	diskPrefix = diskPrefix[:len(diskPrefix)-2]

	memoryPrefix := MemoryBasedStreamConfig.Subjects[0]
	memoryPrefix = memoryPrefix[:len(memoryPrefix)-2]

	disk := &NatsStore{
		Conn:          conn,
		StreamConfig:  &DiskBasedStreamConfig,
		SubjectPrefix: diskPrefix,
	}

	memory := &NatsStore{
		Conn:          conn,
		StreamConfig:  &MemoryBasedStreamConfig,
		SubjectPrefix: memoryPrefix,
	}

	return &Blobs{
		store: &CachingStore{
			Disk:   disk,
			Memory: memory,
		},
	}
}
