package workbench

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPAITextGeneratorPostsOpenAICompatiblePayload(t *testing.T) {
	var requestPath string
	var authorization string
	var payload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPath = r.URL.Path
		authorization = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"  生成话术  "}}]}`))
	}))
	defer server.Close()

	content, err := (HTTPAITextGenerator{}).GenerateText(context.Background(), ScriptTextGenerationInput{
		BaseURL:      server.URL + "/v1/",
		APIKey:       "api-key",
		Model:        "deepseek-chat",
		Prompt:       "生成",
		SystemPrompt: "系统",
		Temperature:  0.2,
	})

	if err != nil {
		t.Fatalf("GenerateText returned error: %v", err)
	}
	if content != "生成话术" {
		t.Fatalf("content = %q", content)
	}
	if requestPath != "/v1/chat/completions" || authorization != "Bearer api-key" {
		t.Fatalf("unexpected request path/header: %s %s", requestPath, authorization)
	}
	if payload["model"] != "deepseek-chat" || payload["temperature"] != float64(0.2) {
		t.Fatalf("unexpected payload: %#v", payload)
	}
	messages := payload["messages"].([]any)
	if messages[0].(map[string]any)["content"] != "系统" || messages[1].(map[string]any)["content"] != "生成" {
		t.Fatalf("unexpected messages: %#v", messages)
	}
}

func TestHTTPAITextGeneratorExtractsListContentAndReportsErrors(t *testing.T) {
	if got := extractTextContent([]any{map[string]any{"text": "第一段"}, map[string]any{"content": "第二段"}}); got != "第一段\n第二段" {
		t.Fatalf("extractTextContent = %q", got)
	}
	if got := resolveChatCompletionsURL("https://generativelanguage.googleapis.com/v1beta"); !strings.HasSuffix(got, "/openai/chat/completions") {
		t.Fatalf("gemini url = %s", got)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "upstream failed", http.StatusBadGateway)
	}))
	defer server.Close()

	_, err := (HTTPAITextGenerator{}).GenerateText(context.Background(), ScriptTextGenerationInput{
		BaseURL: server.URL,
		APIKey:  "api-key",
		Model:   "deepseek-chat",
		Prompt:  "生成",
	})
	if err == nil || !strings.Contains(err.Error(), "upstream failed") {
		t.Fatalf("expected upstream error, got %v", err)
	}
}
