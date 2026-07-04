package wsgateway

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestPresenceReporterReportsCurrentClientCount(t *testing.T) {
	hub := NewHub()
	hub.Register("conversations", nil, &recordingSender{})
	hub.Register("tasks", nil, &recordingSender{})
	store := &recordingPresenceStore{}

	err := (PresenceReporter{Hub: hub, Store: store, InstanceID: "go-test"}).Report(context.Background())
	if err != nil {
		t.Fatalf("Report returned error: %v", err)
	}
	if len(store.records) != 1 || store.records[0].instanceID != "go-test" || store.records[0].count != 2 {
		t.Fatalf("records = %#v", store.records)
	}
}

func TestPresenceReporterStartReportsZeroOnCleanup(t *testing.T) {
	hub := NewHub()
	hub.Register("conversations", nil, &recordingSender{})
	store := &recordingPresenceStore{}
	cleanup := (PresenceReporter{
		Hub:        hub,
		Store:      store,
		InstanceID: "go-test",
		Interval:   time.Hour,
	}).Start(context.Background())

	deadline := time.After(time.Second)
	for {
		if len(store.snapshot()) > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("initial presence report not observed: %#v", store.records)
		case <-time.After(time.Millisecond):
		}
	}
	if err := cleanup(); err != nil {
		t.Fatalf("cleanup returned error: %v", err)
	}
	records := store.snapshot()
	if len(records) < 2 || records[len(records)-1].count != 0 {
		t.Fatalf("records = %#v", records)
	}
}

type recordingPresenceStore struct {
	mu      sync.Mutex
	records []presenceRecord
}

type presenceRecord struct {
	instanceID string
	count      int
}

func (store *recordingPresenceStore) UpdateLocalClientCount(_ context.Context, instanceID string, clientCount int) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.records = append(store.records, presenceRecord{instanceID: instanceID, count: clientCount})
	return nil
}

func (store *recordingPresenceStore) snapshot() []presenceRecord {
	store.mu.Lock()
	defer store.mu.Unlock()
	return append([]presenceRecord(nil), store.records...)
}
