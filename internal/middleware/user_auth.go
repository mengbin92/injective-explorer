// Package middleware provides HTTP middleware for authentication and rate limiting.
package middleware

import (
	"context"

	"github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/middleware"
	khttp "github.com/go-kratos/kratos/v2/transport/http"
)

// UserAuthMiddleware extracts user_id from X-User-ID header for token management endpoints.
// This is a simple implementation - in production, this should validate JWT or session token.
//
// Returns:
//   - middleware.Middleware: A middleware function that extracts user ID from header
func UserAuthMiddleware() middleware.Middleware {
	return func(handler middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req interface{}) (interface{}, error) {
			if request, ok := khttp.RequestFromServerContext(ctx); ok {
				// Try to get user_id from header (set by external auth system)
				userID := request.Header.Get("X-User-ID")
				if userID == "" {
					return nil, errors.New(401, "UNAUTHORIZED", "Missing X-User-ID header")
				}

				// Store user_id in context
				ctx = context.WithValue(ctx, ContextKeyUserID, userID)

				return handler(ctx, req)
			}

			return handler(ctx, req)
		}
	}
}
