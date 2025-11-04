// Package server provides server initialization for both gRPC and HTTP servers.
package server

import (
	v1 "injective-explorer/api/explorer/v1"
	"injective-explorer/internal/conf"
	"injective-explorer/internal/service"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/middleware/recovery"
	"github.com/go-kratos/kratos/v2/transport/http"
	"github.com/gorilla/handlers"
)

// NewHTTPServer creates and configures a new HTTP server instance.
// It sets up middleware, network configuration, address, and timeout from the provided configuration.
// The server registers the demo service HTTP handler.
//
// Parameters:
//   - c: Server configuration containing HTTP settings
//   - logger: Logger instance for server logging
//
// Returns:
//   - *http.Server: A configured HTTP server ready to accept connections
func NewHTTPServer(c *conf.Server, logger log.Logger) *http.Server {
	var opts = []http.ServerOption{
		http.Middleware(
			recovery.Recovery(),
		),
		http.Filter(handlers.CORS(
			handlers.AllowedOrigins([]string{"*"}),
			handlers.AllowedMethods([]string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}),
			handlers.AllowedHeaders([]string{"Content-Type", "Authorization", "X-Requested-With", "Your-Custom-Header"}), // 注意点：包含所有用到的请求头
			handlers.AllowCredentials(), // 允许携带凭证（如 cookies）
		)),
	}
	if c.Http.Network != "" {
		opts = append(opts, http.Network(c.Http.Network))
	}
	if c.Http.Addr != "" {
		opts = append(opts, http.Address(c.Http.Addr))
	}
	if c.Http.Timeout != nil {
		opts = append(opts, http.Timeout(c.Http.Timeout.AsDuration()))
	}
	srv := http.NewServer(opts...)

	demoService := service.NewDemoService()
	v1.RegisterDemoHTTPServer(srv, demoService)

	return srv
}
