package wsgateway

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"time"
)

// PresenceStore receives local websocket client counts.
type PresenceStore interface {
	UpdateLocalClientCount(ctx context.Context, instanceID string, clientCount int) error
}

// PresenceReporter periodically reports local hub client counts.
type PresenceReporter struct {
	Hub        *Hub
	Store      PresenceStore
	InstanceID string
	Interval   time.Duration
}

// Start reports presence until ctx is cancelled. Cleanup reports zero clients.
func (reporter PresenceReporter) Start(ctx context.Context) func() error {
	if reporter.Hub == nil || reporter.Store == nil {
		return func() error { return nil }
	}
	if ctx == nil {
		ctx = context.Background()
	}
	instanceID := reporter.instanceID()
	interval := reporter.interval()
	reportCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = reporter.report(reportCtx, instanceID)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-reportCtx.Done():
				return
			case <-ticker.C:
				_ = reporter.report(reportCtx, instanceID)
			}
		}
	}()
	return func() error {
		cancel()
		select {
		case <-done:
		case <-time.After(time.Second):
		}
		return reporter.Store.UpdateLocalClientCount(context.Background(), instanceID, 0)
	}
}

// Report writes the current local client count once.
func (reporter PresenceReporter) Report(ctx context.Context) error {
	if reporter.Hub == nil || reporter.Store == nil {
		return nil
	}
	return reporter.report(ctx, reporter.instanceID())
}

func (reporter PresenceReporter) report(ctx context.Context, instanceID string) error {
	return reporter.Store.UpdateLocalClientCount(ctx, instanceID, reporter.Hub.ClientCount())
}

func (reporter PresenceReporter) instanceID() string {
	if reporter.InstanceID != "" {
		return reporter.InstanceID
	}
	return NewInstanceID()
}

func (reporter PresenceReporter) interval() time.Duration {
	if reporter.Interval <= 0 {
		return 5 * time.Second
	}
	return reporter.Interval
}

// NewInstanceID returns a process-stable-ish identifier for Redis presence fields.
func NewInstanceID() string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err == nil {
		return "go-" + hex.EncodeToString(bytes[:])
	}
	hostname, _ := os.Hostname()
	return fmt.Sprintf("go-%s-%d", hostname, os.Getpid())
}
