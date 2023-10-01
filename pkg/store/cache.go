package store

import (
	"bytes"
	"context"
	"io"

	"github.com/nats-io/nats.go"

	"github.com/charmbracelet/log"
)

type CachingStore struct {
	Disk   Store
	Memory Store
}

func (c *CachingStore) Get(key string, ctx context.Context) (reader io.ReadCloser, err error) {
	// try in Memory store first
	reader, err = c.Memory.Get(key, ctx)
	if err == ErrKeyNotFound {
		log.Debug("cache miss", "key", key)
		// fallback and try the Disk based store last
		reader, err = c.Disk.Get(key, ctx)
		if err == nil {
			reader = cacheWriter{
				store:  c.Memory,
				key:    key,
				reader: reader,
				buf:    bytes.NewBuffer(nil),
			}
		}
	}
	return
}

func (c *CachingStore) Put(key string, reader io.ReadCloser, ctx context.Context) error {
	return c.Disk.Put(key, reader, ctx)
}

func (c *CachingStore) PutAsync(key string, reader io.ReadCloser, ctx context.Context) (nats.PubAckFuture, error) {
	return c.Disk.PutAsync(key, reader, ctx)
}

func (c *CachingStore) Delete(key string, ctx context.Context) error {
	if err := c.Memory.Delete(key, ctx); err != nil {
		return err
	}
	return c.Disk.Delete(key, ctx)
}

type cacheWriter struct {
	store  Store
	key    string
	reader io.Reader
	buf    *bytes.Buffer
}

func (c cacheWriter) Read(p []byte) (n int, err error) {
	if n, err = c.reader.Read(p); err != nil {
		return
	}
	_, err = c.buf.Write(p[:n])
	return
}

func (c cacheWriter) Close() (err error) {
	log.Debug("populating cache", "key", c.key)
	if err = c.store.Put(c.key, io.NopCloser(bytes.NewReader(c.buf.Bytes())), context.Background()); err != nil {
		log.Error("failed to populate cache", "key", c.key, "error", err)
	}
	return
}
