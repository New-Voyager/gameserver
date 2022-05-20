package rpc

import (
	context "context"

	grpc "google.golang.org/grpc"
)

type TableServer struct {
	UnimplementedTableServiceServer
}

func (s *TableServer) HostTable(ctx context.Context, in *HostTableInfo, opts ...grpc.CallOption) (*Result, error) {
	return nil, nil
}

func (s *TableServer) TerimateTable(ctx context.Context, in *TerminateTableInfo, opts ...grpc.CallOption) (*Result, error) {
	return nil, nil
}

func (s *TableServer) RunHand(ctx context.Context, in *HandInfo, opts ...grpc.CallOption) (*Result, error) {
	return nil, nil
}
