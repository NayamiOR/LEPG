# LEPG 项目现状报告 & 开发路线图

## 项目概况

LEPG（轻量级边缘穿透网关）是一个基于 Go 的 IoT 边缘网关系统，采用 Client/Server 架构。客户端采集工业设备数据（Modbus/MQTT），本地 SQLite 缓存后通过自定义 TLV 二进制协议上传到服务端。服务端认证后存储数据，并通过内嵌 MQTT Broker 分发给外部消费者。

**数据链路拓扑：**

```
现场设备 (Modbus/MQTT) → 客户端 → [TLV over TCP] → 服务端 → MQTT Broker → 外部消费者
                              ↓                              ↓
                          SQLite 缓存                    SQLite 存储
                       (断点续传/离线重传)
```

---

## 一、各模块完成度总览

### 基础设施层（已完成）

| 模块 | 完成度 | 状态说明 |
|------|--------|----------|
| **自定义 TLV 协议** | ✅ 100% | 编解码、分帧、校验和、类型化包均已实现，有单元测试 |
| **CRC16-CCITT 校验** | ✅ 100% | 含测试和基准测试 |
| **自定义时间戳** | ✅ 100% | 2020-01-01 纪元，32位范围至~2065年 |
| **配置系统（Provider Chain）** | ✅ 100% | 四层优先级链（Default < Env < File < Flag），依赖注入，完整校验 |
| **错误类型体系** | ✅ 100% | 协议错误 + 配置错误 + 聚合错误 |
| **数据模型** | ✅ 100% | Reading、Modbus 类型（bool/int16/uint16/int32/uint32/float32）、字节序转换 |
| **CLI 入口** | ✅ 100% | Cobra 框架，init/run 子命令，信号处理 |

### 客户端（lepgc）

| 模块 | 完成度 | 状态说明 |
|------|--------|----------|
| **SQLite 缓存** | ✅ 100% | Bun ORM，WAL 模式，状态追踪（pending→uploading→uploaded/failed） |
| **断点续传** | ✅ 100% | `uploadLoop` 每轮取 pending 条目上传，失败标记 status=3 下次继续重试 |
| **主流程管线** | ✅ 100% | conc.WaitGroup 多协程：MQTT Broker → channel → SQLite → 上传循环 |
| **Modbus TCP 轮询** | ⚠️ ~80% | FC1-4 实现，支持所有数据类型和缩放；RTU 未实现、写操作未实现 |
| **MQTT Broker（本地）** | ⚠️ ~70% | 可接收数据并转 Reading，但不校验配置中注册的设备/点位 |
| **Modbus RTU** | ❌ 0% | 配置结构已定义，无轮询代码 |
| **Modbus 写操作** | ❌ 0% | FC5/6/16 配置已支持，无实现 |

### 服务端（lepgs）

| 模块 | 完成度 | 状态说明 |
|------|--------|----------|
| **TCP 监听 + 连接管理** | ✅ 100% | 每连接一个 goroutine |
| **握手认证** | ✅ 100% | SN/Token 静态校验，返回 OK/BadSn/BadToken |
| **数据接收 + SQLite 存储** | ✅ 100% | gob 解码 Upload 消息，存入 SQLite |
| **SQLite 查询** | ✅ 100% | 按 SN、设备名、时间范围过滤 |
| **连接管理器** | ✅ 100% | 内存版 + Redis 版均实现（Redis 版未在主流程启用） |
| **内嵌 MQTT Broker** | ⚠️ ~50% | comqtt v2，TCP + WS 监听正常；无认证（AllowHook），无 ACL |
| **数据桥接（TLV→MQTT）** | ❌ ~20% | `MqttPublisher` 已实现但未接入，当前使用 `NopPublisher`，**零数据输出** |

### 未实现功能

| 功能 | 说明 |
|------|------|
| **心跳协议** | `MsgTypeHeartbeat` 已定义，无发送/接收/超时逻辑 |
| **错误消息协议** | `MsgTypeError` 已定义，无处理逻辑 |
| **MQTT 认证/ACL** | 设计文档完整，代码未实现 |
| **TLS/WSS 加密隧道** | TODO 中列出 |
| **规则引擎** | 未开始 |

---

## 二、关键问题（按影响排序）

1. **服务端数据桥接断路** — 最核心的功能缺口。服务端接收了数据、存入了 SQLite，但 MQTT Broker 对外发布的流量为零。`MqttPublisher` 已写好，只需接入替换 `NopPublisher` 并实现 Reading→JSON 序列化。
2. **客户端 MQTT 不校验数据** — `handleMqttReading` 接受任何 SN 和点位，不检查是否在 `MqttConfig` 中注册。
3. **无 MQTT 测试** — 整个 MQTT 数据路径零自动化测试。
4. **Modbus 解析逻辑待验证** — `modbus.go:74` 有 TODO 注释提示需要检查纠正。

---

## 三、服务端功能规划

### 已实现

| 功能 | 文件 | 说明 |
|------|------|------|
| TCP 监听 | `server.go` | `net.Listen` + `Accept` 循环 |
| 握手认证 | `server.go` | SN/Token 静态校验，HandshakeResponse |
| 数据接收 | `server.go` | gob 解码 → `SaveReadings` |
| MQTT Broker | `mqtt.go` | comqtt v2，TCP + WS |
| Publisher 接口 | `publisher.go` | `NopPublisher` / `MqttPublisher` |
| SQLite 存储 | `cache/sqlite.go` | 含 `QueryReadings` 过滤查询 |
| 连接管理器 | `cache/connections/` | 内存版 + Redis 版 |

### 待实现

| 优先级 | 功能 | 说明 | 参考 |
|--------|------|------|------|
| P1 | **数据桥接** | Reading→JSON 序列化，`MqttPublisher` 接入替换 `NopPublisher` | `MQTT Broker Design.md` §2 |
| P2 | **MQTT 认证** | 自定义 AuthHook，MQTT username/password 映射 SN/Token | `MQTT Broker Design.md` §3 |
| P3 | **ACL 规则 + QoS 分级** | 设备只能访问自己 SN 的 Topic；reading QoS 0、status QoS 1 + Retain | `MQTT Broker Design.md` §4-5 |
| P3 | **心跳超时检测** | 客户端长时间无消息则断开连接 | |
| P3 | **设备上下线通知** | 发布 `device/{SN}/status`（Retain），新订阅者立即获取状态 | |
| P4 | **性能测试** | 100+ 连接、1000 msg/s 吞吐、24h 稳定性 | `MQTT Broker Design.md` §6 |
| P5 | **MQTT 消息持久化** | Bolt Hook，Broker 重启后恢复 session/retained message | `MQTT Broker Design.md` §7 |
| P6 | **WebSocket TLS** | WSS 支持 | |
| 远期 | **命令下发** | 从 MQTT `device/{SN}/command` 接收指令，转发到对应边缘客户端 | |
| 远期 | **HTTP API** | RESTful 接口供外部系统查询历史数据、管理设备 | |
| 远期 | **Web 管理界面** | 设备管理、实时数据可视化、在线配置编辑 | |

---

## 四、开发路线图

### Phase 0：服务端数据通路打通（预计 1-2 天）

**目标**：数据全链路流通 — 设备 → 客户端 → 服务端 → MQTT Broker → 外部消费者

| 任务 | 文件 | 说明 |
|------|------|------|
| 接入 `MqttPublisher` 替换 `NopPublisher` | `cmd/server/main.go` | 一行替换 |
| 实现 Reading→JSON 序列化 | `internal/server/server.go` | 定义 `MqttReading` 结构体，取消 TODO 注释启用 publisher 调用 |
| Topic 格式统一 | `internal/client/mqtt.go` / `internal/server/mqtt.go` | 确保两端 Topic 命名一致 |
| 端到端验证 | 手动测试 | 启动服务端 + 客户端（MQTT 模式），外部客户端订阅 `device/+/reading` |

**验证**：外部 MQTT 客户端能订阅并收到设备数据

---

### Phase 1：客户端 MQTT 数据校验（预计 1 天）

**目标**：客户端只接受配置中注册的设备和数据点

| 任务 | 文件 |
|------|------|
| 校验设备 SN 是否在 MqttConfig 中注册 | `internal/client/mqtt.go` |
| 校验数据点名称是否属于该设备 | `internal/client/mqtt.go` |
| 缓存 DeviceHash（启动时计算一次） | `internal/client/mqtt.go` |

**验证**：未注册设备/点位的 MQTT 消息被拒绝，注册的正常通过

---

### Phase 2：MQTT 认证与 ACL（预计 2-3 天）

**目标**：MQTT Broker 具备基本安全能力

| 任务 | 文件 | 说明 |
|------|------|------|
| 自定义 AuthHook（SN/Token 认证） | `internal/server/mqtt.go` | `OnConnectAuthenticate` + `OnACLCheck` |
| ACL 规则 | 新文件 | 设备只能发布自己的 Topic，管理员可订阅通配符 |
| 客户端 MQTT Broker 同步添加认证 | `internal/client/mqtt.go` | |
| 非本地监听安全检查 | `internal/server/mqtt.go` | 非回环地址无认证则拒绝启动 |

**验证**：无凭证客户端被拒绝；设备只能访问自己 SN 下的 Topic

---

### Phase 3：心跳与设备生命周期（预计 1-2 天）

**目标**：连接保活 + 设备在线/离线状态管理

| 任务 | 文件 | 说明 |
|------|------|------|
| 客户端定时发送心跳 | `internal/client/client.go` | `MsgTypeHeartbeat` |
| 服务端检测心跳超时 | `internal/server/server.go` | 超时关闭连接 |
| 设备上线/离线 MQTT 通知 | `internal/server/server.go` | `device/{sn}/status`（QoS 1 + Retain） |

**验证**：设备断开后 status Topic 收到离线消息；重连后收到在线消息

---

### Phase 4：MQTT 测试覆盖（预计 1-2 天）

**目标**：MQTT 数据路径有可靠的自动化测试

| 任务 | 说明 |
|------|------|
| 客户端 MQTT 数据接收测试 | JSON 解析、配置校验、异常数据 |
| 服务端数据桥接测试 | Reading→JSON→MQTT Publish 流程 |
| 集成测试（可选） | 使用真实 MQTT Broker |

**验证**：`make test` 覆盖 MQTT 路径

---

### Phase 5：生产级认证（预计 2-3 天）

**目标**：从静态 SN/Token 升级到安全的认证流程

| 任务 | 说明 | 参考 |
|------|------|------|
| 一次性 Token 注册 | 客户端 CLI 生成 SN，服务端提供一次性 Token | `docs/Verification.md` |
| 握手后 Token 失效 + 随机长密码 | 首次握手成功后替换 | |
| 客户端持久化密码 | 存储到本地配置或安全存储 | |

**验证**：旧 Token 无法二次使用

---

### Phase 6：Modbus RTU + 写操作（预计 2-3 天）

**目标**：补全 Modbus 功能

| 任务 | 文件 |
|------|------|
| RTU 轮询 | `internal/client/modbus.go` |
| TCP/RTU 分派 | `internal/client/client.go` |
| FC5/FC6/FC16 写操作 | `internal/client/modbus.go` |
| 验证 TCP 解析逻辑（modbus.go:74 TODO） | `internal/client/modbus.go` |
| 离线检测（`enable_monitor` + `offline_threshold`） | `internal/client/modbus.go` |

**验证**：RTU 设备正常采集；写寄存器操作成功

---

### Phase 7：TLS 加密隧道 + 自适应心跳（预计 2 天）

| 任务 | 说明 |
|------|------|
| 客户端/服务端 TLS 连接 | TCP 隧道启用 TLS |
| WebSocket TLS | MQTT Broker WSS |
| 自适应心跳间隔 | 根据网络状况动态调整 |

---

### Phase 8：上传重连优化 + 规则引擎（预计 2-3 天）

| 任务 | 说明 |
|------|------|
| 上传失败后自动重连 + 续传 | 当前上传失败直接退出循环，需改为重连后继续取 pending 数据上传 |
| 基础规则引擎 | 阈值告警、简单计算（求平均、差值） |
| Access 和 CacheEnabled 配置生效 | 当前解析了但未使用 |

---

### 远期规划（暂无时间表）

| 方向 | 内容 |
|------|------|
| **集群部署** | k3s + Redis 替换内存连接管理 |
| **HTTP API** | RESTful 接口供外部系统调用 |
| **Web 管理界面** | 设备管理、实时数据可视化、在线配置编辑 |
| **命令下发** | MQTT Topic → 边缘设备控制指令 |
| **OPC-UA** | 第三种设备接入协议 |
| **告警通知** | 邮件/Webhook/微信 |
| **数据聚合** | 时序存储、历史查询 |
| **OTA 升级** | 固件批量升级 |
| **配置热加载** | 远程配置中心（Consul/etcd） |
| **systemd 服务** | 开机自启、自动重启 |

---

## 五、建议优先级排序

```
高   → 数据通路打通（Phase 0）     ← 最大的功能缺口
高   → 客户端 MQTT 校验（Phase 1）
中   → MQTT 认证 ACL（Phase 2）
中   → 心跳与生命周期（Phase 3）
中   → 测试覆盖（Phase 4）
中   → 生产认证（Phase 5）
低   → Modbus RTU/写操作（Phase 6）
低   → TLS 加密（Phase 7）
低   → 上传重连 + 规则引擎（Phase 8）
远期 → Web UI、集群、OPC-UA 等
```
