// Package taskstatuspublisher adapts send-dispatcher task.status events to a realtime hub.
package taskstatuspublisher

import (
	"context"
	"fmt"
	"strings"

	"wework-go/internal/senddispatcher"
)

// Hub is the minimal realtime publish shape used by the adapter.
type Hub interface {
	Publish(ctx context.Context, channel string, event string, topic string, payload map[string]any) error
}

// Publisher implements senddispatcher.TaskStatusPublisher.
type Publisher struct {
	Hub Hub
}

var _ senddispatcher.TaskStatusPublisher = (*Publisher)(nil)

// New wraps a realtime hub with the task.status publisher adapter.
func New(hub Hub) *Publisher {
	return &Publisher{Hub: hub}
}

// PublishTaskStatus publishes one task.status event through the configured hub.
func (publisher *Publisher) PublishTaskStatus(ctx context.Context, event senddispatcher.TaskStatusEvent) error {
	if publisher == nil || publisher.Hub == nil {
		return fmt.Errorf("task status publisher hub is not configured")
	}
	return publisher.Hub.Publish(
		ctx,
		defaultText(event.Channel, "tasks"),
		defaultText(event.Event, "task.status"),
		defaultText(event.Topic, "task.status"),
		clonePayload(event.Payload),
	)
}

func defaultText(value string, fallback string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback
	}
	return trimmed
}

func clonePayload(input map[string]any) map[string]any {
	if input == nil {
		return map[string]any{}
	}
	output := make(map[string]any, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}
