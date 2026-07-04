// Package observability centralizes phase-two logging and request metadata.
// It keeps the Go rewrite on the same timestamp/logger/level/message shape
// required by the legacy operational discipline.
package observability

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// Logger writes human-readable operational logs in the repository format.
type Logger struct {
	name   string
	output io.Writer
	now    func() time.Time
}

// NewLogger builds a stdout logger with the given component name.
func NewLogger(name string) Logger {
	return Logger{name: cleanLoggerName(name), output: os.Stdout, now: time.Now}
}

// NewLoggerWithOutput builds a logger for tests or command-specific sinks.
func NewLoggerWithOutput(name string, output io.Writer) Logger {
	if output == nil {
		output = os.Stdout
	}
	return Logger{name: cleanLoggerName(name), output: output, now: time.Now}
}

// Infof writes one INFO log line.
func (logger Logger) Infof(format string, args ...any) {
	logger.logf("INFO", format, args...)
}

// Errorf writes one ERROR log line.
func (logger Logger) Errorf(format string, args ...any) {
	logger.logf("ERROR", format, args...)
}

// logf writes one formatted line with the canonical timestamp and level.
func (logger Logger) logf(level string, format string, args ...any) {
	if logger.output == nil {
		logger.output = os.Stdout
	}
	if logger.now == nil {
		logger.now = time.Now
	}
	message := fmt.Sprintf(format, args...)
	timestamp := logger.now().Format("2006-01-02 15:04:05")
	fmt.Fprintf(logger.output, "%s - %s - %s - %s\n", timestamp, cleanLoggerName(logger.name), level, message)
}

// cleanLoggerName keeps empty names from leaking into operational logs.
func cleanLoggerName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "wework-go"
	}
	return name
}
