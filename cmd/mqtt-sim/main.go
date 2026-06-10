// LEPG MQTT 传感器模拟器
// 模拟六种工业边缘设备，通过 MQTT 协议发布传感器数据
//
// 用法:
//
//	go run ./cmd/mqtt-sim
//	go run ./cmd/mqtt-sim --broker 192.168.1.100:1883 --interval 3
//	go run ./cmd/mqtt-sim --devices temp_hum,pressure
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

const (
	topicReading = "device/%s/reading/%s"
	topicStatus  = "device/%s/status"
	qosReading   = 0
	qosStatus    = 1
)

// --- 数据类型 ---

type point struct {
	Name     string
	DataType string
}

type device struct {
	SN       string
	Name     string
	Interval time.Duration
	Points   []point
	Read     func(pointName string, tick int) any
}

type readingPayload struct {
	Type    string `json:"type"`
	Value   any    `json:"value"`
	Quality int    `json:"quality"`
	TS      int64  `json:"ts"`
}

type statusPayload struct {
	Status    string `json:"status"`
	Timestamp int64  `json:"timestamp"`
}

func roundTo(x float64, n int) float64 {
	p := math.Pow10(n)
	return math.Round(x*p) / p
}

// --- 设备工厂 ---

func newTempHum(interval time.Duration) *device {
	return &device{
		SN:       "SENSOR-TH-001",
		Name:     "温湿度传感器",
		Interval: interval,
		Points: []point{
			{"temperature", "float32"},
			{"humidity", "float32"},
		},
		Read: func(p string, tick int) any {
			t := float64(tick)
			switch p {
			case "temperature":
				return roundTo(25.0+8.0*math.Sin(t*0.05)+rand.NormFloat64()*0.3, 2)
			case "humidity":
				return roundTo(55.0+15.0*math.Sin(t*0.03+1)+rand.NormFloat64(), 2)
			}
			return nil
		},
	}
}

func newPower(interval time.Duration) *device {
	return &device{
		SN:       "METER-PWR-001",
		Name:     "电表",
		Interval: interval,
		Points: []point{
			{"voltage", "float32"},
			{"current", "float32"},
			{"power", "float32"},
			{"frequency", "float32"},
		},
		Read: func(p string, tick int) any {
			t := float64(tick)
			load := math.Max(0, math.Sin(t*0.08)*0.5+0.5+rand.NormFloat64()*0.05)
			switch p {
			case "voltage":
				return roundTo(220.0+rand.NormFloat64()*2, 1)
			case "current":
				return roundTo(load*50, 2)
			case "power":
				return roundTo(load*11, 2)
			case "frequency":
				return roundTo(50.0+rand.NormFloat64()*0.05, 2)
			}
			return nil
		},
	}
}

func newPressure(interval time.Duration) *device {
	return &device{
		SN:       "SENSOR-PRS-001",
		Name:     "压力变送器",
		Interval: interval,
		Points:   []point{{"pressure", "float32"}},
		Read: func(_ string, tick int) any {
			t := float64(tick)
			base := 1.2 + 0.3*math.Sin(t*0.02)
			step := 0.0
			if rand.Float64() < 0.02 {
				step = 0.8
			}
			return roundTo(base+step+rand.NormFloat64()*0.02, 3)
		},
	}
}

func newGas(interval time.Duration) *device {
	return &device{
		SN:       "SENSOR-GAS-001",
		Name:     "气体检测器",
		Interval: interval,
		Points: []point{
			{"co2", "uint16"},
			{"smoke_alarm", "bool"},
		},
		Read: func(p string, tick int) any {
			t := float64(tick)
			switch p {
			case "co2":
				return int(800 + 1500*math.Max(0, math.Sin(t*0.01)) + rand.NormFloat64()*50)
			case "smoke_alarm":
				return rand.Float64() < 0.005
			}
			return nil
		},
	}
}

func newFlow(interval time.Duration) *device {
	total := 12456.0
	intervalSec := interval.Seconds()
	var mu sync.Mutex

	return &device{
		SN:       "SENSOR-FLW-001",
		Name:     "流量计",
		Interval: interval,
		Points: []point{
			{"flow_rate", "float32"},
			{"total", "uint32"},
		},
		Read: func(p string, tick int) any {
			mu.Lock()
			defer mu.Unlock()

			t := float64(tick)
			rate := math.Max(0, 50+30*math.Sin(t*0.06)+rand.NormFloat64()*3)
			total += rate / 3600 * intervalSec

			switch p {
			case "flow_rate":
				return roundTo(rate, 2)
			case "total":
				return int(total)
			}
			return nil
		},
	}
}

func newComplexJSON(interval time.Duration) *device {
	uptime := 86400.0
	energy := 125.6
	intervalSec := interval.Seconds()
	var mu sync.Mutex

	return &device{
		SN:       "GW-JSON-001",
		Name:     "边缘网关(复杂JSON)",
		Interval: interval,
		Points:   []point{{"status_report", "json"}},
		Read: func(_ string, tick int) any {
			mu.Lock()
			defer mu.Unlock()

			t := float64(tick)

			// 两台电机
			type motor struct {
				ID          string  `json:"id"`
				Status      string  `json:"status"`
				RPM         float64 `json:"rpm"`
				Temperature float64 `json:"temperature"`
			}

			motors := make([]motor, 2)
			for i, base := range []struct{ rpm, temp float64 }{{1500, 60}, {1200, 55}} {
				fi := float64(i)
				rpm := math.Max(0, base.rpm*(0.5+0.5*math.Sin(t*0.04+fi*1.5))+rand.NormFloat64()*20)
				temp := base.temp + (rpm/base.rpm)*15 + rand.NormFloat64()*1.5
				status := "running"
				if rpm < 100 {
					status = "idle"
				}
				motors[i] = motor{
					ID:          fmt.Sprintf("motor-%d", i+1),
					Status:      status,
					RPM:         roundTo(rpm, 1),
					Temperature: roundTo(temp, 1),
				}
			}

			// 整体状态
			r := rand.Float64()
			overall := "running"
			if r < 0.03 {
				overall = "fault"
			} else if r < 0.10 {
				overall = "warning"
			}

			type alarm struct {
				Code  string `json:"code"`
				Msg   string `json:"msg"`
				Level string `json:"level"`
			}
			var alarms []alarm
			if overall == "warning" {
				alarms = append(alarms, alarm{"W001", "motor vibration above threshold", "warning"})
			}
			if overall == "fault" {
				alarms = append(alarms, alarm{"E001", "motor over-temperature detected", "critical"})
			}

			uptime += intervalSec
			energy += 0.1 + rand.NormFloat64()*0.02

			type metrics struct {
				UptimeSeconds int     `json:"uptime_seconds"`
				EnergyKWh     float64 `json:"energy_kwh"`
				Efficiency    float64 `json:"efficiency"`
			}

			inner := map[string]any{
				"device_id":      "GW-JSON-001",
				"overall_status": overall,
				"sub_devices":    motors,
				"alarms":         alarms,
				"metrics": metrics{
					UptimeSeconds: int(uptime),
					EnergyKWh:     roundTo(energy, 2),
					Efficiency:    roundTo(0.85+0.10*math.Sin(t*0.02)+rand.NormFloat64()*0.01, 4),
				},
			}

			// value 是 JSON 字符串（双重编码），匹配 Go 端 DataTypeJSON 的解析方式
			data, _ := json.Marshal(inner)
			return string(data)
		},
	}
}

// --- 设备注册表 ---

type deviceFactory func(interval time.Duration) *device

var allDevices = map[string]struct {
	Factory   deviceFactory
	DefaultSec int
}{
	"temp_hum":    {newTempHum, 2},
	"power":       {newPower, 1},
	"pressure":    {newPressure, 3},
	"gas":         {newGas, 5},
	"flow":        {newFlow, 2},
	"complex_json": {newComplexJSON, 5},
}

// --- MQTT 连接与发布 ---

func connectWithRetry(ctx context.Context, client mqtt.Client, sn string) bool {
	delay := time.Second
	for {
		token := client.Connect()
		token.Wait()
		if token.Error() == nil {
			return true
		}
		slog.Warn("Broker 未就绪，重连中", "sn", sn, "delay", delay, "error", token.Error())

		select {
		case <-ctx.Done():
			return false
		case <-time.After(delay):
		}
		delay = min(delay*2, 30*time.Second)
	}
}

func publishReadings(client mqtt.Client, d *device, tick int) bool {
	for _, p := range d.Points {
		payload := readingPayload{
			Type:    p.DataType,
			Value:   d.Read(p.Name, tick),
			Quality: 0,
			TS:      time.Now().UnixMilli(),
		}
		data, err := json.Marshal(payload)
		if err != nil {
			return false
		}
		topic := fmt.Sprintf(topicReading, d.SN, p.Name)
		token := client.Publish(topic, qosReading, false, data)
		token.Wait()
		if token.Error() != nil {
			return false
		}
	}
	return true
}

func publishStatus(client mqtt.Client, sn, status string) {
	topic := fmt.Sprintf(topicStatus, sn)
	payload := statusPayload{status, time.Now().UnixMilli()}
	data, _ := json.Marshal(payload)
	token := client.Publish(topic, qosStatus, true, data)
	token.Wait()
}

func runDevice(ctx context.Context, d *device, brokerURL string) {
	statusTopic := fmt.Sprintf(topicStatus, d.SN)

	for {
		disconnected := make(chan struct{}, 1)

		offlineJSON, _ := json.Marshal(statusPayload{"offline", time.Now().UnixMilli()})

		opts := mqtt.NewClientOptions().AddBroker(brokerURL)
		opts.SetClientID(d.SN)
		opts.SetAutoReconnect(false)
		opts.SetWill(statusTopic, string(offlineJSON), qosStatus, true)
		opts.SetConnectionLostHandler(func(_ mqtt.Client, err error) {
			slog.Info("连接断开，准备重连", "sn", d.SN, "error", err)
			select {
			case disconnected <- struct{}{}:
			default:
			}
		})

		client := mqtt.NewClient(opts)

		if !connectWithRetry(ctx, client, d.SN) {
			fmt.Printf("  [%s] 已停止\n", d.SN)
			return
		}

		publishStatus(client, d.SN, "online")
		fmt.Printf("  [%s] 已上线\n", d.SN)

		tick := 0
		ticker := time.NewTicker(d.Interval)
		var pubFailed bool

	loop:
		for {
			select {
			case <-ctx.Done():
				ticker.Stop()
				publishStatus(client, d.SN, "offline")
				client.Disconnect(1000)
				fmt.Printf("  [%s] 已停止\n", d.SN)
				return
			case <-disconnected:
				ticker.Stop()
				break loop
			case <-ticker.C:
				tick++
				if !publishReadings(client, d, tick) {
					ticker.Stop()
					pubFailed = true
					break loop
				}
			}
		}

		if !pubFailed {
			publishStatus(client, d.SN, "offline")
		}
		client.Disconnect(1000)

		// 等待后重连
		select {
		case <-ctx.Done():
			fmt.Printf("  [%s] 已停止\n", d.SN)
			return
		case <-time.After(3 * time.Second):
		}
	}
}

// --- CLI ---

func parseArgs() (brokerURL string, interval time.Duration, selectedDevices []string) {
	broker := flag.String("broker", "127.0.0.1:1883", "MQTT Broker 地址 (host:port)")
	intervalSec := flag.Float64("interval", 0, "覆盖所有设备的采集间隔(秒)，0=使用设备默认值")
	devices := flag.String("devices", "all", "启用的设备，逗号分隔")
	flag.Parse()

	// 解析 broker URL
	host, portStr, hasPort := strings.Cut(*broker, ":")
	if !hasPort {
		portStr = "1883"
	}
	brokerURL = fmt.Sprintf("tcp://%s:%s", host, portStr)

	if *intervalSec > 0 {
		interval = time.Duration(*intervalSec * float64(time.Second))
	}

	if *devices == "all" {
		for k := range allDevices {
			selectedDevices = append(selectedDevices, k)
		}
	} else {
		for d := range strings.SplitSeq(*devices, ",") {
			selectedDevices = append(selectedDevices, strings.TrimSpace(d))
		}
	}
	return
}

func main() {
	brokerURL, intervalOverride, selected := parseArgs()

	// 验证设备名
	var unknown []string
	for _, k := range selected {
		if _, ok := allDevices[k]; !ok {
			unknown = append(unknown, k)
		}
	}
	if len(unknown) > 0 {
		fmt.Printf("未知设备: %s\n", strings.Join(unknown, ", "))
		var valid []string
		for k := range allDevices {
			valid = append(valid, k)
		}
		fmt.Printf("可选: %s\n", strings.Join(valid, ", "))
		return
	}

	// 实例化设备
	var devices []*device
	for _, key := range selected {
		entry := allDevices[key]
		dur := time.Duration(entry.DefaultSec) * time.Second
		if intervalOverride > 0 {
			dur = intervalOverride
		}
		devices = append(devices, entry.Factory(dur))
	}

	fmt.Println("=======================================================")
	fmt.Println("LEPG MQTT 传感器模拟器")
	fmt.Printf("Broker: %s\n", brokerURL)
	fmt.Println("-------------------------------------------------------")
	for _, d := range devices {
		pointNames := make([]string, len(d.Points))
		for i, p := range d.Points {
			pointNames[i] = p.Name
		}
		fmt.Printf("  %-14s [%s]  间隔 %ds  采集点: %s\n",
			d.Name, d.SN, int(d.Interval.Seconds()), strings.Join(pointNames, ", "))
	}
	fmt.Println("=======================================================")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	for _, d := range devices {
		go runDevice(ctx, d, brokerURL)
	}

	<-ctx.Done()
	fmt.Println("\n正在停止所有设备...")

	// 给设备一点时间发布 offline 状态
	time.Sleep(1500 * time.Millisecond)
	fmt.Println("所有设备已停止")
}
