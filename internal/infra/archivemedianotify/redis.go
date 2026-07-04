// Package archivemedianotify publishes best-effort Redis wakeups for archive media workers.
package archivemedianotify

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/redis/go-redis/v9"

	"wework-go/internal/infra/archivemediatask"
)

const (
	DefaultChannel      = "archive_media:notify"
	DefaultEnterpriseID = "default"
	DefaultSource       = "self_decrypt"
	DefaultReason       = "task-enqueued"
)

// RedisPublisher is the go-redis Publish shape used by Notifier.
type RedisPublisher interface {
	Publish(ctx context.Context, channel string, message any) *redis.IntCmd
}

// Signal is the cross-process archive media wake payload.
type Signal struct {
	EnterpriseID string
	Source       string
	Reason       string
}

// Notifier publishes lightweight archive media wake messages.
type Notifier struct {
	Client  RedisPublisher
	Channel string
}

// New creates a Redis-backed archive media notifier.
func New(client RedisPublisher, channel string) *Notifier {
	return &Notifier{Client: client, Channel: defaultText(channel, DefaultChannel)}
}

// NotifyArchiveMediaEnqueued publishes one wake message per queued media task.
func (notifier *Notifier) NotifyArchiveMediaEnqueued(ctx context.Context, results []archivemediatask.EnqueueResult) error {
	if notifier == nil || notifier.Client == nil || len(results) == 0 {
		return nil
	}
	var publishErr error
	for _, result := range results {
		if strings.TrimSpace(result.Record.TaskID) == "" {
			continue
		}
		signal := SignalFromResult(result)
		if err := notifier.Publish(ctx, signal); err != nil && publishErr == nil {
			publishErr = err
		}
	}
	return publishErr
}

// Publish sends a direct archive media wake signal.
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

// SignalFromResult normalizes one media enqueue result into a Redis wake payload.
func SignalFromResult(result archivemediatask.EnqueueResult) Signal {
	return Signal{
		EnterpriseID: defaultText(result.Record.EnterpriseID, DefaultEnterpriseID),
		Source:       defaultText(result.Record.Source, DefaultSource),
		Reason:       DefaultReason,
	}
}

// PayloadJSON mirrors Python archive media wake payload keys.
func (signal Signal) PayloadJSON() (string, error) {
	payload := map[string]any{
		"enterprise_id": defaultText(signal.EnterpriseID, DefaultEnterpriseID),
		"source":        defaultText(signal.Source, DefaultSource),
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
