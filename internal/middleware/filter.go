// Package middleware provides HTTP middleware for authentication and rate limiting.
package middleware

import (
	"context"
	"strings"

	"injective-explorer/internal/service"

	"github.com/go-kratos/kratos/v2/middleware"
	khttp "github.com/go-kratos/kratos/v2/transport/http"
)

// whitelistedTokenID is used to identify whitelisted IPs in the context.
// It is set to 0 to distinguish from actual token IDs (which start from 1).
const whitelistedTokenID = uint(0)

// SelectiveAuthAndRateLimitMiddleware returns a middleware that conditionally applies authentication,
// rate limiting, and statistics recording based on the request path and configuration.
// When testMode is true, all middleware is bypassed.
// Token management endpoints use user authentication handled in the service layer.
// Whitelisted IPs bypass authentication and rate limiting but still have statistics recorded.
func SelectiveAuthAndRateLimitMiddleware(
	rateLimiter *RateLimiter,
	statsService *service.UsageStatsService,
	ipLocationService *service.IPLocationService,
	ipWhitelistChecker *IPWhitelistChecker,
	testMode bool,
) middleware.Middleware {
	authMiddleware := AuthMiddleware()
	rateLimitMiddleware := RateLimitMiddleware(rateLimiter)
	statsMiddleware := StatsMiddleware(statsService, ipLocationService)

	return func(handler middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req interface{}) (interface{}, error) {
			if testMode {
				return handler(ctx, req)
			}

			request, ok := khttp.RequestFromServerContext(ctx)
			if !ok {
				return handler(ctx, req)
			}

			path := request.URL.Path

			if path == "/api/v1/health" {
				return handler(ctx, req)
			}

			if strings.HasPrefix(path, "/api/v1/tokens") {
				return handler(ctx, req)
			}

			if ipWhitelistChecker != nil {
				ip := getClientIP(request)
				if ipWhitelistChecker.IsWhitelisted(ip) {
					ctx = context.WithValue(ctx, ContextKeyTokenID, whitelistedTokenID)
					ctx = context.WithValue(ctx, ContextKeyUserID, "whitelisted_ip_"+ip)
					ctx = context.WithValue(ctx, ContextKeyPlanType, "whitelisted")

					statsHandler := statsMiddleware(handler)
					return statsHandler(ctx, req)
				}
			}

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
