package server

import (
	"fmt"
	"log/slog"
	"strings"

	mqtt "github.com/wind-c/comqtt/v2/mqtt"
	"github.com/wind-c/comqtt/v2/mqtt/hooks/auth"
	"github.com/wind-c/comqtt/v2/mqtt/listeners"
)

const (
	TopicPrefix  = "device/"
	TopicReading = "device/%s/reading"
	TopicEvent   = "device/%s/event"
	TopicStatus  = "device/%s/status"
)

type MqttBroker struct {
	server *mqtt.Server
	cfg    *MqttConfig
}

func NewMqttBroker(cfg *MqttConfig) *MqttBroker {
	opts := &mqtt.Options{InlineClient: true}
	s := mqtt.New(opts)
	return &MqttBroker{server: s, cfg: cfg}
}

func (b *MqttBroker) Start() error {
	if !isLocalAddr(b.cfg.TCPAddr) {
		// TODO: 后续替换为自定义认证 Hook，框架阶段允许但打印警告
		slog.Warn("MQTT broker binding to non-loopback address without custom auth",
			"addr", b.cfg.TCPAddr)
	}

	// TODO: 后续替换为基于 SN/Token 的自定义认证 Hook
	b.server.AddHook(new(auth.AllowHook), nil)

	tcp := listeners.NewTCP("mqtt-tcp", b.cfg.TCPAddr, nil)
	if err := b.server.AddListener(tcp); err != nil {
		return fmt.Errorf("add mqtt tcp listener: %w", err)
	}

	if b.cfg.WSAddr != "" {
		ws := listeners.NewWebsocket("mqtt-ws", b.cfg.WSAddr, nil)
		if err := b.server.AddListener(ws); err != nil {
			return fmt.Errorf("add mqtt ws listener: %w", err)
		}
	}

	go func() {
		if err := b.server.Serve(); err != nil {
			slog.Error("mqtt broker serve error", "error", err)
		}
	}()

	slog.Info("mqtt broker started", "tcp", b.cfg.TCPAddr, "ws", b.cfg.WSAddr)
	return nil
}

func (b *MqttBroker) Stop() {
	b.server.Close()
}

func (b *MqttBroker) Publish(topic string, payload []byte, retain bool, qos byte) error {
	if err := b.server.Publish(topic, payload, retain, qos); err != nil {
		slog.Error("mqtt publish failed", "topic", topic, "error", err)
		return err
	}
	return nil
}

func isLocalAddr(addr string) bool {
	if strings.HasPrefix(addr, ":") {
		return false
	}
	return strings.HasPrefix(addr, "127.") || strings.HasPrefix(addr, "localhost")
}
