// Package golden compares external reference HTTP responses with Go candidates.
// It is a harness building block: callers provide running endpoint URLs and
// deterministic cases, while this package handles request replay, response
// normalization, and drift reporting.
package golden

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const (
	defaultMethod       = http.MethodGet
	defaultTimeout      = 5 * time.Second
	defaultMaxBodyBytes = 1 << 20
)

// Endpoint describes one side of a reference/Go response comparison.
type Endpoint struct {
	Name    string
	BaseURL string
	Headers map[string]string
}

// Case defines one deterministic HTTP request to replay against both sides.
type Case struct {
	Name            string
	Method          string
	Path            string
	Headers         map[string]string
	Body            []byte
	SkipBodyCompare bool
}

// Options tunes comparison behavior without weakening status/body assertions.
type Options struct {
	Timeout          time.Duration
	MaxBodyBytes     int64
	IgnoreJSONFields []string
}

// Response captures the normalized comparison surface for one endpoint.
type Response struct {
	Endpoint   string
	StatusCode int
	Body       string
}

// Result reports whether one case matched and, if not, why.
type Result struct {
	Case   string
	Match  bool
	Python Response
	Go     Response
	Diffs  []string
}

// Compare replays one case against reference and Go endpoints and compares output.
func Compare(ctx context.Context, client *http.Client, reference Endpoint, goTarget Endpoint, testCase Case, options Options) (Result, error) {
	if client == nil {
		client = http.DefaultClient
	}
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	referenceResponse, err := doRequest(ctx, client, reference, testCase, options)
	if err != nil {
		return Result{}, fmt.Errorf("request reference endpoint: %w", err)
	}
	goResponse, err := doRequest(ctx, client, goTarget, testCase, options)
	if err != nil {
		return Result{}, fmt.Errorf("request go endpoint: %w", err)
	}

	result := Result{
		Case:   testCase.Name,
		Python: referenceResponse,
		Go:     goResponse,
	}
	if referenceResponse.StatusCode != goResponse.StatusCode {
		result.Diffs = append(result.Diffs, fmt.Sprintf("status: reference=%d go=%d", referenceResponse.StatusCode, goResponse.StatusCode))
	}
	if !testCase.SkipBodyCompare && referenceResponse.Body != goResponse.Body {
		result.Diffs = append(result.Diffs, "body: normalized response differs")
	}
	result.Match = len(result.Diffs) == 0
	return result, nil
}

func doRequest(ctx context.Context, client *http.Client, endpoint Endpoint, testCase Case, options Options) (Response, error) {
	request, err := buildRequest(ctx, endpoint, testCase)
	if err != nil {
		return Response{}, err
	}
	response, err := client.Do(request)
	if err != nil {
		return Response{}, err
	}
	defer response.Body.Close()

	maxBytes := options.MaxBodyBytes
	if maxBytes <= 0 {
		maxBytes = defaultMaxBodyBytes
	}
	rawBody, err := io.ReadAll(io.LimitReader(response.Body, maxBytes+1))
	if err != nil {
		return Response{}, fmt.Errorf("read response body: %w", err)
	}
	if int64(len(rawBody)) > maxBytes {
		return Response{}, fmt.Errorf("response body exceeds %d bytes", maxBytes)
	}

	return Response{
		Endpoint:   endpoint.Name,
		StatusCode: response.StatusCode,
		Body:       normalizeBody(rawBody, options.IgnoreJSONFields),
	}, nil
}

func buildRequest(ctx context.Context, endpoint Endpoint, testCase Case) (*http.Request, error) {
	targetURL, err := joinURL(endpoint.BaseURL, testCase.Path)
	if err != nil {
		return nil, err
	}
	method := strings.TrimSpace(testCase.Method)
	if method == "" {
		method = defaultMethod
	}
	request, err := http.NewRequestWithContext(ctx, method, targetURL, bytes.NewReader(testCase.Body))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	for key, value := range endpoint.Headers {
		request.Header.Set(key, value)
	}
	for key, value := range testCase.Headers {
		request.Header.Set(key, value)
	}
	return request, nil
}

func joinURL(baseURL string, requestPath string) (string, error) {
	base, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return "", fmt.Errorf("parse base url %q: %w", baseURL, err)
	}
	if base.Scheme == "" || base.Host == "" {
		return "", fmt.Errorf("base url must include scheme and host: %q", baseURL)
	}
	relative, err := url.Parse(requestPath)
	if err != nil {
		return "", fmt.Errorf("parse request path %q: %w", requestPath, err)
	}
	return base.ResolveReference(relative).String(), nil
}

func normalizeBody(raw []byte, ignoredFields []string) string {
	trimmed := bytes.TrimSpace(raw)
	var decoded any
	if err := json.Unmarshal(trimmed, &decoded); err != nil {
		return string(trimmed)
	}
	for _, fieldPath := range ignoredFields {
		removeJSONPath(decoded, splitFieldPath(fieldPath))
	}
	normalized, err := json.Marshal(decoded)
	if err != nil {
		return string(trimmed)
	}
	return string(normalized)
}

func splitFieldPath(path string) []string {
	parts := strings.Split(path, ".")
	cleaned := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			cleaned = append(cleaned, part)
		}
	}
	return cleaned
}

func removeJSONPath(value any, path []string) {
	if len(path) == 0 {
		return
	}
	switch node := value.(type) {
	case map[string]any:
		if len(path) == 1 {
			delete(node, path[0])
			return
		}
		removeJSONPath(node[path[0]], path[1:])
	case []any:
		for _, item := range node {
			removeJSONPath(item, path)
		}
	}
}

// StableDiffLines returns deterministic, human-readable drift lines.
func StableDiffLines(result Result) []string {
	if result.Match {
		return nil
	}
	lines := make([]string, 0, len(result.Diffs)+1)
	lines = append(lines, fmt.Sprintf("%s: mismatch", result.Case))
	diffs := append([]string(nil), result.Diffs...)
	sort.Strings(diffs)
	lines = append(lines, diffs...)
	return lines
}
