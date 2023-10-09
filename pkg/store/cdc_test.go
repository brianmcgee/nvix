package store

import (
	"bytes"
	"context"
	"io"
	"math/rand"
	"testing"

	"github.com/brianmcgee/nvix/pkg/subject"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"

	"lukechampine.com/blake3"

	"github.com/brianmcgee/nvix/pkg/test"
	"github.com/inhies/go-bytesize"
)

var (
	DiskBasedStreamConfig = nats.StreamConfig{
		Name: "blob_store",
		Subjects: []string{
			subject.WithPrefix("STORE.BLOB.*"),
			subject.WithPrefix("STORE.CHUNK.*"),
		},
		Replicas:          1,
		Discard:           nats.DiscardOld,
		MaxMsgsPerSubject: 1,
		Storage:           nats.FileStorage,
		AllowRollup:       true,
		AllowDirect:       true,
		Compression:       nats.S2Compression,
		// automatically publish into the cache topic
		RePublish: &nats.RePublish{
			Source:      subject.WithPrefix("STORE.*.*"),
			Destination: subject.WithPrefix("CACHE.{{wildcard(1)}}.{{wildcard(2)}}"),
		},
	}

	MemoryBasedStreamConfig = nats.StreamConfig{
		Name: "blob_cache",
		Subjects: []string{
			subject.WithPrefix("CACHE.BLOB.*"),
			subject.WithPrefix("CACHE.CHUNK.*"),
		},
		Replicas:          1,
		Discard:           nats.DiscardOld,
		MaxMsgsPerSubject: 1,
		Storage:           nats.MemoryStorage,
		AllowRollup:       true,
		AllowDirect:       true,
	}
)

func newChunkStore(conn *nats.Conn) Store {
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

	return &CachingStore{
		Disk:   disk,
		Memory: memory,
	}
}

func newMetaStore(conn *nats.Conn) Store {
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

	return &CachingStore{
		Disk:   disk,
		Memory: memory,
	}
}

func newCdcStore(t test.TestingT, conn *nats.Conn, js nats.JetStreamContext) *CdcStore {
	if _, err := js.AddStream(&DiskBasedStreamConfig); err != nil {
		t.Fatal(err)
	}

	if _, err := js.AddStream(&MemoryBasedStreamConfig); err != nil {
		t.Fatal(err)
	}

	return &CdcStore{
		Meta:   newMetaStore(conn),
		Chunks: newChunkStore(conn),
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
	256 << 20,
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
			b.SetBytes(int64(size))
			b.ReportAllocs()
			b.ResetTimer()

			b.RunParallel(func(pb *testing.PB) {
				rng := rand.New(rand.NewSource(1))
				data := make([]byte, size)
				rng.Read(data)

				r := bytes.NewReader(data)

				for pb.Next() {
					r.Reset(data)
					if _, err := store.Put(io.NopCloser(r), context.Background()); err != nil {
						b.Fatal(err)
					}
				}
			})
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

			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
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
		})
	}
}
