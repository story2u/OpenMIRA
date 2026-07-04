package outboxrelay

import (
	"context"
	"fmt"
	"strings"
	"time"

	"wework-go/internal/outbox"
)

// ClaimOptions controls the relay claim call.
type ClaimOptions struct {
	Limit             int
	IncludeEventTypes []string
	ExcludeEventTypes []string
}

// ClaimFunc claims one batch of events from the durable outbox store.
type ClaimFunc func(ctx context.Context, options ClaimOptions) ([]outbox.Record, error)

// Dispatcher consumes one outbox event.
type Dispatcher func(ctx context.Context, event outbox.Record) error

// PartitionDispatcher consumes one serial partition as a batch.
type PartitionDispatcher func(ctx context.Context, partitionKey string, events []outbox.Record) error

// MarkPublishedManyFunc marks many events as published.
type MarkPublishedManyFunc func(ctx context.Context, eventIDs []string) (int64, error)

// MarkPublishedFunc marks one event as published.
type MarkPublishedFunc func(ctx context.Context, eventID string) error

// MarkRetryFunc marks one event for retry.
type MarkRetryFunc func(ctx context.Context, eventID string, errText string, retryDelaySeconds float64) error

// Service performs caller-controlled outbox relay ticks.
type Service struct {
	Claim               ClaimFunc
	Dispatch            Dispatcher
	DispatchPartition   PartitionDispatcher
	MarkPublishedMany   MarkPublishedManyFunc
	MarkPublished       MarkPublishedFunc
	MarkRetry           MarkRetryFunc
	Options             Options
	IncludeEventTypes   []string
	ExcludeEventTypes   []string
	ProcessedTotal      int
	LastProcessedAt     time.Time
	LastError           string
	ProcessingLeaseSecs int
}

// FlushOnce claims one batch and drains it through the configured dispatcher.
func (service *Service) FlushOnce(ctx context.Context, limit int) (int, error) {
	if service.Claim == nil {
		return 0, fmt.Errorf("outbox relay claim function is not configured")
	}
	batchSize := limit
	if batchSize <= 0 {
		batchSize = service.options().BatchSize
	}
	events, err := service.Claim(ctx, ClaimOptions{
		Limit:             batchSize,
		IncludeEventTypes: service.IncludeEventTypes,
		ExcludeEventTypes: service.ExcludeEventTypes,
	})
	if err != nil {
		service.LastError = err.Error()
		return 0, err
	}
	if len(events) == 0 {
		return 0, nil
	}
	processed := 0
	for _, partition := range GroupByPartition(events) {
		count, err := service.drainPartition(ctx, partition)
		if err != nil {
			return processed, err
		}
		processed += count
	}
	return processed, nil
}

func (service *Service) drainPartition(ctx context.Context, partition Partition) (int, error) {
	if service.DispatchPartition != nil {
		if err := service.DispatchPartition(ctx, partition.Key, partition.Events); err != nil {
			if retryErr := service.retryEvents(ctx, partition.Events, err); retryErr != nil {
				return 0, retryErr
			}
			service.LastError = err.Error()
			return 0, nil
		}
		if err := service.flushPublished(ctx, eventIDs(partition.Events)); err != nil {
			return 0, err
		}
		service.markProcessed(len(partition.Events))
		return len(partition.Events), nil
	}
	if service.Dispatch == nil {
		return 0, fmt.Errorf("outbox relay dispatcher is not configured")
	}
	processed := 0
	publishedBatch := []string{}
	for _, event := range partition.Events {
		if err := service.Dispatch(ctx, event); err != nil {
			if len(publishedBatch) > 0 {
				if flushErr := service.flushPublished(ctx, publishedBatch); flushErr != nil {
					return processed, flushErr
				}
				publishedBatch = []string{}
			}
			if retryErr := service.retryEvents(ctx, []outbox.Record{event}, err); retryErr != nil {
				return processed, retryErr
			}
			service.LastError = err.Error()
			continue
		}
		publishedBatch = append(publishedBatch, strings.TrimSpace(event.EventID))
		if len(publishedBatch) >= AckBatchSize {
			if err := service.flushPublished(ctx, publishedBatch); err != nil {
				return processed, err
			}
			publishedBatch = []string{}
		}
		service.markProcessed(1)
		processed++
	}
	if len(publishedBatch) > 0 {
		if err := service.flushPublished(ctx, publishedBatch); err != nil {
			return processed, err
		}
	}
	return processed, nil
}

func (service *Service) flushPublished(ctx context.Context, eventIDs []string) error {
	normalized := normalizePublishedIDs(eventIDs, false)
	if len(normalized) == 0 {
		return nil
	}
	if service.MarkPublishedMany != nil {
		_, err := service.MarkPublishedMany(ctx, normalized)
		if err == nil {
			return nil
		}
	}
	if service.MarkPublished == nil {
		return fmt.Errorf("outbox relay published marker is not configured")
	}
	for _, eventID := range normalized {
		if err := service.MarkPublished(ctx, eventID); err != nil {
			return err
		}
	}
	return nil
}

func (service *Service) retryEvents(ctx context.Context, events []outbox.Record, cause error) error {
	if service.MarkRetry == nil {
		return fmt.Errorf("outbox relay retry marker is not configured")
	}
	for _, action := range RetryActions(events, cause, service.options().RetryBaseSec) {
		if err := service.MarkRetry(ctx, action.EventID, action.Error, action.RetryDelaySeconds); err != nil {
			return err
		}
	}
	return nil
}

func (service *Service) markProcessed(count int) {
	service.ProcessedTotal += count
	service.LastProcessedAt = time.Now().UTC()
	service.LastError = ""
}

func (service *Service) options() Options {
	options := service.Options
	defaults := ResolveOptions(nil)
	if options.BatchSize <= 0 {
		options.BatchSize = defaults.BatchSize
	}
	if options.RetryBaseSec <= 0 {
		options.RetryBaseSec = defaults.RetryBaseSec
	}
	return options
}

func eventIDs(events []outbox.Record) []string {
	ids := make([]string, 0, len(events))
	for _, event := range events {
		ids = append(ids, strings.TrimSpace(event.EventID))
	}
	return ids
}
