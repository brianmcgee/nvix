package store

import (
	"bytes"
	"context"
	"io"
	"math/rand"
	"testing"

	"github.com/brianmcgee/nvix/pkg/test"
	"github.com/inhies/go-bytesize"
)

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

	if _, err := js.AddStream(&DiskBasedStreamConfig); err != nil {
		b.Fatal(err)
	}

	if _, err := js.AddStream(&MemoryBasedStreamConfig); err != nil {
		b.Fatal(err)
	}

	store := CdcStore{
		Meta:   NewMetaStore(conn),
		Chunks: NewChunkStore(conn),
	}

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
