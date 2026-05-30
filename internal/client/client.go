package client

import (
	"LEPG/internal/client/cache"
	"LEPG/internal/errors"
	"LEPG/internal/msg"
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"
)

func MainFunc(ctx context.Context, cfg *ClientConfig) error {
	slog.Info("Client configuration", "config", cfg)

	if len(cfg.Devices) == 0 {
		slog.Warn("No devices configured, client will run without Modbus polling")
		return nil
	}

	store, err := cache.NewSQLiteStore(ctx, cfg.Paths.DataPath)
	if err != nil {
		return fmt.Errorf("create SQLite store: %w", err)
	}
	defer store.Close()

	ch := make(chan cache.Reading, cfg.BufferSize)
	var mainWg sync.WaitGroup

	// Goroutine 1: 轮询 → channel
	mainWg.Go(func() {
		slog.Info("Starting device polling")
		var pollWg sync.WaitGroup
		for _, device := range cfg.Devices {
			slog.Info("Starting:", "device", device.Name, "type", device.Type)
			pollWg.Go(func() {
				if err := TcpDevicePolling(ch, device); err != nil {
					slog.Error("Device polling failed", "device", device.Name, "error", err)
				}
			})
		}
		pollWg.Wait()
		close(ch)
	})

	// Goroutine 2: channel → SQLite
	mainWg.Go(func() {
		batchSize := 10
		maxInterval := 30 * time.Second
		consumeAndWrite(ctx, ch, store, batchSize, maxInterval)
	})

	// Goroutine 3: SQLite → 上传（占位）
	mainWg.Go(func() {
		uploadLoop(ctx, cfg, store)
	})

	mainWg.Wait()
	return nil
}

func consumeAndWrite(ctx context.Context, ch <-chan cache.Reading, store cache.Store, batchSize int, maxInterval time.Duration) {
	buffer := make([]*cache.Reading, 0, batchSize)
	ticker := time.NewTicker(maxInterval)
	defer ticker.Stop()

	flush := func() {
		if len(buffer) == 0 {
			return
		}
		if err := store.SaveReadings(ctx, buffer); err != nil {
			slog.Error("Failed to save readings", "error", err)
			return
		}
		slog.Info("Saved readings")
		buffer = buffer[:0]
	}

	for {
		select {
		case r, ok := <-ch:
			if !ok {
				flush()
				return
			}
			buffer = append(buffer, &r)
			if len(buffer) >= batchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-ctx.Done():
			flush()
			return
		}
	}
}

func uploadLoop(ctx context.Context, cfg *ClientConfig, store cache.Store) {
	slog.Info("Upload loop started")

	conn, err := dialWithRetry(ctx, cfg)
	if err != nil {
		slog.Error("failed to connect after all retries", "error", err)
		return
	}
	defer conn.Close()

	slog.Info("connected to server", "address", fmt.Sprintf("%s:%d", cfg.ServerUrl, cfg.Port))

	if err := performHandshake(conn, cfg); err != nil {
		slog.Error("handshake failed", "error", err)
		return
	}

	slog.Info("handshake successful")

	// TODO: 从 SQLite store 读取数据上传
	<-ctx.Done()
	slog.Info("Upload loop stopped")
}

func dialWithRetry(ctx context.Context, cfg *ClientConfig) (net.Conn, error) {
	addr := net.JoinHostPort(cfg.ServerUrl, fmt.Sprintf("%d", cfg.Port))
	baseInterval := time.Duration(cfg.RetryInterval) * time.Millisecond

	for attempt := 0; attempt < cfg.MaxRetry; attempt++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
		if err == nil {
			return conn, nil
		}

		slog.Warn("connection failed, retrying",
			"attempt", attempt+1,
			"max_retries", cfg.MaxRetry,
			"error", err)

		backoff := baseInterval * time.Duration(1<<uint(attempt))
		if backoff > 60*time.Second {
			backoff = 60 * time.Second
		}

		timer := time.NewTimer(backoff)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		}
	}

	return nil, fmt.Errorf("failed to connect to %s after %d attempts", addr, cfg.MaxRetry)
}

func performHandshake(conn net.Conn, cfg *ClientConfig) error {
	hsPayload, _ := (&msg.HandshakePayload{
		Version: 1,
		Sn:      cfg.Sn,
		Token:   cfg.Token,
	}).Encode()

	hsMsg := msg.New(msg.MsgTypeHandshake, hsPayload)
	encoded, err := hsMsg.Encode()
	if err != nil {
		return fmt.Errorf("encode handshake: %w", err)
	}

	if _, err := conn.Write(encoded); err != nil {
		return fmt.Errorf("send handshake: %w", err)
	}

	respMsg, err := msg.DecodeFrame(conn)
	if err != nil {
		return fmt.Errorf("read handshake response: %w", err)
	}

	if respMsg.Type != msg.MsgTypeHandshake {
		return fmt.Errorf("unexpected response type: 0x%02x", respMsg.Type)
	}

	resp, err := msg.DecodeHandshakeResponsePayload(respMsg.Payload)
	if err != nil {
		return fmt.Errorf("decode handshake response: %w", err)
	}

	if resp.Code != msg.HandshakeOK {
		return fmt.Errorf("handshake rejected (code=%d): %s: %w",
			resp.Code, resp.Message, errors.ErrHandshakeRejected)
	}

	return nil
}
