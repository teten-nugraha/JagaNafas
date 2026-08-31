// Package logging sets up structured JSON logging (log/slog) that writes to
// both stdout (so `docker logs` / compose keep working) and a log file on
// disk, so troubleshooting doesn't require the container to still be running.
package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// New opens (creating parent dirs and the file if needed) the log file at
// path and returns a slog.Logger that writes JSON records to both it and
// stdout. Callers must Close() the returned io.Closer on shutdown to flush
// and release the file handle.
func New(path string, levelStr string) (*slog.Logger, io.Closer, error) {
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, nil, fmt.Errorf("create log dir %q: %w", dir, err)
		}
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, nil, fmt.Errorf("open log file %q: %w", path, err)
	}

	writer := io.MultiWriter(os.Stdout, f)
	handler := slog.NewJSONHandler(writer, &slog.HandlerOptions{
		Level:     parseLevel(levelStr),
		AddSource: true,
	})

	return slog.New(handler), f, nil
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
