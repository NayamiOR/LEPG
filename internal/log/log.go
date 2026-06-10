// Package log provides dual-output logging with console (colorized) and file (plain text) handlers.
package log

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/SladkyCitron/slogcolor"
	"gopkg.in/natefinch/lumberjack.v2"
)

// Config holds logging configuration for Setup.
type Config struct {
	Level      string // "debug", "info", "warn", "error"
	Path       string // directory for log files, e.g. "logs/"
	MaxSize    int    // max MB per file (default 10)
	MaxBackups int    // max number of old files (default 3)
	MaxAge     int    // max days to retain (default 28)
	AppName    string // binary name used for the log filename, e.g. "lepgc"
}

// fileWriter holds the current lumberjack writer so it can be closed on shutdown.
var fileWriter *lumberjack.Logger

// multiHandler fans out each log record to both a console and a file handler.
type multiHandler struct {
	console slog.Handler
	file    slog.Handler
}

func (h *multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.console.Enabled(ctx, level) || h.file.Enabled(ctx, level)
}

func (h *multiHandler) Handle(ctx context.Context, r slog.Record) error {
	if err := h.console.Handle(ctx, r); err != nil {
		return err
	}
	return h.file.Handle(ctx, r)
}

func (h *multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &multiHandler{
		console: h.console.WithAttrs(attrs),
		file:    h.file.WithAttrs(attrs),
	}
}

func (h *multiHandler) WithGroup(name string) slog.Handler {
	return &multiHandler{
		console: h.console.WithGroup(name),
		file:    h.file.WithGroup(name),
	}
}

// ParseLevel converts a log level string to slog.Level.
func ParseLevel(s string) (slog.Level, error) {
	switch s {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("invalid log level: %q", s)
	}
}

// Setup configures the global slog default logger to output to both
// stderr (colorized via slogcolor) and a rotating log file (plain text).
//
// It should be called once after config is loaded, replacing the basic
// console-only logger set up in init().
func Setup(cfg Config) error {
	level, err := ParseLevel(cfg.Level)
	if err != nil {
		return err
	}

	// Apply defaults for rotation fields
	maxSize := cfg.MaxSize
	if maxSize <= 0 {
		maxSize = 10
	}
	maxBackups := cfg.MaxBackups
	if maxBackups <= 0 {
		maxBackups = 3
	}
	maxAge := cfg.MaxAge
	if maxAge <= 0 {
		maxAge = 28
	}

	// Ensure log directory exists
	if err := os.MkdirAll(cfg.Path, 0755); err != nil {
		return fmt.Errorf("create log directory %s: %w", cfg.Path, err)
	}

	logFile := filepath.Join(cfg.Path, cfg.AppName+".log")

	// File handler: rotating, plain text
	fileWriter = &lumberjack.Logger{
		Filename:   logFile,
		MaxSize:    maxSize,
		MaxBackups: maxBackups,
		MaxAge:     maxAge,
	}
	fileHandler := slog.NewTextHandler(fileWriter, &slog.HandlerOptions{Level: level})

	// Console handler: colorized
	consoleOpts := &slogcolor.Options{
		Level: level,
	}
	consoleHandler := slogcolor.NewHandler(os.Stderr, consoleOpts)

	slog.SetDefault(slog.New(&multiHandler{
		console: consoleHandler,
		file:    fileHandler,
	}))

	return nil
}

// Close flushes and releases the log file handle.
// Call this before process exit or in test cleanup.
func Close() error {
	if fileWriter != nil {
		return fileWriter.Close()
	}
	return nil
}
