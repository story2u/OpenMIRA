// Package outboundconnectorclient adapts an external HTTP outbound connector to the Go
// send-dispatcher execution boundary.
package outboundconnectorclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"im-go/internal/senddispatcher"
)

const defaultTimeout = 180 * time.Second

// Options controls optional HTTP behavior for the connector client.
type Options struct {
	HTTPClient *http.Client
	Token      string
	Timeout    time.Duration
}

// Client calls an HTTP-compatible outbound connector.
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
	Token      string
	Timeout    time.Duration
}

var _ senddispatcher.SDKExecutor = (*Client)(nil)
var _ senddispatcher.SDKBatchExecutor = (*Client)(nil)

// New builds a connector client. It does not ping the connector at startup.
func New(baseURL string, options Options) *Client {
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	httpClient := options.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: timeout}
	}
	return &Client{
		BaseURL:    strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		HTTPClient: httpClient,
		Token:      strings.TrimSpace(options.Token),
		Timeout:    timeout,
	}
}

// Execute calls POST /execute with {"task": ...}.
func (client *Client) Execute(ctx context.Context, task senddispatcher.SDKTaskPayload) (senddispatcher.SDKExecutorResult, error) {
	var raw any
	if err := client.doJSON(ctx, http.MethodPost, "/execute", map[string]any{"task": task}, &raw); err != nil {
		return nil, err
	}
	return decodeResult(raw)
}

// ExecuteBatch calls POST /execute-batch with {"tasks": [...]}.
func (client *Client) ExecuteBatch(ctx context.Context, tasks []senddispatcher.SDKTaskPayload) ([]senddispatcher.SDKExecutorResult, error) {
	var raw any
	if err := client.doJSON(ctx, http.MethodPost, "/execute-batch", map[string]any{"tasks": tasks}, &raw); err != nil {
		return nil, err
	}
	return decodeResults(raw)
}

// ListDeviceIDs calls GET /devices and extracts connector-visible device ids.
func (client *Client) ListDeviceIDs(ctx context.Context) ([]string, error) {
	var raw any
	if err := client.doJSON(ctx, http.MethodGet, "/devices", nil, &raw); err != nil {
		return nil, err
	}
	return decodeDeviceIDs(raw), nil
}

func (client *Client) doJSON(ctx context.Context, method string, path string, body any, output *any) error {
	if client == nil || client.HTTPClient == nil || strings.TrimSpace(client.BaseURL) == "" {
		return fmt.Errorf("send connector is not configured")
	}
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	request, err := http.NewRequestWithContext(ctx, method, client.BaseURL+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if client.Token != "" {
		request.Header.Set("Authorization", "Bearer "+client.Token)
	}
	response, err := client.HTTPClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("send connector returned %d: %s", response.StatusCode, strings.TrimSpace(string(data)))
	}
	if len(data) == 0 {
		*output = map[string]any{}
		return nil
	}
	if err := json.Unmarshal(data, output); err != nil {
		return err
	}
	return nil
}

func decodeResult(raw any) (senddispatcher.SDKExecutorResult, error) {
	if wrapped, ok := raw.(map[string]any); ok {
		if result, ok := wrapped["result"].(map[string]any); ok {
			return senddispatcher.SDKExecutorResult(result), nil
		}
		return senddispatcher.SDKExecutorResult(wrapped), nil
	}
	return nil, fmt.Errorf("send connector returned invalid result")
}

func decodeResults(raw any) ([]senddispatcher.SDKExecutorResult, error) {
	if wrapped, ok := raw.(map[string]any); ok {
		if results, ok := wrapped["results"].([]any); ok {
			return decodeResultList(results), nil
		}
		if results, ok := wrapped["tasks"].([]any); ok {
			return decodeResultList(results), nil
		}
	}
	if results, ok := raw.([]any); ok {
		return decodeResultList(results), nil
	}
	return nil, fmt.Errorf("send connector returned invalid batch result")
}

func decodeResultList(items []any) []senddispatcher.SDKExecutorResult {
	results := make([]senddispatcher.SDKExecutorResult, 0, len(items))
	for _, item := range items {
		result, ok := item.(map[string]any)
		if !ok {
			result = map[string]any{"success": false, "error": "send connector returned invalid task result"}
		}
		results = append(results, senddispatcher.SDKExecutorResult(result))
	}
	return results
}

func decodeDeviceIDs(raw any) []string {
	if wrapped, ok := raw.(map[string]any); ok {
		if devices, ok := wrapped["devices"]; ok {
			return decodeDeviceIDs(devices)
		}
	}
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	seen := map[string]struct{}{}
	deviceIDs := make([]string, 0, len(items))
	for _, item := range items {
		deviceID := deviceIDFromItem(item)
		if deviceID == "" {
			continue
		}
		if _, ok := seen[deviceID]; ok {
			continue
		}
		seen[deviceID] = struct{}{}
		deviceIDs = append(deviceIDs, deviceID)
	}
	return deviceIDs
}

func deviceIDFromItem(item any) string {
	switch typed := item.(type) {
	case string:
		return strings.TrimSpace(typed)
	case map[string]any:
		for _, key := range []string{"device_id", "id", "name"} {
			if value, ok := typed[key]; ok {
				deviceID := strings.TrimSpace(fmt.Sprint(value))
				if deviceID != "" && deviceID != "<nil>" {
					return deviceID
				}
			}
		}
	}
	return ""
}
