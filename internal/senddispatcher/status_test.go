package senddispatcher

import (
	"testing"
	"time"
)

// TestBuildStatusSnapshotMirrorsPythonCoreShape protects ownership snapshot fields.
func TestBuildStatusSnapshotMirrorsPythonCoreShape(t *testing.T) {
	capturedAt := time.Date(2026, 6, 29, 9, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	backlog := BacklogSummary{
		AcceptedTotal:        2,
		OldestAcceptedAgeSec: 30,
		ByDevice: map[string]BacklogDeviceSummary{
			"zimo": {Accepted: 2, OldestAgeSec: 30},
		},
	}
	snapshot := BuildStatusSnapshot(StatusSnapshotInput{
		WorkerID:         "worker-1",
		VisibleDeviceIDs: []string{" zimo ", "ada", "zimo", ""},
		Allowlist:        []string{"zimo", "bob", "zimo"},
		Exclude:          []string{"ada"},
		Backlog:          backlog,
		CapturedAt:       capturedAt,
	})
	if snapshot.WorkerID != "worker-1" || snapshot.VisibleDeviceCount != 2 || snapshot.OwnedDeviceCount != 1 {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	if len(snapshot.VisibleDeviceIDs) != 2 || snapshot.VisibleDeviceIDs[0] != "zimo" || snapshot.VisibleDeviceIDs[1] != "ada" {
		t.Fatalf("visible devices = %#v", snapshot.VisibleDeviceIDs)
	}
	if len(snapshot.OwnedDeviceIDs) != 1 || snapshot.OwnedDeviceIDs[0] != "zimo" {
		t.Fatalf("owned devices = %#v", snapshot.OwnedDeviceIDs)
	}
	if len(snapshot.Allowlist) != 2 || snapshot.Allowlist[0] != "zimo" || snapshot.Allowlist[1] != "bob" {
		t.Fatalf("allowlist = %#v", snapshot.Allowlist)
	}
	if snapshot.Backlog.AcceptedTotal != 2 || snapshot.CapturedAt.Location() != time.UTC {
		t.Fatalf("backlog=%#v capturedAt=%v", snapshot.Backlog, snapshot.CapturedAt)
	}
}

// TestStatusSnapshotCacheUsesWorkerTTLAndCopies protects Python short-cache semantics.
func TestStatusSnapshotCacheUsesWorkerTTLAndCopies(t *testing.T) {
	now := time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC)
	cache := NewStatusSnapshotCache()
	cache.Put(StatusSnapshot{
		WorkerID:         "worker-a",
		VisibleDeviceIDs: []string{"dev-a"},
		Backlog: BacklogSummary{
			AcceptedTotal: 1,
			ByDevice:      map[string]BacklogDeviceSummary{"dev-a": {Accepted: 1}},
		},
	}, now, 2)

	snapshot, ok := cache.Get(" worker-a ", now.Add(time.Second))
	if !ok {
		t.Fatal("cache miss before TTL")
	}
	snapshot.VisibleDeviceIDs[0] = "mutated"
	snapshot.Backlog.ByDevice["dev-a"] = BacklogDeviceSummary{Accepted: 99}

	again, ok := cache.Get("worker-a", now.Add(time.Second))
	if !ok {
		t.Fatal("cache miss after returned snapshot mutation")
	}
	if again.VisibleDeviceIDs[0] != "dev-a" || again.Backlog.ByDevice["dev-a"].Accepted != 1 {
		t.Fatalf("cached snapshot was mutated: %#v", again)
	}
	if _, ok := cache.Get("worker-b", now.Add(time.Second)); ok {
		t.Fatal("cache hit different worker")
	}
	if _, ok := cache.Get("worker-a", now.Add(2*time.Second)); ok {
		t.Fatal("cache hit at expiry boundary")
	}
}

// TestHeartbeatDueMirrorsPythonWorkerIntervalRules protects heartbeat throttling.
func TestHeartbeatDueMirrorsPythonWorkerIntervalRules(t *testing.T) {
	now := time.Date(2026, 6, 29, 9, 0, 10, 0, time.UTC)
	if !HeartbeatDue(nil, "worker-1", now, 10) {
		t.Fatal("first heartbeat was not due")
	}
	previous := RememberHeartbeat(nil, " worker-1 ", now.Add(-9*time.Second))
	if HeartbeatDue(previous, "worker-1", now, 10) {
		t.Fatal("heartbeat due before interval")
	}
	if !HeartbeatDue(previous, "worker-1", now.Add(time.Second), 10) {
		t.Fatal("heartbeat not due at interval")
	}
	previous = RememberHeartbeat(nil, "", now)
	if _, ok := previous["__default__"]; !ok {
		t.Fatalf("default worker heartbeat missing: %#v", previous)
	}
	if previous["__default__"].Location() != time.UTC {
		t.Fatalf("heartbeat timestamp not UTC: %v", previous["__default__"])
	}
}
