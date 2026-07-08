package client

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"time"

	"LEPG/internal/msg"
	"LEPG/internal/model"

	mqtt "github.com/wind-c/comqtt/v2/mqtt"
	"github.com/wind-c/comqtt/v2/mqtt/hooks/auth"
	"github.com/wind-c/comqtt/v2/mqtt/listeners"
	"github.com/wind-c/comqtt/v2/mqtt/packets"
)

// sourceFormat represents a parsed source string like "json:data.temp".
type sourceFormat struct {
	format string // "json", "plain", "kv"
	path   string // field path for json/kv, empty for plain
}

func parseSource(source string) (sourceFormat, error) {
	switch {
	case strings.HasPrefix(source, "json:"):
		return sourceFormat{format: "json", path: source[5:]}, nil
	case source == "plain:":
		return sourceFormat{format: "plain"}, nil
	case strings.HasPrefix(source, "kv:"):
		return sourceFormat{format: "kv", path: source[3:]}, nil
	default:
		return sourceFormat{}, fmt.Errorf("invalid source: %s", source)
	}
}

type statusHook struct {
	mqtt.HookBase
	clientIDIndex map[string]string
	deviceHashes  map[string]string
	notifyCh      chan<- msg.NotifyPayload
}

func (h *statusHook) Provides(b byte) bool {
	return bytes.Contains([]byte{mqtt.OnConnect, mqtt.OnDisconnect}, []byte{b})
}

func (h *statusHook) OnConnect(cl *mqtt.Client, pk packets.Packet) error {
	return nil
}

func (h *statusHook) OnDisconnect(cl *mqtt.Client, err error, expire bool) {
	if !expire {
		return
	}
	deviceName, ok := h.clientIDIndex[cl.ID]
	if !ok {
		return
	}
	deviceHash := h.deviceHashes[deviceName]
	payload := msg.NotifyPayload{
		EventCode:  0x02,
		DeviceHash: deviceHash,
		Timestamp:  uint32(time.Now().Unix()),
		Severity:   0,
		Message:    "device offline",
	}
	select {
	case h.notifyCh <- payload:
	default:
	}
}

// StartMqttBroker starts a local MQTT broker and routes incoming readings to ch.
func StartMqttBroker(ctx context.Context, ch chan<- model.Reading, notifyCh chan<- msg.NotifyPayload, mqttCfg *MqttConfig) error {
	opts := &mqtt.Options{InlineClient: true}
	server := mqtt.New(opts)
	server.AddHook(new(auth.AllowHook), nil)

	tcp := listeners.NewTCP("mqtt-tcp", mqttCfg.BrokerAddr, nil)
	if err := server.AddListener(tcp); err != nil {
		return fmt.Errorf("add mqtt tcp listener: %w", err)
	}

	// Pre-compute device hashes and client ID index.
	deviceHashes := make(map[string]string, len(mqttCfg.Devices))
	clientIDIndex := make(map[string]string, len(mqttCfg.Devices))
	for _, dev := range mqttCfg.Devices {
		deviceHashes[dev.Name] = virtualDeviceHash(dev.Name)
		if dev.ClientID != "" {
			clientIDIndex[dev.ClientID] = dev.Name
		}
	}

	// Register status hook for online/offline notifications.
	server.AddHook(&statusHook{
		clientIDIndex: clientIDIndex,
		deviceHashes:  deviceHashes,
		notifyCh:      notifyCh,
	}, nil)

	topicCount := 0
	for _, dev := range mqttCfg.Devices {
		for _, tc := range dev.Topics {
			sf, err := parseSource(tc.Source)
			if err != nil {
				return fmt.Errorf("device %s topic %s: %w", dev.Name, tc.Topic, err)
			}
			topicCount++
			devName := dev.Name
			server.Subscribe(tc.Topic, topicCount, func(cl *mqtt.Client, sub packets.Subscription, pk packets.Packet) {
				handleMqttReading(pk.TopicName, pk.Payload, ch, deviceHashes, tc, devName, sf)
			})
		}
	}

	go func() {
		if err := server.Serve(); err != nil {
			slog.Error("mqtt broker serve error", "error", err)
		}
	}()
	slog.Info("mqtt broker started", "addr", mqttCfg.BrokerAddr, "topics", topicCount)

	<-ctx.Done()
	server.Close()
	return nil
}

func handleMqttReading(
	topic string,
	payload []byte,
	ch chan<- model.Reading,
	deviceHashes map[string]string,
	tc *TopicConfig,
	devName string,
	sf sourceFormat,
) {
	// Extract value and validate against declared type.
	val, err := extractValue(sf, payload)
	if err != nil {
		slog.Error("mqtt: extract value failed", "topic", topic, "source", tc.Source, "error", err)
		return
	}
	if err := checkDataType(val, tc.DataType); err != nil {
		slog.Error("mqtt: type mismatch", "topic", topic, "data_type", tc.DataType, "value", val, "error", err)
		return
	}

	// Extract timestamp; fall back to system time.
	ts, err := extractTimestamp(sf, payload)
	if err != nil {
		slog.Error("mqtt: parse timestamp failed", "topic", topic, "error", err)
		return
	}
	if ts == 0 {
		ts = time.Now().UnixMilli()
	}

	reading := model.Reading{
		Device:     deviceHashes[devName],
		DeviceName: devName,
		Point:      model.HashPoint(devName, tc.PointName),
		PointName:  tc.PointName,
		DataType:   tc.DataType,
		Value:      model.SerializeValue(tc.DataType, val),
		Quality:    model.QualityGood,
		Unit:       tc.Unit,
		Timestamp:  ts,
	}

	logReading("mqtt", devName, tc.PointName, tc.DataType, val, tc.Unit)

	ch <- reading
}

// ----- value extraction -----

func extractValue(sf sourceFormat, payload []byte) (any, error) {
	switch sf.format {
	case "json":
		return extractJSONValue(payload, sf.path)
	case "plain":
		return extractPlainValue(payload)
	case "kv":
		return extractKVValue(payload, sf.path)
	default:
		return nil, fmt.Errorf("unknown format: %s", sf.format)
	}
}

// extractJSONValue walks a dot-separated path into a JSON payload and returns the leaf value.
func extractJSONValue(payload []byte, path string) (any, error) {
	var root any
	if err := json.Unmarshal(payload, &root); err != nil {
		return nil, fmt.Errorf("json parse: %w", err)
	}

	parts := strings.Split(path, ".")
	current := root
	for i, part := range parts {
		m, ok := current.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("cannot index into non-object at %q", strings.Join(parts[:i], "."))
		}
		v, exists := m[part]
		if !exists {
			return nil, fmt.Errorf("key %q not found", path)
		}
		current = v
	}
	return current, nil
}

// extractPlainValue parses the entire payload as a scalar.
func extractPlainValue(payload []byte) (any, error) {
	s := strings.TrimSpace(string(payload))
	if s == "" {
		return nil, fmt.Errorf("plain: empty payload")
	}

	// Try number first, then string.
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f, nil
	}
	if b, err := strconv.ParseBool(s); err == nil {
		return b, nil
	}
	return s, nil
}

// extractKVValue parses "KEY1=VAL1;KEY2=VAL2;..." and returns the value for the given key.
func extractKVValue(payload []byte, key string) (any, error) {
	s := strings.TrimSpace(string(payload))
	pairs := strings.Split(s, ";")
	for _, pair := range pairs {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		kv := strings.SplitN(pair, "=", 2)
		if len(kv) != 2 {
			continue
		}
		if strings.TrimSpace(kv[0]) == key {
			raw := strings.TrimSpace(kv[1])
			// Try number, then bool, then string.
			if f, err := strconv.ParseFloat(raw, 64); err == nil {
				return f, nil
			}
			if b, err := strconv.ParseBool(raw); err == nil {
				return b, nil
			}
			return raw, nil
		}
	}
	return nil, fmt.Errorf("kv: key %q not found", key)
}

// ----- type checking -----

func checkDataType(val any, dt model.DataType) error {
	switch dt {
	case model.DataTypeFloat32, model.DataTypeFloat64:
		if _, ok := val.(float64); !ok {
			return fmt.Errorf("expected number, got %T", val)
		}
	case model.DataTypeInt16, model.DataTypeUint16, model.DataTypeInt32, model.DataTypeUint32:
		f, ok := val.(float64)
		if !ok {
			return fmt.Errorf("expected integer, got %T", val)
		}
		if f != math.Trunc(f) {
			return fmt.Errorf("expected integer, got %v", f)
		}
	case model.DataTypeBool:
		if _, ok := val.(bool); !ok {
			return fmt.Errorf("expected bool, got %T", val)
		}
	case model.DataTypeString:
		if _, ok := val.(string); !ok {
			return fmt.Errorf("expected string, got %T", val)
		}
	case model.DataTypeJSON:
		// anything goes
	}
	return nil
}

// ----- timestamp extraction -----

func extractTimestamp(sf sourceFormat, payload []byte) (int64, error) {
	switch sf.format {
	case "json":
		return extractJSONTimestamp(payload)
	case "kv":
		return extractKVTimestamp(payload)
	case "plain":
		return 0, nil // caller uses system time
	default:
		return 0, nil
	}
}

func extractJSONTimestamp(payload []byte) (int64, error) {
	var root map[string]any
	if err := json.Unmarshal(payload, &root); err != nil {
		return 0, nil // can't parse → use system time, not an error
	}
	// Try common timestamp field names.
	for _, name := range []string{"ts", "timestamp", "time"} {
		if v, ok := root[name]; ok {
			return detectTimestamp(v)
		}
	}
	return 0, nil
}

func extractKVTimestamp(payload []byte) (int64, error) {
	s := strings.TrimSpace(string(payload))
	pairs := strings.Split(s, ";")
	for _, pair := range pairs {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		kv := strings.SplitN(pair, "=", 2)
		if len(kv) != 2 {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(kv[0]), "ts") {
			return detectTimestampString(strings.TrimSpace(kv[1]))
		}
	}
	return 0, nil
}

func detectTimestamp(v any) (int64, error) {
	switch t := v.(type) {
	case float64:
		// Auto-detect ms vs seconds. Threshold: 1e12 = year 2286 in ms,
		// anything below is seconds.
		if t > 1e12 {
			return int64(t), nil
		}
		return int64(t * 1000), nil
	case string:
		return detectTimestampString(t)
	default:
		return 0, fmt.Errorf("unsupported timestamp type %T", v)
	}
}

func detectTimestampString(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}

	// Try numeric first.
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		if n > 1e12 {
			return n, nil
		}
		return n * 1000, nil
	}

	// Try RFC3339.
	layouts := []string{
		time.RFC3339,
		time.RFC3339Nano,
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		"2006/01/02 15:04:05",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UnixMilli(), nil
		}
	}
	return 0, fmt.Errorf("unrecognized timestamp format: %q", s)
}

// ----- helpers -----

func virtualDeviceHash(name string) string {
	h := sha256.Sum256([]byte("mqtt:" + name))
	return fmt.Sprintf("%x", h[:8])
}
