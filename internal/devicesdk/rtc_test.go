package devicesdk

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"wework-go/internal/auth"
)

func TestServiceRTCSessionBuildsLiveKitPayloadAndMarksActive(t *testing.T) {
	cacheFile := writeManagerCache(t, `[{
		"device_id": "slot-18",
		"host": "192.168.1.30",
		"slot": 18,
		"aliases": ["p1-18-slot"]
	}]`)
	store := NewMemoryRTCStateStore()
	store.SetControlState("slot-18", map[string]any{
		"controller_identity": "user-admin-001-slot-18",
		"controller_user_id":  "admin-001",
		"controller_name":     "管理员",
		"controller_role":     "admin",
		"acquired_at":         1782921600.0,
		"expires_at":          1782981900.0,
	})
	now := time.Date(2026, 7, 2, 8, 0, 0, 0, time.UTC)
	service := Service{
		Config: Config{
			ManagerCacheFile:          cacheFile,
			LiveKitURL:                "https://livekit.example",
			LiveKitAPIKey:             "lk-key",
			LiveKitAPISecret:          "lk-secret",
			LiveKitTokenTTLSeconds:    120,
			LiveKitDeviceRoomPrefix:   "device",
			RTCBridgeActiveTTLSeconds: 30,
		},
		RTCState: store,
		Now:      func() time.Time { return now },
		NewID:    func(prefix string) string { return prefix + "abc123xyz" },
	}

	payload, err := service.RTCSession(context.Background(), "p1-18-slot", "1", "auto", RequestOrigin{Scheme: "https", Host: "cloud.example"}, auth.Session{
		AssigneeID:   "admin-001",
		AssigneeName: "管理员",
		Role:         "admin",
	})
	if err != nil {
		t.Fatalf("RTCSession returned error: %v", err)
	}

	if payload["success"] != true || payload["device_id"] != "slot-18" || payload["mode"] != "livekit" || payload["requested_mode"] != "auto" {
		t.Fatalf("payload = %#v", payload)
	}
	if payload["livekit_url"] != "wss://livekit.example" || payload["room_name"] != "device-slot-18" {
		t.Fatalf("livekit payload = url %#v room %#v", payload["livekit_url"], payload["room_name"])
	}
	if payload["url"] != "https://cloud.example/admin/livekit-device?device_id=slot-18" || payload["entry_url"] != payload["url"] {
		t.Fatalf("entry urls = %#v/%#v", payload["url"], payload["entry_url"])
	}
	identity := payload["participant_identity"].(string)
	if !strings.HasPrefix(identity, "user-admin-001-slot-18-1782979200000-abc123") {
		t.Fatalf("participant identity = %q", identity)
	}
	controlState := payload["control_state"].(map[string]any)
	if controlState["controlled"] != true || controlState["controller_identity"] != "user-admin-001-slot-18" {
		t.Fatalf("control_state = %#v", controlState)
	}
	mark, ok := store.ActiveMark("slot-18")
	if !ok || mark.RoomName != "device-slot-18" || mark.ParticipantIdentity != identity {
		t.Fatalf("active mark = %+v ok=%v", mark, ok)
	}
	if !mark.ExpiresAt.Equal(now.Add(30 * time.Second)) {
		t.Fatalf("active ttl = %s, want %s", mark.ExpiresAt, now.Add(30*time.Second))
	}

	claims := decodeJWTPayload(t, payload["token"].(string))
	if claims["iss"] != "lk-key" || claims["sub"] != identity || claims["name"] != "管理员" {
		t.Fatalf("token claims = %#v", claims)
	}
	video := claims["video"].(map[string]any)
	if video["room"] != "device-slot-18" || video["roomJoin"] != true || video["canPublishData"] != true {
		t.Fatalf("video claims = %#v", video)
	}
}

func TestServiceRTCSessionRejectsLegacyMode(t *testing.T) {
	cacheFile := writeManagerCache(t, `[{"device_id":"slot-18","slot":18}]`)
	service := Service{Config: Config{
		ManagerCacheFile: cacheFile,
		LiveKitURL:       "wss://livekit.example",
		LiveKitAPIKey:    "lk-key",
		LiveKitAPISecret: "lk-secret",
	}}

	_, err := service.RTCSession(context.Background(), "slot-18", "1", "legacy", RequestOrigin{}, auth.Session{AssigneeID: "cs-1", Role: "cs"})

	if !errors.Is(err, ErrSDKLegacyRTCDisabled) {
		t.Fatalf("error = %v, want %v", err, ErrSDKLegacyRTCDisabled)
	}
}

func TestServiceRTCSessionRequiresLiveKitConfig(t *testing.T) {
	cacheFile := writeManagerCache(t, `[{"device_id":"slot-18","slot":18}]`)
	service := Service{Config: Config{ManagerCacheFile: cacheFile}}

	_, err := service.RTCSession(context.Background(), "slot-18", "1", "auto", RequestOrigin{}, auth.Session{AssigneeID: "cs-1", Role: "cs"})

	if !errors.Is(err, ErrSDKLiveKitNotConfigured) {
		t.Fatalf("error = %v, want %v", err, ErrSDKLiveKitNotConfigured)
	}
}

func TestServiceRTCSessionReportsExpiredControlStateAsUncontrolled(t *testing.T) {
	cacheFile := writeManagerCache(t, `[{"device_id":"slot-18","slot":18}]`)
	store := NewMemoryRTCStateStore()
	store.SetControlState("slot-18", map[string]any{
		"controller_identity": "stale",
		"expires_at":          100.0,
	})
	service := Service{
		Config: Config{
			ManagerCacheFile: cacheFile,
			LiveKitURL:       "wss://livekit.example",
			LiveKitAPIKey:    "lk-key",
			LiveKitAPISecret: "lk-secret",
		},
		RTCState: store,
		Now:      func() time.Time { return time.Unix(200, 0).UTC() },
		NewID:    func(prefix string) string { return prefix + "fixed1" },
	}

	payload, err := service.RTCSession(context.Background(), "slot-18", "1", "auto", RequestOrigin{Scheme: "https", Host: "cloud.example"}, auth.Session{AssigneeID: "cs-1", Role: "cs"})
	if err != nil {
		t.Fatalf("RTCSession returned error: %v", err)
	}

	controlState := payload["control_state"].(map[string]any)
	if controlState["controlled"] != false || controlState["controller_identity"] != "" || controlState["expires_at"] != 0.0 {
		t.Fatalf("control_state = %#v", controlState)
	}
}

func TestServiceRTCActiveRefreshesBridgeMark(t *testing.T) {
	cacheFile := writeManagerCache(t, `[{"device_id":"slot-18","slot":18,"aliases":["p1-18-slot"]}]`)
	store := NewMemoryRTCStateStore()
	now := time.Date(2026, 7, 2, 8, 0, 0, 0, time.UTC)
	service := Service{
		Config: Config{
			ManagerCacheFile:          cacheFile,
			LiveKitDeviceRoomPrefix:   "device",
			RTCBridgeActiveTTLSeconds: 45,
		},
		RTCState: store,
		Now:      func() time.Time { return now },
	}

	payload, err := service.RTCActive(context.Background(), "p1-18-slot", "user-admin-slot-18")
	if err != nil {
		t.Fatalf("RTCActive returned error: %v", err)
	}

	if payload["success"] != true || payload["device_id"] != "slot-18" || payload["room_name"] != "device-slot-18" || payload["participant_identity"] != "user-admin-slot-18" {
		t.Fatalf("payload = %#v", payload)
	}
	mark, ok := store.ActiveMark("slot-18")
	if !ok || !mark.ExpiresAt.Equal(now.Add(45*time.Second)) {
		t.Fatalf("mark = %+v ok=%v", mark, ok)
	}
}

func TestServiceRTCActiveRequiresParticipantIdentity(t *testing.T) {
	cacheFile := writeManagerCache(t, `[{"device_id":"slot-18","slot":18}]`)
	service := Service{Config: Config{ManagerCacheFile: cacheFile}}

	_, err := service.RTCActive(context.Background(), "slot-18", " ")

	if !errors.Is(err, ErrSDKParticipantIdentityRequired) {
		t.Fatalf("error = %v, want %v", err, ErrSDKParticipantIdentityRequired)
	}
}

func TestServiceListRTCActiveReturnsUnexpiredMarks(t *testing.T) {
	store := NewMemoryRTCStateStore()
	now := time.Now().UTC()
	if err := store.MarkBridgeActive(context.Background(), BridgeActiveMark{
		DeviceID:            "slot-18",
		RoomName:            "device-slot-18",
		ParticipantIdentity: "user-admin-slot-18",
		ActiveAt:            now,
		ExpiresAt:           now.Add(time.Minute),
	}, time.Minute); err != nil {
		t.Fatalf("MarkBridgeActive returned error: %v", err)
	}
	if err := store.MarkBridgeActive(context.Background(), BridgeActiveMark{
		DeviceID:            "slot-19",
		RoomName:            "device-slot-19",
		ParticipantIdentity: "user-admin-slot-19",
		ActiveAt:            now.Add(-time.Minute),
		ExpiresAt:           now.Add(-time.Second),
	}, time.Minute); err != nil {
		t.Fatalf("MarkBridgeActive returned error: %v", err)
	}
	service := Service{RTCState: store}

	payload, err := service.ListRTCActive(context.Background())
	if err != nil {
		t.Fatalf("ListRTCActive returned error: %v", err)
	}

	devices := payload["devices"].([]map[string]any)
	if len(devices) != 1 || devices[0]["device_id"] != "slot-18" {
		t.Fatalf("devices = %#v", devices)
	}
}

func TestServiceAcquireControlAllowsSameUserRenewal(t *testing.T) {
	cacheFile := writeManagerCache(t, `[{"device_id":"slot-18","slot":18}]`)
	store := NewMemoryRTCStateStore()
	now := time.Date(2026, 7, 2, 8, 0, 0, 0, time.UTC)
	store.SetControlState("slot-18", map[string]any{
		"controller_identity": "old-identity",
		"controller_user_id":  "admin-001",
		"controller_role":     "admin",
		"expires_at":          unixSeconds(now.Add(time.Minute)),
	})
	service := Service{
		Config:   Config{ManagerCacheFile: cacheFile, RTCControlTTLSeconds: 60},
		RTCState: store,
		Now:      func() time.Time { return now },
	}

	payload, err := service.AcquireControl(context.Background(), "slot-18", "new-identity", auth.Session{AssigneeID: "admin-001", AssigneeName: "管理员", Role: "admin"})
	if err != nil {
		t.Fatalf("AcquireControl returned error: %v", err)
	}

	if payload["controlled"] != true || payload["controller_identity"] != "new-identity" || payload["controller_user_id"] != "admin-001" {
		t.Fatalf("payload = %#v", payload)
	}
	state, err := store.ReadControlState(context.Background(), "slot-18")
	if err != nil {
		t.Fatalf("ReadControlState returned error: %v", err)
	}
	if state["controller_identity"] != "new-identity" {
		t.Fatalf("stored state = %#v", state)
	}
}

func TestServiceAcquireControlReturnsConflictForDifferentUser(t *testing.T) {
	cacheFile := writeManagerCache(t, `[{"device_id":"slot-18","slot":18}]`)
	store := NewMemoryRTCStateStore()
	now := time.Date(2026, 7, 2, 8, 0, 0, 0, time.UTC)
	store.SetControlState("slot-18", map[string]any{
		"controller_identity": "existing",
		"controller_user_id":  "admin-001",
		"controller_role":     "admin",
		"expires_at":          unixSeconds(now.Add(time.Minute)),
	})
	service := Service{
		Config:   Config{ManagerCacheFile: cacheFile},
		RTCState: store,
		Now:      func() time.Time { return now },
	}

	_, err := service.AcquireControl(context.Background(), "slot-18", "other", auth.Session{AssigneeID: "cs-002", Role: "cs"})

	var conflict ControlConflictError
	if !errors.As(err, &conflict) || conflict.State["controller_identity"] != "existing" {
		t.Fatalf("error = %#v, conflict = %#v", err, conflict.State)
	}
}

func TestServiceReleaseControlRequiresOwnerForCS(t *testing.T) {
	cacheFile := writeManagerCache(t, `[{"device_id":"slot-18","slot":18}]`)
	store := NewMemoryRTCStateStore()
	store.SetControlState("slot-18", map[string]any{"controller_identity": "owner"})
	service := Service{Config: Config{ManagerCacheFile: cacheFile}, RTCState: store}

	_, err := service.ReleaseControl(context.Background(), "slot-18", "other", auth.Session{AssigneeID: "cs-002", Role: "cs"})

	if !errors.Is(err, ErrSDKControlReleaseForbidden) {
		t.Fatalf("error = %v, want %v", err, ErrSDKControlReleaseForbidden)
	}
}

func TestServiceStealControlWritesOperatorLease(t *testing.T) {
	cacheFile := writeManagerCache(t, `[{"device_id":"slot-18","slot":18}]`)
	store := NewMemoryRTCStateStore()
	now := time.Date(2026, 7, 2, 8, 0, 0, 0, time.UTC)
	service := Service{
		Config:   Config{ManagerCacheFile: cacheFile, RTCControlTTLSeconds: 90},
		RTCState: store,
		Now:      func() time.Time { return now },
	}

	payload, err := service.StealControl(context.Background(), "slot-18", "operator-identity", auth.Session{AssigneeID: "sup-001", Role: "supervisor"})
	if err != nil {
		t.Fatalf("StealControl returned error: %v", err)
	}

	if payload["controller_identity"] != "operator-identity" || payload["controller_role"] != "supervisor" || payload["expires_at"] != unixSeconds(now.Add(90*time.Second)) {
		t.Fatalf("payload = %#v", payload)
	}
}

func decodeJWTPayload(t *testing.T, token string) map[string]any {
	t.Helper()
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("token parts = %d, want 3", len(parts))
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	var claims map[string]any
	if err := json.Unmarshal(raw, &claims); err != nil {
		t.Fatalf("unmarshal claims: %v", err)
	}
	return claims
}
