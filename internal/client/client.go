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

	// Upload sliding window
	uploadWindowSize         = 3
	uploadAckTimeout         = 3 * time.Second
	maxConsecutiveAllTimeout = 3
)

// msgRouter routes server messages by type to registered channels.
type msgRouter struct {
	mu     sync.Mutex
	routes map[uint8]chan<- interface{}
}

// --- Modbus runtime registry (package-level, for external Write access) ---

var (
	modbusRuntimes   = make(map[string]*ModbusRuntime)
	modbusRuntimesMu sync.RWMutex
)

// registerModbusRuntime adds a runtime to the registry.
func registerModbusRuntime(name string, rt *ModbusRuntime) {
	modbusRuntimesMu.Lock()
	modbusRuntimes[name] = rt
	modbusRuntimesMu.Unlock()
}

// unregisterModbusRuntime removes a runtime from the registry.
func unregisterModbusRuntime(name string) {
	modbusRuntimesMu.Lock()
	delete(modbusRuntimes, name)
	modbusRuntimesMu.Unlock()
}

// GetModbusRuntime returns the runtime for a device by name.
func GetModbusRuntime(name string) (*ModbusRuntime, bool) {
	modbusRuntimesMu.RLock()
	rt, ok := modbusRuntimes[name]
	modbusRuntimesMu.RUnlock()
	return rt, ok
}

func newMsgRouter() *msgRouter {
	return &msgRouter{routes: make(map[uint8]chan<- interface{})}
}

func (r *msgRouter) register(msgType uint8, ch chan<- interface{}) {
	r.mu.Lock()
	r.routes[msgType] = ch
	r.mu.Unlock()
}

func (r *msgRouter) unregister(msgType uint8) {
	r.mu.Lock()
	delete(r.routes, msgType)
	r.mu.Unlock()
}

func (r *msgRouter) dispatch(msgType uint8, payload interface{}) {
	r.mu.Lock()
	ch, ok := r.routes[msgType]
	r.mu.Unlock()
	if ok {
		ch <- payload
	}
}

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
	notifyCh := make(chan msg.NotifyPayload, 32)
	var mainWg sync.WaitGroup
	var producerWg sync.WaitGroup

	// Goroutine: MQTT broker (if configured)
	if cfg.Mqtt != nil {
		producerWg.Add(1)
		mainWg.Go(func() {
			defer producerWg.Done()
			slog.Info("Starting MQTT broker")
			if err := StartMqttBroker(ctx, ch, notifyCh, cfg.Mqtt); err != nil {
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
		sessionLoop(ctx, cfg, store, notifyCh)
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

func sessionLoop(ctx context.Context, cfg *ClientConfig, store cache.Store, notifyCh <-chan msg.NotifyPayload) {
	slog.Info("Upload loop started")
	retryInterval := time.Duration(cfg.RetryInterval) * time.Millisecond

	for { // reconnect loop
		err := runSession(ctx, cfg, store, notifyCh)
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
func runSession(ctx context.Context, cfg *ClientConfig, store cache.Store, notifyCh <-chan msg.NotifyPayload) error {
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

	router := newMsgRouter()
	uploadAckCh := make(chan interface{}, uploadWindowSize)
	router.register(msg.MsgTypeUploadAck, uploadAckCh)

	connDead := make(chan struct{})
	go readLoop(conn, connDead, router)

	hbTicker := time.NewTicker(heartbeatInterval)
	defer hbTicker.Stop()

	pollInterval := time.Duration(cfg.UploadInterval) * time.Millisecond
	uploadTimer := time.NewTimer(0) // trigger immediately on first iteration
	defer uploadTimer.Stop()

	const maxPayloadSize = 65535

	var consecutiveAllTimeout int

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-connDead:
			return fmt.Errorf("connection lost (read timeout or closed)")
		case n := <-notifyCh:
			if err := sendNotify(conn, factory, &n); err != nil {
				return fmt.Errorf("notify: %w", err)
			}
		case <-hbTicker.C:
			if err := sendHeartbeat(conn, factory, router); err != nil {
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

			if err := uploadReadings(ctx, conn, store, factory, readings, maxPayloadSize, uploadAckCh, &consecutiveAllTimeout); err != nil {
				return fmt.Errorf("upload: %w", err)
			}
			if consecutiveAllTimeout >= maxConsecutiveAllTimeout {
				return fmt.Errorf("too many consecutive upload ack timeouts (%d)", consecutiveAllTimeout)
			}
		}
	}
}

// readLoop continuously reads frames from the server. Each successful read
// implicitly refreshes the read deadline (set before every DecodeFrame call);
// as long as the server keeps responding (e.g. HeartbeatAck), no timeout fires.
// On timeout or any read error it closes dead so the writer loop reconnects.
func readLoop(conn net.Conn, dead chan<- struct{}, router *msgRouter) {
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
		parsed, err := msg.ParseMsg(&m)
		if err != nil {
			slog.Warn("failed to parse server message", "type", m.Type, "error", err)
			continue
		}
		router.dispatch(m.Type, parsed)
	}
}

func sendHeartbeat(conn net.Conn, factory *msg.MsgFactory, router *msgRouter) error {
	ch := make(chan interface{}, 1)
	router.register(msg.MsgTypeHeartbeatAck, ch)
	defer router.unregister(msg.MsgTypeHeartbeatAck)

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

	select {
	case <-ch:
		return nil
	case <-time.After(5 * time.Second):
		return fmt.Errorf("heartbeat ack timeout")
	}
}

func uploadReadings(ctx context.Context, conn net.Conn, store cache.Store, factory *msg.MsgFactory, readings []*cache.CachedReading, maxPayloadSize int, ackCh chan interface{}, consecutiveAllTimeout *int) error {
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

	if int(uploadMsg.PayloadLen) > maxPayloadSize {
		if len(readings) == 1 {
			slog.Warn("Single reading exceeds max payload size, skipping", "id", readings[0].ID)
			store.UpdateReadingsStatus(ctx, batchIDs, cache.UploadFailed)
			return nil
		}
		mid := len(readings) / 2
		if err := uploadReadings(ctx, conn, store, factory, readings[:mid], maxPayloadSize, ackCh, consecutiveAllTimeout); err != nil {
			return err
		}
		return uploadReadings(ctx, conn, store, factory, readings[mid:], maxPayloadSize, ackCh, consecutiveAllTimeout)
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

	select {
	case ack := <-ackCh:
		ackPayload := ack.(*msg.AckPayload)
		if ackPayload.Code == msg.Ok {
			if err := store.UpdateReadingsStatus(ctx, batchIDs, cache.UploadSent); err != nil {
				slog.Error("Failed to mark readings as uploaded", "error", err)
			}
			*consecutiveAllTimeout = 0
			slog.Info("Uploaded batch", "count", len(batchIDs), "payload_size", uploadMsg.PayloadLen)
		} else {
			store.UpdateReadingsStatus(ctx, batchIDs, cache.UploadNotSent)
			slog.Warn("Upload rejected by server", "code", ackPayload.Code)
		}
	case <-time.After(uploadAckTimeout):
		store.UpdateReadingsStatus(ctx, batchIDs, cache.UploadNotSent)
		*consecutiveAllTimeout++
		slog.Warn("Upload ack timeout", "count", len(batchIDs))
	case <-ctx.Done():
		return ctx.Err()
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

func sendNotify(conn net.Conn, factory *msg.MsgFactory, payload *msg.NotifyPayload) error {
	m, err := factory.NewMsg(msg.MsgTypeNotify, payload)
	if err != nil {
		return err
	}
	encoded, err := m.Encode()
	if err != nil {
		return err
	}
	_, err = conn.Write(encoded)
	return err
}
