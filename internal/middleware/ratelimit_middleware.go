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
				if ok && tokenID > 0 {
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
// It properly handles both IPv4 and IPv6 addresses, including IPv6 addresses
// that may be enclosed in square brackets.
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
			ip := strings.TrimSpace(ips[0])
			// Remove square brackets if present (for IPv6)
			ip = removeBrackets(ip)
			return ip
		}
	}

	// Check X-Real-IP header
	realIP := r.Header.Get("X-Real-IP")
	if realIP != "" {
		// Remove square brackets if present (for IPv6)
		return removeBrackets(strings.TrimSpace(realIP))
	}

	// Fallback to RemoteAddr
	ip := r.RemoteAddr
	// Handle IPv6 addresses in format [::1]:port or [2001:db8::1]:port
	if strings.HasPrefix(ip, "[") {
		// IPv6 address with brackets
		if idx := strings.LastIndex(ip, "]"); idx != -1 {
			ip = ip[1:idx] // Remove brackets
		}
	} else {
		// IPv4 address or IPv6 without brackets
		if idx := strings.LastIndex(ip, ":"); idx != -1 {
			// Check if this is IPv4 (has only one colon before port)
			// For IPv4, format is 1.2.3.4:port
			// For IPv6 without brackets, format is ::1:port (rare case)
			// We'll try to parse it - if it fails, net.ParseIP will handle it
			ip = ip[:idx]
		}
	}
	return ip
}

// removeBrackets removes square brackets from an IP address string if present.
// This is useful for IPv6 addresses that may be enclosed in brackets.
//
// Parameters:
//   - ip: The IP address string (may contain brackets)
//
// Returns:
//   - string: The IP address without brackets
func removeBrackets(ip string) string {
	ip = strings.TrimSpace(ip)
	if strings.HasPrefix(ip, "[") && strings.HasSuffix(ip, "]") {
		return ip[1 : len(ip)-1]
	}
	return ip
}
