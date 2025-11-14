// Package server provides server initialization for both gRPC and HTTP servers.
package server

import (
	"net/http"
	"strings"

	"injective-explorer/internal/conf"

	"github.com/go-kratos/kratos/v2/log"
	krpc "github.com/go-kratos/kratos/v2/transport/grpc"
	grpcweb "github.com/improbable-eng/grpc-web/go/grpcweb"
	"google.golang.org/grpc"
)

// NewGRPCWebServer creates and configures a new HTTP server that wraps a gRPC server
// to support gRPC-Web clients. This allows browsers to connect to gRPC services
// using the gRPC-Web protocol.
//
// Parameters:
//   - c: Server configuration containing HTTP settings for gRPC-Web
//   - grpcServer: The Kratos gRPC server (used for reference, actual server comes from GetStandardGRPCServer)
//   - logger: Logger instance for server logging
//
// Returns:
//   - *http.Server: A configured HTTP server that handles gRPC-Web requests
func NewGRPCWebServer(c *conf.Server, grpcServer *krpc.Server, logger log.Logger) *http.Server {
	// Get the standard grpc.Server that was created in NewGRPCServer
	underlyingGrpcServer := GetStandardGRPCServer()
	if underlyingGrpcServer == nil {
		log.NewHelper(logger).Errorf("standard grpc.Server is nil, gRPC-Web will not work properly")
		// Create a fallback server (services won't be registered, but at least it won't crash)
		underlyingGrpcServer = grpc.NewServer()
	}

	// Configure CORS based on config
	var corsOptions []grpcweb.Option
	if c.GrpcWeb != nil && c.GrpcWeb.EnableCors {
		corsOptions = append(corsOptions, grpcweb.WithCorsForRegisteredEndpointsOnly(false))

		// Configure allowed origins
		if len(c.GrpcWeb.AllowedOrigins) > 0 {
			allowedOrigins := make(map[string]bool)
			allowAll := false
			for _, origin := range c.GrpcWeb.AllowedOrigins {
				if origin == "*" {
					allowAll = true
					break
				}
				allowedOrigins[origin] = true
			}

			if allowAll {
				corsOptions = append(corsOptions, grpcweb.WithOriginFunc(func(origin string) bool {
					return true
				}))
			} else {
				corsOptions = append(corsOptions, grpcweb.WithOriginFunc(func(origin string) bool {
					return allowedOrigins[origin]
				}))
			}
		} else {
			// Default: allow all origins if enable_cors is true but no origins specified
			corsOptions = append(corsOptions, grpcweb.WithOriginFunc(func(origin string) bool {
				return true
			}))
		}
	}

	// Wrap the gRPC server with gRPC-Web support
	wrappedGrpc := grpcweb.WrapServer(underlyingGrpcServer, corsOptions...)

	// Create HTTP handler that routes gRPC-Web requests to the wrapped server
	handler := http.HandlerFunc(func(resp http.ResponseWriter, req *http.Request) {
		// Log request for debugging
		log.NewHelper(logger).Debugf("gRPC-Web request: method=%s, path=%s, content-type=%s",
			req.Method, req.URL.Path, req.Header.Get("Content-Type"))

		// Check if this is a gRPC-Web request
		isGrpcWeb := wrappedGrpc.IsGrpcWebRequest(req)
		isCors := wrappedGrpc.IsAcceptableGrpcCorsRequest(req)

		// Also check for gRPC-Web headers manually (for better compatibility)
		contentType := req.Header.Get("Content-Type")
		hasGrpcWebHeader := req.Header.Get("X-Grpc-Web") == "1" ||
			contentType == "application/grpc-web+proto" ||
			contentType == "application/grpc-web-text"

		if isGrpcWeb || isCors || hasGrpcWebHeader {
			// Handle gRPC-Web request
			log.NewHelper(logger).Debugf("handling gRPC-Web request: isGrpcWeb=%v, isCors=%v, hasHeader=%v",
				isGrpcWeb, isCors, hasGrpcWebHeader)
			wrappedGrpc.ServeHTTP(resp, req)
			return
		}

		// For non-gRPC-Web requests, return 404 with helpful message
		log.NewHelper(logger).Warnf("non-gRPC-Web request rejected: method=%s, path=%s, content-type=%s",
			req.Method, req.URL.Path, contentType)
		resp.WriteHeader(http.StatusNotFound)
		resp.Write([]byte("Not Found: This endpoint only handles gRPC-Web requests. " +
			"Please set Content-Type: application/grpc-web+proto and X-Grpc-Web: 1 headers."))
	})

	// Create HTTP server
	// Use address from config, default to :8080 if not specified
	addr := ":8080" // Default port for gRPC-Web
	if c.GrpcWeb != nil && c.GrpcWeb.Addr != "" {
		addr = c.GrpcWeb.Addr
		// Ensure address starts with ":" if it's just a port number
		if !strings.HasPrefix(addr, ":") && !strings.Contains(addr, ":") {
			addr = ":" + addr
		}
	}

	srv := &http.Server{
		Addr:    addr,
		Handler: handler,
	}

	corsEnabled := c.GrpcWeb != nil && c.GrpcWeb.EnableCors
	allowedOrigins := "all"
	if c.GrpcWeb != nil && len(c.GrpcWeb.AllowedOrigins) > 0 {
		allowedOrigins = strings.Join(c.GrpcWeb.AllowedOrigins, ", ")
	}
	log.NewHelper(logger).Infof("gRPC-Web server configured on %s (CORS: %v, allowed origins: %s)",
		addr, corsEnabled, allowedOrigins)

	return srv
}
