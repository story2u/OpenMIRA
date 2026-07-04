// Package devicesmanual owns the manual device write candidate.
package devicesmanual

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	// ErrAgentIDRequired matches the legacy manual device validation detail.
	ErrAgentIDRequired = errors.New("agent_id is required")
	// ErrDeviceIDRequired matches the legacy manual device validation detail.
	ErrDeviceIDRequired = errors.New("device_id is required")
	// ErrStoreUnavailable means the candidate was mounted without a devices store.
	ErrStoreUnavailable = errors.New("manual device store is not configured")
)

// Store persists manual device rows in the legacy devices table.
type Store interface {
	UpsertManualDevice(ctx context.Context, record Record) (Record, error)
	DeleteManualDevice(ctx context.Context, agentID string, deviceID string) (bool, error)
}

// EventPublisher publishes Python-compatible ws_hub events.
type EventPublisher interface {
	Publish(ctx context.Context, channel string, event string, topic string, payload map[string]any) error
}

// Service coordinates validation, storage, and realtime fanout.
type Service struct {
	Store  Store
	Events EventPublisher
	Now    func() time.Time
}

// UpsertCommand is the POST /api/v1/devices/manual request after HTTP defaults.
type UpsertCommand struct {
	AgentID        string
	DeviceID       string
	Online         bool
	WeWorkLoggedIn *bool
	Model          string
	AndroidVersion string
}

// Record mirrors the stable columns returned by the Python DeviceRepository.
type Record struct {
	AgentID         string
	DeviceID        string
	Online          bool
	WeWorkLoggedIn  *bool
	WeWorkStatus    *string
	Model           *string
	AndroidVersion  *string
	LastError       *string
	CPUUsage        *float64
	MemoryUsage     *float64
	AppInForeground *bool
	NetworkState    *string
	ClientVersion   *string
	Timestamp       time.Time
	Version         string
	TraceID         string
}

// UpsertManualDevice validates and stores a manual device row.
func (service Service) UpsertManualDevice(ctx context.Context, command UpsertCommand) (map[string]any, error) {
	if service.Store == nil {
		return nil, ErrStoreUnavailable
	}
	agentID := strings.TrimSpace(command.AgentID)
	if agentID == "" {
		return nil, ErrAgentIDRequired
	}
	deviceID := strings.TrimSpace(command.DeviceID)
	if deviceID == "" {
		return nil, ErrDeviceIDRequired
	}
	now := service.now().UTC()
	record := Record{
		AgentID:        agentID,
		DeviceID:       deviceID,
		Online:         command.Online,
		WeWorkLoggedIn: command.WeWorkLoggedIn,
		Model:          optionalText(command.Model),
		AndroidVersion: optionalText(command.AndroidVersion),
		Timestamp:      now,
		Version:        "manual",
		TraceID:        fmt.Sprintf("manual-%d", now.Unix()),
	}
	saved, err := service.Store.UpsertManualDevice(ctx, record)
	if err != nil {
		return nil, err
	}
	payload := saved.Payload()
	if service.Events != nil {
		if err := service.Events.Publish(ctx, "devices", "device.manual.upserted", "device.heartbeat", payload); err != nil {
			return nil, err
		}
	}
	return map[string]any{"success": true, "device": payload}, nil
}

// DeleteManualDevice validates and removes a manual device row by device key.
func (service Service) DeleteManualDevice(ctx context.Context, agentID string, deviceID string) (map[string]any, error) {
	if service.Store == nil {
		return nil, ErrStoreUnavailable
	}
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return nil, ErrAgentIDRequired
	}
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return nil, ErrDeviceIDRequired
	}
	deleted, err := service.Store.DeleteManualDevice(ctx, agentID, deviceID)
	if err != nil {
		return nil, err
	}
	if deleted && service.Events != nil {
		payload := map[string]any{"agent_id": agentID, "device_id": deviceID}
		if err := service.Events.Publish(ctx, "devices", "device.manual.deleted", "device.heartbeat", payload); err != nil {
			return nil, err
		}
	}
	return map[string]any{"success": deleted}, nil
}

// Payload returns the legacy JSON shape for a devices table row.
func (record Record) Payload() map[string]any {
	timestamp := ""
	if !record.Timestamp.IsZero() {
		timestamp = record.Timestamp.UTC().Format(time.RFC3339Nano)
	}
	return map[string]any{
		"agent_id":          record.AgentID,
		"device_id":         record.DeviceID,
		"online":            record.Online,
		"wework_logged_in":  optionalBoolValue(record.WeWorkLoggedIn),
		"wework_status":     optionalStringValue(record.WeWorkStatus),
		"model":             optionalStringValue(record.Model),
		"android_version":   optionalStringValue(record.AndroidVersion),
		"last_error":        optionalStringValue(record.LastError),
		"cpu_usage":         optionalFloatValue(record.CPUUsage),
		"memory_usage":      optionalFloatValue(record.MemoryUsage),
		"app_in_foreground": optionalBoolValue(record.AppInForeground),
		"network_state":     optionalStringValue(record.NetworkState),
		"client_version":    optionalStringValue(record.ClientVersion),
		"timestamp":         timestamp,
		"version":           record.Version,
		"trace_id":          record.TraceID,
	}
}

func (service Service) now() time.Time {
	if service.Now != nil {
		return service.Now()
	}
	return time.Now()
}

func optionalText(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func optionalStringValue(value *string) any {
	if value == nil {
		return nil
	}
	text := strings.TrimSpace(*value)
	if text == "" {
		return nil
	}
	return text
}

func optionalBoolValue(value *bool) any {
	if value == nil {
		return nil
	}
	return *value
}

func optionalFloatValue(value *float64) any {
	if value == nil {
		return nil
	}
	return *value
}
