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
	// Configure CORS with security best practices
	// Security: CORS configuration must follow browser security rules:
	// - If AllowedOrigins contains "*", AllowCredentials must be false
	// - If AllowCredentials is true, AllowedOrigins must specify exact origins (not "*")
	var corsOpts []handlers.CORSOption
	allowedOrigins := []string{"*"} // Default: allow all origins
	allowCredentials := false       // Default: disabled for security when using "*"

	// Security: If using "*" origin, credentials must be disabled
	// This prevents CORS security vulnerabilities
	if len(allowedOrigins) == 1 && allowedOrigins[0] == "*" {
		allowCredentials = false
	}

	corsOpts = append(corsOpts,
		handlers.AllowedOrigins(allowedOrigins),
		handlers.AllowedMethods([]string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}),
		handlers.AllowedHeaders([]string{
			"Content-Type",
			"Authorization",
			"X-Requested-With",
			// Note: Accept, Origin, and Access-Control-Request-* headers are simple headers
			// and don't need to be explicitly allowed
		}),
		handlers.MaxAge(86400), // Cache preflight requests for 24 hours
	)

	if allowCredentials {
		corsOpts = append(corsOpts, handlers.AllowCredentials())
	}

	var opts = []http.ServerOption{
		http.Middleware(
			recovery.Recovery(),
		),
		// Add CORS filter to handle cross-origin requests
		// The CORS middleware will automatically handle OPTIONS preflight requests
		http.Filter(handlers.CORS(corsOpts...)),
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
