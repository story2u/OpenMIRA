package observability

import (
	"bytes"
	"testing"
	"time"
)

func TestLoggerUsesCanonicalFormat(t *testing.T) {
	var output bytes.Buffer
	logger := Logger{
		name:   "test-api",
		output: &output,
		now: func() time.Time {
			return time.Date(2026, 6, 28, 12, 30, 45, 0, time.UTC)
		},
	}

	logger.Infof("started port=%d", 9000)

	want := "2026-06-28 12:30:45 - test-api - INFO - started port=9000\n"
	if output.String() != want {
		t.Fatalf("log line = %q, want %q", output.String(), want)
	}
}
