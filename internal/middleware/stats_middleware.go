// Package middleware provides HTTP middleware for authentication and rate limiting.
package middleware

import (
	"context"
	"net/http"
	"time"

	"injective-explorer/internal/global"
	"injective-explorer/internal/models"
	"injective-explorer/internal/service"

	"github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/middleware"
	khttp "github.com/go-kratos/kratos/v2/transport/http"
)

// StatsMiddleware records API request statistics asynchronously.
// It measures response time and logs request details to the database.
// Optionally includes IP geolocation if IPLocationService is provided.
//
// Parameters:
//   - statsService: The usage stats service instance
//   - ipLocationService: Optional IP location service for geolocation (can be nil)
//
// Returns:
//   - middleware.Middleware: A middleware function that records request statistics
func StatsMiddleware(statsService *service.UsageStatsService, ipLocationService *service.IPLocationService) middleware.Middleware {
	return func(handler middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req interface{}) (interface{}, error) {
			startTime := time.Now()

			// Execute handler
			reply, err := handler(ctx, req)

			// Calculate response time
			responseTime := int(time.Since(startTime).Milliseconds())

			// Record statistics (async, non-blocking)
			if request, ok := khttp.RequestFromServerContext(ctx); ok {
				go func() {
					// Extract token and user info from context
					tokenID, _ := ctx.Value(ContextKeyTokenID).(uint)
					userID, _ := ctx.Value(ContextKeyUserID).(string)

					// Only log if we have token info (authenticated requests) or whitelisted IP
					if tokenID == 0 && userID == "" {
						return // Skip logging for completely unauthenticated requests
					}

					// Get client IP
					ip := getClientIP(request)

					// Get IP location asynchronously (non-blocking)
					var ipLocation string
					if ipLocationService != nil {
						ipLocation = ipLocationService.GetLocation(context.Background(), ip)
					}

					// Determine status code based on error
					statusCode := http.StatusOK
					var errorMessage string
					if err != nil {
						// Try to extract status code from kratos error
						// Kratos errors.New(code, reason, message) stores code in the error
						if kratosErr := errors.FromError(err); kratosErr != nil {
							statusCode = int(kratosErr.GetCode())
						} else {
							// Default to 500 for unknown errors
							statusCode = http.StatusInternalServerError
						}

						// Truncate error message to avoid very long messages
						errMsg := err.Error()
						if len(errMsg) > 1000 {
							errMsg = errMsg[:1000] + "..."
						}
						errorMessage = errMsg
					}

					// Create log entry
					log := &models.APIRequestLog{
						TokenID:      tokenID,
						UserID:       userID,
						IPAddress:    ip,
						IPLocation:   ipLocation,
						Endpoint:     request.URL.Path,
						Method:       request.Method,
						StatusCode:   statusCode,
						ResponseTime: responseTime,
						ErrorMessage: errorMessage,
					}

					// Async record (non-blocking)
					if err := statsService.RecordRequest(context.Background(), log); err != nil {
						// Log error but don't fail the request
						// In production, you might want to use a proper logger here
						global.Logger.Errorf("failed to record request statistics: %v", err)
					}
				}()
			}

			return reply, err
		}
	}
}
