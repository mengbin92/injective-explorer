// Package server provides server initialization for both gRPC and HTTP servers.
package server

import (
	v1 "injective-explorer/api/explorer/v1"
	"injective-explorer/internal/conf"
	"injective-explorer/internal/service"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/middleware/recovery"
	"github.com/go-kratos/kratos/v2/transport/grpc"
)

// NewGRPCServer creates and configures a new gRPC server instance.
// It sets up middleware, network configuration, address, and timeout from the provided configuration.
// The server registers the explorer service implementation.
//
// Parameters:
//   - c: Server configuration containing gRPC settings
//   - logger: Logger instance for server logging
//
// Returns:
//   - *grpc.Server: A configured gRPC server ready to accept connections
func NewGRPCServer(c *conf.Server, logger log.Logger) *grpc.Server {
	var opts = []grpc.ServerOption{
		grpc.Middleware(
			recovery.Recovery(),
		),
	}
	if c.Grpc.Network != "" {
		opts = append(opts, grpc.Network(c.Grpc.Network))
	}
	if c.Grpc.Addr != "" {
		opts = append(opts, grpc.Address(c.Grpc.Addr))
	}
	if c.Grpc.Timeout != nil {
		opts = append(opts, grpc.Timeout(c.Grpc.Timeout.AsDuration()))
	}
	srv := grpc.NewServer(opts...)

	explorerService := service.NewExplorerService()
	v1.RegisterExplorerServer(srv, explorerService)

	contractInfoService := service.NewContractInfoService()
	v1.RegisterContractInfoServiceServer(srv, contractInfoService)

	return srv
}
