package workbench

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"im-go/internal/auth"
)

var (
	ErrScriptPromptRequired           = errors.New("prompt is required")
	ErrScriptAIAPIKeyMissing          = errors.New("AI_API_KEY 未配置，无法生成话术")
	ErrScriptTextGeneratorUnavailable = errors.New("workbench script generation service is unavailable")
)

const defaultScriptStyle = "专业亲和"

// ScriptGenerateBody is the legacy JSON body for POST /api/v1/scripts/generate.
type ScriptGenerateBody struct {
	Prompt       string `json:"prompt"`
	SystemPrompt string `json:"system_prompt"`
	Style        string `json:"style"`
}

// ScriptGenerateRequest carries the authenticated quick-reply generation request.
type ScriptGenerateRequest struct {
	Session auth.Session
	Body    ScriptGenerateBody
}

// ScriptTextGenerationInput is passed to an OpenAI-compatible text generator.
type ScriptTextGenerationInput struct {
	Prompt       string
	BaseURL      string
	APIKey       string
	Model        string
	Timeout      time.Duration
	Temperature  float64
	SystemPrompt string
}

// ScriptTextGenerator is the external AI boundary used by quick-reply generation.
type ScriptTextGenerator interface {
	GenerateText(ctx context.Context, input ScriptTextGenerationInput) (string, error)
}

// ScriptAIGenerationError preserves Python's 502 detail prefix.
type ScriptAIGenerationError struct {
	Err error
}

func (err ScriptAIGenerationError) Error() string {
	if err.Err == nil {
		return "AI 生成失败"
	}
	return "AI 生成失败: " + err.Err.Error()
}

func (err ScriptAIGenerationError) Unwrap() error {
	return err.Err
}

// NewScriptGenerateRequest normalizes the HTTP boundary for script generation.
func NewScriptGenerateRequest(body ScriptGenerateBody, session auth.Session) ScriptGenerateRequest {
	body.Prompt = strings.TrimSpace(body.Prompt)
	body.SystemPrompt = strings.TrimSpace(body.SystemPrompt)
	body.Style = strings.TrimSpace(body.Style)
	return ScriptGenerateRequest{Session: session, Body: body}
}

// GenerateScript builds and sends the legacy AI quick-reply generation prompt.
func (service Service) GenerateScript(ctx context.Context, request ScriptGenerateRequest) (Payload, error) {
	if strings.TrimSpace(request.Body.Prompt) == "" {
		return nil, ErrScriptPromptRequired
	}
	if service.AIConfigStore == nil {
		return nil, ErrAIConfigStoreUnavailable
	}
	if service.ScriptTextGenerator == nil {
		return nil, ErrScriptTextGeneratorUnavailable
	}
	reader := aiConfigReader{ctx: ctx, store: service.AIConfigStore}
	cfg, err := reader.scriptGenerationConfig()
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, ErrScriptAIAPIKeyMissing
	}
	style := strings.TrimSpace(request.Body.Style)
	if style == "" {
		style = defaultScriptStyle
	}
	systemPrompt := strings.TrimSpace(request.Body.SystemPrompt)
	if systemPrompt == "" {
		systemPrompt = cfg.SystemPrompt
	}
	content, err := service.ScriptTextGenerator.GenerateText(ctx, ScriptTextGenerationInput{
		Prompt:       buildScriptGenerationPrompt(request.Body.Prompt, style),
		BaseURL:      cfg.BaseURL,
		APIKey:       cfg.APIKey,
		Model:        cfg.Model,
		Timeout:      cfg.Timeout,
		Temperature:  cfg.Temperature,
		SystemPrompt: systemPrompt,
	})
	if err != nil {
		return nil, ScriptAIGenerationError{Err: err}
	}
	return Payload{"success": true, "content": strings.TrimSpace(content)}, nil
}

type scriptGenerationConfig struct {
	BaseURL      string
	APIKey       string
	Model        string
	Timeout      time.Duration
	Temperature  float64
	SystemPrompt string
}

func (reader aiConfigReader) scriptGenerationConfig() (scriptGenerationConfig, error) {
	baseURL, err := reader.setting("ai.base_url", envString("AI_BASE_URL", "https://api.deepseek.com/v1"))
	if err != nil {
		return scriptGenerationConfig{}, err
	}
	model, err := reader.setting("ai.model", envString("AI_MODEL", "deepseek-chat"))
	if err != nil {
		return scriptGenerationConfig{}, err
	}
	timeoutSec, err := reader.floatSetting("ai.timeout_sec", envString("AI_TIMEOUT_SEC", "20"), 20)
	if err != nil {
		return scriptGenerationConfig{}, err
	}
	temperature, err := reader.floatSetting("ai.temperature", "0.7", 0.7)
	if err != nil {
		return scriptGenerationConfig{}, err
	}
	systemPrompt, err := reader.setting("ai.system_prompt", "你是企微消息端助手，请使用专业、友好、清晰的中文回复客户。先准确理解问题，再给出可执行建议；不夸大、不承诺无法保证的结果，必要时引导转人工跟进。")
	if err != nil {
		return scriptGenerationConfig{}, err
	}
	apiKey, err := reader.setting("ai.api_key", "")
	if err != nil {
		return scriptGenerationConfig{}, err
	}
	if strings.TrimSpace(apiKey) == "" {
		apiKey = strings.TrimSpace(os.Getenv("AI_API_KEY"))
	}
	timeout := time.Duration(timeoutSec * float64(time.Second))
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	return scriptGenerationConfig{
		BaseURL:      baseURL,
		APIKey:       apiKey,
		Model:        model,
		Timeout:      timeout,
		Temperature:  temperature,
		SystemPrompt: systemPrompt,
	}, nil
}

func buildScriptGenerationPrompt(prompt string, style string) string {
	return fmt.Sprintf(
		"请基于企微消息端场景，生成一段可直接发送给客户的消息端快捷话术。\n风格要求：%s。\n输出要求：\n1）只输出最终可发给客户的中文话术，不要解释。\n2）控制在80~160字。\n3）避免夸大承诺，必要时引导转人工或进一步沟通。\n用户需求：%s",
		strings.TrimSpace(style),
		strings.TrimSpace(prompt),
	)
}
