package directory

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/41north/async.go"
	"github.com/charmbracelet/log"
	"golang.org/x/sync/errgroup"

	"github.com/juju/errors"
	"golang.org/x/exp/slices"
)

func UploadFiles(
	ctx context.Context,
	path string,
	uploadFn func(path string) ([]byte, error),
) (map[string]async.Future[[]byte], error) {
	l := log.WithPrefix("upload_files").With("path", path)

	digests := make(map[string]async.Future[[]byte])

	eg, ctx := errgroup.WithContext(ctx)
	eg.SetLimit(8) // todo make configurable

	iterator, err := NewDepthFirstIterator(path)
	if err != nil {
		return nil, err
	}

	for {
		info, err := iterator.Next()
		if err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}

		filePath, err := filepath.Abs(iterator.Dir() + "/" + info.Name())
		if err != nil {
			return nil, err
		}

		if info.IsDir() {
			l.Debug("skipping directory", "filePath", filePath)
			continue
		} else if (info.Mode() & os.ModeSymlink) != 0 {
			l.Debug("skipping symlink", "filePath", filePath)
			continue
		}

		future := async.NewFuture[[]byte]()
		digests[filePath] = future

		l.Debug("scheduled upload", "filePath", filePath)
		eg.Go(func() error {
			digest, err := uploadFn(filePath)
			if err != nil {
				return err
			}

			future.Set(digest)
			return nil
		})
	}

	return digests, nil
}

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

func NewDirIterator(path string, entries []os.DirEntry) (*DirIterator, error) {
	// lexicographically sort the entries first
	slices.SortFunc(entries, func(a, b os.DirEntry) int {
		return strings.Compare(a.Name(), b.Name())
	})
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}

	return &DirIterator{path: absPath, entries: entries}, nil
}

type DepthFirstIterator struct {
	root  string
	stack []*DirIterator
}

// Depth returns the number of subdirectories deep that we have traversed and in which we are currently iterating.
func (i *DepthFirstIterator) Depth() int {
	return len(i.stack)
}

// Dir returns the path of the current directory being iterated on.
// It should be combined with the os.FileInfo.Name() to get the path for the os.FileInfo returned by
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
			dirIterator, err := NewDirIterator(path, dirEntries)
			if err != nil {
				return nil, err
			}

			i.stack = append(i.stack, dirIterator)

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

	dirIterator, err := NewDirIterator(path, entries)
	if err != nil {
		return nil, err
	}

	return &DepthFirstIterator{
		root:  path,
		stack: []*DirIterator{dirIterator},
	}, nil
}
