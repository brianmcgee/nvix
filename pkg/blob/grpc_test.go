package blob

import (
	"bytes"
	"context"
	"io"
	"math/rand"
	"net"
	"testing"

	"github.com/inhies/go-bytesize"

	"github.com/brianmcgee/nvix/pkg/test"

	pb "code.tvl.fyi/tvix/castore/protos"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
)

var sizes = []bytesize.ByteSize{
	1 << 10,
	4 << 10,
	16 << 10,
	32 << 10,
	64 << 10,
	128 << 10,
	256 << 10,
	512 << 10,
	1 << 20,
	4 << 20,
	8 << 20,
	16 << 20,
	32 << 20,
	64 << 20,
	128 << 20,
	256 << 20,
	512 << 20,
}

func blobServer(s *server.Server, t test.TestingT) (*grpc.Server, net.Listener) {
	t.Helper()

	blobService, err := NewServer(test.NatsConn(t, s))
	if err != nil {
		t.Fatalf("failed to create blob service: %v", err)
	}

	srv := grpc.NewServer(grpc.MaxRecvMsgSize(16 * 1024 * 1024))
	pb.RegisterBlobServiceServer(srv, blobService)

	lis, err := net.Listen("tcp", ":0")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	go func() {
		if err := srv.Serve(lis); err != nil {
			t.Errorf("error serving server: %v", err)
		}
	}()

	return srv, lis
}

func BenchmarkBlobService_Put(b *testing.B) {
	s := test.RunBasicJetStreamServer(b)
	defer test.ShutdownJSServerAndRemoveStorage(b, s)

	srv, lis := blobServer(s, b)
	defer srv.Stop()

	conn := test.GrpcConn(lis, b)
	client := pb.NewBlobServiceClient(conn)

	for _, size := range sizes {
		size := size
		b.Run(size.String(), func(b *testing.B) {
			b.SetBytes(int64(size))
			b.ReportAllocs()
			b.ResetTimer()

			b.RunParallel(func(p *testing.PB) {
				rng := rand.New(rand.NewSource(1))
				data := make([]byte, size)
				rng.Read(data)

				r := bytes.NewReader(data)

				sendBuf := make([]byte, (16*1024*1024)-5)

				for p.Next() {
					r.Reset(data)

					put, err := client.Put(context.Background())
					if err != nil {
						b.Fatal(err)
					}

					for {
						if n, err := r.Read(sendBuf); err != nil {
							if err == io.EOF {
								break
							} else {
								b.Fatal(err)
							}
						} else if err = put.Send(&pb.BlobChunk{Data: sendBuf[:n]}); err != nil {
							b.Fatal(err)
						}
					}

					if _, err = put.CloseAndRecv(); err != nil {
						b.Fatal(err)
					}
				}
			})
		})
	}
}

func BenchmarkBlobService_Read(b *testing.B) {
	s := test.RunBasicJetStreamServer(b)
	defer test.ShutdownJSServerAndRemoveStorage(b, s)

	srv, lis := blobServer(s, b)
	defer srv.Stop()

	conn := test.GrpcConn(lis, b)
	client := pb.NewBlobServiceClient(conn)

	for _, size := range sizes {
		size := size

		rng := rand.New(rand.NewSource(1))
		data := make([]byte, size)
		rng.Read(data)

		r := bytes.NewReader(data)

		put, err := client.Put(context.Background())
		if err != nil {
			b.Fatal(err)
		}

		sendBuf := make([]byte, (16*1024*1024)-5)
		for {
			if n, err := r.Read(sendBuf); err != nil {
				if err == io.EOF {
					break
				} else {
					b.Fatal(err)
				}
			} else if err = put.Send(&pb.BlobChunk{Data: sendBuf[:n]}); err != nil {
				b.Fatal(err)
			}
		}

		resp, err := put.CloseAndRecv()
		if err != nil {
			b.Fatal(err)
		}

		b.Run(size.String(), func(b *testing.B) {
			b.SetBytes(int64(size))
			b.ReportAllocs()
			b.ResetTimer()

			b.RunParallel(func(p *testing.PB) {
				for p.Next() {
					buf := bytes.NewBuffer(nil)

					read, err := client.Read(context.Background(), &pb.ReadBlobRequest{Digest: resp.Digest})
					if err != nil {
						b.Fatal(err)
					}

					for {
						chunk, err := read.Recv()
						if err == io.EOF {
							break
						} else if err != nil {
							b.Fatal(err)
						}
						_, err = buf.Write(chunk.Data)
						if err != nil {
							b.Fatal(err)
						}
					}

					if buf.Len() != len(data) {
						b.Fatalf("Received %v bytes, expected %v", buf.Len(), len(data))
					}
				}
			})
		})
	}
}

func TestBlobService_Put(t *testing.T) {
	s := test.RunBasicJetStreamServer(t)
	defer test.ShutdownJSServerAndRemoveStorage(t, s)

	as := assert.New(t)

	srv, lis := blobServer(s, t)
	defer srv.Stop()

	conn := test.GrpcConn(lis, t)
	client := pb.NewBlobServiceClient(conn)

	payload := make([]byte, 100*1024*1024)
	_, err := rand.Read(payload)
	if err != nil {
		t.Fatalf("failed to generate random bytes: %v", err)
	}

	chunkSize := (4 * 1024 * 1024) - 1024 // stay just under 4MB grpc limit
	resp, err := putBlob(client, bytes.NewReader(payload), chunkSize, t)
	if err != nil {
		t.Fatalf("failed to received a response: %v", err)
	}

	meta, err := client.Stat(context.Background(), &pb.StatBlobRequest{Digest: resp.Digest})
	as.Nil(err)
	as.NotNil(meta)

	reader, writer := io.Pipe()
	go func() {
		getBlob(client, resp.Digest, writer, t)
	}()

	payload2, err := io.ReadAll(reader)
	if err != nil {
		log.Fatalf("failed to read blob: %v", err)
	}

	assert.Equal(t, payload, payload2)
}
