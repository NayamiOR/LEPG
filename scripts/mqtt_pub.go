// mosquitto_pub 等价脚本:
//
//	mosquitto_pub -d -q 1 -h 43.167.246.8 -p 1883 -t v1/devices/me/telemetry -u "VnXbY4Ufjdx2f2i6HnYz" -m "{temperature:25}"
//
// 用法:
//
//	go run scripts/mqtt_pub.go
//	go run scripts/mqtt_pub.go --token-as-password   # 把 token 当 password 发送
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/google/uuid"
)

const (
	host    = "43.167.246.8"
	port    = 1883
	topic   = "v1/devices/me/telemetry"
	token   = "DJIOle2bqYfMtpdfoZvs"
	qos     = 1
	message = `{"temperature":35}`
)

// var (
//
//	tokenAsPassword = flag.Bool("token-as-password", false, "将 token 作为 MQTT password（而非 username）发送")
//
// )
var f = false
var tokenAsPassword = &f

func main() {
	flag.Parse()

	clientID := "lepg-pub-" + uuid.NewString()[:8]
	brokerURL := fmt.Sprintf("tcp://%s:%d", host, port)

	// ============================================================
	// Step 2: MQTT 连接
	// ============================================================
	authMode := "username"
	if *tokenAsPassword {
		authMode = "password"
	}

	opts := mqtt.NewClientOptions().
		AddBroker(brokerURL).
		SetClientID(clientID).
		SetAutoReconnect(false).
		SetConnectTimeout(15 * time.Second).
		SetKeepAlive(30 * time.Second).
		SetProtocolVersion(4). // MQTT 3.1.1
		SetCleanSession(true).
		SetOrderMatters(false)

	// ThingsBoard access token: 尝试 username 或 password
	if *tokenAsPassword {
		opts.SetPassword(token)
	} else {
		opts.SetUsername(token)
	}

	// 调试回调：连接成功/断开时打印
	opts.SetOnConnectHandler(func(c mqtt.Client) {
		fmt.Println("🔗 MQTT CONNACK 收到，连接已建立")
	})
	opts.SetConnectionLostHandler(func(c mqtt.Client, err error) {
		fmt.Fprintf(os.Stderr, "⚠️  连接断开: %v\n", err)
	})

	client := mqtt.NewClient(opts)

	fmt.Printf("正在连接 %s (token 在 %s 字段)...\n", brokerURL, authMode)
	connToken := client.Connect()
	if !connToken.WaitTimeout(15 * time.Second) {
		fmt.Fprintln(os.Stderr, "\n❌ 错误: MQTT 连接超时（15s）")

		if !*tokenAsPassword {
			fmt.Fprintf(os.Stderr, "\n💡 提示: ThingsBoard 的 access token 有时需要放在 password 字段\n")
			fmt.Fprintf(os.Stderr, "   试试: go run scripts/mqtt_pub.go --token-as-password\n")
		}
		os.Exit(1)
	}
	if err := connToken.Error(); err != nil {
		fmt.Fprintf(os.Stderr, "❌ 错误: MQTT CONNECT 被拒绝: %v\n", err)

		// 如果 username 方式被拒，提示尝试 password
		if !*tokenAsPassword {
			fmt.Fprintf(os.Stderr, "\n💡 提示: 试试把 token 放到 password 字段:\n")
			fmt.Fprintf(os.Stderr, "   go run scripts/mqtt_pub.go --token-as-password\n")
		}
		os.Exit(1)
	}
	fmt.Println("✅ 已连接到 broker")

	defer func() {
		client.Disconnect(500)
		fmt.Println("🔌 已断开")
	}()

	// ============================================================
	// Step 3: 发布消息
	// ============================================================
	fmt.Printf("\n正在发布到 %s (QoS=%d)...\n", topic, qos)
	pubToken := client.Publish(topic, byte(qos), false, message)
	if !pubToken.WaitTimeout(10 * time.Second) {
		fmt.Fprintln(os.Stderr, "❌ 错误: 发布超时")
		os.Exit(1)
	}
	if err := pubToken.Error(); err != nil {
		fmt.Fprintf(os.Stderr, "❌ 错误: 发布失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✅ 发布成功")
}

type MqttSink struct {
	client mqtt.Client
}

func NewMqttSink(client mqtt.Client) *MqttSink {
	return &MqttSink{client: client}
}

func (m *MqttSink) publish() error {
	pubToken := m.client.Publish(topic, byte(qos), false, message)
	if !pubToken.WaitTimeout(10 * time.Second) {
		err := fmt.Errorf("发布超时")
		fmt.Fprintln(os.Stderr, "❌ 错误: 发布超时")
		return err
	}
	if err := pubToken.Error(); err != nil {
		err := fmt.Errorf("发布失败: %w", err)
		fmt.Fprintf(os.Stderr, "❌ 错误: 发布失败: %v\n", err)
		return err
	}
	return nil
}
