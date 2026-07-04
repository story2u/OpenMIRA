package archivemedia

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	DefaultBridgeTimeout       = 20 * time.Second
	DefaultDownloadTimeout     = 30 * time.Second
	DefaultMediaMaxChunkRounds = 256
)

// HTTPBridgePuller pulls archive media from the self-decrypt bridge.
type HTTPBridgePuller struct {
	PullURL         string
	PullToken       string
	Timeout         time.Duration
	DownloadTimeout time.Duration
	MaxChunkRounds  int
	HTTPClient      *http.Client
}

// PullArchiveMedia pulls all chunks for one archive media task.
func (puller HTTPBridgePuller) PullArchiveMedia(ctx context.Context, input PullInput) (PullResult, error) {
	pullURL := strings.TrimSpace(puller.PullURL)
	if pullURL == "" {
		return PullResult{}, fmt.Errorf("ARCHIVE_SELF_DECRYPT_PULL_URL is not configured")
	}
	currentIndex := strings.TrimSpace(input.IndexBuf)
	chunks := bytes.Buffer{}
	var lastResponse map[string]any
	for round := 0; round < puller.maxChunkRounds(); round++ {
		response, err := puller.pullChunk(ctx, pullURL, input, currentIndex)
		if err != nil {
			return PullResult{}, err
		}
		lastResponse = response
		normalized, err := NormalizeMediaPullResponse(response)
		if err != nil {
			return PullResult{}, err
		}
		if normalized.DownloadURL != "" {
			content, err := puller.downloadURL(ctx, normalized.DownloadURL)
			if err != nil {
				return PullResult{}, err
			}
			return PullResult{Response: response, Content: content, NextIndex: normalized.NextIndex, IsFinish: true}, nil
		}
		if len(normalized.FullPayloadBytes) > 0 {
			return PullResult{Response: response, Content: normalized.FullPayloadBytes, NextIndex: normalized.NextIndex, IsFinish: true}, nil
		}
		if len(normalized.ChunkBytes) > 0 {
			_, _ = chunks.Write(normalized.ChunkBytes)
		}
		if normalized.IsFinish {
			return PullResult{Response: buildAssembledPayload(response, chunks.Len()), Content: chunks.Bytes(), NextIndex: normalized.NextIndex, IsFinish: true}, nil
		}
		if normalized.NextIndex == "" {
			return PullResult{}, fmt.Errorf("archive media chunk response missing outindexbuf before finish")
		}
		if normalized.NextIndex == currentIndex {
			return PullResult{}, fmt.Errorf("archive media chunk cursor did not advance")
		}
		if len(normalized.ChunkBytes) == 0 {
			return PullResult{}, fmt.Errorf("archive media chunk response is empty before finish")
		}
		currentIndex = normalized.NextIndex
	}
	return PullResult{Response: lastResponse}, fmt.Errorf("archive media chunk loop exceeded max rounds: %d", puller.maxChunkRounds())
}

func (puller HTTPBridgePuller) pullChunk(ctx context.Context, pullURL string, input PullInput, indexBuf string) (map[string]any, error) {
	payload := BuildMediaPayload(input.SDKFileID, indexBuf, input.EnterpriseID, input.Source)
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, pullURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	for key, value := range buildAuthHeaders(puller.PullToken) {
		request.Header.Set(key, value)
	}
	response, err := puller.httpClient(puller.Timeout, DefaultBridgeTimeout).Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, bridgeStatusError(response.StatusCode, responseBody)
	}
	var data map[string]any
	if err := json.Unmarshal(responseBody, &data); err != nil {
		return nil, err
	}
	return data, nil
}

func (puller HTTPBridgePuller) downloadURL(ctx context.Context, rawURL string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	response, err := puller.httpClient(puller.DownloadTimeout, DefaultDownloadTimeout).Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, bridgeStatusError(response.StatusCode, body)
	}
	return body, nil
}

func (puller HTTPBridgePuller) httpClient(timeout time.Duration, fallback time.Duration) *http.Client {
	if puller.HTTPClient != nil {
		return puller.HTTPClient
	}
	if timeout <= 0 {
		timeout = fallback
	}
	return &http.Client{Timeout: timeout}
}

func (puller HTTPBridgePuller) maxChunkRounds() int {
	if puller.MaxChunkRounds < 1 {
		return DefaultMediaMaxChunkRounds
	}
	return puller.MaxChunkRounds
}

// BuildMediaPayload mirrors Python build_media_payload.
func BuildMediaPayload(sdkFileID string, indexBuf string, enterpriseID string, source string) map[string]any {
	payload := map[string]any{
		"sdk_file_id": strings.TrimSpace(sdkFileID),
		"index_buf":   strings.TrimSpace(indexBuf),
	}
	if enterpriseID = strings.TrimSpace(enterpriseID); enterpriseID != "" {
		payload["enterprise_id"] = enterpriseID
	}
	if source = strings.TrimSpace(source); source != "" {
		payload["source"] = source
	}
	return payload
}

// NormalizedMediaPullResponse describes one bridge media response.
type NormalizedMediaPullResponse struct {
	NextIndex        string
	IsFinish         bool
	DownloadURL      string
	FullPayloadBytes []byte
	ChunkBytes       []byte
}

// NormalizeMediaPullResponse classifies one-shot payloads and chunk payloads.
func NormalizeMediaPullResponse(response map[string]any) (NormalizedMediaPullResponse, error) {
	if response == nil {
		response = map[string]any{}
	}
	downloadURL := firstNonBlank(textAny(response["download_url"]), textAny(response["file_url"]), textAny(response["url"]))
	nextIndex := firstNonBlank(textAny(response["outindexbuf"]), textAny(response["out_index_buf"]))
	fullBase64 := firstNonBlank(textAny(response["file_base64"]), textAny(response["media_base64"]))
	chunkBase64 := firstNonBlank(textAny(response["data_base64"]), textAny(response["data"]), textAny(response["chunk"]), textAny(response["chunk_base64"]))
	fullPayload, err := decodeBase64Payload(fullBase64, "file_base64/media_base64")
	if err != nil {
		return NormalizedMediaPullResponse{}, err
	}
	chunkPayload := []byte(nil)
	if len(fullPayload) == 0 && downloadURL == "" {
		chunkPayload, err = decodeBase64Payload(chunkBase64, "data_base64/chunk")
		if err != nil {
			return NormalizedMediaPullResponse{}, err
		}
	}
	return NormalizedMediaPullResponse{
		NextIndex:        nextIndex,
		IsFinish:         truthy(response["is_finish"]),
		DownloadURL:      downloadURL,
		FullPayloadBytes: fullPayload,
		ChunkBytes:       chunkPayload,
	}, nil
}

func decodeBase64Payload(value string, fieldName string) ([]byte, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("archive media %s is not valid base64: %w", fieldName, err)
	}
	return decoded, nil
}

func buildAssembledPayload(response map[string]any, assembledBytes int) map[string]any {
	output := map[string]any{}
	for key, value := range response {
		output[key] = value
	}
	output["assembled_bytes"] = assembledBytes
	output["mode"] = "chunk_stream"
	return output
}

func buildAuthHeaders(token string) map[string]string {
	token = strings.TrimSpace(token)
	if token == "" {
		return map[string]string{}
	}
	return map[string]string{"Authorization": "Bearer " + token}
}

func bridgeStatusError(statusCode int, body []byte) error {
	detail := ""
	var data map[string]any
	if err := json.Unmarshal(body, &data); err == nil {
		detail = firstNonBlank(textAny(data["detail"]), textAny(data["message"]))
	}
	if detail == "" {
		detail = strings.TrimSpace(string(body))
	}
	if detail != "" {
		return fmt.Errorf("archive bridge request failed: status=%d detail=%s", statusCode, detail)
	}
	return fmt.Errorf("archive bridge request failed: status=%d", statusCode)
}

func textAny(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case []byte:
		return string(typed)
	default:
		return fmt.Sprint(typed)
	}
}

func truthy(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case int:
		return typed != 0
	case int64:
		return typed != 0
	case float64:
		return typed != 0
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "", "0", "false", "no", "off":
			return false
		default:
			return true
		}
	default:
		return false
	}
}
