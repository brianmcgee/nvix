package directory

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/brianmcgee/nvix/pkg/store"

	castorev1 "code.tvl.fyi/tvix/castore-go"

	"github.com/41north/async.go"
	"github.com/charmbracelet/log"
	"golang.org/x/sync/errgroup"

	"github.com/juju/errors"
	"golang.org/x/exp/slices"
)

func Import(
	ctx context.Context,
	path string,
	blobClient castorev1.BlobServiceClient,
	directoryClient castorev1.DirectoryServiceClient,
) ([]byte, error) {
	// todo handle single file and single symlink

	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return nil, err
	}

	if !info.IsDir() {
		return nil, errors.Errorf("path must be a directory: %v", absPath)
	} else if info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.Errorf("path cannot be a symlink: %v", absPath)
	}

	fileDigests, err := UploadFiles(ctx, absPath, blobClient)
	if err != nil {
		return nil, err
	}

	log.Info("file upload finished")

	put, err := directoryClient.Put(ctx)
	if err != nil {
		return nil, err
	}

	iter, err := NewDepthFirstIterator(absPath)
	if err != nil {
		return nil, err
	}

	dirCache := make(map[int]*castorev1.Directory)
	digestCache := make(map[string][]byte)

	getOrCreateDir := func(depth int) *castorev1.Directory {
		dir, ok := dirCache[depth]
		if !ok {
			dir = &castorev1.Directory{}
			dirCache[depth] = dir
		}
		return dir
	}

	var rootDir *castorev1.Directory

	for {
		info, err := iter.Next()
		if err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}

		if info.IsDir() {

			// fetch the directory object for just below our current depth
			dir := getOrCreateDir(iter.Depth() + 1)

			// we only receive a directory when we have finished iterating a directory's contents
			digest, err := dir.Digest()
			if err != nil {
				return nil, errors.Annotate(err, "failed to generate directory digest")
			}

			// cache the digest for the directory
			digestCache[iter.Dir()] = digest

			// put remotely
			if err = put.Send(dir); err != nil {
				log.Debug("directory put failed")
				for _, d := range dir.Directories {
					log.Debug("directory", "name", string(d.Name))
				}
				for _, f := range dir.Files {
					log.Debug("file", "name", string(f.Name))
				}
				for _, s := range dir.Symlinks {
					log.Debug("symlink", "name", string(s.Name))
				}
				return nil, err
			}

			// delete the dir entry so a new one is created next time we are at that depth
			delete(dirCache, iter.Depth()+1)

			if iter.Depth() > 0 {
				// append this directory to it's parent
				parentDir := getOrCreateDir(iter.Depth())
				parentDir.Directories = append(parentDir.Directories, &castorev1.DirectoryNode{
					Name:   []byte(info.Name()),
					Digest: digest,
				})

			} else {
				rootDir = dir
			}

			continue
		}

		if info.Mode()&os.ModeSymlink != 0 {
			// resolve the symlink into an absolute path
			target, err := filepath.EvalSymlinks(iter.Dir() + "/" + info.Name())
			if err != nil {
				return nil, err
			} else if target, err = filepath.Abs(target); err != nil {
				return nil, err
			}

			// look up the digest in the cache
			digest, ok := digestCache[target]
			if !ok {
				return nil, errors.Errorf("symlink refers to a target that has not already been processed: %v", target)
			}

			// append to the current directory being built
			dir := getOrCreateDir(iter.Depth())
			dir.Symlinks = append(dir.Symlinks, &castorev1.SymlinkNode{
				Name:   []byte(info.Name()),
				Target: digest,
			})

			continue
		}

		// regular file

		dir := getOrCreateDir(iter.Depth())

		absPath := iter.Dir() + "/" + info.Name()

		digestFuture, ok := fileDigests[absPath]
		if !ok {
			return nil, errors.Errorf("could not find a file digest future: %v", absPath)
		}

		select {
		case <-ctx.Done():
			return nil, err
		case result := <-digestFuture.Get():
			digest, err := result.Unwrap()
			if err != nil {
				return nil, err
			}

			dir.Files = append(dir.Files, &castorev1.FileNode{
				Name:   []byte(info.Name()),
				Digest: digest,
			})
		}

	}

	rootDigest, err := rootDir.Digest()
	if err != nil {
		return nil, err
	}

	resp, err := put.CloseAndRecv()
	if err != nil {
		return nil, err
	}

	if bytes.Compare(rootDigest, resp.RootDigest) != 0 {
		// todo add digests to error message
		return nil, errors.New("Root digest mismatch")
	}

	return rootDigest, nil
}

func UploadFiles(
	ctx context.Context,
	path string,
	client castorev1.BlobServiceClient,
) (map[string]async.Future[async.Result[[]byte]], error) {
	l := log.WithPrefix("upload_files").With("path", path)

	digests := make(map[string]async.Future[async.Result[[]byte]])

	eg, ctx := errgroup.WithContext(ctx)
	eg.SetLimit(runtime.NumCPU())

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

		future := async.NewFuture[async.Result[[]byte]]()
		digests[filePath] = future

		l.Debug("scheduled upload", "filePath", filePath)
		eg.Go(func() (err error) {
			l := log.WithPrefix("file_upload").With("path", filePath)
			start := time.Now()
			l.Debug("starting upload")

			defer func() {
				if err != nil {
					future.Set(async.NewResultErr[[]byte](err))
				}
			}()

			var file *os.File
			if file, err = os.Open(filePath); err != nil {
				return
			}

			// 1Mb chunks
			chunk := make([]byte, 1024*1024)

			var put castorev1.BlobService_PutClient
			if put, err = client.Put(ctx); err != nil {
				return
			}

			var n int

			for {
				n, err = file.Read(chunk)
				if err == io.EOF {
					break
				} else if err != nil {
					return
				}

				if err = put.Send(&castorev1.BlobChunk{
					Data: chunk[:n],
				}); err != nil {
					return
				}
			}

			var resp *castorev1.PutBlobResponse
			if resp, err = put.CloseAndRecv(); err != nil {
				return
			}

			elapsed := time.Now().Sub(start)
			l.Debug("upload complete", "elapsed", elapsed, "digest", store.Digest(resp.Digest))

			future.Set(async.NewResultValue(resp.Digest))
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
