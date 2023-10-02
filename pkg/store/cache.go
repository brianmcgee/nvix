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

func (c *CachingStore) Stat(key string, ctx context.Context) (ok bool, err error) {
	ok, err = c.Memory.Stat(key, ctx)
	if err == nil {
		return
	} else if !ok {
		// check disk
		ok, err = c.Disk.Stat(key, ctx)
	}
	return
}

func (c *CachingStore) Get(key string, ctx context.Context) (reader io.ReadCloser, err error) {
	// try in Memory store first
	reader, err = c.Memory.Get(key, ctx)
	if err == nil {
		reader = &cacheReader{
			key:    key,
			disk:   c.Disk,
			memory: c.Memory,
			ctx:    ctx,
			reader: reader,
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

type cacheReader struct {
	key    string
	disk   Store
	memory Store
	ctx    context.Context

	reader  io.ReadCloser
	faulted bool
}

func (c *cacheReader) Read(p []byte) (n int, err error) {
	n, err = c.reader.Read(p)
	if err == ErrKeyNotFound && !c.faulted {
		_ = c.reader.Close()

		log.Debug("cache miss", "key", c.key)
		// fallback and try the Disk based store last
		c.reader, err = c.disk.Get(c.key, c.ctx)
		if err == nil {
			c.reader = cacheWriter{
				store:  c.memory,
				key:    c.key,
				reader: c.reader,
				buf:    bytes.NewBuffer(nil),
			}
		}

		c.faulted = true
		n, err = c.reader.Read(p)
	}
	return
}

func (c *cacheReader) Close() error {
	return c.reader.Close()
}
