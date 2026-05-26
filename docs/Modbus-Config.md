# Modbus 设备配置

## 概述

LEPG Client 支持通过 Modbus RTU/TCP 协议连接工业设备，进行数据采集和远程控制。配置采用结构化 TOML 格式，支持多设备、多数据点的灵活配置。

## 配置文件位置

Modbus 设备配置位于 Client 配置文件中：

```
config/client.toml
```

## 配置结构

```toml
[[devices]]
name              = "设备名称"
type              = "rtu" | "tcp"
poll_interval     = "10s"
timeout           = "5s"
offline_threshold = "30s"
enable_monitor    = true

# RTU 专属（type = "rtu" 时填写）
[devices.rtu]
port        = "/dev/ttyS0" | "COM3"
baud_rate   = 9600
data_bits   = 8
parity      = "N" | "E" | "O"
stop_bits   = 1

# TCP 专属（type = "tcp" 时填写）
[devices.tcp]
host        = "192.168.1.10"
port        = 502

# 通用字段
slave_id = 1

# 数据点配置
[[devices.points]]
name          = "数据点名称"
function_code = 3
address       = 1
quantity      = 1
data_type     = "int16"
scale         = 0.1
unit          = "°C"
access        = "ro" | "rw" | "wo"
cache_enabled = true
```

## 设备级别配置

### 基础参数

| 字段            | 类型     | 必填 | 默认值 | 说明                                              |
| --------------- | -------- | ---- | ------ | ------------------------------------------------- |
| `name`          | string   | ✓    | -      | 设备唯一标识名，用于日志和数据输出的 device 字段  |
| `type`          | string   | ✓    | -      | 连接类型：`"rtu"` 或 `"tcp"`                      |
| `poll_interval` | duration | ✓    | -      | 轮询间隔，支持 `s`/`m` 后缀（如 `"10s"`, `"1m"`） |
| `timeout`       | duration | ✓    | `"5s"` | 单次 Modbus 请求超时时间                          |
| `slave_id`      | int      | ✓    | -      | Modbus 从机地址（1-247）                          |

### 健康监控参数

| 字段                | 类型     | 必填 | 默认值  | 说明                     |
| ------------------- | -------- | ---- | ------- | ------------------------ |
| `offline_threshold` | duration | ✗    | `"30s"` | 连续失败多久判定设备离线 |
| `enable_monitor`    | bool     | ✗    | `true`  | 是否启用健康监控和告警   |

### RTU 专属参数

当 `type = "rtu"` 时必须配置 `[devices.rtu]` 块：

| 字段        | 类型   | 必填 | 默认值 | 说明                                                                     |
| ----------- | ------ | ---- | ------ | ------------------------------------------------------------------------ |
| `port`      | string | ✓    | -      | 串口号<br>Linux: `/dev/ttyS0`, `/dev/ttyUSB0`<br>Windows: `COM3`, `COM4` |
| `baud_rate` | int    | ✓    | -      | 波特率（常用：9600, 19200, 38400）                                       |
| `data_bits` | int    | ✗    | `8`    | 数据位（通常为 8）                                                       |
| `parity`    | string | ✗    | `"N"`  | 校验位：<br>`"N"`: 无校验<br>`"E"`: 偶校验<br>`"O"`: 奇校验              |
| `stop_bits` | int    | ✗    | `1`    | 停止位（通常为 1）                                                       |

### TCP 专属参数

当 `type = "tcp"` 时必须配置 `[devices.tcp]` 块：

| 字段   | 类型   | 必填 | 默认值 | 说明                        |
| ------ | ------ | ---- | ------ | --------------------------- |
| `host` | string | ✓    | -      | IP 地址或主机名             |
| `port` | int    | ✗    | `502`  | Modbus TCP 端口（默认 502） |

## 数据点级别配置

每个设备可定义多个数据点（`[[devices.points]]`），每个点独立配置。

### 基础参数

| 字段            | 类型   | 必填 | 默认值 | 说明                                                         |
| --------------- | ------ | ---- | ------ | ------------------------------------------------------------ |
| `name`          | string | ✓    | -      | 数据点名称，作为输出 JSON 的字段名                           |
| `function_code` | int    | ✓    | -      | Modbus 功能码（见下表）                                      |
| `address`       | int    | ✓    | -      | 寄存器起始地址（十进制）                                     |
| `quantity`      | int    | ✓    | -      | 寄存器数量（见下文数据类型说明）                             |
| `data_type`     | string | ✓    | -      | 数据类型：`bool`/`int16`/`uint16`/`int32`/`uint32`/`float32` |

### 功能码（Function Code）

| 功能码 | 名称                     | 说明         | 适用数据类型                      | access |
| ------ | ------------------------ | ------------ | --------------------------------- | ------ |
| 1      | Read Coils               | 读线圈       | bool                              | ro     |
| 2      | Read Discrete Inputs     | 读离散输入   | bool                              | ro     |
| 3      | Read Holding Registers   | 读保持寄存器 | int16/uint16/int32/uint32/float32 | ro     |
| 4      | Read Input Registers     | 读输入寄存器 | int16/uint16/int32/uint32/float32 | ro     |
| 5      | Write Single Coil        | 写单个线圈   | bool                              | wo/wo  |
| 6      | Write Single Register    | 写单个寄存器 | int16/uint16                      | wo/wo  |
| 16     | Write Multiple Registers | 写多个寄存器 | int16/uint16/int32/uint32/float32 | wo/wo  |

### 数据类型与寄存器数量

| `data_type` | 说明                    | `quantity` 值 | 寄存器占用 |
| ----------- | ----------------------- | ------------- | ---------- |
| `bool`      | 布尔值（线圈/离散输入） | 1             | 1 bit      |
| `int16`     | 16位有符号整数          | 1             | 1 寄存器   |
| `uint16`    | 16位无符号整数          | 1             | 1 寄存器   |
| `int32`     | 32位有符号整数          | 2             | 2 寄存器   |
| `uint32`    | 32位无符号整数          | 2             | 2 寄存器   |
| `float32`   | 32位浮点数（IEEE 754）  | 2             | 2 寄存器   |

**注意**：32位类型（int32/uint32/float32）必须设置 `quantity = 2`。

### 数据转换参数

| 字段    | 类型   | 必填 | 默认值 | 说明                                             |
| ------- | ------ | ---- | ------ | ------------------------------------------------ |
| `scale` | float  | ✗    | `1.0`  | 换算系数，原始值乘以该系数得到工程值             |
| `unit`  | string | ✗    | -      | 工程单位，如 `"°C"`, `"%"`, `"V"`, `"A"`, `"kW"` |

**示例**：
- 设备返回值：`250`，`scale = 0.1` → 工程值 `25.0`
- 设备返回值：`1000`，`scale = 0.01` → 工程值 `10.0`

### 访问权限

| 字段     | 类型   | 必填 | 默认值 | 说明                                                       |
| -------- | ------ | ---- | ------ | ---------------------------------------------------------- |
| `access` | string | ✗    | `"ro"` | 访问权限：<br>`"ro"`: 只读<br>`"rw"`: 读写<br>`"wo"`: 只写 |

**权限与功能码兼容性**：
- `"ro"`：仅支持功能码 1/2/3/4（读操作）
- `"rw"`：支持所有功能码（可读可写）
- `"wo"`：仅支持功能码 5/6/16（写操作）

### 持久化缓存

| 字段            | 类型 | 必填 | 默认值 | 说明                         |
| --------------- | ---- | ---- | ------ | ---------------------------- |
| `cache_enabled` | bool | ✗    | `true` | 是否启用本地缓存（断点续传） |

- `true`：网络断开时数据缓存到 SQLite，恢复后续传
- `false`：实时数据，不缓存，失败即丢弃

## 配置示例

### RTU 温湿度传感器

```toml
[[devices]]
name          = "温湿度传感器"
type          = "rtu"
poll_interval = "10s"
timeout       = "5s"
offline_threshold = "30s"
enable_monitor    = true

[devices.rtu]
port        = "/dev/ttyS0"
baud_rate   = 9600
data_bits   = 8
parity      = "N"
stop_bits   = 1

slave_id = 1

[[devices.points]]
name          = "temperature"
function_code = 3
address       = 1
quantity      = 1
data_type     = "int16"
scale         = 0.1
unit          = "°C"
access        = "ro"
cache_enabled = true

[[devices.points]]
name          = "humidity"
function_code = 3
address       = 2
quantity      = 1
data_type     = "int16"
scale         = 0.1
unit          = "%"
access        = "ro"
cache_enabled = true
```

### TCP 电表

```toml
[[devices]]
name          = "智能电表"
type          = "tcp"
poll_interval = "30s"
timeout       = "5s"
offline_threshold = "60s"
enable_monitor    = true

[devices.tcp]
host        = "192.168.1.10"
port        = 502

slave_id = 1

[[devices.points]]
name          = "voltage"
function_code = 3
address       = 100
quantity      = 1
data_type     = "uint16"
scale         = 0.1
unit          = "V"
access        = "ro"
cache_enabled = true

[[devices.points]]
name          = "current"
function_code = 3
address       = 102
quantity      = 1
data_type     = "uint16"
scale         = 0.01
unit          = "A"
access        = "ro"
cache_enabled = true

[[devices.points]]
name          = "power"
function_code = 3
address       = 104
quantity      = 2
data_type     = "float32"
unit          = "kW"
access        = "ro"
cache_enabled = true
```

### 读写控制点

```toml
[[devices]]
name          = "PLC控制器"
type          = "tcp"
poll_interval = "5s"
timeout       = "3s"

[devices.tcp]
host     = "192.168.1.20"
port     = 502

slave_id = 1

# 只读点：状态监控
[[devices.points]]
name          = "running_status"
function_code = 1
address       = 0
quantity      = 1
data_type     = "bool"
access        = "ro"
cache_enabled = false

# 读写点：远程控制
[[devices.points]]
name          = "start_command"
function_code = 5
address       = 100
quantity      = 1
data_type     = "bool"
access        = "rw"
cache_enabled = false

# 读写点：设定值
[[devices.points]]
name          = "speed_setpoint"
function_code = 16
address       = 200
quantity      = 1
data_type     = "int16"
scale         = 1.0
unit          = "RPM"
access        = "rw"
cache_enabled = false
```

## 数据输出格式

采集的数据将封装为以下 JSON 格式上传：

```json
{
  "device": "温湿度传感器",
  "timestamp": 1704067200,
  "data": {
    "temperature": {
      "value": 25.3,
      "unit": "°C"
    },
    "humidity": {
      "value": 60.5,
      "unit": "%"
    }
  }
}
```

### 字段说明

| 字段        | 类型   | 说明                       |
| ----------- | ------ | -------------------------- |
| `device`    | string | 设备名称（来自 `name`）    |
| `timestamp` | int64  | Unix 时间戳（秒）          |
| `data`      | object | 数据点集合，键为数据点名称 |

### 数据点对象

```json
{
  "value": 25.3,
  "unit": "°C"
}
```

- `value`：工程值（原始值 × `scale`）
- `unit`：工程单位（来自配置，可省略）

## 配置验证

配置加载时会进行自动验证：

1. **设备级别验证**：
   - `name` 不能为空
   - `type` 必须为 `"rtu"` 或 `"tcp"`
   - RTU 设备必须有 `[devices.rtu]` 块
   - TCP 设备必须有 `[devices.tcp]` 块
   - `slave_id` 必须在 1-247 范围内
   - 至少包含一个数据点

2. **数据点级别验证**：
   - `name` 不能为空
   - `function_code` 必须为 1/2/3/4/5/6/16
   - `access` 与 `function_code` 必须兼容
   - `data_type` 必须为有效值
   - `quantity` 必须大于 0

## 健康监控

当 `enable_monitor = true` 时，系统会：

1. **在线状态跟踪**：
   - 记录每次 Modbus 请求的成功/失败
   - 连续失败超过 `offline_threshold` 时标记设备离线
   - 设备恢复在线时自动标记并记录日志

2. **日志输出**：
   ```
   2024-01-01T10:00:00Z INFO Modbus device '温湿度传感器' is offline after 30s failures
   2024-01-01T10:01:00Z INFO Modbus device '温湿度传感器' is back online
   ```

## 断点续传集成

数据点配置的 `cache_enabled` 控制是否参与断点续传：

- **启用缓存**（`cache_enabled = true`）：
  - 数据写入本地 SQLite 数据库
  - 网络恢复后按时间顺序上传
  - 适用于需要保证数据完整性的场景（如能耗统计）

- **禁用缓存**（`cache_enabled = false`）：
  - 数据仅实时上传，失败即丢弃
  - 适用于实时性要求高的场景（如控制指令）

## 常见问题

### Q: 如何确定设备的 `slave_id`？

A: 查看设备手册或使用 Modbus 扫描工具：
```bash
# 使用 mbpoll 扫描
mbpoll -m rtu -b 9600 -a 1-247 /dev/ttyS0
```

### Q: 寄存器地址从哪里开始？

A: Modbus 地址有两种约定：
- **PLC 约定**：从 1 开始（如地址 10001）
- **协议约定**：从 0 开始（如地址 0）

LEPG 使用**协议约定**（从 0 开始），PLC 地址需减 1。

### Q: 如何调试 Modbus 连接？

A: 启用 Debug 日志查看详细通信：
```bash
lepgc run --log_level=debug
```

### Q: RTU 和 TCP 如何选择？

A:
- **RTU**：串口设备，通常用于本地传感器/执行器
- **TCP**：以太网设备，通常用于远程 PLC 或网关

### Q: `scale` 如何计算？

A: 根据设备数据手册确定：
- 设备返回 `250` 表示 `25.0°C` → `scale = 0.1`
- 设备返回 `1000` 表示 `10.00A` → `scale = 0.01`

公式：`工程值 = 原始值 × scale`

## 相关代码

- **结构定义**：`internal/client/modbus.go`
- **配置加载**：`internal/client/config.go`
- **客户端主循环**：`internal/client/client.go`
- **消息协议**：`internal/msg/msg.go`
