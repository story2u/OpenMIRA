// Package voicetranscriptionnotify publishes best-effort Redis wakeups for voice transcription workers.
package voicetranscriptionnotify

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/redis/go-redis/v9"

	"wework-go/internal/infra/voicetranscriptiontask"
)

const (
	DefaultChannel      = "voice_transcription:notify"
	DefaultEnterpriseID = "default"
	DefaultReason       = "task-enqueued"
)

// RedisPublisher is the go-redis Publish shape used by Notifier.
type RedisPublisher interface {
	Publish(ctx context.Context, channel string, message any) *redis.IntCmd
}

// Signal is the cross-process voice transcription wake payload.
type Signal struct {
	EnterpriseID string
	TaskID       string
	ArchiveMsgID string
	Reason       string
}

// Notifier publishes lightweight voice transcription wake messages.
type Notifier struct {
	Client  RedisPublisher
	Channel string
}

// New creates a Redis-backed voice transcription notifier.
func New(client RedisPublisher, channel string) *Notifier {
	return &Notifier{Client: client, Channel: defaultText(channel, DefaultChannel)}
}

// NotifyVoiceTranscriptionEnqueued publishes a wake message after a voice transcription task is queued.
func (notifier *Notifier) NotifyVoiceTranscriptionEnqueued(ctx context.Context, result voicetranscriptiontask.EnqueueResult) error {
	if notifier == nil || notifier.Client == nil || strings.TrimSpace(result.TaskID) == "" {
		return nil
	}
	return notifier.Publish(ctx, SignalFromResult(result))
}

// Publish sends a direct voice transcription wake signal.
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

// SignalFromResult normalizes one enqueue result into a Redis wake payload.
func SignalFromResult(result voicetranscriptiontask.EnqueueResult) Signal {
	return Signal{
		EnterpriseID: defaultText(result.EnterpriseID, DefaultEnterpriseID),
		TaskID:       strings.TrimSpace(result.TaskID),
		ArchiveMsgID: strings.TrimSpace(result.ArchiveMsgID),
		Reason:       DefaultReason,
	}
}

// PayloadJSON mirrors the lightweight worker wake payload keys.
func (signal Signal) PayloadJSON() (string, error) {
	payload := map[string]any{
		"enterprise_id": defaultText(signal.EnterpriseID, DefaultEnterpriseID),
		"task_id":       strings.TrimSpace(signal.TaskID),
		"archive_msgid": strings.TrimSpace(signal.ArchiveMsgID),
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
