package store

import (
	"context"
	"encoding/base64"
	"syscall"

	pb "code.tvl.fyi/tvix/castore-go"

	"github.com/brianmcgee/nvix/pkg/cli"
	"github.com/brianmcgee/nvix/pkg/directory"
	"github.com/juju/errors"
	"github.com/ztrue/shutdown"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Import struct {
	Log cli.LogOptions `embed:""`

	Path                string `arg:"" type:"existingdir"`
	BlobServiceUrl      string `env:"BLOB_SERVICE_URL" default:"localhost:5000"`
	DirectoryServiceUrl string `env:"DIRECTORY_SERVICE_URL" default:"localhost:5000"`
}

func (i *Import) Run() error {
	i.Log.ConfigureLogger()

	blobConn, err := grpc.Dial(i.BlobServiceUrl, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return errors.Annotate(err, "failed to connect to blob service")
	}

	directoryConn, err := grpc.Dial(i.DirectoryServiceUrl, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return errors.Annotate(err, "failed to connect to directory service")
	}

	blobClient := pb.NewBlobServiceClient(blobConn)
	directoryClient := pb.NewDirectoryServiceClient(directoryConn)

	ctx, cancel := context.WithCancel(context.Background())

	shutdown.Add(func() {
		cancel()
	})

	go shutdown.Listen(syscall.SIGINT, syscall.SIGTERM)

	digest, err := directory.Import(ctx, i.Path, blobClient, directoryClient)
	if err != nil {
		return err
	}

	println(base64.StdEncoding.EncodeToString(digest))
	return nil
}
