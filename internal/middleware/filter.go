// Package middleware provides HTTP middleware for authentication and rate limiting.
package middleware

import (
	"context"
	"strings"

	"injective-explorer/internal/service"

	"github.com/go-kratos/kratos/v2/middleware"
	khttp "github.com/go-kratos/kratos/v2/transport/http"
)

// SelectiveAuthAndRateLimitMiddleware creates a middleware that applies token auth, rate limiting,
// and statistics recording to all routes except token management endpoints (/api/v1/tokens).
// Token management endpoints use user authentication (handled in service).
// Whitelisted IPs bypass token authentication and rate limiting.
//
// Parameters:
//   - rateLimiter: The rate limiter instance
//   - statsService: The usage stats service instance
//   - ipLocationService: Optional IP location service for geolocation (can be nil)
//   - ipWhitelistChecker: Optional IP whitelist checker (can be nil if no whitelist is configured)
//
// Returns:
//   - middleware.Middleware: A middleware function that selectively applies auth, rate limiting, and stats
func SelectiveAuthAndRateLimitMiddleware(rateLimiter *RateLimiter, statsService *service.UsageStatsService, ipLocationService *service.IPLocationService, ipWhitelistChecker *IPWhitelistChecker) middleware.Middleware {
	authMiddleware := AuthMiddleware()
	rateLimitMiddleware := RateLimitMiddleware(rateLimiter)
	statsMiddleware := StatsMiddleware(statsService, ipLocationService)

	return func(handler middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req interface{}) (interface{}, error) {
			if request, ok := khttp.RequestFromServerContext(ctx); ok {
				// Check if this is a token management endpoint
				path := request.URL.Path
				if strings.HasPrefix(path, "/api/v1/tokens") {
					// Token management endpoints use user auth (handled in service)
					// Skip token auth, rate limiting, and stats
					return handler(ctx, req)
				}

				// Check if IP is whitelisted
				if ipWhitelistChecker != nil {
					ip := getClientIP(request)
					if ipWhitelistChecker.IsWhitelisted(ip) {
						// Whitelisted IPs bypass token auth and rate limiting
						// Still apply stats middleware for logging
						statsHandler := statsMiddleware(handler)
						return statsHandler(ctx, req)
					}
				}
			}

			// Apply middleware chain: auth -> rate limit -> stats -> handler
			authHandler := authMiddleware(func(ctx context.Context, req interface{}) (interface{}, error) {
				rateLimitHandler := rateLimitMiddleware(func(ctx context.Context, req interface{}) (interface{}, error) {
					statsHandler := statsMiddleware(handler)
					return statsHandler(ctx, req)
				})
				return rateLimitHandler(ctx, req)
			})

			return authHandler(ctx, req)
		}
	}
}
