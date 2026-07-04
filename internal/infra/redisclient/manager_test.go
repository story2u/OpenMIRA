// Package redisclient tests legacy Redis URL fallback and parsing behavior.
// The tests do not connect to Redis; they lock down configuration semantics
// before business services start using shared clients.
package redisclient

import (
	"errors"
	"testing"
)

// TestResolveURLsAppliesLegacyFallbacks mirrors Python RedisManager behavior.
func TestResolveURLsAppliesLegacyFallbacks(t *testing.T) {
	urls := ResolveURLs(Config{RealtimeURL: "redis://localhost:6379/0"})
	if urls.Realtime != "redis://localhost:6379/0" {
		t.Fatalf("realtime url = %q", urls.Realtime)
	}
	if urls.Cache != urls.Realtime || urls.Lock != urls.Cache || urls.Eventbus != urls.Realtime {
		t.Fatalf("unexpected fallback URLs: %+v", urls)
	}
}

// TestResolveURLsKeepsExplicitKindsSeparate protects Redis DB/role boundaries.
func TestResolveURLsKeepsExplicitKindsSeparate(t *testing.T) {
	urls := ResolveURLs(Config{
		RealtimeURL: "redis://redis:6379/0",
		CacheURL:    "redis://redis:6379/1",
		LockURL:     "redis://redis:6379/2",
		EventbusURL: "redis://redis:6379/3",
	})
	if urls.Realtime == urls.Cache || urls.Cache == urls.Lock || urls.Eventbus == urls.Realtime {
		t.Fatalf("explicit urls collapsed unexpectedly: %+v", urls)
	}
}

// TestURLsRejectUnknownKind keeps callers from inventing parallel Redis clients.
func TestURLsRejectUnknownKind(t *testing.T) {
	_, err := ResolveURLs(Config{}).URL("unknown")
	if !errors.Is(err, ErrUnsupportedKind) {
		t.Fatalf("URL error = %v, want %v", err, ErrUnsupportedKind)
	}
}

// TestClientOptionsParsesRedisURL validates URL parsing without network access.
func TestClientOptionsParsesRedisURL(t *testing.T) {
	options, err := ClientOptions("redis://:secret@redis.example:6380/2")
	if err != nil {
		t.Fatalf("ClientOptions returned error: %v", err)
	}
	if options.Addr != "redis.example:6380" || options.Password != "secret" || options.DB != 2 {
		t.Fatalf("unexpected redis options: addr=%q password=%q db=%d", options.Addr, options.Password, options.DB)
	}
}

// TestManagerReturnsNilClientForUnconfiguredURL keeps optional Redis explicit.
func TestManagerReturnsNilClientForUnconfiguredURL(t *testing.T) {
	manager := NewManager(Config{})
	client, err := manager.Client(KindCache)
	if err != nil {
		t.Fatalf("Client returned error: %v", err)
	}
	if client != nil {
		t.Fatal("client != nil for unconfigured Redis URL")
	}
}
