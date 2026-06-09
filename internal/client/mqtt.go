package client

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"

	"LEPG/internal/model"

	mqtt "github.com/wind-c/comqtt/v2/mqtt"
	"github.com/wind-c/comqtt/v2/mqtt/hooks/auth"
	"github.com/wind-c/comqtt/v2/mqtt/listeners"
	"github.com/wind-c/comqtt/v2/mqtt/packets"
)

type topicRoute struct {
	deviceName string
	topicCfg   *TopicConfig
}

type mqttReadingJSON struct {
	Type    string `json:"type"`
	Value   any    `json:"value"`
	Quality uint8  `json:"quality"`
	TS      int64  `json:"ts"`
}

func StartMqttBroker(ctx context.Context, ch chan<- model.Reading, mqttCfg *MqttConfig) error {
	opts := &mqtt.Options{InlineClient: true}
	server := mqtt.New(opts)
	server.AddHook(new(auth.AllowHook), nil)

	tcp := listeners.NewTCP("mqtt-tcp", mqttCfg.BrokerAddr, nil)
	if err := server.AddListener(tcp); err != nil {
		return fmt.Errorf("add mqtt tcp listener: %w", err)
	}

	routes := make(map[string]topicRoute)
	for _, dev := range mqttCfg.Devices {
		for _, tc := range dev.Topics {
			routes[tc.Topic] = topicRoute{deviceName: dev.Name, topicCfg: tc}
			server.Subscribe(tc.Topic, int(tc.QoS), func(cl *mqtt.Client, sub packets.Subscription, pk packets.Packet) {
				handleMqttReading(pk.TopicName, pk.Payload, ch, routes)
			})
		}
	}

	go func() {
		if err := server.Serve(); err != nil {
			slog.Error("mqtt broker serve error", "error", err)
		}
	}()
	slog.Info("mqtt broker started", "addr", mqttCfg.BrokerAddr, "topics", len(routes))

	<-ctx.Done()
	server.Close()
	return nil
}

func handleMqttReading(topic string, payload []byte, ch chan<- model.Reading, routes map[string]topicRoute) {
	route, ok := routes[topic]
	if !ok {
		slog.Warn("mqtt: unknown topic", "topic", topic)
		return
	}

	var r mqttReadingJSON
	if err := json.Unmarshal(payload, &r); err != nil {
		slog.Error("mqtt: parse reading failed", "topic", topic, "error", err)
		return
	}

	var numVal float64
	var boolVal bool

	switch v := r.Value.(type) {
	case float64:
		numVal = v
	case bool:
		boolVal = v
	}

	quality := model.QualityGood
	if r.Quality > 0 {
		quality = model.Quality(r.Quality)
	}

	dt := model.DataType(r.Type)

	reading := model.Reading{
		Device:     virtualDeviceHash(route.deviceName),
		DeviceName: route.deviceName,
		Point:      model.HashPoint(route.deviceName, route.topicCfg.PointName),
		PointName:  route.topicCfg.PointName,
		DataType:   dt,
		Value:      model.SerializeValue(dt, numVal, boolVal),
		Quality:    quality,
		Unit:       route.topicCfg.Unit,
		Timestamp:  r.TS,
	}

	slog.Info("mqtt reading", "device", route.deviceName, "point", route.topicCfg.PointName, "value", reading.Value)

	ch <- reading
}

func virtualDeviceHash(name string) string {
	h := sha256.Sum256([]byte("mqtt:" + name))
	return fmt.Sprintf("%x", h[:8])
}
