package client

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"LEPG/internal/model"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/google/uuid"
)

// ClientPublisher publishes readings to the local MQTT broker.
type ClientPublisher struct {
	client mqtt.Client
}

// NewClientPublisher creates and connects an MQTT client to the given broker.
func NewClientPublisher(brokerAddr string) (*ClientPublisher, error) {
	opts := mqtt.NewClientOptions().
		AddBroker(brokerAddr).
		SetClientID("lepgc-pub-" + uuid.New().String()[:8]).
		// 本地 broker 与 publisher 在同一进程内异步启动，首次连接可能落在
		// Serve() 就绪之前，开启连接重试以消除启动竞态。
		SetConnectRetry(true).
		SetConnectRetryInterval(500 * time.Millisecond).
		SetConnectTimeout(5 * time.Second)

	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		return nil, fmt.Errorf("connect to mqtt broker at %s: %w", brokerAddr, token.Error())
	}

	slog.Info("mqtt publisher connected", "broker", brokerAddr)
	return &ClientPublisher{client: client}, nil
}

// PublishReading serializes one reading as MqttReading JSON and publishes to
// device/{deviceName}/reading.
func (p *ClientPublisher) PublishReading(reading model.Reading) error {
	mr := fromModelReading(reading)
	payload, err := json.Marshal(mr)
	if err != nil {
		return fmt.Errorf("marshal reading: %w", err)
	}

	topic := fmt.Sprintf("device/%s/reading", reading.DeviceName)
	if token := p.client.Publish(topic, 1, false, payload); token.Wait() && token.Error() != nil {
		return fmt.Errorf("publish: %w", token.Error())
	}
	return nil
}

// PublishRaw publishes raw bytes to an arbitrary topic.
func (p *ClientPublisher) PublishRaw(topic string, payload []byte) error {
	if token := p.client.Publish(topic, 1, false, payload); token.Wait() && token.Error() != nil {
		return fmt.Errorf("publish to %s: %w", topic, token.Error())
	}
	return nil
}

// Close disconnects the MQTT client.
func (p *ClientPublisher) Close() {
	p.client.Disconnect(250)
}

// MqttReading mirrors server publisher's format for single-reading JSON payloads.
type MqttReading struct {
	DeviceName string `json:"device_name"`
	PointName  string `json:"point_name"`
	DataType   string `json:"data_type"`
	Value      any    `json:"value"`
	Unit       string `json:"unit,omitempty"`
	Timestamp  int64  `json:"timestamp"`
}

func fromModelReading(r model.Reading) MqttReading {
	var val any
	switch {
	case r.DataType == model.DataTypeBool:
		v, err := strconv.ParseBool(r.Value)
		if err != nil {
			v = false
		}
		val = v
	case r.DataType == model.DataTypeJSON:
		val = r.Value
	default:
		v, err := strconv.ParseFloat(r.Value, 64)
		if err != nil {
			v = 0
		}
		val = v
	}
	return MqttReading{
		DeviceName: r.DeviceName,
		PointName:  r.PointName,
		DataType:   string(r.DataType),
		Value:      val,
		Unit:       r.Unit,
		Timestamp:  r.Timestamp,
	}
}
