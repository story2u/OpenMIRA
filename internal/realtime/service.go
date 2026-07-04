// Package realtime builds read-only realtime replay payloads.
package realtime

import (
	"context"
	"strings"
	"time"
)

const (
	// DefaultReplayLimit mirrors FastAPI Query default for replay_events.
	DefaultReplayLimit = 100
	// MaxReplayLimit mirrors FastAPI Query le=1000.
	MaxReplayLimit = 1000
)

var snapshotScopes = []string{
	"conversations:conversation.message",
	"conversations:conversation.assignment",
	"chat:identity.updated",
}

// Payload is a JSON-compatible response body.
type Payload map[string]any

// EventRecord is one realtime_event_log row after Python normalization.
type EventRecord struct {
	ScopeKey    string
	Cursor      int64
	Channel     string
	Event       string
	Topic       string
	Consistency string
	Payload     map[string]any
	CreatedAt   any
}

// EventLogStore reads strong realtime event log rows.
type EventLogStore interface {
	ListAfterCursor(ctx context.Context, scopeKey string, afterCursor int64, limit int) ([]EventRecord, error)
	LatestCursor(ctx context.Context, scopeKey string) (int64, error)
}

// Service owns read-only realtime compensation payload assembly.
type Service struct {
	Events EventLogStore
	Now    func() time.Time
}

// ReplayRequest carries GET /api/v1/realtime/events/replay query params.
type ReplayRequest struct {
	Scope       string
	AfterCursor int64
	Limit       int
}

// ReplayEvents builds the legacy replay response.
func (service Service) ReplayEvents(ctx context.Context, request ReplayRequest) (Payload, error) {
	scope := strings.TrimSpace(request.Scope)
	after := request.AfterCursor
	limit := normalizeLimit(request.Limit)
	if scope == "" {
		return emptyReplay(0), nil
	}
	if service.Events == nil {
		return emptyReplay(0), nil
	}
	records, err := service.Events.ListAfterCursor(ctx, scope, after, limit+1)
	if err != nil {
		return emptyReplay(after), nil
	}
	hasMore := len(records) > limit
	if hasMore {
		records = records[:limit]
	}
	events := make([]Payload, 0, len(records))
	for _, record := range records {
		events = append(events, eventPayload(record))
	}
	latest := after
	if len(records) > 0 {
		latest = records[len(records)-1].Cursor
	}
	return Payload{
		"events":        events,
		"has_more":      hasMore,
		"latest_cursor": latest,
	}, nil
}

// SnapshotWorkbench builds the legacy workbench snapshot cursor response.
func (service Service) SnapshotWorkbench(ctx context.Context) (Payload, error) {
	cursors := map[string]int64{}
	if service.Events != nil {
		for _, scope := range snapshotScopes {
			cursor, err := service.Events.LatestCursor(ctx, scope)
			if err != nil {
				continue
			}
			if cursor > 0 {
				cursors[scope] = cursor
			}
		}
	}
	return Payload{
		"cursors":         cursors,
		"resync_required": false,
		"timestamp":       service.now().UTC().Format(time.RFC3339Nano),
	}, nil
}

func emptyReplay(latestCursor int64) Payload {
	return Payload{
		"events":        []Payload{},
		"has_more":      false,
		"latest_cursor": latestCursor,
	}
}

func eventPayload(record EventRecord) Payload {
	payload := record.Payload
	if payload == nil {
		payload = map[string]any{}
	}
	return Payload{
		"scope_key":   strings.TrimSpace(record.ScopeKey),
		"cursor":      record.Cursor,
		"channel":     strings.TrimSpace(record.Channel),
		"event":       strings.TrimSpace(record.Event),
		"topic":       strings.TrimSpace(record.Topic),
		"consistency": defaultText(record.Consistency, "strong"),
		"payload":     payload,
		"created_at":  record.CreatedAt,
	}
}

func (service Service) now() time.Time {
	if service.Now != nil {
		return service.Now()
	}
	return time.Now()
}

func normalizeLimit(limit int) int {
	if limit < 1 {
		return 1
	}
	if limit > MaxReplayLimit {
		return MaxReplayLimit
	}
	return limit
}

func defaultText(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}
