package workbench

import (
	"context"
	"strings"
	"time"

	"wework-go/internal/auth"
)

// AIConfigTestBody is the JSON input for POST /admin/ai-config/test.
type AIConfigTestBody struct {
	Prompt       string   `json:"prompt"`
	BaseURL      string   `json:"base_url"`
	APIKey       string   `json:"api_key"`
	Model        string   `json:"model"`
	TimeoutSec   *float64 `json:"timeout_sec"`
	Temperature  *float64 `json:"temperature"`
	SystemPrompt string   `json:"system_prompt"`
}

// AIConfigTestRequest carries the authenticated AI provider probe request.
type AIConfigTestRequest struct {
	Session auth.Session
	Body    AIConfigTestBody
}

// NewAIConfigTestRequest normalizes the AI provider probe request boundary.
func NewAIConfigTestRequest(body AIConfigTestBody, session auth.Session) AIConfigTestRequest {
	body.Prompt = strings.TrimSpace(body.Prompt)
	body.BaseURL = strings.TrimSpace(body.BaseURL)
	body.APIKey = strings.TrimSpace(body.APIKey)
	body.Model = strings.TrimSpace(body.Model)
	body.SystemPrompt = strings.TrimSpace(body.SystemPrompt)
	return AIConfigTestRequest{Session: session, Body: body}
}

// TestAIConfig validates the current or submitted OpenAI-compatible settings by
// asking the configured provider for one short reply.
func (service Service) TestAIConfig(ctx context.Context, request AIConfigTestRequest) (Payload, error) {
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
	cfg = applyAIConfigProbeOverrides(cfg, request.Body)
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return nil, ErrAIConfigBaseURLRequired
	}
	if strings.TrimSpace(cfg.Model) == "" {
		return nil, ErrAIConfigModelRequired
	}
	if cfg.Timeout <= 0 {
		return nil, ErrAIConfigTimeoutInvalid
	}
	if cfg.Temperature < 0 || cfg.Temperature > 2 {
		return nil, ErrAIConfigTemperature
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, ErrScriptAIAPIKeyMissing
	}
	reply, err := service.ScriptTextGenerator.GenerateText(ctx, ScriptTextGenerationInput{
		Prompt:       request.Body.Prompt,
		BaseURL:      cfg.BaseURL,
		APIKey:       cfg.APIKey,
		Model:        cfg.Model,
		Timeout:      cfg.Timeout,
		Temperature:  cfg.Temperature,
		SystemPrompt: cfg.SystemPrompt,
	})
	if err != nil {
		return nil, ScriptAIGenerationError{Err: err}
	}
	reply = strings.TrimSpace(reply)
	return Payload{"success": true, "reply": reply, "content": reply}, nil
}

func applyAIConfigProbeOverrides(cfg scriptGenerationConfig, body AIConfigTestBody) scriptGenerationConfig {
	if body.BaseURL != "" {
		cfg.BaseURL = body.BaseURL
	}
	if body.APIKey != "" {
		cfg.APIKey = body.APIKey
	}
	if body.Model != "" {
		cfg.Model = body.Model
	}
	if body.TimeoutSec != nil {
		cfg.Timeout = time.Duration(*body.TimeoutSec * float64(time.Second))
	}
	if body.Temperature != nil {
		cfg.Temperature = *body.Temperature
	}
	if body.SystemPrompt != "" {
		cfg.SystemPrompt = body.SystemPrompt
	}
	return cfg
}
