#!/usr/bin/env python3
"""
LEPG MQTT 传感器模拟器
模拟多种工业边缘设备，通过 MQTT 协议发布传感器数据

用法:
  python scripts/mqtt_simulator.py
  python scripts/mqtt_simulator.py --broker 192.168.1.100:1883 --interval 3
  python scripts/mqtt_simulator.py --devices temp_hum,pressure

依赖: pip install paho-mqtt
"""

import argparse
import json
import math
import random
import signal
import sys
import threading
import time
from abc import ABC, abstractmethod

import paho.mqtt.client as mqtt

BROKER_DEFAULT = "127.0.0.1:1883"
TOPIC_READING = "device/{}/reading/{}"
TOPIC_STATUS = "device/{}/status"
QOS_READING = 0
QOS_STATUS = 1


class Point:
    """单个采集点定义"""

    def __init__(self, name, data_type):
        self.name = name
        self.data_type = data_type


class SensorDevice(ABC):
    """传感器设备基类"""

    def __init__(self, sn, name, interval, points):
        self.sn = sn
        self.name = name
        self.interval = interval
        self.points = points
        self.tick = 0

    @abstractmethod
    def read_point(self, point, tick):
        """返回单个采集点的当前值"""
        ...

    def publish_readings(self, client):
        """逐点采集并独立发布到各自的 topic"""
        self.tick += 1
        for p in self.points:
            ts = int(time.time() * 1000)
            payload = json.dumps({
                "type": p.data_type,
                "value": self.read_point(p, self.tick),
                "quality": 0,
                "ts": ts,
            }, ensure_ascii=False)
            topic = TOPIC_READING.format(self.sn, p.name)
            result = client.publish(topic, payload, qos=QOS_READING)
            if result.rc != mqtt.MQTT_ERR_SUCCESS:
                return False
        return True


# ---- 具体设备实现 ----


class TempHumSensor(SensorDevice):
    """温湿度传感器 — 正弦波模拟日间温度变化 + 高斯噪声"""

    def __init__(self):
        super().__init__(
            sn="SENSOR-TH-001",
            name="温湿度传感器",
            interval=2,
            points=[
                Point("temperature", "float32"),
                Point("humidity", "float32"),
            ],
        )

    def read_point(self, point, tick):
        if point.name == "temperature":
            return round(25.0 + 8.0 * math.sin(tick * 0.05) + random.gauss(0, 0.3), 2)
        return round(55.0 + 15.0 * math.sin(tick * 0.03 + 1) + random.gauss(0, 1), 2)


class PowerMeter(SensorDevice):
    """电表 — 负载曲线 + 电压小幅波动"""

    def __init__(self):
        super().__init__(
            sn="METER-PWR-001",
            name="电表",
            interval=1,
            points=[
                Point("voltage", "float32"),
                Point("current", "float32"),
                Point("power", "float32"),
                Point("frequency", "float32"),
            ],
        )

    def read_point(self, point, tick):
        load = max(0.0, math.sin(tick * 0.08) * 0.5 + 0.5 + random.gauss(0, 0.05))
        if point.name == "voltage":
            return round(220.0 + random.gauss(0, 2), 1)
        if point.name == "current":
            return round(load * 50.0, 2)
        if point.name == "power":
            return round(load * 11.0, 2)
        return round(50.0 + random.gauss(0, 0.05), 2)


class PressureSensor(SensorDevice):
    """压力变送器 — 缓慢漂移 + 偶发阶跃"""

    def __init__(self):
        super().__init__(
            sn="SENSOR-PRS-001",
            name="压力变送器",
            interval=3,
            points=[Point("pressure", "float32")],
        )

    def read_point(self, point, tick):
        base = 1.2 + 0.3 * math.sin(tick * 0.02)
        step = 0.8 if random.random() < 0.02 else 0.0
        return round(base + step + random.gauss(0, 0.02), 3)


class GasDetector(SensorDevice):
    """气体检测器 — CO2 缓慢升降，偶发烟雾报警"""

    def __init__(self):
        super().__init__(
            sn="SENSOR-GAS-001",
            name="气体检测器",
            interval=5,
            points=[
                Point("co2", "uint16"),
                Point("smoke_alarm", "bool"),
            ],
        )

    def read_point(self, point, tick):
        if point.name == "co2":
            return int(800 + 1500 * max(0, math.sin(tick * 0.01)) + random.gauss(0, 50))
        return random.random() < 0.005


class FlowMeter(SensorDevice):
    """流量计 — 瞬时流量波动 + 累计值递增"""

    def __init__(self):
        super().__init__(
            sn="SENSOR-FLW-001",
            name="流量计",
            interval=2,
            points=[
                Point("flow_rate", "float32"),
                Point("total", "uint32"),
            ],
        )
        self._total = 12456.0

    def read_point(self, point, tick):
        rate = max(0, 50 + 30 * math.sin(tick * 0.06) + random.gauss(0, 3))
        self._total += rate / 3600 * self.interval
        if point.name == "flow_rate":
            return round(rate, 2)
        return int(self._total)


# ---- 设备注册表 ----

ALL_DEVICES = {
    "temp_hum": TempHumSensor,
    "power": PowerMeter,
    "pressure": PressureSensor,
    "gas": GasDetector,
    "flow": FlowMeter,
}


def connect_with_retry(client, device_sn, broker_host, broker_port, stop):
    """带重试的连接，直到成功或收到停止信号"""
    delay = 1
    while not stop.is_set():
        try:
            client.connect(broker_host, broker_port, keepalive=60)
            return True
        except Exception as e:
            print(f"  [{device_sn}] Broker 未就绪，{delay}s 后重连... ({e})")
            if stop.wait(delay):
                return False
            delay = min(delay * 2, 30)
    return False


def run_device(device: SensorDevice, broker_host: str, broker_port: int, stop: threading.Event):
    """单设备采集 + 发布循环，支持断线重连"""
    status_topic = TOPIC_STATUS.format(device.sn)

    while not stop.is_set():
        client = mqtt.Client(mqtt.CallbackAPIVersion.VERSION2, client_id=device.sn)
        client.will_set(status_topic, json.dumps({"status": "offline", "timestamp": int(time.time() * 1000)}), qos=QOS_STATUS, retain=True)

        if not connect_with_retry(client, device.sn, broker_host, broker_port, stop):
            break

        client.loop_start()

        client.publish(status_topic, json.dumps({"status": "online", "timestamp": int(time.time() * 1000)}), qos=QOS_STATUS, retain=True)
        print(f"  [{device.sn}] 已上线")

        disconnected = threading.Event()

        def on_disconnect(cl, userdata, flags, rc, props):
            if rc != 0:
                print(f"  [{device.sn}] 连接断开 (rc={rc})，准备重连...")
            disconnected.set()

        client.on_disconnect = on_disconnect

        while not stop.is_set() and not disconnected.is_set():
            if not device.publish_readings(client):
                break
            stop.wait(device.interval)

        client.publish(status_topic, json.dumps({"status": "offline", "timestamp": int(time.time() * 1000)}), qos=QOS_STATUS, retain=True)
        client.loop_stop()
        client.disconnect()

        if not stop.is_set():
            print(f"  [{device.sn}] 等待 Broker 恢复...")
            stop.wait(3)

    print(f"  [{device.sn}] 已停止")


def parse_args():
    parser = argparse.ArgumentParser(description="LEPG MQTT 传感器模拟器")
    parser.add_argument("--broker", default=BROKER_DEFAULT, help=f"MQTT Broker 地址 (默认: {BROKER_DEFAULT})")
    parser.add_argument("--interval", type=float, default=0, help="覆盖所有设备的采集间隔(秒)，0=使用设备默认值")
    parser.add_argument("--devices", default="all", help=f"启用的设备，逗号分隔 (可选: {','.join(ALL_DEVICES)}, all)")
    return parser.parse_args()


def main():
    args = parse_args()

    host, _, port_str = args.broker.partition(":")
    broker_port = int(port_str) if port_str else 1883

    if args.devices == "all":
        selected = list(ALL_DEVICES.keys())
    else:
        selected = [d.strip() for d in args.devices.split(",")]

    unknown = set(selected) - set(ALL_DEVICES)
    if unknown:
        print(f"未知设备: {', '.join(unknown)}")
        print(f"可选: {', '.join(ALL_DEVICES)}")
        sys.exit(1)

    devices = []
    for key in selected:
        d = ALL_DEVICES[key]()
        if args.interval > 0:
            d.interval = args.interval
        devices.append(d)

    print("=" * 55)
    print("LEPG MQTT 传感器模拟器")
    print(f"Broker: {host}:{broker_port}")
    print("-" * 55)
    for d in devices:
        print(f"  {d.name:10s} [{d.sn}]  间隔 {d.interval}s  采集点: {', '.join(p.name for p in d.points)}")
    print("=" * 55)

    stop = threading.Event()

    def on_signal(sig, frame):
        print("\n正在停止所有设备...")
        stop.set()

    signal.signal(signal.SIGINT, on_signal)
    signal.signal(signal.SIGTERM, on_signal)

    threads = []
    for d in devices:
        t = threading.Thread(target=run_device, args=(d, host, broker_port, stop), daemon=True)
        t.start()
        threads.append(t)

    for t in threads:
        t.join()
    print("所有设备已停止")


if __name__ == "__main__":
    main()
