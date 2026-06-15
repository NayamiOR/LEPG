package server

import (
	"LEPG/internal/model"
	"LEPG/internal/msg"
	"LEPG/internal/server/cache"
	"context"
	"fmt"
	"log/slog"
	"net"
	"time"
)

// heartbeatTimeout is the server-side read deadline for detecting silent
// clients. Fixed at compile time (not user-configurable).
const heartbeatTimeout = 90 * time.Second

// ReceiveLoop 接收循环
func ReceiveLoop(cfg *ServerConfig, s cache.Store, publisher EventPublisher) error {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.Port))
	if err != nil {
		return err
	}
	defer ln.Close()
	slog.Info("server started", "port", cfg.Port)

	for {
		conn, err := ln.Accept()
		if err != nil {
			slog.Error("accept connection failed", "error", err)
			continue
		}

		slog.Info("accept a connection", "remote_addr", conn.RemoteAddr().String())
		go HandleConnection(conn, cfg, s, publisher)
	}
}

func HandleConnection(conn net.Conn, cfg *ServerConfig, s cache.Store, publisher EventPublisher) {
	defer conn.Close()

	remoteAddr := conn.RemoteAddr().String()
	slog.Info("handling connection", "remote_addr", remoteAddr)

	factory := msg.NewMsgFactory()

	// 第一步：等待握手消息
	hsMsg, err := msg.DecodeFrame(conn)
	if err != nil {
		slog.Warn("failed to read handshake", "remote_addr", remoteAddr, "error", err)
		return
	}

	if hsMsg.Type != msg.MsgTypeHandshake {
		slog.Warn("expected handshake message", "remote_addr", remoteAddr, "got_type", hsMsg.Type)
		sendHandshakeResponse(conn, factory, hsMsg.MsgID, msg.Failed)
		return
	}

	parsed, err := msg.ParseMsg(&hsMsg)
	if err != nil {
		slog.Warn("invalid handshake payload", "remote_addr", remoteAddr, "error", err)
		sendHandshakeResponse(conn, factory, hsMsg.MsgID, msg.Failed)
		return
	}
	hsPayload, ok := parsed.(*msg.HandshakePayload)
	if !ok {
		slog.Warn("invalid handshake payload", "remote_addr", remoteAddr, "payload", parsed)
		sendHandshakeResponse(conn, factory, hsMsg.MsgID, msg.Failed)
		return
	}

	// 校验 Sn + Token
	var matched *ClientDef
	for i := range cfg.Clients {
		if cfg.Clients[i].Sn == hsPayload.Sn {
			matched = &cfg.Clients[i]
			break
		}
	}

	if matched == nil {
		slog.Warn("unknown client SN", "remote_addr", remoteAddr, "sn", hsPayload.Sn)
		sendHandshakeResponse(conn, factory, hsMsg.MsgID, msg.BadSn)
		return
	}
	if matched.Token != hsPayload.Token {
		slog.Warn("token mismatch", "remote_addr", remoteAddr, "sn", hsPayload.Sn)
		sendHandshakeResponse(conn, factory, hsMsg.MsgID, msg.BadToken)
		return
	}

	// 鉴权成功
	slog.Info("client authenticated", "remote_addr", remoteAddr, "sn", hsPayload.Sn)
	sendHandshakeResponse(conn, factory, hsMsg.MsgID, msg.Ok)

	// 握手成功后开启心跳读超时；每收到一条消息即刷新
	readTimeout := heartbeatTimeout
	conn.SetReadDeadline(time.Now().Add(readTimeout))

	// 进入正常消息处理循环
	messageCount := 1
	for {
		message, err := msg.DecodeFrame(conn)
		if err != nil {
			if messageCount > 1 {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					slog.Warn("heartbeat timeout, closing connection",
						"remote_addr", remoteAddr,
						"sn", hsPayload.Sn)
				} else {
					slog.Info("connection closed",
						"remote_addr", remoteAddr,
						"sn", hsPayload.Sn,
						"messages_processed", messageCount,
						"error", err)
				}
			}
			return
		}
		conn.SetReadDeadline(time.Now().Add(readTimeout))

		messageCount++
		slog.Info("received message",
			"remote_addr", remoteAddr,
			"sn", hsPayload.Sn,
			"count", messageCount,
			"type", message.Type,
			"msg_id", message.MsgID,
			"payload_len", message.PayloadLen)

		switch message.Type {
		case msg.MsgTypeUpload:
			uploadParsed, err := msg.ParseMsg(&message)
			if err != nil {
				slog.Warn("invalid upload payload", "remote_addr", remoteAddr, "sn", hsPayload.Sn, "error", err)
				continue
			}
			upload, ok := uploadParsed.(*msg.UploadPayload)
			if !ok {
				slog.Warn("invalid upload payload", "remote_addr", remoteAddr, "sn", hsPayload.Sn, "payload", uploadParsed)
				continue
			}

			readings := make([]*model.Reading, len(upload.Readings))
			for i := range upload.Readings {
				readings[i] = &upload.Readings[i]
			}
			if len(readings) > 0 {
				if err := s.SaveReadings(context.Background(), hsPayload.Sn, readings); err != nil {
					slog.Error("failed to save readings",
						"remote_addr", remoteAddr,
						"sn", hsPayload.Sn,
						"count", len(readings),
						"error", err)
				} else {
					slog.Info("saved readings",
						"remote_addr", remoteAddr,
						"sn", hsPayload.Sn,
						"count", len(readings))
					// TODO: 后续实现 payload 序列化（JSON/MessagePack），框架阶段仅预留调用点
					// payload := serializeReadings(readings)
					// if err := publisher.PublishDeviceReadings(hsPayload.Sn, payload); err != nil {
					//     slog.Error("mqtt publish readings failed", "sn", hsPayload.Sn, "error", err)
					// }
					_ = publisher
				}
			}
		case msg.MsgTypeHeartbeat:
			sendAck(conn, factory, msg.MsgTypeHeartbeatAck, message.MsgID, msg.Ok)
		default:
			slog.Warn("unexpected message type",
				"remote_addr", remoteAddr, "sn", hsPayload.Sn, "type", message.Type)
		}
	}
}

func sendHandshakeResponse(conn net.Conn, factory *msg.MsgFactory, reqMsgID uint16, code uint8) {
	sendAck(conn, factory, msg.MsgTypeHandshakeAck, reqMsgID, code)
}

// sendAck encodes and writes an Ack frame of the given type (handshake ack /
// heartbeat ack). Shared by sendHandshakeResponse and the heartbeat handler.
func sendAck(conn net.Conn, factory *msg.MsgFactory, ackType uint8, reqMsgID uint16, code uint8) {
	resp, err := factory.NewMsg(ackType, &msg.AckPayload{MsgID: reqMsgID, Code: code})
	if err != nil {
		slog.Error("failed to build ack", "type", ackType, "error", err)
		return
	}
	encoded, err := resp.Encode()
	if err != nil {
		slog.Error("failed to encode ack", "type", ackType, "error", err)
		return
	}
	if _, err := conn.Write(encoded); err != nil {
		slog.Error("failed to send ack", "type", ackType, "error", err)
	}
}
