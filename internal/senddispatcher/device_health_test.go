package senddispatcher

import (
	"strings"
	"testing"
	"time"
)

// TestSDKDeviceHealthTransportFailureDecision mirrors recent transport cooldown writes.
func TestSDKDeviceHealthTransportFailureDecision(t *testing.T) {
	now := time.Date(2026, 6, 30, 9, 0, 0, 0, time.UTC)
	decision := BuildSDKDeviceHealthDecision(
		" p1-slot-18 ",
		false,
		"sdk subprocess timeout after 180s",
		" task-send-1 ",
		" send_text ",
		nil,
		SDKDeviceHealthOptions{Now: func() time.Time { return now }},
	)

	if decision.TransportFailure == nil || decision.TransportFailure.DeviceID != "p1-slot-18" || decision.TransportFailure.TaskID != "task-send-1" {
		t.Fatalf("decision = %#v", decision)
	}
	if decision.TransportFailure.Available || decision.TransportFailure.ExpiresAt.Sub(now) != 180*time.Second {
		t.Fatalf("transport failure = %#v", decision.TransportFailure)
	}
	if decision.UIUnstableFailure != nil || decision.ClearUIUnstable {
		t.Fatalf("unexpected UI decision = %#v", decision)
	}
}

// TestSDKDeviceHealthSkipsProbeAndRecentTransportPrefix protects fail-fast boundaries.
func TestSDKDeviceHealthSkipsProbeAndRecentTransportPrefix(t *testing.T) {
	now := time.Date(2026, 6, 30, 9, 0, 0, 0, time.UTC)
	probe := BuildSDKDeviceHealthDecision("p1-slot-18", false, "sdk subprocess timeout after 180s", "probe-1", "wework_login_status", nil, SDKDeviceHealthOptions{Now: func() time.Time { return now }})
	if probe.TransportFailure != nil || probe.UIUnstableFailure != nil || probe.ClearTransport || probe.ClearUIUnstable {
		t.Fatalf("probe decision = %#v", probe)
	}
	recent := BuildSDKDeviceHealthDecision("p1-slot-18", false, "recent SDK transport failure for p1-slot-18: connection failed", "task-1", "send_text", nil, SDKDeviceHealthOptions{Now: func() time.Time { return now }})
	if recent.TransportFailure != nil || recent.UIUnstableFailure != nil || recent.ClearTransport || recent.ClearUIUnstable {
		t.Fatalf("recent decision = %#v", recent)
	}
	if stripped := StripRecentSDKTransportFailurePrefix("recent SDK transport failure for p1-slot-18: recent SDK transport failure: connection failed", "p1-slot-18"); stripped != "connection failed" {
		t.Fatalf("stripped = %q", stripped)
	}
}

// TestSDKDeviceHealthUIUnstableDecision mirrors threshold cooldown behavior.
func TestSDKDeviceHealthUIUnstableDecision(t *testing.T) {
	now := time.Date(2026, 6, 30, 9, 0, 0, 0, time.UTC)
	options := SDKDeviceHealthOptions{Now: func() time.Time { return now }}
	first := BuildSDKDeviceHealthDecision("p1-slot-18", false, "click_plus_button plus button not found", "task-ui-1", "send_image", nil, options)
	if first.UIUnstableFailure == nil || first.UIUnstableFailure.Count != 1 || first.UIUnstableFailure.CoolingDown {
		t.Fatalf("first decision = %#v", first)
	}
	second := BuildSDKDeviceHealthDecision("p1-slot-18", false, "click_plus_button plus button not found", "task-ui-2", "send_image", first.UIUnstableFailure, options)
	third := BuildSDKDeviceHealthDecision("p1-slot-18", false, "click_plus_button plus button not found", "task-ui-3", "send_image", second.UIUnstableFailure, options)
	if third.UIUnstableFailure == nil || third.UIUnstableFailure.Count != 3 || !third.UIUnstableFailure.CoolingDown || third.UIUnstableFailure.Stage != "compose_surface" {
		t.Fatalf("third decision = %#v", third)
	}
	if third.UIUnstableFailure.ExpiresAt.Sub(now) != 120*time.Second {
		t.Fatalf("cooldown ttl = %s", third.UIUnstableFailure.ExpiresAt.Sub(now))
	}
	if decision := BuildSDKDeviceHealthDecision("p1-slot-18", true, "", "task-ok", "send_text", third.UIUnstableFailure, options); !decision.ClearTransport || !decision.ClearUIUnstable {
		t.Fatalf("success decision = %#v", decision)
	}
}

// TestSDKDeviceHealthClearsUIForBusinessAndLoginState mirrors non-UI failures.
func TestSDKDeviceHealthClearsUIForBusinessAndLoginState(t *testing.T) {
	now := time.Date(2026, 6, 30, 9, 0, 0, 0, time.UTC)
	previous := &SDKDeviceUIUnstableState{Count: 2, ExpiresAt: now.Add(time.Minute)}
	business := BuildSDKDeviceHealthDecision("p1-slot-18", false, "包含企业设置的敏感词", "task-1", "send_text", previous, SDKDeviceHealthOptions{Now: func() time.Time { return now }})
	if !business.ClearUIUnstable || business.UIUnstableFailure != nil || business.TransportFailure != nil {
		t.Fatalf("business decision = %#v", business)
	}
	login := BuildSDKDeviceHealthDecision("p1-slot-18", false, "login page visible", "task-2", "send_text", previous, SDKDeviceHealthOptions{Now: func() time.Time { return now }})
	if !login.ClearUIUnstable || login.UIUnstableFailure != nil || login.TransportFailure != nil {
		t.Fatalf("login decision = %#v", login)
	}
}

// TestSDKDeviceHealthTruncatesLongErrors keeps cache payload bounded.
func TestSDKDeviceHealthTruncatesLongErrors(t *testing.T) {
	longError := "sdk subprocess timeout after 180s " + strings.Repeat("x", 300)
	decision := BuildSDKDeviceHealthDecision("p1-slot-18", false, longError, "task-1", "send_text", nil, SDKDeviceHealthOptions{})
	if decision.TransportFailure == nil || len([]rune(decision.TransportFailure.Error)) != 240 {
		t.Fatalf("transport failure = %#v", decision.TransportFailure)
	}
}
