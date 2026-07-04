package archivepull

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const defaultTimeout = 20 * time.Second

// Client calls the self-decrypt archive bridge.
type Client struct {
	PullURL    string
	PullToken  string
	Timeout    time.Duration
	HTTPClient *http.Client
}

// PullInput contains one self-decrypt pull request.
type PullInput struct {
	Source       string
	Cursor       *string
	Limit        int
	EnterpriseID string
	Mode         string
	PullURL      string
	PullToken    string
}

// PullSelfDecrypt calls the configured self-decrypt bridge and normalizes the response.
func (client Client) PullSelfDecrypt(ctx context.Context, input PullInput) (Result, error) {
	pullURL := strings.TrimSpace(input.PullURL)
	if pullURL == "" {
		pullURL = strings.TrimSpace(client.PullURL)
	}
	if pullURL == "" {
		return Result{}, fmt.Errorf("ARCHIVE_SELF_DECRYPT_PULL_URL is not configured")
	}
	payload := BuildPullPayload(input.Source, input.Cursor, input.Limit, input.EnterpriseID, input.Mode)
	body, err := json.Marshal(payload)
	if err != nil {
		return Result{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, pullURL, bytes.NewReader(body))
	if err != nil {
		return Result{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	pullToken := strings.TrimSpace(input.PullToken)
	if pullToken == "" {
		pullToken = strings.TrimSpace(client.PullToken)
	}
	for key, value := range BuildAuthHeaders(pullToken) {
		request.Header.Set(key, value)
	}
	response, err := client.httpClient().Do(request)
	if err != nil {
		return Result{}, err
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return Result{}, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return Result{}, bridgeStatusError(response.StatusCode, responseBody)
	}
	var data map[string]any
	if err := json.Unmarshal(responseBody, &data); err != nil {
		return Result{}, err
	}
	return NormalizePullResponse(data, input.Source), nil
}

func (client Client) httpClient() *http.Client {
	if client.HTTPClient != nil {
		return client.HTTPClient
	}
	timeout := client.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	return &http.Client{Timeout: timeout}
}

func bridgeStatusError(statusCode int, body []byte) error {
	detail := ""
	var data map[string]any
	if err := json.Unmarshal(body, &data); err == nil {
		detail = firstText(data["detail"], data["message"])
	}
	if detail == "" {
		detail = strings.TrimSpace(string(body))
	}
	if detail != "" {
		return fmt.Errorf("archive bridge request failed: status=%d detail=%s", statusCode, detail)
	}
	return fmt.Errorf("archive bridge request failed: status=%d", statusCode)
}
