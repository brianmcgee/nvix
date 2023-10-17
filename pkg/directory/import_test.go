package directory

import (
	"io"
	"os"
	"testing"

	"github.com/charmbracelet/log"
	"github.com/stretchr/testify/require"
)

var (
	canonicalEntries = []struct {
		dir   string
		name  string
		depth int
		isDir bool
	}{
		{dir: "./a", name: "c.txt", depth: 2, isDir: false},
		{dir: "./a", name: "d.txt", depth: 2, isDir: false},
		{dir: "./a/e", name: "f.txt", depth: 3, isDir: false},
		{dir: "./a", name: "e", depth: 2, isDir: true},
		{dir: ".", name: "a", depth: 1, isDir: true},
		{dir: "./b", name: "g.txt", depth: 2, isDir: false},
		{dir: "./b", name: "h.txt", depth: 2, isDir: false},
		{dir: ".", name: "b", depth: 1, isDir: true},
		{dir: "", name: ".", depth: 0, isDir: true},
	}
)

func TestDepthFirstIterator_Canonical(t *testing.T) {
	log.SetLevel(log.InfoLevel)

	r := require.New(t)

	err := os.Chdir("../../test/testdata/dfi/canonical")
	r.Nil(err)

	iterator, err := NewDepthFirstIterator(".")
	r.Nil(err)

	idx := 0

	for {
		info, err := iterator.Next()
		if err == io.EOF {
			return
		}

		r.Nil(err)

		entry := canonicalEntries[idx]

		r.Equal(entry.dir, iterator.Dir())
		r.Equal(entry.name, info.Name())
		r.Equal(entry.isDir, info.IsDir())

		idx += 1
	}
}
