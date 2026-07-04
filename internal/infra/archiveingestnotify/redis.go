// Package archiveingestnotify publishes best-effort Redis wakeups for archive ingest workers.
package archiveingestnotify

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/redis/go-redis/v9"

	"wework-go/internal/infra/archiveingesttask"
)

const (
	DefaultChannel      = "archive_ingest:notify"
	DefaultEnterpriseID = "default"
	DefaultSource       = "self_decrypt"
	DefaultReason       = "task-enqueued"
)

// RedisPublisher is the go-redis Publish shape used by Notifier.
type RedisPublisher interface {
	Publish(ctx context.Context, channel string, message any) *redis.IntCmd
}

// Signal is the cross-process archive ingest wake payload.
type Signal struct {
	EnterpriseID string
	Source       string
	TaskID       string
	Cursor       string
	Reason       string
}

// Notifier publishes lightweight archive ingest wake messages.
type Notifier struct {
	Client  RedisPublisher
	Channel string
}

// New creates a Redis-backed archive ingest notifier.
func New(client RedisPublisher, channel string) *Notifier {
	return &Notifier{Client: client, Channel: defaultText(channel, DefaultChannel)}
}

// NotifyArchiveIngestEnqueued publishes a wake message after an ingest task is queued.
func (notifier *Notifier) NotifyArchiveIngestEnqueued(ctx context.Context, record archiveingesttask.Record) error {
	if notifier == nil || notifier.Client == nil || strings.TrimSpace(record.TaskID) == "" {
		return nil
	}
	return notifier.Publish(ctx, SignalFromRecord(record))
}

// Publish sends a direct archive ingest wake signal.
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

// SignalFromRecord normalizes one staged ingest record into a Redis wake payload.
func SignalFromRecord(record archiveingesttask.Record) Signal {
	return Signal{
		EnterpriseID: defaultText(record.EnterpriseID, DefaultEnterpriseID),
		Source:       defaultText(record.Source, DefaultSource),
		TaskID:       strings.TrimSpace(record.TaskID),
		Cursor:       strings.TrimSpace(record.Cursor),
		Reason:       DefaultReason,
	}
}

// PayloadJSON mirrors the lightweight worker wake payload keys.
func (signal Signal) PayloadJSON() (string, error) {
	payload := map[string]any{
		"enterprise_id": defaultText(signal.EnterpriseID, DefaultEnterpriseID),
		"source":        defaultText(signal.Source, DefaultSource),
		"task_id":       strings.TrimSpace(signal.TaskID),
		"cursor":        strings.TrimSpace(signal.Cursor),
		"reason":        defaultText(signal.Reason, DefaultReason),
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
