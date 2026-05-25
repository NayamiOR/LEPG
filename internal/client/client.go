package client

import (
	"LEPG/internal/msg"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"
)

func MainFunc(cfg *ClientConfig) error {
	wg := &sync.WaitGroup{}

	// 启动所有配置的 Modbus 设备轮询
	if len(cfg.Devices) == 0 {
		slog.Warn("No devices configured, client will run without Modbus polling")
	} else {
		for _, device := range cfg.Devices {
			slog.Info("Starting device polling", "device", device.Name, "type", device.Type)

			// 为每个设备启动独立的 goroutine
			device := device // 创建局部变量避免闭包问题
			wg.Go(func() {
				if err := TcpDevicePolling(device); err != nil {
					slog.Error("Device polling failed", "device", device.Name, "error", err)
				}
			})
		}
	}

	wg.Wait()
	return nil
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
		conn, err = net.Dial("tcp", fmt.Sprintf("%s:%d", cfg.ServerUrl, cfg.Port))

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
