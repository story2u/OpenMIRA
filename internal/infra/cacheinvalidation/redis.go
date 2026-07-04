// Package cacheinvalidation adapts Go write candidates to the legacy shared
// cache namespace invalidation protocol.
package cacheinvalidation

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/redis/go-redis/v9"
)

const (
	DefaultPrefix  = "wework"
	DefaultChannel = "cache:invalidate"
)

// RedisClient is the go-redis command subset needed for namespace invalidation.
type RedisClient interface {
	Incr(ctx context.Context, key string) *redis.IntCmd
	Publish(ctx context.Context, channel string, message any) *redis.IntCmd
}

// Options configures the shared cache namespace protocol.
type Options struct {
	Prefix  string
	Channel string
}

// Invalidator bumps namespace versions and broadcasts one invalidation payload.
type Invalidator struct {
	Client  RedisClient
	Prefix  string
	Channel string
}

// New wraps a Redis client with Python-compatible shared cache defaults.
func New(client RedisClient, options Options) *Invalidator {
	return &Invalidator{
		Client:  client,
		Prefix:  defaultText(options.Prefix, DefaultPrefix),
		Channel: defaultText(options.Channel, DefaultChannel),
	}
}

// InvalidateNamespaces bumps wework:version:{namespace} and publishes
// {"namespaces":[...]} to the shared cache invalidation channel.
func (invalidator *Invalidator) InvalidateNamespaces(ctx context.Context, namespaces ...string) error {
	if invalidator == nil || invalidator.Client == nil {
		return nil
	}
	normalized := normalizeNamespaces(namespaces)
	if len(normalized) == 0 {
		return nil
	}
	for _, namespace := range normalized {
		if err := invalidator.Client.Incr(ctx, invalidator.versionKey(namespace)).Err(); err != nil {
			return err
		}
	}
	payload, err := json.Marshal(map[string][]string{"namespaces": normalized})
	if err != nil {
		return err
	}
	return invalidator.Client.Publish(ctx, defaultText(invalidator.Channel, DefaultChannel), string(payload)).Err()
}

func (invalidator *Invalidator) versionKey(namespace string) string {
	return strings.TrimRight(defaultText(invalidator.Prefix, DefaultPrefix), ":") + ":version:" + namespace
}

func normalizeNamespaces(namespaces []string) []string {
	seen := map[string]bool{}
	normalized := make([]string, 0, len(namespaces))
	for _, namespace := range namespaces {
		value := strings.TrimSpace(namespace)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		normalized = append(normalized, value)
	}
	return normalized
}

func defaultText(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}
