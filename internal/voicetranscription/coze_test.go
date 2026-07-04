package voicetranscription

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPExecutorPostsWorkflowInputAndParsesText(t *testing.T) {
	var gotAuth string
	var gotPayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		gotAuth = request.Header.Get("Authorization")
		if err := json.NewDecoder(request.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"code":0,"execute_id":"exec-1","detail":{"logid":"log-1"},"data":"{\"output\":{\"text\":\"你好\"}}"}`))
	}))
	defer server.Close()

	result, err := (HTTPExecutor{
		BaseURL:    server.URL,
		WorkflowID: "workflow-1",
		APIToken:   "token-1",
	}).TranscribeVoice(context.Background(), ExecuteInput{InputURL: "https://media.example/audio.amr"})
	if err != nil {
		t.Fatalf("TranscribeVoice returned error: %v", err)
	}
	if gotAuth != "Bearer token-1" || gotPayload["workflow_id"] != "workflow-1" {
		t.Fatalf("request auth=%q payload=%#v", gotAuth, gotPayload)
	}
	parameters := gotPayload["parameters"].(map[string]any)
	if parameters["input"] != "https://media.example/audio.amr" {
		t.Fatalf("parameters = %#v", parameters)
	}
	if result.TranscriptText != "你好" || result.ExecuteID != "exec-1" || result.LogID != "log-1" || !strings.Contains(result.RawResponseJSON, `"code":0`) {
		t.Fatalf("result = %#v", result)
	}
}

func TestHTTPExecutorUsesTokenSourceWhenAPIKeyMissing(t *testing.T) {
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		gotAuth = request.Header.Get("Authorization")
		_, _ = writer.Write([]byte(`{"code":0,"data":{"output":"ok"}}`))
	}))
	defer server.Close()

	result, err := (HTTPExecutor{
		BaseURL:     server.URL,
		WorkflowID:  "workflow-1",
		TokenSource: staticTokenSource("jwt-access-token"),
	}).TranscribeVoice(context.Background(), ExecuteInput{InputURL: "https://media.example/audio.amr"})
	if err != nil {
		t.Fatalf("TranscribeVoice returned error: %v", err)
	}
	if gotAuth != "Bearer jwt-access-token" || result.TranscriptText != "ok" {
		t.Fatalf("auth=%q result=%#v", gotAuth, result)
	}
}

func TestParseCozeResponseExtractsNestedTextList(t *testing.T) {
	result, err := ParseCozeResponse([]byte(`{"code":0,"data":{"output":[{"text":"第一句"},{"content":"第二句"}]}}`), "input")
	if err != nil {
		t.Fatalf("ParseCozeResponse returned error: %v", err)
	}
	if result.TranscriptText != "第一句\n第二句" {
		t.Fatalf("transcript = %q", result.TranscriptText)
	}
}

func TestParseCozeResponseTreatsEmptyTranscriptFieldErrorAsSuccess(t *testing.T) {
	result, err := ParseCozeResponse([]byte(`{"code":6014,"msg":"failed to extract null field","debug_url":"https://coze.example/debug?execute_id=exec-empty","detail":{"logid":"log-empty"}}`), "https://media.example/audio.amr")
	if err != nil {
		t.Fatalf("ParseCozeResponse returned error: %v", err)
	}
	if result.TranscriptText != "" || result.ExecuteID != "exec-empty" || result.LogID != "log-empty" {
		t.Fatalf("result = %#v", result)
	}
}

func TestParseCozeResponseClassifiesRetryableAndTerminalErrors(t *testing.T) {
	_, err := ParseCozeResponse([]byte(`{"code":7001,"msg":"rate limit busy","execute_id":"exec-r","detail":{"logid":"log-r"}}`), "input")
	var retryable RetryableError
	if err == nil || !errors.As(err, &retryable) || retryable.ExecuteID != "exec-r" || retryable.LogID != "log-r" {
		t.Fatalf("retryable err = %#v", err)
	}

	_, err = ParseCozeResponse([]byte(`{"code":7002,"msg":"unauthorized token"}`), "input")
	var terminal TerminalError
	if err == nil || !errors.As(err, &terminal) {
		t.Fatalf("terminal err = %#v", err)
	}
}

func TestHTTPExecutorClassifiesRetryableHTTPStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Error(writer, "too many", http.StatusTooManyRequests)
	}))
	defer server.Close()

	_, err := (HTTPExecutor{BaseURL: server.URL, WorkflowID: "workflow-1", APIToken: "token-1"}).TranscribeVoice(context.Background(), ExecuteInput{InputURL: "input"})
	var retryable RetryableError
	if err == nil || !errors.As(err, &retryable) || !strings.Contains(retryable.RawResponseJSON, "too many") {
		t.Fatalf("err = %#v", err)
	}
}

func TestHTTPExecutorRequiresToken(t *testing.T) {
	_, err := (HTTPExecutor{BaseURL: "https://coze.example", WorkflowID: "workflow-1"}).TranscribeVoice(context.Background(), ExecuteInput{InputURL: "input"})
	var retryable RetryableError
	if err == nil || !errors.As(err, &retryable) {
		t.Fatalf("err = %#v", err)
	}
}

type staticTokenSource string

func (source staticTokenSource) AccessToken(ctx context.Context) (string, error) {
	return string(source), nil
}
