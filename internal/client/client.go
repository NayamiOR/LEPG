package client

import (
	"LEPG/internal/client/cache"
	"LEPG/internal/errors"
	"LEPG/internal/msg"
	"bytes"
	"context"
	"encoding/gob"
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

	const maxPayloadSize = 65535
	pollInterval := time.Duration(cfg.UploadInterval) * time.Millisecond
	timer := time.NewTimer(0) // trigger immediately on first iteration
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("Upload loop stopped")
			return
		case <-timer.C:
			timer.Reset(pollInterval)

			readings, err := store.LoadPendingReadings(ctx, cfg.UploadBatchSize)
			if err != nil {
				slog.Error("Failed to load pending readings", "error", err)
				continue
			}
			if len(readings) == 0 {
				continue
			}

			if err := uploadReadings(ctx, conn, store, readings, maxPayloadSize); err != nil {
				slog.Error("Upload failed", "error", err)
				return
			}
		}
	}
}

func uploadReadings(ctx context.Context, conn net.Conn, store cache.Store, readings []*cache.Reading, maxPayloadSize int) error {
	i := 0
	for i < len(readings) {
		var payloadBuf bytes.Buffer
		enc := gob.NewEncoder(&payloadBuf)
		var batchIDs []int64

		for i < len(readings) {
			prevLen := payloadBuf.Len()
			if err := enc.Encode(readings[i]); err != nil {
				return fmt.Errorf("gob encode reading %d: %w", readings[i].ID, err)
			}
			if payloadBuf.Len() > maxPayloadSize {
				if prevLen == 0 {
					slog.Warn("Single reading exceeds max payload size, skipping", "id", readings[i].ID)
					store.UpdateReadingsStatus(ctx, []int64{readings[i].ID}, 3)
					i++
					payloadBuf.Reset()
					continue
				}
				payloadBuf.Truncate(prevLen)
				break
			}
			batchIDs = append(batchIDs, readings[i].ID)
			i++
		}

		if payloadBuf.Len() == 0 {
			continue
		}

		if err := store.UpdateReadingsStatus(ctx, batchIDs, 1); err != nil {
			return fmt.Errorf("mark readings as uploading: %w", err)
		}

		uploadMsg := msg.New(msg.MsgTypeUpload, payloadBuf.Bytes())
		encoded, err := uploadMsg.Encode()
		if err != nil {
			store.UpdateReadingsStatus(ctx, batchIDs, 3)
			return fmt.Errorf("encode upload message: %w", err)
		}

		if _, err := conn.Write(encoded); err != nil {
			store.UpdateReadingsStatus(ctx, batchIDs, 3)
			return fmt.Errorf("write upload message: %w", err)
		}

		if err := store.UpdateReadingsStatus(ctx, batchIDs, 2); err != nil {
			slog.Error("Failed to mark readings as uploaded", "error", err)
		}

		slog.Info("Uploaded batch", "count", len(batchIDs), "payload_size", payloadBuf.Len())
	}
	return nil
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
