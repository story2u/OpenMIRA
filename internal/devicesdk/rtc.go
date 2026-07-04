package devicesdk

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"im-go/internal/auth"
)

// RTCStateStore owns the short-lived LiveKit control and Bridge-active facts.
type RTCStateStore interface {
	ReadControlState(ctx context.Context, deviceID string) (map[string]any, error)
	WriteControlState(ctx context.Context, deviceID string, state map[string]any, ttl time.Duration) error
	ClearControlState(ctx context.Context, deviceID string) error
	MarkBridgeActive(ctx context.Context, mark BridgeActiveMark, ttl time.Duration) error
	ListBridgeActive(ctx context.Context) ([]map[string]any, error)
}

// ControlConflictError carries the active controller state for HTTP 409 detail.
type ControlConflictError struct {
	State map[string]any
}

func (err ControlConflictError) Error() string {
	return ErrSDKControlAlreadyOwned.Error()
}

func (err ControlConflictError) Unwrap() error {
	return ErrSDKControlAlreadyOwned
}

// BridgeActiveMark is the Redis/memory payload consumed by the P1 LiveKit Bridge.
type BridgeActiveMark struct {
	DeviceID            string
	RoomName            string
	ParticipantIdentity string
	ActiveAt            time.Time
	ExpiresAt           time.Time
}

// ControlInputRequest carries one low-latency LiveKit control event.
type ControlInputRequest struct {
	ParticipantIdentity string
	Kind                string
	Action              string
	X                   float64
	Y                   float64
	DeltaX              float64
	DeltaY              float64
	Key                 string
	Text                string
	TimestampMillis     int64
}

// ControlInputExecutor sends validated device input to the low-latency P1
// control bridge. The Go service owns auth, slot lookup, lease checks, and
// normalization before this boundary.
type ControlInputExecutor interface {
	SendControlInput(ctx context.Context, command ControlInputCommand) (ControlInputResult, error)
}

// ControlInputCommand is the normalized command sent to a bridge or native
// executor.
type ControlInputCommand struct {
	DeviceID            string
	ParticipantIdentity string
	Kind                string
	Action              string
	RatioX              float64
	RatioY              float64
	X                   int
	Y                   int
	DeltaX              float64
	DeltaY              float64
	Key                 string
	NormalizedKey       string
	KeyCode             int
	Text                string
	TimestampMillis     int64
	ScreenWidth         int
	ScreenHeight        int
}

// ControlInputResult carries execution timing returned by the bridge.
type ControlInputResult struct {
	Sent          bool
	Detail        string
	Route         string
	AcquireMillis int
	SendMillis    int
}

// ControlInputError wraps a stable sentinel with a bridge-supplied detail.
type ControlInputError struct {
	Cause  error
	Detail string
}

func (err ControlInputError) Error() string {
	if strings.TrimSpace(err.Detail) != "" {
		return strings.TrimSpace(err.Detail)
	}
	if err.Cause != nil {
		return err.Cause.Error()
	}
	return "device control input failed"
}

func (err ControlInputError) Unwrap() error {
	return err.Cause
}

// Payload returns the legacy JSON shape stored under rtc:bridge-active keys.
func (mark BridgeActiveMark) Payload() map[string]any {
	return map[string]any{
		"device_id":            SanitizeRTCSegment(mark.DeviceID),
		"room_name":            strings.TrimSpace(mark.RoomName),
		"participant_identity": strings.TrimSpace(mark.ParticipantIdentity),
		"active_at":            unixSeconds(mark.ActiveAt),
		"expires_at":           unixSeconds(mark.ExpiresAt),
	}
}

// MemoryRTCStateStore is the no-Redis fallback used by local harness tests.
type MemoryRTCStateStore struct {
	mu      sync.Mutex
	control map[string]map[string]any
	active  map[string]BridgeActiveMark
}

// NewMemoryRTCStateStore creates an in-process RTC state store.
func NewMemoryRTCStateStore() *MemoryRTCStateStore {
	return &MemoryRTCStateStore{
		control: map[string]map[string]any{},
		active:  map[string]BridgeActiveMark{},
	}
}

// ReadControlState returns the current device controller lease, if one exists.
func (store *MemoryRTCStateStore) ReadControlState(_ context.Context, deviceID string) (map[string]any, error) {
	if store == nil {
		return nil, nil
	}
	key := SanitizeRTCSegment(deviceID)
	store.mu.Lock()
	defer store.mu.Unlock()
	state := store.control[key]
	if state == nil {
		return nil, nil
	}
	return cloneMap(state), nil
}

// WriteControlState stores the current controller lease.
func (store *MemoryRTCStateStore) WriteControlState(_ context.Context, deviceID string, state map[string]any, _ time.Duration) error {
	if store == nil {
		return nil
	}
	key := SanitizeRTCSegment(deviceID)
	store.mu.Lock()
	defer store.mu.Unlock()
	store.control[key] = cloneMap(state)
	return nil
}

// ClearControlState removes the current controller lease.
func (store *MemoryRTCStateStore) ClearControlState(_ context.Context, deviceID string) error {
	if store == nil {
		return nil
	}
	key := SanitizeRTCSegment(deviceID)
	store.mu.Lock()
	defer store.mu.Unlock()
	delete(store.control, key)
	return nil
}

// MarkBridgeActive records one active LiveKit device room mark.
func (store *MemoryRTCStateStore) MarkBridgeActive(_ context.Context, mark BridgeActiveMark, _ time.Duration) error {
	if store == nil {
		return nil
	}
	key := SanitizeRTCSegment(mark.DeviceID)
	store.mu.Lock()
	defer store.mu.Unlock()
	store.active[key] = mark
	return nil
}

// ListBridgeActive returns unexpired active marks for the Bridge scheduler.
func (store *MemoryRTCStateStore) ListBridgeActive(_ context.Context) ([]map[string]any, error) {
	if store == nil {
		return []map[string]any{}, nil
	}
	now := time.Now()
	store.mu.Lock()
	defer store.mu.Unlock()
	items := make([]map[string]any, 0, len(store.active))
	for key, mark := range store.active {
		if !mark.ExpiresAt.IsZero() && !mark.ExpiresAt.After(now) {
			delete(store.active, key)
			continue
		}
		items = append(items, mark.Payload())
	}
	return items, nil
}

// SetControlState seeds a controller lease for tests and local harnesses.
func (store *MemoryRTCStateStore) SetControlState(deviceID string, state map[string]any) {
	if store == nil {
		return
	}
	key := SanitizeRTCSegment(deviceID)
	store.mu.Lock()
	defer store.mu.Unlock()
	store.control[key] = cloneMap(state)
}

// ActiveMark returns a recorded active mark for tests.
func (store *MemoryRTCStateStore) ActiveMark(deviceID string) (BridgeActiveMark, bool) {
	if store == nil {
		return BridgeActiveMark{}, false
	}
	key := SanitizeRTCSegment(deviceID)
	store.mu.Lock()
	defer store.mu.Unlock()
	mark, ok := store.active[key]
	return mark, ok
}

var defaultRTCStateStore = NewMemoryRTCStateStore()

// RTCSession returns the legacy /devices/{device_id}/sdk/rtc-session payload.
func (service Service) RTCSession(ctx context.Context, deviceID string, quality string, mode string, origin RequestOrigin, session auth.Session) (map[string]any, error) {
	if err := ValidateQuality(quality); err != nil {
		return nil, err
	}
	managerDevice, ok := service.findSlot(deviceID)
	if !ok {
		return nil, ErrSDKDeviceNotConfigured
	}
	slot := legacySlot(managerDevice, deviceID)
	canonicalDeviceID := clean(firstValue(slot["device_id"], deviceID))
	requestedMode := normalizeRTCMode(mode)
	configuredMode, modeReason := service.configuredRTCMode()
	selectedMode := configuredMode
	if requestedMode != "auto" {
		selectedMode = requestedMode
	}
	if selectedMode == "legacy" {
		return nil, ErrSDKLegacyRTCDisabled
	}
	liveKitURL := service.liveKitWSURL()
	if liveKitURL == "" || strings.TrimSpace(service.Config.LiveKitAPIKey) == "" || strings.TrimSpace(service.Config.LiveKitAPISecret) == "" {
		return nil, ErrSDKLiveKitNotConfigured
	}

	now := service.now()
	roomName := service.deviceRoomName(canonicalDeviceID)
	identity := service.participantIdentity(session, canonicalDeviceID, now)
	token, err := service.createLiveKitAccessToken(identity, roomName, session, now)
	if err != nil {
		return nil, err
	}
	controlState, err := service.controlStateResponse(ctx, canonicalDeviceID, now)
	if err != nil {
		return nil, err
	}
	activeTTL := service.bridgeActiveTTL()
	mark := BridgeActiveMark{
		DeviceID:            canonicalDeviceID,
		RoomName:            roomName,
		ParticipantIdentity: identity,
		ActiveAt:            now,
		ExpiresAt:           now.Add(activeTTL),
	}
	if err := service.rtcStateStore().MarkBridgeActive(ctx, mark, activeTTL); err != nil {
		return nil, err
	}
	entryURL := buildLiveKitEntryURL(origin, canonicalDeviceID)
	return map[string]any{
		"success":              true,
		"device_id":            canonicalDeviceID,
		"mode":                 "livekit",
		"mode_reason":          modeReason,
		"requested_mode":       requestedMode,
		"url":                  entryURL,
		"entry_url":            entryURL,
		"livekit_url":          liveKitURL,
		"room_name":            roomName,
		"participant_identity": identity,
		"token":                token,
		"token_ttl_sec":        service.liveKitTokenTTLSeconds(),
		"bridge_identity":      "bridge-" + SanitizeRTCSegment(canonicalDeviceID),
		"control_state":        controlState,
	}, nil
}

// RTCActive refreshes the Bridge active mark for one LiveKit device room.
func (service Service) RTCActive(ctx context.Context, deviceID string, participantIdentity string) (map[string]any, error) {
	managerDevice, ok := service.findSlot(deviceID)
	if !ok {
		return nil, ErrSDKDeviceNotConfigured
	}
	participantIdentity = strings.TrimSpace(participantIdentity)
	if participantIdentity == "" {
		return nil, ErrSDKParticipantIdentityRequired
	}
	slot := legacySlot(managerDevice, deviceID)
	canonicalDeviceID := clean(firstValue(slot["device_id"], deviceID))
	now := service.now()
	activeTTL := service.bridgeActiveTTL()
	mark := BridgeActiveMark{
		DeviceID:            canonicalDeviceID,
		RoomName:            service.deviceRoomName(canonicalDeviceID),
		ParticipantIdentity: participantIdentity,
		ActiveAt:            now,
		ExpiresAt:           now.Add(activeTTL),
	}
	if err := service.rtcStateStore().MarkBridgeActive(ctx, mark, activeTTL); err != nil {
		return nil, err
	}
	payload := mark.Payload()
	payload["success"] = true
	return payload, nil
}

// ListRTCActive returns recently viewed LiveKit device rooms for the Bridge.
func (service Service) ListRTCActive(ctx context.Context) (map[string]any, error) {
	devices, err := service.rtcStateStore().ListBridgeActive(ctx)
	if err != nil {
		return nil, err
	}
	return map[string]any{"success": true, "devices": devices}, nil
}

// ControlState returns the current LiveKit controller lease for one device.
func (service Service) ControlState(ctx context.Context, deviceID string) (map[string]any, error) {
	managerDevice, ok := service.findSlot(deviceID)
	if !ok {
		return nil, ErrSDKDeviceNotConfigured
	}
	slot := legacySlot(managerDevice, deviceID)
	canonicalDeviceID := clean(firstValue(slot["device_id"], deviceID))
	return service.controlStateResponse(ctx, canonicalDeviceID, service.now())
}

// ControlInput validates controller ownership and forwards the normalized event
// to the configured low-latency executor.
func (service Service) ControlInput(ctx context.Context, deviceID string, request ControlInputRequest) (map[string]any, error) {
	routeStarted := time.Now()
	slotStarted := time.Now()
	managerDevice, ok := service.findSlot(deviceID)
	slotMillis := elapsedMillis(slotStarted)
	if !ok {
		return nil, ErrSDKDeviceNotConfigured
	}
	participantIdentity := strings.TrimSpace(request.ParticipantIdentity)
	if participantIdentity == "" {
		return nil, ErrSDKParticipantIdentityRequired
	}
	slot := legacySlot(managerDevice, deviceID)
	canonicalDeviceID := clean(firstValue(slot["device_id"], deviceID))
	stateStarted := time.Now()
	existing, err := service.rtcStateStore().ReadControlState(ctx, canonicalDeviceID)
	controlStateMillis := elapsedMillis(stateStarted)
	if err != nil {
		return nil, err
	}
	expiresAt := floatValue(existing["expires_at"])
	if expiresAt > 0 && expiresAt <= unixSeconds(service.now()) {
		return nil, ErrSDKControlInputForbidden
	}
	if strings.TrimSpace(stringValue(existing["controller_identity"])) != participantIdentity {
		return nil, ErrSDKControlInputForbidden
	}
	if service.ControlExecutor == nil {
		return nil, ErrSDKControlInputUnavailable
	}
	width, height := service.controlScreenSize(slot)
	x, y := pointerToAndroid(request.X, request.Y, width, height)
	normalizedKey := normalizedControlKey(request.Key)
	command := ControlInputCommand{
		DeviceID:            canonicalDeviceID,
		ParticipantIdentity: participantIdentity,
		Kind:                strings.ToLower(strings.TrimSpace(request.Kind)),
		Action:              strings.ToLower(strings.TrimSpace(request.Action)),
		RatioX:              clampRatio(request.X),
		RatioY:              clampRatio(request.Y),
		X:                   x,
		Y:                   y,
		DeltaX:              request.DeltaX,
		DeltaY:              request.DeltaY,
		Key:                 strings.TrimSpace(request.Key),
		NormalizedKey:       normalizedKey,
		KeyCode:             androidControlKeyCode(normalizedKey),
		Text:                request.Text,
		TimestampMillis:     request.TimestampMillis,
		ScreenWidth:         width,
		ScreenHeight:        height,
	}
	dispatchStarted := time.Now()
	result, err := service.ControlExecutor.SendControlInput(ctx, command)
	dispatchMillis := elapsedMillis(dispatchStarted)
	if err != nil {
		return nil, err
	}
	if !result.Sent {
		return nil, ControlInputError{Cause: ErrSDKControlInputFailed, Detail: result.Detail}
	}
	route := strings.TrimSpace(result.Route)
	if route == "" {
		route = DefaultControlInputRoute
	}
	return map[string]any{
		"success":          true,
		"device_id":        canonicalDeviceID,
		"route":            route,
		"sent":             true,
		"detail":           strings.TrimSpace(result.Detail),
		"age_ms":           controlInputAgeMillis(request.TimestampMillis),
		"slot_ms":          slotMillis,
		"control_state_ms": controlStateMillis,
		"acquire_ms":       result.AcquireMillis,
		"send_ms":          result.SendMillis,
		"dispatch_ms":      dispatchMillis,
		"total_ms":         elapsedMillis(routeStarted),
		"screen_width":     width,
		"screen_height":    height,
	}, nil
}

// AcquireControl acquires or renews the single-controller lease.
func (service Service) AcquireControl(ctx context.Context, deviceID string, participantIdentity string, session auth.Session) (map[string]any, error) {
	managerDevice, ok := service.findSlot(deviceID)
	if !ok {
		return nil, ErrSDKDeviceNotConfigured
	}
	participantIdentity = strings.TrimSpace(participantIdentity)
	if participantIdentity == "" {
		return nil, ErrSDKParticipantIdentityRequired
	}
	slot := legacySlot(managerDevice, deviceID)
	canonicalDeviceID := clean(firstValue(slot["device_id"], deviceID))
	now := service.now()
	existing, err := service.rtcStateStore().ReadControlState(ctx, canonicalDeviceID)
	if err != nil {
		return nil, err
	}
	owner := controlOwner(session, participantIdentity)
	if controlStateBlocksAcquire(existing, participantIdentity, owner, now) {
		return nil, ControlConflictError{State: stateResponse(canonicalDeviceID, existing, now)}
	}
	state := cloneMap(owner)
	state["acquired_at"] = unixSeconds(now)
	state["expires_at"] = unixSeconds(now.Add(service.controlTTL()))
	if err := service.rtcStateStore().WriteControlState(ctx, canonicalDeviceID, state, service.controlTTL()); err != nil {
		return nil, err
	}
	return stateResponse(canonicalDeviceID, state, now), nil
}

// ReleaseControl clears a controller lease when the caller is allowed.
func (service Service) ReleaseControl(ctx context.Context, deviceID string, participantIdentity string, session auth.Session) (map[string]any, error) {
	managerDevice, ok := service.findSlot(deviceID)
	if !ok {
		return nil, ErrSDKDeviceNotConfigured
	}
	slot := legacySlot(managerDevice, deviceID)
	canonicalDeviceID := clean(firstValue(slot["device_id"], deviceID))
	existing, err := service.rtcStateStore().ReadControlState(ctx, canonicalDeviceID)
	if err != nil {
		return nil, err
	}
	callerIdentity := strings.TrimSpace(participantIdentity)
	role := strings.TrimSpace(session.Role)
	existingIdentity := strings.TrimSpace(stringValue(existing["controller_identity"]))
	if existingIdentity != "" && existingIdentity != callerIdentity && role != "admin" && role != "supervisor" {
		return nil, ErrSDKControlReleaseForbidden
	}
	if err := service.rtcStateStore().ClearControlState(ctx, canonicalDeviceID); err != nil {
		return nil, err
	}
	return stateResponse(canonicalDeviceID, nil, service.now()), nil
}

// StealControl lets an operator take over the single-controller lease.
func (service Service) StealControl(ctx context.Context, deviceID string, participantIdentity string, session auth.Session) (map[string]any, error) {
	managerDevice, ok := service.findSlot(deviceID)
	if !ok {
		return nil, ErrSDKDeviceNotConfigured
	}
	participantIdentity = strings.TrimSpace(participantIdentity)
	if participantIdentity == "" {
		return nil, ErrSDKParticipantIdentityRequired
	}
	slot := legacySlot(managerDevice, deviceID)
	canonicalDeviceID := clean(firstValue(slot["device_id"], deviceID))
	now := service.now()
	state := cloneMap(controlOwner(session, participantIdentity))
	state["acquired_at"] = unixSeconds(now)
	state["expires_at"] = unixSeconds(now.Add(service.controlTTL()))
	if err := service.rtcStateStore().WriteControlState(ctx, canonicalDeviceID, state, service.controlTTL()); err != nil {
		return nil, err
	}
	return stateResponse(canonicalDeviceID, state, now), nil
}

func (service Service) configuredRTCMode() (string, string) {
	configured := normalizeRTCMode(firstNonEmpty(service.Config.RTCModeDefault, "livekit"))
	if configured == "legacy" {
		return "livekit", "legacy_default_ignored"
	}
	return "livekit", "default"
}

func (service Service) liveKitWSURL() string {
	raw := strings.TrimRight(strings.TrimSpace(service.Config.LiveKitURL), "/")
	switch {
	case strings.HasPrefix(raw, "https://"):
		return "wss://" + strings.TrimPrefix(raw, "https://")
	case strings.HasPrefix(raw, "http://"):
		return "ws://" + strings.TrimPrefix(raw, "http://")
	default:
		return raw
	}
}

func (service Service) liveKitTokenTTLSeconds() int {
	ttl := service.Config.LiveKitTokenTTLSeconds
	if ttl < 60 {
		return 60
	}
	return ttl
}

func (service Service) bridgeActiveTTL() time.Duration {
	seconds := service.Config.RTCBridgeActiveTTLSeconds
	if seconds < 20 {
		seconds = 90
	}
	if seconds > 600 {
		seconds = 600
	}
	return time.Duration(seconds) * time.Second
}

func (service Service) controlTTL() time.Duration {
	seconds := service.Config.RTCControlTTLSeconds
	if seconds < 15 {
		seconds = 120
	}
	if seconds > 3600 {
		seconds = 3600
	}
	return time.Duration(seconds) * time.Second
}

func (service Service) controlScreenSize(slot map[string]any) (int, int) {
	width := service.Config.RTCControlScreenWidth
	if width <= 0 {
		width = positiveInt(firstValue(slot["p1_width"], slot["width"]))
	}
	if width <= 0 {
		width = 1080
	}
	height := service.Config.RTCControlScreenHeight
	if height <= 0 {
		height = positiveInt(firstValue(slot["p1_height"], slot["height"]))
	}
	if height <= 0 {
		height = 1920
	}
	return width, height
}

func pointerToAndroid(ratioX float64, ratioY float64, width int, height int) (int, int) {
	width = maxInt(width, 1)
	height = maxInt(height, 1)
	x := int(clampRatio(ratioX)*float64(width-1) + 0.5)
	y := int(clampRatio(ratioY)*float64(height-1) + 0.5)
	return x, y
}

func clampRatio(value float64) float64 {
	switch {
	case value < 0:
		return 0
	case value > 1:
		return 1
	default:
		return value
	}
}

func normalizedControlKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer(" ", "", "\t", "", "\n", "", "\r", "", "_", "", "-", "")
	return replacer.Replace(value)
}

func androidControlKeyCode(key string) int {
	switch normalizedControlKey(key) {
	case "back", "goback":
		return 4
	case "home", "gohome":
		return 3
	case "recents", "recent", "appswitch", "goclean":
		return 187
	case "backspace", "delete":
		return 67
	case "enter":
		return 66
	case "arrowleft", "left":
		return 21
	case "arrowright", "right":
		return 22
	case "arrowup", "up":
		return 19
	case "arrowdown", "down":
		return 20
	default:
		return 0
	}
}

func controlInputAgeMillis(timestampMillis int64) int {
	if timestampMillis <= 0 {
		return -1
	}
	return int(time.Now().UnixMilli() - timestampMillis)
}

func elapsedMillis(started time.Time) int {
	if started.IsZero() {
		return 0
	}
	return int(time.Since(started) / time.Millisecond)
}

func (service Service) deviceRoomName(deviceID string) string {
	prefix := SanitizeRTCSegment(firstNonEmpty(service.Config.LiveKitDeviceRoomPrefix, "device"))
	return prefix + "-" + SanitizeRTCSegment(deviceID)
}

func (service Service) participantIdentity(session auth.Session, deviceID string, now time.Time) string {
	assigneeID := SanitizeRTCSegment(firstNonEmpty(session.AssigneeID, session.Role, "user"))
	devicePart := SanitizeRTCSegment(deviceID)
	if len(devicePart) > 32 {
		devicePart = devicePart[:32]
	}
	return fmt.Sprintf("user-%s-%s-%d-%s", assigneeID, devicePart, now.UnixMilli(), service.identitySuffix())
}

func (service Service) identitySuffix() string {
	suffix := SanitizeRTCSegment(service.newID(""))
	if suffix == "" {
		suffix = service.newID("identity-")
	}
	if len(suffix) > 6 {
		return suffix[:6]
	}
	return suffix
}

func (service Service) createLiveKitAccessToken(identity string, roomName string, session auth.Session, now time.Time) (string, error) {
	apiKey := strings.TrimSpace(service.Config.LiveKitAPIKey)
	apiSecret := strings.TrimSpace(service.Config.LiveKitAPISecret)
	identity = strings.TrimSpace(identity)
	roomName = strings.TrimSpace(roomName)
	if apiKey == "" || apiSecret == "" || identity == "" || roomName == "" {
		return "", ErrSDKLiveKitNotConfigured
	}
	ttl := service.liveKitTokenTTLSeconds()
	name := strings.TrimSpace(firstNonEmpty(session.AssigneeName, session.AssigneeID, "viewer"))
	claims := map[string]any{
		"iss":  apiKey,
		"sub":  identity,
		"iat":  now.Unix(),
		"nbf":  now.Add(-5 * time.Second).Unix(),
		"exp":  now.Add(time.Duration(ttl) * time.Second).Unix(),
		"name": name,
		"video": map[string]any{
			"roomJoin":       true,
			"room":           roomName,
			"canPublish":     true,
			"canSubscribe":   true,
			"canPublishData": true,
		},
	}
	return signHS256(claims, apiSecret)
}

func (service Service) controlStateResponse(ctx context.Context, deviceID string, now time.Time) (map[string]any, error) {
	state, err := service.rtcStateStore().ReadControlState(ctx, deviceID)
	if err != nil {
		return nil, err
	}
	return stateResponse(deviceID, state, now), nil
}

func (service Service) rtcStateStore() RTCStateStore {
	if service.RTCState != nil {
		return service.RTCState
	}
	return defaultRTCStateStore
}

func stateResponse(deviceID string, state map[string]any, now time.Time) map[string]any {
	data := cloneMap(state)
	expiresAt := floatValue(data["expires_at"])
	if expiresAt > 0 && expiresAt <= unixSeconds(now) {
		data = map[string]any{}
	}
	controllerIdentity := strings.TrimSpace(stringValue(data["controller_identity"]))
	return map[string]any{
		"success":             true,
		"device_id":           strings.TrimSpace(deviceID),
		"controlled":          controllerIdentity != "",
		"controller_identity": controllerIdentity,
		"controller_user_id":  strings.TrimSpace(stringValue(data["controller_user_id"])),
		"controller_name":     strings.TrimSpace(stringValue(data["controller_name"])),
		"controller_role":     strings.TrimSpace(stringValue(data["controller_role"])),
		"acquired_at":         floatValue(data["acquired_at"]),
		"expires_at":          floatValue(data["expires_at"]),
	}
}

func controlOwner(session auth.Session, participantIdentity string) map[string]any {
	controllerUserID := firstNonEmpty(session.AssigneeID, session.Role, "user")
	return map[string]any{
		"controller_user_id":  strings.TrimSpace(controllerUserID),
		"controller_name":     strings.TrimSpace(firstNonEmpty(session.AssigneeName, session.AssigneeID)),
		"controller_role":     strings.TrimSpace(session.Role),
		"controller_identity": strings.TrimSpace(participantIdentity),
	}
}

func controlStateBlocksAcquire(existing map[string]any, participantIdentity string, owner map[string]any, now time.Time) bool {
	if len(existing) == 0 {
		return false
	}
	expiresAt := floatValue(existing["expires_at"])
	if expiresAt > 0 && expiresAt <= unixSeconds(now) {
		return false
	}
	existingIdentity := strings.TrimSpace(stringValue(existing["controller_identity"]))
	if existingIdentity == strings.TrimSpace(participantIdentity) {
		return false
	}
	existingUserID := strings.TrimSpace(stringValue(existing["controller_user_id"]))
	ownerUserID := strings.TrimSpace(stringValue(owner["controller_user_id"]))
	existingRole := strings.TrimSpace(stringValue(existing["controller_role"]))
	ownerRole := strings.TrimSpace(stringValue(owner["controller_role"]))
	if existingUserID != "" && ownerUserID != "" && existingUserID == ownerUserID && existingRole == ownerRole {
		return false
	}
	return true
}

func buildLiveKitEntryURL(origin RequestOrigin, deviceID string) string {
	baseURL := strings.TrimRight(origin.String(), "/")
	if baseURL == "" {
		baseURL = "http://"
	}
	return baseURL + "/admin/livekit-device?device_id=" + url.QueryEscape(strings.TrimSpace(deviceID))
}

var rtcSegmentPattern = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

// SanitizeRTCSegment returns a LiveKit/Redis-safe segment.
func SanitizeRTCSegment(value string) string {
	normalized := rtcSegmentPattern.ReplaceAllString(strings.TrimSpace(value), "-")
	normalized = strings.Trim(normalized, "-")
	if len(normalized) > 96 {
		normalized = normalized[:96]
	}
	if normalized == "" {
		return "device"
	}
	return normalized
}

func normalizeRTCMode(value string) string {
	mode := strings.ToLower(strings.TrimSpace(value))
	switch mode {
	case "auto", "legacy", "livekit":
		return mode
	default:
		return "auto"
	}
}

func signHS256(claims map[string]any, secret string) (string, error) {
	header := map[string]any{"alg": "HS256", "typ": "JWT"}
	encodedHeader, err := encodeJWTPart(header)
	if err != nil {
		return "", err
	}
	encodedClaims, err := encodeJWTPart(claims)
	if err != nil {
		return "", err
	}
	signingInput := encodedHeader + "." + encodedClaims
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(signingInput))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return signingInput + "." + signature, nil
}

func encodeJWTPart(payload map[string]any) (string, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func cloneMap(input map[string]any) map[string]any {
	if input == nil {
		return map[string]any{}
	}
	output := make(map[string]any, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

func stringValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	default:
		return fmt.Sprint(value)
	}
}

func floatValue(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case json.Number:
		parsed, _ := typed.Float64()
		return parsed
	default:
		var parsed float64
		_, _ = fmt.Sscan(strings.TrimSpace(fmt.Sprint(value)), &parsed)
		return parsed
	}
}

func unixSeconds(value time.Time) float64 {
	if value.IsZero() {
		return 0
	}
	return float64(value.UnixNano()) / float64(time.Second)
}
