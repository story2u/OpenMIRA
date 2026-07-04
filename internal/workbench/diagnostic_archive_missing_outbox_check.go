package workbench

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"wework-go/internal/archiveingest"
	"wework-go/internal/auth"
	"wework-go/internal/outbox"
)

var (
	// ErrDiagnosticArchiveMissingOutboxStoreUnavailable means archive outbox gap rows cannot be read.
	ErrDiagnosticArchiveMissingOutboxStoreUnavailable = errors.New("workbench diagnostic archive missing outbox store is unavailable")
	// ErrDiagnosticArchiveMissingOutboxReplayUnavailable means archive outbox gap rows cannot be replayed.
	ErrDiagnosticArchiveMissingOutboxReplayUnavailable = errors.New("outbox repository is not available")
)

var archiveMissingOutboxShanghai = time.FixedZone("Asia/Shanghai", 8*60*60)

// ArchiveMissingOutboxValidationError preserves FastAPI/HTTPException details for body validation.
type ArchiveMissingOutboxValidationError struct {
	Detail        string
	Unprocessable bool
}

// Error returns the legacy diagnostic detail.
func (err ArchiveMissingOutboxValidationError) Error() string {
	return strings.TrimSpace(err.Detail)
}

// ArchiveMissingOutboxCheckBody mirrors the JSON body for archive missing outbox checks.
type ArchiveMissingOutboxCheckBody struct {
	EnterpriseID string `json:"enterprise_id"`
	StartAt      string `json:"start_at"`
	EndAt        string `json:"end_at"`
	Limit        *int   `json:"limit"`
}

// ArchiveMissingOutboxReplayBody mirrors the JSON body for archive missing outbox replay.
type ArchiveMissingOutboxReplayBody struct {
	EnterpriseID string `json:"enterprise_id"`
	StartAt      string `json:"start_at"`
	EndAt        string `json:"end_at"`
	Limit        *int   `json:"limit"`
	DryRun       *bool  `json:"dry_run"`
}

// ArchiveMissingOutboxCheckRequest carries normalized check parameters.
type ArchiveMissingOutboxCheckRequest struct {
	Session      auth.Session
	EnterpriseID string
	StartAt      string
	EndAt        string
	Limit        int
}

// ArchiveMissingOutboxReplayRequest carries normalized replay parameters.
type ArchiveMissingOutboxReplayRequest struct {
	Session      auth.Session
	EnterpriseID string
	StartAt      string
	EndAt        string
	Limit        int
	DryRun       bool
}

// ArchiveMissingOutboxCheckQuery is the repository scan contract.
type ArchiveMissingOutboxCheckQuery struct {
	EnterpriseID string
	StartAt      string
	EndAt        string
	Limit        int
}

// ArchiveMissingOutboxRecord carries one archive incoming message missing canonical outbox.
type ArchiveMissingOutboxRecord struct {
	TraceID          string
	TenantID         string
	ArchiveMsgID     string
	ConversationID   string
	ConversationKey  string
	WeWorkUserID     string
	ExternalUserID   string
	RoomID           string
	ConversationType string
	DeviceID         string
	SenderID         string
	SenderName       string
	SenderAvatar     string
	SenderRemark     string
	ConversationName string
	FirstMessageAt   any
	AIAutoReply      bool
	Content          string
	MsgType          string
	Timestamp        any
	MessageCreatedAt any
}

// ArchiveMissingOutboxReplayOutbox appends rebuilt canonical outbox events.
type ArchiveMissingOutboxReplayOutbox interface {
	EnqueueMany(ctx context.Context, events []outbox.EventEnvelope) ([]outbox.Record, error)
}

// NewArchiveMissingOutboxCheckRequest validates and normalizes the POST body.
func NewArchiveMissingOutboxCheckRequest(body ArchiveMissingOutboxCheckBody, session auth.Session) (ArchiveMissingOutboxCheckRequest, error) {
	enterpriseID := strings.TrimSpace(body.EnterpriseID)
	if enterpriseID == "" {
		return ArchiveMissingOutboxCheckRequest{}, archiveMissingOutboxBadRequest("enterprise_id is required")
	}
	startAt, err := normalizeArchiveMissingOutboxTimeParam(body.StartAt, "start_at")
	if err != nil {
		return ArchiveMissingOutboxCheckRequest{}, err
	}
	endAt, err := normalizeArchiveMissingOutboxTimeParam(body.EndAt, "end_at")
	if err != nil {
		return ArchiveMissingOutboxCheckRequest{}, err
	}
	if startAt >= endAt {
		return ArchiveMissingOutboxCheckRequest{}, archiveMissingOutboxBadRequest("start_at must be earlier than end_at")
	}
	limit := 100
	if body.Limit != nil {
		limit = *body.Limit
	}
	if limit < 1 || limit > 500 {
		return ArchiveMissingOutboxCheckRequest{}, ArchiveMissingOutboxValidationError{
			Detail:        "invalid limit, expected 1..500",
			Unprocessable: true,
		}
	}
	return ArchiveMissingOutboxCheckRequest{
		Session:      session,
		EnterpriseID: enterpriseID,
		StartAt:      startAt,
		EndAt:        endAt,
		Limit:        limit,
	}, nil
}

// NewArchiveMissingOutboxReplayRequest validates and normalizes the replay POST body.
func NewArchiveMissingOutboxReplayRequest(body ArchiveMissingOutboxReplayBody, session auth.Session) (ArchiveMissingOutboxReplayRequest, error) {
	check, err := NewArchiveMissingOutboxCheckRequest(ArchiveMissingOutboxCheckBody{
		EnterpriseID: body.EnterpriseID,
		StartAt:      body.StartAt,
		EndAt:        body.EndAt,
		Limit:        body.Limit,
	}, session)
	if err != nil {
		return ArchiveMissingOutboxReplayRequest{}, err
	}
	dryRun := true
	if body.DryRun != nil {
		dryRun = *body.DryRun
	}
	return ArchiveMissingOutboxReplayRequest{
		Session:      session,
		EnterpriseID: check.EnterpriseID,
		StartAt:      check.StartAt,
		EndAt:        check.EndAt,
		Limit:        check.Limit,
		DryRun:       dryRun,
	}, nil
}

// DiagnosticArchiveMissingOutboxCheck builds /api/v1/admin/diagnostic/archive-missing-message-outbox/check.
func (service Service) DiagnosticArchiveMissingOutboxCheck(ctx context.Context, request ArchiveMissingOutboxCheckRequest) (Payload, error) {
	if service.DiagnosticMissingOutbox == nil {
		return nil, ErrDiagnosticArchiveMissingOutboxStoreUnavailable
	}
	records, err := service.DiagnosticMissingOutbox.ListArchiveMissingMessageOutbox(ctx, ArchiveMissingOutboxCheckQuery{
		EnterpriseID: request.EnterpriseID,
		StartAt:      request.StartAt,
		EndAt:        request.EndAt,
		Limit:        request.Limit,
	})
	if err != nil {
		return nil, err
	}
	items := make([]Payload, 0, len(records))
	for _, record := range records {
		traceID := strings.TrimSpace(record.TraceID)
		expectedEventID := ""
		if traceID != "" {
			expectedEventID = buildArchiveTenantScopedEventID(request.EnterpriseID, traceID, "conversation-message", "conversation-message", record.ConversationID)
		}
		items = append(items, Payload{
			"trace_id":           traceID,
			"archive_msgid":      strings.TrimSpace(record.ArchiveMsgID),
			"conversation_id":    strings.TrimSpace(record.ConversationID),
			"conversation_key":   strings.TrimSpace(record.ConversationKey),
			"wework_user_id":     strings.TrimSpace(record.WeWorkUserID),
			"external_userid":    strings.TrimSpace(record.ExternalUserID),
			"room_id":            strings.TrimSpace(record.RoomID),
			"msg_type":           strings.TrimSpace(record.MsgType),
			"timestamp":          record.Timestamp,
			"message_created_at": record.MessageCreatedAt,
			"expected_event_id":  expectedEventID,
		})
	}
	return Payload{
		"enterprise_id":   request.EnterpriseID,
		"start_at":        request.StartAt,
		"end_at":          request.EndAt,
		"candidate_count": len(items),
		"items":           items,
	}, nil
}

// DiagnosticArchiveMissingOutboxReplay builds /api/v1/admin/diagnostic/archive-missing-message-outbox/replay.
func (service Service) DiagnosticArchiveMissingOutboxReplay(ctx context.Context, request ArchiveMissingOutboxReplayRequest) (Payload, error) {
	if service.DiagnosticMissingOutbox == nil {
		return nil, ErrDiagnosticArchiveMissingOutboxStoreUnavailable
	}
	if service.DiagnosticMissingOutboxReplayOutbox == nil {
		return nil, ErrDiagnosticArchiveMissingOutboxReplayUnavailable
	}
	records, err := service.DiagnosticMissingOutbox.ListArchiveMissingMessageOutbox(ctx, ArchiveMissingOutboxCheckQuery{
		EnterpriseID: request.EnterpriseID,
		StartAt:      request.StartAt,
		EndAt:        request.EndAt,
		Limit:        request.Limit,
	})
	if err != nil {
		return nil, err
	}
	occurredAt := service.now()
	events := make([]outbox.EventEnvelope, 0, len(records))
	items := make([]Payload, 0, len(records))
	for _, record := range records {
		payload := archiveMissingOutboxPayload(record, request.EnterpriseID, occurredAt)
		event := archiveingest.BuildArchiveConversationMessageReceivedEvent(request.EnterpriseID, strings.TrimSpace(record.TraceID), payload, occurredAt)
		events = append(events, event)
		items = append(items, archiveMissingOutboxItem(record, request.EnterpriseID, event.EventID))
	}
	outboxRecords := []outbox.Record{}
	if !request.DryRun && len(events) > 0 {
		outboxRecords, err = service.DiagnosticMissingOutboxReplayOutbox.EnqueueMany(ctx, events)
		if err != nil {
			return nil, err
		}
	}
	return Payload{
		"dry_run":         request.DryRun,
		"enterprise_id":   request.EnterpriseID,
		"start_at":        request.StartAt,
		"end_at":          request.EndAt,
		"candidate_count": len(items),
		"replayed_count":  len(outboxRecords),
		"items":           items,
	}, nil
}

func archiveMissingOutboxBadRequest(detail string) error {
	return ArchiveMissingOutboxValidationError{Detail: detail}
}

func archiveMissingOutboxItem(record ArchiveMissingOutboxRecord, enterpriseID string, eventID string) Payload {
	traceID := strings.TrimSpace(record.TraceID)
	expectedEventID := ""
	if traceID != "" {
		expectedEventID = buildArchiveTenantScopedEventID(enterpriseID, traceID, "conversation-message", "conversation-message", record.ConversationID)
	}
	item := Payload{
		"trace_id":           traceID,
		"archive_msgid":      strings.TrimSpace(record.ArchiveMsgID),
		"conversation_id":    strings.TrimSpace(record.ConversationID),
		"conversation_key":   strings.TrimSpace(record.ConversationKey),
		"wework_user_id":     strings.TrimSpace(record.WeWorkUserID),
		"external_userid":    strings.TrimSpace(record.ExternalUserID),
		"room_id":            strings.TrimSpace(record.RoomID),
		"msg_type":           strings.TrimSpace(record.MsgType),
		"timestamp":          record.Timestamp,
		"message_created_at": record.MessageCreatedAt,
		"expected_event_id":  expectedEventID,
	}
	if strings.TrimSpace(eventID) != "" {
		item["event_id"] = strings.TrimSpace(eventID)
	}
	return item
}

func archiveMissingOutboxPayload(record ArchiveMissingOutboxRecord, enterpriseID string, fallbackNow time.Time) map[string]any {
	conversationID := strings.TrimSpace(record.ConversationID)
	conversationKey := firstNonBlank(strings.TrimSpace(record.ConversationKey), conversationID)
	timestamp := archiveMissingOutboxPayloadTime(record.Timestamp, fallbackNow)
	payload := map[string]any{
		"conversation_id":          conversationID,
		"conversation_key":         conversationKey,
		"resolved_conversation_id": conversationKey,
		"tenant_id":                strings.TrimSpace(enterpriseID),
		"trace_id":                 strings.TrimSpace(record.TraceID),
		"archive_msgid":            strings.TrimSpace(record.ArchiveMsgID),
		"wework_user_id":           strings.TrimSpace(record.WeWorkUserID),
		"external_userid":          strings.TrimSpace(record.ExternalUserID),
		"room_id":                  strings.TrimSpace(record.RoomID),
		"conversation_type":        firstNonBlank(strings.TrimSpace(record.ConversationType), "single"),
		"sender_id":                strings.TrimSpace(record.SenderID),
		"sender_name":              strings.TrimSpace(record.SenderName),
		"sender_avatar":            strings.TrimSpace(record.SenderAvatar),
		"sender_remark":            strings.TrimSpace(record.SenderRemark),
		"conversation_name":        firstNonBlank(strings.TrimSpace(record.ConversationName), strings.TrimSpace(record.SenderName)),
		"content":                  record.Content,
		"msg_type":                 firstNonBlank(strings.TrimSpace(record.MsgType), "text"),
		"direction":                "incoming",
		"device_id":                strings.TrimSpace(record.DeviceID),
		"timestamp":                timestamp,
		"created_at":               timestamp,
		"is_system_event":          false,
		"publish_event":            "conversation.archive_ingested",
		"message_created":          true,
		"identity_needs_refresh":   false,
		"ingest_source":            "archive_history",
		"canonical_source":         "archive_primary",
		"reconciled_from_archive":  false,
		"ai_auto_reply":            record.AIAutoReply,
		"first_message_at":         stringOrNil(record.FirstMessageAt),
		"auto_reply_candidate":     false,
		"outbox_repaired":          true,
	}
	return payload
}

func archiveMissingOutboxPayloadTime(value any, fallback time.Time) string {
	switch typed := value.(type) {
	case time.Time:
		if !typed.IsZero() {
			return typed.Format(time.RFC3339Nano)
		}
	case []byte:
		text := strings.TrimSpace(string(typed))
		if text != "" {
			return strings.ReplaceAll(text, " ", "T")
		}
	case string:
		text := strings.TrimSpace(typed)
		if text != "" {
			return strings.ReplaceAll(text, " ", "T")
		}
	}
	if text := strings.TrimSpace(fmt.Sprint(value)); value != nil && text != "" {
		normalized := strings.ReplaceAll(text, " ", "T")
		if strings.Contains(normalized, "T") {
			return normalized
		}
		return text
	}
	if fallback.IsZero() {
		fallback = time.Now().UTC()
	}
	return fallback.UTC().Format(time.RFC3339Nano)
}

func stringOrNil(value any) any {
	if value == nil {
		return nil
	}
	switch typed := value.(type) {
	case time.Time:
		if typed.IsZero() {
			return nil
		}
		return typed.Format(time.RFC3339Nano)
	case []byte:
		text := strings.TrimSpace(string(typed))
		if text == "" {
			return nil
		}
		return text
	}
	text := strings.TrimSpace(fmt.Sprint(value))
	if text == "" {
		return nil
	}
	return text
}

func normalizeArchiveMissingOutboxTimeParam(value string, fieldName string) (string, error) {
	raw := strings.TrimSpace(value)
	text := strings.ReplaceAll(raw, "T", " ")
	if text == "" {
		return "", archiveMissingOutboxBadRequest(fmt.Sprintf("%s is required", fieldName))
	}
	if len(text) == len("2006-01-02") {
		text += " 00:00:00"
	}
	if parsed, ok := parseArchiveMissingOutboxAwareTime(raw); ok {
		return parsed.In(archiveMissingOutboxShanghai).Format("2006-01-02 15:04:05"), nil
	}
	if parsed, ok := parseArchiveMissingOutboxAwareTime(text); ok {
		return parsed.In(archiveMissingOutboxShanghai).Format("2006-01-02 15:04:05"), nil
	}
	for _, layout := range []string{"2006-01-02 15:04:05.999999", "2006-01-02 15:04:05"} {
		if parsed, err := time.ParseInLocation(layout, text, archiveMissingOutboxShanghai); err == nil {
			return parsed.Format("2006-01-02 15:04:05"), nil
		}
	}
	return "", archiveMissingOutboxBadRequest(fmt.Sprintf("%s must be ISO datetime", fieldName))
}

func parseArchiveMissingOutboxAwareTime(value string) (time.Time, bool) {
	text := strings.TrimSpace(value)
	if text == "" {
		return time.Time{}, false
	}
	if parsed, err := time.Parse(time.RFC3339Nano, text); err == nil {
		return parsed, true
	}
	for _, layout := range []string{
		"2006-01-02 15:04:05.999999Z07:00",
		"2006-01-02 15:04:05Z07:00",
		"2006-01-02 15:04:05.999999-07:00",
		"2006-01-02 15:04:05-07:00",
	} {
		if parsed, err := time.Parse(layout, text); err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

func buildArchiveTenantScopedEventID(enterpriseID string, traceID string, suffix string, fallbackPrefix string, fallbackSeed string) string {
	enterpriseKey := strings.TrimSpace(enterpriseID)
	if enterpriseKey == "" {
		enterpriseKey = "default"
	}
	traceKey := strings.TrimSpace(traceID)
	suffixKey := strings.TrimSpace(suffix)
	if traceKey != "" && suffixKey != "" {
		return enterpriseKey + ":" + traceKey + ":" + suffixKey
	}
	fallbackKey := strings.TrimSpace(fallbackSeed)
	if fallbackKey == "" {
		fallbackKey = "archive-event"
	}
	prefixKey := strings.TrimSpace(fallbackPrefix)
	if prefixKey == "" {
		prefixKey = "archive-event"
	}
	return prefixKey + ":" + enterpriseKey + ":" + fallbackKey
}
