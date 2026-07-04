// Package devicebridge stores lightweight MYT call-audio bridge heartbeats.
package devicebridge

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	statusVersion   = 1
	defaultStaleSec = 3600.0
)

var ErrDeviceIDRequired = errors.New("device_id or adb_device is required")

// Service reads and writes the call-audio bridge status document.
type Service struct {
	StatusFile   string
	HostDataRoot string
	StaleSec     float64
	Now          func() time.Time
}

// Read returns the normalized bridge status for one device id.
func (service Service) Read(deviceID string) map[string]any {
	key := clean(deviceID)
	raw := service.statusDevices()[key]
	if raw == nil {
		raw = service.findStatusByIdentifiers([]string{key})
	}
	if raw == nil {
		identifiers := []string{}
		if key != "" {
			identifiers = append(identifiers, key)
		}
		return notConfiguredStatus(key, identifiers)
	}
	identifiers := []string{}
	if key != "" {
		identifiers = append(identifiers, key)
	}
	return service.normalizeStatus(raw, identifiers)
}

// StatusForRow matches bridge status using the same row identifiers as device lists.
func (service Service) StatusForRow(row map[string]any) map[string]any {
	identifiers := rowIdentifiers(row)
	raw := service.findStatusByIdentifiers(identifiers)
	if raw == nil {
		return notConfiguredStatus("", identifiers)
	}
	return service.normalizeStatus(raw, identifiers)
}

// Write persists a heartbeat and returns the normalized status.
func (service Service) Write(deviceID string, payload map[string]any) (map[string]any, error) {
	key := firstClean(deviceID, payload["device_id"], payload["adb_device"])
	if key == "" {
		return nil, ErrDeviceIDRequired
	}
	now := service.now().UTC().Format(time.RFC3339Nano)
	status := compactStatus(payload, key, now)
	document := service.loadDocument()
	devices, _ := document["devices"].(map[string]any)
	if devices == nil {
		devices = map[string]any{}
	}
	devices[key] = status
	document["version"] = statusVersion
	document["updated_at"] = now
	document["devices"] = devices
	if err := service.writeDocument(document); err != nil {
		return nil, err
	}
	return service.normalizeStatus(status, []string{key}), nil
}

func (service Service) loadDocument() map[string]any {
	raw, err := os.ReadFile(service.statusFile())
	if err != nil {
		return map[string]any{"version": statusVersion, "devices": map[string]any{}}
	}
	var data any
	if err := json.Unmarshal(raw, &data); err != nil {
		return map[string]any{"version": statusVersion, "devices": map[string]any{}}
	}
	document, ok := data.(map[string]any)
	if !ok {
		return map[string]any{"version": statusVersion, "devices": map[string]any{}}
	}
	if devices, ok := document["devices"].(map[string]any); ok {
		return map[string]any{"version": intValue(document["version"]), "devices": devices}
	}
	return map[string]any{"version": statusVersion, "devices": document}
}

func (service Service) writeDocument(document map[string]any) error {
	path := service.statusFile()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	tmp := filepath.Join(filepath.Dir(path), filepath.Base(path)+".tmp")
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (service Service) statusDevices() map[string]map[string]any {
	document := service.loadDocument()
	rawDevices, _ := document["devices"].(map[string]any)
	devices := map[string]map[string]any{}
	for key, value := range rawDevices {
		normalizedKey := clean(key)
		entry, ok := value.(map[string]any)
		if normalizedKey == "" || !ok {
			continue
		}
		devices[normalizedKey] = entry
	}
	return devices
}

func (service Service) findStatusByIdentifiers(identifiers []string) map[string]any {
	wanted := map[string]struct{}{}
	for _, identifier := range identifiers {
		if value := clean(identifier); value != "" {
			wanted[value] = struct{}{}
		}
	}
	if len(wanted) == 0 {
		return nil
	}
	for key, value := range service.statusDevices() {
		candidates := entryIdentifiers(key, value)
		for candidate := range candidates {
			if _, ok := wanted[candidate]; ok {
				matched := copyMap(value)
				matched["matched_identifier"] = candidate
				return matched
			}
		}
	}
	return nil
}

func (service Service) normalizeStatus(raw map[string]any, identifiers []string) map[string]any {
	raw = service.mergeRuntimeLogStatus(raw)
	updatedAt := clean(raw["updated_at"])
	ageSec := service.ageSeconds(updatedAt)
	stale := true
	var age any
	if ageSec != nil {
		stale = *ageSec > service.staleSec()
		age = *ageSec
	}
	configured := boolValue(raw["configured"]) || boolValue(raw["running"])
	running := boolValue(raw["running"]) && !stale
	status := clean(raw["status"])
	if status == "" {
		switch {
		case running:
			status = "running"
		case configured:
			status = "configured"
		default:
			status = "not_configured"
		}
	}
	if stale && configured {
		status = "stale"
	}
	return map[string]any{
		"configured":          configured,
		"running":             running,
		"stale":               stale,
		"status":              status,
		"detail":              clean(raw["detail"]),
		"device_id":           clean(raw["device_id"]),
		"adb_device":          clean(raw["adb_device"]),
		"frida_port":          positiveInt(raw["frida_port"]),
		"process_id":          positiveInt(raw["process_id"]),
		"log_file":            clean(raw["log_file"]),
		"source":              clean(raw["source"]),
		"updated_at":          updatedAt,
		"runtime_observed_at": clean(raw["runtime_observed_at"]),
		"age_sec":             age,
		"matched_identifier":  clean(raw["matched_identifier"]),
		"identifiers":         identifiers,
	}
}

func compactStatus(payload map[string]any, key string, updatedAt string) map[string]any {
	identifiers := make([]string, 0)
	if raw, ok := payload["identifiers"].([]any); ok {
		for _, item := range raw {
			value := clean(item)
			if value != "" && !containsString(identifiers, value) {
				identifiers = append(identifiers, value)
			}
		}
	}
	if len(identifiers) > 20 {
		identifiers = identifiers[:20]
	}
	return map[string]any{
		"configured":          boolValue(payload["configured"]) || boolValue(payload["running"]),
		"running":             boolValue(payload["running"]),
		"status":              limit(clean(payload["status"]), 80),
		"detail":              limit(clean(payload["detail"]), 1000),
		"device_id":           limit(key, 200),
		"adb_device":          limit(clean(payload["adb_device"]), 200),
		"frida_port":          positiveInt(payload["frida_port"]),
		"process_id":          positiveInt(payload["process_id"]),
		"log_file":            limit(clean(payload["log_file"]), 1000),
		"source":              limit(clean(payload["source"]), 120),
		"runtime_observed_at": limit(clean(payload["runtime_observed_at"]), 80),
		"identifiers":         identifiers,
		"updated_at":          updatedAt,
	}
}

func (service Service) ageSeconds(updatedAt string) *float64 {
	if updatedAt == "" {
		return nil
	}
	parsed, err := parseTimestamp(updatedAt)
	if err != nil {
		return nil
	}
	age := service.now().UTC().Sub(parsed.UTC()).Seconds()
	if age < 0 {
		age = 0
	}
	return &age
}

func (service Service) statusFile() string {
	return strings.TrimSpace(service.StatusFile)
}

func (service Service) staleSec() float64 {
	if service.StaleSec < 30 {
		return defaultStaleSec
	}
	return service.StaleSec
}

func (service Service) now() time.Time {
	if service.Now != nil {
		return service.Now()
	}
	return time.Now()
}

func entryIdentifiers(key string, value map[string]any) map[string]struct{} {
	identifiers := map[string]struct{}{}
	for _, item := range []any{key, value["device_id"], value["adb_device"]} {
		if normalized := clean(item); normalized != "" {
			identifiers[normalized] = struct{}{}
		}
	}
	if raw, ok := value["identifiers"].([]any); ok {
		for _, item := range raw {
			if normalized := clean(item); normalized != "" {
				identifiers[normalized] = struct{}{}
			}
		}
	}
	if raw, ok := value["identifiers"].([]string); ok {
		for _, item := range raw {
			if normalized := clean(item); normalized != "" {
				identifiers[normalized] = struct{}{}
			}
		}
	}
	return identifiers
}

func rowIdentifiers(row map[string]any) []string {
	identifiers := make([]string, 0)
	add := func(value any) {
		normalized := clean(value)
		if normalized != "" && !containsString(identifiers, normalized) {
			identifiers = append(identifiers, normalized)
		}
	}
	add(row["device_id"])
	add(row["p1_container_name"])
	for _, alias := range stringValues(row["p1_aliases"]) {
		add(alias)
	}
	adbPort := positiveInt(row["p1_adb_port"])
	if adbPort > 0 {
		for _, key := range []string{"p1_host", "p1_device_ip", "p1_manager_host"} {
			if host := clean(row[key]); host != "" {
				add(fmt.Sprintf("%s:%d", host, adbPort))
			}
		}
	}
	return identifiers
}

func notConfiguredStatus(matchedIdentifier string, identifiers []string) map[string]any {
	return map[string]any{
		"configured":         false,
		"running":            false,
		"stale":              false,
		"status":             "not_configured",
		"detail":             "call audio bridge status has not been reported",
		"matched_identifier": matchedIdentifier,
		"identifiers":        identifiers,
	}
}

func copyMap(value map[string]any) map[string]any {
	copied := make(map[string]any, len(value))
	for key, item := range value {
		copied[key] = item
	}
	return copied
}

func firstClean(values ...any) string {
	for _, value := range values {
		if normalized := clean(value); normalized != "" {
			return normalized
		}
	}
	return ""
}

func clean(value any) string {
	if value == nil {
		return ""
	}
	return strings.Trim(strings.TrimSpace(fmt.Sprint(value)), "\"'")
}

func parseTimestamp(value string) (time.Time, error) {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return time.Time{}, fmt.Errorf("empty timestamp")
	}
	if parsed, err := time.Parse(time.RFC3339Nano, strings.ReplaceAll(normalized, "Z", "+00:00")); err == nil {
		return parsed, nil
	}
	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02T15:04:05"} {
		if parsed, err := time.ParseInLocation(layout, normalized, time.UTC); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid timestamp")
}

func limit(value string, maxLen int) string {
	if len(value) <= maxLen {
		return value
	}
	return value[:maxLen]
}

func boolValue(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "1", "true", "yes", "on":
			return true
		default:
			return false
		}
	default:
		return false
	}
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

func intValue(value any) int {
	parsed := positiveInt(value)
	if parsed == 0 {
		return statusVersion
	}
	return parsed
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
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
