---
title: 客户端配置 (lepgc)
description: lepgc 的业务配置字段、连接重试、上传缓冲、Modbus 设备与 MQTT 虚拟设备配置及验证规则
type: reference
tags: [config, client, reference]
---

# 客户端配置 (lepgc)

## MetaConfig（元设置）

| 属性 | 值 |
|------|----|
| SearchPath | `config/client.toml` |
| InitPath | `/etc/lepgc/config.toml` |

## 业务配置字段

### 基本设置

| TOML 键 | 类型 | 默认值 | 必填 | 允许来源 | 说明 |
|---------|------|--------|------|---------|------|
| `server` | string | `"http://localhost"` | 否 | file, flag, env, default | 服务器地址，支持 `-u` 标志 |
| `port` | int | `8883` | 否 | file, flag, env, default | 服务器端口，支持 `-p` 标志 |
| `log_level` | string | `"info"` | 否 | file, env, default | 日志级别：`debug` / `info` / `warn` / `error` |
| `sn` | string | — | **是** | file, env | **凭据字段**，设备序列号，禁止通过 flag 传入 |
| `token` | string | — | **是** | file, env | **凭据字段**，认证令牌，禁止通过 flag 传入 |

### 连接与重试

| TOML 键 | 类型 | 默认值 | 必填 | 允许来源 | 说明 |
|---------|------|--------|------|---------|------|
| `max_retry` | int | `10` | 否 | file, env, default | 最大重连次数，>= 0 |
| `retry_interval` | int | `5000` | 否 | file, env, default | 重连间隔（毫秒），>= 0 |

### 上传与缓冲

| TOML 键 | 类型 | 默认值 | 必填 | 允许来源 | 说明 |
|---------|------|--------|------|---------|------|
| `buffer_size` | int | `1000` | 否 | file, env, default | 内部通道缓冲区大小，> 0 |
| `upload_batch_size` | int | `100` | 否 | file, env, default | 批量读取大小，> 0 |
| `upload_interval` | int | `5000` | 否 | file, env, default | 上传轮询间隔（毫秒），> 0 |

### 路径设置

| TOML 键 | 类型 | 默认值 | 必填 | 允许来源 | 说明 |
|---------|------|--------|------|---------|------|
| `log_path` | string | `"./logs/client.log"` | 否 | file, env, default | 日志文件路径 |
| `data_path` | string | `"./data/data.db"` | 否 | file, env, default | SQLite 本地数据库路径 |

### Modbus 设备列表（Unmarshal 填充）

通过 `[[devices]]` 定义 Modbus 采集设备。该字段无 `sources` 标签，由 `IUnmarshaler` 反序列化。详细字段说明见 [[modbus-config|Modbus 设备配置]]。

**设备级配置：**

```toml
[[devices]]
name = "sensor-1"           # 设备唯一标识（必填）
type = "tcp"                 # 连接类型：tcp / rtu（必填）
timeout = "5s"               # 请求超时（Go duration 格式）
offline_threshold = "30s"    # 离线判定阈值（Go duration 格式）
enable_monitor = true        # 是否启用健康监控
slave_id = 1                 # Modbus 从站地址，1-247（必填）
poll_interval = "1s"         # 轮询间隔（Go duration 格式）
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `name` | string | 是 | 设备唯一标识，全局不可重复 |
| `type` | string | 是 | 连接类型：`"tcp"` 或 `"rtu"` |
| `timeout` | duration | 否 | 请求超时，Go duration 字符串（如 `"5s"`、`"1000ms"`） |
| `offline_threshold` | duration | 否 | 离线检测阈值 |
| `enable_monitor` | bool | 否 | 启用健康监控，默认 `true` |
| `slave_id` | byte | 是 | Modbus 从站地址，范围 1-247 |
| `poll_interval` | duration | 否 | 轮询间隔 |

**TCP 连接配置**（`type = "tcp"` 时必填）：

```toml
[devices.tcp]
host = "127.0.0.1"
port = 5020
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `host` | string | 是 | 设备 IP 地址或主机名 |
| `port` | int | 是 | 设备端口（默认 502） |

**RTU 连接配置**（`type = "rtu"` 时必填）：

```toml
[devices.rtu]
port = "COM3"          # Windows: "COM3", Linux: "/dev/ttyUSB0"
baud_rate = 9600
data_bits = 8
parity = "N"           # "N" / "E" / "O"
stop_bits = 1
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `port` | string | 是 | 串口设备路径 |
| `baud_rate` | int | 是 | 波特率（如 9600、19200） |
| `data_bits` | int | 否 | 数据位，默认 8 |
| `parity` | string | 否 | 校验位：`"N"`（无） / `"E"`（偶） / `"O"`（奇），默认 `"N"` |
| `stop_bits` | int | 否 | 停止位，默认 1 |

**数据点配置**（`[[devices.points]]`）：

```toml
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
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `name` | string | 是 | 数据点标识（JSON 字段名） |
| `function_code` | int | 是 | Modbus 功能码：1, 2, 3, 4, 5, 6, 16 |
| `address` | uint16 | 是 | 寄存器起始地址（十进制，Go PDU 地址，0-based） |
| `quantity` | uint16 | 是 | 寄存器数量，> 0 |
| `data_type` | string | 是 | 数据类型（见下表） |
| `byte_order` | string | 条件必填 | 多寄存器类型（int32/uint32/float32）时必填 |
| `scale` | float64 | 否 | 缩放因子，默认 1.0 |
| `offset` | float64 | 否 | 偏移量，默认 0.0 |
| `unit` | string | 否 | 工程单位（如 `"°C"`、`"%"`、`"V"`） |
| `access` | string | 否 | 访问权限：`"ro"` / `"rw"` / `"wo"`，默认 `"ro"` |
| `cache_enabled` | bool | 否 | 启用本地缓存（断点续传），默认 `true` |

**数据类型取值：**

| 值 | 说明 | 寄存器数 |
|----|------|---------|
| `"bool"` | 布尔（线圈/离散输入） | 1 |
| `"int16"` | 16 位有符号整数 | 1 |
| `"uint16"` | 16 位无符号整数 | 1 |
| `"int32"` | 32 位有符号整数 | 2 |
| `"uint32"` | 32 位无符号整数 | 2 |
| `"float32"` | 32 位浮点数（IEEE 754） | 2 |
| `"float64"` | 64 位浮点数（IEEE 754） | 4 |
| `"json"` | JSON 数据 | 变长 |

**字节序取值（仅多寄存器类型需要）：**

| 值 | 名称 | 说明 |
|----|------|------|
| `"abcd"` | Big-Endian | 标准 Modbus 字节序 |
| `"dcba"` | Little-Endian | 反转字节序 |
| `"badc"` | Mid-Little Endian | 字节大端，字小端 |
| `"cdab"` | Mid-Big Endian | 字节小端，字大端 |

> **地址注意**：Go Modbus 库使用 PDU 地址（0-based），与 Python pymodbus 的寄存器地址（1-based）相差 1。转换公式：`Python 地址 = Go 地址 + 1`。

### MQTT 虚拟设备配置（Unmarshal 填充）

通过 `[mqtt]` 和 `[[mqtt.devices]]` 定义 MQTT 虚拟设备。该字段无 `sources` 标签，由 `IUnmarshaler` 反序列化。

**MQTT Broker 连接：**

```toml
[mqtt]
broker_addr = "127.0.0.1:1883"
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `broker_addr` | string | 是 | MQTT broker 地址 |

**MQTT 设备定义：**

```toml
[[mqtt.devices]]
name = "温湿度传感器"
client_id = "SENSOR-TH-001"
username = "***"               # 可选
password = "***"               # 可选
keep_alive = "60s"
clean_session = false
offline_threshold = "30s"
enable_monitor = true
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `name` | string | 是 | 设备名称，同组内不可重复 |
| `client_id` | string | 否 | MQTT Client ID |
| `username` | string | 否 | MQTT 认证用户名 |
| `password` | string | 否 | MQTT 认证密码 |
| `keep_alive` | duration | 否 | MQTT Keep Alive |
| `clean_session` | bool | 否 | 清除会话标志 |
| `offline_threshold` | duration | 否 | 离线检测阈值 |
| `enable_monitor` | bool | 否 | 启用健康监控 |

**MQTT 订阅主题（`[[mqtt.devices.topics]]`）：**

```toml
[[mqtt.devices.topics]]
topic = "device/SENSOR-TH-001/reading/temperature"
qos = 1
point_name = "temperature"
unit = "°C"
retain = false
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `topic` | string | 是 | 订阅主题，同设备内不可重复 |
| `qos` | byte | 否 | QoS 级别：0 / 1 / 2 |
| `point_name` | string | 是 | 数据点名称 |
| `unit` | string | 否 | 工程单位 |
| `retain` | bool | 否 | 是否保留消息 |

## 验证规则

`ClientConfig.Validate()` 在配置加载后自动执行，检查以下约束：

| 字段 | 规则 |
|------|------|
| `sn` | 必填，不能为空 |
| `token` | 必填，不能为空 |
| `port` | 必须在 1-65535 范围内 |
| `log_level` | 必须是 `debug`、`info`、`warn`、`error` 之一 |
| `max_retry` | >= 0 |
| `retry_interval` | >= 0 |
| `buffer_size` | > 0 |
| `upload_batch_size` | > 0 |
| `upload_interval` | > 0 |
| `data_path` | 不能为空 |
| `devices[].name` | 全局唯一 |
| `devices[]` | 每个设备独立验证（见下） |

**Modbus 设备验证：**

| 字段 | 规则 |
|------|------|
| `name` | 不能为空 |
| `type` | 必须是 `"rtu"` 或 `"tcp"` |
| `rtu` | `type = "rtu"` 时必须提供 |
| `tcp` | `type = "tcp"` 时必须提供 |
| `slave_id` | 范围 1-247 |
| `points` | 至少一个数据点 |

**Modbus 数据点验证：**

| 字段 | 规则 |
|------|------|
| `name` | 不能为空 |
| `function_code` | 必须是 1, 2, 3, 4, 5, 6, 16 之一 |
| `access` + `function_code` | `ro` 不能与写功能码（5, 6, 16）组合 |
| `data_type` | 必须是 `bool`、`int16`、`uint16`、`int32`、`uint32`、`float32` 之一 |
| `byte_order` | 多寄存器类型时必填，单寄存器时忽略 |
| `quantity` | > 0 |

**MQTT 配置验证：**

| 字段 | 规则 |
|------|------|
| `mqtt.broker_addr` | 配置了 `[mqtt]` 时不能为空 |
| `mqtt.devices` | 配置了 `[mqtt]` 时至少一个设备 |
| `mqtt.devices[].name` | 同组内唯一 |
| `mqtt.devices[].topics` | 至少一个主题 |
| `mqtt.devices[].topics[].topic` | 同设备内唯一，不能为空 |
| `mqtt.devices[].topics[].point_name` | 不能为空 |
| `mqtt.devices[].topics[].qos` | 0, 1 或 2 |

---

## 相关笔记

- [[overview|配置系统总览]]
- [[server-config|服务器配置]]
- [[modbus-config|Modbus 设备配置]]
- [[config-examples|完整配置示例]]
- [[cli-and-env|CLI 命令与环境变量]]
- [[sensitive-fields|敏感字段处理]]
- [[config-migration|迁移指南]]
- [[config-dev-guide|开发者指南]]
- [[config-internals|内部机制]]
