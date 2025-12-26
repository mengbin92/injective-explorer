// Package middleware provides HTTP middleware for authentication and rate limiting.
package middleware

import (
	"context"
	"strings"

	"github.com/go-kratos/kratos/v2/middleware"
	khttp "github.com/go-kratos/kratos/v2/transport/http"
)

// SelectiveAuthAndRateLimitMiddleware creates a middleware that applies token auth and rate limiting
// to all routes except token management endpoints (/api/v1/tokens).
// Token management endpoints use user authentication (handled in service).
//
// Parameters:
//   - rateLimiter: The rate limiter instance
//
// Returns:
//   - middleware.Middleware: A middleware function that selectively applies auth and rate limiting
func SelectiveAuthAndRateLimitMiddleware(rateLimiter *RateLimiter) middleware.Middleware {
	authMiddleware := AuthMiddleware()
	rateLimitMiddleware := RateLimitMiddleware(rateLimiter)

	return func(handler middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req interface{}) (interface{}, error) {
			// Check if this is a token management endpoint
			if request, ok := khttp.RequestFromServerContext(ctx); ok {
				path := request.URL.Path
				if strings.HasPrefix(path, "/api/v1/tokens") {
					// Token management endpoints use user auth (handled in service)
					// Skip token auth and rate limiting
					return handler(ctx, req)
				}
			}

			// Apply auth middleware
			authHandler := authMiddleware(func(ctx context.Context, req interface{}) (interface{}, error) {
				// Apply rate limit middleware
				rateLimitHandler := rateLimitMiddleware(handler)
				return rateLimitHandler(ctx, req)
			})

			return authHandler(ctx, req)
		}
	}
}
