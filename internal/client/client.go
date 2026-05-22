package client

import (
	"LEPG/internal/msg"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"
)

func MainFunc(cfg *ClientConfig) error {
	wg := &sync.WaitGroup{}
	wg.Go(func() {
		// if err := TestWrite(cfg); err != nil {
		// 	slog.Error("TestWrite failed", "err", err)
		// }
	})
	wg.Go(func() {
		if err := UploadLoop(cfg); err != nil {
			slog.Error("UploadLoop failed", "err", err)
		}
	})

	wg.Wait()
	return nil
}

func TestWrite(cfg *ClientConfig) error {
	x := 0

	conn, err := net.Dial("tcp", fmt.Sprintf("%s:%d", cfg.ServerUrl, cfg.Port))
	if err != nil {
		return err
	}

	for {
		fmt.Println(x)
		_, err := fmt.Fprintf(conn, "hello %d", x)
		if err != nil {
			return err
		}
		time.Sleep(time.Nanosecond * 100)
		x += 1
	}

}

func UploadLoop(cfg *ClientConfig) error {
	// 创建消息工厂
	factory := msg.NewMsgFactory()

	// 建立网络连接
	conn, err := net.Dial("tcp", fmt.Sprintf("%s:%d", cfg.ServerUrl, cfg.Port))
	if err != nil {
		return fmt.Errorf("failed to connect to server %s:%d: %w", cfg.ServerUrl, cfg.Port, err)
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
