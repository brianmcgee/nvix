package pathinfo

import (
	"bytes"
	capb "code.tvl.fyi/tvix/castore/protos"
	tvpb "code.tvl.fyi/tvix/store/protos"
	"context"
	"github.com/brianmcgee/nvix/pkg/blob"
	"github.com/brianmcgee/nvix/pkg/directory"
	"github.com/brianmcgee/nvix/pkg/store"
	"github.com/charmbracelet/log"
	"github.com/golang/protobuf/proto"
	"github.com/juju/errors"
	multihash "github.com/multiformats/go-multihash/core"
	"github.com/nats-io/nats.go"
	"github.com/nix-community/go-nix/pkg/hash"
	"github.com/nix-community/go-nix/pkg/nixbase32"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"io"
)

func NewServer(conn *nats.Conn, blob *blob.Server, directory *directory.Server) (*Service, error) {
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

	return &Service{
		conn:      conn,
		store:     NewPathInfoStore(conn),
		outIdx:    NewPathInfoOutIdxStore(conn),
		blob:      blob,
		directory: directory,
	}, nil
}

type Service struct {
	tvpb.UnimplementedPathInfoServiceServer
	conn      *nats.Conn
	store     store.Store
	outIdx    store.Store
	blob      *blob.Server
	directory *directory.Server
}

func pathInfoDigest(node *capb.Node) (*store.Digest, error) {
	var digest store.Digest
	if node.GetDirectory() != nil {
		digest = store.Digest(node.GetDirectory().Digest)
		// todo check directory exists
	} else if node.GetFile() != nil {
		digest = store.Digest(node.GetFile().Digest)
	} else if node.GetSymlink() != nil {
		digest = store.Digest(node.GetSymlink().Target)
	} else {
		return nil, status.Error(codes.Internal, "unexpected node type")
	}

	return &digest, nil
}

func (s *Service) Get(ctx context.Context, req *tvpb.GetPathInfoRequest) (*tvpb.PathInfo, error) {
	// only supporting get by output hash
	l := log.WithPrefix("path_info.get")

	outHash := nixbase32.EncodeToString(req.GetByOutputHash())
	l.Debug("executing", "outHash", outHash)

	reader, err := s.outIdx.Get(outHash, ctx)

	if err != nil {
		return nil, err
	}
	defer func() {
		_ = reader.Close()
	}()

	digest, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}

	reader, err = s.store.Get(store.Digest(digest).String(), ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to get path info")
	}

	b, err := io.ReadAll(reader)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to read path info")
	}

	var pathInfo tvpb.PathInfo
	if err = proto.Unmarshal(b, &pathInfo); err != nil {
		return nil, status.Error(codes.Internal, "failed to unmarshal path info")
	}

	return &pathInfo, nil
}

func (s *Service) Put(ctx context.Context, pathInfo *tvpb.PathInfo) (*tvpb.PathInfo, error) {
	digest, err := pathInfoDigest(pathInfo.Node)
	if err != nil {
		return nil, err
	}

	l := log.WithPrefix("path_info.put")
	l.Debug("executing", "digest", digest.String())

	b, err := proto.Marshal(pathInfo)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to marshal path info")
	}

	err = s.store.Put(digest.String(), io.NopCloser(bytes.NewReader(b)), ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to put path info")
	}

	err = s.outIdx.Put(
		nixbase32.EncodeToString(pathInfo.Narinfo.NarSha256),
		io.NopCloser(bytes.NewReader(digest[:])),
		ctx,
	)

	// no signatures to add so just return the same path info
	return pathInfo, err
}

func (s *Service) CalculateNAR(ctx context.Context, node *capb.Node) (*tvpb.CalculateNARResponse, error) {
	r, w := io.Pipe()

	go func() {
		err := tvpb.Export(w, node,
			func(digest []byte) (*capb.Directory, error) {
				return s.directory.GetByDigest(store.Digest(digest), ctx)
			}, func(digest []byte) (io.ReadCloser, error) {
				return s.blob.GetByDigest(store.Digest(digest), ctx)
			})
		_ = w.CloseWithError(err)
	}()

	hasher, err := hash.New(multihash.SHA2_256)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to create a SHA256 hasher")
	}

	if _, err = io.Copy(hasher, r); err != nil {
		return nil, status.Error(codes.Internal, "failed to write NAR to SHA256 hasher")
	}

	return &tvpb.CalculateNARResponse{
		NarSize:   hasher.BytesWritten(),
		NarSha256: hasher.Digest(),
	}, nil
}

func (s *Service) List(_ *tvpb.ListPathInfoRequest, server tvpb.PathInfoService_ListServer) error {
	// only list all currently to implement
	ctx, cancel := context.WithCancel(server.Context())
	defer cancel()

	l := log.WithPrefix("path_info.list")
	l.Debug("executing")

	iter, err := s.store.List(ctx)
	if err != nil {
		return status.Error(codes.Internal, "failed to create store iterator")
	}

	defer func() {
		_ = iter.Close()
	}()

	for {
		reader, err := iter.Next()
		if err == io.EOF {
			break
		} else if err != nil {
			return status.Error(codes.Internal, "failed to read next path info")
		}

		b, err := io.ReadAll(reader)
		_ = reader.Close()
		if err != nil {
			return status.Error(codes.Internal, "failed to read path info")
		}

		var pathInfo tvpb.PathInfo
		if err = proto.Unmarshal(b, &pathInfo); err != nil {
			return status.Error(codes.Internal, "failed to unmarshal path info")
		}

		if err = server.Send(&pathInfo); err != nil {
			return err
		}
	}

	return nil
}
