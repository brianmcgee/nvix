package directory

import (
	"github.com/brianmcgee/nvix/pkg/store"
	"github.com/brianmcgee/nvix/pkg/subject"
	"github.com/nats-io/nats.go"
)

var (
	DiskBasedStreamConfig = nats.StreamConfig{
		Name: "directory_store",
		Subjects: []string{
			subject.WithPrefix("STORE.DIRECTORY.*"),
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
			Source:      subject.WithPrefix("STORE.DIRECTORY.*"),
			Destination: subject.WithPrefix("CACHE.DIRECTORY.{{wildcard(1)}}"),
		},
	}

	MemoryBasedStreamConfig = nats.StreamConfig{
		Name: "directory_cache",
		Subjects: []string{
			subject.WithPrefix("CACHE.DIRECTORY.*"),
		},
		Replicas:          1,
		Discard:           nats.DiscardOld,
		MaxBytes:          1024 * 1024 * 128, // todo make configurable from cli
		MaxMsgsPerSubject: 1,
		Storage:           nats.MemoryStorage,
		AllowRollup:       true,
		AllowDirect:       true,
	}
)

func NewDirectoryStore(conn *nats.Conn) store.Store {
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
