package output

import (
	"LEPG/internal/model"
	"bytes"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// ── ThingsBoard HTTP Sinker ───────────────────────────────────

// tbHttpSinker pushes ThingsBoard Gateway telemetry via HTTP POST.
// Uses a pooled http.Client for connection reuse.
type tbHttpSinker struct {
	cfg       OutputConfig
	formatter Formatter
	client    *http.Client
	url       string
}

// NewThingsBoardHttpSinker creates a ThingsBoard HTTP sinker.
func NewThingsBoardHttpSinker(cfg OutputConfig) (*tbHttpSinker, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	timeout := time.Duration(cfg.Timeout) * time.Second
	scheme := cfg.Scheme
	if scheme == "" {
		scheme = "https"
	}
	url := fmt.Sprintf("%s://%s:%d/api/v1/%s/telemetry", scheme, cfg.Host, cfg.Port, cfg.Token)

	return &tbHttpSinker{
		cfg:       cfg,
		formatter: NewThingsBoardFormatter(),
		client: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				MaxIdleConns:    10,
				IdleConnTimeout: 90 * time.Second,
			},
		},
		url: url,
	}, nil
}

func (s *tbHttpSinker) Name() string { return s.cfg.Name }

func (s *tbHttpSinker) Send(deviceKey string, readings []model.Reading) error {
	payload, err := s.formatter.Format(deviceKey, readings)
	if err != nil {
		return fmt.Errorf("tb format: %w", err)
	}

	var lastErr error
	backoff := 1 * time.Second

	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(backoff)
			backoff *= 2
		}

		resp, err := s.client.Post(s.url, "application/json", bytes.NewReader(payload))
		if err != nil {
			lastErr = err
			slog.Warn("tb http post failed",
				"sink", s.cfg.Name,
				"attempt", attempt+1,
				"error", err,
			)
			continue
		}
		resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			slog.Debug("tb http published",
				"sink", s.cfg.Name,
				"device_key", deviceKey,
				"count", len(readings),
			)
			return nil
		}

		lastErr = fmt.Errorf("HTTP %d", resp.StatusCode)
		slog.Warn("tb http non-2xx",
			"sink", s.cfg.Name,
			"attempt", attempt+1,
			"status", resp.StatusCode,
		)
	}

	return fmt.Errorf("tb http send after 3 retries: %w", lastErr)
}

func (s *tbHttpSinker) HealthCheck() error { return nil }

func (s *tbHttpSinker) Close() error {
	s.client.CloseIdleConnections()
	return nil
}
