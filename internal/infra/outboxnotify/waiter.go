package outboxnotify

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisSubscriber is the go-redis Subscribe shape used by Waiter.
type RedisSubscriber interface {
	Subscribe(ctx context.Context, channels ...string) PubSub
}

// RedisSubscribeClient is the concrete go-redis Subscribe shape.
type RedisSubscribeClient interface {
	Subscribe(ctx context.Context, channels ...string) *redis.PubSub
}

// PubSub is the small Redis Pub/Sub surface needed by Waiter.
type PubSub interface {
	ReceiveMessage(ctx context.Context) (*redis.Message, error)
	Close() error
}

// Waiter waits for outbox wake messages with timeout fallback.
type Waiter struct {
	Client   RedisSubscriber
	Channel  string
	Channels []string

	mu     sync.Mutex
	pubsub PubSub
}

// NewWaiter creates a Redis-backed outbox wake waiter.
func NewWaiter(client RedisSubscriber, channel string) *Waiter {
	return &Waiter{Client: client, Channel: defaultText(channel, DefaultChannel)}
}

// NewMultiWaiter creates a Redis-backed waiter subscribed to all channels.
func NewMultiWaiter(client RedisSubscriber, channels []string) *Waiter {
	return &Waiter{Client: client, Channels: normalizeChannels(channels)}
}

// NewRedisWaiter adapts a go-redis client to Waiter.
func NewRedisWaiter(client RedisSubscribeClient, channel string) *Waiter {
	if client == nil {
		return NewWaiter(nil, channel)
	}
	return NewWaiter(redisSubscriber{Client: client}, channel)
}

// NewRedisMultiWaiter adapts a go-redis client to a multi-channel Waiter.
func NewRedisMultiWaiter(client RedisSubscribeClient, channels []string) *Waiter {
	if client == nil {
		return NewMultiWaiter(nil, channels)
	}
	return NewMultiWaiter(redisSubscriber{Client: client}, channels)
}

type redisSubscriber struct {
	Client RedisSubscribeClient
}

func (subscriber redisSubscriber) Subscribe(ctx context.Context, channels ...string) PubSub {
	return subscriber.Client.Subscribe(ctx, channels...)
}

// Wait blocks until a wake message arrives, the timeout expires, or the context is cancelled.
func (waiter *Waiter) Wait(ctx context.Context, timeout time.Duration) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if waiter == nil || waiter.Client == nil {
		return sleepContext(ctx, timeout)
	}
	pubsub := waiter.ensurePubSub(ctx)
	if pubsub == nil {
		return sleepContext(ctx, timeout)
	}
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	_, err := pubsub.ReceiveMessage(waitCtx)
	if err == nil {
		return nil
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return nil
	}
	if errors.Is(err, context.Canceled) && ctx.Err() != nil {
		return ctx.Err()
	}
	waiter.resetPubSub()
	return sleepContext(ctx, timeout)
}

// Close closes the current subscription, if any.
func (waiter *Waiter) Close() error {
	if waiter == nil {
		return nil
	}
	waiter.mu.Lock()
	pubsub := waiter.pubsub
	waiter.pubsub = nil
	waiter.mu.Unlock()
	if pubsub == nil {
		return nil
	}
	return pubsub.Close()
}

func (waiter *Waiter) ensurePubSub(ctx context.Context) PubSub {
	waiter.mu.Lock()
	defer waiter.mu.Unlock()
	if waiter.pubsub != nil {
		return waiter.pubsub
	}
	waiter.pubsub = waiter.Client.Subscribe(ctx, waiter.subscribeChannels()...)
	return waiter.pubsub
}

func (waiter *Waiter) resetPubSub() {
	waiter.mu.Lock()
	pubsub := waiter.pubsub
	waiter.pubsub = nil
	waiter.mu.Unlock()
	if pubsub != nil {
		_ = pubsub.Close()
	}
}

func sleepContext(ctx context.Context, timeout time.Duration) error {
	if timeout <= 0 {
		return nil
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (waiter *Waiter) subscribeChannels() []string {
	if waiter == nil {
		return []string{DefaultChannel}
	}
	channels := waiter.Channels
	if len(channels) == 0 {
		channels = []string{waiter.Channel}
	}
	return normalizeChannels(channels)
}

func normalizeChannels(channels []string) []string {
	normalized := make([]string, 0, len(channels))
	seen := map[string]struct{}{}
	for _, channel := range channels {
		channel = defaultText(channel, "")
		if channel == "" {
			continue
		}
		if _, ok := seen[channel]; ok {
			continue
		}
		seen[channel] = struct{}{}
		normalized = append(normalized, channel)
	}
	if len(normalized) == 0 {
		return []string{DefaultChannel}
	}
	return normalized
}
