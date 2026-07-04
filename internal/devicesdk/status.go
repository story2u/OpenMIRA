package devicesdk

import (
	"context"
	"strings"
	"time"

	"wework-go/internal/devicebridge"
	"wework-go/internal/senddispatcher"
)

// LoginSessionReader reads the current WeCom login fact for a device.
type LoginSessionReader interface {
	GetLoginSession(ctx context.Context, deviceID string) (LoginSession, error)
}

// TransportHealthReader reads recent SDK transport cooldown facts.
type TransportHealthReader interface {
	GetRecentSDKDeviceTransportFailure(ctx context.Context, deviceID string) (*senddispatcher.SDKDeviceTransportFailure, error)
}

// LoginSession is the status subset exposed by the legacy SDK status route.
type LoginSession struct {
	Status           string
	AccountName      string
	WeWorkUserID     string
	OrganizationName string
}

// Status returns the legacy /devices/{device_id}/sdk/status payload.
func (service Service) Status(ctx context.Context, deviceID string, includeManager bool) (map[string]any, error) {
	_ = includeManager
	managerDevice, ok := service.findSlot(deviceID)
	if !ok {
		return nil, ErrSDKDeviceNotConfigured
	}
	slot := legacySlot(managerDevice, deviceID)
	canonicalDeviceID := clean(firstValue(slot["device_id"], deviceID))
	login, err := service.loginSession(ctx, canonicalDeviceID)
	if err != nil {
		return nil, err
	}
	transportFailure, err := service.transportFailure(ctx, canonicalDeviceID)
	if err != nil {
		return nil, err
	}
	statusRow := map[string]any{
		"device_id":         canonicalDeviceID,
		"p1_host":           slot["host"],
		"p1_device_ip":      slot["device_ip"],
		"p1_manager_host":   slot["manager_host"],
		"p1_adb_port":       slot["p1_adb_port"],
		"p1_container_name": slot["container_name"],
		"p1_aliases":        slot["aliases"],
	}
	return map[string]any{
		"success":   true,
		"device_id": slot["device_id"],
		"slot":      slot,
		"login_status": map[string]any{
			"status":            loginStatus(login.Status),
			"account_name":      strings.TrimSpace(login.AccountName),
			"wework_user_id":    strings.TrimSpace(login.WeWorkUserID),
			"organization_name": strings.TrimSpace(login.OrganizationName),
		},
		"transport_health":    transportHealthPayload(transportFailure),
		"media_stream_config": service.mediaConfig().Status(),
		"call_audio_bridge":   service.bridgeService().StatusForRow(statusRow),
		"manager":             map[string]any{},
	}, nil
}

func (service Service) loginSession(ctx context.Context, deviceID string) (LoginSession, error) {
	if service.LoginSessions == nil {
		return LoginSession{Status: "idle"}, nil
	}
	session, err := service.LoginSessions.GetLoginSession(ctx, deviceID)
	if err != nil {
		return LoginSession{}, err
	}
	if strings.TrimSpace(session.Status) == "" {
		session.Status = "idle"
	}
	return session, nil
}

func (service Service) transportFailure(ctx context.Context, deviceID string) (*senddispatcher.SDKDeviceTransportFailure, error) {
	if service.TransportHealth == nil {
		return nil, nil
	}
	return service.TransportHealth.GetRecentSDKDeviceTransportFailure(ctx, deviceID)
}

func (service Service) bridgeService() devicebridge.Service {
	return devicebridge.Service{
		StatusFile:   service.Config.CallAudioBridgeStatusFile,
		HostDataRoot: service.Config.CallAudioBridgeHostDataRoot,
		StaleSec:     service.Config.CallAudioBridgeStaleSec,
	}
}

func (service Service) mediaConfig() devicebridge.MediaConfig {
	return devicebridge.MediaConfig{
		PlaybackTemplate:      service.Config.RTCMediaCameraAddrTemplate,
		PublishTemplate:       service.Config.RTCMediaWHIPPublishURLTemplate,
		DirectPublishTemplate: service.Config.RTCMediaDirectWHIPPublishURLTemplate,
		P1PlaybackHost:        service.Config.RTCMediaP1PlaybackHost,
	}
}

func loginStatus(value string) string {
	status := strings.TrimSpace(value)
	if status == "" {
		return "idle"
	}
	return status
}

func transportHealthPayload(failure *senddispatcher.SDKDeviceTransportFailure) map[string]any {
	if failure == nil {
		return map[string]any{
			"available":  true,
			"error":      "",
			"updated_at": "",
		}
	}
	return map[string]any{
		"available":  false,
		"error":      strings.TrimSpace(failure.Error),
		"updated_at": formatStatusTime(failure.UpdatedAt),
	}
}

func formatStatusTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}
