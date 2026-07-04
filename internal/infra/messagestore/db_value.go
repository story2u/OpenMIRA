// DB value conversion helpers keep row scanning tolerant of MySQL, SQLite, and
// test fake value types while the message read model is migrated in slices.
package messagestore

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

func parseInt64(value string) (int64, bool) {
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	return parsed, err == nil
}

func intFromDB(value any) int {
	parsed, _ := int64FromDB(value)
	return int(parsed)
}

func int64ValueFromDB(value any) int64 {
	parsed, _ := int64FromDB(value)
	return parsed
}

func int64PtrFromDB(value any) *int64 {
	parsed, ok := int64FromDB(value)
	if !ok {
		return nil
	}
	return &parsed
}

func int64FromDB(value any) (int64, bool) {
	switch current := value.(type) {
	case nil:
		return 0, false
	case int:
		return int64(current), true
	case int64:
		return current, true
	case int32:
		return int64(current), true
	case []byte:
		return parseInt64(string(current))
	case string:
		return parseInt64(current)
	default:
		return parseInt64(fmt.Sprint(current))
	}
}

func stringFromDB(value any) string {
	switch current := value.(type) {
	case nil:
		return ""
	case []byte:
		return strings.TrimSpace(string(current))
	case string:
		return strings.TrimSpace(current)
	case fmt.Stringer:
		return strings.TrimSpace(current.String())
	default:
		return strings.TrimSpace(fmt.Sprint(current))
	}
}

func boolFromDB(value any) bool {
	switch current := value.(type) {
	case bool:
		return current
	case int:
		return current != 0
	case int64:
		return current != 0
	case int32:
		return current != 0
	case []byte:
		return boolString(string(current))
	case string:
		return boolString(current)
	default:
		return false
	}
}

func boolString(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func firstNonNil(values ...any) any {
	for _, value := range values {
		if value != nil && strings.TrimSpace(fmt.Sprint(value)) != "" {
			return value
		}
	}
	return nil
}

func timePtrFromDB(value any) *time.Time {
	parsed := timeFromDB(value)
	if parsed.IsZero() {
		return nil
	}
	return &parsed
}

func timeFromDB(value any) time.Time {
	switch current := value.(type) {
	case nil:
		return time.Time{}
	case time.Time:
		return current
	case []byte:
		return parseTimeString(string(current))
	case string:
		return parseTimeString(current)
	default:
		return parseTimeString(fmt.Sprint(current))
	}
}

func parseTimeString(value string) time.Time {
	text := strings.TrimSpace(value)
	if text == "" {
		return time.Time{}
	}
	for _, layout := range []string{
		time.RFC3339Nano,
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
	} {
		if parsed, err := time.ParseInLocation(layout, text, beijingLocation); err == nil {
			return parsed
		}
	}
	return time.Time{}
}
