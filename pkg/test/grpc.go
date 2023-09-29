package test

import (
	"context"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

func GrpcConn(lis *bufconn.Listener, t TestingT) *grpc.ClientConn {
	t.Helper()
	conn, err := grpc.Dial("",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(ctx context.Context, s string) (net.Conn, error) {
			return lis.Dial()
		}))
	if err != nil {
		t.Fatalf("failed to create a grpc server connection: %v", err)
	}
	return conn
}
