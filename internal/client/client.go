package client

import (
	"LEPG/internal/client/cache"
	"LEPG/internal/msg"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"
)

func MainFunc(cfg *ClientConfig, ctx context.Context) error {
	slog.Info("Client configuration", "config", cfg)

	// handshakeCh := make(chan HandshakeResult, 1)
	// go func() {
	// 	conn, err := net.Dial("tcp", net.JoinHostPort(cfg.ServerUrl, fmt.Sprintf("%d", cfg.Port)))
	// 	if err != nil {
	// 		handshakeCh <- HandshakeResult{Err: fmt.Errorf("connect failed: %w", err)}
	// 		return
	// 	}
	// 	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	// 	defer cancel()
	// 	select {
	// 	case result := <-doHandshake(conn):
	// 		handshakeCh <- result
	// 	case <-ctx.Done():
	// 		conn.Close()
	// 		handshakeCh <- HandshakeResult{Err: fmt.Errorf("handshake timeout")}
	// 	}
	// }()

	if len(cfg.Devices) == 0 {
		slog.Warn("No devices configured, client will run without Modbus polling")
		return nil
	}

	// 所有设备共享一个 channel
	ch := make(chan cache.Reading, cfg.BufferSize)

	// Fan-out: 启动所有设备轮询
	var wg sync.WaitGroup
	for _, device := range cfg.Devices {
		slog.Info("Starting device polling", "device", device.Name, "type", device.Type)
		wg.Go(func() {
			if err := TcpDevicePolling(ch, device); err != nil {
				slog.Error("Device polling failed", "device", device.Name, "error", err)
			}
		})
	}

	// 所有轮询结束后关闭 channel，通知消费者退出
	go func() {
		wg.Wait()
		close(ch)
	}()

	// Fan-in: 消费者主 goroutine 批量写入 SQLite
	store, err := cache.NewSQLiteStore(cfg.Paths.DataPath)
	if err != nil {
		return fmt.Errorf("create SQLite store: %w", err)
	}
	defer store.Close()

	buffer := make([]*cache.Reading, 0, cfg.BufferSize)
	batchSize := 10
	maxInterval := 30 * time.Second
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
		slog.Info("Saved readings", "count", len(buffer))
		buffer = buffer[:0]
	}

	for {
		select {
		case r, ok := <-ch:
			if !ok {
				flush()
				return nil
			}
			buffer = append(buffer, &r)
			if len(buffer) >= batchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-ctx.Done():
			flush()
			return ctx.Err()
		}
	}
}

// 测试和后端的连接以及装包解包
func UploadLoopExample(cfg *ClientConfig) error {
	// 创建消息工厂
	factory := msg.NewMsgFactory()

	// 建立网络连接

	var conn net.Conn
	var err error
	fmt.Println("Trying to connect to server with max retry:", cfg.MaxRetry)
	for retryCount := 0; retryCount < cfg.MaxRetry+1; retryCount++ {
		if conn != nil {
			break
		}
		conn, err = net.Dial("tcp", net.JoinHostPort(cfg.ServerUrl, fmt.Sprintf("%d", cfg.Port)))

		if err != nil {
			if _, ok := errors.AsType[*net.OpError](err); ok {
				if retryCount == cfg.MaxRetry {
					return fmt.Errorf("failed to connect to server %s:%d after %d retries: %w", cfg.ServerUrl, cfg.Port, retryCount, err)
				}
				slog.Warn("failed to connect to server, retrying...", "error", err)
				time.Sleep(time.Millisecond * time.Duration(cfg.RetryInterval))
			} else {
				return fmt.Errorf("failed to connect to server %s:%d: Unknown error: %w", cfg.ServerUrl, cfg.Port, err)
			}
		}
	}
	defer conn.Close()

	slog.Info("connected to server", "address", fmt.Sprintf("%s:%d", cfg.ServerUrl, cfg.Port))

	// mock 数据序列
	mockData := []string{
		"hello world",
		"test message 1",
		"test message 2",
		"data upload test",
		"heartbeat check",
		"system status",
	}

	messageCount := 0
	for {
		// 获取 mock 数据
		data := mockData[messageCount%len(mockData)]

		// 创建消息
		message := factory.NewMsg(0, msg.MsgTypeUpload, []byte(data))

		// 编码消息
		encodedData, err := message.Encode()
		if err != nil {
			slog.Error("failed to encode message", "error", err)
			continue
		}

		// 发送消息
		_, err = conn.Write(encodedData)
		if err != nil {
			return fmt.Errorf("failed to send message: %w", err)
		}

		messageCount++

		slog.Info("sent message",
			"count", messageCount,
			"type", message.Type,
			"msg_id", message.MsgID,
			"payload_size", len(message.Payload),
			"data", data)

		// 等待一段时间再发送下一条消息
		time.Sleep(time.Millisecond * 500)
	}
}
