package store

import (
	"context"
	"io"

	"github.com/nats-io/nats.go"

	"github.com/juju/errors"
	"github.com/nix-community/go-nix/pkg/nixbase32"
)

const (
	ErrKeyNotFound = errors.ConstError("key not found")
)

type Digest [32]byte

func (d Digest) String() string {
	return nixbase32.EncodeToString(d[:])
}

type Store interface {
	Get(key string, ctx context.Context) (io.ReadCloser, error)
	Put(key string, reader io.ReadCloser, ctx context.Context) error
	PutAsync(key string, reader io.ReadCloser, ctx context.Context) (nats.PubAckFuture, error)
	Stat(key string, ctx context.Context) (bool, error)
	Delete(key string, ctx context.Context) error
}
