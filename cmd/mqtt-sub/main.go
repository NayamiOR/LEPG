// LEPG MQTT Pull 测试订阅工具
// 订阅服务端 comqtt broker 的 device/+/reading 主题，实时打印读数日志。
//
// 用法:
//
//	go run ./cmd/mqtt-sub
//	go run ./cmd/mqtt-sub --broker 192.168.1.100:1883
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type mqttReading struct {
	DeviceName string `json:"device_name"`
	PointName  string `json:"point_name"`
	Value      any    `json:"value"`
}

func main() {
	broker := flag.String("broker", "127.0.0.1:1883", "MQTT broker address")
	flag.Parse()

	brokerURL := fmt.Sprintf("tcp://%s", *broker)

	opts := mqtt.NewClientOptions().AddBroker(brokerURL)
	opts.SetClientID("lepg-mqtt-sub")

	opts.SetDefaultPublishHandler(func(_ mqtt.Client, msg mqtt.Message) {
		var readings []mqttReading
		if err := json.Unmarshal(msg.Payload(), &readings); err != nil {
			slog.Warn("parse payload failed", "topic", msg.Topic(), "error", err)
			return
		}
		for _, r := range readings {
			fmt.Printf("%-26s  %-16s  %-14s  %v\n",
				msg.Topic(), r.DeviceName, r.PointName, r.Value)
		}
	})

	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		slog.Error("connect failed", "error", token.Error())
		os.Exit(1)
	}
	slog.Info("connected", "broker", *broker)

	topic := "device/+/reading"
	if token := client.Subscribe(topic, 1, nil); token.Wait() && token.Error() != nil {
		slog.Error("subscribe failed", "topic", topic, "error", token.Error())
		os.Exit(1)
	}
	slog.Info("subscribed", "topic", topic)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down")
	client.Disconnect(500)
}
