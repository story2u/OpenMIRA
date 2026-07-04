package archivemedia

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

const (
	DefaultLockTTL = 30 * time.Second
	MinimumLockTTL = 10 * time.Second
)

// ArchiveMediaScopeKey returns the media-worker lock scope shared by all tasks
// in one enterprise/source lane.
func ArchiveMediaScopeKey(enterpriseID string, source string) string {
	return defaultText(enterpriseID, "default") + "|" + defaultText(source, "self_decrypt")
}

// ArchiveMediaLockKey returns the Redis key used by Go archive media workers.
func ArchiveMediaLockKey(enterpriseID string, source string) string {
	return "archive-media:lock:" + ArchiveMediaScopeKey(enterpriseID, source)
}

// NewLockToken returns a random owner token for a media scope lease.
func NewLockToken() string {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buffer)
}

type scopeLockLease struct {
	Key      string
	Token    string
	TTL      time.Duration
	Acquired bool
}

func (service Service) acquireScopeLock(ctx context.Context, enterpriseID string, source string) (scopeLockLease, bool, error) {
	if service.Locks == nil {
		return scopeLockLease{}, true, nil
	}
	token := ""
	if service.NewLockToken != nil {
		token = strings.TrimSpace(service.NewLockToken())
	}
	if token == "" {
		token = NewLockToken()
	}
	lease := scopeLockLease{
		Key:   ArchiveMediaLockKey(enterpriseID, source),
		Token: token,
		TTL:   service.lockTTL(),
	}
	acquired, err := service.Locks.AcquireArchiveMediaLock(ctx, lease.Key, lease.Token, lease.TTL)
	if err != nil {
		return scopeLockLease{}, true, nil
	}
	if !acquired {
		return lease, false, nil
	}
	lease.Acquired = true
	return lease, true, nil
}

func (service Service) releaseScopeLock(ctx context.Context, lease scopeLockLease) {
	if service.Locks == nil || !lease.Acquired || strings.TrimSpace(lease.Key) == "" || strings.TrimSpace(lease.Token) == "" {
		return
	}
	_ = service.Locks.ReleaseArchiveMediaLock(ctx, lease.Key, lease.Token)
}

func (service Service) startScopeLockWatchdog(ctx context.Context, lease scopeLockLease) func() {
	renewEvery := service.lockRenewInterval(lease.TTL)
	if service.Locks == nil || !lease.Acquired || renewEvery <= 0 {
		return func() {}
	}
	done := make(chan struct{})
	stop := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(renewEvery)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				_ = service.Locks.RefreshArchiveMediaLock(ctx, lease.Key, lease.Token, lease.TTL)
			case <-stop:
				return
			case <-ctx.Done():
				return
			}
		}
	}()
	return func() {
		close(stop)
		select {
		case <-done:
		case <-time.After(time.Second):
		}
	}
}

func (service Service) lockTTL() time.Duration {
	if service.LockTTL < MinimumLockTTL {
		return DefaultLockTTL
	}
	return service.LockTTL
}

func (service Service) lockRenewInterval(ttl time.Duration) time.Duration {
	if service.LockRenew > 0 {
		if service.LockRenew >= ttl {
			return ttl - time.Second
		}
		return service.LockRenew
	}
	renew := ttl / 3
	if renew < time.Second {
		return time.Second
	}
	if renew >= ttl {
		return ttl - time.Second
	}
	return renew
}
