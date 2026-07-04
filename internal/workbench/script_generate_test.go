package workbench

import (
	"context"
	"errors"
	"strings"
	"testing"

	"wework-go/internal/auth"
)

func TestServiceGenerateScriptBuildsLegacyPrompt(t *testing.T) {
	generator := &recordingScriptGenerator{content: "  您好，已为您整理好处理建议。  "}
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

	payload, err := service.GenerateScript(context.Background(), NewScriptGenerateRequest(ScriptGenerateBody{
		Prompt:       " 客户想了解预约流程 ",
		Style:        " 简洁可靠 ",
		SystemPrompt: " 自定义系统提示 ",
	}, auth.Session{Role: "cs", AssigneeID: "cs-001"}))

	if err != nil {
		t.Fatalf("GenerateScript returned error: %v", err)
	}
	if payload["success"] != true || payload["content"] != "您好，已为您整理好处理建议。" {
		t.Fatalf("payload = %#v", payload)
	}
	if generator.input.BaseURL != "https://ai.example/v1" || generator.input.APIKey != "db-key" || generator.input.Model != "deepseek-chat" {
		t.Fatalf("unexpected generator input: %+v", generator.input)
	}
	if generator.input.SystemPrompt != "自定义系统提示" || generator.input.Temperature != 0.3 {
		t.Fatalf("unexpected prompt config: %+v", generator.input)
	}
	if !strings.Contains(generator.input.Prompt, "风格要求：简洁可靠。") || !strings.Contains(generator.input.Prompt, "用户需求：客户想了解预约流程") {
		t.Fatalf("unexpected generation prompt: %s", generator.input.Prompt)
	}
}

func TestServiceGenerateScriptValidatesInputsAndConfig(t *testing.T) {
	service := Service{AIConfigStore: fakeAIConfigStore{}, ScriptTextGenerator: &recordingScriptGenerator{}}
	if _, err := service.GenerateScript(context.Background(), NewScriptGenerateRequest(ScriptGenerateBody{Prompt: " "}, auth.Session{})); !errors.Is(err, ErrScriptPromptRequired) {
		t.Fatalf("blank prompt error = %v", err)
	}

	if _, err := service.GenerateScript(context.Background(), NewScriptGenerateRequest(ScriptGenerateBody{Prompt: "hello"}, auth.Session{})); !errors.Is(err, ErrScriptAIAPIKeyMissing) {
		t.Fatalf("missing api key error = %v", err)
	}

	service = Service{AIConfigStore: fakeAIConfigStore{values: map[string]string{"ai.api_key": "key"}}}
	if _, err := service.GenerateScript(context.Background(), NewScriptGenerateRequest(ScriptGenerateBody{Prompt: "hello"}, auth.Session{})); !errors.Is(err, ErrScriptTextGeneratorUnavailable) {
		t.Fatalf("missing generator error = %v", err)
	}
}

type recordingScriptGenerator struct {
	input   ScriptTextGenerationInput
	content string
	err     error
}

func (generator *recordingScriptGenerator) GenerateText(ctx context.Context, input ScriptTextGenerationInput) (string, error) {
	generator.input = input
	return generator.content, generator.err
}
