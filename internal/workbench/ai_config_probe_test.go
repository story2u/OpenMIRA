package workbench

import (
	"context"
	"errors"
	"testing"

	"wework-go/internal/auth"
)

func TestServiceTestAIConfigUsesSavedConfigWithRequestOverrides(t *testing.T) {
	timeoutSec := 6.0
	temperature := 0.2
	generator := &recordingScriptGenerator{content: "  连接正常  "}
	service := Service{
		AIConfigStore: fakeAIConfigStore{values: map[string]string{
			"ai.base_url":      "https://ai.example/v1",
			"ai.model":         "deepseek-chat",
			"ai.timeout_sec":   "9",
			"ai.temperature":   "0.3",
			"ai.api_key":       "db-key",
			"ai.system_prompt": "默认系统提示",
		}},
		ScriptTextGenerator: generator,
	}

	payload, err := service.TestAIConfig(context.Background(), NewAIConfigTestRequest(AIConfigTestBody{
		Prompt:       " 请回复 pong ",
		BaseURL:      " https://probe.example/v1 ",
		APIKey:       " request-key ",
		Model:        " probe-model ",
		TimeoutSec:   &timeoutSec,
		Temperature:  &temperature,
		SystemPrompt: " 探测系统提示 ",
	}, auth.Session{Role: "admin", AssigneeID: "admin-001"}))

	if err != nil {
		t.Fatalf("TestAIConfig returned error: %v", err)
	}
	if payload["success"] != true || payload["reply"] != "连接正常" || payload["content"] != "连接正常" {
		t.Fatalf("payload = %#v", payload)
	}
	if generator.input.Prompt != "请回复 pong" || generator.input.BaseURL != "https://probe.example/v1" || generator.input.APIKey != "request-key" {
		t.Fatalf("unexpected generator input: %+v", generator.input)
	}
	if generator.input.Model != "probe-model" || generator.input.SystemPrompt != "探测系统提示" || generator.input.Temperature != 0.2 {
		t.Fatalf("unexpected probe config: %+v", generator.input)
	}
	if generator.input.Timeout.Seconds() != 6 {
		t.Fatalf("timeout = %s, want 6s", generator.input.Timeout)
	}
}

func TestServiceTestAIConfigValidatesInputsAndConfig(t *testing.T) {
	service := Service{AIConfigStore: fakeAIConfigStore{}, ScriptTextGenerator: &recordingScriptGenerator{}}
	if _, err := service.TestAIConfig(context.Background(), NewAIConfigTestRequest(AIConfigTestBody{Prompt: " "}, auth.Session{})); !errors.Is(err, ErrScriptPromptRequired) {
		t.Fatalf("blank prompt error = %v", err)
	}

	if _, err := service.TestAIConfig(context.Background(), NewAIConfigTestRequest(AIConfigTestBody{Prompt: "hello"}, auth.Session{})); !errors.Is(err, ErrScriptAIAPIKeyMissing) {
		t.Fatalf("missing api key error = %v", err)
	}

	timeoutSec := 0.0
	if _, err := service.TestAIConfig(context.Background(), NewAIConfigTestRequest(AIConfigTestBody{Prompt: "hello", APIKey: "key", TimeoutSec: &timeoutSec}, auth.Session{})); !errors.Is(err, ErrAIConfigTimeoutInvalid) {
		t.Fatalf("invalid timeout error = %v", err)
	}

	temperature := 2.1
	if _, err := service.TestAIConfig(context.Background(), NewAIConfigTestRequest(AIConfigTestBody{Prompt: "hello", APIKey: "key", Temperature: &temperature}, auth.Session{})); !errors.Is(err, ErrAIConfigTemperature) {
		t.Fatalf("invalid temperature error = %v", err)
	}

	service = Service{AIConfigStore: fakeAIConfigStore{values: map[string]string{"ai.api_key": "key"}}}
	if _, err := service.TestAIConfig(context.Background(), NewAIConfigTestRequest(AIConfigTestBody{Prompt: "hello"}, auth.Session{})); !errors.Is(err, ErrScriptTextGeneratorUnavailable) {
		t.Fatalf("missing generator error = %v", err)
	}
}

func TestServiceTestAIConfigWrapsProviderErrors(t *testing.T) {
	generator := &recordingScriptGenerator{err: errors.New("provider timeout")}
	service := Service{
		AIConfigStore:       fakeAIConfigStore{values: map[string]string{"ai.api_key": "key"}},
		ScriptTextGenerator: generator,
	}

	_, err := service.TestAIConfig(context.Background(), NewAIConfigTestRequest(AIConfigTestBody{Prompt: "hello"}, auth.Session{}))

	var generationErr ScriptAIGenerationError
	if !errors.As(err, &generationErr) || generationErr.Error() != "AI 生成失败: provider timeout" {
		t.Fatalf("error = %v", err)
	}
}
