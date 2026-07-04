package senddispatcher

import (
	"context"
	"strings"
	"time"
)

const (
	defaultSDKDeviceTransportTTLSeconds        = 180
	defaultSDKDeviceUIUnstableWindowSeconds    = 300
	defaultSDKDeviceUIUnstableCooldownSeconds  = 120
	defaultSDKDeviceUIUnstableFailureThreshold = 3
)

// SDKDeviceHealthOptions controls deterministic device-health decisions.
type SDKDeviceHealthOptions struct {
	Now                       func() time.Time
	TransportTTLSeconds       int
	UIUnstableWindowSeconds   int
	UIUnstableCooldownSeconds int
	UIUnstableThreshold       int
}

// SDKDeviceHealthRecorder records one SDK execution outcome through an injected adapter.
type SDKDeviceHealthRecorder interface {
	RecordSDKDeviceTaskResult(ctx context.Context, record SDKDeviceTaskResult) error
}

// SDKDeviceHealthReader reads recent SDK device health state before dispatch.
type SDKDeviceHealthReader interface {
	GetRecentSDKDeviceTransportFailure(ctx context.Context, deviceID string) (*SDKDeviceTransportFailure, error)
	GetRecentSDKDeviceUIUnstableState(ctx context.Context, deviceID string) (*SDKDeviceUIUnstableState, error)
}

// SDKDeviceTaskResult is the repository-neutral input for SDK device health recording.
type SDKDeviceTaskResult struct {
	DeviceID string
	Success  bool
	Error    string
	TaskID   string
	TaskType string
}

// SDKDeviceHealthDecision describes cache/local-map writes after one SDK result.
type SDKDeviceHealthDecision struct {
	DeviceID          string
	ClearTransport    bool
	ClearUIUnstable   bool
	TransportFailure  *SDKDeviceTransportFailure
	UIUnstableFailure *SDKDeviceUIUnstableState
}

// SDKDeviceTransportFailure is the payload stored under sdk:device_transport:{device_id}.
type SDKDeviceTransportFailure struct {
	Available bool
	DeviceID  string
	Error     string
	TaskID    string
	TaskType  string
	UpdatedAt time.Time
	ExpiresAt time.Time
}

// SDKDeviceUIUnstableState is the payload stored under sdk:device_ui_unstable:{device_id}.
type SDKDeviceUIUnstableState struct {
	DeviceID    string
	Count       int
	Threshold   int
	CoolingDown bool
	Stage       string
	Error       string
	TaskID      string
	TaskType    string
	UpdatedAt   time.Time
	ExpiresAt   time.Time
}

// BuildSDKDeviceHealthDecision mirrors Python record_sdk_device_task_result decision rules.
func BuildSDKDeviceHealthDecision(deviceID string, success bool, errorText string, taskID string, taskType string, previousUI *SDKDeviceUIUnstableState, options SDKDeviceHealthOptions) SDKDeviceHealthDecision {
	normalizedDeviceID := strings.TrimSpace(deviceID)
	if normalizedDeviceID == "" {
		return SDKDeviceHealthDecision{}
	}
	now := sdkHealthNow(options)
	decision := SDKDeviceHealthDecision{DeviceID: normalizedDeviceID}
	if success {
		decision.ClearTransport = true
		decision.ClearUIUnstable = true
		return decision
	}
	if IsRecentSDKTransportFailure(errorText) || isSDKStatusProbeTask(taskType) {
		return decision
	}

	classification := ClassifySDKFailure(errorText, taskType)
	switch classification.DeviceHealthImpact {
	case "ui_unstable":
		decision.UIUnstableFailure = buildSDKDeviceUIUnstableState(normalizedDeviceID, errorText, taskID, taskType, classification.Stage, previousUI, now, options)
	case "none", "login_state":
		decision.ClearUIUnstable = true
	}
	if classification.Stage == "transport" {
		ttl := boundedSeconds(options.TransportTTLSeconds, defaultSDKDeviceTransportTTLSeconds, 10)
		decision.TransportFailure = &SDKDeviceTransportFailure{
			Available: false,
			DeviceID:  normalizedDeviceID,
			Error:     truncateHealthText(StripRecentSDKTransportFailurePrefix(errorText, normalizedDeviceID)),
			TaskID:    strings.TrimSpace(taskID),
			TaskType:  strings.TrimSpace(taskType),
			UpdatedAt: now,
			ExpiresAt: now.Add(time.Duration(ttl) * time.Second),
		}
	}
	return decision
}

// IsRecentSDKTransportFailure identifies fail-fast errors from an existing cooldown.
func IsRecentSDKTransportFailure(errorText string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(errorText)), "recent sdk transport failure")
}

// StripRecentSDKTransportFailurePrefix collapses nested fail-fast prefixes.
func StripRecentSDKTransportFailurePrefix(errorText string, deviceID string) string {
	text := strings.TrimSpace(errorText)
	if text == "" {
		return ""
	}
	prefixes := []string{"recent SDK transport failure:"}
	if normalizedDeviceID := strings.TrimSpace(deviceID); normalizedDeviceID != "" {
		prefixes = append([]string{"recent SDK transport failure for " + normalizedDeviceID + ":"}, prefixes...)
	}
	changed := true
	for changed {
		changed = false
		for _, prefix := range prefixes {
			if strings.HasPrefix(strings.ToLower(text), strings.ToLower(prefix)) {
				text = strings.TrimSpace(text[len(prefix):])
				changed = true
			}
		}
	}
	return text
}

func buildSDKDeviceUIUnstableState(deviceID string, errorText string, taskID string, taskType string, stage string, previous *SDKDeviceUIUnstableState, now time.Time, options SDKDeviceHealthOptions) *SDKDeviceUIUnstableState {
	previousCount := 0
	if previous != nil && (previous.ExpiresAt.IsZero() || previous.ExpiresAt.After(now)) {
		previousCount = previous.Count
	}
	count := previousCount + 1
	threshold := boundedSeconds(options.UIUnstableThreshold, defaultSDKDeviceUIUnstableFailureThreshold, 1)
	coolingDown := count >= threshold
	ttl := boundedSeconds(options.UIUnstableWindowSeconds, defaultSDKDeviceUIUnstableWindowSeconds, 30)
	if coolingDown {
		ttl = boundedSeconds(options.UIUnstableCooldownSeconds, defaultSDKDeviceUIUnstableCooldownSeconds, 30)
	}
	return &SDKDeviceUIUnstableState{
		DeviceID:    deviceID,
		Count:       count,
		Threshold:   threshold,
		CoolingDown: coolingDown,
		Stage:       strings.TrimSpace(stage),
		Error:       truncateHealthText(errorText),
		TaskID:      strings.TrimSpace(taskID),
		TaskType:    strings.TrimSpace(taskType),
		UpdatedAt:   now,
		ExpiresAt:   now.Add(time.Duration(ttl) * time.Second),
	}
}

func isSDKStatusProbeTask(taskType string) bool {
	return strings.TrimSpace(taskType) == "wework_login_status"
}

func sdkHealthNow(options SDKDeviceHealthOptions) time.Time {
	if options.Now != nil {
		return options.Now().UTC()
	}
	return time.Now().UTC()
}

func boundedSeconds(value int, fallback int, minimum int) int {
	if fallback < minimum {
		fallback = minimum
	}
	if value <= 0 {
		value = fallback
	}
	if value < minimum {
		return minimum
	}
	return value
}

func truncateHealthText(value string) string {
	text := strings.TrimSpace(value)
	if len([]rune(text)) <= 240 {
		return text
	}
	runes := []rune(text)
	return string(runes[:240])
}
