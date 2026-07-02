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

// Heartbeat timing, fixed at compile time (not user-configurable).
const (
	heartbeatInterval = 30 * time.Second
	heartbeatTimeout  = 90 * time.Second
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
	retryInterval := time.Duration(cfg.RetryInterval) * time.Millisecond

	for { // reconnect loop
		err := runSession(ctx, cfg, store)
		if ctx.Err() != nil {
			slog.Info("Upload loop stopped")
			return
		}
		slog.Warn("session ended, reconnecting", "error", err)
		select {
		case <-ctx.Done():
			return
		case <-time.After(retryInterval):
		}
	}
}

// runSession establishes a single connection session: dial → handshake → run the
// read/write loop. It returns an error (non-nil unless ctx is canceled) so the
// caller can reconnect. Writes (upload + heartbeat) happen in this goroutine;
// reads (server messages + timeout detection) happen in a dedicated readLoop.
func runSession(ctx context.Context, cfg *ClientConfig, store cache.Store) error {
	conn, err := dialWithRetry(ctx, cfg)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer conn.Close()

	slog.Info("connected to server", "address", fmt.Sprintf("%s:%d", cfg.ServerUrl, cfg.Port))

	factory := msg.NewMsgFactory()

	if err := performHandshake(conn, cfg, factory); err != nil {
		return fmt.Errorf("handshake: %w", err)
	}

	slog.Info("handshake successful")

	// Read goroutine: drains server messages (HeartbeatAck) and enforces the
	// read deadline. Closing connDead signals the writer loop to reconnect.
	connDead := make(chan struct{})
	go readLoop(conn, connDead)

	hbTicker := time.NewTicker(heartbeatInterval)
	defer hbTicker.Stop()

	pollInterval := time.Duration(cfg.UploadInterval) * time.Millisecond
	uploadTimer := time.NewTimer(0) // trigger immediately on first iteration
	defer uploadTimer.Stop()

	const maxPayloadSize = 65535

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-connDead:
			return fmt.Errorf("connection lost (read timeout or closed)")
		case <-hbTicker.C:
			if err := sendHeartbeat(conn, factory); err != nil {
				return fmt.Errorf("heartbeat: %w", err)
			}
		case <-uploadTimer.C:
			uploadTimer.Reset(pollInterval)

			readings, err := store.LoadPendingReadings(ctx, cfg.UploadBatchSize)
			if err != nil {
				slog.Error("Failed to load pending readings", "error", err)
				continue
			}
			if len(readings) == 0 {
				continue
			}

			if err := uploadReadings(ctx, conn, store, factory, readings, maxPayloadSize); err != nil {
				return fmt.Errorf("upload: %w", err)
			}
		}
	}
}

// readLoop continuously reads frames from the server. Each successful read
// implicitly refreshes the read deadline (set before every DecodeFrame call);
// as long as the server keeps responding (e.g. HeartbeatAck), no timeout fires.
// On timeout or any read error it closes dead so the writer loop reconnects.
func readLoop(conn net.Conn, dead chan<- struct{}) {
	defer close(dead)
	readTimeout := heartbeatTimeout
	for {
		if err := conn.SetReadDeadline(time.Now().Add(readTimeout)); err != nil {
			return
		}
		m, err := msg.DecodeFrame(conn)
		if err != nil {
			return
		}
		slog.Debug("received message from server", "type", m.Type, "msg_id", m.MsgID)
	}
}

func sendHeartbeat(conn net.Conn, factory *msg.MsgFactory) error {
	hbMsg, err := factory.NewMsg(msg.MsgTypeHeartbeat, &msg.HeartbeatPayload{})
	if err != nil {
		return fmt.Errorf("build: %w", err)
	}
	encoded, err := hbMsg.Encode()
	if err != nil {
		return fmt.Errorf("encode: %w", err)
	}
	if _, err := conn.Write(encoded); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	return nil
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
