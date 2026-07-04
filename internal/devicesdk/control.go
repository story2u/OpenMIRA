package devicesdk

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"time"

	"wework-go/internal/tasks"
)

// TaskCreator stores one SDK control task.
type TaskCreator interface {
	Create(ctx context.Context, request tasks.CreateRequest) (tasks.Record, error)
}

// Control submits one legacy SDK device control task.
func (service Service) Control(ctx context.Context, deviceID string, taskType string, payload map[string]any) (map[string]any, error) {
	managerDevice, ok := service.findSlot(deviceID)
	if !ok {
		return nil, ErrSDKDeviceNotConfigured
	}
	if service.TaskCreator == nil {
		return nil, ErrSDKTaskServiceNotConfigured
	}
	slot := legacySlot(managerDevice, deviceID)
	canonicalDeviceID := clean(firstValue(slot["device_id"], deviceID))
	traceID := service.newID("trace-")
	record, err := service.TaskCreator.Create(ctx, tasks.CreateRequest{
		TaskID:    service.newID("task-"),
		Source:    "cloud-web",
		Target:    tasks.Target{AgentID: "sdk:" + canonicalDeviceID, DeviceID: canonicalDeviceID},
		TaskType:  strings.TrimSpace(taskType),
		Payload:   clonePayload(payload),
		CreatedAt: service.now(),
		TraceID:   &traceID,
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"success": controlTaskSuccess(record.Status),
		"task":    record,
		"result":  map[string]any{},
	}, nil
}

func (service Service) now() time.Time {
	if service.Now != nil {
		return service.Now().UTC()
	}
	return time.Now().UTC()
}

func (service Service) newID(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	if service.NewID != nil {
		return service.NewID(prefix)
	}
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return prefix + strings.ReplaceAll(time.Now().UTC().Format("20060102150405.000000000"), ".", "")
	}
	return prefix + hex.EncodeToString(bytes[:])
}

func controlTaskSuccess(status tasks.Status) bool {
	switch status {
	case tasks.StatusAccepted, tasks.StatusRunning, tasks.StatusSuccess:
		return true
	default:
		return false
	}
}

func clonePayload(payload map[string]any) map[string]any {
	cloned := make(map[string]any, len(payload))
	for key, value := range payload {
		cloned[key] = value
	}
	return cloned
}
