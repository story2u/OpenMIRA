package devicebridge

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

// Target is the minimal ADB bridge target consumed by external supervisors.
type Target struct {
	DeviceID      string
	ADBDevice     string
	Host          string
	ADBPort       int
	Slot          any
	ContainerName string
	Identifiers   []string
}

// TargetStore reads bridge targets from Python-compatible disk snapshots.
type TargetStore struct {
	TargetsFile      string
	ManagerCacheFile string
}

// MediaConfig carries the P1 media relay template settings exposed by targets.
type MediaConfig struct {
	PlaybackTemplate      string
	PublishTemplate       string
	DirectPublishTemplate string
	P1PlaybackHost        string
}

// ListTargets loads manager-cache and explicit targets, then dedupes by device id.
func (store TargetStore) ListTargets(ctx context.Context) ([]Target, error) {
	_ = ctx
	targets := map[string]Target{}
	for _, target := range targetsFromFile(store.ManagerCacheFile, "devices") {
		targets[target.DeviceID] = target
	}
	for _, target := range targetsFromFile(store.TargetsFile, "targets") {
		targets[target.DeviceID] = target
	}
	ordered := make([]Target, 0, len(targets))
	for _, target := range targets {
		ordered = append(ordered, target)
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].DeviceID < ordered[j].DeviceID
	})
	return ordered, nil
}

// Row returns the device row shape used to match status identifiers.
func (target Target) Row() map[string]any {
	return map[string]any{
		"device_id":         target.DeviceID,
		"p1_host":           target.Host,
		"p1_adb_port":       target.ADBPort,
		"p1_container_name": target.ContainerName,
		"p1_aliases":        target.Identifiers,
	}
}

// Payload returns the legacy /targets item shape with attached bridge status.
func (target Target) Payload(callAudioBridge map[string]any) map[string]any {
	return map[string]any{
		"device_id":         target.DeviceID,
		"adb_device":        target.ADBDevice,
		"host":              target.Host,
		"adb_port":          target.ADBPort,
		"slot":              target.Slot,
		"container_name":    target.ContainerName,
		"identifiers":       target.Identifiers,
		"call_audio_bridge": callAudioBridge,
	}
}

// Status returns the legacy media_stream_config payload.
func (config MediaConfig) Status() map[string]any {
	playback := clean(config.PlaybackTemplate)
	publish := clean(config.PublishTemplate)
	missing := make([]string, 0, 2)
	if playback == "" {
		missing = append(missing, "RTC_MEDIA_CAMERA_ADDR_TEMPLATE")
	}
	if publish == "" {
		missing = append(missing, "RTC_MEDIA_WHIP_PUBLISH_URL_TEMPLATE")
	}
	configured := playback != "" && publish != ""
	status := "not_configured"
	if configured {
		status = "configured"
	}
	return map[string]any{
		"configured":              configured,
		"status":                  status,
		"missing":                 missing,
		"playback_template":       playback,
		"publish_template":        publish,
		"direct_publish_template": clean(config.DirectPublishTemplate),
		"p1_playback_host":        clean(config.P1PlaybackHost),
	}
}

func targetsFromFile(path string, listKey string) []Target {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var payload any
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		return nil
	}
	items := listItems(payload, listKey)
	targets := make([]Target, 0, len(items))
	for _, item := range items {
		if target, ok := targetFromSlot(item); ok {
			targets = append(targets, target)
		}
	}
	return targets
}

func listItems(payload any, listKey string) []map[string]any {
	switch typed := payload.(type) {
	case []any:
		return mapItems(typed)
	case map[string]any:
		if raw, ok := typed[listKey].([]any); ok {
			return mapItems(raw)
		}
		if raw, ok := typed["targets"].([]any); ok {
			return mapItems(raw)
		}
		if raw, ok := typed["devices"].([]any); ok {
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

func targetFromSlot(slot map[string]any) (Target, bool) {
	deviceID := clean(slot["device_id"])
	host := firstClean(slot["host"], slot["p1_host"])
	adbPort := positiveInt(firstValue(slot["p1_adb_port"], slot["adb"], slot["adb_port"]))
	adbDevice := clean(slot["adb_device"])
	if host == "" || adbPort <= 0 {
		host, adbPort = parseADBDevice(adbDevice)
	}
	if deviceID == "" || host == "" || adbPort <= 0 {
		return Target{}, false
	}
	adbDevice = fmt.Sprintf("%s:%d", host, adbPort)
	identifiers := make([]string, 0, 4)
	addIdentifier := func(value any) {
		normalized := clean(value)
		if normalized != "" && !containsString(identifiers, normalized) {
			identifiers = append(identifiers, normalized)
		}
	}
	addIdentifier(deviceID)
	addIdentifier(adbDevice)
	containerName := firstClean(slot["container_name"], slot["p1_container_name"])
	addIdentifier(containerName)
	for _, alias := range stringValues(firstValue(slot["identifiers"], slot["aliases"], slot["p1_aliases"])) {
		addIdentifier(alias)
	}
	return Target{
		DeviceID:      deviceID,
		ADBDevice:     adbDevice,
		Host:          host,
		ADBPort:       adbPort,
		Slot:          firstValue(slot["slot"], slot["p1_slot"]),
		ContainerName: containerName,
		Identifiers:   identifiers,
	}, true
}

func parseADBDevice(value string) (string, int) {
	normalized := strings.TrimSpace(value)
	index := strings.LastIndex(normalized, ":")
	if index <= 0 || index == len(normalized)-1 {
		return "", 0
	}
	host := strings.TrimSpace(normalized[:index])
	port := positiveInt(normalized[index+1:])
	if host == "" || port <= 0 {
		return "", 0
	}
	return host, port
}

func firstValue(values ...any) any {
	for _, value := range values {
		if value != nil && clean(value) != "" {
			return value
		}
	}
	return nil
}
