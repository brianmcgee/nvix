package store

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math/rand"
	"testing"

	"github.com/nats-io/nuid"

	"github.com/brianmcgee/nvix/pkg/test"
	"github.com/inhies/go-bytesize"
	"github.com/nats-io/nats.go"
)

var natsStoreSizes = []bytesize.ByteSize{
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
	(8 << 20) - 1024, // stay just under max msg size
}

func BenchmarkNatsStore_Put(b *testing.B) {
	s := test.RunBasicJetStreamServer(b)
	defer test.ShutdownJSServerAndRemoveStorage(b, s)

	conn, js := test.JsClient(b, s)

	js, err := conn.JetStream()
	if err != nil {
		b.Fatal(err)
	}

	storageTypes := []nats.StorageType{
		nats.FileStorage,
		nats.MemoryStorage,
	}

	streamConfig := nats.StreamConfig{
		Replicas:          1,
		Discard:           nats.DiscardOld,
		MaxMsgsPerSubject: 1,
		Storage:           nats.FileStorage,
		AllowRollup:       true,
		AllowDirect:       true,
	}

	for _, storage := range storageTypes {

		subjectPrefix := fmt.Sprintf("STORE.%v", storage)

		streamConfig.Name = storage.String()
		streamConfig.Subjects = []string{subjectPrefix + ".*"}
		streamConfig.Storage = storage

		if _, err := js.AddStream(&streamConfig); err != nil {
			b.Fatal(err)
		}

		store := NatsStore{
			Conn:          conn,
			StreamConfig:  &streamConfig,
			SubjectPrefix: subjectPrefix,
		}

		for _, size := range natsStoreSizes {
			size := size
			b.Run(fmt.Sprintf("%s-%v", streamConfig.Name, size), func(b *testing.B) {
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
						if err := store.Put(nuid.Next(), io.NopCloser(r), context.Background()); err != nil {
							b.Fatal(err)
						}
					}
				})
			})
		}
	}
}

func BenchmarkNatsStore_Get(b *testing.B) {
	s := test.RunBasicJetStreamServer(b)
	defer test.ShutdownJSServerAndRemoveStorage(b, s)

	conn, js := test.JsClient(b, s)

	js, err := conn.JetStream()
	if err != nil {
		b.Fatal(err)
	}

	storageTypes := []nats.StorageType{
		nats.FileStorage,
		nats.MemoryStorage,
	}

	streamConfig := nats.StreamConfig{
		Replicas:          1,
		Discard:           nats.DiscardOld,
		MaxMsgsPerSubject: 1,
		Storage:           nats.FileStorage,
		AllowRollup:       true,
		AllowDirect:       true,
	}

	for _, storage := range storageTypes {

		subjectPrefix := fmt.Sprintf("STORE.%v", storage)

		streamConfig.Name = storage.String()
		streamConfig.Subjects = []string{subjectPrefix + ".*"}
		streamConfig.Storage = storage

		if _, err := js.AddStream(&streamConfig); err != nil {
			b.Fatal(err)
		}

		store := NatsStore{
			Conn:          conn,
			StreamConfig:  &streamConfig,
			SubjectPrefix: subjectPrefix,
		}

		for _, size := range natsStoreSizes {
			size := size

			rng := rand.New(rand.NewSource(1))
			data := make([]byte, size)
			rng.Read(data)

			r := bytes.NewReader(data)

			key := fmt.Sprintf("key-%d", int(size))
			if err := store.Put(key, io.NopCloser(r), context.Background()); err != nil {
				b.Fatal(err)
			}

			b.Run(fmt.Sprintf("%s-%v", streamConfig.Name, size), func(b *testing.B) {
				b.SetBytes(int64(size))
				b.ReportAllocs()
				b.ResetTimer()

				b.RunParallel(func(pb *testing.PB) {
					for pb.Next() {
						reader, err := store.Get(key, context.Background())
						if err != nil {
							b.Fatal(err)
						}

						getData, err := io.ReadAll(reader)
						if err != nil {
							b.Fatal(err)
						}

						if len(getData) != len(data) {
							b.Fatalf("expected %d bytes, received %b", len(data), len(getData))
						}
					}
				})
			})
		}
	}
}
