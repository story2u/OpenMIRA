package workbench

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// HTTPAITextGenerator calls an OpenAI-compatible chat completions endpoint.
type HTTPAITextGenerator struct {
	HTTPClient *http.Client
}

// GenerateText posts the prompt to /chat/completions and extracts the first text response.
func (generator HTTPAITextGenerator) GenerateText(ctx context.Context, input ScriptTextGenerationInput) (string, error) {
	endpoint := resolveChatCompletionsURL(input.BaseURL)
	payload, err := json.Marshal(map[string]any{
		"model": strings.TrimSpace(input.Model),
		"messages": []map[string]string{
			{"role": "system", "content": defaultText(strings.TrimSpace(input.SystemPrompt), "你是企微客服助手，请使用专业、友好、清晰的中文回复客户。先准确理解问题，再给出可执行建议；不夸大、不承诺无法保证的结果，必要时引导转人工跟进。")},
			{"role": "user", "content": input.Prompt},
		},
		"temperature": input.Temperature,
	})
	if err != nil {
		return "", err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	request.Header.Set("Authorization", "Bearer "+strings.TrimSpace(input.APIKey))
	request.Header.Set("Content-Type", "application/json")
	response, err := generator.httpClient(input).Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return "", err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		detail := strings.TrimSpace(string(body))
		if detail == "" {
			detail = fmt.Sprintf("http status=%d", response.StatusCode)
		}
		return "", errors.New(detail)
	}
	var decoded struct {
		Choices []struct {
			Message struct {
				Content any `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return "", err
	}
	if len(decoded.Choices) == 0 {
		return "", fmt.Errorf("AI response missing choices")
	}
	content := extractTextContent(decoded.Choices[0].Message.Content)
	if content == "" {
		return "", fmt.Errorf("AI response content is empty")
	}
	return content, nil
}

func (generator HTTPAITextGenerator) httpClient(input ScriptTextGenerationInput) *http.Client {
	if generator.HTTPClient != nil {
		return generator.HTTPClient
	}
	timeout := input.Timeout
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	return &http.Client{Timeout: timeout}
}

func resolveChatCompletionsURL(baseURL string) string {
	cleaned := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if cleaned == "" {
		cleaned = "https://api.deepseek.com/v1"
	}
	if strings.Contains(strings.ToLower(cleaned), "generativelanguage.googleapis.com") && !strings.HasSuffix(cleaned, "/openai") {
		cleaned += "/openai"
	}
	return cleaned + "/chat/completions"
}

func extractTextContent(raw any) string {
	switch value := raw.(type) {
	case string:
		return strings.TrimSpace(value)
	case []any:
		chunks := make([]string, 0, len(value))
		for _, item := range value {
			switch typed := item.(type) {
			case map[string]any:
				text := firstNonBlank(stringFromAny(typed["text"]), stringFromAny(typed["content"]))
				if text != "" {
					chunks = append(chunks, text)
				}
			default:
				text := strings.TrimSpace(fmt.Sprint(typed))
				if text != "" && text != "<nil>" {
					chunks = append(chunks, text)
				}
			}
		}
		return strings.TrimSpace(strings.Join(chunks, "\n"))
	case nil:
		return ""
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}
