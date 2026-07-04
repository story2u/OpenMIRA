// Package replay compares legacy Python and Go websocket/event stream samples.
// It is a phase-2+ harness building block to support replay and event schema
// compatibility work before high-risk streaming migration.
package replay

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// Event represents one websocket-like domain event with optional metadata.
type Event struct {
	Channel   string         `json:"channel,omitempty"`
	EventType string         `json:"event_type,omitempty"`
	Cursor    string         `json:"cursor,omitempty"`
	Timestamp string         `json:"timestamp,omitempty"`
	Raw       map[string]any `json:"-"`
}

// EventSummary keeps the stable identity fields for drift reporting.
type EventSummary struct {
	Channel   string
	EventType string
	Cursor    string
	Timestamp string
}

// CompareOptions configures event comparison behavior.
type CompareOptions struct {
	IgnoreJSONFields []string
}

// ComparisonResult is a deterministic event drift unit.
type ComparisonResult struct {
	Index  int          `json:"index"`
	Match  bool         `json:"match"`
	Python EventSummary `json:"python"`
	Go     EventSummary `json:"go"`
	Diffs  []string     `json:"diffs"`
}

// ComparisonReport is the machine-readable output for replay/gate assertions.
type ComparisonReport struct {
	Name            string             `json:"name"`
	Mode            string             `json:"mode"`
	Match           bool               `json:"match"`
	PairCount       int                `json:"pair_count"`
	PythonCount     int                `json:"python_count"`
	GoCount         int                `json:"go_count"`
	MissingInGo     int                `json:"missing_in_go"`
	MissingInPython int                `json:"missing_in_python"`
	Results         []ComparisonResult `json:"results"`
}

// LoadEvents reads event fixtures from disk.
//
// Supported formats:
// 1) JSON array: [{...}, {...}]
// 2) JSON object with "events" array: {"events":[...]}
// 3) JSON object single event: {...}
// 4) NDJSON: one json object per line
func LoadEvents(path string) ([]Event, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read replay fixture %q: %w", path, err)
	}
	if events, err := parseEventsJSON(raw); err == nil {
		if len(events) == 0 {
			return nil, fmt.Errorf("no events in replay fixture %q", path)
		}
		return events, nil
	}
	events, err := parseEventsNDJSON(raw)
	if err != nil {
		return nil, fmt.Errorf("parse replay fixture %q: %w", path, err)
	}
	if len(events) == 0 {
		return nil, fmt.Errorf("no events in replay fixture %q", path)
	}
	return events, nil
}

// CompareStreams compares python and go fixture streams in index order.
func CompareStreams(name string, pythonEvents []Event, goEvents []Event, options CompareOptions) ComparisonReport {
	report := ComparisonReport{
		Name:        name,
		Mode:        "compare",
		Match:       true,
		PythonCount: len(pythonEvents),
		GoCount:     len(goEvents),
		PairCount:   minInt(len(pythonEvents), len(goEvents)),
	}
	maxCount := maxInt(len(pythonEvents), len(goEvents))

	for idx := 0; idx < maxCount; idx++ {
		switch {
		case idx >= len(pythonEvents):
			report.MissingInPython++
			goSummary := eventSummary(goEvents[idx])
			result := ComparisonResult{
				Index: idx,
				Match: false,
				Go:    goSummary,
				Diffs: []string{"python event missing"},
			}
			report.Results = append(report.Results, result)
			report.Match = false
		case idx >= len(goEvents):
			report.MissingInGo++
			pythonSummary := eventSummary(pythonEvents[idx])
			result := ComparisonResult{
				Index:  idx,
				Match:  false,
				Python: pythonSummary,
				Diffs:  []string{"go event missing"},
			}
			report.Results = append(report.Results, result)
			report.Match = false
		default:
			result := compareEventPair(idx, pythonEvents[idx], goEvents[idx], options)
			report.Results = append(report.Results, result)
			if !result.Match {
				report.Match = false
			}
		}
	}
	return report
}

func compareEventPair(index int, python Event, goEvent Event, options CompareOptions) ComparisonResult {
	result := ComparisonResult{
		Index:  index,
		Python: eventSummary(python),
		Go:     eventSummary(goEvent),
	}

	if python.Channel != goEvent.Channel {
		result.Diffs = append(result.Diffs, fmt.Sprintf("channel mismatch: python=%q go=%q", python.Channel, goEvent.Channel))
	}
	if python.EventType != goEvent.EventType {
		result.Diffs = append(result.Diffs, fmt.Sprintf("event mismatch: python=%q go=%q", python.EventType, goEvent.EventType))
	}
	if python.Cursor != "" || goEvent.Cursor != "" {
		if python.Cursor != goEvent.Cursor {
			result.Diffs = append(result.Diffs, fmt.Sprintf("cursor mismatch: python=%q go=%q", python.Cursor, goEvent.Cursor))
		}
	}
	if !bytes.Equal(normalizeEventBody(python.Raw, options.IgnoreJSONFields), normalizeEventBody(goEvent.Raw, options.IgnoreJSONFields)) {
		result.Diffs = append(result.Diffs, "payload mismatch")
	}
	result.Match = len(result.Diffs) == 0
	return result
}

func parseEventsJSON(raw []byte) ([]Event, error) {
	var root any
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("empty payload")
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.UseNumber()
	if err := decoder.Decode(&root); err != nil {
		return nil, err
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("replay JSON must be a single JSON value: %w", err)
	}

	switch container := root.(type) {
	case []any:
		return parseEventSlice(container)
	case map[string]any:
		if eventsRaw, ok := container["events"]; ok {
			if eventList, ok := eventsRaw.([]any); ok {
				return parseEventSlice(eventList)
			}
			return nil, fmt.Errorf("\"events\" field must be an array")
		}
		return parseEventSlice([]any{container})
	default:
		return nil, fmt.Errorf("replay JSON must be an event array or object")
	}
}

func parseEventsNDJSON(raw []byte) ([]Event, error) {
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	events := make([]Event, 0)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var item map[string]any
		if err := json.Unmarshal(line, &item); err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNo, err)
		}
		event, err := parseEventObject(item, fmt.Sprintf("line %d", lineNo))
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return events, nil
}

func parseEventSlice(values []any) ([]Event, error) {
	events := make([]Event, 0, len(values))
	for idx, value := range values {
		valueMap, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("event[%d] must be an object", idx)
		}
		event, err := parseEventObject(valueMap, fmt.Sprintf("event[%d]", idx))
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, nil
}

func parseEventObject(valueMap map[string]any, source string) (Event, error) {
	raw := cloneJSONMap(valueMap)
	eventType := firstNonEmpty(strings.TrimSpace(asString(valueMap["event"])), strings.TrimSpace(asString(valueMap["type"])), strings.TrimSpace(asString(valueMap["event_type"])), strings.TrimSpace(asString(valueMap["name"])))
	if eventType == "" {
		eventType = strings.TrimSpace(asString(valueMap["eventType"]))
	}
	if eventType == "" {
		return Event{}, fmt.Errorf("%s: event name is required", source)
	}

	return Event{
		Channel:   asString(valueMap["channel"]),
		EventType: eventType,
		Cursor:    firstNonEmpty(asString(valueMap["cursor"]), asString(valueMap["after_cursor"]), asString(valueMap["seq"])),
		Timestamp: firstNonEmpty(asString(valueMap["timestamp"]), asString(valueMap["time"]), asString(valueMap["created_at"])),
		Raw:       raw,
	}, nil
}

func eventSummary(event Event) EventSummary {
	return EventSummary{
		Channel:   event.Channel,
		EventType: event.EventType,
		Cursor:    event.Cursor,
		Timestamp: event.Timestamp,
	}
}

func normalizeEventBody(raw map[string]any, ignoreFields []string) []byte {
	if raw == nil {
		return nil
	}
	copy := cloneJSONMap(raw)
	for _, fieldPath := range ignoreFields {
		removeJSONPath(copy, splitFieldPath(fieldPath))
	}
	normalized, err := json.Marshal(copy)
	if err != nil {
		return nil
	}
	return normalized
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func asString(value any) string {
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}

func cloneJSONMap(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	b, err := json.Marshal(value)
	if err != nil {
		cloned := make(map[string]any, len(value))
		for k, v := range value {
			cloned[k] = v
		}
		return cloned
	}
	var copied map[string]any
	if err := json.Unmarshal(b, &copied); err != nil {
		cloned := make(map[string]any, len(value))
		for k, v := range value {
			cloned[k] = v
		}
		return cloned
	}
	return copied
}

func removeJSONPath(value any, path []string) {
	if len(path) == 0 {
		return
	}
	switch node := value.(type) {
	case map[string]any:
		if len(path) == 1 {
			delete(node, path[0])
			return
		}
		removeJSONPath(node[path[0]], path[1:])
	case []any:
		for _, item := range node {
			removeJSONPath(item, path)
		}
	}
}

func splitFieldPath(path string) []string {
	parts := strings.Split(path, ".")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func minInt(x int, y int) int {
	if x < y {
		return x
	}
	return y
}

func maxInt(x int, y int) int {
	if x > y {
		return x
	}
	return y
}
