package client

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"time"

	"LEPG/internal/model"

	mqtt "github.com/wind-c/comqtt/v2/mqtt"
	"github.com/wind-c/comqtt/v2/mqtt/hooks/auth"
	"github.com/wind-c/comqtt/v2/mqtt/listeners"
	"github.com/wind-c/comqtt/v2/mqtt/packets"
)

const (
	mqttTopicReading = "device/%s/reading"
	mqttTopicStatus  = "device/%s/status"
	mqttTopicFilter  = "device/+/reading"
	mqttBrokerAddr   = "127.0.0.1:1883"
)

type virtualPoint struct {
	Name     string
	DataType model.DataType
	Unit     string
}

type virtualDevice struct {
	SN       string
	Name     string
	Interval time.Duration
	Points   []virtualPoint
	GenFunc  func(pointName string, tick int) (float64, bool)
	tick     int
}

type mqttReadingJSON struct {
	DeviceName string      `json:"device_name"`
	PointName  string      `json:"point_name"`
	DataType   string      `json:"data_type"`
	Value      interface{} `json:"value"`
	Unit       string      `json:"unit"`
	Timestamp  int64       `json:"timestamp"`
}

func StartMqttBroker(ctx context.Context, ch chan<- model.Reading) error {
	opts := &mqtt.Options{InlineClient: true}
	server := mqtt.New(opts)
	server.AddHook(new(auth.AllowHook), nil)

	tcp := listeners.NewTCP("mqtt-tcp", mqttBrokerAddr, nil)
	if err := server.AddListener(tcp); err != nil {
		return fmt.Errorf("add mqtt tcp listener: %w", err)
	}

	server.Subscribe(mqttTopicFilter, 1, func(cl *mqtt.Client, sub packets.Subscription, pk packets.Packet) {
		handleMqttReading(pk.TopicName, pk.Payload, ch)
	})

	go func() {
		if err := server.Serve(); err != nil {
			slog.Error("mqtt broker serve error", "error", err)
		}
	}()
	slog.Info("mqtt broker started", "addr", mqttBrokerAddr)

	devices := hardcodedDevices()
	for _, d := range devices {
		go runVirtualDevice(ctx, server, d)
	}

	<-ctx.Done()
	server.Close()
	return nil
}

func handleMqttReading(topic string, payload []byte, ch chan<- model.Reading) {
	var readings []mqttReadingJSON
	if err := json.Unmarshal(payload, &readings); err != nil {
		slog.Error("mqtt: parse reading failed", "topic", topic, "error", err)
		return
	}

	for _, r := range readings {
		var numVal float64
		var boolVal bool

		switch v := r.Value.(type) {
		case float64:
			numVal = v
		case bool:
			boolVal = v
		}

		reading := model.Reading{
			DeviceName: r.DeviceName,
			PointName:  r.PointName,
			DataType:   model.DataType(r.DataType),
			NumVal:     numVal,
			BoolVal:    boolVal,
			Unit:       r.Unit,
			Timestamp:  r.Timestamp,
			DeviceHash: virtualDeviceHash(r.DeviceName),
			Status:     0,
		}

		if r.DataType == string(model.DataTypeBool) {
			slog.Info("mqtt reading", "device", r.DeviceName, "point", r.PointName, "value", boolVal, "unit", r.Unit)
		} else {
			slog.Info("mqtt reading", "device", r.DeviceName, "point", r.PointName, "value", numVal, "unit", r.Unit)
		}

		ch <- reading
	}
}

func virtualDeviceHash(sn string) string {
	h := sha256.Sum256([]byte("mqtt:" + sn))
	return fmt.Sprintf("%x", h[:8])
}

func runVirtualDevice(ctx context.Context, server *mqtt.Server, d *virtualDevice) {
	ticker := time.NewTicker(d.Interval)
	defer ticker.Stop()

	statusTopic := fmt.Sprintf(mqttTopicStatus, d.SN)
	onlineStatus, _ := json.Marshal(map[string]any{"status": "online", "timestamp": time.Now().UnixMilli()})
	server.Publish(statusTopic, onlineStatus, true, 1)

	slog.Info("virtual device online", "device", d.Name, "sn", d.SN)

	for {
		select {
		case <-ctx.Done():
			offlineStatus, _ := json.Marshal(map[string]any{"status": "offline", "timestamp": time.Now().UnixMilli()})
			server.Publish(statusTopic, offlineStatus, true, 1)
			slog.Info("virtual device offline", "device", d.Name, "sn", d.SN)
			return
		case <-ticker.C:
			d.tick++
			topic := fmt.Sprintf(mqttTopicReading, d.SN)
			payload := d.generateReadings()
			data, _ := json.Marshal(payload)
			server.Publish(topic, data, false, 0)
		}
	}
}

func (d *virtualDevice) generateReadings() []mqttReadingJSON {
	ts := time.Now().UnixMilli()
	readings := make([]mqttReadingJSON, len(d.Points))
	for i, p := range d.Points {
		numVal, boolVal := d.GenFunc(p.Name, d.tick)
		var value any
		if p.DataType == model.DataTypeBool {
			value = boolVal
		} else {
			value = numVal
		}
		readings[i] = mqttReadingJSON{
			DeviceName: d.SN,
			PointName:  p.Name,
			DataType:   string(p.DataType),
			Value:      value,
			Unit:       p.Unit,
			Timestamp:  ts,
		}
	}
	return readings
}

func roundTo(v float64, places int) float64 {
	pow := math.Pow10(places)
	return math.Round(v*pow) / pow
}

func hardcodedDevices() []*virtualDevice {
	tempHum := &virtualDevice{
		SN: "SENSOR-TH-001", Name: "温湿度传感器",
		Interval: 2 * time.Second,
		Points: []virtualPoint{
			{Name: "temperature", DataType: model.DataTypeFloat32, Unit: "°C"},
			{Name: "humidity", DataType: model.DataTypeFloat32, Unit: "%RH"},
		},
		GenFunc: func(point string, tick int) (float64, bool) {
			if point == "temperature" {
				return roundTo(25+8*math.Sin(float64(tick)*0.05)+rand.NormFloat64()*0.3, 2), false
			}
			return roundTo(55+15*math.Sin(float64(tick)*0.03+1)+rand.NormFloat64(), 2), false
		},
	}

	powerMeter := &virtualDevice{
		SN: "METER-PWR-001", Name: "电表",
		Interval: 1 * time.Second,
		Points: []virtualPoint{
			{Name: "voltage", DataType: model.DataTypeFloat32, Unit: "V"},
			{Name: "current", DataType: model.DataTypeFloat32, Unit: "A"},
			{Name: "power", DataType: model.DataTypeFloat32, Unit: "kW"},
			{Name: "frequency", DataType: model.DataTypeFloat32, Unit: "Hz"},
		},
		GenFunc: func(point string, tick int) (float64, bool) {
			load := math.Sin(float64(tick)*0.08)*0.5 + 0.5 + rand.NormFloat64()*0.05
			if load < 0 {
				load = 0
			}
			switch point {
			case "voltage":
				return roundTo(220+rand.NormFloat64()*2, 1), false
			case "current":
				return roundTo(load*50, 2), false
			case "power":
				return roundTo(load*11, 2), false
			case "frequency":
				return roundTo(50+rand.NormFloat64()*0.05, 2), false
			}
			return 0, false
		},
	}

	pressure := &virtualDevice{
		SN: "SENSOR-PRS-001", Name: "压力变送器",
		Interval: 3 * time.Second,
		Points: []virtualPoint{
			{Name: "pressure", DataType: model.DataTypeFloat32, Unit: "MPa"},
		},
		GenFunc: func(point string, tick int) (float64, bool) {
			base := 1.2 + 0.3*math.Sin(float64(tick)*0.02)
			step := 0.0
			if rand.Float64() < 0.02 {
				step = 0.8
			}
			return roundTo(base+step+rand.NormFloat64()*0.02, 3), false
		},
	}

	gas := &virtualDevice{
		SN: "SENSOR-GAS-001", Name: "气体检测器",
		Interval: 5 * time.Second,
		Points: []virtualPoint{
			{Name: "co2", DataType: model.DataTypeUint16, Unit: "ppm"},
			{Name: "smoke_alarm", DataType: model.DataTypeBool, Unit: ""},
		},
		GenFunc: func(point string, tick int) (float64, bool) {
			if point == "co2" {
				v := 800 + 1500*math.Max(0, math.Sin(float64(tick)*0.01)) + rand.NormFloat64()*50
				return roundTo(v, 0), false
			}
			return 0, rand.Float64() < 0.005
		},
	}

	flowTotal := 12456.0
	flowMeter := &virtualDevice{
		SN: "SENSOR-FLW-001", Name: "流量计",
		Interval: 2 * time.Second,
		Points: []virtualPoint{
			{Name: "flow_rate", DataType: model.DataTypeFloat32, Unit: "m³/h"},
			{Name: "total", DataType: model.DataTypeUint32, Unit: "m³"},
		},
		GenFunc: func(point string, tick int) (float64, bool) {
			rate := 50 + 30*math.Sin(float64(tick)*0.06) + rand.NormFloat64()*3
			if rate < 0 {
				rate = 0
			}
			flowTotal += rate / 3600 * 2
			if point == "flow_rate" {
				return roundTo(rate, 2), false
			}
			return flowTotal, false
		},
	}

	return []*virtualDevice{tempHum, powerMeter, pressure, gas, flowMeter}
}
