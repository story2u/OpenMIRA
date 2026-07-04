// Status validation mirrors TaskStatusUpdate from the Python API schema.
package tasks

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// ValidateStatusUpdateJSON parses POST /tasks/{task_id}/status bodies.
func ValidateStatusUpdateJSON(data []byte) (StatusUpdate, error) {
	var raw map[string]json.RawMessage
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&raw); err != nil {
		return StatusUpdate{}, invalid("body", "must be a JSON object")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return StatusUpdate{}, invalid("body", "must contain exactly one JSON object")
	}
	statusRaw, ok := raw["status"]
	if !ok {
		return StatusUpdate{}, invalid("status", "is required")
	}
	statusText, err := decodeString(statusRaw, "status")
	if err != nil {
		return StatusUpdate{}, err
	}
	status := Status(statusText)
	if !ValidStatus(status) {
		return StatusUpdate{}, invalid("status", "is not allowed")
	}
	var errorText *string
	if errorRaw, ok := raw["error"]; ok && string(errorRaw) != "null" {
		var value string
		if err := json.Unmarshal(errorRaw, &value); err != nil {
			return StatusUpdate{}, invalid("error", "must be a string")
		}
		if strings.TrimSpace(value) == "" {
			value = ""
		}
		errorText = &value
	}
	return StatusUpdate{Status: status, Error: errorText}, nil
}

// ParseQuery converts the legacy list-task query fields into a service query.
func ParseQuery(values map[string][]string) (Query, error) {
	var query Query
	statusText := firstQuery(values, "status")
	if statusText != "" {
		status := Status(statusText)
		if !ValidStatus(status) {
			return Query{}, fmt.Errorf("%w: status: is not allowed", ErrInvalidCreate)
		}
		query.Status = &status
	}
	query.AgentID = firstQuery(values, "agent_id")
	query.DeviceID = firstQuery(values, "device_id")
	query.TaskType = firstQuery(values, "task_type")
	limitText := firstQuery(values, "limit")
	if limitText != "" {
		parsed, err := strconv.Atoi(limitText)
		if err != nil {
			return Query{}, fmt.Errorf("%w: limit: must be an integer", ErrInvalidCreate)
		}
		query.Limit = &parsed
	}
	return query, nil
}

func firstQuery(values map[string][]string, key string) string {
	items := values[key]
	if len(items) == 0 {
		return ""
	}
	return strings.TrimSpace(items[0])
}
