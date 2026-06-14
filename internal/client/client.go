package client

import (
	"LEPG/internal/client/cache"
	"LEPG/internal/errors"
	"LEPG/internal/model"
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

	if len(cfg.Devices) > 0 {
		fmt.Print(formatDeviceList(cfg.Devices))
	}
	if cfg.Mqtt != nil && len(cfg.Mqtt.Devices) > 0 {
		fmt.Print(formatMqttDeviceList(cfg.Mqtt.BrokerAddr, cfg.Mqtt.Devices))
	}

	store, err := cache.NewSQLiteStore(ctx, cfg.Paths.DataPath)
	if err != nil {
		return fmt.Errorf("create SQLite store: %w", err)
	}
	defer store.Close()

	ch := make(chan model.Reading, cfg.BufferSize)
	var mainWg sync.WaitGroup
	var producerWg sync.WaitGroup

	// Goroutine: MQTT broker (if configured)
	if cfg.Mqtt != nil {
		producerWg.Add(1)
		mainWg.Go(func() {
			defer producerWg.Done()
			slog.Info("Starting MQTT broker")
			if err := StartMqttBroker(ctx, ch, cfg.Mqtt); err != nil {
				slog.Error("MQTT broker failed", "error", err)
			}
		})
	}

	// Goroutine: Modbus polling (if configured)
	if len(cfg.Devices) > 0 {
		producerWg.Add(1)
		mainWg.Go(func() {
			defer producerWg.Done()
			slog.Info("Starting device polling")
			var pollWg sync.WaitGroup
			for _, device := range cfg.Devices {
				slog.Info("Starting:", "device", device.Name, "type", device.Type)
				pollWg.Go(func() {
					if err := ModbusDevicePolling(ctx, ch, device); err != nil {
						slog.Error("Device polling failed", "device", device.Name, "error", err)
					}
				})
			}
			pollWg.Wait()
		})
	}

	// Close channel when all producers done
	mainWg.Go(func() {
		producerWg.Wait()
		close(ch)
	})

	// Goroutine: channel → SQLite
	mainWg.Go(func() {
		batchSize := 10
		maxInterval := 30 * time.Second
		consumeAndWrite(ctx, ch, store, batchSize, maxInterval)
	})

	// Goroutine: SQLite → upload
	mainWg.Go(func() {
		uploadLoop(ctx, cfg, store)
	})

	mainWg.Wait()
	return nil
}

func consumeAndWrite(ctx context.Context, ch <-chan model.Reading, store cache.Store, batchSize int, maxInterval time.Duration) {
	buffer := make([]*cache.CachedReading, 0, batchSize)
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
			buffer = append(buffer, &cache.CachedReading{Reading: r, Status: cache.UploadNotSent})
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

	factory := msg.NewMsgFactory()

	if err := performHandshake(conn, cfg, factory); err != nil {
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

			if err := uploadReadings(ctx, conn, store, factory, readings, maxPayloadSize); err != nil {
				slog.Error("Upload failed", "error", err)
				return
			}
		}
	}
}

func uploadReadings(ctx context.Context, conn net.Conn, store cache.Store, factory *msg.MsgFactory, readings []*cache.CachedReading, maxPayloadSize int) error {
	if len(readings) == 0 {
		return nil
	}

	batch := make([]model.Reading, len(readings))
	batchIDs := make([]int64, len(readings))
	for i, r := range readings {
		batch[i] = r.Reading
		batchIDs[i] = r.ID
	}

	uploadMsg, err := factory.NewMsg(msg.MsgTypeUpload, &msg.UploadPayload{Readings: batch})
	if err != nil {
		store.UpdateReadingsStatus(ctx, batchIDs, cache.UploadFailed)
		return fmt.Errorf("build upload message: %w", err)
	}

	// Batch exceeds the frame limit.
	if int(uploadMsg.PayloadLen) > maxPayloadSize {
		if len(readings) == 1 {
			// A single reading is too large to ever send — fail and skip it.
			slog.Warn("Single reading exceeds max payload size, skipping", "id", readings[0].ID)
			store.UpdateReadingsStatus(ctx, batchIDs, cache.UploadFailed)
			return nil
		}
		mid := len(readings) / 2
		if err := uploadReadings(ctx, conn, store, factory, readings[:mid], maxPayloadSize); err != nil {
			return err
		}
		return uploadReadings(ctx, conn, store, factory, readings[mid:], maxPayloadSize)
	}

	if err := store.UpdateReadingsStatus(ctx, batchIDs, cache.UploadSending); err != nil {
		return fmt.Errorf("mark readings as uploading: %w", err)
	}

	encoded, err := uploadMsg.Encode()
	if err != nil {
		store.UpdateReadingsStatus(ctx, batchIDs, cache.UploadFailed)
		return fmt.Errorf("encode upload message: %w", err)
	}

	if _, err := conn.Write(encoded); err != nil {
		store.UpdateReadingsStatus(ctx, batchIDs, cache.UploadFailed)
		return fmt.Errorf("write upload message: %w", err)
	}

	if err := store.UpdateReadingsStatus(ctx, batchIDs, cache.UploadSent); err != nil {
		slog.Error("Failed to mark readings as uploaded", "error", err)
	}

	slog.Info("Uploaded batch", "count", len(batchIDs), "payload_size", uploadMsg.PayloadLen)
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

func performHandshake(conn net.Conn, cfg *ClientConfig, factory *msg.MsgFactory) error {
	hsMsg, err := factory.NewMsg(msg.MsgTypeHandshake, &msg.HandshakePayload{
		FirmwareVersion: 1,
		Sn:              cfg.Sn,
		Token:           cfg.Token,
	})
	if err != nil {
		return fmt.Errorf("build handshake: %w", err)
	}
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

	if respMsg.Type != msg.MsgTypeHandshakeAck {
		return fmt.Errorf("unexpected response type: 0x%02x", respMsg.Type)
	}

	parsed, err := msg.ParseMsg(&respMsg)
	if err != nil {
		return fmt.Errorf("decode handshake response: %w", err)
	}
	ack, ok := parsed.(*msg.AckPayload)
	if !ok {
		return fmt.Errorf("unexpected handshake response payload: %T", parsed)
	}

	if ack.Code != msg.Ok {
		return fmt.Errorf("handshake rejected (code=%d): %w", ack.Code, errors.ErrHandshakeRejected)
	}

	return nil
}
