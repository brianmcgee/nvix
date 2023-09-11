package blob

import (
	"encoding/base64"
	"fmt"
)

var SubjectPrefix = "TVIX.STORE"

func ChunkSubject(chunkId string) string {
	return fmt.Sprintf("%s.CHUNK.%s", SubjectPrefix, chunkId)
}

func ChunkSubjectForDigest(digest []byte) string {
	return ChunkSubject(base64.StdEncoding.EncodeToString(digest))
}

func BlobSubject(chunkId string) string {
	return fmt.Sprintf("%s.BLOB.%s", SubjectPrefix, chunkId)
}

func BlobSubjectForDigest(digest []byte) string {
	return BlobSubject(base64.StdEncoding.EncodeToString(digest))
}
