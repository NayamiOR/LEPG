---
title: 完整配置示例
description: 服务器与客户端的完整 TOML 配置示例，含 Modbus 与 MQTT 虚拟设备
type: reference
tags: [config, example, reference]
---

# 完整配置示例

## 服务器配置

```toml
# ==============================
# LEPG 服务器配置 (lepgs)
# ==============================

# 日志级别：debug / info / warn / error
log_level = "info"

# TCP 监听端口
port = 8883

# SQLite 数据库文件路径
data_path = "/var/cache/lepgs/lepgs.db"

# ---- MQTT Broker 配置 ----
mqtt_tcp = "127.0.0.1:1883"     # TCP 监听地址
mqtt_ws = "127.0.0.1:8083"      # WebSocket 监听地址

# ---- Redis 配置（预留） ----
# redis_addr = "127.0.0.1:6379"
# redis_password = "***"          # 敏感字段，无默认值
# redis_db = 0

# ---- 客户端列表 ----
# 定义允许连接到服务器的客户端
[[clients]]
sn = "CLIENT001"
token = "***"
description = "测试客户端1"

[[clients]]
sn = "CLIENT002"
token = "***"
description = "测试客户端2"
```

## 客户端配置

```toml
# ==============================
# LEPG 客户端配置 (lepgc)
# ==============================

# ---- 服务器连接 ----
server = "127.0.0.1"            # 服务器地址
port = 8883                     # 服务器端口
sn = "CLIENT001"                # 设备序列号（必填，凭据）
token = "***"                   # 认证令牌（必填，凭据）

# ---- 日志 ----
log_level = "info"              # debug / info / warn / error

# ---- 连接与重试 ----
max_retry = 10                  # 最大重连次数（0 = 无限重试需自行实现循环）
retry_interval = 5000           # 重连间隔（毫秒）

# ---- 上传与缓冲 ----
buffer_size = 1000              # 内部通道缓冲区大小
upload_batch_size = 100         # 批量读取大小
upload_interval = 5000          # 上传轮询间隔（毫秒）

# ---- 文件路径 ----
# log_path = "./logs/client.log"
# data_path = "./data/data.db"

# ================================
# Modbus 设备配置
# ================================

# TCP 设备示例
[[devices]]
name = "sensor-1"
type = "tcp"
timeout = "5s"
offline_threshold = "30s"
enable_monitor = true
slave_id = 1
poll_interval = "1s"

[devices.tcp]
host = "127.0.0.1"
port = 5020

[[devices.points]]
name = "temperature"
function_code = 3
address = 0
quantity = 1
data_type = "int16"
byte_order = "abcd"
scale = 0.1
offset = 0
unit = "°C"
access = "ro"
cache_enabled = true

[[devices.points]]
name = "humidity"
function_code = 3
address = 1
quantity = 1
data_type = "uint16"
byte_order = "abcd"
scale = 0.1
offset = 0
unit = "%"
access = "ro"

# RTU 设备示例（注释掉，按需启用）
# [[devices]]
# name = "rtu-device"
# type = "rtu"
# timeout = "5s"
# slave_id = 1
# poll_interval = "5s"
#
# [devices.rtu]
# port = "COM3"
# baud_rate = 9600
# data_bits = 8
# parity = "N"
# stop_bits = 1

# ================================
# MQTT 虚拟设备配置
# ================================

[mqtt]
broker_addr = "127.0.0.1:1883"

[[mqtt.devices]]
name = "温湿度传感器"
client_id = "SENSOR-TH-001"
keep_alive = "60s"
offline_threshold = "30s"
enable_monitor = true

[[mqtt.devices.topics]]
topic = "device/SENSOR-TH-001/reading/temperature"
qos = 1
point_name = "temperature"
unit = "°C"

[[mqtt.devices.topics]]
topic = "device/SENSOR-TH-001/reading/humidity"
qos = 1
point_name = "humidity"
unit = "%RH"
```

---

## 相关笔记

- [[overview|配置系统总览]]
- [[server-config|服务器配置]]
- [[client-config|客户端配置]]
- [[modbus-config|Modbus 设备配置]]
- [[cli-and-env|CLI 命令与环境变量]]
- [[sensitive-fields|敏感字段处理]]
- [[config-migration|迁移指南]]
- [[config-dev-guide|开发者指南]]
- [[config-internals|内部机制]]
