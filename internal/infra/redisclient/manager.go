// Package redisclient centralizes Redis clients for the Go rewrite.
// It preserves the legacy realtime/cache/lock/eventbus URL fallback rules so
// later services do not create ad hoc Redis connections or guess namespaces.
package redisclient

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/redis/go-redis/v9"
)

const (
	// KindRealtime is used for WebSocket Pub/Sub and realtime cursors.
	KindRealtime = "realtime"
	// KindCache is used for shared cache and projection mirrors.
	KindCache = "cache"
	// KindLock is used for distributed locks and short leases.
	KindLock = "lock"
	// KindEventbus is used for Redis Streams ingest/eventbus traffic.
	KindEventbus = "eventbus"
)

// ErrUnsupportedKind means a caller requested an unknown Redis client kind.
var ErrUnsupportedKind = errors.New("redis client kind is unsupported")

// Config contains Redis URLs from legacy environment variables.
type Config struct {
	RealtimeURL string
	CacheURL    string
	LockURL     string
	EventbusURL string
}

// URLs is the resolved Redis URL set after applying legacy fallback rules.
type URLs struct {
	Realtime string
	Cache    string
	Lock     string
	Eventbus string
}

// Manager lazily creates and owns Redis clients by kind.
type Manager struct {
	urls    URLs
	mu      sync.Mutex
	clients map[string]*redis.Client
}

// ResolveURLs applies Python RedisManager-compatible URL fallback rules.
func ResolveURLs(config Config) URLs {
	realtime := strings.TrimSpace(config.RealtimeURL)
	cache := strings.TrimSpace(config.CacheURL)
	if cache == "" {
		cache = realtime
	}
	lock := strings.TrimSpace(config.LockURL)
	if lock == "" {
		lock = cache
	}
	eventbus := strings.TrimSpace(config.EventbusURL)
	if eventbus == "" {
		eventbus = realtime
	}
	return URLs{
		Realtime: realtime,
		Cache:    cache,
		Lock:     lock,
		Eventbus: eventbus,
	}
}

// NewManager creates a lazy Redis client manager.
func NewManager(config Config) *Manager {
	return &Manager{
		urls:    ResolveURLs(config),
		clients: map[string]*redis.Client{},
	}
}

// URL returns the resolved URL for kind.
func (manager *Manager) URL(kind string) (string, error) {
	if manager == nil {
		return "", nil
	}
	return manager.urls.URL(kind)
}

// Client returns a cached client, or nil when the resolved URL is empty.
func (manager *Manager) Client(kind string) (*redis.Client, error) {
	if manager == nil {
		return nil, nil
	}
	kind = normalizeKind(kind)
	urlValue, err := manager.urls.URL(kind)
	if err != nil {
		return nil, err
	}
	if urlValue == "" {
		return nil, nil
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if client := manager.clients[kind]; client != nil {
		return client, nil
	}
	options, err := ClientOptions(urlValue)
	if err != nil {
		return nil, err
	}
	client := redis.NewClient(options)
	manager.clients[kind] = client
	return client, nil
}

// Ping verifies connectivity for one configured Redis kind.
func (manager *Manager) Ping(ctx context.Context, kind string) error {
	client, err := manager.Client(kind)
	if err != nil || client == nil {
		return err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return client.Ping(ctx).Err()
}

// Close releases all clients owned by the manager.
func (manager *Manager) Close() error {
	if manager == nil {
		return nil
	}
	manager.mu.Lock()
	clients := make([]*redis.Client, 0, len(manager.clients))
	for _, client := range manager.clients {
		clients = append(clients, client)
	}
	manager.clients = map[string]*redis.Client{}
	manager.mu.Unlock()

	var closeErr error
	for _, client := range clients {
		if err := client.Close(); err != nil && closeErr == nil {
			closeErr = err
		}
	}
	return closeErr
}

// URL returns the resolved URL for kind.
func (urls URLs) URL(kind string) (string, error) {
	switch normalizeKind(kind) {
	case KindRealtime:
		return urls.Realtime, nil
	case KindCache:
		return urls.Cache, nil
	case KindLock:
		return urls.Lock, nil
	case KindEventbus:
		return urls.Eventbus, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrUnsupportedKind, kind)
	}
}

// ClientOptions parses a Redis URL without connecting to Redis.
func ClientOptions(redisURL string) (*redis.Options, error) {
	redisURL = strings.TrimSpace(redisURL)
	if redisURL == "" {
		return nil, nil
	}
	return redis.ParseURL(redisURL)
}

func normalizeKind(kind string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(kind), "-", "_"))
}
