package server

import (
	"LEPG/internal/model"
	"LEPG/internal/msg"
	"LEPG/internal/output"
	"LEPG/internal/server/cache"
	"LEPG/internal/server/cache/connections"
	"LEPG/internal/utils"
	"container/list"
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	// heartbeatTimeout is the server-side read deadline for detecting silent clients.
	heartbeatTimeout = 90 * time.Second
	// dedupCacheSize is the maximum number of payload hashes kept for deduplication.
	dedupCacheSize = 200
)

// dedupCache is a thread-safe LRU cache for upload payload deduplication.
type dedupCache struct {
	mu       sync.Mutex
	capacity int
	lruList  *list.List
	items    map[string]*list.Element
}

func newDedupCache(capacity int) *dedupCache {
	return &dedupCache{
		capacity: capacity,
		lruList:  list.New(),
		items:    make(map[string]*list.Element),
	}
}

func (c *dedupCache) exists(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if elem, ok := c.items[key]; ok {
		c.lruList.MoveToFront(elem)
		return true
	}
	return false
}

func (c *dedupCache) add(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if elem, ok := c.items[key]; ok {
		c.lruList.MoveToFront(elem)
		return
	}
	elem := c.lruList.PushFront(key)
	c.items[key] = elem
	if c.lruList.Len() > c.capacity {
		if oldest := c.lruList.Back(); oldest != nil {
			c.lruList.Remove(oldest)
			delete(c.items, oldest.Value.(string))
		}
	}
}

func (c *dedupCache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lruList.Init()
	c.items = make(map[string]*list.Element)
}

func payloadKey(payload []byte) string {
	h := sha256.Sum256(payload)
	return string(h[:])
}

var uploadDedup = newDedupCache(dedupCacheSize)

// ReceiveLoop 接收循环
func ReceiveLoop(cfg *ServerConfig, s cache.Store, publisher EventPublisher, connMgr connections.ConnectionManager, router *output.OutputRouter) error {
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
		go HandleConnection(conn, cfg, s, publisher, connMgr, router)
	}
}

func HandleConnection(conn net.Conn, cfg *ServerConfig, s cache.Store, publisher EventPublisher, connMgr connections.ConnectionManager, router *output.OutputRouter) {
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
			handleUpload(conn, factory, s, publisher, router, &message, hsPayload.Sn, remoteAddr)
		case msg.MsgTypeHeartbeat:
			handleHeartbeat(conn, connMgr, factory, &message, hsPayload.Sn)
		default:
			slog.Warn("unexpected message type",
				"remote_addr", remoteAddr, "sn", hsPayload.Sn, "type", message.Type)
		}
	}
}

// handleUpload 解析并持久化上传的传感器读数。
func handleUpload(conn net.Conn, factory *msg.MsgFactory, s cache.Store, publisher EventPublisher, router *output.OutputRouter, message *msg.Msg, sn, remoteAddr string) {
	key := sn + ":" + payloadKey(message.Payload)
	if uploadDedup.exists(key) {
		sendAck(conn, factory, msg.MsgTypeUploadAck, message.MsgID, msg.Ok)
		slog.Info("duplicate upload, skipped save", "sn", sn, "msg_id", message.MsgID)
		return
	}
	uploadDedup.add(key)

	uploadParsed, err := msg.ParseMsg(message)
	if err != nil {
		sendAck(conn, factory, msg.MsgTypeUploadAck, message.MsgID, msg.Failed)
		slog.Warn("invalid upload payload", "sn", sn, "error", err)
		return
	}
	upload, ok := uploadParsed.(*msg.UploadPayload)
	if !ok {
		sendAck(conn, factory, msg.MsgTypeUploadAck, message.MsgID, msg.Failed)
		slog.Warn("invalid upload payload type", "sn", sn)
		return
	}

	readings := make([]*model.Reading, len(upload.Readings))
	for i := range upload.Readings {
		readings[i] = &upload.Readings[i]
	}
	if len(readings) > 0 {
		if err := s.SaveReadings(context.Background(), sn, readings); err != nil {
			sendAck(conn, factory, msg.MsgTypeUploadAck, message.MsgID, msg.Failed)
			slog.Error("failed to save readings",
				"sn", sn,
				"count", len(readings),
				"error", err)
		} else {
			sendAck(conn, factory, msg.MsgTypeUploadAck, message.MsgID, msg.Ok)
			slog.Info("saved readings",
				"sn", sn,
				"count", len(readings))

			// 1. 内嵌 Broker（拉模式）— 发布到 device/{SN}/reading
			if payload, err := serializeReadings(readings); err != nil {
				slog.Warn("failed to serialize readings for mqtt", "sn", sn, "error", err)
			} else if err := publisher.PublishDeviceReadings(sn, payload); err != nil {
				slog.Warn("failed to publish readings to mqtt broker", "sn", sn, "error", err)
			}

			// 2. 对外 Push — 按 DeviceName 分组后通过 OutputRouter fan-out
			if router != nil {
				var grouped map[string][]model.Reading
				if len(readings) < 8 {
					grouped = groupReadingsByDeviceName(readings)
				} else {
					grouped = groupReadingsPrealloc(readings)
				}
				for deviceName, devReadings := range grouped {
					deviceKey := fmt.Sprintf("%s-%s", sn, deviceName)
					router.Send(deviceKey, devReadings)
				}
			}		}
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

// sendAck encodes and writes an Ack frame of the given type. Shared by
// sendHandshakeResponse, handleUpload, and the heartbeat handler.
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
	if resp.Checksum == 0 {
		slog.Error("failed to compute checksum", "type", ackType, "error", err)
		return
	}
	if _, err := conn.Write(encoded); err != nil {
		slog.Error("failed to send ack", "type", ackType, "error", err)
	}
}

// groupReadingsByDeviceName groups readings by their DeviceName field.
// Readings without a DeviceName are grouped under "__unknown__".
// 保持无分支的原始形态：实测给函数体加条件分支会破坏编译器对 map 的
// 逃逸/栈上优化（n=1 从 144B/1 allocs 退化到 544B/3），故小输入路径独立成函数。
func groupReadingsByDeviceName(readings []*model.Reading) map[string][]model.Reading {
	grouped := make(map[string][]model.Reading)
	for _, r := range readings {
		key := r.DeviceName
		if key == "" {
			key = "__unknown__"
		}
		grouped[key] = append(grouped[key], *r)
	}
	return grouped
}

// groupReadingsPrealloc 是大输入（≥8 条）的两遍预分配版本（2026-08-21）：
// 先统计每组数量再预分配容量，避免 append 从 nil 反复翻倍 realloc。
// 实测 n=1000 分配字节 -60%。调用方按规模选择，见 handleUpload。
func groupReadingsPrealloc(readings []*model.Reading) map[string][]model.Reading {
	counts := make(map[string]int, len(readings))
	for _, r := range readings {
		key := r.DeviceName
		if key == "" {
			key = "__unknown__"
		}
		counts[key]++
	}
	grouped := make(map[string][]model.Reading, len(counts))
	for k, n := range counts {
		grouped[k] = make([]model.Reading, 0, n)
	}
	for _, r := range readings {
		key := r.DeviceName
		if key == "" {
			key = "__unknown__"
		}
		grouped[key] = append(grouped[key], *r)
	}
	return grouped
}
