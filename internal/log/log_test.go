package log

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input string
		want  string // expected slog.Level string, or "error" for error
	}{
		{"debug", "DEBUG"},
		{"info", "INFO"},
		{"warn", "WARN"},
		{"error", "ERROR"},
		{"verbose", "error"},
		{"", "error"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			level, err := ParseLevel(tt.input)
			if tt.want == "error" {
				if err == nil {
					t.Error("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := level.String(); got != tt.want {
				t.Errorf("ParseLevel(%q) = %s, want %s", tt.input, got, tt.want)
			}
		})
	}
}

func TestSetup_CreatesDirAndWrites(t *testing.T) {
	tmpDir := t.TempDir()
	logDir := filepath.Join(tmpDir, "logs")

	err := Setup(Config{
		Level:      "info",
		Path:       logDir,
		MaxSize:    10,
		MaxBackups: 3,
		MaxAge:     28,
		AppName:    "testapp",
	})
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	defer Close()

	// Trigger a write — lumberjack creates the file lazily on first write
	slog.Info("test message from unit test")

	logFile := filepath.Join(logDir, "testapp.log")
	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	if !strings.Contains(string(data), "test message from unit test") {
		t.Errorf("log file content = %q, want to contain test message", string(data))
	}
}

func TestSetup_InvalidLevel(t *testing.T) {
	err := Setup(Config{
		Level:   "invalid",
		Path:    t.TempDir(),
		AppName: "test",
	})
	if err == nil {
		t.Error("expected error for invalid level")
	}
}

func TestSetup_DefaultsForRotation(t *testing.T) {
	tmpDir := t.TempDir()

	// Pass 0 for all rotation fields — should use defaults
	err := Setup(Config{
		Level:   "debug",
		Path:    filepath.Join(tmpDir, "logs"),
		AppName: "defaults",
	})
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	defer Close()

	// Trigger write to create the file
	slog.Debug("verify defaults")

	logFile := filepath.Join(tmpDir, "logs", "defaults.log")
	if _, err := os.Stat(logFile); os.IsNotExist(err) {
		t.Error("log file not created with default rotation values")
	}
}
