// Package middleware provides HTTP middleware for authentication and rate limiting.
package middleware

import (
	"context"
	"fmt"
	"time"

	"injective-explorer/internal/global"
	"injective-explorer/internal/service"
	"injective-explorer/provider/cache"

	"github.com/bytedance/sonic"
	"github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/middleware"
	khttp "github.com/go-kratos/kratos/v2/transport/http"
)

// UserAuthMiddleware extracts user_id from X-User-ID header for token management endpoints.
// This is a simple implementation - in production, this should validate JWT or session token.
func UserAuthMiddleware() middleware.Middleware {
	return func(handler middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req interface{}) (interface{}, error) {
			if request, ok := khttp.RequestFromServerContext(ctx); ok {
				userID := request.Header.Get("X-User-ID")
				if userID == "" {
					return nil, errors.New(401, "UNAUTHORIZED", "Missing X-User-ID header")
				}

				ctx = context.WithValue(ctx, ContextKeyUserID, userID)
				return handler(ctx, req)
			}

			return handler(ctx, req)
		}
	}
}

// EnhancedUserAuthMiddleware provides enhanced authentication with external user service.
// It validates user identity and status by calling the external user detail API.
// If fallback is true, allows access when user service is unavailable.
func EnhancedUserAuthMiddleware(userClient *service.UserServiceClient, fallback bool) middleware.Middleware {
	return func(handler middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req interface{}) (interface{}, error) {
			httpReq, ok := khttp.RequestFromServerContext(ctx)
			if !ok {
				return handler(ctx, req)
			}

			userID := httpReq.Header.Get("X-User-ID")
			if userID == "" {
				return nil, errors.New(401, "UNAUTHORIZED", "Missing X-User-ID header")
			}

			userDetail, err := getUserDetailWithCache(ctx, userClient, userID)
			if err != nil {
				global.Logger.Warnf("user detail verification failed for %s: %v", userID, err)

				if fallback {
					userDetail = &service.UserDetail{
						ID:      0,
						Account: userID,
						Source:  "unknown",
						Status:  1,
						Action:  "unknown",
					}
				} else {
					return nil, errors.New(401, "UNAUTHORIZED", "User verification failed")
				}
			}

			if !userDetail.IsActive() {
				return nil, errors.New(403, "FORBIDDEN", "User account is disabled")
			}

			ctx = context.WithValue(ctx, ContextKeyUserID, userID)
			ctx = context.WithValue(ctx, ContextKeyUserDetail, userDetail)

			return handler(ctx, req)
		}
	}
}

func getUserDetailWithCache(ctx context.Context, client *service.UserServiceClient, userID string) (*service.UserDetail, error) {
	cacheKey := fmt.Sprintf("user:detail:%s", userID)

	if cached, err := cache.GetRedisClient().Get(ctx, cacheKey).Result(); err == nil && cached != "" {
		var detail service.UserDetail
		if err := sonic.UnmarshalString(cached, &detail); err == nil {
			return &detail, nil
		}
	}

	detail, err := client.GetUserDetail(ctx, userID)
	if err != nil {
		return nil, err
	}

	if data, err := sonic.MarshalString(detail); err == nil {
		if err := cache.GetRedisClient().Set(ctx, cacheKey, data, 5*time.Minute).Err(); err != nil {
			global.Logger.Warnf("failed to cache user detail: %v", err)
		}
	}

	return detail, nil
}
