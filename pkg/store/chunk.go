package store

import (
	"context"
	"io"

	"github.com/nats-io/nats.go"
)

type Chunks struct {
	store Store
}

func (c *Chunks) Get(digest Digest, ctx context.Context) (io.ReadCloser, error) {
	return c.store.Get(digest.String(), ctx)
}

func (c *Chunks) Put(digest Digest, reader io.ReadCloser, ctx context.Context) error {
	return c.store.Put(digest.String(), reader, ctx)
}

func (c *Chunks) Delete(digest Digest, ctx context.Context) error {
	return c.store.Delete(digest.String(), ctx)
}

func NewChunkStore(conn *nats.Conn) *Chunks {
	diskPrefix := DiskBasedStreamConfig.Subjects[1]
	diskPrefix = diskPrefix[:len(diskPrefix)-2]

	memoryPrefix := MemoryBasedStreamConfig.Subjects[1]
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

	return &Chunks{
		store: &CachingStore{
			Disk:   disk,
			Memory: memory,
		},
	}
}
