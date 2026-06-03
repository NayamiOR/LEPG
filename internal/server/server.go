package server

import (
	"LEPG/internal/model"
	"LEPG/internal/msg"
	"LEPG/internal/server/cache"
	"bytes"
	"context"
	"encoding/gob"
	"fmt"
	"log/slog"
	"net"
)

// ReceiveLoop 接收循环
func ReceiveLoop(cfg *ServerConfig, s cache.Store) error {
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
		go HandleConnection(conn, cfg.Clients, s)
	}
}

func HandleConnection(conn net.Conn, clients []ClientDef, s cache.Store) {
	defer conn.Close()

	remoteAddr := conn.RemoteAddr().String()
	slog.Info("handling connection", "remote_addr", remoteAddr)

	// 第一步：等待握手消息
	hsMsg, err := msg.DecodeFrame(conn)
	if err != nil {
		slog.Warn("failed to read handshake", "remote_addr", remoteAddr, "error", err)
		return
	}

	if hsMsg.Type != msg.MsgTypeHandshake {
		slog.Warn("expected handshake message", "remote_addr", remoteAddr, "got_type", hsMsg.Type)
		sendHandshakeResponse(conn, msg.HandshakeFailed, "expected handshake")
		return
	}

	hsPayload, err := msg.DecodeHandshakePayload(hsMsg.Payload)
	if err != nil {
		slog.Warn("invalid handshake payload", "remote_addr", remoteAddr, "error", err)
		sendHandshakeResponse(conn, msg.HandshakeFailed, "malformed handshake")
		return
	}

	// 校验 Sn + Token
	var matched *ClientDef
	for i := range clients {
		if clients[i].Sn == hsPayload.Sn {
			matched = &clients[i]
			break
		}
	}

	if matched == nil {
		slog.Warn("unknown client SN", "remote_addr", remoteAddr, "sn", hsPayload.Sn)
		sendHandshakeResponse(conn, msg.HandshakeBadSn, "unknown SN")
		return
	}
	if matched.Token != hsPayload.Token {
		slog.Warn("token mismatch", "remote_addr", remoteAddr, "sn", hsPayload.Sn)
		sendHandshakeResponse(conn, msg.HandshakeBadToken, "token mismatch")
		return
	}

	// 鉴权成功
	slog.Info("client authenticated", "remote_addr", remoteAddr, "sn", hsPayload.Sn)
	sendHandshakeResponse(conn, msg.HandshakeOK, "OK")

	// 进入正常消息处理循环
	messageCount := 1
	for {
		message, err := msg.DecodeFrame(conn)
		if err != nil {
			if messageCount > 1 {
				slog.Info("connection closed",
					"remote_addr", remoteAddr,
					"sn", hsPayload.Sn,
					"messages_processed", messageCount,
					"error", err)
			}
			return
		}

		messageCount++
		slog.Info("received message",
			"remote_addr", remoteAddr,
			"sn", hsPayload.Sn,
			"count", messageCount,
			"type", message.Type,
			"msg_id", message.MsgID,
			"payload_len", message.PayloadLen)

		if message.Type == msg.MsgTypeUpload {
			var readings []*model.Reading
			dec := gob.NewDecoder(bytes.NewReader(message.Payload))
			for {
				var r model.Reading
				if err := dec.Decode(&r); err != nil {
					break
				}
				readings = append(readings, &r)
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
				}
			}
		}
	}
}

func sendHandshakeResponse(conn net.Conn, code uint8, message string) {
	resp := &msg.HandshakeResponsePayload{Code: code, Message: message}
	payload, _ := resp.Encode()
	responseMsg := msg.New(msg.MsgTypeHandshake, payload)
	responseMsg.Flags = msg.FlagResponse
	encoded, _ := responseMsg.Encode()
	conn.Write(encoded)
}
