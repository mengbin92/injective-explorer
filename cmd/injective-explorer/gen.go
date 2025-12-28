package main

import (
	"injective-explorer/internal/conf"
	"injective-explorer/internal/server"

	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/log"

	_ "go.uber.org/automaxprocs"
)

// Injectors from wire.go:

// wireApp init kratos application.
func wireApp(confServer *conf.Server, confData *conf.Data, logger log.Logger, bootstrap *conf.Bootstrap) (*kratos.App, func(), error) {

	grpcServer := server.NewGRPCServer(confServer, logger)
	httpServer := server.NewHTTPServer(confServer, logger, bootstrap)
	app := newApp(logger, grpcServer, httpServer)
	return app, nil, nil
}
