package blob

import (
	"github.com/jotfs/fastcdc-go"
)

var ChunkOptions = fastcdc.Options{
	MinSize:     4 * 1024 * 1024,
	AverageSize: 6 * 1024 * 1024,
	MaxSize:     (8 * 1024 * 1024) - 1024, // we allow 1kb for headers to avoid max message size
}
