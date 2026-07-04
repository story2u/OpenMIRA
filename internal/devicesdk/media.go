package devicesdk

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"wework-go/internal/auth"
)

// MediaValidationError represents Python-compatible 422 media request failures.
type MediaValidationError struct {
	Detail string
}

func (err MediaValidationError) Error() string {
	return strings.TrimSpace(err.Detail)
}

// MediaStartRequest is the normalized /devices/{device_id}/media/start body.
type MediaStartRequest struct {
	ParticipantIdentity string
	StreamInstance      string
	Camera              bool
	Microphone          bool
	Activate            bool
	CameraAddr          string
	WHIPPublishURL      string
	CameraStreamType    *int
	CameraResolution    *int
}

// CameraStreamRequest is the normalized /media/camera-stream body.
type CameraStreamRequest struct {
	Addr       string
	StreamType int
	Resolution int
	Start      bool
}

// AudioPlaybackRequest is the normalized /media/audio body.
type AudioPlaybackRequest struct {
	Path   string
	Action string
}

// MediaStopRequest is the normalized /media/stop body.
type MediaStopRequest struct {
	ParticipantIdentity string
	Camera              bool
	Microphone          bool
}

var mediaInstanceMemory = struct {
	mu     sync.Mutex
	values map[string]mediaInstanceState
}{
	values: map[string]mediaInstanceState{},
}

type mediaInstanceState struct {
	StreamInstance string
	ExpiresAt      time.Time
}

// StartMedia returns the legacy media-start payload for prepare-only requests.
func (service Service) StartMedia(ctx context.Context, deviceID string, request MediaStartRequest, session auth.Session) (map[string]any, error) {
	if err := request.validate(); err != nil {
		return nil, err
	}
	managerDevice, ok := service.findSlot(deviceID)
	if !ok {
		return nil, ErrSDKDeviceNotConfigured
	}
	slot := legacySlot(managerDevice, deviceID)
	if err := validateMediaSlot(slot); err != nil {
		return nil, err
	}
	canonicalDeviceID := clean(firstValue(slot["device_id"], deviceID))
	now := service.now()
	controlState, err := service.activeControlState(ctx, canonicalDeviceID, now)
	if err != nil {
		return nil, err
	}
	participantIdentity := strings.TrimSpace(request.ParticipantIdentity)
	role := strings.TrimSpace(session.Role)
	controllerIdentity := strings.TrimSpace(stringValue(controlState["controller_identity"]))
	isCurrentController := controlState != nil && controllerIdentity == participantIdentity
	if controlState == nil || (!isCurrentController && role != "admin" && role != "supervisor") {
		return nil, ErrSDKMediaStartForbidden
	}

	roomName := service.deviceRoomName(canonicalDeviceID)
	request.StreamInstance = service.resolveMediaStreamInstance(
		canonicalDeviceID,
		participantIdentity,
		request.StreamInstance,
		!request.Activate && (request.Camera || request.Microphone),
		now,
	)
	if shouldSkipLegacyMediaActivation(request, isCurrentController) {
		skipped := map[string]any{
			"status":          "ignored_legacy_empty_stream_instance",
			"detail":          "legacy clients must prepare media or refresh before activating P1 stream source",
			"consumer_status": "legacy_client_must_prepare_or_refresh",
		}
		return map[string]any{
			"success":             true,
			"device_id":           canonicalDeviceID,
			"room_name":           roomName,
			"controller_identity": controllerIdentity,
			"camera":              chooseMediaPayload(request.Camera, skipped),
			"audio":               chooseMediaPayload(request.Microphone, skipped),
		}, nil
	}
	if request.Activate && (request.Camera || request.Microphone) {
		return nil, ErrSDKMediaActivationUnavailable
	}

	camera, audio := service.prepareMediaInputs(request, canonicalDeviceID, roomName, participantIdentity)
	return map[string]any{
		"success":             true,
		"device_id":           canonicalDeviceID,
		"room_name":           roomName,
		"controller_identity": controllerIdentity,
		"camera":              camera,
		"audio":               audio,
	}, nil
}

// ConfigureCameraStream validates the legacy P1 camera-stream request boundary.
func (service Service) ConfigureCameraStream(ctx context.Context, deviceID string, request CameraStreamRequest) (map[string]any, error) {
	_ = ctx
	if err := request.validate(); err != nil {
		return nil, err
	}
	managerDevice, ok := service.findSlot(deviceID)
	if !ok {
		return nil, ErrSDKDeviceNotConfigured
	}
	slot := legacySlot(managerDevice, deviceID)
	if err := validateMediaSlot(slot); err != nil {
		return nil, err
	}
	return nil, ErrSDKMediaControlUnavailable
}

// StopCameraStream validates the legacy P1 camera stop request boundary.
func (service Service) StopCameraStream(ctx context.Context, deviceID string) (map[string]any, error) {
	_ = ctx
	managerDevice, ok := service.findSlot(deviceID)
	if !ok {
		return nil, ErrSDKDeviceNotConfigured
	}
	slot := legacySlot(managerDevice, deviceID)
	if err := validateMediaSlot(slot); err != nil {
		return nil, err
	}
	return nil, ErrSDKMediaControlUnavailable
}

// AudioPlayback validates the legacy P1 audio playback request boundary.
func (service Service) AudioPlayback(ctx context.Context, deviceID string, request AudioPlaybackRequest) (map[string]any, error) {
	_ = ctx
	if err := request.validate(); err != nil {
		return nil, err
	}
	managerDevice, ok := service.findSlot(deviceID)
	if !ok {
		return nil, ErrSDKDeviceNotConfigured
	}
	slot := legacySlot(managerDevice, deviceID)
	if err := validateMediaSlot(slot); err != nil {
		return nil, err
	}
	return nil, ErrSDKMediaControlUnavailable
}

// StopMedia validates controller ownership before the future P1 media stop path.
func (service Service) StopMedia(ctx context.Context, deviceID string, request MediaStopRequest, session auth.Session) (map[string]any, error) {
	managerDevice, ok := service.findSlot(deviceID)
	if !ok {
		return nil, ErrSDKDeviceNotConfigured
	}
	slot := legacySlot(managerDevice, deviceID)
	canonicalDeviceID := clean(firstValue(slot["device_id"], deviceID))
	controlState, err := service.activeControlState(ctx, canonicalDeviceID, service.now())
	if err != nil {
		return nil, err
	}
	callerIdentity := strings.TrimSpace(request.ParticipantIdentity)
	role := strings.TrimSpace(session.Role)
	if controlState != nil && strings.TrimSpace(stringValue(controlState["controller_identity"])) != callerIdentity && role != "admin" && role != "supervisor" {
		return nil, ErrSDKMediaStopForbidden
	}
	if err := validateMediaSlot(slot); err != nil {
		return nil, err
	}
	return nil, ErrSDKMediaControlUnavailable
}

func (request MediaStartRequest) validate() error {
	if request.CameraStreamType != nil && (*request.CameraStreamType < 1 || *request.CameraStreamType > 3) {
		return MediaValidationError{Detail: "camera_stream_type must be between 1 and 3"}
	}
	if request.CameraResolution != nil && (*request.CameraResolution < 1 || *request.CameraResolution > 2) {
		return MediaValidationError{Detail: "camera_resolution must be between 1 and 2"}
	}
	return nil
}

func (request CameraStreamRequest) validate() error {
	if request.StreamType < 1 || request.StreamType > 3 {
		return MediaValidationError{Detail: "stream_type must be between 1 and 3"}
	}
	if request.Resolution < 1 || request.Resolution > 2 {
		return MediaValidationError{Detail: "resolution must be between 1 and 2"}
	}
	return nil
}

func (request AudioPlaybackRequest) validate() error {
	action := strings.ToLower(strings.TrimSpace(request.Action))
	if action == "" {
		action = "play"
	}
	if action != "play" && action != "stop" {
		return MediaValidationError{Detail: "audio action must be play or stop"}
	}
	return nil
}

func (service Service) activeControlState(ctx context.Context, deviceID string, now time.Time) (map[string]any, error) {
	state, err := service.rtcStateStore().ReadControlState(ctx, deviceID)
	if err != nil {
		return nil, err
	}
	if len(state) == 0 {
		return nil, nil
	}
	expiresAt := floatValue(state["expires_at"])
	if expiresAt > 0 && expiresAt <= unixSeconds(now) {
		return nil, nil
	}
	if strings.TrimSpace(stringValue(state["controller_identity"])) == "" {
		return nil, nil
	}
	return cloneMap(state), nil
}

func validateMediaSlot(slot map[string]any) error {
	if strings.TrimSpace(stringValue(slot["container_name"])) == "" {
		return MediaValidationError{Detail: "P1 container_name is required"}
	}
	managerHost := strings.TrimSpace(firstNonEmpty(
		stringValue(slot["manager_host"]),
		stringValue(slot["device_ip"]),
		stringValue(slot["host"]),
	))
	deviceIP := strings.TrimSpace(firstNonEmpty(
		stringValue(slot["manager_device_ip"]),
		stringValue(slot["device_ip"]),
		stringValue(slot["host"]),
		managerHost,
	))
	if managerHost == "" || deviceIP == "" {
		return MediaValidationError{Detail: "P1 manager host/device ip is required"}
	}
	return nil
}

func (service Service) resolveMediaStreamInstance(deviceID string, participantIdentity string, requested string, rotate bool, now time.Time) string {
	requested = strings.TrimSpace(requested)
	if requested != "" {
		instance := SanitizeRTCSegment(requested)
		writeMediaInstance(deviceID, participantIdentity, instance, now.Add(service.mediaInstanceTTL()))
		return instance
	}
	key := mediaInstanceKey(deviceID, participantIdentity)
	mediaInstanceMemory.mu.Lock()
	defer mediaInstanceMemory.mu.Unlock()
	state, ok := mediaInstanceMemory.values[key]
	if ok && !state.ExpiresAt.IsZero() && !state.ExpiresAt.After(now) {
		delete(mediaInstanceMemory.values, key)
		ok = false
	}
	if rotate {
		instance := SanitizeRTCSegment(fmt.Sprintf("%x-%s", now.UnixMilli(), service.identitySuffix()))
		mediaInstanceMemory.values[key] = mediaInstanceState{StreamInstance: instance, ExpiresAt: now.Add(service.mediaInstanceTTL())}
		return instance
	}
	if ok {
		return strings.TrimSpace(state.StreamInstance)
	}
	return ""
}

func writeMediaInstance(deviceID string, participantIdentity string, streamInstance string, expiresAt time.Time) {
	key := mediaInstanceKey(deviceID, participantIdentity)
	mediaInstanceMemory.mu.Lock()
	defer mediaInstanceMemory.mu.Unlock()
	mediaInstanceMemory.values[key] = mediaInstanceState{StreamInstance: streamInstance, ExpiresAt: expiresAt}
}

func mediaInstanceKey(deviceID string, participantIdentity string) string {
	return SanitizeRTCSegment(deviceID) + ":" + SanitizeRTCSegment(participantIdentity)
}

func (service Service) mediaInstanceTTL() time.Duration {
	seconds := service.Config.RTCMediaInstanceTTLSeconds
	if seconds < 60 {
		seconds = 3600
	}
	if seconds > 21600 {
		seconds = 21600
	}
	return time.Duration(seconds) * time.Second
}

func shouldSkipLegacyMediaActivation(request MediaStartRequest, isCurrentController bool) bool {
	return request.Activate &&
		(request.Camera || request.Microphone) &&
		strings.TrimSpace(request.StreamInstance) == "" &&
		!isCurrentController
}

func chooseMediaPayload(enabled bool, payload map[string]any) map[string]any {
	if enabled {
		return cloneMap(payload)
	}
	return map[string]any{"status": "disabled"}
}

func (service Service) prepareMediaInputs(request MediaStartRequest, deviceID string, roomName string, participantIdentity string) (map[string]any, map[string]any) {
	camera := map[string]any{"status": "disabled"}
	audio := map[string]any{"status": "disabled"}
	if !request.Camera && !request.Microphone {
		return camera, audio
	}
	stream := service.prepareAVStream(request, deviceID, roomName, participantIdentity)
	if request.Camera {
		camera = stream
	}
	if request.Microphone {
		audio = map[string]any{
			"status":                        stream["status"],
			"transport":                     "camera_stream",
			"addr":                          stream["addr"],
			"stream_key":                    stream["stream_key"],
			"playback_url":                  stream["playback_url"],
			"preview_url":                   stream["preview_url"],
			"p1_signaling_url":              stream["p1_signaling_url"],
			"p1_requires_srs_bridge":        boolValue(stream["p1_requires_srs_bridge"]),
			"playback_url_has_port":         boolValue(stream["playback_url_has_port"]),
			"publish_url":                   stream["publish_url"],
			"direct_publish_url":            stream["direct_publish_url"],
			"publish_url_candidates":        stream["publish_url_candidates"],
			"publish_protocol":              stream["publish_protocol"],
			"playback_protocol":             stream["playback_protocol"],
			"stream_type":                   stream["stream_type"],
			"stream_type_label":             stream["stream_type_label"],
			"direct_open_supported":         boolValue(stream["direct_open_supported"]),
			"consumer_status":               stream["consumer_status"],
			"consumer_detail":               stream["consumer_detail"],
			"p1_signaling_probe":            map[string]any{},
			"p1_signaling_probe_status":     "",
			"camera_client_probe":           map[string]any{},
			"camera_client_probe_status":    "",
			"android_modifydev":             map[string]any{},
			"camera_client_reopen_required": boolValue(stream["camera_client_reopen_required"]),
			"detail":                        stream["detail"],
		}
		audio["detail"] = "microphone audio is expected inside the AV relay stream"
	}
	return camera, audio
}

func (service Service) prepareAVStream(request MediaStartRequest, deviceID string, roomName string, participantIdentity string) map[string]any {
	cameraTemplate := strings.TrimSpace(firstNonEmpty(request.CameraAddr, service.Config.RTCMediaCameraAddrTemplate))
	if cameraTemplate == "" {
		return map[string]any{"status": "not_configured", "detail": "RTC_MEDIA_CAMERA_ADDR_TEMPLATE is empty"}
	}
	streamKey := service.mediaStreamKey(deviceID, participantIdentity, request.StreamInstance)
	cameraAddr := service.repairP1PlaybackAddr(normalizeMediaAddr(formatMediaTemplate(cameraTemplate, deviceID, roomName, participantIdentity, request.StreamInstance, streamKey)))
	publishURL := normalizeMediaAddr(formatMediaTemplate(firstNonEmpty(request.WHIPPublishURL, service.Config.RTCMediaWHIPPublishURLTemplate), deviceID, roomName, participantIdentity, request.StreamInstance, streamKey))
	directPublishURL := service.directPublishURL(request, deviceID, roomName, participantIdentity)
	streamType := service.cameraStreamType(request, cameraAddr)
	base := streamResultBase(cameraAddr, publishURL, directPublishURL, streamType, streamKey)
	base["status"] = "prepared"
	return base
}

func (service Service) mediaStreamKey(deviceID string, participantIdentity string, streamInstance string) string {
	deviceSegment := SanitizeRTCSegment(deviceID)
	if !service.Config.RTCMediaStableStreamKeyDisabled {
		return SanitizeRTCSegment(deviceSegment + "-input")
	}
	participantSegment := SanitizeRTCSegment(participantIdentity)
	instanceSegment := ""
	if strings.TrimSpace(streamInstance) != "" {
		instanceSegment = SanitizeRTCSegment(streamInstance)
	}
	if instanceSegment != "" {
		return SanitizeRTCSegment(deviceSegment + "-" + participantSegment + "-" + instanceSegment)
	}
	return SanitizeRTCSegment(deviceSegment + "-" + participantSegment)
}

func formatMediaTemplate(template string, deviceID string, roomName string, participantIdentity string, streamInstance string, streamKey string) string {
	template = strings.TrimSpace(template)
	if template == "" {
		return ""
	}
	deviceSegment := SanitizeRTCSegment(deviceID)
	participantSegment := SanitizeRTCSegment(participantIdentity)
	instanceSegment := ""
	if strings.TrimSpace(streamInstance) != "" {
		instanceSegment = SanitizeRTCSegment(streamInstance)
	}
	replacer := strings.NewReplacer(
		"{device_id}", deviceSegment,
		"{room_name}", SanitizeRTCSegment(roomName),
		"{participant_identity}", participantSegment,
		"{stream_instance}", instanceSegment,
		"{stream_key}", streamKey,
		"{stream_name}", streamKey,
	)
	return replacer.Replace(template)
}

func (service Service) directPublishURL(request MediaStartRequest, deviceID string, roomName string, participantIdentity string) string {
	streamKey := service.mediaStreamKey(deviceID, participantIdentity, request.StreamInstance)
	directURL := normalizeMediaAddr(formatMediaTemplate(service.Config.RTCMediaDirectWHIPPublishURLTemplate, deviceID, roomName, participantIdentity, request.StreamInstance, streamKey))
	if isLoopbackDirectPublishURL(directURL) && !service.Config.RTCMediaDirectWHIPAllowLoopback {
		return ""
	}
	return directURL
}

func streamResultBase(cameraAddr string, publishURL string, directPublishURL string, streamType int, streamKey string) map[string]any {
	playbackProtocol := playbackProtocol(cameraAddr)
	previewURL := whepPreviewURL(publishURL)
	p1SignalingURL := p1SRSPlayAPIURL(cameraAddr)
	hasPlaybackPort := playbackURLHasExplicitPort(cameraAddr)
	cameraClientReopenRequired := playbackProtocol == "whep" || playbackProtocol == "webrtc"
	return map[string]any{
		"addr":                          cameraAddr,
		"stream_key":                    streamKey,
		"playback_url":                  cameraAddr,
		"preview_url":                   previewURL,
		"p1_signaling_url":              p1SignalingURL,
		"p1_requires_srs_bridge":        p1SignalingURL != "",
		"playback_url_has_port":         hasPlaybackPort,
		"publish_url":                   publishURL,
		"direct_publish_url":            directPublishURL,
		"publish_url_candidates":        publishURLCandidates(directPublishURL, publishURL),
		"publish_protocol":              publishProtocol(publishURL, directPublishURL),
		"playback_protocol":             playbackProtocol,
		"stream_type":                   streamType,
		"stream_type_label":             streamTypeLabel(streamType),
		"direct_open_supported":         playbackProtocol != "whep" && playbackProtocol != "webrtc",
		"consumer_status":               consumerStatus(cameraClientReopenRequired),
		"consumer_detail":               consumerDetail(cameraClientReopenRequired),
		"camera_client_reopen_required": cameraClientReopenRequired,
		"detail":                        playbackDetail(playbackProtocol, hasPlaybackPort),
	}
}

func (service Service) cameraStreamType(request MediaStartRequest, cameraAddr string) int {
	streamType := 2
	if request.CameraStreamType != nil {
		streamType = *request.CameraStreamType
	}
	if playbackProtocol(cameraAddr) == "whep" || playbackProtocol(cameraAddr) == "webrtc" {
		return 2
	}
	return streamType
}

func playbackProtocol(addr string) string {
	value := strings.ToLower(normalizeMediaAddr(addr))
	switch {
	case strings.HasPrefix(value, "webrtc://"):
		return "webrtc"
	case strings.HasSuffix(value, "/whep"):
		return "whep"
	case strings.HasPrefix(value, "rtmp://"):
		return "rtmp"
	case strings.HasPrefix(value, "http://"), strings.HasPrefix(value, "https://"):
		return "http"
	default:
		return ""
	}
}

func whepPreviewURL(publishURL string) string {
	value := strings.TrimRight(normalizeMediaAddr(publishURL), "/")
	if strings.HasSuffix(strings.ToLower(value), "/whip") {
		return value[:len(value)-5] + "/whep"
	}
	return ""
}

func p1SRSPlayAPIURL(value string) string {
	parsed, err := url.Parse(normalizeMediaAddr(value))
	if err != nil || parsed.Scheme != "webrtc" || parsed.Host == "" {
		return ""
	}
	host := parsed.Hostname()
	if host == "" {
		return ""
	}
	if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
		host = "[" + host + "]"
	}
	return "http://" + host + ":1985/rtc/v1/play/"
}

func playbackURLHasExplicitPort(value string) bool {
	parsed, err := url.Parse(normalizeMediaAddr(value))
	if err != nil || parsed.Scheme != "webrtc" {
		return false
	}
	if parsed.Port() != "" {
		return true
	}
	return strings.Count(parsed.Host, ":") > 1 && !strings.HasPrefix(parsed.Host, "[")
}

func streamTypeLabel(streamType int) string {
	switch streamType {
	case 1:
		return "rtmp_or_file"
	case 2:
		return "webrtc"
	case 3:
		return "image"
	default:
		return ""
	}
}

func playbackDetail(playbackProtocol string, hasPlaybackPort bool) string {
	if hasPlaybackPort {
		return "P1 SRS-style WebRTC playback appends :1985; do not include a port in webrtc:// playback_url"
	}
	if playbackProtocol == "whep" || playbackProtocol == "webrtc" {
		return "WHEP/WebRTC playback requires a WebRTC client and cannot be opened directly with GET"
	}
	return ""
}

func consumerStatus(cameraClientReopenRequired bool) string {
	if cameraClientReopenRequired {
		return "reopen_camera_client_required"
	}
	return "ready"
}

func consumerDetail(cameraClientReopenRequired bool) string {
	if cameraClientReopenRequired {
		return "P1 fakecam reads this stream when a phone app opens or reopens camera; already-open camera clients keep their current source"
	}
	return ""
}

func publishProtocol(publishURL string, directPublishURL string) string {
	if publishURL != "" || directPublishURL != "" {
		return "whip"
	}
	return ""
}

func publishURLCandidates(values ...string) []string {
	candidates := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		candidate := normalizeMediaAddr(value)
		if candidate == "" || seen[candidate] {
			continue
		}
		candidates = append(candidates, candidate)
		seen[candidate] = true
	}
	return candidates
}

func normalizeMediaAddr(value string) string {
	return strings.Trim(strings.TrimSpace(value), "\"'")
}

func (service Service) repairP1PlaybackAddr(value string) string {
	addr := normalizeMediaAddr(value)
	if !strings.HasSuffix(strings.ToLower(addr), "/whep") {
		return addr
	}
	playbackHost := p1PlaybackHost(service.Config.RTCMediaP1PlaybackHost)
	if playbackHost == "" {
		return addr
	}
	streamName := whepStreamName(addr)
	if streamName == "" {
		return addr
	}
	return "webrtc://" + playbackHost + "/live/" + streamName
}

func whepStreamName(value string) string {
	parsed, err := url.Parse(normalizeMediaAddr(value))
	if err != nil {
		return ""
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) < 2 || strings.ToLower(parts[len(parts)-1]) != "whep" {
		return ""
	}
	return parts[len(parts)-2]
}

func p1PlaybackHost(value string) string {
	raw := normalizeMediaAddr(value)
	if raw == "" {
		return ""
	}
	if strings.Contains(raw, "://") {
		parsed, err := url.Parse(raw)
		if err != nil {
			return ""
		}
		return strings.Trim(parsed.Hostname(), "[]")
	}
	host := strings.SplitN(raw, "/", 2)[0]
	if strings.Count(host, ":") == 1 {
		host = strings.SplitN(host, ":", 2)[0]
	}
	return strings.Trim(host, "[]")
}

func isLoopbackDirectPublishURL(value string) bool {
	host := strings.Trim(urlHost(value), "[]")
	return host == "localhost" || host == "::1" || strings.HasPrefix(host, "127.")
}

func urlHost(value string) string {
	parsed, err := url.Parse(normalizeMediaAddr(value))
	if err != nil {
		return ""
	}
	return parsed.Hostname()
}

func boolValue(value any) bool {
	typed, ok := value.(bool)
	return ok && typed
}
