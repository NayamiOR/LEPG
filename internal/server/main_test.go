package server

import (
	"io"
	"log/slog"
	"os"
	"testing"
)

// TestMain silences the default slog logger for the entire server package
// so that log output from production code never contaminates `go test` or
// `benchstat` results. Set LEPG_TEST_LOG=1 to restore logging for debugging.
func TestMain(m *testing.M) {
	var w io.Writer = io.Discard
	if os.Getenv("LEPG_TEST_LOG") != "" {
		w = os.Stderr
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(w, nil)))
	os.Exit(m.Run())
}
