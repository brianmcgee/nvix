package test

import (
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func GrpcConn(lis net.Listener, t TestingT) *grpc.ClientConn {
	t.Helper()
	conn, err := grpc.Dial(lis.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("failed to create a grpc server connection: %v", err)
	}
	return conn
}
