package store

import (
	"bytes"
	"context"
	"io"
	"math/rand"
	"testing"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"

	"lukechampine.com/blake3"

	"github.com/brianmcgee/nvix/pkg/test"
	"github.com/inhies/go-bytesize"
)

func newCdcStore(t test.TestingT, conn *nats.Conn, js nats.JetStreamContext) *CdcStore {
	if _, err := js.AddStream(&DiskBasedStreamConfig); err != nil {
		t.Fatal(err)
	}

	if _, err := js.AddStream(&MemoryBasedStreamConfig); err != nil {
		t.Fatal(err)
	}

	return &CdcStore{
		Meta:   NewMetaStore(conn),
		Chunks: NewChunkStore(conn),
	}
}

func TestCdcStore_PutAndGet(t *testing.T) {
	as := assert.New(t)

	s := test.RunBasicJetStreamServer(t)
	defer test.ShutdownJSServerAndRemoveStorage(t, s)

	conn, js := test.JsClient(t, s)

	js, err := conn.JetStream()
	if err != nil {
		t.Fatal(err)
	}

	store := newCdcStore(t, conn, js)

	rng := rand.New(rand.NewSource(1))
	data := make([]byte, 20*1024*1024)
	rng.Read(data)

	r := bytes.NewReader(data)

	digest, err := store.Put(io.NopCloser(r), context.Background())
	as.Nil(err)

	hasher := blake3.New(32, nil)
	_, err = hasher.Write(data)
	as.Nil(err)
	as.Equal(Digest(hasher.Sum(nil)), *digest)

	ok, err := store.Stat(*digest, context.Background())
	as.Nil(err)
	as.True(ok)

	reader, err := store.Get(*digest, context.Background())
	as.Nil(err)
	getData, err := io.ReadAll(reader)
	as.Nil(err)
	as.Equal(data, getData)
	as.Nil(reader.Close())

	as.Nil(store.Delete(*digest, context.Background()))

	_, err = store.Get(*digest, context.Background())
	as.ErrorIs(err, ErrKeyNotFound)

	ok, err = store.Stat(*digest, context.Background())
	as.ErrorIs(err, ErrKeyNotFound)
}

var sizes = []bytesize.ByteSize{
	1 << 10,
	4 << 10,
	16 << 10,
	32 << 10,
	64 << 10,
	128 << 10,
	256 << 10,
	512 << 10,
	1 << 20,
	4 << 20,
	8 << 20,
	16 << 20,
	32 << 20,
	64 << 20,
	128 << 20,
	512 << 20,
	1 << 30,
}

func BenchmarkCdcStore_Put(b *testing.B) {
	s := test.RunBasicJetStreamServer(b)
	defer test.ShutdownJSServerAndRemoveStorage(b, s)

	conn, js := test.JsClient(b, s)

	js, err := conn.JetStream()
	if err != nil {
		b.Fatal(err)
	}

	store := newCdcStore(b, conn, js)

	for _, size := range sizes {
		size := size
		b.Run(size.String(), func(b *testing.B) {
			rng := rand.New(rand.NewSource(1))
			data := make([]byte, size)
			rng.Read(data)

			r := bytes.NewReader(data)
			b.SetBytes(int64(size))
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				r.Reset(data)
				if _, err := store.Put(io.NopCloser(r), context.Background()); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkCdcStore_Get(b *testing.B) {
	s := test.RunBasicJetStreamServer(b)
	defer test.ShutdownJSServerAndRemoveStorage(b, s)

	conn, js := test.JsClient(b, s)

	js, err := conn.JetStream()
	if err != nil {
		b.Fatal(err)
	}

	store := newCdcStore(b, conn, js)

	for _, size := range sizes {
		size := size

		rng := rand.New(rand.NewSource(1))
		data := make([]byte, size)
		rng.Read(data)

		r := bytes.NewReader(data)

		digest, err := store.Put(io.NopCloser(r), context.Background())
		if err != nil {
			b.Fatal(err)
		}

		b.Run(size.String(), func(b *testing.B) {
			b.SetBytes(int64(size))
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				reader, err := store.Get(*digest, context.Background())
				if err != nil {
					b.Fatal(err)
				}

				getData, err := io.ReadAll(reader)
				if err != nil {
					b.Fatal(err)
				}

				err = reader.Close()
				if err != nil {
					b.Fatal(err)
				}

				if len(getData) != len(data) {
					b.Fatalf("expected %v bytes, received %v", len(data), len(getData))
				}
			}
		})
	}
}
