// Package server provides server initialization for both gRPC and HTTP servers.
package server

import (
	v1 "injective-explorer/api/explorer/v1"
	"injective-explorer/internal/conf"
	"injective-explorer/internal/service"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/middleware/recovery"
	krpc "github.com/go-kratos/kratos/v2/transport/grpc"
	"google.golang.org/grpc"
)

var (
	// standardGrpcServer is the underlying grpc.Server instance used for gRPC-Web
	standardGrpcServer *grpc.Server
)

// NewGRPCServer creates and configures a new gRPC server instance.
// It sets up middleware, network configuration, address, and timeout from the provided configuration.
// The server registers the demo service implementation.
//
// Parameters:
//   - c: Server configuration containing gRPC settings
//   - logger: Logger instance for server logging
//
// Returns:
//   - *grpc.Server: A configured gRPC server ready to accept connections
func NewGRPCServer(c *conf.Server, logger log.Logger) *krpc.Server {
	var opts = []krpc.ServerOption{
		krpc.Middleware(
			recovery.Recovery(),
		),
	}
	if c.Grpc.Network != "" {
		opts = append(opts, krpc.Network(c.Grpc.Network))
	}
	if c.Grpc.Addr != "" {
		opts = append(opts, krpc.Address(c.Grpc.Addr))
	}
	if c.Grpc.Timeout != nil {
		opts = append(opts, krpc.Timeout(c.Grpc.Timeout.AsDuration()))
	}
	srv := krpc.NewServer(opts...)

	demoService := service.NewDemoService()
	v1.RegisterDemoServer(srv, demoService)

	// Also create and register services on a standard grpc.Server for gRPC-Web
	standardGrpcServer = grpc.NewServer()
	v1.RegisterDemoServer(standardGrpcServer, demoService)

	return srv
}

// GetStandardGRPCServer returns the underlying standard grpc.Server instance.
// This is used by the gRPC-Web wrapper to handle gRPC-Web requests.
func GetStandardGRPCServer() *grpc.Server {
	return standardGrpcServer
}
