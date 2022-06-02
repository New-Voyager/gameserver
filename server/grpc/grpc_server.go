package grpc

import (
	context "context"
	"fmt"
	"log"
	"net"

	grpc "google.golang.org/grpc"
	"voyager.com/logging"
	"voyager.com/server/nats"
	"voyager.com/server/rpc"
)

// var natsGameManager *nats.GameManager

var grpcLogger = logging.GetZeroLogger("rpc::rpc", nil)

type TableServer struct {
	rpc.UnimplementedTableServiceServer
}

var tableServer *grpc.Server
var natsGameManager *nats.GameManager

func Start(gameManager *nats.GameManager, port int) {
	natsGameManager = gameManager
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	tableServer = grpc.NewServer()
	rpc.RegisterTableServiceServer(tableServer, &TableServer{})
	grpcLogger.Info().Msgf("starting grpc server on port %d", port)
	if err := tableServer.Serve(lis); err != nil {
		grpcLogger.Error().Msgf("failed to serve: %v", err)
	}
}

func Stop() {
	if tableServer != nil {
		tableServer.Stop()
	}
}

func (s *TableServer) HostTable(ctx context.Context, in *rpc.HostTableInfo) (*rpc.Result, error) {
	_, err := natsGameManager.NewTournamentGame(in.GameCode, uint64(in.TournamentId), uint32(in.TableNo))
	if err != nil {
		grpcLogger.Error().Msgf("Could not host table for tournament: %d, table: %d Error: %v", in.TournamentId, in.TableNo, err)
		return &rpc.Result{
			Success: false,
			Error:   err.Error(),
		}, err
	}
	return &rpc.Result{
		Success: true,
		Error:   "",
	}, nil
}

func (s *TableServer) TerimateTable(ctx context.Context, in *rpc.TerminateTableInfo) (*rpc.Result, error) {
	return nil, nil
}

func (s *TableServer) RunHand(ctx context.Context, in *rpc.HandInfo) (*rpc.RunHandResult, error) {
	err := natsGameManager.DealTournamentHand(in.GameCode, in)
	if err != nil {
		grpcLogger.Error().Msgf("Could not host table for tournament: %d, table: %d Error: %v", in.TournamentId, in.TableNo, err)
		return &rpc.RunHandResult{
			Success: false,
			Error:   err.Error(),
		}, err
	}

	return nil, nil
}
