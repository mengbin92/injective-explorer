// Package service provides user service client for external user detail API.
package service

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"injective-explorer/internal/conf"
	"injective-explorer/internal/global"

	"github.com/bytedance/sonic"
	"github.com/pkg/errors"
)

// UserDetail represents user detail response from external service.
type UserDetail struct {
	ID            int64  `json:"id"`
	Account       string `json:"account"`
	Source        string `json:"source"`
	AlternativeID int64  `json:"alternative_id"`
	Action        string `json:"action"`
	Status        int    `json:"status"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

// IsActive returns true if user status is active (status=1).
func (u *UserDetail) IsActive() bool {
	return u.Status == 1
}

// UserServiceClient provides client for external user service API.
type UserServiceClient struct {
	baseURL    string
	apiKey     string
	timeout    time.Duration
	maxRetries int
	httpClient *http.Client
}

// NewUserServiceClient creates a new user service client.
func NewUserServiceClient(cfg *conf.APIManagement_UserService) *UserServiceClient {
	timeout := cfg.Timeout.AsDuration()
	if timeout == 0 {
		timeout = 5 * time.Second
	}

	maxRetries := int(cfg.MaxRetries)
	if maxRetries == 0 {
		maxRetries = 3
	}

	return &UserServiceClient{
		baseURL:    fmt.Sprintf("%s/%s", cfg.BaseUrl, cfg.ApiVersion),
		apiKey:     cfg.ApiKey,
		timeout:    timeout,
		maxRetries: maxRetries,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// GetUserDetail fetches user detail from external service.
// It uses the configured apiKey as Bearer token for authentication.
func (c *UserServiceClient) GetUserDetail(ctx context.Context, userID string) (*UserDetail, error) {
	url := fmt.Sprintf("%s/client/user/detail?id=%s", c.baseURL, userID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, errors.Wrap(err, "create request failed")
	}

	// Use configured apiKey as Bearer token for user service authentication
	if c.apiKey != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiKey))
	}
	req.Header.Set("Accept", "application/json")

	var lastErr error
	for i := 0; i < c.maxRetries; i++ {
		if i > 0 {
			global.Logger.Debugf("retrying get user detail, attempt %d/%d", i+1, c.maxRetries)
			time.Sleep(time.Duration(i) * time.Second)
		}

		detail, err := c.doRequest(req)
		if err == nil {
			return detail, nil
		}
		lastErr = err

		// Don't retry on client errors (4xx)
		if errors.Is(err, ErrUserNotFound) || errors.Is(err, ErrUnauthorized) {
			break
		}
	}

	return nil, lastErr
}

var (
	ErrUserNotFound = errors.New("user not found")
	ErrUnauthorized = errors.New("unauthorized")
	ErrInvalidResponse = errors.New("invalid response")
)

func (c *UserServiceClient) doRequest(req *http.Request) (*UserDetail, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "request failed")
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		// continue to parse response
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, ErrUnauthorized
	case http.StatusNotFound:
		return nil, ErrUserNotFound
	default:
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var result struct {
		Code    int        `json:"code"`
		Data    UserDetail `json:"data"`
		Message string     `json:"message"`
	}

	if err := sonic.ConfigDefault.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, errors.Wrap(err, "decode response failed")
	}

	if result.Code != 200 {
		return nil, fmt.Errorf("api error: %s (code: %d)", result.Message, result.Code)
	}

	return &result.Data, nil
}
