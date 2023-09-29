package blob

import (
	"bytes"
	"context"
	"io"
	"testing"

	pb "code.tvl.fyi/tvix/castore/protos"

	"github.com/charmbracelet/log"
)

func putBlob(c pb.BlobServiceClient, r io.Reader, chunkSize int, t *testing.T) (*pb.PutBlobResponse, error) {
	t.Helper()

	put, err := c.Put(context.Background())
	if err != nil {
		t.Fatalf("failed to create put: %v", err)
	}

	chunk := make([]byte, chunkSize)

	for {
		n, err := r.Read(chunk)
		if err == io.EOF {
			break
		} else if err != nil {
			t.Fatalf("failed to read chunk: %v", err)
		}

		if err = put.Send(&pb.BlobChunk{
			Data: chunk[:n],
		}); err != nil {
			t.Fatalf("failed to send blob chunk: %v", err)
		}
	}

	return put.CloseAndRecv()
}

func getBlob(c pb.BlobServiceClient, digest []byte, writer io.WriteCloser, t *testing.T) {
	get, err := c.Read(context.Background(), &pb.ReadBlobRequest{Digest: digest})
	if err != nil {
		t.Fatalf("failed to open read request: %v", err)
	}

	for {
		chunk, err := get.Recv()
		if err == io.EOF {
			if err = writer.Close(); err != nil {
				log.Fatalf("failed to close writer: %v", err)
			}
			return
		} else if err != nil {
			t.Fatalf("failed to get next chunk: %v", err)
		}
		if _, err = io.Copy(writer, bytes.NewReader(chunk.Data)); err != nil {
			t.Fatalf("failed to write chunk into read buffer: %v", err)
		}
	}
}
