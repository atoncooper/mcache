package infra

import (
	"encoding/json"
	"io"
	"maps"
	"os"
	"sync"
	"time"
)

// Logger is the abstraction for third-party log platforms (ELK, Splunk, Fluentd, etc.).
type Logger interface {
	Debug(msg string, fields map[string]any)
	Info(msg string, fields map[string]any)
	Warn(msg string, fields map[string]any)
	Error(msg string, fields map[string]any)
}

// StdoutLogger writes structured JSON logs to an io.Writer.
type StdoutLogger struct {
	w   io.Writer
	mu  sync.Mutex
	enc *json.Encoder
}

// NewStdoutLogger creates a JSON logger that writes to dest (default os.Stdout).
func NewStdoutLogger(dest io.Writer) *StdoutLogger {
	if dest == nil {
		dest = os.Stdout
	}
	return &StdoutLogger{w: dest, enc: json.NewEncoder(dest)}
}

// Debug writes a debug-level log entry.
func (l *StdoutLogger) Debug(msg string, fields map[string]any) {
	l.log("DEBUG", msg, fields)
}

// Info writes an info-level log entry.
func (l *StdoutLogger) Info(msg string, fields map[string]any) {
	l.log("INFO", msg, fields)
}

// Warn writes a warn-level log entry.
func (l *StdoutLogger) Warn(msg string, fields map[string]any) {
	l.log("WARN", msg, fields)
}

// Error writes an error-level log entry.
func (l *StdoutLogger) Error(msg string, fields map[string]any) {
	l.log("ERROR", msg, fields)
}

func (l *StdoutLogger) log(level, msg string, fields map[string]any) {
	entry := map[string]any{
		"ts":      time.Now().UTC().Format(time.RFC3339Nano),
		"level":   level,
		"message": msg,
	}
	maps.Copy(entry, fields)

	l.mu.Lock()
	l.enc.Encode(entry)
	l.mu.Unlock()
}
