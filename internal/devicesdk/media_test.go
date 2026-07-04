package devicesdk

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"wework-go/internal/auth"
)

func TestServiceStartMediaPrepareBuildsRelayMetadata(t *testing.T) {
	cacheFile := writeManagerCache(t, `[{
		"device_id": "slot-18",
		"host": "192.168.1.30",
		"manager_host": "manager.local",
		"manager_device_ip": "10.0.0.18",
		"container_name": "p1-container-18",
		"slot": 18,
		"aliases": ["p1-18-slot"]
	}]`)
	store := NewMemoryRTCStateStore()
	store.SetControlState("slot-18", map[string]any{
		"controller_identity": "viewer-tab",
		"controller_user_id":  "cs-001",
		"controller_role":     "cs",
		"expires_at":          float64(4102444800),
	})
	service := Service{
		Config: Config{
			ManagerCacheFile:                     cacheFile,
			RTCMediaCameraAddrTemplate:           "https://relay.example/live/{stream_key}/whep",
			RTCMediaWHIPPublishURLTemplate:       "https://relay.example/live/{stream_key}/whip",
			RTCMediaDirectWHIPPublishURLTemplate: "http://127.0.0.1/live/{stream_key}/whip",
			RTCMediaP1PlaybackHost:               "p1-playback.example:1985",
			RTCMediaInstanceTTLSeconds:           3600,
		},
		RTCState: store,
		Now:      func() time.Time { return time.Unix(1800000000, 0).UTC() },
		NewID:    func(prefix string) string { return prefix + "abc123" },
	}

	payload, err := service.StartMedia(context.Background(), "p1-18-slot", MediaStartRequest{
		ParticipantIdentity: "viewer-tab",
		Camera:              true,
		Microphone:          true,
		Activate:            false,
	}, auth.Session{AssigneeID: "cs-001", Role: "cs"})
	if err != nil {
		t.Fatalf("StartMedia returned error: %v", err)
	}
	if payload["success"] != true || payload["device_id"] != "slot-18" || payload["room_name"] != "device-slot-18" || payload["controller_identity"] != "viewer-tab" {
		t.Fatalf("payload basics = %+v", payload)
	}
	camera := payload["camera"].(map[string]any)
	if camera["status"] != "prepared" || camera["stream_key"] != "slot-18-input" {
		t.Fatalf("camera = %+v", camera)
	}
	if camera["playback_url"] != "webrtc://p1-playback.example/live/slot-18-input" {
		t.Fatalf("playback_url = %v", camera["playback_url"])
	}
	if camera["preview_url"] != "https://relay.example/live/slot-18-input/whep" || camera["publish_url"] != "https://relay.example/live/slot-18-input/whip" {
		t.Fatalf("preview/publish = %v / %v", camera["preview_url"], camera["publish_url"])
	}
	if camera["direct_publish_url"] != "" {
		t.Fatalf("direct_publish_url = %v, want filtered loopback", camera["direct_publish_url"])
	}
	if camera["p1_signaling_url"] != "http://p1-playback.example:1985/rtc/v1/play/" {
		t.Fatalf("p1_signaling_url = %v", camera["p1_signaling_url"])
	}
	audio := payload["audio"].(map[string]any)
	if audio["status"] != "prepared" || !strings.Contains(audio["detail"].(string), "microphone audio") {
		t.Fatalf("audio = %+v", audio)
	}
}

func TestServiceStartMediaRequiresCurrentControllerForCS(t *testing.T) {
	cacheFile := writeManagerCache(t, `[{"device_id":"slot-18","host":"192.168.1.30","manager_host":"manager.local","container_name":"p1-container-18","slot":18}]`)
	store := NewMemoryRTCStateStore()
	store.SetControlState("slot-18", map[string]any{"controller_identity": "owner", "expires_at": float64(4102444800)})
	service := Service{Config: Config{ManagerCacheFile: cacheFile, RTCMediaCameraAddrTemplate: "webrtc://p1/live/{stream_key}"}, RTCState: store}

	_, err := service.StartMedia(context.Background(), "slot-18", MediaStartRequest{
		ParticipantIdentity: "other",
		Camera:              true,
		Microphone:          false,
		Activate:            false,
	}, auth.Session{AssigneeID: "cs-002", Role: "cs"})
	if !errors.Is(err, ErrSDKMediaStartForbidden) {
		t.Fatalf("error = %v, want %v", err, ErrSDKMediaStartForbidden)
	}
}

func TestServiceStartMediaAllowsSupervisorPrepareForOtherController(t *testing.T) {
	cacheFile := writeManagerCache(t, `[{"device_id":"slot-18","host":"192.168.1.30","manager_host":"manager.local","container_name":"p1-container-18","slot":18}]`)
	store := NewMemoryRTCStateStore()
	store.SetControlState("slot-18", map[string]any{"controller_identity": "owner", "expires_at": float64(4102444800)})
	service := Service{Config: Config{ManagerCacheFile: cacheFile, RTCMediaCameraAddrTemplate: "webrtc://p1/live/{stream_key}"}, RTCState: store}

	payload, err := service.StartMedia(context.Background(), "slot-18", MediaStartRequest{
		ParticipantIdentity: "operator-tab",
		Camera:              true,
		Microphone:          false,
		Activate:            false,
	}, auth.Session{AssigneeID: "sup-001", Role: "supervisor"})
	if err != nil {
		t.Fatalf("StartMedia returned error: %v", err)
	}
	if payload["controller_identity"] != "owner" {
		t.Fatalf("payload = %+v", payload)
	}
}

func TestServiceStartMediaActivationUnavailableForCurrentController(t *testing.T) {
	cacheFile := writeManagerCache(t, `[{"device_id":"slot-18","host":"192.168.1.30","manager_host":"manager.local","container_name":"p1-container-18","slot":18}]`)
	store := NewMemoryRTCStateStore()
	store.SetControlState("slot-18", map[string]any{"controller_identity": "owner", "expires_at": float64(4102444800)})
	service := Service{Config: Config{ManagerCacheFile: cacheFile, RTCMediaCameraAddrTemplate: "webrtc://p1/live/{stream_key}"}, RTCState: store}

	_, err := service.StartMedia(context.Background(), "slot-18", MediaStartRequest{
		ParticipantIdentity: "owner",
		Camera:              true,
		Microphone:          true,
		Activate:            true,
	}, auth.Session{AssigneeID: "cs-001", Role: "cs"})
	if !errors.Is(err, ErrSDKMediaActivationUnavailable) {
		t.Fatalf("error = %v, want %v", err, ErrSDKMediaActivationUnavailable)
	}
}

func TestServiceStartMediaSkipsLegacyActivationForOperatorWithoutStreamInstance(t *testing.T) {
	cacheFile := writeManagerCache(t, `[{"device_id":"slot-18","host":"192.168.1.30","manager_host":"manager.local","container_name":"p1-container-18","slot":18}]`)
	store := NewMemoryRTCStateStore()
	store.SetControlState("slot-18", map[string]any{"controller_identity": "owner", "expires_at": float64(4102444800)})
	service := Service{Config: Config{ManagerCacheFile: cacheFile}, RTCState: store}

	payload, err := service.StartMedia(context.Background(), "slot-18", MediaStartRequest{
		ParticipantIdentity: "fresh-operator-tab",
		Camera:              true,
		Microphone:          true,
		Activate:            true,
	}, auth.Session{AssigneeID: "admin-001", Role: "admin"})
	if err != nil {
		t.Fatalf("StartMedia returned error: %v", err)
	}
	camera := payload["camera"].(map[string]any)
	if camera["status"] != "ignored_legacy_empty_stream_instance" {
		t.Fatalf("camera = %+v", camera)
	}
}

func TestServiceMediaControlEndpointsValidateBeforeUnavailableP1Control(t *testing.T) {
	cacheFile := writeManagerCache(t, `[{"device_id":"slot-18","host":"192.168.1.30","manager_host":"manager.local","container_name":"p1-container-18","slot":18}]`)
	service := Service{Config: Config{ManagerCacheFile: cacheFile}}

	_, err := service.ConfigureCameraStream(context.Background(), "slot-18", CameraStreamRequest{Addr: "webrtc://relay/live/slot-18", StreamType: 2, Resolution: 2, Start: true})
	if !errors.Is(err, ErrSDKMediaControlUnavailable) {
		t.Fatalf("ConfigureCameraStream err = %v, want %v", err, ErrSDKMediaControlUnavailable)
	}
	_, err = service.StopCameraStream(context.Background(), "slot-18")
	if !errors.Is(err, ErrSDKMediaControlUnavailable) {
		t.Fatalf("StopCameraStream err = %v, want %v", err, ErrSDKMediaControlUnavailable)
	}
	_, err = service.AudioPlayback(context.Background(), "slot-18", AudioPlaybackRequest{Action: "play", Path: "/sdcard/input.wav"})
	if !errors.Is(err, ErrSDKMediaControlUnavailable) {
		t.Fatalf("AudioPlayback err = %v, want %v", err, ErrSDKMediaControlUnavailable)
	}
}

func TestServiceMediaControlValidatesRequests(t *testing.T) {
	service := Service{}

	_, err := service.ConfigureCameraStream(context.Background(), "slot-18", CameraStreamRequest{StreamType: 4, Resolution: 2, Start: true})
	var validation MediaValidationError
	if !errors.As(err, &validation) || validation.Detail != "stream_type must be between 1 and 3" {
		t.Fatalf("camera stream err = %v", err)
	}
	_, err = service.AudioPlayback(context.Background(), "slot-18", AudioPlaybackRequest{Action: "pause"})
	if !errors.As(err, &validation) || validation.Detail != "audio action must be play or stop" {
		t.Fatalf("audio err = %v", err)
	}
}

func TestServiceStopMediaRejectsNonControllerBeforeUnavailableP1Control(t *testing.T) {
	cacheFile := writeManagerCache(t, `[{"device_id":"slot-18","host":"192.168.1.30","manager_host":"manager.local","container_name":"p1-container-18","slot":18}]`)
	store := NewMemoryRTCStateStore()
	store.SetControlState("slot-18", map[string]any{"controller_identity": "owner", "expires_at": float64(4102444800)})
	service := Service{Config: Config{ManagerCacheFile: cacheFile}, RTCState: store}

	_, err := service.StopMedia(context.Background(), "slot-18", MediaStopRequest{ParticipantIdentity: "other", Camera: true, Microphone: true}, auth.Session{AssigneeID: "cs-002", Role: "cs"})
	if !errors.Is(err, ErrSDKMediaStopForbidden) {
		t.Fatalf("StopMedia err = %v, want %v", err, ErrSDKMediaStopForbidden)
	}
	_, err = service.StopMedia(context.Background(), "slot-18", MediaStopRequest{ParticipantIdentity: "other", Camera: true, Microphone: true}, auth.Session{AssigneeID: "admin-001", Role: "admin"})
	if !errors.Is(err, ErrSDKMediaControlUnavailable) {
		t.Fatalf("admin StopMedia err = %v, want %v", err, ErrSDKMediaControlUnavailable)
	}
}
