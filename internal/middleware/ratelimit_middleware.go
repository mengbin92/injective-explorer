// Package middleware provides HTTP middleware for authentication and rate limiting.
package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/middleware"
	khttp "github.com/go-kratos/kratos/v2/transport/http"
)

// RateLimitMiddleware applies rate limiting to requests.
// It checks IP rate limit, token rate limit, and user monthly limit.
//
// Parameters:
//   - limiter: The rate limiter instance
//
// Returns:
//   - middleware.Middleware: A middleware function that applies rate limiting
func RateLimitMiddleware(limiter *RateLimiter) middleware.Middleware {
	return func(handler middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req interface{}) (interface{}, error) {
			if request, ok := khttp.RequestFromServerContext(ctx); ok {

				// 1. Check IP rate limit (100 requests per minute)
				ip := getClientIP(request)
				allowed, err := limiter.CheckIPRateLimit(ctx, ip, 100)
				if err != nil {
					return nil, errors.New(500, "RATE_LIMIT_ERROR", "Rate limit check failed")
				}
				if !allowed {
					return nil, errors.New(429, "IP_RATE_LIMIT", "IP request rate limit exceeded")
				}

				// 2. Check token rate limit (5 requests per second)
				tokenID, ok := ctx.Value(ContextKeyTokenID).(uint)
				if ok {
					allowed, err = limiter.CheckTokenRateLimit(ctx, int64(tokenID), 5)
					if err != nil {
						return nil, errors.New(500, "RATE_LIMIT_ERROR", "Rate limit check failed")
					}
					if !allowed {
						return nil, errors.New(429, "TOKEN_RATE_LIMIT", "Token request rate limit exceeded")
					}
				}

				// 3. Check user monthly limit (100,000 requests per month)
				userID, ok := ctx.Value(ContextKeyUserID).(string)
				if ok {
					allowed, _, err := limiter.CheckUserMonthlyLimit(ctx, userID, 100000)
					if err != nil {
						return nil, errors.New(500, "RATE_LIMIT_ERROR", "Rate limit check failed")
					}
					if !allowed {
						return nil, errors.New(429, "MONTHLY_QUOTA_EXCEEDED",
							"Monthly API quota exceeded")
					}

					// Increment usage (async)
					go limiter.IncrementUserUsage(context.Background(), userID)
				}
			}

			return handler(ctx, req)
		}
	}
}

// getClientIP extracts the client IP address from the HTTP request.
// It checks X-Forwarded-For header (for proxy/load balancer), X-Real-IP header,
// and falls back to RemoteAddr.
//
// Parameters:
//   - r: The HTTP request
//
// Returns:
//   - string: The client IP address
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header (for proxy/load balancer)
	forwarded := r.Header.Get("X-Forwarded-For")
	if forwarded != "" {
		// Take first IP in the chain
		ips := strings.Split(forwarded, ",")
		if len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}

	// Check X-Real-IP header
	realIP := r.Header.Get("X-Real-IP")
	if realIP != "" {
		return realIP
	}

	// Fallback to RemoteAddr
	ip := r.RemoteAddr
	if idx := strings.LastIndex(ip, ":"); idx != -1 {
		ip = ip[:idx]
	}
	return ip
}

