// Package devicesdk builds read-only SDK device helper payloads.
package devicesdk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

var ErrSDKDeviceNotConfigured = errors.New("SDK device not configured")
var ErrSDKTaskServiceNotConfigured = errors.New("device sdk task service is not configured")
var ErrSDKLegacyRTCDisabled = errors.New("legacy rtc-session is disabled; use /sdk/webrtc for internal debug")
var ErrSDKLiveKitNotConfigured = errors.New("LiveKit is not configured")
var ErrSDKParticipantIdentityRequired = errors.New("participant_identity is required")
var ErrSDKControlAlreadyOwned = errors.New("device control is already owned")
var ErrSDKControlReleaseForbidden = errors.New("only current controller can release")
var ErrSDKControlInputForbidden = errors.New("only current controller can send input")
var ErrSDKControlInputUnavailable = errors.New("RPA control input provider is not available")
var ErrSDKControlInputFailed = errors.New("RPA control input failed")
var ErrSDKMediaStartForbidden = errors.New("only current controller can start media")
var ErrSDKMediaStopForbidden = errors.New("only current controller can stop media")
var ErrSDKMediaActivationUnavailable = errors.New("P1 media activation is not available in the Go candidate")
var ErrSDKMediaControlUnavailable = errors.New("P1 media control is not available in the Go candidate")

const DefaultControlInputRoute = "rpa-provider"

// Config carries environment-backed WebRTC URL settings.
type Config struct {
	ManagerCacheFile                     string
	WebplayerPublicBaseURL               string
	WebRTCPublicHost                     string
	BackendBaseURL                       string
	CallAudioBridgeStatusFile            string
	CallAudioBridgeHostDataRoot          string
	CallAudioBridgeStaleSec              float64
	RTCMediaCameraAddrTemplate           string
	RTCMediaWHIPPublishURLTemplate       string
	RTCMediaDirectWHIPPublishURLTemplate string
	RTCMediaP1PlaybackHost               string
	RTCMediaStableStreamKeyDisabled      bool
	RTCMediaDirectWHIPAllowLoopback      bool
	RTCMediaInstanceTTLSeconds           int
	WebRTCTCPOverride                    int
	WebRTCUDPOverride                    int
	LiveKitURL                           string
	LiveKitAPIKey                        string
	LiveKitAPISecret                     string
	LiveKitTokenTTLSeconds               int
	LiveKitDeviceRoomPrefix              string
	RTCModeDefault                       string
	RTCBridgeActiveTTLSeconds            int
	RTCControlTTLSeconds                 int
	RTCControlScreenWidth                int
	RTCControlScreenHeight               int
}

// RequestOrigin describes the externally visible request origin.
type RequestOrigin struct {
	Scheme string
	Host   string
}

// Service builds legacy SDK WebRTC debug URLs from manager cache snapshots.
type Service struct {
	Config          Config
	LoginSessions   LoginSessionReader
	TransportHealth TransportHealthReader
	TaskCreator     TaskCreator
	SDKRefresher    SDKDeviceRefresher
	RTCState        RTCStateStore
	ControlExecutor ControlInputExecutor
	Now             func() time.Time
	NewID           func(prefix string) string
}

// WebRTC returns the legacy /devices/{device_id}/sdk/webrtc payload.
func (service Service) WebRTC(ctx context.Context, deviceID string, quality string, origin RequestOrigin) (map[string]any, error) {
	_ = ctx
	managerDevice, ok := service.findSlot(deviceID)
	if !ok {
		return nil, ErrSDKDeviceNotConfigured
	}
	quality = normalizeQuality(quality)
	slot := legacySlot(managerDevice, deviceID)
	slotIndex := positiveInt(managerDevice["slot"])
	if slotIndex <= 0 {
		slotIndex = 1
	}
	tcpPort, udpPort := service.defaultPorts(slotIndex)
	managerPort := positiveInt(firstValue(managerDevice["p1_webrtc2_port"], managerDevice["p1_webrtc_port"]))
	if managerPort > 0 {
		tcpPort = managerPort
		udpPort = managerPort
	}
	host := clean(managerDevice["host"])
	directURL := buildWebRTCURL(host, tcpPort, udpPort, quality, "h264")
	tcpPort, udpPort = extractWebRTCPorts(directURL, tcpPort, udpPort)
	publicURL := service.buildPublicWebRTCURL(directURL, host, tcpPort, udpPort, quality, origin)
	return map[string]any{
		"success":         true,
		"device_id":       clean(slot["device_id"]),
		"slot":            slot,
		"url":             publicURL,
		"fallback_url":    publicURL,
		"direct_url":      directURL,
		"manager_url":     "",
		"manager":         nil,
		"webrtc_tcp_port": tcpPort,
		"webrtc_udp_port": udpPort,
	}, nil
}

func legacySlot(slot map[string]any, fallbackDeviceID string) map[string]any {
	canonicalID := clean(firstValue(slot["device_id"], fallbackDeviceID))
	return map[string]any{
		"device_id":         canonicalID,
		"host":              slot["host"],
		"slot":              slot["slot"],
		"port":              slot["port"],
		"container_name":    slot["container_name"],
		"manager_port":      slot["manager_port"],
		"manager_host":      slot["manager_host"],
		"manager_device_ip": slot["manager_device_ip"],
		"device_ip":         slot["device_ip"],
		"p1_api_port":       slot["p1_api_port"],
		"p1_adb_port":       slot["p1_adb_port"],
		"p1_width":          slot["p1_width"],
		"p1_height":         slot["p1_height"],
		"p1_dpi":            slot["p1_dpi"],
		"p1_fps":            slot["p1_fps"],
		"aliases":           stringValues(slot["aliases"]),
	}
}

func (service Service) findSlot(deviceID string) (map[string]any, bool) {
	wanted := clean(deviceID)
	if wanted == "" {
		return nil, false
	}
	for _, slot := range service.loadManagerDevices() {
		keys := []string{clean(slot["device_id"])}
		keys = append(keys, stringValues(slot["aliases"])...)
		for _, key := range keys {
			if key == wanted {
				return slot, true
			}
		}
	}
	return nil, false
}

func (service Service) loadManagerDevices() []map[string]any {
	path := strings.TrimSpace(service.Config.ManagerCacheFile)
	if path == "" {
		return nil
	}
	raw, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil
	}
	var payload any
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		return nil
	}
	devices := listItems(payload, "devices")
	sort.SliceStable(devices, func(i, j int) bool {
		return clean(devices[i]["device_id"]) < clean(devices[j]["device_id"])
	})
	return devices
}

func (service Service) defaultPorts(slotIndex int) (int, int) {
	if service.Config.WebRTCTCPOverride > 0 && service.Config.WebRTCUDPOverride > 0 {
		return service.Config.WebRTCTCPOverride, service.Config.WebRTCUDPOverride
	}
	return 30000 + (slotIndex-1)*100 + 7, 30000 + (slotIndex-1)*100 + 8
}

func (service Service) buildPublicWebRTCURL(directURL string, directHost string, tcpPort int, udpPort int, quality string, origin RequestOrigin) string {
	parsed, _ := url.Parse(strings.TrimSpace(directURL))
	params := parsed.Query()
	publicHost := resolvePublicWebRTCHost(service.Config, origin)
	if publicHost == "" {
		publicHost = strings.TrimSpace(directHost)
	}
	params.Set("shost", publicHost)
	params.Set("sport", strconv.Itoa(tcpPort))
	params.Set("q", normalizeQuality(quality))
	if strings.TrimSpace(params.Get("v")) == "" {
		params.Set("v", "h264")
	}
	params.Set("rtc_i", publicHost)
	params.Set("rtc_p", strconv.Itoa(udpPort))
	baseURL := strings.TrimRight(resolvePublicWebplayerBaseURL(service.Config, origin), "/")
	return fmt.Sprintf("%s/webplayer/play.html?%s", baseURL, params.Encode())
}

func ValidateQuality(value string) error {
	quality := normalizeQuality(value)
	if quality != "0" && quality != "1" {
		return fmt.Errorf("quality must be 0 or 1")
	}
	return nil
}

func normalizeQuality(value string) string {
	quality := strings.TrimSpace(value)
	if quality == "" {
		return "1"
	}
	return quality
}

func buildWebRTCURL(host string, tcpPort int, udpPort int, quality string, codec string) string {
	return fmt.Sprintf("/webplayer/play.html?shost=%s&sport=%d&q=%s&v=%s&rtc_i=%s&rtc_p=%d", host, tcpPort, normalizeQuality(quality), defaultText(codec, "h264"), host, udpPort)
}

func extractWebRTCPorts(rawURL string, defaultTCP int, defaultUDP int) (int, int) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return defaultTCP, defaultUDP
	}
	params := parsed.Query()
	return coercePort(params.Get("sport"), defaultTCP), coercePort(params.Get("rtc_p"), defaultUDP)
}

func resolvePublicWebplayerBaseURL(config Config, origin RequestOrigin) string {
	if explicit := strings.TrimRight(strings.TrimSpace(config.WebplayerPublicBaseURL), "/"); explicit != "" {
		return explicit
	}
	if requestOrigin := origin.String(); requestOrigin != "" {
		return requestOrigin
	}
	return strings.TrimRight(strings.TrimSpace(config.BackendBaseURL), "/")
}

func resolvePublicWebRTCHost(config Config, origin RequestOrigin) string {
	if explicit := strings.TrimSpace(config.WebRTCPublicHost); explicit != "" {
		if host := hostnameFromURL(explicit); host != "" {
			return host
		}
		return explicit
	}
	return hostnameFromURL(resolvePublicWebplayerBaseURL(config, origin))
}

func (origin RequestOrigin) String() string {
	scheme := strings.TrimSpace(origin.Scheme)
	host := strings.TrimSpace(origin.Host)
	if host == "" {
		return ""
	}
	if scheme == "" {
		scheme = "http"
	}
	return strings.TrimRight(scheme+"://"+host, "/")
}

func hostnameFromURL(value string) string {
	raw := strings.TrimSpace(value)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		parsed, err = url.Parse("//" + raw)
		if err != nil {
			return ""
		}
	}
	return strings.TrimSpace(parsed.Hostname())
}

func listItems(payload any, key string) []map[string]any {
	switch typed := payload.(type) {
	case []any:
		return mapItems(typed)
	case map[string]any:
		if raw, ok := typed[key].([]any); ok {
			return mapItems(raw)
		}
	}
	return nil
}

func mapItems(items []any) []map[string]any {
	mapped := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if row, ok := item.(map[string]any); ok {
			mapped = append(mapped, row)
		}
	}
	return mapped
}

func firstValue(values ...any) any {
	for _, value := range values {
		if clean(value) != "" {
			return value
		}
	}
	return nil
}

func stringValues(value any) []string {
	values := make([]string, 0)
	switch typed := value.(type) {
	case []string:
		for _, item := range typed {
			if normalized := clean(item); normalized != "" {
				values = append(values, normalized)
			}
		}
	case []any:
		for _, item := range typed {
			if normalized := clean(item); normalized != "" {
				values = append(values, normalized)
			}
		}
	}
	return values
}

func coercePort(value any, fallback int) int {
	parsed := positiveInt(value)
	if parsed > 0 {
		return parsed
	}
	return fallback
}

func positiveInt(value any) int {
	switch typed := value.(type) {
	case int:
		if typed > 0 {
			return typed
		}
	case int64:
		if typed > 0 {
			return int(typed)
		}
	case float64:
		if typed > 0 {
			return int(typed)
		}
	case json.Number:
		parsed, _ := typed.Int64()
		if parsed > 0 {
			return int(parsed)
		}
	case string:
		parsed, _ := strconv.Atoi(strings.TrimSpace(typed))
		if parsed > 0 {
			return parsed
		}
	}
	return 0
}

func clean(value any) string {
	if value == nil {
		return ""
	}
	return strings.Trim(strings.TrimSpace(fmt.Sprint(value)), "\"'")
}

func defaultText(value string, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

func SplitForwarded(value string) string {
	return strings.TrimSpace(strings.Split(strings.TrimSpace(value), ",")[0])
}

func HostWithoutPort(value string) string {
	host := strings.TrimSpace(value)
	if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		return parsedHost
	}
	return host
}
