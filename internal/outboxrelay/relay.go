// Package outboxrelay contains pure outbox relay orchestration rules.
package outboxrelay

import (
	"math"
	"strconv"
	"strings"

	"wework-go/internal/outbox"
)

const (
	DefaultBatchSize            = 500
	DefaultPartitionConcurrency = 16
	DefaultPollIntervalSec      = 0.1
	DefaultClaimWorkers         = 2
	DefaultFinalizeChunkSize    = 100
	DefaultRetryBaseSec         = 2.0
	DefaultPruneIntervalSec     = 3600.0
	DefaultPublishedRetention   = 3
	DefaultPruneBatchSize       = 5000
	DefaultNotifyChannel        = "outbox:notify"
	AckBatchSize                = 20
)

// Options captures Python OutboxRelayService env/default parsing.
type Options struct {
	BatchSize            int
	PartitionConcurrency int
	PollIntervalSec      float64
	FinalizeChunkSize    int
	ClaimWorkerCount     int
	RetryBaseSec         float64
	PruneIntervalSec     float64
	PublishedRetention   int
	PruneBatchSize       int
	NotifyChannel        string
	RedisNotifyEnabled   bool
}

// Partition is one serial relay partition.
type Partition struct {
	Key    string
	Events []outbox.Record
}

// RetryAction describes one mark_retry call after dispatch failure.
type RetryAction struct {
	EventID           string
	Error             string
	RetryDelaySeconds float64
}

// ResolveOptions mirrors OutboxRelayService constructor env parsing.
func ResolveOptions(env map[string]string) Options {
	return Options{
		BatchSize:            positiveInt(envText(env, "CLOUD_OUTBOX_RELAY_BATCH_SIZE", strconv.Itoa(DefaultBatchSize)), DefaultBatchSize),
		PartitionConcurrency: positiveInt(envText(env, "CLOUD_OUTBOX_RELAY_PARTITION_CONCURRENCY", strconv.Itoa(DefaultPartitionConcurrency)), DefaultPartitionConcurrency),
		PollIntervalSec:      minFloat(floatValue(envText(env, "CLOUD_OUTBOX_RELAY_POLL_INTERVAL_SEC", formatFloat(DefaultPollIntervalSec)), DefaultPollIntervalSec), 0.05),
		FinalizeChunkSize:    positiveInt(envText(env, "CLOUD_OUTBOX_RELAY_FINALIZE_CHUNK_SIZE", strconv.Itoa(DefaultFinalizeChunkSize)), DefaultFinalizeChunkSize),
		ClaimWorkerCount:     positiveInt(envText(env, "CLOUD_OUTBOX_RELAY_CLAIM_WORKERS", strconv.Itoa(DefaultClaimWorkers)), DefaultClaimWorkers),
		RetryBaseSec:         minFloat(floatValue(envText(env, "CLOUD_OUTBOX_RELAY_RETRY_BASE_SEC", formatFloat(DefaultRetryBaseSec)), DefaultRetryBaseSec), 0.2),
		PruneIntervalSec:     minFloat(floatValue(envText(env, "CLOUD_OUTBOX_PRUNE_INTERVAL_SEC", formatFloat(DefaultPruneIntervalSec)), DefaultPruneIntervalSec), 60.0),
		PublishedRetention:   positiveInt(envText(env, "CLOUD_OUTBOX_PUBLISHED_RETENTION_DAYS", strconv.Itoa(DefaultPublishedRetention)), DefaultPublishedRetention),
		PruneBatchSize:       positiveInt(envText(env, "CLOUD_OUTBOX_PRUNE_BATCH_SIZE", strconv.Itoa(DefaultPruneBatchSize)), DefaultPruneBatchSize),
		NotifyChannel:        envText(env, "CLOUD_OUTBOX_NOTIFY_CHANNEL", DefaultNotifyChannel),
		RedisNotifyEnabled:   boolEnabled(envText(env, "CLOUD_REDIS_OUTBOX_NOTIFY_ENABLED", "true")),
	}
}

// ResolvePartitionKey mirrors OutboxRelayService._resolve_partition_key.
func ResolvePartitionKey(item outbox.Record) string {
	if value := strings.TrimSpace(item.PartitionKey); value != "" {
		return value
	}
	if value := strings.TrimSpace(item.AggregateID); value != "" {
		return value
	}
	return strings.TrimSpace(item.EventID)
}

// GroupByPartition groups records while preserving first-seen partition order.
func GroupByPartition(events []outbox.Record) []Partition {
	positions := map[string]int{}
	partitions := []Partition{}
	for _, item := range events {
		key := ResolvePartitionKey(item)
		position, ok := positions[key]
		if !ok {
			positions[key] = len(partitions)
			partitions = append(partitions, Partition{Key: key})
			position = len(partitions) - 1
		}
		partitions[position].Events = append(partitions[position].Events, item)
	}
	return partitions
}

// RelayChunks mirrors _iter_relay_chunks.
func RelayChunks(entries []string, chunkSize int) [][]string {
	if chunkSize < 1 {
		chunkSize = 1
	}
	chunks := [][]string{}
	for index := 0; index < len(entries); index += chunkSize {
		end := index + chunkSize
		if end > len(entries) {
			end = len(entries)
		}
		chunk := append([]string(nil), entries[index:end]...)
		chunks = append(chunks, chunk)
	}
	return chunks
}

// PublishedAckBatches builds per-dispatcher ack batches without deduplication.
func PublishedAckBatches(events []outbox.Record, ackBatchSize int) [][]string {
	if ackBatchSize < 1 {
		ackBatchSize = 1
	}
	batches := [][]string{}
	current := []string{}
	for _, item := range events {
		current = append(current, strings.TrimSpace(item.EventID))
		if len(current) >= ackBatchSize {
			if normalized := normalizePublishedIDs(current, false); len(normalized) > 0 {
				batches = append(batches, normalized)
			}
			current = []string{}
		}
	}
	if normalized := normalizePublishedIDs(current, false); len(normalized) > 0 {
		batches = append(batches, normalized)
	}
	return batches
}

// FinalizePublishedChunks deduplicates batch-context ids and chunks mark_published_many calls.
func FinalizePublishedChunks(eventIDs []string, chunkSize int) [][]string {
	return RelayChunks(normalizePublishedIDs(eventIDs, true), chunkSize)
}

// RetryDelaySeconds mirrors retry_base_sec * 2**max(0, attempt_count).
func RetryDelaySeconds(retryBaseSec float64, attemptCount int) float64 {
	if retryBaseSec < 0.2 {
		retryBaseSec = 0.2
	}
	if attemptCount < 0 {
		attemptCount = 0
	}
	return retryBaseSec * math.Pow(2, float64(attemptCount))
}

// RetryActions builds mark_retry calls for a failed partition or event.
func RetryActions(events []outbox.Record, err error, retryBaseSec float64) []RetryAction {
	actions := make([]RetryAction, 0, len(events))
	errorText := ""
	if err != nil {
		errorText = err.Error()
	}
	for _, item := range events {
		actions = append(actions, RetryAction{
			EventID:           strings.TrimSpace(item.EventID),
			Error:             errorText,
			RetryDelaySeconds: RetryDelaySeconds(retryBaseSec, item.AttemptCount),
		})
	}
	return actions
}

func normalizePublishedIDs(eventIDs []string, dedupe bool) []string {
	normalized := []string{}
	seen := map[string]struct{}{}
	for _, eventID := range eventIDs {
		value := strings.TrimSpace(eventID)
		if value == "" {
			continue
		}
		if dedupe {
			if _, ok := seen[value]; ok {
				continue
			}
			seen[value] = struct{}{}
		}
		normalized = append(normalized, value)
	}
	return normalized
}

func envText(env map[string]string, key string, fallback string) string {
	if env == nil {
		return fallback
	}
	value := strings.TrimSpace(env[key])
	if value == "" {
		return fallback
	}
	return value
}

func positiveInt(value string, fallback int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed < 1 {
		return fallback
	}
	return parsed
}

func floatValue(value string, fallback float64) float64 {
	parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func minFloat(value float64, minValue float64) float64 {
	if value < minValue {
		return minValue
	}
	return value
}

func boolEnabled(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "0", "false", "no", "off":
		return false
	default:
		return true
	}
}

func formatFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}
