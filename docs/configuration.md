# LEPG 配置参考手册

> 本文档替代旧版 `docs/Config.md`，反映配置系统重构后的最新架构。
>
> 面向两类读者：**运维人员**（部署、调参）和 **开发人员**（扩展配置项、理解内部机制）。

---

## 目录

- [1. 概述](#1-概述)
- [2. 配置来源与优先级](#2-配置来源与优先级)
- [3. 服务器配置 (lepgs)](#3-服务器配置-lepgs)
- [4. 客户端配置 (lepgc)](#4-客户端配置-lepgc)
- [5. 完整 TOML 示例](#5-完整-toml-示例)
- [6. CLI 命令参考](#6-cli-命令参考)
- [7. 环境变量参考](#7-环境变量参考)
- [8. 敏感字段处理](#8-敏感字段处理)
- [9. 迁移指南（旧系统 → 新系统）](#9-迁移指南旧系统--新系统)
- [10. 开发者指南：添加新配置项](#10-开发者指南添加新配置项)
- [11. 内部机制](#11-内部机制)

---

## 1. 概述

LEPG 的配置系统采用 **Provider Chain（提供者链）** 模式，通过依赖注入组装多个配置来源，避免全局单例状态。

核心设计原则：

- **多层来源**：默认值 < 环境变量 < 配置文件 < 命令行参数，优先级由低到高。
- **严格白名单**：每个配置字段通过 `sources` 标签显式声明允许的来源，未声明的来源不会读取该字段。
- **两类字段**：简单字段（`string`/`int`/`bool`）通过 Provider Chain 逐项填充；复杂嵌套结构（数组、对象）通过 `Unmarshal` 一次性反序列化。
- **元设置与业务设置分离**：`MetaConfig`（搜索路径、初始化路径）在 Provider Chain 构建之前即已确定，且运行期间不再变更。

配置文件格式为 **TOML**。服务器默认配置文件为 `config/server.toml`，客户端默认配置文件为 `config/client.toml`。

---

## 2. 配置来源与优先级

Provider Chain 按以下顺序组装，**编号越大优先级越高**：

| 优先级 | 来源 | 实现类 | 说明 |
|--------|------|--------|------|
| 0（最低） | 默认值 | `DefaultProvider` | 从结构体 `default` 标签提取，由 `ExtractDefaults` 自动生成 |
| 1 | 环境变量 | `EnvProvider` | 无前缀，键名小写匹配。例如 `PORT=9999` 对应键 `port` |
| 2 | 配置文件 | `FileProvider` | 支持 TOML 格式 + `.env` 文件合并 |
| 3（最高） | 命令行参数 | `FlagProvider` | 仅限特定字段（见 CLI 参考） |

解析时，系统反向遍历 Provider Chain（从高到低），命中第一个有效值即停止。每个字段的 `sources` 标签控制允许参与解析的 Provider 子集。

### 2.1 环境变量规则

- **无前缀**：`EnvProvider` 初始化时 `prefix` 为空字符串。
- **键名转换**：环境变量名小写化后直接作为配置键。例如 `LOG_LEVEL=debug` 对应键 `log_level`，`REDIS_ADDR=localhost:6379` 对应键 `redis_addr`。
- **类型推断**：字符串值按需转换为 `int`（`strconv.Atoi`）或 `bool`（`strconv.ParseBool`）。

### 2.2 配置文件定位顺序

运行时按以下逻辑确定配置文件路径：

1. 若通过 `--config/-c` 指定了路径 → 使用该路径。
2. 否则 → 使用 `MetaConfig.SearchPath`（服务器：`config/server.toml`，客户端：`config/client.toml`）。

配置文件不存在时不会报错（静默忽略），字段将回退到更低优先级的来源。

### 2.3 `.env` 文件

`FileProvider` 在加载 TOML 配置文件后，会尝试加载工作目录下的 `.env` 文件。`.env` 文件中的值会合并到 FileProvider 中，优先级等同于 TOML 文件。`.env` 文件不存在时不报错。

---

## 3. 服务器配置 (lepgs)

### 3.1 MetaConfig（元设置）

| 属性 | 值 |
|------|----|
| SearchPath | `config/server.toml` |
| InitPath | `/etc/lepgs/config.toml` |

- **SearchPath**：运行时未指定 `--config` 时的默认搜索路径。
- **InitPath**：`init` 命令默认写入的配置文件路径。

### 3.2 业务配置字段

#### 基本设置

| TOML 键 | 类型 | 默认值 | 必填 | 允许来源 | 说明 |
|---------|------|--------|------|---------|------|
| `port` | int | `8883` | 否 | file, flag, env, default | TCP 监听端口，范围 1-65535 |
| `log_level` | string | `"info"` | 否 | file, env, default | 日志级别：`debug` / `info` / `warn` / `error` |
| `data_path` | string | `"/var/cache/lepgs/lepgs.db"` | 否 | file, env, default | SQLite 数据库文件路径 |

#### MQTT Broker 设置

| TOML 键 | 类型 | 默认值 | 必填 | 允许来源 | 说明 |
|---------|------|--------|------|---------|------|
| `mqtt_tcp` | string | `"127.0.0.1:1883"` | 否 | file, env, default | MQTT broker TCP 监听地址 |
| `mqtt_ws` | string | `"127.0.0.1:8083"` | 否 | file, env, default | MQTT broker WebSocket 监听地址 |

#### Redis 设置（预留）

| TOML 键 | 类型 | 默认值 | 必填 | 允许来源 | 说明 |
|---------|------|--------|------|---------|------|
| `redis_addr` | string | `"127.0.0.1:6379"` | 否 | file, env, default | Redis 连接地址 |
| `redis_password` | string | — | 否 | file, env | **敏感字段**，无默认值，`init` 命令不会生成此键 |
| `redis_db` | int | `0` | 否 | file, env, default | Redis 数据库索引 |

#### 客户端列表（Unmarshal 填充）

通过 `[[clients]]` 定义允许连接到服务器的客户端。该字段无 `sources` 标签，不经过 Provider Chain，由 `IUnmarshaler` 直接反序列化。

```toml
[[clients]]
sn = "CLIENT001"
token = "***"
description = "测试客户端1"
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `sn` | string | 是 | 客户端设备序列号 |
| `token` | string | 是 | 客户端认证令牌（**敏感**） |
| `description` | string | 否 | 客户端描述 |

### 3.3 验证规则

`ServerConfig.Validate()` 在配置加载后自动执行，检查以下约束：

| 字段 | 规则 |
|------|------|
| `port` | 必须在 1-65535 范围内 |
| `log_level` | 必须是 `debug`、`info`、`warn`、`error` 之一 |
| `data_path` | 不能为空 |

---

## 4. 客户端配置 (lepgc)

### 4.1 MetaConfig（元设置）

| 属性 | 值 |
|------|----|
| SearchPath | `config/client.toml` |
| InitPath | `/etc/lepgc/config.toml` |

### 4.2 业务配置字段

#### 基本设置

| TOML 键 | 类型 | 默认值 | 必填 | 允许来源 | 说明 |
|---------|------|--------|------|---------|------|
| `server` | string | `"http://localhost"` | 否 | file, flag, env, default | 服务器地址，支持 `-u` 标志 |
| `port` | int | `8883` | 否 | file, flag, env, default | 服务器端口，支持 `-p` 标志 |
| `log_level` | string | `"info"` | 否 | file, env, default | 日志级别：`debug` / `info` / `warn` / `error` |
| `sn` | string | — | **是** | file, env | **凭据字段**，设备序列号，禁止通过 flag 传入 |
| `token` | string | — | **是** | file, env | **凭据字段**，认证令牌，禁止通过 flag 传入 |

#### 连接与重试

| TOML 键 | 类型 | 默认值 | 必填 | 允许来源 | 说明 |
|---------|------|--------|------|---------|------|
| `max_retry` | int | `10` | 否 | file, env, default | 最大重连次数，>= 0 |
| `retry_interval` | int | `5000` | 否 | file, env, default | 重连间隔（毫秒），>= 0 |

#### 上传与缓冲

| TOML 键 | 类型 | 默认值 | 必填 | 允许来源 | 说明 |
|---------|------|--------|------|---------|------|
| `buffer_size` | int | `1000` | 否 | file, env, default | 内部通道缓冲区大小，> 0 |
| `upload_batch_size` | int | `100` | 否 | file, env, default | 批量读取大小，> 0 |
| `upload_interval` | int | `5000` | 否 | file, env, default | 上传轮询间隔（毫秒），> 0 |

#### 路径设置

| TOML 键 | 类型 | 默认值 | 必填 | 允许来源 | 说明 |
|---------|------|--------|------|---------|------|
| `log_path` | string | `"./logs/client.log"` | 否 | file, env, default | 日志文件路径 |
| `data_path` | string | `"./data/data.db"` | 否 | file, env, default | SQLite 本地数据库路径 |

#### Modbus 设备列表（Unmarshal 填充）

通过 `[[devices]]` 定义 Modbus 采集设备。该字段无 `sources` 标签，由 `IUnmarshaler` 反序列化。

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

#### MQTT 虚拟设备配置（Unmarshal 填充）

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

### 4.3 验证规则

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

## 5. 完整 TOML 示例

### 5.1 服务器配置

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

### 5.2 客户端配置

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

## 6. CLI 命令参考

### 6.1 服务器 (lepgs)

```
lepgs [command]
```

| 命令 | 说明 |
|------|------|
| `run` | 启动服务器 |
| `init` | 初始化配置文件 |
| `help` | 帮助信息 |

**全局标志：**

| 标志 | 短标志 | 默认值 | 说明 |
|------|--------|--------|------|
| `--config` | `-c` | — | 配置文件路径（默认搜索 `config/server.toml`） |

**`run` 命令标志：**

| 标志 | 短标志 | 默认值 | 说明 |
|------|--------|--------|------|
| `--port` | `-p` | `0`（不覆盖） | 覆盖 TCP 监听端口 |

**`init` 命令：**

默认写入路径为 `/etc/lepgs/config.toml`。可通过 `-c` 指定其他路径。文件已存在时会拒绝覆盖。

### 6.2 客户端 (lepgc)

```
lepgc [command]
```

| 命令 | 说明 |
|------|------|
| `run` | 启动客户端 |
| `init` | 初始化配置文件 |
| `help` | 帮助信息 |

**全局标志：**

| 标志 | 短标志 | 默认值 | 说明 |
|------|--------|--------|------|
| `--config` | `-c` | — | 配置文件路径（默认搜索 `config/client.toml`） |

**`run` 命令标志：**

| 标志 | 短标志 | 默认值 | 说明 |
|------|--------|--------|------|
| `--url` | `-u` | —（不覆盖） | 覆盖服务器地址 |
| `--port` | `-p` | `0`（不覆盖） | 覆盖服务器端口 |

**`init` 命令：**

默认写入路径为 `/etc/lepgc/config.toml`。可通过 `-c` 指定其他路径。文件已存在时会拒绝覆盖。

---

## 7. 环境变量参考

环境变量无前缀，变量名小写后直接映射为配置键。

### 7.1 服务器环境变量

| 环境变量 | 对应键 | 类型 | 示例 |
|---------|--------|------|------|
| `PORT` | `port` | int | `PORT=9999` |
| `LOG_LEVEL` | `log_level` | string | `LOG_LEVEL=debug` |
| `DATA_PATH` | `data_path` | string | `DATA_PATH=/data/lepgs.db` |
| `MQTT_TCP` | `mqtt_tcp` | string | `MQTT_TCP=0.0.0.0:1883` |
| `MQTT_WS` | `mqtt_ws` | string | `MQTT_WS=0.0.0.0:8083` |
| `REDIS_ADDR` | `redis_addr` | string | `REDIS_ADDR=redis:6379` |
| `REDIS_PASSWORD` | `redis_password` | string | `REDIS_PASSWORD=***` |
| `REDIS_DB` | `redis_db` | int | `REDIS_DB=1` |

> 注意：`[[clients]]` 数组无法通过环境变量配置，只能通过 TOML 文件。

### 7.2 客户端环境变量

| 环境变量 | 对应键 | 类型 | 示例 |
|---------|--------|------|------|
| `SERVER` | `server` | string | `SERVER=192.168.1.100` |
| `PORT` | `port` | int | `PORT=9999` |
| `LOG_LEVEL` | `log_level` | string | `LOG_LEVEL=debug` |
| `SN` | `sn` | string | `SN=DEVICE-001` |
| `TOKEN` | `token` | string | `TOKEN=***` |
| `MAX_RETRY` | `max_retry` | int | `MAX_RETRY=20` |
| `RETRY_INTERVAL` | `retry_interval` | int | `RETRY_INTERVAL=10000` |
| `BUFFER_SIZE` | `buffer_size` | int | `BUFFER_SIZE=2000` |
| `UPLOAD_BATCH_SIZE` | `upload_batch_size` | int | `UPLOAD_BATCH_SIZE=50` |
| `UPLOAD_INTERVAL` | `upload_interval` | int | `UPLOAD_INTERVAL=3000` |
| `LOG_PATH` | `log_path` | string | `LOG_PATH=/var/log/lepgc.log` |
| `DATA_PATH` | `data_path` | string | `DATA_PATH=/data/lepgc.db` |

> 注意：`[[devices]]`、`[mqtt]` 及其子项无法通过环境变量配置，只能通过 TOML 文件。

---

## 8. 敏感字段处理

以下字段包含敏感信息，文档和示例中使用 `***` 表示：

| 字段 | 所在配置 | 允许来源 | 安全说明 |
|------|---------|---------|---------|
| `token`（客户端） | `ClientConfig` | file, env | 禁止通过 CLI flag 传入，避免进程参数泄露 |
| `sn`（客户端） | `ClientConfig` | file, env | 同上 |
| `token`（`[[clients]]`） | `ClientDef` | file only | 仅限 TOML 文件 |
| `redis_password` | `RedisConfig` | file, env | 无默认值，`init` 命令不会生成此键 |
| `username`（MQTT 设备） | `MQTTDeviceConfig` | file only | 仅限 TOML 文件 |
| `password`（MQTT 设备） | `MQTTDeviceConfig` | file only | 仅限 TOML 文件 |

**安全建议：**

- 不要在命令行参数中传递凭据（`sn`、`token` 已在设计上禁止 flag 来源）。
- 生产环境中，优先使用环境变量或受限权限的配置文件。
- 不要将含真实凭据的配置文件提交到版本控制。

---

## 9. 迁移指南（旧系统 → 新系统）

以下是配置系统重构后的主要破坏性变更。

### 9.1 常量与映射变更

| 旧系统 | 新系统 | 影响 |
|--------|--------|------|
| `DefaultConfigFile` 常量 | `ServerMetaConfig.SearchPath` / `ClientMetaConfig.SearchPath` | 配置文件默认路径逻辑不变，但实现方式改变 |
| `defaultServerValues` / `defaultClientValues` 硬编码 map | 结构体 `default` 标签 + `ExtractDefaults` 自动提取 | 默认值现在与结构体定义同位，更易维护 |
| `config_path` 在 defaults map 中运行时使用 | `config_path` 仅在 `GetDefaultValues()` 中为 `init` 命令注入 | 运行时不再有 `config_path` 键 |
| `PathsConfig.ConfigPath` 字段 | 已移除（原为死代码） | 无功能影响 |

### 9.2 API 变更

| 旧系统 | 新系统 |
|--------|--------|
| `NewProviders("server")` — 传配置名（无扩展名） | `NewProviders(flagValues, cfgFile)` — `cfgFile` 为完整文件路径，默认路径由 `MetaConfig.SearchPath` 控制 |
| 服务器无 `Validate()` | 服务器新增 `Validate()`，启动时自动校验配置 |

### 9.3 行为变更

| 变更项 | 说明 |
|--------|------|
| `DataPath` 可配置 | 服务器端 `data_path` 现在可通过 Provider Chain 设置（原为硬编码） |
| `sources` 白名单机制 | 每个字段显式声明允许的来源，未声明的不参与解析 |
| 敏感字段保护 | `sn`、`token` 禁止通过 CLI flag 传入；`redis_password` 无默认值且 `init` 不生成 |

### 9.4 配置文件兼容性

TOML 配置文件的键名 **完全不变**，现有配置文件无需修改即可在新系统上使用。

---

## 10. 开发者指南：添加新配置项

添加一个新的配置字段只需以下步骤：

### 步骤 1：在结构体中添加字段

在 `internal/server/config.go`（`ServerConfig`）或 `internal/client/config.go`（`ClientConfig` / 子结构体）中添加字段，携带 `config`、`default`、`sources` 三个标签：

```go
// 简单字段示例
MaxConnections int `config:"max_connections" default:"100" sources:"file,env,default"`

// 敏感字段（无 default，限制来源）
ApiKey string `config:"api_key" sources:"file,env"`
```

标签说明：

| 标签 | 必填 | 说明 |
|------|------|------|
| `config` | 是（有 `sources` 时） | TOML 键名 / Provider 查找键 |
| `default` | 否 | 默认值字符串，不设则无默认值 |
| `sources` | 是（参与 Provider Chain 的字段） | 允许的来源列表，逗号分隔：`file,env,flag,default` |

### 步骤 2：添加验证（如需要）

在对应的 `Validate()` 方法中添加校验逻辑：

```go
if c.MaxConnections <= 0 {
    errs = append(errs, errors.NewConfigInvalidError("max_connections", "must be positive"))
}
```

### 步骤 3：完成

`PopulateFromProvider` 和 `ExtractDefaults` 通过反射自动处理新字段，无需修改任何框架代码。

如果新字段是复杂嵌套结构（数组、对象），则需要：
1. 不加 `sources` 标签（不参与 Provider Chain 逐项填充）。
2. 使用 `mapstructure` 标签定义 TOML 映射。
3. 在 `InitXxxConfig` 中通过 `IUnmarshaler` 反序列化。
4. 在 `Validate()` 中添加验证。

---

## 11. 内部机制

本节供开发人员深入理解配置系统内部工作原理。

### 11.1 文件结构

```
internal/config/
├── config.go          # NewProviders 工厂，版本信息
├── populate.go        # PopulateFromProvider, ExtractDefaults, Validatable
├── populate_test.go   # 反射工具测试
├── provider.go        # IProvider, IUnmarshaler, ProviderChain
└── provider/
    ├── default.go     # DefaultProvider（map 存储）
    ├── file.go        # FileProvider（基于 viper，TOML + .env）
    ├── env.go         # EnvProvider（无前缀，小写映射）
    └── flag.go        # FlagProvider（map 存储，由 main.go 注入）

internal/server/
├── config.go          # ServerMetaConfig, ServerConfig, Validate, GetDefaultValues
└── config_test.go     # Validate 测试

internal/client/
├── config.go          # ClientMetaConfig, ClientConfig, Validate, 设备/MQTT 子结构体
└── config_validate_test.go  # Validate 测试
```

### 11.2 PopulateFromProvider 工作流程

```
输入：指向配置结构体的指针 + ProviderChain

1. 反射遍历结构体字段
2. 对每个字段：
   a. 无 sources 标签 → 跳过（但如果是 struct 且子字段有 config 标签 → 递归）
   b. 有 sources 标签但无 config 标签 → 报错
   c. 解析 sources 为 Source bitmask
   d. 反向遍历 ProviderChain（index len-1 → 0，高→低优先级）
   e. 跳过不在白名单的 Provider（sources & sourceOf(provider) == 0）
   f. 命中第一个 IsSet(key) 的 Provider → 设置值并返回
   g. 所有 Provider 未命中 → 尝试 default 标签值
3. 支持 string / int / bool 三种类型
```

### 11.3 ExtractDefaults 工作流程

```
输入：一个或多个指向配置结构体的指针

1. 反射遍历结构体字段
2. 同时有 config 和 default 标签（且 default 非空）→ 加入结果 map
3. Kind == Struct → 递归提取子字段默认值
4. 返回 flat map[string]any

用途：
  - 为 DefaultProvider 提供初始数据
  - 为 init 命令生成模板配置文件
```

### 11.4 ProviderChain 解析顺序

`ProviderChain` 内部 `providers` 切片存储顺序为 `[Default, Env, File, Flag]`（低→高）。

所有 `GetXxx(key)` 方法从切片末尾开始向前查找（高→低优先级），命中第一个有效值即返回。

`PopulateFromProvider` 也采用相同的反向遍历策略，但额外受 `sources` 白名单约束。

### 11.5 IUnmarshaler 接口

```go
type IUnmarshaler interface {
    Unmarshal(rawVal any) error
}
```

仅 `FileProvider` 实现此接口。`ProviderChain` 将 `Unmarshal` 调用委托给内部的 `FileProvider`。

用途：解析 TOML 中的复杂嵌套结构（`[[clients]]`、`[[devices]]`、`[mqtt]` 等），这些结构不适合通过 Provider Chain 逐项填充。

在 `InitServerConfig` / `InitClientConfig` 中通过类型断言获取：

```go
if u, ok := provider.(config.IUnmarshaler); ok {
    var wrapper struct {
        Clients []ClientDef `mapstructure:"clients"`
    }
    if err := u.Unmarshal(&wrapper); err != nil {
        return nil, errors.Wrap(err, "failed to unmarshal clients")
    }
    cfg.Clients = wrapper.Clients
}
```

### 11.6 init 命令工作流程

1. 调用 `GetDefaultValues()` 获取默认值 map（由 `ExtractDefaults` 从结构体标签自动提取）。
2. 额外注入 `config_path` 键（值为 `MetaConfig.InitPath`）。
3. 若指定了 `-c` 标志，覆盖写入路径。
4. 检查目标文件是否已存在（防覆盖）。
5. 创建目录（如需要）并使用 `viper.SafeWriteConfigAs` 写入。
