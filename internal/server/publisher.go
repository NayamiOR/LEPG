package server

import (
	"LEPG/internal/model"
	"encoding/json"
	"fmt"
	"strconv"
)

// EventPublisher 抽象数据输出接口，解耦 ReceiveLoop 与具体 broker 实现
type EventPublisher interface {
	PublishDeviceReadings(sn string, payload []byte) error
}

// NopPublisher 空实现，用于不需要 MQTT 输出的场景
type NopPublisher struct{}

func (n *NopPublisher) PublishDeviceReadings(sn string, payload []byte) error {
	return nil
}

// MqttPublisher 基于 MqttBroker 的实现
type MqttPublisher struct {
	broker *MqttBroker
}

func NewMqttPublisher(broker *MqttBroker) *MqttPublisher {
	return &MqttPublisher{broker: broker}
}

func (p *MqttPublisher) PublishDeviceReadings(sn string, payload []byte) error {
	topic := fmt.Sprintf(TopicReading, sn)
	return p.broker.Publish(topic, payload, false, 1)
}

// MqttReading is a single JSON payload published to the device/{SN}/reading topic.
type MqttReading struct {
	DeviceName string `json:"device_name"`
	PointName  string `json:"point_name"`
	DataType   string `json:"data_type"`
	Value      any    `json:"value"`
	Unit       string `json:"unit,omitempty"`
	Timestamp  int64  `json:"timestamp"`
}

// serializeReadings serializes a slice of model.Reading into a JSON batch array.
func serializeReadings(readings []*model.Reading) ([]byte, error) {
	mqttReadings := make([]MqttReading, len(readings))
	for i, r := range readings {
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
		mqttReadings[i] = MqttReading{
			DeviceName: r.DeviceName,
			PointName:  r.PointName,
			DataType:   string(r.DataType),
			Value:      val,
			Unit:       r.Unit,
			Timestamp:  r.Timestamp,
		}
	}
	return json.Marshal(mqttReadings)
}
