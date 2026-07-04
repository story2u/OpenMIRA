// Sensitive words expose the admin-managed interception dictionary.
// Write candidates only mutate sensitive_words, refresh the Go process cache,
// and append Python-compatible audit rows; AI/SOP blocking remains separate.
package workbench

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"wework-go/internal/auth"
)

var (
	// ErrSensitiveWordStoreUnavailable means sensitive words cannot be loaded.
	ErrSensitiveWordStoreUnavailable = errors.New("workbench sensitive word store is unavailable")
	// ErrSensitiveWordRequired preserves FastAPI's explicit blank-word failure.
	ErrSensitiveWordRequired = errors.New("word is required")
)

// SensitiveWordRecord is the stable HTTP shape for sensitive_words rows.
type SensitiveWordRecord struct {
	WordID    string
	Word      string
	Enabled   bool
	CreatedAt string
	UpdatedAt string
}

// SensitiveWordCommand carries one sensitive word upsert command.
type SensitiveWordCommand struct {
	WordID  string
	Word    string
	Enabled bool
}

// SensitiveWordsRequest carries the authenticated management session.
type SensitiveWordsRequest struct {
	Session auth.Session
}

// SensitiveWordUpsertRequest carries the legacy POST request body.
type SensitiveWordUpsertRequest struct {
	Session auth.Session
	Command SensitiveWordCommand
}

// SensitiveWordDeleteRequest carries the legacy DELETE path parameter.
type SensitiveWordDeleteRequest struct {
	Session auth.Session
	WordID  string
}

// SensitiveWordUpsertBody is the JSON input for POST /sensitive-words.
type SensitiveWordUpsertBody struct {
	WordID  string `json:"word_id"`
	Word    string `json:"word"`
	Enabled *bool  `json:"enabled"`
}

// NewSensitiveWordsRequest normalizes the sensitive words request boundary.
func NewSensitiveWordsRequest(session auth.Session) SensitiveWordsRequest {
	return SensitiveWordsRequest{Session: session}
}

// NewSensitiveWordUpsertRequest normalizes the upsert body boundary.
func NewSensitiveWordUpsertRequest(body SensitiveWordUpsertBody, session auth.Session) SensitiveWordUpsertRequest {
	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	return SensitiveWordUpsertRequest{
		Session: session,
		Command: SensitiveWordCommand{
			WordID:  strings.TrimSpace(body.WordID),
			Word:    strings.TrimSpace(body.Word),
			Enabled: enabled,
		},
	}
}

// NewSensitiveWordDeleteRequest normalizes the delete path parameter.
func NewSensitiveWordDeleteRequest(wordID string, session auth.Session) SensitiveWordDeleteRequest {
	return SensitiveWordDeleteRequest{Session: session, WordID: strings.TrimSpace(wordID)}
}

// SensitiveWords builds the read-only /api/v1/admin/sensitive-words payload.
func (service Service) SensitiveWords(ctx context.Context, request SensitiveWordsRequest) (Payload, error) {
	if service.SensitiveWordStore == nil {
		return nil, ErrSensitiveWordStoreUnavailable
	}
	words, err := service.SensitiveWordStore.ListSensitiveWords(ctx)
	if err != nil {
		return nil, err
	}
	return Payload{"words": sensitiveWordPayload(words)}, nil
}

// UpsertSensitiveWord handles POST /api/v1/admin/sensitive-words.
func (service Service) UpsertSensitiveWord(ctx context.Context, request SensitiveWordUpsertRequest) (Payload, error) {
	if service.SensitiveWordStore == nil {
		return nil, ErrSensitiveWordStoreUnavailable
	}
	command := request.Command
	command.Word = strings.TrimSpace(command.Word)
	if command.Word == "" {
		return nil, ErrSensitiveWordRequired
	}
	word, err := service.SensitiveWordStore.UpsertSensitiveWord(ctx, command)
	if err != nil {
		return nil, err
	}
	if err := service.SensitiveWordStore.ReloadSensitiveWordCache(ctx); err != nil {
		return nil, err
	}
	if service.AuditLogWriter != nil {
		if _, err := service.AuditLogWriter.AddAuditLog(ctx, AuditLogEntry{
			Operator:   strings.TrimSpace(request.Session.AssigneeID),
			ActionType: "config",
			Detail:     fmt.Sprintf("新增/更新敏感词: %s", command.Word),
		}); err != nil {
			return nil, err
		}
	}
	return Payload{"success": true, "word": sensitiveWordRecordPayload(word)}, nil
}

// DeleteSensitiveWord handles DELETE /api/v1/admin/sensitive-words/{word_id}.
func (service Service) DeleteSensitiveWord(ctx context.Context, request SensitiveWordDeleteRequest) (Payload, error) {
	if service.SensitiveWordStore == nil {
		return nil, ErrSensitiveWordStoreUnavailable
	}
	deleted, err := service.SensitiveWordStore.DeleteSensitiveWord(ctx, strings.TrimSpace(request.WordID))
	if err != nil {
		return nil, err
	}
	if deleted {
		if err := service.SensitiveWordStore.ReloadSensitiveWordCache(ctx); err != nil {
			return nil, err
		}
	}
	return Payload{"success": deleted}, nil
}

func sensitiveWordPayload(words []SensitiveWordRecord) []ProjectionRow {
	payload := make([]ProjectionRow, 0, len(words))
	for _, word := range words {
		wordID := strings.TrimSpace(word.WordID)
		if wordID == "" {
			continue
		}
		payload = append(payload, sensitiveWordRecordPayload(word))
	}
	return payload
}

func sensitiveWordRecordPayload(word SensitiveWordRecord) ProjectionRow {
	return ProjectionRow{
		"word_id":    strings.TrimSpace(word.WordID),
		"word":       strings.TrimSpace(word.Word),
		"enabled":    word.Enabled,
		"created_at": nilIfBlank(strings.TrimSpace(word.CreatedAt)),
		"updated_at": nilIfBlank(strings.TrimSpace(word.UpdatedAt)),
	}
}
