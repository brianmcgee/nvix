package store

import (
	"context"
	"encoding/base64"
	"io"

	"github.com/brianmcgee/nvix/pkg/util"

	"github.com/nats-io/nats.go"

	"github.com/juju/errors"
)

const (
	ErrKeyNotFound = errors.ConstError("key not found")
)

type Digest [32]byte

func (d Digest) String() string {
	return base64.StdEncoding.EncodeToString(d[:])
}

type Store interface {
	Init(ctx context.Context) error
	Get(key string, ctx context.Context) (io.ReadCloser, error)
	Put(key string, reader io.ReadCloser, ctx context.Context) error
	PutAsync(key string, reader io.ReadCloser, ctx context.Context) (nats.PubAckFuture, error)
	List(ctx context.Context) (util.Iterator[io.ReadCloser], error)
	Stat(key string, ctx context.Context) (bool, error)
	Delete(key string, ctx context.Context) error
}
