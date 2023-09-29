package store

import (
	"github.com/brianmcgee/nvix/pkg/subject"
	"github.com/nats-io/nats.go"
)

var (
	DiskBasedStreamConfig = nats.StreamConfig{
		Name: "store",
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
		Name: "cache",
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
