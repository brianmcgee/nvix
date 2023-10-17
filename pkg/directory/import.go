package directory

import (
	"io"
	"os"
	"strings"

	"github.com/juju/errors"
	"golang.org/x/exp/slices"
)

type DirIterator struct {
	path    string
	entries []os.DirEntry
	idx     int
}

func (i *DirIterator) Next() (os.DirEntry, error) {
	if i.idx == len(i.entries) {
		return nil, io.EOF
	} else {
		idx := i.idx
		i.idx += 1
		return i.entries[idx], nil
	}
}

func NewDirIterator(path string, entries []os.DirEntry) DirIterator {
	// lexicographically sort the entries first
	slices.SortFunc(entries, func(a, b os.DirEntry) int {
		return strings.Compare(a.Name(), b.Name())
	})
	return DirIterator{path: path, entries: entries}
}

type DepthFirstIterator struct {
	root  string
	stack []*DirIterator
}

// Dir returns the fully qualified path for the current directory being iterated on.
// It should be combined with the os.FileInfo.Name() to get the fully qualified path for the os.FileInfo returned by
// DepthFirstIterator.Next().
func (i *DepthFirstIterator) Dir() string {
	size := len(i.stack)
	if size == 0 {
		return ""
	}
	return i.stack[size-1].path
}

func (i *DepthFirstIterator) Next() (info os.FileInfo, err error) {
	var next os.DirEntry
	for {
		// we have exhausted all the iterators
		if len(i.stack) == 0 {
			return nil, io.EOF
		}

		// fetch the latest iterator and attempt to read the next item
		iterator := i.stack[len(i.stack)-1]

		next, err = iterator.Next()
		if err == io.EOF {
			// we have exhausted the current directory
			// remove the current iterator from the stack
			i.stack = i.stack[:len(i.stack)-1]

			// return the current iterator's file info
			info, err = os.Stat(iterator.path)
			return
		}

		path := iterator.path + "/" + next.Name()

		if next.IsDir() {
			// list the directory contents
			dirEntries, err := os.ReadDir(path)
			if err != nil {
				return nil, err
			}

			// create a new dir iterator and append to the stack
			dirIterator := NewDirIterator(path, dirEntries)
			i.stack = append(i.stack, &dirIterator)

			// try to read in the next loop
			continue
		}

		return next.Info()
	}
}

func NewDepthFirstIterator(path string) (*DepthFirstIterator, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	} else if !info.IsDir() {
		return nil, errors.Errorf("path is not a directory: %v", path)
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}

	dirIterator := NewDirIterator(path, entries)

	return &DepthFirstIterator{
		root:  path,
		stack: []*DirIterator{&dirIterator},
	}, nil
}