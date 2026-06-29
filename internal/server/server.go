package server

import (
	"LEPG/internal/model"
	"LEPG/internal/msg"
	"LEPG/internal/server/cache"
	"LEPG/internal/server/cache/connections"
	"LEPG/internal/utils"
	"context"
	"fmt"
	"log/slog"
	"net"
	"time"

	"github.com/google/uuid"
)

// heartbeatTimeout is the server-side read deadline for detecting silent
// clients. Fixed at compile time (not user-configurable).
const heartbeatTimeout = 90 * time.Second

// ReceiveLoop 接收循环
func ReceiveLoop(cfg *ServerConfig, s cache.Store, publisher EventPublisher, connMgr connections.ConnectionManager) error {
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
		go HandleConnection(conn, cfg, s, publisher, connMgr)
	}
}

func HandleConnection(conn net.Conn, cfg *ServerConfig, s cache.Store, publisher EventPublisher, connMgr connections.ConnectionManager) {
	defer conn.Close()

	remoteAddr := conn.RemoteAddr().String()
	slog.Info("handling connection", "remote_addr", remoteAddr)

	factory := msg.NewMsgFactory()

	// 第一步：握手 + 鉴权
	hsPayload, err := authenticateClient(conn, cfg, factory, remoteAddr)
	if err != nil {
		return
	}

	// 第二步：注册连接状态，返回清理函数
	cleanup := registerConnection(connMgr, hsPayload.Sn, remoteAddr)
	defer cleanup()

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
			handleUpload(s, publisher, &message, hsPayload.Sn, remoteAddr)
		case msg.MsgTypeHeartbeat:
			handleHeartbeat(conn, connMgr, factory, &message, hsPayload.Sn)
		default:
			slog.Warn("unexpected message type",
				"remote_addr", remoteAddr, "sn", hsPayload.Sn, "type", message.Type)
		}
	}
}

// handleUpload 解析并持久化上传的传感器读数。
func handleUpload(s cache.Store, publisher EventPublisher, message *msg.Msg, sn, remoteAddr string) {
	uploadParsed, err := msg.ParseMsg(message)
	if err != nil {
		slog.Warn("invalid upload payload", "remote_addr", remoteAddr, "sn", sn, "error", err)
		return
	}
	upload, ok := uploadParsed.(*msg.UploadPayload)
	if !ok {
		slog.Warn("invalid upload payload", "remote_addr", remoteAddr, "sn", sn, "payload", uploadParsed)
		return
	}

	readings := make([]*model.Reading, len(upload.Readings))
	for i := range upload.Readings {
		readings[i] = &upload.Readings[i]
	}
	if len(readings) > 0 {
		if err := s.SaveReadings(context.Background(), sn, readings); err != nil {
			slog.Error("failed to save readings",
				"remote_addr", remoteAddr,
				"sn", sn,
				"count", len(readings),
				"error", err)
		} else {
			slog.Info("saved readings",
				"remote_addr", remoteAddr,
				"sn", sn,
				"count", len(readings))
			// TODO: 后续实现 payload 序列化（JSON/MessagePack），框架阶段仅预留调用点
			// payload := serializeReadings(readings)
			// if err := publisher.PublishDeviceReadings(sn, payload); err != nil {
			//     slog.Error("mqtt publish readings failed", "sn", sn, "error", err)
			// }
			_ = publisher
		}
	}
}

// handleHeartbeat 更新心跳时间并回复 HeartbeatAck。
func handleHeartbeat(conn net.Conn, connMgr connections.ConnectionManager, factory *msg.MsgFactory, message *msg.Msg, sn string) {
	if err := connMgr.UpdateHeartbeat(sn); err != nil {
		slog.Warn("failed to update heartbeat", "sn", sn, "error", err)
	}
	sendAck(conn, factory, msg.MsgTypeHeartbeatAck, message.MsgID, msg.Ok)
}

// registerConnection 注册网关在线状态并返回连接断开时的清理函数。
func registerConnection(connMgr connections.ConnectionManager, sn, remoteAddr string) func() {
	connInfo := &connections.Connection{
		DeviceHash:    sn,
		ConnectionID:  uuid.NewString(),
		ClientIP:      remoteAddr,
		ConnectedAt:   utils.NewTimestamp(),
		LastHeartbeat: utils.NewTimestamp(),
	}
	if err := connMgr.RegisterConnection(connInfo); err != nil {
		slog.Warn("failed to register connection", "sn", sn, "error", err)
	}
	return func() {
		if err := connMgr.RemoveConnection(sn); err != nil {
			slog.Warn("failed to remove connection", "sn", sn, "error", err)
		}
	}
}

// authenticateClient 等待握手消息并校验 SN+Token，鉴权成功返回解析后的 HandshakePayload。
// 所有失败路径均已内部发送对应错误响应，调用方仅需检查 error。
func authenticateClient(conn net.Conn, cfg *ServerConfig, factory *msg.MsgFactory, remoteAddr string) (*msg.HandshakePayload, error) {
	hsMsg, err := msg.DecodeFrame(conn)
	if err != nil {
		slog.Warn("failed to read handshake", "remote_addr", remoteAddr, "error", err)
		return nil, err
	}

	if hsMsg.Type != msg.MsgTypeHandshake {
		slog.Warn("expected handshake message", "remote_addr", remoteAddr, "got_type", hsMsg.Type)
		sendHandshakeResponse(conn, factory, hsMsg.MsgID, msg.Failed)
		return nil, fmt.Errorf("expected handshake, got type %d", hsMsg.Type)
	}

	parsed, err := msg.ParseMsg(&hsMsg)
	if err != nil {
		slog.Warn("invalid handshake payload", "remote_addr", remoteAddr, "error", err)
		sendHandshakeResponse(conn, factory, hsMsg.MsgID, msg.Failed)
		return nil, err
	}
	hsPayload, ok := parsed.(*msg.HandshakePayload)
	if !ok {
		slog.Warn("invalid handshake payload", "remote_addr", remoteAddr, "payload", parsed)
		sendHandshakeResponse(conn, factory, hsMsg.MsgID, msg.Failed)
		return nil, fmt.Errorf("invalid handshake payload type: %T", parsed)
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
		return nil, fmt.Errorf("unknown client SN: %s", hsPayload.Sn)
	}
	if matched.Token != hsPayload.Token {
		slog.Warn("token mismatch", "remote_addr", remoteAddr, "sn", hsPayload.Sn)
		sendHandshakeResponse(conn, factory, hsMsg.MsgID, msg.BadToken)
		return nil, fmt.Errorf("token mismatch for SN: %s", hsPayload.Sn)
	}

	// 鉴权成功
	slog.Info("client authenticated", "remote_addr", remoteAddr, "sn", hsPayload.Sn)
	sendHandshakeResponse(conn, factory, hsMsg.MsgID, msg.Ok)
	return hsPayload, nil
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
