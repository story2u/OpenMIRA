// Reply scripts expose the admin-managed and CS quick-reply library. AI
// generation remains with Python until that route is migrated separately.
package workbench

import (
	"context"
	"errors"
	"strings"

	"wework-go/internal/auth"
)

var (
	// ErrReplyScriptStoreUnavailable means reply scripts cannot be loaded.
	ErrReplyScriptStoreUnavailable = errors.New("workbench reply script store is unavailable")
	// ErrReplyScriptTitleRequired preserves FastAPI's explicit blank-title failure.
	ErrReplyScriptTitleRequired = errors.New("title is required")
	// ErrReplyScriptContentRequired preserves FastAPI's explicit blank-content failure.
	ErrReplyScriptContentRequired = errors.New("content is required")
)

const defaultTargetAudienceAll = "__ALL__"

// ReplyScriptRecord is the stable HTTP shape for reply_scripts rows.
type ReplyScriptRecord struct {
	ScriptID       string
	Title          string
	Content        string
	Category       string
	Enabled        bool
	TargetAudience string
	CreatedAt      string
	UpdatedAt      string
}

// ReplyScriptCommand carries one quick-reply script upsert command.
type ReplyScriptCommand struct {
	ScriptID       string
	Title          string
	Content        string
	Category       string
	Enabled        bool
	TargetAudience string
}

// ReplyScriptsRequest carries the authenticated management session.
type ReplyScriptsRequest struct {
	Session auth.Session
}

// ReplyScriptUpsertRequest carries the legacy POST request body.
type ReplyScriptUpsertRequest struct {
	Session auth.Session
	Command ReplyScriptCommand
}

// ReplyScriptDeleteRequest carries the legacy DELETE path parameter.
type ReplyScriptDeleteRequest struct {
	Session  auth.Session
	ScriptID string
}

// ReplyScriptUpsertBody is the JSON input for POST /admin/scripts.
type ReplyScriptUpsertBody struct {
	ScriptID       string `json:"script_id"`
	Title          string `json:"title"`
	Content        string `json:"content"`
	Category       string `json:"category"`
	Enabled        *bool  `json:"enabled"`
	TargetAudience string `json:"target_audience"`
}

// NewReplyScriptsRequest normalizes the reply scripts request boundary.
func NewReplyScriptsRequest(session auth.Session) ReplyScriptsRequest {
	return ReplyScriptsRequest{Session: session}
}

// NewReplyScriptUpsertRequest normalizes the upsert body boundary.
func NewReplyScriptUpsertRequest(body ReplyScriptUpsertBody, session auth.Session) ReplyScriptUpsertRequest {
	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	category := strings.TrimSpace(body.Category)
	if category == "" {
		category = "default"
	}
	return ReplyScriptUpsertRequest{
		Session: session,
		Command: ReplyScriptCommand{
			ScriptID:       strings.TrimSpace(body.ScriptID),
			Title:          strings.TrimSpace(body.Title),
			Content:        strings.TrimSpace(body.Content),
			Category:       category,
			Enabled:        enabled,
			TargetAudience: normalizeReplyScriptTargetAudience(body.TargetAudience),
		},
	}
}

// NewReplyScriptDeleteRequest normalizes the delete path parameter.
func NewReplyScriptDeleteRequest(scriptID string, session auth.Session) ReplyScriptDeleteRequest {
	return ReplyScriptDeleteRequest{Session: session, ScriptID: strings.TrimSpace(scriptID)}
}

// ReplyScripts builds the read-only /api/v1/admin/scripts payload.
func (service Service) ReplyScripts(ctx context.Context, request ReplyScriptsRequest) (Payload, error) {
	if service.ReplyScriptStore == nil {
		return nil, ErrReplyScriptStoreUnavailable
	}
	scripts, err := service.ReplyScriptStore.ListReplyScripts(ctx)
	if err != nil {
		return nil, err
	}
	return Payload{"scripts": replyScriptPayload(scripts)}, nil
}

// ScriptLibrary builds the read-only /api/v1/scripts payload.
func (service Service) ScriptLibrary(ctx context.Context, request ReplyScriptsRequest) (Payload, error) {
	if service.ReplyScriptStore == nil {
		return nil, ErrReplyScriptStoreUnavailable
	}
	scripts, err := service.ReplyScriptStore.ListReplyScripts(ctx)
	if err != nil {
		return nil, err
	}
	return Payload{"scripts": replyScriptPayload(filterScriptsForSession(scripts, request.Session))}, nil
}

// UpsertReplyScript handles POST /api/v1/admin/scripts.
func (service Service) UpsertReplyScript(ctx context.Context, request ReplyScriptUpsertRequest) (Payload, error) {
	store := service.replyScriptWriteStore()
	if store == nil {
		return nil, ErrReplyScriptStoreUnavailable
	}
	command := request.Command
	command.Title = strings.TrimSpace(command.Title)
	command.Content = strings.TrimSpace(command.Content)
	command.Category = strings.TrimSpace(command.Category)
	if command.Category == "" {
		command.Category = "default"
	}
	command.TargetAudience = normalizeReplyScriptTargetAudience(command.TargetAudience)
	if command.Title == "" {
		return nil, ErrReplyScriptTitleRequired
	}
	if command.Content == "" {
		return nil, ErrReplyScriptContentRequired
	}
	script, err := store.UpsertReplyScript(ctx, command)
	if err != nil {
		return nil, err
	}
	payload := replyScriptRecordPayload(script)
	if service.ReplyScriptEvents != nil {
		if err := service.ReplyScriptEvents.Publish(ctx, "devices", "script.updated", "sop.changed", map[string]any(payload)); err != nil {
			return nil, err
		}
	}
	return Payload{"success": true, "script": payload}, nil
}

// DeleteReplyScript handles DELETE /api/v1/admin/scripts/{script_id}.
func (service Service) DeleteReplyScript(ctx context.Context, request ReplyScriptDeleteRequest) (Payload, error) {
	store := service.replyScriptWriteStore()
	if store == nil {
		return nil, ErrReplyScriptStoreUnavailable
	}
	scriptID := strings.TrimSpace(request.ScriptID)
	deleted, err := store.DeleteReplyScript(ctx, scriptID)
	if err != nil {
		return nil, err
	}
	if deleted && service.ReplyScriptEvents != nil {
		if err := service.ReplyScriptEvents.Publish(ctx, "devices", "script.deleted", "sop.changed", map[string]any{"script_id": scriptID}); err != nil {
			return nil, err
		}
	}
	return Payload{"success": deleted}, nil
}

// filterScriptsForSession applies the legacy CS enabled-only rule.
func filterScriptsForSession(scripts []ReplyScriptRecord, session auth.Session) []ReplyScriptRecord {
	if strings.TrimSpace(session.Role) != "cs" {
		return scripts
	}
	filtered := make([]ReplyScriptRecord, 0, len(scripts))
	for _, script := range scripts {
		if script.Enabled {
			filtered = append(filtered, script)
		}
	}
	return filtered
}

// replyScriptPayload serializes rows to the legacy scripts[] shape.
func replyScriptPayload(scripts []ReplyScriptRecord) []ProjectionRow {
	payload := make([]ProjectionRow, 0, len(scripts))
	for _, script := range scripts {
		scriptID := strings.TrimSpace(script.ScriptID)
		if scriptID == "" {
			continue
		}
		payload = append(payload, replyScriptRecordPayload(script))
	}
	return payload
}

func replyScriptRecordPayload(script ReplyScriptRecord) ProjectionRow {
	return ProjectionRow{
		"script_id":       strings.TrimSpace(script.ScriptID),
		"title":           strings.TrimSpace(script.Title),
		"content":         strings.TrimSpace(script.Content),
		"category":        strings.TrimSpace(script.Category),
		"enabled":         script.Enabled,
		"target_audience": strings.TrimSpace(script.TargetAudience),
		"created_at":      nilIfBlank(strings.TrimSpace(script.CreatedAt)),
		"updated_at":      nilIfBlank(strings.TrimSpace(script.UpdatedAt)),
	}
}

func (service Service) replyScriptWriteStore() ReplyScriptWriteStore {
	if service.ReplyScriptWriteStore != nil {
		return service.ReplyScriptWriteStore
	}
	if store, ok := service.ReplyScriptStore.(ReplyScriptWriteStore); ok {
		return store
	}
	return nil
}

func normalizeReplyScriptTargetAudience(value string) string {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return defaultTargetAudienceNone
	}
	if normalized == defaultTargetAudienceAll || normalized == defaultTargetAudienceNone {
		return normalized
	}
	normalized = strings.NewReplacer("\n", ",", "，", ",", "；", ",").Replace(normalized)
	parts := strings.Split(normalized, ",")
	values := make([]string, 0, len(parts))
	seen := map[string]bool{}
	for _, part := range parts {
		candidate := strings.TrimSpace(part)
		if candidate == "" || candidate == defaultTargetAudienceAll || candidate == defaultTargetAudienceNone || seen[candidate] {
			continue
		}
		seen[candidate] = true
		values = append(values, candidate)
	}
	if len(values) == 0 {
		return defaultTargetAudienceNone
	}
	return strings.Join(values, ",")
}
