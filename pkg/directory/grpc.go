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
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/nats-io/nats.go"
)

func NewServer(conn *nats.Conn) (*Server, error) {
	return &Server{
		conn:  conn,
		store: NewDirectoryStore(conn),
	}, nil
}

type Server struct {
	capb.UnimplementedDirectoryServiceServer
	conn  *nats.Conn
	store store.Store
}

func (s *Server) Get(req *capb.GetDirectoryRequest, server capb.DirectoryService_GetServer) error {
	l := log.WithPrefix("directory.get")

	rootDigest := store.Digest(req.GetDigest())
	l.Debug("request", "digest", rootDigest)

	ctx, cancel := context.WithCancel(server.Context())
	defer cancel()

	// todo handle get by what

	rootDirectory, err := s.GetByDigest(rootDigest, ctx)
	if err != nil {
		l.Errorf("failure: %v", err)
		return status.Errorf(codes.NotFound, "directory not found: %v", rootDigest)
	}

	dirs := []*capb.Directory{rootDirectory}

	iterateDirs := func(directory *capb.Directory) error {
		for _, dir := range directory.Directories {
			digest := store.Digest(dir.Digest)
			if d, err := s.GetByDigest(digest, ctx); err != nil {
				return err
			} else if req.Recursive {
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

func (s *Server) GetByDigest(digest store.Digest, ctx context.Context) (*capb.Directory, error) {
	l := log.WithPrefix("directory.getByDigest")

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

func (s *Server) Put(server capb.DirectoryService_PutServer) error {
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

	l.Debug("finished", "digest", store.Digest(rootDigest).String())
	return nil
}
