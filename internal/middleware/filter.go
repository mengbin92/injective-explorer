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
// When testMode is true, all auth, rate limiting, and stats are bypassed for testing purposes.
//
// Parameters:
//   - rateLimiter: The rate limiter instance
//   - statsService: The usage stats service instance
//   - ipLocationService: Optional IP location service for geolocation (can be nil)
//   - ipWhitelistChecker: Optional IP whitelist checker (can be nil if no whitelist is configured)
//   - testMode: When true, bypasses all auth, rate limiting, and stats for testing
//
// Returns:
//   - middleware.Middleware: A middleware function that selectively applies auth, rate limiting, and stats
func SelectiveAuthAndRateLimitMiddleware(rateLimiter *RateLimiter, statsService *service.UsageStatsService, ipLocationService *service.IPLocationService, ipWhitelistChecker *IPWhitelistChecker, testMode bool) middleware.Middleware {
	authMiddleware := AuthMiddleware()
	rateLimitMiddleware := RateLimitMiddleware(rateLimiter)
	statsMiddleware := StatsMiddleware(statsService, ipLocationService)

	return func(handler middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req interface{}) (interface{}, error) {
			// Test mode: bypass all auth, rate limiting, and stats for testing purposes
			if testMode {
				return handler(ctx, req)
			}

			if request, ok := khttp.RequestFromServerContext(ctx); ok {
				path := request.URL.Path
				
				// Health check endpoint: skip all middleware for maximum performance
				if path == "/api/v1/health" {
					return handler(ctx, req)
				}
				
				// Check if this is a token management endpoint
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
						// Create virtual token info for IP address to enable stats logging
						ctx = context.WithValue(ctx, ContextKeyTokenID, uint(0)) // 0 indicates whitelisted IP
						ctx = context.WithValue(ctx, ContextKeyUserID, "whitelisted_ip_"+ip)
						ctx = context.WithValue(ctx, ContextKeyPlanType, "whitelisted")

						// Apply stats middleware for logging
						statsHandler := statsMiddleware(handler)
						return statsHandler(ctx, req)
					}
				}
			}

			// Apply middleware chain: stats -> auth -> rate limit -> handler
			// Stats middleware is placed at the outermost layer to record all requests,
			// including 401 authentication failures
			statsHandler := statsMiddleware(func(ctx context.Context, req interface{}) (interface{}, error) {
				authHandler := authMiddleware(func(ctx context.Context, req interface{}) (interface{}, error) {
					rateLimitHandler := rateLimitMiddleware(handler)
					return rateLimitHandler(ctx, req)
				})
				return authHandler(ctx, req)
			})

			return statsHandler(ctx, req)
		}
	}
}
