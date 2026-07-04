// Package archivesyncnotify publishes best-effort Redis wakeups for archive sync workers.
package archivesyncnotify

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/redis/go-redis/v9"

	"wework-go/internal/outbox"
)

const (
	DefaultChannel            = "archive_sync:notify"
	EventArchiveSyncRequested = "archive.sync.requested"
	DefaultEnterpriseID       = "default"
	DefaultSource             = "self_decrypt"
	DefaultReason             = "trigger_pull_signal"
	DefaultDirectSignalReason = "signal"
)

// RedisPublisher is the go-redis Publish shape used by Notifier.
type RedisPublisher interface {
	Publish(ctx context.Context, channel string, message any) *redis.IntCmd
}

// Signal is the cross-process archive sync wake payload.
type Signal struct {
	EnterpriseID string
	Source       string
	Cursor       string
	Reason       string
}

// Notifier publishes lightweight archive sync wake messages.
type Notifier struct {
	Client  RedisPublisher
	Channel string
}

// New creates a Redis-backed archive sync notifier.
func New(client RedisPublisher, channel string) *Notifier {
	return &Notifier{Client: client, Channel: defaultText(channel, DefaultChannel)}
}

// NotifyArchiveSyncRequested publishes one wake message per archive.sync.requested record.
func (notifier *Notifier) NotifyArchiveSyncRequested(ctx context.Context, records []outbox.Record) error {
	if notifier == nil || notifier.Client == nil || len(records) == 0 {
		return nil
	}
	var publishErr error
	for _, record := range records {
		signal, ok := SignalFromRecord(record)
		if !ok {
			continue
		}
		if err := notifier.Publish(ctx, signal); err != nil && publishErr == nil {
			publishErr = err
		}
	}
	return publishErr
}

// Publish sends a direct archive sync wake signal.
func (notifier *Notifier) Publish(ctx context.Context, signal Signal) error {
	if notifier == nil || notifier.Client == nil {
		return nil
	}
	payload, err := signal.PayloadJSON()
	if err != nil {
		return err
	}
	return notifier.Client.Publish(ctx, defaultText(notifier.Channel, DefaultChannel), payload).Err()
}

// SignalFromRecord normalizes an archive.sync.requested outbox record into a Redis wake payload.
func SignalFromRecord(record outbox.Record) (Signal, bool) {
	if strings.TrimSpace(record.EventType) != EventArchiveSyncRequested {
		return Signal{}, false
	}
	payload := record.Payload
	if payload == nil {
		payload = map[string]any{}
	}
	return Signal{
		EnterpriseID: defaultText(textValue(payload["enterprise_id"]), defaultText(record.TenantID, DefaultEnterpriseID)),
		Source:       defaultText(textValue(payload["source"]), DefaultSource),
		Cursor:       strings.TrimSpace(textValue(payload["cursor"])),
		Reason:       defaultText(firstText(payload["reason"], payload["trigger_reason"]), DefaultReason),
	}, true
}

// PayloadJSON mirrors Python archive sync wake payload keys.
func (signal Signal) PayloadJSON() (string, error) {
	payload := map[string]any{
		"enterprise_id": defaultText(signal.EnterpriseID, DefaultEnterpriseID),
		"source":        defaultText(signal.Source, DefaultSource),
		"cursor":        nil,
		"reason":        defaultText(signal.Reason, DefaultDirectSignalReason),
	}
	if cursor := strings.TrimSpace(signal.Cursor); cursor != "" {
		payload["cursor"] = cursor
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func defaultText(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return strings.TrimSpace(fallback)
	}
	return value
}

func firstText(values ...any) string {
	for _, value := range values {
		if text := strings.TrimSpace(textValue(value)); text != "" {
			return text
		}
	}
	return ""
}

func textValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case []byte:
		return string(typed)
	default:
		return fmt.Sprint(typed)
	}
}
