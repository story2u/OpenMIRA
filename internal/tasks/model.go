// Package tasks contains the phase-six task contract primitives.
// The package does not execute SDK work; it only validates legacy task-create
// payloads and builds accepted task records for later storage adapters.
package tasks

import "time"

// Status is the legacy task status enum.
type Status string

const (
	StatusAccepted  Status = "accepted"
	StatusRunning   Status = "running"
	StatusSuccess   Status = "success"
	StatusFailed    Status = "failed"
	StatusTimeout   Status = "timeout"
	StatusCancelled Status = "cancelled"
)

// ValidStatus reports whether a status belongs to the legacy enum.
func ValidStatus(status Status) bool {
	switch status {
	case StatusAccepted, StatusRunning, StatusSuccess, StatusFailed, StatusTimeout, StatusCancelled:
		return true
	default:
		return false
	}
}

// Target mirrors TaskTarget in the Python domain model.
type Target struct {
	AgentID  string `json:"agent_id"`
	DeviceID string `json:"device_id"`
}

// CreateRequest is a validated task-create payload.
type CreateRequest struct {
	TaskID       string         `json:"task_id"`
	Source       string         `json:"source"`
	Target       Target         `json:"target"`
	TaskType     string         `json:"task_type"`
	Payload      map[string]any `json:"payload"`
	CreatedAt    time.Time      `json:"created_at"`
	TraceID      *string        `json:"trace_id"`
	WeWorkUserID *string        `json:"-"`
	EnterpriseID *string        `json:"-"`
}

// Record mirrors the HTTP response shape of Python TaskRecord.
type Record struct {
	TaskID                string         `json:"task_id"`
	Source                string         `json:"source"`
	Target                Target         `json:"target"`
	TaskType              string         `json:"task_type"`
	Payload               map[string]any `json:"payload"`
	Status                Status         `json:"status"`
	CreatedAt             time.Time      `json:"created_at"`
	UpdatedAt             time.Time      `json:"updated_at"`
	TraceID               *string        `json:"trace_id"`
	Error                 *string        `json:"error"`
	RetryCount            int            `json:"retry_count"`
	NextRetryAt           *time.Time     `json:"next_retry_at"`
	WeWorkUserID          *string        `json:"wework_user_id"`
	EnterpriseID          *string        `json:"enterprise_id"`
	DispatchedAt          *time.Time     `json:"dispatched_at"`
	ScriptStartedAt       *time.Time     `json:"script_started_at"`
	SkippedDeviceDispatch bool           `json:"skipped_device_dispatch,omitempty"`
}

// Query filters task list reads with the same fields as the Python endpoint.
type Query struct {
	Status   *Status
	AgentID  string
	DeviceID string
	TaskType string
	Limit    *int
}

// StatusUpdate is the validated body for POST /tasks/{task_id}/status.
type StatusUpdate struct {
	Status          Status
	Error           *string
	UpdatedAt       *time.Time
	DispatchedAt    *time.Time
	ScriptStartedAt *time.Time
}

// NewAcceptedRecord creates the initial accepted state without dispatching work.
func NewAcceptedRecord(request CreateRequest, now time.Time) Record {
	if now.IsZero() {
		now = time.Now()
	}
	now = now.UTC()
	return Record{
		TaskID:       request.TaskID,
		Source:       request.Source,
		Target:       request.Target,
		TaskType:     request.TaskType,
		Payload:      request.Payload,
		Status:       StatusAccepted,
		CreatedAt:    request.CreatedAt,
		UpdatedAt:    now,
		TraceID:      request.TraceID,
		RetryCount:   0,
		WeWorkUserID: request.WeWorkUserID,
		EnterpriseID: request.EnterpriseID,
	}
}
