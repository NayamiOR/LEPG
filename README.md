# LEPG

> **Lightweight Edge Piercing Gateway** — 轻量级 IoT 边缘网关与数据中继系统

## 产品定位与场景

**LEPG** 面向 **几百设备规模** 的个人开发者、小团队和轻工业场景，主打 **开箱即用的部署体验** + **工业级的稳定性能**：

- **简单易用**：无需单独搭建内网穿透、协议转换或消息队列，10 分钟跑通全链路
- **工业性能**：在树莓派、小服务器上稳定运行，支持数千设备点 × 数十个数据点的持续采集与转发
- **轻量部署**：单设备 docker-compose 即可，小 k3s 集群亦可，资源占用低

### 核心场景

| 场景分类           | 设备规模   | 目标用户                                                        | 核心痛点                                                 | 不可替代价值                                      |
| ------------------ | -------- | ------------------------------------------------------------- | ---------------------------------------------------- | ---------------------------------------------- |
| 个人爱好者/学生开发者  | 1~20台   | 智能家居改造爱好者、IoT 毕设学生、DIY 电子爱好者                           | 要自己搭 frp 穿透、写协议转换脚本、搭 MQTT Broker，折腾几天搞不定，重型网关树莓派跑不动 | 10分钟部署，单二进制30M 以内，树莓派随便跑，不用搭任何额外服务       |
| 微型线下实体场景     | 20~100台 | 个体工商户（冷链温度监测、设备状态采集）、小型作坊设备运维、小型农业/水产养殖监测       | 商用网关动辄几千上万，云厂商 IoT 方案年费贵还需要专人维护，预算完全撑不住            | 免费开源，成本只有商用方案的1%，零运维，基础的远程看数、异常上报需求完全满足 |
| 小团队 IoT 原型验证  | 10~50台  | 10人以内 IoT 创业团队、高校实验室 IoT 课题小组                             | 要花1-2个月自行开发设备接入层，耽误业务原型验证进度                          | 直接用 LEPG，不用自己写穿透、协议转换逻辑，一周就能跑通全链路       |
| 轻工业数据采集场景    | 100~500台 | 小型制造车间设备监控、轻工业产线状态采集、分布式设备群运维                         | 工业级 IoT 平台太重（需专用服务器、专业运维），商用网关功能受限且价格高昂，自研成本高     | docker-compose 一键部署，树莓派/小服务器即可运行，工业级稳定性      |

### 排除场景

- 大规模工业生产场景（数千设备以上，需专用集群部署）
- 高可靠关键业务场景（需 99.99% 可用性保证、双机热备等）
- 超高频实时控制场景（ms 级控制响应需求）

---

## 技术栈

### 核心技术

| 类别 | 技术 | 用途 |
|------|------|------|
| **语言** | Go 1.23+ | 核心开发语言 |
| **协议** | 自定义 TLV | 数据传输格式 |
| **数据库** | SQLite | 客户端本地缓存（断点续传） |
| | PostgreSQL | 服务端持久化存储 |
| **工业协议** | Modbus TCP/RTU | 设备数据采集 |
| **消息队列** | MQTT | 设备数据接入与转发 |
| **缓存** | Redis | 连接状态管理 |

### 主要依赖

- **CLI 框架**: [spf13/cobra](https://github.com/spf13/cobra)
- **配置管理**: [spf13/viper](https://github.com/spf13/viper)
- **ORM**: [uptrace/bun](https://github.com/uptrace/bun)
- **Modbus**: [grid-electricity/modbus](https://github.com/grid-electricity/modbus)

### 架构模式

- **Provider Chain**: 分层配置系统（Default < Env < File < Flag）
- **依赖注入**: 配置注入而非全局单例
- **Goroutine-per-connection**: 服务端并发处理

---

## 系统架构

### 全链路 Workflow

整个场景 Workflow 分为四层：

- **设备层**：自有传感器等终端，产生信息
- **边缘网关客户端**：边缘端，运行在树莓派、小主机、OpenWrt 上
- **中继端（服务端）**：数据统一整合中间件，运行在云服务器、小 k3s 集群、单设备 docker-compose 等轻量环境
- **用户侧**：用户的业务系统、MQTT 客户端、第三方 IoT 平台等数据消费者

#### 上行链路

| 链路环节          | 所属角色             | 核心模块              | 具体要做的事                                                                                                                                      | 状态         |
| ---------------- | ---------------- | ----------------- | ------------------------------------------------------------------------------------------------------------------------------- | ---------- |
| 1. 设备输出原始数据   | 现场设备层            | 无（用户自有设备）         | 按私有协议输出原始字节流，如 Modbus 传感器输出寄存器原始值、BLE 设备输出广播数据等                                                                                              | 无          |
| 2. 协议适配解析     | 边缘网关客户端          | 协议适配模块            | ① 底层调用开源编解码库解析原始协议帧<br>② 多设备轮询调度、超时重试<br>③ 解析后的原始值按用户配置映射成结构化 KV（如寄存器0x01映射为 `temperature: 25.6`）                             | ✅ Modbus TCP<br>⚠️ Modbus RTU |
| 3. 本地数据预处理    | 边缘网关客户端          | 数据预处理模块           | ① 断网时本地缓存数据，联网后自动补传<br>② 可选异常数据过滤/打标<br>③ 不需要预处理的场景支持直接透传原始数据                                                            | ✅         |
| 4. 内网数据穿透到公网  | 边缘网关客户端+中继端（服务端） | 自定义 TCP 隧道 + 心跳保活  | ① 边缘客户端启动后自动和中继端建立长连接，完成身份认证<br>② 边缘客户端把结构化数据通过长连接传到中继端<br>③ 心跳保活 + 断线重连                                                  | ✅         |
| 5. 数据标准化输出与转发 | 中继端（服务端）         | 数据转发桥             | 中继端将设备数据经过缓冲、过滤、规则化处理后，通过北向接口对外暴露标准消费接口<br>① 标准 MQTT Topic<br>② 标准 HTTP Webhook（规划中）<br>③ 支持转发到第三方系统（规划中） | ✅ MQTT<br>⚠️ HTTP  |
| 6. 用户侧消费数据    | 用户侧              | 无（用户自有业务）         | 用户直接用标准 MQTT 客户端/HTTP 请求就能拿到结构化的设备数据                                                                                                          | 无          |

#### 下行链路

| 链路环节          | 所属角色        | 核心模块          | 具体要做的事                                                    | 状态 |
| ------------- | ----------- | ------------- | --------------------------------------------------------- | -- |
| 1. 用户下发控制指令   | 用户侧         | 无             | 用户按约定格式下发控制指令到公网服务端的对应 MQTT Topic/HTTP 接口           | 无  |
| 2. 指令透传到边缘网关  | 中继端（服务端）+边缘客户端 | 穿透服务端+穿透客户端模块 | 中继端根据设备 ID 找到对应的边缘网关长连接，把指令转发到边缘客户端                       | 🔄  |
| 3. 指令转成设备私有协议 | 边缘网关客户端     | 协议适配模块        | 把结构化的控制指令转成设备能识别的私有协议帧                                | 🔄  |
| 4. 指令下发到设备执行  | 边缘网关客户端     | 协议适配模块        | 把协议帧下发到对应的现场设备，设备执行后返回响应，原路返回给用户侧                       | 🔄  |

> ✅ 已实现 | ⚠️ 部分实现 | 🔄 规划中 | ❌ 未实现

### 流程示例

用户要远程采集家里的 Modbus 温湿度传感器数据，用 LEPG 的操作流程：

1. **边缘端部署**：把温湿度传感器接在树莓派串口/网络上，树莓派运行 LEPG 客户端，配置传感器参数和中继端地址
2. **自动采集**：网关自动按配置轮询传感器，把读到的原始值转成 `{"temperature": 25.6}`
3. **数据传输**：数据通过加密长连接传到中继端，中继端处理后推送到用户的专属 MQTT Topic
4. **远程监控**：用户在公司用手机 MQTT 客户端订阅这个 Topic，直接就能看到家里的实时温度
5. **远程控制**（规划中）：用户发指令 `{"relay":1}` 到对应 Topic，网关收到后转成 Modbus 写寄存器的指令下发

---

## 功能模块

### 通用模块

- **自定义网络协议**：TLV 格式的 TCP 隧道协议，支持多路复用、智能重连、流量监控、安全认证
- **可观测性**：日志系统（基于 Go slog）、实时监控（规划中）、性能指标（规划中）

### 边缘端

- **数据采集**：
  - ✅ Modbus TCP 轮询（支持 FC1-4，所有数据类型）
  - ⚠️ Modbus RTU 轮询（规划中）
  - ⚠️ MQTT 设备接入（部分实现）
- **数据预处理与缓存**：
  - ✅ SQLite 本地缓存（WAL 模式，状态追踪）
  - ✅ 断点续传
  - ⚠️ 异常数据过滤/打标（规划中）
  - ✅ 透传原始数据支持
- **边缘计算**（规划中）：高频数据本地预聚合

### 中继端

中继端定位为 **多来源数据统一整合的轻量中间件**，提供可独立部署的轻量级控制台和后端服务（规划中）。

#### 核心职责

- **多来源数据整合**：✅ 统一接收多个边缘网关上报的异构设备数据
- **南向对接**：✅ 适配工业协议（Modbus TCP），通过边缘网关统一接入
- **北向数据服务**：✅ 对外提供 MQTT 接口，⚠️ HTTP Webhook（规划中）
- **数据处理管线**：✅ PostgreSQL 持久化存储，⚠️ 规则引擎（规划中）
- **设备管理**：
  - ✅ 设备注册表（PostgreSQL）
  - ✅ 连接管理（内存版 + Redis 版）
  - ✅ 心跳检测
  - ⚠️ 配置下发（规划中）
  - ⚠️ 远程调试（规划中）

#### 设备抽象与数字孪生

- ✅ 统一数据模型（Reading、DataType）
- ✅ 字节序转换（支持 abcd、badc、cdab、dcba）
- ⚠️ 高频数据预聚合（规划中）

#### 设备全生命周期管理

| 能力         | 说明                                     | 状态  |
| ---------- | -------------------------------------- | --- |
| 注册与认证      | 网关和设备的身份注册、凭证管理、接入认证（SN/Token）           | ✅  |
| 状态监控与告警    | 在线/离线状态检测、数据异常告警、设备健康度视图              | ⚠️  |
| 配置模板管理     | 设备配置模板定义与批量下发（如采集频率、数据映射规则等）          | 🔄  |

#### 数据转发桥

负责提供北向接口，将处理后的设备数据推送到外部系统：

- ✅ MQTT Broker 转发（TCP + WebSocket）
- ⚠️ HTTP Webhook 推送（规划中）
- ⚠️ 第三方 IoT 平台对接（规划中）

#### 调试工具（规划中）

- 远程日志拉取
- 命令交互终端

---

## 部署与使用

### 构建项目

```bash
# 克隆仓库
git clone https://github.com/nayami/lepg.git
cd lepg

# 构建客户端和服务端
make build

# 构建产物位于 bin/ 目录
# bin/lepgc  - 客户端
# bin/lepgs  - 服务端
```

### 快速开始

#### 1. 初始化配置

```bash
# 客户端：初始化默认配置到 /etc/lepgc/config.toml
./bin/lepgc init

# 服务端：初始化默认配置到 /etc/lepgs/config.toml
./bin/lepgs init
```

#### 2. 启动服务端（云端）

```bash
# 使用默认配置文件
./bin/lepgs run

# 指定配置文件
./bin/lepgs run -c /path/to/server.toml

# 覆盖端口
./bin/lepgs run -p 8883
```

#### 3. 启动客户端（边缘端）

```bash
# 使用默认配置文件
./bin/lepgc run

# 指定配置文件
./bin/lepgc run -c /path/to/client.toml

# 覆盖服务器地址和端口
./bin/lepgc run -u relay.example.com -p 8883
```

#### 4. 使用模拟器测试

```bash
# 运行 Modbus 设备模拟器（4 个设备）
make simmodbus

# 运行 MQTT 传感器模拟器（6 个设备）
make simmqtt

# 同时运行所有模拟器
make sim
```

### Docker 部署（规划中）

```bash
# 云端部署中继服务器
docker-compose -f docker-compose.server.yml up -d

# 边缘端部署网关
docker-compose -f docker-compose.client.yml up -d
```

### K3s 部署（规划中）

```bash
# 部署到 k3s 集群
kubectl apply -f k8s/server.yaml
kubectl apply -f k8s/client.yaml
```

---

## 配置文件

### 客户端配置 (`config/client.toml`)

```toml
# 日志配置
log_level = 'info'      # 日志级别: debug, info, warn, error
log_path = 'logs/'      # 日志目录

# 服务端连接配置
server = '127.0.0.1'    # 中继端地址
port = 8883             # 中继端端口
sn = "CLIENT001"        # 客户端序列号
token = "token123456"   # 认证令牌

# 重连配置
max_retry = 10          # 最大重试次数
retry_interval = 5000   # 重试间隔（毫秒）

# 本地存储
data_path = "./data/data.db"  # SQLite 数据库路径

# Modbus TCP 设备配置
[[devices]]
name = "sensor-1"             # 设备名称
type = "tcp"                  # 设备类型: tcp, rtu
timeout = "5s"                # 连接超时
offline_threshold = "30s"     # 离线阈值
enable_monitor = true         # 启用监控
slave_id = 1                  # Modbus 从站 ID
poll_interval = "1s"          # 轮询间隔

[devices.tcp]
host = "127.0.0.1"
port = 5020

# 数据点配置
[[devices.points]]
name = "temperature"          # 点位名称
function_code = 3             # 功能码: 1=ReadCoils, 2=ReadDiscreteInputs, 3=ReadHoldingRegisters, 4=ReadInputRegisters
address = 0                   # 寄存器地址（PDU 地址，0-based）
quantity = 1                  # 读取数量
data_type = "int16"           # 数据类型: int16, uint16, int32, uint32, float32
byte_order = "abcd"           # 字节序: abcd, badc, cdab, dcba
scale = 0.1                   # 缩放因子
offset = 0                    # 偏移量
unit = "°C"                   # 单位
access = "ro"                 # 访问权限: ro, rw
cache_enabled = true          # 启用缓存

# MQTT 虚拟设备配置
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
```

### 服务端配置 (`config/server.toml`)

```toml
# 日志配置
log_level = 'info'
log_path = 'logs/'

# 服务监听配置
port = 8883

# PostgreSQL 配置
pg_host = "127.0.0.1"
pg_port = 5432
pg_user = "lepgs"
pg_password = ""
pg_dbname = "lepgs"
pg_sslmode = "disable"

# MQTT Broker 配置
mqtt_tcp = "127.0.0.1:1883"    # MQTT TCP 监听地址
mqtt_ws = "127.0.0.1:8083"     # MQTT WebSocket 监听地址

# 客户端认证配置
[[clients]]
sn = "CLIENT001"
token = "token123456"
description = "测试客户端1"
```

---

## 开发指南

### 开发环境

- Go 1.23+
- PostgreSQL 14+（服务端）
- MQTT Broker（可选，如 EMQX）

### 开发命令

```bash
# 运行客户端
make run-client    # 或 make c
go run cmd/client/main.go run

# 运行服务端
make run-server    # 或 make s
go run cmd/server/main.go run

# 运行测试
make test
make test-coverage

# 清理构建文件
make clean
```

### 项目结构

```
LEPG/
│   ├── cmd/
│   ├── client/               # 客户端入口
│   ├── server/               # 服务端入口
│   ├── modbus-sim/           # Modbus 模拟器
│   ├── mqtt-sim/             # MQTT 模拟器
│   └── mqtt-sub/             # MQTT Pull 测试订阅工具
├── internal/
│   ├── client/               # 客户端实现
│   ├── server/               # 服务端实现
│   ├── config/               # 配置系统（Provider Chain）
│   ├── msg/                  # TLV 消息协议
│   ├── model/                # 数据模型
│   ├── modbus/               # Modbus 协议处理
│   ├── mqtt/                 # MQTT 处理
│   ├── db/                   # 数据库（SQLite、PostgreSQL）
│   ├── registry/             # 设备注册表
│   ├── publisher/            # 数据发布
│   ├── monitor/              # 设备监控
│   └── utils/                # 工具函数
├── config/                   # 默认配置文件
├── docs/                     # Foam 知识库
├── scripts/                  # 脚本工具
└── Makefile                  # 构建配置
```

---

## 协议设计

LEPG 使用自定义的 TLV (Type-Length-Value) 协议进行数据传输：

```
[Magic:2B][Version:1B][Flags:1B][Type:1B][MsgID:2B][PayloadLen:2B][Timestamp:4B][Payload:N][Checksum:2B]
```

### 消息类型

| Type  | 名称              | 说明     |
| ----- | --------------- | ------ |
| 0x01  | Handshake        | 握手请求   |
| 0x02  | HandshakeAck     | 握手响应   |
| 0x03  | Upload           | 数据上传   |
| 0x04  | UploadAck        | 上传确认   |
| 0x05  | Heartbeat        | 心跳     |
| 0x06  | HeartbeatAck     | 心跳响应   |
| 0x07  | Notify           | 通知（规划） |

详见 [docs/design/message-protocol.md](docs/design/message-protocol.md)

---

## 路线图

### 已实现

- TLV 协议实现
- 配置系统（Provider Chain）
- SQLite 本地缓存（WAL 模式，状态追踪）
- PostgreSQL 持久化存储
- Modbus TCP 轮询（FC1-4，所有数据类型）
- 心跳保活与断线重连
- 设备注册表（PostgreSQL）
- 连接管理（内存版 + Redis 版）
- MQTT Broker（服务端，TCP + WebSocket）
- 字节序转换（abcd、badc、cdab、dcba）
- **北向 Push 输出链**（Sinker/Formatter/OutputRouter，TB Gateway MQTT + HTTP）
- **MQTT Pull 数据桥接**（`device/{SN}/reading`，JSON 批量数组）
- HTTP Sinker HTTPS 支持
- 统一格式日志（`internal/client/log.go`）
- 核心模块单元测试（model/payload/formatter/output/connections/server）
- 上传失败自动重连 + 续传（断点续传增强）
- 配置文件 .example 模板化

### 进行中 / 近期计划

- Modbus 写操作（FC5/6/16）
- 设备上下线 MQTT 通知（`device/{SN}/status`）
- 客户端 MQTT 数据校验
- MQTT 认证与 ACL
- HTTP 健康检查端点

### 未来规划

- Modbus RTU 支持
- Prometheus 指标导出
- TLS/WSS 加密隧道
- 基础规则引擎（阈值告警）
- 通用 MQTT Sinker（对接非 TB 平台）
- 数据自动清理（TTL）
- HTTP API 与 Webhook
