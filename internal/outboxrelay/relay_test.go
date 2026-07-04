package outboxrelay

import (
	"errors"
	"reflect"
	"testing"

	"wework-go/internal/outbox"
)

func TestResolveOptionsMirrorsPythonDefaults(t *testing.T) {
	options := ResolveOptions(nil)
	if options.BatchSize != 500 || options.PartitionConcurrency != 16 || options.ClaimWorkerCount != 2 {
		t.Fatalf("options = %#v", options)
	}
	if options.PollIntervalSec != 0.1 || options.RetryBaseSec != 2 || options.PruneIntervalSec != 3600 {
		t.Fatalf("timing options = %#v", options)
	}
	if options.FinalizeChunkSize != 100 || options.PublishedRetention != 3 || options.PruneBatchSize != 5000 {
		t.Fatalf("maintenance options = %#v", options)
	}
	if options.NotifyChannel != "outbox:notify" || !options.RedisNotifyEnabled {
		t.Fatalf("notify options = %#v", options)
	}
}

func TestResolveOptionsClampsPythonMinimums(t *testing.T) {
	options := ResolveOptions(map[string]string{
		"CLOUD_OUTBOX_RELAY_BATCH_SIZE":            "0",
		"CLOUD_OUTBOX_RELAY_PARTITION_CONCURRENCY": "bad",
		"CLOUD_OUTBOX_RELAY_POLL_INTERVAL_SEC":     "0.01",
		"CLOUD_OUTBOX_RELAY_RETRY_BASE_SEC":        "0.1",
		"CLOUD_OUTBOX_PRUNE_INTERVAL_SEC":          "10",
		"CLOUD_OUTBOX_NOTIFY_CHANNEL":              " custom:notify ",
		"CLOUD_REDIS_OUTBOX_NOTIFY_ENABLED":        "off",
	})
	if options.BatchSize != 500 || options.PartitionConcurrency != 16 {
		t.Fatalf("integer fallbacks = %#v", options)
	}
	if options.PollIntervalSec != 0.05 || options.RetryBaseSec != 0.2 || options.PruneIntervalSec != 60 {
		t.Fatalf("minimums = %#v", options)
	}
	if options.NotifyChannel != "custom:notify" || options.RedisNotifyEnabled {
		t.Fatalf("notify options = %#v", options)
	}
}

func TestGroupByPartitionPreservesFirstSeenOrder(t *testing.T) {
	events := []outbox.Record{
		record("evt-1", "agg-1", "partition-a", 0),
		record("evt-2", "agg-2", "", 0),
		record("evt-3", "", "", 0),
		record("evt-4", "agg-4", "partition-a", 0),
	}

	partitions := GroupByPartition(events)
	if len(partitions) != 3 {
		t.Fatalf("partitions = %#v", partitions)
	}
	if partitions[0].Key != "partition-a" || partitions[1].Key != "agg-2" || partitions[2].Key != "evt-3" {
		t.Fatalf("partition keys = %#v", partitions)
	}
	if len(partitions[0].Events) != 2 || partitions[0].Events[1].EventID != "evt-4" {
		t.Fatalf("partition-a events = %#v", partitions[0].Events)
	}
}

func TestPublishedAckBatchesDoNotDeduplicate(t *testing.T) {
	events := []outbox.Record{}
	for index := 0; index < 21; index++ {
		events = append(events, record("evt-"+string(rune('a'+index)), "", "", 0))
	}
	events[5].EventID = " evt-a "
	events[6].EventID = ""

	batches := PublishedAckBatches(events, AckBatchSize)
	if len(batches) != 2 || len(batches[0]) != 19 || len(batches[1]) != 1 {
		t.Fatalf("batches = %#v", batches)
	}
	if batches[0][0] != "evt-a" || batches[0][5] != "evt-a" {
		t.Fatalf("duplicates should be preserved: %#v", batches[0])
	}
}

func TestFinalizePublishedChunksDeduplicatesAndChunks(t *testing.T) {
	chunks := FinalizePublishedChunks([]string{" evt-1 ", "evt-2", "evt-1", "", "evt-3"}, 2)
	want := [][]string{{"evt-1", "evt-2"}, {"evt-3"}}
	if !reflect.DeepEqual(chunks, want) {
		t.Fatalf("chunks = %#v want %#v", chunks, want)
	}
}

func TestRelayChunksClampsChunkSize(t *testing.T) {
	chunks := RelayChunks([]string{"a", "b"}, 0)
	want := [][]string{{"a"}, {"b"}}
	if !reflect.DeepEqual(chunks, want) {
		t.Fatalf("chunks = %#v", chunks)
	}
}

func TestRetryActionsUseAttemptExponent(t *testing.T) {
	actions := RetryActions([]outbox.Record{
		record("evt-1", "", "", 0),
		record("evt-2", "", "", 3),
	}, errors.New("boom"), 2)

	if len(actions) != 2 || actions[0].RetryDelaySeconds != 2 || actions[1].RetryDelaySeconds != 16 {
		t.Fatalf("actions = %#v", actions)
	}
	if actions[0].Error != "boom" || actions[1].EventID != "evt-2" {
		t.Fatalf("actions = %#v", actions)
	}
	if got := RetryDelaySeconds(0.1, -1); got != 0.2 {
		t.Fatalf("RetryDelaySeconds = %v", got)
	}
}

func record(eventID string, aggregateID string, partitionKey string, attemptCount int) outbox.Record {
	return outbox.Record{
		EventEnvelope: outbox.EventEnvelope{
			EventID:      eventID,
			AggregateID:  aggregateID,
			PartitionKey: partitionKey,
		},
		AttemptCount: attemptCount,
	}
}
