package rpc

import (
	context "context"
	"fmt"
	"log"
	"net"

	grpc "google.golang.org/grpc"
)

type TableServer struct {
	UnimplementedTableServiceServer
}

var tableServer *grpc.Server

func Start(port int) {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	tableServer = grpc.NewServer()
	RegisterTableServiceServer(tableServer, &TableServer{})
	log.Printf("server listening at %v", lis.Addr())
	if err := tableServer.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}

func Stop() {
	if tableServer != nil {
		tableServer.Stop()
	}
}

func (s *TableServer) HostTable(ctx context.Context, in *HostTableInfo) (*Result, error) {
	return nil, nil
}

func (s *TableServer) TerimateTable(ctx context.Context, in *TerminateTableInfo) (*Result, error) {
	return nil, nil
}

func (s *TableServer) RunHand(ctx context.Context, in *HandInfo) (*RunHandResult, error) {
	return nil, nil
}
