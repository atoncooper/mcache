package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/atoncooper/mcache"
	"github.com/atoncooper/mcache/infra"
)

var cliLogger infra.Logger
var cliLoggerMu sync.RWMutex

func init() {
	cliLogger = newDefaultLogger()
}

func newDefaultLogger() infra.Logger {
	return &textLogger{w: os.Stderr, levelIdx: levelToIndex("info")}
}

// SetLogger replaces the package-level logger (used by all CLI commands).
// Safe for concurrent use.
func SetLogger(l infra.Logger) {
	cliLoggerMu.Lock()
	defer cliLoggerMu.Unlock()
	cliLogger = l
}

// Logger returns the current package-level logger.
func Logger() infra.Logger {
	cliLoggerMu.RLock()
	defer cliLoggerMu.RUnlock()
	return cliLogger
}

// level ordering for filtering
func levelToIndex(level string) int {
	switch strings.ToLower(level) {
	case "debug":
		return 0
	case "info":
		return 1
	case "warn":
		return 2
	case "error":
		return 3
	default:
		return 1 // default to info
	}
}

func initLogger(cfg mcache.LoggingConfig) infra.Logger {
	var writers []io.Writer
	writers = append(writers, os.Stdout)

	output := cfg.Output
	if output == "" {
		output = "stdout"
	}

	if output != "stdout" {
		dir := filepath.Dir(output)
		if dir != "." && dir != "/" {
			_ = os.MkdirAll(dir, 0755)
		}
		f, err := os.OpenFile(output, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: open log file %s: %v\n", output, err)
			os.Exit(1)
		}
		writers = append(writers, f)
	}

	w := io.MultiWriter(writers...)
	idx := levelToIndex(cfg.Level)
	format := strings.ToLower(cfg.Format)
	if format == "text" {
		return &textLogger{w: w, levelIdx: idx}
	}
	return &jsonLogger{inner: infra.NewStdoutLogger(w), levelIdx: idx}
}

// --- jsonLogger ---

type jsonLogger struct {
	inner    infra.Logger
	levelIdx int
}

func (l *jsonLogger) Debug(msg string, fields map[string]any) {
	if l.levelIdx > 0 {
		return
	}
	l.inner.Debug(msg, fields)
}

func (l *jsonLogger) Info(msg string, fields map[string]any) {
	if l.levelIdx > 1 {
		return
	}
	l.inner.Info(msg, fields)
}

func (l *jsonLogger) Warn(msg string, fields map[string]any) {
	if l.levelIdx > 2 {
		return
	}
	l.inner.Warn(msg, fields)
}

func (l *jsonLogger) Error(msg string, fields map[string]any) {
	l.inner.Error(msg, fields)
}

// --- textLogger ---

type textLogger struct {
	w        io.Writer
	levelIdx int
}

func (l *textLogger) Debug(msg string, fields map[string]any) {
	if l.levelIdx > 0 {
		return
	}
	l.write("DEBUG", msg, fields)
}

func (l *textLogger) Info(msg string, fields map[string]any) {
	if l.levelIdx > 1 {
		return
	}
	l.write("INFO", msg, fields)
}

func (l *textLogger) Warn(msg string, fields map[string]any) {
	if l.levelIdx > 2 {
		return
	}
	l.write("WARN", msg, fields)
}

func (l *textLogger) Error(msg string, fields map[string]any) {
	l.write("ERROR", msg, fields)
}

func (l *textLogger) write(level, msg string, fields map[string]any) {
	fmt.Fprintf(l.w, "[%s] %s %s", time.Now().Format("2006-01-02T15:04:05"), level, msg)
	if len(fields) > 0 {
		for k, v := range fields {
			fmt.Fprintf(l.w, " %s=%v", k, v)
		}
	}
	fmt.Fprintln(l.w)
}
