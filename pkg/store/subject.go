package store

import (
	"fmt"
)

var SubjectPrefix = "TVIX.STORE"

func ChunkSubject(chunkId string) string {
	return fmt.Sprintf("%s.CHUNK.%s", SubjectPrefix, chunkId)
}

func BlobSubject(chunkId string) string {
	return fmt.Sprintf("%s.BLOB.%s", SubjectPrefix, chunkId)
}
