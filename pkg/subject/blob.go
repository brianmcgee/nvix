package subject

import (
	"encoding/base64"
	"fmt"
)

var subjectPrefix = "TVIX.STORE"

func SetPrefix(prefix string) {
	subjectPrefix = prefix
}

func GetPrefix() string {
	return subjectPrefix
}

func ChunkPrefix() string {
	return fmt.Sprintf("%s.CHUNK", subjectPrefix)
}

func ChunkById(chunkId string) string {
	return fmt.Sprintf("%s.%s", ChunkPrefix(), chunkId)
}

func ChunkByDigest(digest []byte) string {
	return ChunkById(base64.StdEncoding.EncodeToString(digest))
}

func BlobPrefix() string {
	return fmt.Sprintf("%s.BLOB", subjectPrefix)
}

func BlobById(chunkId string) string {
	return fmt.Sprintf("%s.%s", BlobPrefix(), chunkId)
}

func BlobByDigest(digest []byte) string {
	return BlobById(base64.StdEncoding.EncodeToString(digest))
}
