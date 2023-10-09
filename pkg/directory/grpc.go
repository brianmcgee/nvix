package directory

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"

	capb "code.tvl.fyi/tvix/castore/protos"

	"github.com/brianmcgee/nvix/pkg/store"
	"github.com/charmbracelet/log"
	"github.com/golang/protobuf/proto"
	"github.com/juju/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/nats-io/nats.go"
)

func NewService(conn *nats.Conn) (capb.DirectoryServiceServer, error) {
	js, err := conn.JetStream()
	if err != nil {
		return nil, errors.Annotate(err, "failed to create a JetStream context")
	}

	if _, err := js.AddStream(&DiskBasedStreamConfig); err != nil {
		return nil, errors.Annotate(err, "failed to create disk based stream")
	}

	if _, err := js.AddStream(&MemoryBasedStreamConfig); err != nil {
		return nil, errors.Annotate(err, "failed to create memory based stream")
	}

	return &service{
		conn:  conn,
		store: NewDirectoryStore(conn),
	}, nil
}

type service struct {
	capb.UnimplementedDirectoryServiceServer
	conn  *nats.Conn
	store store.Store
}

// Get retrieves a stream of Directory messages, by using the lookup
// parameters in GetDirectoryRequest.
// Keep in mind multiple DirectoryNodes in different parts of the graph might
// have the same digest if they have the same underlying contents,
// so sending subsequent ones can be omitted.
func (s *service) Get(req *capb.GetDirectoryRequest, server capb.DirectoryService_GetServer) error {
	l := log.WithPrefix("directory.get")

	rootDigest := store.Digest(req.GetDigest())
	l.Debug("request", "digest", rootDigest)

	ctx, cancel := context.WithCancel(server.Context())
	defer cancel()

	fetch := func(digest store.Digest) (*capb.Directory, error) {
		reader, err := s.store.Get(digest.String(), ctx)
		if err != nil {
			l.Errorf("failure: %v", err)
			return nil, status.Errorf(codes.NotFound, "digest not found: %v", digest)
		}
		defer func() {
			_ = reader.Close()
		}()

		b, err := io.ReadAll(reader)
		if err != nil {
			l.Errorf("failure: %v", err)
			return nil, status.Error(codes.Internal, "failed to read directory entry from store")
		}
		var dir capb.Directory
		if err = proto.Unmarshal(b, &dir); err != nil {
			l.Errorf("failure: %v", err)
			return nil, status.Error(codes.Internal, "failed to unmarshal directory entry from store")
		}
		return &dir, nil
	}

	// todo handle get by what

	rootDirectory, err := fetch(rootDigest)
	if err != nil {
		l.Errorf("failure: %v", err)
		return status.Errorf(codes.NotFound, "directory not found: %v", rootDigest)
	}

	dirs := []*capb.Directory{rootDirectory}

	iterateDirs := func(directory *capb.Directory) error {
		for _, dir := range directory.Directories {
			digest := store.Digest(dir.Digest)
			if d, err := fetch(digest); err != nil {
				return err
			} else {
				dirs = append(dirs, d)
			}
		}
		return nil
	}

	for _, dir := range dirs {
		if err = iterateDirs(dir); err != nil {
			l.Errorf("failure: %v", err)
			return status.Error(codes.Internal, "failed to iterate directories")
		} else if err = server.Send(dir); err != nil {
			l.Errorf("failure: %v", err)
			return status.Error(codes.Internal, "failed to send directory")
		}
		// remove from head
		dirs = dirs[1:]
	}

	return nil
}

// Put uploads a graph of Directory messages.
// Individual Directory messages need to be send in an order walking up
// from the leaves to the root - a Directory message can only refer to
// Directory messages previously sent in the same stream.
// Keep in mind multiple DirectoryNodes in different parts of the graph might
// have the same digest if they have the same underlying contents,
// so sending subsequent ones can be omitted.
// We might add a separate method, allowing to send partial graphs at a later
// time, if requiring to send the full graph turns out to be a problem.
func (s *service) Put(server capb.DirectoryService_PutServer) error {
	l := log.WithPrefix("directory.put")

	ctx, cancel := context.WithCancel(server.Context())
	defer cancel()

	var rootDigest []byte
	var futures []nats.PubAckFuture

	cache := make(map[string]*capb.Directory)

	for {
		directory, err := server.Recv()
		if err == io.EOF {
			break
		} else if err != nil {
			l.Error("failed to receive directory", "err", err)
			return status.Errorf(codes.Unknown, "failed to receive directory")
		}

		if err := validateDirectory(directory); err != nil {
			return status.Errorf(codes.InvalidArgument, "bad request: %v", err)
		}

		digest, err := directory.Digest()
		if err != nil {
			return status.Error(codes.Unknown, "failed to generate directory digest")
		}

		digestStr := base64.StdEncoding.EncodeToString(digest)
		cache[digestStr] = directory

		for _, node := range directory.Directories {
			target := base64.StdEncoding.EncodeToString(node.Digest)
			if _, ok := cache[target]; !ok {
				return status.Errorf(codes.InvalidArgument, "directory node refers to unknown directory digest: %v", target)
			}
		}

		b, err := proto.Marshal(directory)
		if err != nil {
			l.Error("failed to marshal directory", "err", err)
			return status.Error(codes.Internal, "failed to marshal directory")
		}

		future, err := s.store.PutAsync(digestStr, io.NopCloser(bytes.NewReader(b)), ctx)
		if err != nil {
			l.Error("failed to put directory in store", "err", err)
			return status.Errorf(codes.Internal, "failed to put directory in store")
		}

		rootDigest = digest
		futures = append(futures, future)
	}

	for _, f := range futures {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-f.Err():
			// TODO how to handle partial writes due to failure?
			l.Error(codes.Internal, "put future has returned an error", "err", err)
			return status.Errorf(codes.Internal, "failed to put directory in store")
		case ack := <-f.Ok():
			l.Debug("put acknowledged", "ack", ack)
		}
	}

	l.Debug("all puts complete")

	resp := &capb.PutDirectoryResponse{
		RootDigest: rootDigest,
	}
	if err := server.SendAndClose(resp); err != nil {
		l.Error("failed to send put response", "err", err)
	}

	l.Debug("finished")
	return nil
}
