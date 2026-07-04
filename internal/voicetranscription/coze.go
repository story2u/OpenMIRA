package voicetranscription

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var retryableHTTPStatuses = map[int]struct{}{
	http.StatusRequestTimeout:      {},
	http.StatusConflict:            {},
	http.StatusTooEarly:            {},
	http.StatusTooManyRequests:     {},
	http.StatusInternalServerError: {},
	http.StatusBadGateway:          {},
	http.StatusServiceUnavailable:  {},
	http.StatusGatewayTimeout:      {},
}

var retryableHints = []string{"rate", "limit", "busy", "timeout", "temporarily", "unavailable", "too many"}
var terminalHints = []string{"auth", "token", "unauthorized", "forbidden", "workflow", "invalid", "expired", "expire"}

// HTTPExecutor calls the Coze workflow used by the legacy Python worker.
type HTTPExecutor struct {
	BaseURL     string
	WorkflowID  string
	APIToken    string
	TokenSource TokenSource
	Timeout     time.Duration
	HTTPClient  *http.Client
}

// TokenSource resolves a bearer token for Coze requests.
type TokenSource interface {
	AccessToken(ctx context.Context) (string, error)
}

// TranscribeVoice posts the signed media URL to Coze and normalizes the response.
func (executor HTTPExecutor) TranscribeVoice(ctx context.Context, input ExecuteInput) (ExecuteResult, error) {
	baseURL := defaultText(executor.BaseURL, DefaultVoiceTranscriptionBaseURL)
	workflowID := defaultText(executor.WorkflowID, DefaultVoiceTranscriptionFlowID)
	if baseURL == "" {
		return ExecuteResult{}, RetryableError{Message: "VOICE_TRANSCRIPTION_COZE_BASE_URL is not configured"}
	}
	if workflowID == "" {
		return ExecuteResult{}, RetryableError{Message: "VOICE_TRANSCRIPTION_WORKFLOW_ID is not configured"}
	}
	apiToken, err := executor.accessToken(ctx)
	if err != nil {
		return ExecuteResult{}, err
	}
	if apiToken == "" {
		return ExecuteResult{}, RetryableError{Message: "VOICE_TRANSCRIPTION_COZE_API_KEY or JWT OAuth credentials are not configured"}
	}
	body, err := json.Marshal(map[string]any{
		"workflow_id": workflowID,
		"parameters":  map[string]any{"input": strings.TrimSpace(input.InputURL)},
	})
	if err != nil {
		return ExecuteResult{}, TerminalError{Message: err.Error()}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL, bytes.NewReader(body))
	if err != nil {
		return ExecuteResult{}, TerminalError{Message: err.Error()}
	}
	request.Header.Set("Authorization", "Bearer "+apiToken)
	request.Header.Set("Content-Type", "application/json; charset=utf-8")
	response, err := executor.httpClient().Do(request)
	if err != nil {
		return ExecuteResult{}, RetryableError{Message: "coze request failed: " + err.Error()}
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return ExecuteResult{}, RetryableError{Message: "coze response read failed: " + err.Error()}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message := strings.TrimSpace(string(responseBody))
		if message == "" {
			message = fmt.Sprintf("http status=%d", response.StatusCode)
		}
		if _, ok := retryableHTTPStatuses[response.StatusCode]; ok {
			return ExecuteResult{}, RetryableError{Message: message, RawResponseJSON: message}
		}
		return ExecuteResult{}, TerminalError{Message: message, RawResponseJSON: message}
	}
	result, err := ParseCozeResponse(responseBody, input.InputURL)
	if err != nil {
		return ExecuteResult{}, err
	}
	return result, nil
}

func (executor HTTPExecutor) accessToken(ctx context.Context) (string, error) {
	if apiToken := strings.TrimSpace(executor.APIToken); apiToken != "" {
		return apiToken, nil
	}
	if executor.TokenSource == nil {
		return "", nil
	}
	return executor.TokenSource.AccessToken(ctx)
}

func (executor HTTPExecutor) httpClient() *http.Client {
	if executor.HTTPClient != nil {
		return executor.HTTPClient
	}
	timeout := executor.Timeout
	if timeout <= 0 {
		timeout = 90 * time.Second
	}
	return &http.Client{Timeout: timeout}
}

// ParseCozeResponse normalizes the workflow response body.
func ParseCozeResponse(body []byte, inputURL string) (ExecuteResult, error) {
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return ExecuteResult{}, TerminalError{
			Message:         "coze response json decode failed: " + err.Error(),
			RawResponseJSON: strings.TrimSpace(string(body)),
		}
	}
	rawJSON := compactJSON(raw)
	code := intNumber(raw["code"])
	if code != 0 {
		executeID := extractCozeExecuteID(raw)
		logID := textValue(mapValue(raw["detail"])["logid"])
		message := extractCozeErrorMessage(raw, fmt.Sprintf("Coze returned code=%d", code))
		if isEmptyTranscriptResponse(raw, inputURL) {
			return ExecuteResult{
				TranscriptText:  "",
				ExecuteID:       executeID,
				LogID:           logID,
				RawResponseJSON: rawJSON,
			}, nil
		}
		if looksRetryableError(message) {
			return ExecuteResult{}, RetryableError{
				Message:         message,
				ExecuteID:       executeID,
				LogID:           logID,
				RawResponseJSON: rawJSON,
			}
		}
		return ExecuteResult{}, TerminalError{
			Message:         message,
			ExecuteID:       executeID,
			LogID:           logID,
			RawResponseJSON: rawJSON,
		}
	}
	data, err := parseDataField(raw["data"])
	if err != nil {
		return ExecuteResult{}, err
	}
	return ExecuteResult{
		TranscriptText:  extractTranscriptText(data),
		ExecuteID:       firstNonBlank(textValue(raw["execute_id"]), textValue(data["execute_id"])),
		LogID:           firstNonBlank(textValue(mapValue(raw["detail"])["logid"]), textValue(data["logid"])),
		RawResponseJSON: rawJSON,
	}, nil
}

func parseDataField(value any) (map[string]any, error) {
	if data := mapValue(value); len(data) > 0 {
		return data, nil
	}
	text := strings.TrimSpace(textValue(value))
	if text == "" {
		return map[string]any{}, nil
	}
	var data map[string]any
	if err := json.Unmarshal([]byte(text), &data); err != nil {
		return nil, TerminalError{Message: "coze data json decode failed: " + err.Error()}
	}
	return data, nil
}

func extractTranscriptText(data map[string]any) string {
	output, ok := data["output"]
	if !ok {
		output = data
	}
	return extractTextValue(output)
}

func extractTextValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return ""
		}
		if strings.HasPrefix(text, "{") || strings.HasPrefix(text, "[") {
			var parsed any
			if err := json.Unmarshal([]byte(text), &parsed); err == nil {
				if nested := extractTextValue(parsed); nested != "" {
					return nested
				}
			}
		}
		return text
	case map[string]any:
		for _, key := range []string{"text", "transcript_text", "transcript", "content", "result", "answer", "output"} {
			if nested, ok := typed[key]; ok {
				if text := extractTextValue(nested); text != "" {
					return text
				}
			}
		}
		return ""
	case []any:
		parts := []string{}
		for _, item := range typed {
			if text := extractTextValue(item); text != "" {
				parts = append(parts, text)
			}
		}
		return strings.TrimSpace(strings.Join(parts, "\n"))
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func extractCozeErrorMessage(raw map[string]any, fallback string) string {
	message := defaultText(textValue(raw["msg"]), fallback)
	executeID := textValue(raw["execute_id"])
	logID := textValue(mapValue(raw["detail"])["logid"])
	extras := []string{}
	if strings.TrimSpace(executeID) != "" {
		extras = append(extras, "execute_id="+strings.TrimSpace(executeID))
	}
	if strings.TrimSpace(logID) != "" {
		extras = append(extras, "logid="+strings.TrimSpace(logID))
	}
	if len(extras) == 0 {
		return message
	}
	return message + " (" + strings.Join(extras, ", ") + ")"
}

func extractCozeExecuteID(raw map[string]any) string {
	if executeID := strings.TrimSpace(textValue(raw["execute_id"])); executeID != "" {
		return executeID
	}
	debugURL := strings.TrimSpace(textValue(raw["debug_url"]))
	if debugURL == "" {
		return ""
	}
	parsed, err := url.Parse(debugURL)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(parsed.Query().Get("execute_id"))
}

func isEmptyTranscriptResponse(raw map[string]any, inputURL string) bool {
	if strings.TrimSpace(inputURL) == "" || intNumber(raw["code"]) != 6014 {
		return false
	}
	message := strings.ToLower(strings.TrimSpace(textValue(raw["msg"])))
	return strings.Contains(message, "null") && strings.Contains(message, "field") && strings.Contains(message, "extract")
}

func looksRetryableError(message string) bool {
	lowered := strings.ToLower(strings.TrimSpace(message))
	if lowered == "" {
		return false
	}
	for _, hint := range terminalHints {
		if strings.Contains(lowered, hint) {
			return false
		}
	}
	for _, hint := range retryableHints {
		if strings.Contains(lowered, hint) {
			return true
		}
	}
	return false
}

func compactJSON(value map[string]any) string {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return ""
	}
	return strings.TrimSpace(buffer.String())
}

func mapValue(value any) map[string]any {
	if typed, ok := value.(map[string]any); ok {
		return typed
	}
	return map[string]any{}
}

func textValue(value any) string {
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

func intNumber(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case json.Number:
		parsed, _ := typed.Int64()
		return int(parsed)
	case string:
		var parsed int
		_, _ = fmt.Sscanf(strings.TrimSpace(typed), "%d", &parsed)
		return parsed
	default:
		return 0
	}
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if text := strings.TrimSpace(value); text != "" {
			return text
		}
	}
	return ""
}
