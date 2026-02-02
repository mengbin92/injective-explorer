// Package middleware provides HTTP middleware for authentication and rate limiting.
package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"injective-explorer/internal/models"
	"injective-explorer/provider/cache"
	"injective-explorer/provider/db"

	"github.com/bytedance/sonic"
	"github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/middleware"
	khttp "github.com/go-kratos/kratos/v2/transport/http"
	"gorm.io/gorm"
)

// Context keys for storing token information in context.
type contextKey string

const (
	// ContextKeyTokenID is the context key for token ID
	ContextKeyTokenID contextKey = "token_id"
	// ContextKeyUserID is the context key for user ID
	ContextKeyUserID contextKey = "user_id"
	// ContextKeyPlanType is the context key for plan type
	ContextKeyPlanType contextKey = "plan_type"
	// ContextKeyUserDetail is the context key for user detail from external service
	ContextKeyUserDetail contextKey = "user_detail"
)

// AuthMiddleware validates API token from Authorization header.
// It extracts the Bearer token, validates it against the database,
// and stores token information in the context.
//
// Returns:
//   - middleware.Middleware: A middleware function that validates API tokens
func AuthMiddleware() middleware.Middleware {
	return func(handler middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req interface{}) (interface{}, error) {
			// Extract token from HTTP header
			// In Kratos v2, HTTP request is available through RequestFromServerContext
			if tr, ok := khttp.RequestFromServerContext(ctx); ok {
				authHeader := tr.Header.Get("Authorization")
				if authHeader == "" {
					return nil, errors.New(401, "UNAUTHORIZED", "Missing Authorization header")
				}

				// Parse Bearer token
				parts := strings.Split(authHeader, " ")
				if len(parts) != 2 || parts[0] != "Bearer" {
					return nil, errors.New(401, "UNAUTHORIZED", "Invalid Authorization header format")
				}

				token := parts[1]

				// Validate token
				tokenInfo, err := validateToken(ctx, token)
				if err != nil {
					return nil, err
				}

				// Check token status
				if tokenInfo.Status != "active" {
					return nil, errors.New(401, "TOKEN_DISABLED", "API token is disabled")
				}

				// Update last used time (async)
				go updateLastUsedTime(tokenInfo.ID)

				// Store token info in context
				ctx = context.WithValue(ctx, ContextKeyTokenID, tokenInfo.ID)
				ctx = context.WithValue(ctx, ContextKeyUserID, tokenInfo.UserID)
				ctx = context.WithValue(ctx, ContextKeyPlanType, tokenInfo.PlanType)

				return handler(ctx, req)
			}

			return handler(ctx, req)
		}
	}
}

// validateToken validates a token by hashing it and querying the database.
// It uses Redis cache to reduce database queries for better performance.
//
// Parameters:
//   - ctx: Context for the operation
//   - token: The API token string to validate
//
// Returns:
//   - *models.APIToken: The token information if valid
//   - error: Error if validation fails (token not found, database error, etc.)
func validateToken(ctx context.Context, token string) (*models.APIToken, error) {
	// Hash token
	hash := sha256.Sum256([]byte(token))
	tokenHash := hex.EncodeToString(hash[:])

	// 1. Try Redis cache first (performance optimization)
	redisClient := cache.GetRedisClient()
	if redisClient != nil {
		cacheKey := fmt.Sprintf("token:hash:%s", tokenHash)
		cachedData, err := redisClient.Get(ctx, cacheKey).Result()
		if err == nil && cachedData != "" {
			// Cache hit, unmarshal token info
			var tokenInfo models.APIToken
			if err := sonic.Unmarshal([]byte(cachedData), &tokenInfo); err == nil {
				return &tokenInfo, nil
			}
			// If unmarshal fails, continue to database query
		}
		// If cache miss or error, continue to database query (graceful degradation)
	}

	// 2. Cache miss or Redis unavailable, query database
	var tokenInfo models.APIToken
	err := db.Get().Where("token_hash = ?", tokenHash).First(&tokenInfo).Error
	if err == gorm.ErrRecordNotFound {
		return nil, errors.New(401, "TOKEN_INVALID", "Invalid API token")
	}
	if err != nil {
		return nil, errors.New(500, "DATABASE_ERROR", "Failed to validate token")
	}

	// 3. Write to cache asynchronously (non-blocking)
	// Only cache active tokens to avoid caching disabled tokens
	if redisClient != nil && tokenInfo.Status == "active" {
		go func() {
			cacheKey := fmt.Sprintf("token:hash:%s", tokenHash)
			tokenJSON, err := sonic.Marshal(tokenInfo)
			if err == nil {
				// Cache for 5 minutes
				redisClient.Set(context.Background(), cacheKey, tokenJSON, 5*time.Minute)
			}
		}()
	}

	return &tokenInfo, nil
}

// updateLastUsedTime updates the last used time for a token.
// This function runs asynchronously and does not block the request.
//
// Parameters:
//   - tokenID: The token ID to update
func updateLastUsedTime(tokenID uint) {
	now := time.Now()
	db.Get().Model(&models.APIToken{}).
		Where("id = ?", tokenID).
		Update("last_used_at", now)
}
