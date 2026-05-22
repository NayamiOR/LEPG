package server

import (
	"LEPG/internal/msg"
	"fmt"
	"log/slog"
	"net"
)

// ReceiveLoop 接收循环
func ReceiveLoop() error {
	cfg := GetServerConfig()
	if cfg == nil {
		return fmt.Errorf("server config not initialized")
	}
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.Port))
	if err != nil {
		return err
	}
	defer ln.Close()
	slog.Info("server started", "port", cfg.Port)

	for {
		conn, err := ln.Accept()
		if err != nil {
			return err
		}
		message, err := msg.DecodeFrame(conn)
		if err != nil {
			conn.Close()
			return err
		}

		if message.Type != msg.MsgTypeHandshake {

		}

		slog.Info("accept a connection")
		go HandleConnection(conn)
	}
}

func HandleConnection(conn net.Conn) {
	defer conn.Close()

	remoteAddr := conn.RemoteAddr().String()
	slog.Info("handling connection", "remote_addr", remoteAddr)

	messageCount := 0

	// 循环处理连接中的多条消息
	for {
		message, err := msg.DecodeFrame(conn)
		if err != nil {
			// 连接关闭或读取错误
			if messageCount > 0 {
				slog.Info("connection closed",
					"remote_addr", remoteAddr,
					"messages_processed", messageCount,
					"error", err)
			} else {
				slog.Warn("connection failed to receive first message",
					"remote_addr", remoteAddr,
					"error", err)
			}
			return
		}

		messageCount++

		// 成功接收消息，记录日志
		slog.Info("received message",
			"remote_addr", remoteAddr,
			"count", messageCount,
			"type", message.Type,
			"msg_id", message.MsgID,
			"flags", message.Flags,
			"payload_len", message.PayloadLen,
			"timestamp", message.Timestamp,
			"checksum", message.Checksum)

		// 如果有payload，也记录内容
		if len(message.Payload) > 0 {
			slog.Info("message payload",
				"remote_addr", remoteAddr,
				"size", len(message.Payload),
				"data", string(message.Payload))
		}

		// TODO: 这里可以添加业务逻辑处理消息
		// 例如：根据 message.Type 进行不同的处理
	}
}
