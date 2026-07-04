// Package friendadded owns the manual friend-added event candidate.
package friendadded

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"wework-go/internal/readmodelcache"
)

var (
	// ErrInvalidPayload means the JSON body does not match the legacy model.
	ErrInvalidPayload = errors.New("invalid friend-added payload")
	// ErrStoreUnavailable means the candidate was mounted without a store.
	ErrStoreUnavailable = errors.New("friend-added event store is not configured")
)

// Store persists friend_added_events rows and returns false for duplicate trace_id.
type Store interface {
	AddFriendEvent(ctx context.Context, event Event) (bool, error)
}

// ConversationTouchStore is an optional store capability mirroring Python's
// best-effort friend-added conversation precreation.
type ConversationTouchStore interface {
	TouchConversationFirstMessageAt(ctx context.Context, touch ConversationTouch) error
}

// EventPublisher publishes Python-compatible ws_hub events.
type EventPublisher interface {
	Publish(ctx context.Context, channel string, event string, topic string, payload map[string]any) error
}

// ReadModelInvalidator invalidates cached conversation read-model namespaces.
type ReadModelInvalidator interface {
	InvalidateNamespaces(ctx context.Context, namespaces ...string) error
}

// Service coordinates persistence, dedupe response, and realtime fanout.
type Service struct {
	Store                Store
	Events               EventPublisher
	Accounts             AccountStore
	SOPFlows             SOPFlowStore
	SOPPolicies          SOPPolicyStore
	Outbox               OutboxEnqueuer
	ReadModelInvalidator ReadModelInvalidator
	Now                  func() time.Time
}

// Request mirrors Python's FriendAddedEventCreate JSON contract.
type Request struct {
	DeviceID         string
	FriendName       string
	FriendID         string
	Source           string
	Timestamp        time.Time
	TraceID          string
	TenantID         *string
	AccountID        *string
	WeWorkUserID     *string
	AutoGreetContent string
}

// Event mirrors the durable friend_added_events row.
type Event struct {
	TraceID    string
	DeviceID   string
	FriendName string
	FriendID   string
	Source     string
	Timestamp  time.Time
	CreatedAt  time.Time
}

// ConversationTouch carries the fields needed to precreate a single friend conversation.
type ConversationTouch struct {
	DeviceID       string
	FriendID       string
	FriendName     string
	FirstMessageAt time.Time
	TenantID       string
	AccountID      string
	WeWorkUserID   string
}

// Response is the stable legacy endpoint response.
type Response struct {
	Accepted        bool   `json:"accepted"`
	Deduplicated    bool   `json:"deduplicated"`
	TraceID         string `json:"trace_id"`
	AutoGreetQueued bool   `json:"auto_greet_queued"`
}

// DecodeRequestJSON parses the legacy request and preserves Python defaults.
func DecodeRequestJSON(data []byte) (Request, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var raw map[string]json.RawMessage
	if err := decoder.Decode(&raw); err != nil || raw == nil {
		return Request{}, ErrInvalidPayload
	}
	var request Request
	var err error
	if request.DeviceID, err = requiredString(raw, "device_id"); err != nil {
		return Request{}, err
	}
	if request.FriendName, err = requiredString(raw, "friend_name"); err != nil {
		return Request{}, err
	}
	if request.TraceID, err = requiredString(raw, "trace_id"); err != nil {
		return Request{}, err
	}
	if request.Timestamp, err = requiredTimestamp(raw, "timestamp"); err != nil {
		return Request{}, err
	}
	if request.FriendID, err = optionalString(raw, "friend_id", ""); err != nil {
		return Request{}, err
	}
	if request.Source, err = optionalString(raw, "source", ""); err != nil {
		return Request{}, err
	}
	if request.TenantID, err = optionalStringPointer(raw, "tenant_id"); err != nil {
		return Request{}, err
	}
	if request.AccountID, err = optionalStringPointer(raw, "account_id"); err != nil {
		return Request{}, err
	}
	if request.WeWorkUserID, err = optionalStringPointer(raw, "wework_user_id"); err != nil {
		return Request{}, err
	}
	return request, nil
}

// Ingest stores the event, publishes friend.added, and returns the legacy ack.
func (service Service) Ingest(ctx context.Context, request Request) (Response, error) {
	if service.Store == nil {
		return Response{}, ErrStoreUnavailable
	}
	event := Event{
		TraceID:    request.TraceID,
		DeviceID:   request.DeviceID,
		FriendName: request.FriendName,
		FriendID:   request.FriendID,
		Source:     request.Source,
		Timestamp:  request.Timestamp.UTC(),
		CreatedAt:  service.now().UTC(),
	}
	inserted, err := service.Store.AddFriendEvent(ctx, event)
	if err != nil {
		return Response{}, err
	}
	autoGreetQueued := false
	if inserted {
		service.touchConversationFirstMessageAt(ctx, request)
		autoGreetQueued = service.queueAutoGreet(ctx, request)
		service.invalidateReadModels(ctx)
	}
	if service.Events != nil {
		if err := service.Events.Publish(ctx, "conversations", "friend.added", "friend.added", request.Payload()); err != nil {
			return Response{}, err
		}
	}
	return Response{
		Accepted:        true,
		Deduplicated:    !inserted,
		TraceID:         request.TraceID,
		AutoGreetQueued: autoGreetQueued,
	}, nil
}

func (service Service) invalidateReadModels(ctx context.Context) {
	if service.ReadModelInvalidator == nil {
		return
	}
	_ = service.ReadModelInvalidator.InvalidateNamespaces(ctx, readmodelcache.AllNamespaces()...)
}

// Payload returns the ws_hub payload equivalent to req.model_dump(mode="json").
func (request Request) Payload() map[string]any {
	timestamp := ""
	if !request.Timestamp.IsZero() {
		timestamp = request.Timestamp.UTC().Format(time.RFC3339Nano)
	}
	return map[string]any{
		"device_id":      request.DeviceID,
		"friend_name":    request.FriendName,
		"friend_id":      request.FriendID,
		"source":         request.Source,
		"timestamp":      timestamp,
		"trace_id":       request.TraceID,
		"tenant_id":      optionalPointerValue(request.TenantID),
		"account_id":     optionalPointerValue(request.AccountID),
		"wework_user_id": optionalPointerValue(request.WeWorkUserID),
	}
}

func (service Service) now() time.Time {
	if service.Now != nil {
		return service.Now()
	}
	return time.Now().UTC()
}

func (service Service) touchConversationFirstMessageAt(ctx context.Context, request Request) {
	store, ok := service.Store.(ConversationTouchStore)
	if !ok || store == nil {
		return
	}
	_ = store.TouchConversationFirstMessageAt(ctx, ConversationTouch{
		DeviceID:       request.DeviceID,
		FriendID:       strings.TrimSpace(request.FriendID),
		FriendName:     request.FriendName,
		FirstMessageAt: request.Timestamp.UTC(),
		TenantID:       optionalTrimmedPointerValue(request.TenantID),
		AccountID:      optionalTrimmedPointerValue(request.AccountID),
		WeWorkUserID:   optionalTrimmedPointerValue(request.WeWorkUserID),
	})
}

func requiredString(raw map[string]json.RawMessage, field string) (string, error) {
	value, ok := raw[field]
	if !ok || isJSONNull(value) {
		return "", fmt.Errorf("%w: %s is required", ErrInvalidPayload, field)
	}
	return decodeString(value, field)
}

func optionalString(raw map[string]json.RawMessage, field string, fallback string) (string, error) {
	value, ok := raw[field]
	if !ok {
		return fallback, nil
	}
	if isJSONNull(value) {
		return "", fmt.Errorf("%w: %s must be a string", ErrInvalidPayload, field)
	}
	return decodeString(value, field)
}

func optionalStringPointer(raw map[string]json.RawMessage, field string) (*string, error) {
	value, ok := raw[field]
	if !ok || isJSONNull(value) {
		return nil, nil
	}
	text, err := decodeString(value, field)
	if err != nil {
		return nil, err
	}
	return &text, nil
}

func requiredTimestamp(raw map[string]json.RawMessage, field string) (time.Time, error) {
	value, ok := raw[field]
	if !ok || isJSONNull(value) {
		return time.Time{}, fmt.Errorf("%w: %s is required", ErrInvalidPayload, field)
	}
	text, err := decodeString(value, field)
	if err != nil {
		return time.Time{}, err
	}
	parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(text))
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: %s must be an RFC3339 timestamp", ErrInvalidPayload, field)
	}
	return parsed.UTC(), nil
}

func decodeString(value json.RawMessage, field string) (string, error) {
	var text string
	if err := json.Unmarshal(value, &text); err != nil {
		return "", fmt.Errorf("%w: %s must be a string", ErrInvalidPayload, field)
	}
	return text, nil
}

func isJSONNull(value json.RawMessage) bool {
	return strings.EqualFold(strings.TrimSpace(string(value)), "null")
}

func optionalPointerValue(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}

func optionalTrimmedPointerValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}
