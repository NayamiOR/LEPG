package server

import "fmt"

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
