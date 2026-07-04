// Package devicesdkhttp adapts read-only SDK device helper routes to HTTP.
package devicesdkhttp

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"wework-go/internal/auth"
	"wework-go/internal/devicesdk"
)

// Service builds SDK helper payloads.
type Service interface {
	ListDevices(ctx context.Context) (map[string]any, error)
	RefreshDiscovery(ctx context.Context) (map[string]any, error)
	ProbeDiscovery(ctx context.Context, request devicesdk.DiscoveryProbeRequest) (map[string]any, error)
	WebRTC(ctx context.Context, deviceID string, quality string, origin devicesdk.RequestOrigin) (map[string]any, error)
	Status(ctx context.Context, deviceID string, includeManager bool) (map[string]any, error)
	Control(ctx context.Context, deviceID string, taskType string, payload map[string]any) (map[string]any, error)
	RTCSession(ctx context.Context, deviceID string, quality string, mode string, origin devicesdk.RequestOrigin, session auth.Session) (map[string]any, error)
	RTCActive(ctx context.Context, deviceID string, participantIdentity string) (map[string]any, error)
	ListRTCActive(ctx context.Context) (map[string]any, error)
	ControlState(ctx context.Context, deviceID string) (map[string]any, error)
	ControlInput(ctx context.Context, deviceID string, request devicesdk.ControlInputRequest) (map[string]any, error)
	AcquireControl(ctx context.Context, deviceID string, participantIdentity string, session auth.Session) (map[string]any, error)
	ReleaseControl(ctx context.Context, deviceID string, participantIdentity string, session auth.Session) (map[string]any, error)
	StealControl(ctx context.Context, deviceID string, participantIdentity string, session auth.Session) (map[string]any, error)
	StartMedia(ctx context.Context, deviceID string, request devicesdk.MediaStartRequest, session auth.Session) (map[string]any, error)
	ConfigureCameraStream(ctx context.Context, deviceID string, request devicesdk.CameraStreamRequest) (map[string]any, error)
	StopCameraStream(ctx context.Context, deviceID string) (map[string]any, error)
	AudioPlayback(ctx context.Context, deviceID string, request devicesdk.AudioPlaybackRequest) (map[string]any, error)
	StopMedia(ctx context.Context, deviceID string, request devicesdk.MediaStopRequest, session auth.Session) (map[string]any, error)
}

// Handler owns read-only SDK helper routes.
type Handler struct {
	Service              Service
	Guard                auth.Guard
	AgentToken           string
	AllowLegacyAgentAuth bool
}

// New builds an SDK helper HTTP adapter.
func New(service Service, guard auth.Guard) Handler {
	return Handler{Service: service, Guard: guard}
}

// NewWithAgentAuth builds an SDK helper adapter with Bridge agent auth.
func NewWithAgentAuth(service Service, guard auth.Guard, agentToken string, allowLegacyAgentAuth bool) Handler {
	return Handler{Service: service, Guard: guard, AgentToken: agentToken, AllowLegacyAgentAuth: allowLegacyAgentAuth}
}

// ListDevicesHandler serializes GET /api/v1/devices.
func (handler Handler) ListDevicesHandler(w http.ResponseWriter, r *http.Request) {
	if _, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor", "cs"); err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.Service == nil {
		writeError(w, http.StatusServiceUnavailable, "device sdk service is not configured")
		return
	}
	payload, err := handler.Service.ListDevices(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// RefreshDiscoveryHandler serializes POST /api/v1/devices/discovery/refresh.
func (handler Handler) RefreshDiscoveryHandler(w http.ResponseWriter, r *http.Request) {
	if _, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor"); err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.Service == nil {
		writeError(w, http.StatusServiceUnavailable, "device sdk service is not configured")
		return
	}
	payload, err := handler.Service.RefreshDiscovery(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// ProbeDiscoveryHandler serializes POST /api/v1/devices/discovery/probe.
func (handler Handler) ProbeDiscoveryHandler(w http.ResponseWriter, r *http.Request) {
	if _, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor"); err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.Service == nil {
		writeError(w, http.StatusServiceUnavailable, "device sdk service is not configured")
		return
	}
	var request struct {
		DeviceIP       string  `json:"device_ip"`
		ManagerHost    string  `json:"manager_host"`
		ManagerPort    int     `json:"manager_port"`
		SDKHost        string  `json:"sdk_host"`
		WebRTCHost     string  `json:"webrtc_host"`
		TimeoutSec     float64 `json:"timeout_sec"`
		ApplyOnSuccess bool    `json:"apply_on_success"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.UseNumber()
	if err := decoder.Decode(&request); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid json body")
		return
	}
	payload, err := handler.Service.ProbeDiscovery(r.Context(), devicesdk.DiscoveryProbeRequest{
		DeviceIP:       request.DeviceIP,
		ManagerHost:    request.ManagerHost,
		ManagerPort:    request.ManagerPort,
		SDKHost:        request.SDKHost,
		WebRTCHost:     request.WebRTCHost,
		TimeoutSec:     request.TimeoutSec,
		ApplyOnSuccess: request.ApplyOnSuccess,
	})
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, payload)
	case errors.Is(err, devicesdk.ErrSDKDiscoveryProbeTargetRequired):
		writeError(w, http.StatusUnprocessableEntity, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}

// WebRTCHandler serializes GET /api/v1/devices/{device_id}/sdk/webrtc.
func (handler Handler) WebRTCHandler(w http.ResponseWriter, r *http.Request) {
	if _, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor", "cs"); err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.Service == nil {
		writeError(w, http.StatusServiceUnavailable, "device sdk service is not configured")
		return
	}
	quality := r.URL.Query().Get("quality")
	if err := devicesdk.ValidateQuality(quality); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	payload, err := handler.Service.WebRTC(r.Context(), strings.TrimSpace(r.PathValue("device_id")), quality, requestOrigin(r))
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, payload)
	case errors.Is(err, devicesdk.ErrSDKDeviceNotConfigured):
		writeError(w, http.StatusNotFound, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}

// StatusHandler serializes GET /api/v1/devices/{device_id}/sdk/status.
func (handler Handler) StatusHandler(w http.ResponseWriter, r *http.Request) {
	if _, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor", "cs"); err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.Service == nil {
		writeError(w, http.StatusServiceUnavailable, "device sdk service is not configured")
		return
	}
	includeManager, err := parseLegacyBool(r.URL.Query().Get("include_manager"), true)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	payload, err := handler.Service.Status(r.Context(), strings.TrimSpace(r.PathValue("device_id")), includeManager)
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, payload)
	case errors.Is(err, devicesdk.ErrSDKDeviceNotConfigured):
		writeError(w, http.StatusNotFound, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}

// RTCSessionHandler serializes GET /api/v1/devices/{device_id}/sdk/rtc-session.
func (handler Handler) RTCSessionHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor", "cs")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.Service == nil {
		writeError(w, http.StatusServiceUnavailable, "device sdk service is not configured")
		return
	}
	quality := r.URL.Query().Get("quality")
	if err := devicesdk.ValidateQuality(quality); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	mode := strings.TrimSpace(r.URL.Query().Get("mode"))
	if mode == "" {
		mode = "auto"
	}
	if mode != "auto" && mode != "legacy" && mode != "livekit" {
		writeError(w, http.StatusUnprocessableEntity, "mode must be auto, legacy, or livekit")
		return
	}
	payload, err := handler.Service.RTCSession(r.Context(), strings.TrimSpace(r.PathValue("device_id")), quality, mode, requestOrigin(r), session)
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, payload)
	case errors.Is(err, devicesdk.ErrSDKDeviceNotConfigured):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, devicesdk.ErrSDKLegacyRTCDisabled):
		writeError(w, http.StatusGone, err.Error())
	case errors.Is(err, devicesdk.ErrSDKLiveKitNotConfigured):
		writeError(w, http.StatusServiceUnavailable, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}

// RTCActiveHandler serializes POST /api/v1/devices/{device_id}/rtc-active.
func (handler Handler) RTCActiveHandler(w http.ResponseWriter, r *http.Request) {
	if _, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor", "cs"); err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.Service == nil {
		writeError(w, http.StatusServiceUnavailable, "device sdk service is not configured")
		return
	}
	var request struct {
		ParticipantIdentity string `json:"participant_identity"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.UseNumber()
	if err := decoder.Decode(&request); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid json body")
		return
	}
	payload, err := handler.Service.RTCActive(r.Context(), strings.TrimSpace(r.PathValue("device_id")), request.ParticipantIdentity)
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, payload)
	case errors.Is(err, devicesdk.ErrSDKDeviceNotConfigured):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, devicesdk.ErrSDKParticipantIdentityRequired):
		writeError(w, http.StatusUnprocessableEntity, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}

// ListRTCActiveHandler serializes GET /api/v1/devices/rtc/active.
func (handler Handler) ListRTCActiveHandler(w http.ResponseWriter, r *http.Request) {
	if !handler.requireAnyAuth(r.Context(), r.Header.Get("Authorization"), r.Header.Get("X-Agent-Token"), w) {
		return
	}
	if handler.Service == nil {
		writeError(w, http.StatusServiceUnavailable, "device sdk service is not configured")
		return
	}
	payload, err := handler.Service.ListRTCActive(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// ControlStateHandler serializes GET /api/v1/devices/{device_id}/control/state.
func (handler Handler) ControlStateHandler(w http.ResponseWriter, r *http.Request) {
	if !handler.requireAnyAuth(r.Context(), r.Header.Get("Authorization"), r.Header.Get("X-Agent-Token"), w) {
		return
	}
	if handler.Service == nil {
		writeError(w, http.StatusServiceUnavailable, "device sdk service is not configured")
		return
	}
	payload, err := handler.Service.ControlState(r.Context(), strings.TrimSpace(r.PathValue("device_id")))
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, payload)
	case errors.Is(err, devicesdk.ErrSDKDeviceNotConfigured):
		writeError(w, http.StatusNotFound, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}

// ControlInputHandler serializes POST /api/v1/devices/{device_id}/control/input.
func (handler Handler) ControlInputHandler(w http.ResponseWriter, r *http.Request) {
	if !handler.requireAnyAuth(r.Context(), r.Header.Get("Authorization"), r.Header.Get("X-Agent-Token"), w) {
		return
	}
	if handler.Service == nil {
		writeError(w, http.StatusServiceUnavailable, "device sdk service is not configured")
		return
	}
	request, ok := readControlInputRequest(w, r)
	if !ok {
		return
	}
	payload, err := handler.Service.ControlInput(r.Context(), strings.TrimSpace(r.PathValue("device_id")), request)
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, payload)
	case errors.Is(err, devicesdk.ErrSDKDeviceNotConfigured):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, devicesdk.ErrSDKParticipantIdentityRequired):
		writeError(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, devicesdk.ErrSDKControlInputForbidden):
		writeError(w, http.StatusForbidden, err.Error())
	case errors.Is(err, devicesdk.ErrSDKControlInputUnavailable):
		writeError(w, http.StatusServiceUnavailable, err.Error())
	case errors.Is(err, devicesdk.ErrSDKControlInputFailed):
		writeError(w, http.StatusBadGateway, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}

// AcquireControlHandler serializes POST /api/v1/devices/{device_id}/control/acquire.
func (handler Handler) AcquireControlHandler(w http.ResponseWriter, r *http.Request) {
	session, ok := handler.requireControlRole(w, r, "admin", "supervisor", "cs")
	if !ok {
		return
	}
	participantIdentity, ok := readParticipantIdentity(w, r)
	if !ok {
		return
	}
	payload, err := handler.Service.AcquireControl(r.Context(), strings.TrimSpace(r.PathValue("device_id")), participantIdentity, session)
	var conflict devicesdk.ControlConflictError
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, payload)
	case errors.As(err, &conflict):
		writeJSON(w, http.StatusConflict, map[string]any{"detail": conflict.State})
	case errors.Is(err, devicesdk.ErrSDKDeviceNotConfigured):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, devicesdk.ErrSDKParticipantIdentityRequired):
		writeError(w, http.StatusUnprocessableEntity, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}

// ReleaseControlHandler serializes POST /api/v1/devices/{device_id}/control/release.
func (handler Handler) ReleaseControlHandler(w http.ResponseWriter, r *http.Request) {
	session, ok := handler.requireControlRole(w, r, "admin", "supervisor", "cs")
	if !ok {
		return
	}
	participantIdentity, ok := readParticipantIdentity(w, r)
	if !ok {
		return
	}
	payload, err := handler.Service.ReleaseControl(r.Context(), strings.TrimSpace(r.PathValue("device_id")), participantIdentity, session)
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, payload)
	case errors.Is(err, devicesdk.ErrSDKDeviceNotConfigured):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, devicesdk.ErrSDKControlReleaseForbidden):
		writeError(w, http.StatusForbidden, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}

// StealControlHandler serializes POST /api/v1/devices/{device_id}/control/steal.
func (handler Handler) StealControlHandler(w http.ResponseWriter, r *http.Request) {
	session, ok := handler.requireControlRole(w, r, "admin", "supervisor")
	if !ok {
		return
	}
	participantIdentity, ok := readParticipantIdentity(w, r)
	if !ok {
		return
	}
	payload, err := handler.Service.StealControl(r.Context(), strings.TrimSpace(r.PathValue("device_id")), participantIdentity, session)
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, payload)
	case errors.Is(err, devicesdk.ErrSDKDeviceNotConfigured):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, devicesdk.ErrSDKParticipantIdentityRequired):
		writeError(w, http.StatusUnprocessableEntity, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}

// StartMediaHandler serializes POST /api/v1/devices/{device_id}/media/start.
func (handler Handler) StartMediaHandler(w http.ResponseWriter, r *http.Request) {
	session, ok := handler.requireControlRole(w, r, "admin", "supervisor", "cs")
	if !ok {
		return
	}
	request, ok := readMediaStartRequest(w, r)
	if !ok {
		return
	}
	payload, err := handler.Service.StartMedia(r.Context(), strings.TrimSpace(r.PathValue("device_id")), request, session)
	var validation devicesdk.MediaValidationError
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, payload)
	case errors.Is(err, devicesdk.ErrSDKDeviceNotConfigured):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, devicesdk.ErrSDKMediaStartForbidden):
		writeError(w, http.StatusForbidden, err.Error())
	case errors.Is(err, devicesdk.ErrSDKMediaActivationUnavailable):
		writeError(w, http.StatusServiceUnavailable, err.Error())
	case errors.As(err, &validation):
		writeError(w, http.StatusUnprocessableEntity, validation.Error())
	default:
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}

// CameraStreamHandler serializes POST /api/v1/devices/{device_id}/media/camera-stream.
func (handler Handler) CameraStreamHandler(w http.ResponseWriter, r *http.Request) {
	if _, ok := handler.requireControlRole(w, r, "admin", "supervisor"); !ok {
		return
	}
	request, ok := readCameraStreamRequest(w, r)
	if !ok {
		return
	}
	payload, err := handler.Service.ConfigureCameraStream(r.Context(), strings.TrimSpace(r.PathValue("device_id")), request)
	handler.writeMediaControlResponse(w, payload, err)
}

// StopCameraStreamHandler serializes DELETE /api/v1/devices/{device_id}/media/camera-stream.
func (handler Handler) StopCameraStreamHandler(w http.ResponseWriter, r *http.Request) {
	if _, ok := handler.requireControlRole(w, r, "admin", "supervisor"); !ok {
		return
	}
	payload, err := handler.Service.StopCameraStream(r.Context(), strings.TrimSpace(r.PathValue("device_id")))
	handler.writeMediaControlResponse(w, payload, err)
}

// AudioPlaybackHandler serializes POST /api/v1/devices/{device_id}/media/audio.
func (handler Handler) AudioPlaybackHandler(w http.ResponseWriter, r *http.Request) {
	if _, ok := handler.requireControlRole(w, r, "admin", "supervisor"); !ok {
		return
	}
	request, ok := readAudioPlaybackRequest(w, r)
	if !ok {
		return
	}
	payload, err := handler.Service.AudioPlayback(r.Context(), strings.TrimSpace(r.PathValue("device_id")), request)
	handler.writeMediaControlResponse(w, payload, err)
}

// StopMediaHandler serializes POST /api/v1/devices/{device_id}/media/stop.
func (handler Handler) StopMediaHandler(w http.ResponseWriter, r *http.Request) {
	session, ok := handler.requireControlRole(w, r, "admin", "supervisor", "cs")
	if !ok {
		return
	}
	request, ok := readMediaStopRequest(w, r)
	if !ok {
		return
	}
	payload, err := handler.Service.StopMedia(r.Context(), strings.TrimSpace(r.PathValue("device_id")), request, session)
	handler.writeMediaControlResponse(w, payload, err)
}

func (handler Handler) writeMediaControlResponse(w http.ResponseWriter, payload map[string]any, err error) {
	var validation devicesdk.MediaValidationError
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, payload)
	case errors.Is(err, devicesdk.ErrSDKDeviceNotConfigured):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, devicesdk.ErrSDKMediaStopForbidden):
		writeError(w, http.StatusForbidden, err.Error())
	case errors.Is(err, devicesdk.ErrSDKMediaControlUnavailable):
		writeError(w, http.StatusServiceUnavailable, err.Error())
	case errors.As(err, &validation):
		writeError(w, http.StatusUnprocessableEntity, validation.Error())
	default:
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}

// OpenWeWorkHandler serializes POST /api/v1/devices/{device_id}/sdk/open-wework.
func (handler Handler) OpenWeWorkHandler(w http.ResponseWriter, r *http.Request) {
	handler.controlHandler(w, r, []string{"admin", "supervisor"}, "device_open_app", map[string]any{
		"username":     "__device__",
		"package_name": "com.tencent.wework",
	})
}

// StopWeWorkHandler serializes POST /api/v1/devices/{device_id}/sdk/stop-wework.
func (handler Handler) StopWeWorkHandler(w http.ResponseWriter, r *http.Request) {
	handler.controlHandler(w, r, []string{"admin", "supervisor"}, "device_stop_app", map[string]any{
		"username":     "__device__",
		"package_name": "com.tencent.wework",
	})
}

// PrepareCallAudioOutputHandler serializes POST /api/v1/devices/{device_id}/sdk/prepare-call-audio-output.
func (handler Handler) PrepareCallAudioOutputHandler(w http.ResponseWriter, r *http.Request) {
	if _, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor", "cs"); err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.Service == nil {
		writeError(w, http.StatusServiceUnavailable, "device sdk service is not configured")
		return
	}
	callType := strings.TrimSpace(r.URL.Query().Get("call_type"))
	if callType == "" {
		callType = "voice"
	}
	if callType != "voice" && callType != "video" {
		writeError(w, http.StatusUnprocessableEntity, "call_type must be voice or video")
		return
	}
	handler.writeControlResponse(w, r, "wework_prepare_call_audio_output", map[string]any{
		"username":  "__device__",
		"call_type": callType,
	})
}

func (handler Handler) controlHandler(w http.ResponseWriter, r *http.Request, roles []string, taskType string, payload map[string]any) {
	if _, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), roles...); err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.Service == nil {
		writeError(w, http.StatusServiceUnavailable, "device sdk service is not configured")
		return
	}
	handler.writeControlResponse(w, r, taskType, payload)
}

func (handler Handler) writeControlResponse(w http.ResponseWriter, r *http.Request, taskType string, payload map[string]any) {
	response, err := handler.Service.Control(r.Context(), strings.TrimSpace(r.PathValue("device_id")), taskType, payload)
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, response)
	case errors.Is(err, devicesdk.ErrSDKDeviceNotConfigured):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, devicesdk.ErrSDKTaskServiceNotConfigured):
		writeError(w, http.StatusServiceUnavailable, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}

func (handler Handler) requireControlRole(w http.ResponseWriter, r *http.Request, roles ...string) (auth.Session, bool) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), roles...)
	if err != nil {
		writeAuthError(w, err)
		return auth.Session{}, false
	}
	if handler.Service == nil {
		writeError(w, http.StatusServiceUnavailable, "device sdk service is not configured")
		return auth.Session{}, false
	}
	return session, true
}

func readParticipantIdentity(w http.ResponseWriter, r *http.Request) (string, bool) {
	var request struct {
		ParticipantIdentity string `json:"participant_identity"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.UseNumber()
	if err := decoder.Decode(&request); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid json body")
		return "", false
	}
	return request.ParticipantIdentity, true
}

func readControlInputRequest(w http.ResponseWriter, r *http.Request) (devicesdk.ControlInputRequest, bool) {
	var request struct {
		ParticipantIdentity string  `json:"participant_identity"`
		Kind                string  `json:"kind"`
		Action              string  `json:"action"`
		X                   float64 `json:"x"`
		Y                   float64 `json:"y"`
		DeltaX              float64 `json:"delta_x"`
		DeltaY              float64 `json:"delta_y"`
		Key                 string  `json:"key"`
		Text                string  `json:"text"`
		TimestampMillis     int64   `json:"ts"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.UseNumber()
	if err := decoder.Decode(&request); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid json body")
		return devicesdk.ControlInputRequest{}, false
	}
	return devicesdk.ControlInputRequest{
		ParticipantIdentity: request.ParticipantIdentity,
		Kind:                request.Kind,
		Action:              request.Action,
		X:                   request.X,
		Y:                   request.Y,
		DeltaX:              request.DeltaX,
		DeltaY:              request.DeltaY,
		Key:                 request.Key,
		Text:                request.Text,
		TimestampMillis:     request.TimestampMillis,
	}, true
}

func readMediaStartRequest(w http.ResponseWriter, r *http.Request) (devicesdk.MediaStartRequest, bool) {
	var request struct {
		ParticipantIdentity string `json:"participant_identity"`
		StreamInstance      string `json:"stream_instance"`
		Camera              *bool  `json:"camera"`
		Microphone          *bool  `json:"microphone"`
		Activate            *bool  `json:"activate"`
		CameraAddr          string `json:"camera_addr"`
		WHIPPublishURL      string `json:"whip_publish_url"`
		CameraStreamType    *int   `json:"camera_stream_type"`
		CameraResolution    *int   `json:"camera_resolution"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.UseNumber()
	if err := decoder.Decode(&request); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid json body")
		return devicesdk.MediaStartRequest{}, false
	}
	camera := true
	if request.Camera != nil {
		camera = *request.Camera
	}
	microphone := true
	if request.Microphone != nil {
		microphone = *request.Microphone
	}
	activate := true
	if request.Activate != nil {
		activate = *request.Activate
	}
	return devicesdk.MediaStartRequest{
		ParticipantIdentity: request.ParticipantIdentity,
		StreamInstance:      request.StreamInstance,
		Camera:              camera,
		Microphone:          microphone,
		Activate:            activate,
		CameraAddr:          request.CameraAddr,
		WHIPPublishURL:      request.WHIPPublishURL,
		CameraStreamType:    request.CameraStreamType,
		CameraResolution:    request.CameraResolution,
	}, true
}

func readCameraStreamRequest(w http.ResponseWriter, r *http.Request) (devicesdk.CameraStreamRequest, bool) {
	var request struct {
		Addr       string `json:"addr"`
		StreamType *int   `json:"stream_type"`
		Resolution *int   `json:"resolution"`
		Start      *bool  `json:"start"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.UseNumber()
	if err := decoder.Decode(&request); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid json body")
		return devicesdk.CameraStreamRequest{}, false
	}
	streamType := 2
	if request.StreamType != nil {
		streamType = *request.StreamType
	}
	resolution := 2
	if request.Resolution != nil {
		resolution = *request.Resolution
	}
	start := true
	if request.Start != nil {
		start = *request.Start
	}
	return devicesdk.CameraStreamRequest{
		Addr:       request.Addr,
		StreamType: streamType,
		Resolution: resolution,
		Start:      start,
	}, true
}

func readAudioPlaybackRequest(w http.ResponseWriter, r *http.Request) (devicesdk.AudioPlaybackRequest, bool) {
	var request struct {
		Path   string `json:"path"`
		Action string `json:"action"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.UseNumber()
	if err := decoder.Decode(&request); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid json body")
		return devicesdk.AudioPlaybackRequest{}, false
	}
	action := strings.TrimSpace(request.Action)
	if action == "" {
		action = "play"
	}
	return devicesdk.AudioPlaybackRequest{Path: request.Path, Action: action}, true
}

func readMediaStopRequest(w http.ResponseWriter, r *http.Request) (devicesdk.MediaStopRequest, bool) {
	var request struct {
		ParticipantIdentity string `json:"participant_identity"`
		Camera              *bool  `json:"camera"`
		Microphone          *bool  `json:"microphone"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.UseNumber()
	if err := decoder.Decode(&request); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid json body")
		return devicesdk.MediaStopRequest{}, false
	}
	camera := true
	if request.Camera != nil {
		camera = *request.Camera
	}
	microphone := true
	if request.Microphone != nil {
		microphone = *request.Microphone
	}
	return devicesdk.MediaStopRequest{
		ParticipantIdentity: request.ParticipantIdentity,
		Camera:              camera,
		Microphone:          microphone,
	}, true
}

func (handler Handler) requireAnyAuth(ctx context.Context, authorization string, agentToken string, w http.ResponseWriter) bool {
	if auth.ParseBearerToken(authorization) != "" {
		if _, err := handler.Guard.RequireRoles(ctx, authorization); err == nil {
			return true
		}
	}
	expectedAgentToken := strings.TrimSpace(handler.AgentToken)
	if expectedAgentToken != "" && subtle.ConstantTimeCompare([]byte(strings.TrimSpace(agentToken)), []byte(expectedAgentToken)) == 1 {
		return true
	}
	if handler.AllowLegacyAgentAuth {
		return true
	}
	writeError(w, http.StatusUnauthorized, "authentication required")
	return false
}

func requestOrigin(r *http.Request) devicesdk.RequestOrigin {
	proto := firstForwarded(r.Header.Get("X-Forwarded-Proto"))
	if proto == "" {
		if r.TLS != nil {
			proto = "https"
		} else {
			proto = "http"
		}
	}
	host := firstForwarded(r.Header.Get("X-Forwarded-Host"))
	if host == "" {
		host = strings.TrimSpace(r.Host)
	}
	return devicesdk.RequestOrigin{Scheme: proto, Host: host}
}

func firstForwarded(value string) string {
	return strings.TrimSpace(strings.Split(strings.TrimSpace(value), ",")[0])
}

func parseLegacyBool(value string, fallback bool) (bool, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return fallback, nil
	}
	switch normalized {
	case "1", "true", "t", "yes", "y", "on":
		return true, nil
	case "0", "false", "f", "no", "n", "off":
		return false, nil
	default:
		return false, errors.New("include_manager must be a boolean")
	}
}

func writeAuthError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, auth.ErrMissingBearerToken):
		writeError(w, http.StatusUnauthorized, "missing bearer token")
	case errors.Is(err, auth.ErrInvalidOrExpiredSession):
		writeError(w, http.StatusUnauthorized, "session invalid or expired")
	case errors.Is(err, auth.ErrPermissionDenied):
		writeError(w, http.StatusForbidden, "permission denied")
	default:
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, detail string) {
	writeJSON(w, status, map[string]string{"detail": detail})
}
