package blob

import (
	"github.com/brianmcgee/nvix/pkg/store"
	"github.com/brianmcgee/nvix/pkg/subject"
	"github.com/nats-io/nats.go"
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
		MaxBytes:          1024 * 1024 * 128, // todo make configurable from cli
		Storage:           nats.MemoryStorage,
		AllowRollup:       true,
		AllowDirect:       true,
	}
)

func NewChunkStore(conn *nats.Conn) store.Store {
	diskPrefix := DiskBasedStreamConfig.Subjects[1]
	diskPrefix = diskPrefix[:len(diskPrefix)-2]

	memoryPrefix := MemoryBasedStreamConfig.Subjects[1]
	memoryPrefix = memoryPrefix[:len(memoryPrefix)-2]

	disk := &store.NatsStore{
		Conn:          conn,
		StreamConfig:  &DiskBasedStreamConfig,
		SubjectPrefix: diskPrefix,
	}

	memory := &store.NatsStore{
		Conn:          conn,
		StreamConfig:  &MemoryBasedStreamConfig,
		SubjectPrefix: memoryPrefix,
	}

	return &store.CachingStore{
		Disk:   disk,
		Memory: memory,
	}
}

func NewMetaStore(conn *nats.Conn) store.Store {
	diskPrefix := DiskBasedStreamConfig.Subjects[0]
	diskPrefix = diskPrefix[:len(diskPrefix)-2]

	memoryPrefix := MemoryBasedStreamConfig.Subjects[0]
	memoryPrefix = memoryPrefix[:len(memoryPrefix)-2]

	disk := &store.NatsStore{
		Conn:          conn,
		StreamConfig:  &DiskBasedStreamConfig,
		SubjectPrefix: diskPrefix,
	}

	memory := &store.NatsStore{
		Conn:          conn,
		StreamConfig:  &MemoryBasedStreamConfig,
		SubjectPrefix: memoryPrefix,
	}

	return &store.CachingStore{
		Disk:   disk,
		Memory: memory,
	}
}
