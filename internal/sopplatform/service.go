// Package sopplatform provides SOP platform connectivity checks.
package sopplatform

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	DefaultTimeout   = 5 * time.Second
	DefaultUserAgent = "SOP-Platform-Tester/1.0"
)

var ErrTaskURLRequired = errors.New("task_url is required")

// HTTPDoer is the minimal http.Client boundary needed by Service.
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// Service validates that an SOP platform task URL is reachable by HEAD.
type Service struct {
	Client    HTTPDoer
	Timeout   time.Duration
	UserAgent string
}

// Request carries the admin-provided platform URL.
type Request struct {
	TaskURL string `json:"task_url"`
}

// Result mirrors the legacy response shape.
type Result struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// TestConnection performs a HEAD request without pulling business data.
func (service Service) TestConnection(ctx context.Context, request Request) (Result, error) {
	taskURL := strings.TrimSpace(request.TaskURL)
	if taskURL == "" {
		return Result{}, ErrTaskURLRequired
	}
	timeout := service.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	headRequest, err := http.NewRequestWithContext(requestCtx, http.MethodHead, taskURL, nil)
	if err != nil {
		return Result{Success: false, Message: "连接错误: " + trimError(err)}, nil
	}
	headRequest.Header.Set("User-Agent", defaultText(service.UserAgent, DefaultUserAgent))
	response, err := service.httpClient().Do(headRequest)
	if err != nil {
		return Result{Success: false, Message: "连接错误: " + trimError(err)}, nil
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusBadRequest {
		return Result{Success: true, Message: fmt.Sprintf("连接成功 (HTTP %d)", response.StatusCode)}, nil
	}
	return Result{Success: false, Message: fmt.Sprintf("HTTP 错误: %d", response.StatusCode)}, nil
}

func (service Service) httpClient() HTTPDoer {
	if service.Client != nil {
		return service.Client
	}
	return &http.Client{Timeout: defaultDuration(service.Timeout, DefaultTimeout)}
}

func trimError(err error) string {
	message := strings.TrimSpace(err.Error())
	if len(message) > 100 {
		return message[:100]
	}
	return message
}

func defaultText(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func defaultDuration(value time.Duration, fallback time.Duration) time.Duration {
	if value <= 0 {
		return fallback
	}
	return value
}
