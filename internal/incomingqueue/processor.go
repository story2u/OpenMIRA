package incomingqueue

import (
	"context"
	"fmt"
	"strings"
)

// Handler processes one incoming event payload.
type Handler func(ctx context.Context, payload map[string]any) error

// QueueWriter is the queue side-effect boundary used by Processor.
type QueueWriter interface {
	Enqueue(ctx context.Context, payload map[string]any, newID func() string) (string, map[string]any, error)
	EnqueueDLQ(ctx context.Context, payload map[string]any) (string, error)
	Ack(ctx context.Context, ids ...string) error
}

// Processor dispatches decoded stream messages to registered handlers.
type Processor struct {
	Queue       QueueWriter
	MaxRetries  int
	NewID       func() string
	Default     Handler
	Handlers    map[string][]Handler
	NoAck       bool
	LastError   string
	Processed   int
	Retried     int
	DeadLetters int
	ByType      map[string]int
}

// Register adds one handler for an event type.
func (processor *Processor) Register(eventType string, handler Handler) {
	normalized := strings.TrimSpace(eventType)
	if normalized == "" || handler == nil {
		return
	}
	if processor.Handlers == nil {
		processor.Handlers = map[string][]Handler{}
	}
	processor.Handlers[normalized] = append(processor.Handlers[normalized], handler)
}

// ProcessMessage handles one decoded Redis Stream message and acknowledges it after retry/DLQ decisions.
func (processor *Processor) ProcessMessage(ctx context.Context, message Message) (ProcessResult, error) {
	payload := cloneMap(message.Payload)
	eventType := strings.TrimSpace(textValue(payload["event_type"]))
	if eventType == "" {
		eventType = EventTypeDeviceMessageIncoming
	}
	result := ProcessResult{MessageID: strings.TrimSpace(message.ID), EventType: eventType}
	err := processor.dispatch(ctx, eventType, payload)
	if err != nil {
		processor.LastError = err.Error()
		decision := BuildFailureDecision(payload, err, processor.maxRetries())
		if decision.Retry {
			result.Retried = true
			processor.Retried++
			if processor.Queue != nil {
				if _, _, enqueueErr := processor.Queue.Enqueue(ctx, decision.Payload, processor.NewID); enqueueErr != nil {
					return result, enqueueErr
				}
			}
		}
		if decision.DeadLetter {
			result.DeadLettered = true
			processor.DeadLetters++
			if processor.Queue != nil {
				if _, dlqErr := processor.Queue.EnqueueDLQ(ctx, decision.Payload); dlqErr != nil {
					return result, dlqErr
				}
			}
		}
	} else {
		processor.LastError = ""
		processor.Processed++
		if processor.ByType == nil {
			processor.ByType = map[string]int{}
		}
		processor.ByType[eventType]++
	}
	if !processor.NoAck && result.MessageID != "" && processor.Queue != nil {
		if ackErr := processor.Queue.Ack(ctx, result.MessageID); ackErr != nil {
			return result, ackErr
		}
		result.Acked = true
	}
	return result, nil
}

func (processor *Processor) dispatch(ctx context.Context, eventType string, payload map[string]any) error {
	handlers := processor.Handlers[eventType]
	if len(handlers) == 0 && processor.Default != nil {
		handlers = []Handler{processor.Default}
	}
	if len(handlers) == 0 {
		return nil
	}
	for _, handler := range handlers {
		if handler == nil {
			continue
		}
		if err := handler(ctx, payload); err != nil {
			return err
		}
	}
	return nil
}

func (processor *Processor) maxRetries() int {
	if processor.MaxRetries < 0 {
		return DefaultMaxRetries
	}
	return processor.MaxRetries
}

// ProcessResult reports side effects for one message.
type ProcessResult struct {
	MessageID    string
	EventType    string
	Acked        bool
	Retried      bool
	DeadLettered bool
}

// ErrNoQueue is returned by callers that require queue side effects.
var ErrNoQueue = fmt.Errorf("incoming queue writer is not configured")
