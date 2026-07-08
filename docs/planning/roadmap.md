---
title: LEPG 项目现状报告 & 开发路线图
description: LEPG 各模块完成度、关键问题、服务端功能规划与 Phase 0–8 分阶段开发路线图
type: roadmap
tags: [roadmap, planning, status]
date: 2026-07-03
---

# LEPG 项目现状报告 & 开发路线图

> 最后更新：2026-07-04。基于截至 `78971b1` 的代码审查 + Modbus 写操作实现。

## 项目概况

LEPG（轻量级边缘穿透网关）是一个基于 Go 的 IoT 边缘网关系统，采用 Client/Server 架构。客户端采集工业设备数据（Modbus/MQTT），本地 SQLite 缓存后通过自定义 TLV 二进制协议上传到服务端。服务端认证后存储到 PostgreSQL，通过内嵌 MQTT Broker（Pull 模式）和北向 Push Sinker 两种方式对外输出。

**数据链路拓扑：**

```
现场设备 (Modbus/MQTT) → 客户端 → [TLV over TCP] → 服务端 → MQTT Broker (Pull) → 外部消费者
                              ↓                              ↓
                          SQLite 缓存            ┌── PostgreSQL 存储
                       (断点续传/离线重传)        └── Push Sinker (TB MQTT/HTTP) → 外部平台
```

> 完整数据链路图见 [assets/data-link.excalidraw](../assets/data-link.excalidraw)。

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
| **上传重连优化** | ✅ 100% | `LoadPendingReadings` 覆盖 `NotSent + Failed` 两种状态，`uploadLoop` `runSession` 返回错误后外层 reconnect loop 重建 session 继续取 pending（原 Phase 8 目标） |
| **主流程管线** | ✅ 100% | conc.WaitGroup 多协程：MQTT Broker → channel → SQLite → 上传循环 |
| **统一格式日志** | ✅ 100% | `internal/client/log.go` — `logReading()` 统一列宽格式化输出 |
|| **Modbus TCP 轮询** | ✅ ~95% | FC1-4 实现，支持所有数据类型和缩放；写操作已通过 ModbusRuntime 补全；RTU 未实现 |
| **MQTT Broker（本地）** | ✅ ~85% | source/data_type 可配置解析（json/plain/kv 三种格式）；TS 自动识别（unix ms/s + RFC3339）；DeviceHash 缓存；设备离线通知（statusHook→notifyCh→MsgTypeNotify）；端口 1884；无认证 ACL；上线通知待做 |
| **Modbus RTU** | ❌ 0% | 配置结构已定义，无轮询代码 |
|| **Modbus 写操作** | ✅ 100% | FC5/6/16 通过 ModbusRuntime 实现：共用连接+双 goroutine、状态机连接管理、同步 Write 接口含值反算（scale/offset/byte order）、FC/DataType 兼容校验；含单元测试和 modbus-sim 集成验证 |

### 服务端（lepgs）

| 模块 | 完成度 | 状态说明 |
|------|--------|----------|
| **TCP 监听 + 连接管理** | ✅ 100% | 每连接一个 goroutine |
| **握手认证** | ✅ 100% | SN/Token 静态校验，返回 OK/BadSn/BadToken |
| **数据接收 + PostgreSQL 存储** | ✅ 100% | gob 解码 Upload 消息，存入 PostgreSQL |
| **PostgreSQL 查询** | ✅ 100% | 按 SN、设备名、时间范围过滤 |
| **连接管理器** | ✅ 100% | 内存版 + Redis 版均实现（Redis 版未在主流程启用） |
| **内嵌 MQTT Broker** | ✅ ~80% | comqtt v2，TCP + WS 监听正常；**数据桥接已打通**（`MqttPublisher` + `serializeReadings` → `device/{SN}/reading`）；无认证（AllowHook），无 ACL |
| **北向数据输出（Push）** | ✅ 100% | `internal/output/` 包实现 Sinker/Formatter/OutputRouter 三层架构；ThingsBoard Gateway MQTT + HTTP（含 HTTPS 支持）两种 Push 传输；按 DeviceName 分组 fan-out；`[[outputs]]` TOML 数组驱动 |
| **MQTT Pull 测试工具** | ✅ 100% | `cmd/mqtt-sub` — 订阅 `device/+/reading` 实时打印读数日志 |

### 未实现功能

| 功能 | 说明 |
|------|------|
| **MQTT 认证/ACL** | 设计文档完整，代码未实现 |
| ~~设备上下线通知~~ ⚠️ | MQTT 离线通知已实现；上线通知待 Phase 4；Modbus 离线检测待做 |
| **TLS/WSS 加密隧道** | 无任何实现 |
| **规则引擎** | 未开始 |
| **Modbus RTU** | TCP 已完整，RTU 轮询未实现 |
| ~~消息通知协议~~ ⚠️ | `MsgTypeNotify` 已接入 sessionLoop；server 侧处理逻辑待做 |

---

## 二、关键问题（按影响排序）

1. ~~**服务端数据桥接断路**~~ ✅ 已解决 — `MqttPublisher` + `serializeReadings` 正式接入，`device/{SN}/reading` 数据流通。
2. ~~**客户端 MQTT 不校验数据**~~ ✅ 已解决 — 审查后发现是伪需求：`routes` map 从 `MqttConfig` 构建，topic 命中后拿到的 deviceName/pointName 100% 在配置中。真正的缺口是 payload 格式硬编码，已通过 Phase 3b（source/data_type 可配置解析）解决。
3. ~~**Modbus 写操作空实现**~~ ✅ 已解决 — `ModbusRuntime` 实现 FC5/6/16，含状态机、值反算、FC/DataType 兼容校验、单元测试。
4. **设备上下线通知缺失** — ⚠️ 部分解决：MQTT 设备 session 过期离线通知已实现（statusHook→notifyCh→MsgTypeNotify）；Modbus 设备离线检测待做；上线通知暂未实现（待 Phase 4 ACL 一起）。

---

## 三、服务端功能规划

### 已实现

| 功能 | 文件 | 说明 |
|------|------|------|
| TCP 监听 | `server.go` | `net.Listen` + `Accept` 循环 |
| 握手认证 | `server.go` | SN/Token 静态校验，HandshakeResponse |
| 数据接收 | `server.go` | gob 解码 → `SaveReadings` |
| MQTT Broker | `mqtt.go` | comqtt v2，TCP + WS |
| **数据桥接（Pull）** | `publisher.go` | `MqttPublisher` → `device/{SN}/reading`（JSON 批量数组） ✅ |
| PostgreSQL 存储 | `cache/postgres.go` | 含 `QueryReadings` 过滤查询 |
| 连接管理器 | `cache/connections/` | 内存版 + Redis 版 |
| **北向数据输出（Push）** | `internal/output/` | Sinker/Formatter/OutputRouter；TB Gateway MQTT + HTTP（含 HTTPS）；`[[outputs]]` TOML 驱动 ✅ |
| **MQTT Pull 测试工具** | `cmd/mqtt-sub/` | 订阅 `device/+/reading`，实时打印 |

### 待实现

| 优先级 | 功能 | 说明 | 参考 |
|--------|------|------|------|
| P1 | **MQTT 认证** | 自定义 AuthHook，MQTT username/password 映射 SN/Token | [[mqtt-broker-design]] §3 |
| P1 | **设备上下线通知** | 发布 `device/{SN}/status`（Retain），新订阅者立即获取状态 | |
| P2 | **ACL 规则 + QoS 分级** | 设备只能访问自己 SN 的 Topic；reading QoS 0、status QoS 1 + Retain | [[mqtt-broker-design]] §4-5 |
| P3 | **性能测试** | 100+ 连接、1000 msg/s 吞吐、24h 稳定性 | [[mqtt-broker-design]] §6 |
| P4 | **MQTT 消息持久化** | Bolt Hook，Broker 重启后恢复 session/retained message | [[mqtt-broker-design]] §7 |
| P4 | **WebSocket TLS** | WSS 支持 | |
| P4 | **Modbus RTU** | 串口设备接入 | roadmap Phase 6 |
| 远期 | **命令下发** | 从 MQTT `device/{SN}/command` 接收指令，转发到对应边缘客户端 | |
| 远期 | **HTTP API** | RESTful 接口供外部系统查询历史数据、管理设备 | |
| 远期 | **Web 管理界面** | 设备管理、实时数据可视化、在线配置编辑 | |

---

## 四、开发路线图

### Phase 0：服务端数据通路打通 ✅ 已完成

**成果**：`internal/output/` 包实现 Sinker/Formatter/OutputRouter 三层 Push 架构；支持 ThingsBoard Gateway MQTT + HTTP 两种传输；按 DeviceName 分组 fan-out；`[[outputs]]` TOML 数组驱动。

**修复**：客户端-服务端 MQTT Broker 同占 1883 端口冲突，客户端改为 1884。

---

### Phase 1：MQTT Pull 数据桥接 ✅ 已完成（2026-07-02）

**成果**（`82c607a` → `78971b1`）：
- `MqttPublisher` 解除 `NopPublisher`，正式接入 comqtt Broker
- `serializeReadings` 将 `[]*model.Reading` 序列化为 JSON 批量数组
- 数据发布到 `device/{SN}/reading` topic
- `cmd/mqtt-sub` 测试订阅工具（实时打印读数日志）
- HTTP Push Sinker 增加 HTTPS 支持

**验证**：外部 MQTT 客户端订阅 `device/{SN}/reading` 可收到结构化 JSON 数据。

---

### Phase 2：日志与测试规范化 ✅ 已完成（2026-07-01/02）

**成果**：
- `internal/client/log.go` — `logReading()` 统一列宽格式输出，集中管理所有数据源（Modbus/MQTT）的采集日志
- 全面单元测试覆盖：`model_test.go`（254行）、`payload_test.go`（461行）、`formatter_test.go`（236行）、`output_test.go`（331行）、`connections_test.go`（145行）、`server_test.go`（365行）
- 配置文件 `.example` 模板化（敏感信息不提交 Git）

---

### Phase 2.5：UploadAck 确认机制 ✅ 已完成（2026-07-03）

**成果**：
- 服务端：入库后发送 `UploadAck(Ok/Failed)`，SHA256 payload 去重（LRU 200条），解析失败也回 Failed
- 客户端：msgRouter 路由分发、uploadReadings 滑动窗口等 ack（3s 超时回退 NotSent）、sendHeartbeat 同步等回包、连续 3 轮全超时触发重连
- 设计文档：`项目/LEPG/UploadAck协议实现设计.md`

---

### Phase 2.6：Modbus 写操作（FC5/6/16）✅ 已完成（2026-07-04）

**成果**：
- `internal/client/modbus_runtime.go` — ModbusRuntime 包装层：共用连接+双 goroutine（poll+write）、状态机连接管理（online/degraded）、同步 Write 接口（内部 scale/offset/byte order 反算）
- `internal/client/modbus_runtime_test.go` — 18 个单元测试：parsePointValue（9）、Write 拒绝条件（7）、Registry、错误构造
- `internal/client/client.go` — ModbusRuntime registry：`GetModbusRuntime()` 供外部按设备名获取运行时
- `internal/client/modbus.go` — 抽取 `parsePointValue`，`ModbusDevicePolling` 改为 Runtime 快捷包装
- 设计文档：`项目/LEPG/Modbus写操作设计.md`

---

### Phase 3：客户端 MQTT 数据校验 ✅ 已完成（2026-07-05）

**审查结论**：原计划"校验设备 SN/点位是否在 MqttConfig 中"是伪需求。`routes` map 从 config 构建，topic 命中后的 deviceName/pointName 必然在配置中，自我验证无意义。

**实际完成（Phase 3b）**：MQTT payload 可配置解析 — 替代硬编码 `{type,value,quality,ts}` 格式。

| 任务 | 文件 | 说明 |
|------|------|------|
| `TopicConfig` 新增 `source` + `data_type` | `internal/client/config.go` | `source="json:path"` / `"plain:"` / `"kv:key"`；`data_type` 强制声明 |
| 三种格式解析器 | `internal/client/mqtt.go` | json（扁平+嵌套点号路径）/ plain（标量）/ kv（KEY=VALUE;...） |
| 类型强校验 | `internal/client/mqtt.go` | float/int/bool/string/json，不匹配报错丢弃 |
| TS 自动识别 | `internal/client/mqtt.go` | Unix ms/s 自动判、RFC3339 多格式兼容；缺失回退系统时间 |
| DeviceHash 缓存 | `internal/client/mqtt.go` | 启动时预计算，每消息查表不再算 SHA-256 |
| `DataType` 迁至 `reading.go` | `internal/model/` | 从 `modbus.go` 迁到 `reading.go`，`modbus.go` 只保留 Modbus 专有类型 |
| `mqtt-sim` 适配 | `cmd/mqtt-sim/main.go` | payload 简化为 `{value,ts}` |
| 配置迁移 | `config/client.toml` | 所有 topic 加 `data_type` + `source="json:value"` |

**验证**：`go build ./...` 通过；老配置必报错（source/data_type 强制要求）；三种格式覆盖 90-95% 工业 MQTT 网关场景。

---

### Phase 4：MQTT 认证与 ACL（预计 2-3 天）

**目标**：MQTT Broker 具备基本安全能力

| 任务 | 文件 | 说明 |
|------|------|------|
| 自定义 AuthHook（SN/Token 认证） | `internal/server/mqtt.go` | `OnConnectAuthenticate` + `OnACLCheck` |
| ACL 规则 | 新文件 | 设备只能发布自己的 Topic，管理员可订阅通配符 |
| 客户端 MQTT Broker 同步添加认证 | `internal/client/mqtt.go` | |
| 非本地监听安全检查 | `internal/server/mqtt.go` | 非回环地址无认证则拒绝启动 |

**验证**：无凭证客户端被拒绝；设备只能访问自己 SN 下的 Topic

---

### Phase 5：设备生命周期管理 — ⚠️ 部分完成（2026-07-05）

**目标**：设备在线/离线状态感知

| 任务 | 文件 | 说明 |
|------|------|------|
| ~~客户端定时发送心跳~~ ✅ | `internal/client/client.go` | 已实现 |
| ~~服务端检测心跳超时~~ ✅ | `internal/server/server.go` | 已实现 |
| **MQTT 设备离线通知** ✅ | `internal/client/mqtt.go` + `client.go` | comqtt statusHook（OnDisconnect expire=true）→ notifyCh → sessionLoop select → MsgTypeNotify TCP 发送；notifyCh 非阻塞写入 |
| MQTT 设备上线通知 | `internal/client/mqtt.go` | 待 Phase 4 ACL 一起实现 |
| Modbus 设备离线检测 | `internal/client/modbus.go` | 待做：`OfflineThreshold` 超时检测 + 复用同一 notifyCh |
| PostgreSQL 设备状态更新 | `internal/server/cache/postgres.go` | 待做：`status` 字段同步 |

**架构**：`uploadLoop` → `sessionLoop`（重命名），新增 `notifyCh <-chan msg.NotifyPayload` 参数。notify 和 upload 在同一条 TCP 连接上并行发送，不经过 SQLite。

---

### Phase 6：Modbus RTU（预计 1-2 天）

**目标**：补全 Modbus RTU 设备接入

~~FC5/FC6/FC16 写操作~~ ✅ 已在 Phase 6 提前完成：`ModbusRuntime` 实现共用连接+双 goroutine、状态机连接管理、同步 Write 接口含值反算（scale/offset/byte order）、FC/DataType 兼容校验、单元测试（18 tests）+ modbus-sim 集成验证。详见 [[Modbus写操作设计]]。

| 任务 | 文件 |
|------|------|
| RTU 轮询 | `internal/client/modbus.go` |
| 离线检测（`enable_monitor` + `offline_threshold`） | `internal/client/modbus.go` |

**验证**：RTU 设备正常采集；写寄存器操作成功

---

### Phase 7：TLS 加密隧道 + 自适应心跳（预计 2 天）

| 任务 | 说明 |
|------|------|
| 客户端/服务端 TLS 连接 | TCP 隧道启用 TLS |
| WebSocket TLS | MQTT Broker WSS |

---

### Phase 8：规则引擎与运维增强（预计 2-3 天）

| 任务 | 说明 |
|------|------|
| ~~上传失败后自动重连 + 续传~~ ✅ | 已实现：`LoadPendingReadings` 覆盖 `NotSent + Failed`；`uploadLoop` 有外层 reconnect loop |
| 基础规则引擎 | 阈值告警、简单计算（求平均、差值） |
| Access 和 CacheEnabled 配置生效 | 当前解析了但未使用 |
| HTTP 健康检查端点 `/healthz` | Docker/K8s 探活必备 |

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
已完成 ✅
  ├── Phase 0     Push 数据通路（TB Gateway MQTT + HTTP）
  ├── Phase 1     MQTT Pull 数据桥接（device/{SN}/reading）
  ├── Phase 2     日志规范化 + 单元测试覆盖 + .example 模板 + HTTPS 支持
  ├── Phase 2.5   UploadAck 确认机制（入库确认 + SHA256 去重 + 超时重试 + 触发重连）
  ├── Phase 2.6   Modbus 写操作（FC5/6/16）
  ├── Phase 3     MQTT payload 可配置解析（source + data_type + TS 自动识别 + DeviceHash 缓存）
  └── Phase 5 ⚠️  MQTT 设备离线通知（statusHook→notifyCh→MsgTypeNotify）

高 →
  ├── Phase 4   MQTT 认证 ACL + 上线通知
  ├── Phase 5   Modbus 离线检测 + PostgreSQL 设备状态
  └── Phase 7   健康检查端点

中 →
  ├── Phase 6   Modbus RTU
  ├── Notify 协议处理
  ├── TTL 数据清理
  └── TB 属性上报

低 →
  ├── Modbus RTU
  ├── Prometheus 指标导出
  └── 规则引擎

远期 →
  ├── Web UI、集群、OPC-UA 等
  └── TLS/WSS
```

---

## 相关笔记

- [[mqtt-broker-design|MQTT Broker 设计]]
- [[handshake-heartbeat-analysis|握手与心跳逻辑分析报告]]
- [[device-state-analysis|设备状态记录实现状态分析报告]]
- [[authentication|网关认证逻辑]]
- [[message-protocol|消息协议]]
- [[overview|配置系统总览]]
- [[数据输出模块最终设计|数据输出模块最终设计]]
- [[端口分配一览|端口分配一览]]
- [[ThingsBoard-Gateway-Payload格式|ThingsBoard Gateway Payload 格式]]
- [[功能现状与价值新增分析报告|功能现状与价值新增分析报告]]
