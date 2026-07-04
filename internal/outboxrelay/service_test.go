package outboxrelay

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"wework-go/internal/outbox"
)

func TestFlushOnceDispatchesEventsAndBatchesPublishedMarks(t *testing.T) {
	store := &fakeStore{claimed: makeEvents(21)}
	service := &Service{
		Claim:             store.claim,
		Dispatch:          func(context.Context, outbox.Record) error { return nil },
		MarkPublishedMany: store.markPublishedMany,
		MarkRetry:         store.markRetry,
		Options:           Options{BatchSize: 50, RetryBaseSec: 2},
	}

	processed, err := service.FlushOnce(context.Background(), 0)
	if err != nil {
		t.Fatalf("FlushOnce returned error: %v", err)
	}
	if processed != 21 || service.ProcessedTotal != 21 || service.LastError != "" {
		t.Fatalf("processed=%d service=%#v", processed, service)
	}
	if len(store.publishedMany) != 2 || len(store.publishedMany[0]) != 20 || len(store.publishedMany[1]) != 1 {
		t.Fatalf("published batches = %#v", store.publishedMany)
	}
	if store.claimOptions.Limit != 50 {
		t.Fatalf("claim options = %#v", store.claimOptions)
	}
}

func TestFlushOnceFlushesSuccessesBeforeRetryAndContinues(t *testing.T) {
	store := &fakeStore{claimed: []outbox.Record{
		event("evt-1", "partition-a", 0),
		event("evt-2", "partition-a", 2),
		event("evt-3", "partition-a", 0),
	}}
	service := &Service{
		Claim: store.claim,
		Dispatch: func(ctx context.Context, event outbox.Record) error {
			if event.EventID == "evt-2" {
				return errors.New("boom")
			}
			return nil
		},
		MarkPublishedMany: store.markPublishedMany,
		MarkRetry:         store.markRetry,
		Options:           Options{BatchSize: 10, RetryBaseSec: 2},
	}

	processed, err := service.FlushOnce(context.Background(), 0)
	if err != nil {
		t.Fatalf("FlushOnce returned error: %v", err)
	}
	if processed != 2 || service.ProcessedTotal != 2 || service.LastError != "" {
		t.Fatalf("processed=%d service=%#v", processed, service)
	}
	if !reflect.DeepEqual(store.publishedMany, [][]string{{"evt-1"}, {"evt-3"}}) {
		t.Fatalf("publishedMany = %#v", store.publishedMany)
	}
	if len(store.retries) != 1 || store.retries[0].eventID != "evt-2" || store.retries[0].delay != 8 {
		t.Fatalf("retries = %#v", store.retries)
	}
}

func TestFlushOncePartitionDispatcherMarksWholePartition(t *testing.T) {
	store := &fakeStore{claimed: []outbox.Record{
		event("evt-1", "partition-a", 0),
		event("evt-2", "partition-a", 0),
		event("evt-3", "partition-b", 0),
	}}
	partitions := []string{}
	service := &Service{
		Claim: store.claim,
		DispatchPartition: func(ctx context.Context, partitionKey string, events []outbox.Record) error {
			partitions = append(partitions, partitionKey)
			return nil
		},
		MarkPublishedMany: store.markPublishedMany,
		MarkRetry:         store.markRetry,
		Options:           Options{BatchSize: 10, RetryBaseSec: 2},
	}

	processed, err := service.FlushOnce(context.Background(), 0)
	if err != nil {
		t.Fatalf("FlushOnce returned error: %v", err)
	}
	if processed != 3 || !reflect.DeepEqual(partitions, []string{"partition-a", "partition-b"}) {
		t.Fatalf("processed=%d partitions=%#v", processed, partitions)
	}
	if !reflect.DeepEqual(store.publishedMany, [][]string{{"evt-1", "evt-2"}, {"evt-3"}}) {
		t.Fatalf("publishedMany = %#v", store.publishedMany)
	}
}

func TestFlushOncePartitionDispatcherRetriesWholePartition(t *testing.T) {
	store := &fakeStore{claimed: []outbox.Record{
		event("evt-1", "partition-a", 0),
		event("evt-2", "partition-a", 1),
	}}
	service := &Service{
		Claim: store.claim,
		DispatchPartition: func(context.Context, string, []outbox.Record) error {
			return errors.New("partition failed")
		},
		MarkPublishedMany: store.markPublishedMany,
		MarkRetry:         store.markRetry,
		Options:           Options{BatchSize: 10, RetryBaseSec: 2},
	}

	processed, err := service.FlushOnce(context.Background(), 0)
	if err != nil {
		t.Fatalf("FlushOnce returned error: %v", err)
	}
	if processed != 0 || service.LastError != "partition failed" {
		t.Fatalf("processed=%d last_error=%q", processed, service.LastError)
	}
	if len(store.retries) != 2 || store.retries[0].delay != 2 || store.retries[1].delay != 4 {
		t.Fatalf("retries = %#v", store.retries)
	}
	if len(store.publishedMany) != 0 {
		t.Fatalf("publishedMany = %#v", store.publishedMany)
	}
}

type fakeStore struct {
	claimed       []outbox.Record
	claimOptions  ClaimOptions
	publishedMany [][]string
	retries       []retryCall
}

type retryCall struct {
	eventID string
	errText string
	delay   float64
}

func (store *fakeStore) claim(ctx context.Context, options ClaimOptions) ([]outbox.Record, error) {
	store.claimOptions = options
	return store.claimed, nil
}

func (store *fakeStore) markPublishedMany(ctx context.Context, eventIDs []string) (int64, error) {
	store.publishedMany = append(store.publishedMany, append([]string(nil), eventIDs...))
	return int64(len(eventIDs)), nil
}

func (store *fakeStore) markRetry(ctx context.Context, eventID string, errText string, delay float64) error {
	store.retries = append(store.retries, retryCall{eventID: eventID, errText: errText, delay: delay})
	return nil
}

func makeEvents(count int) []outbox.Record {
	events := make([]outbox.Record, 0, count)
	for index := 0; index < count; index++ {
		events = append(events, event("evt-"+string(rune('a'+index)), "partition-a", 0))
	}
	return events
}

func event(eventID string, partition string, attempt int) outbox.Record {
	return outbox.Record{
		EventEnvelope: outbox.EventEnvelope{
			EventID:      eventID,
			PartitionKey: partition,
		},
		AttemptCount: attempt,
	}
}
