---
title: 设备状态记录实现状态分析报告
description: 基于 main 分支的 LEPG 设备状态记录现状——网关在线/离线已接线（Redis），边缘设备状态/存活通路仍全断
type: report
tags: [device-state, connection, report, mqtt, cache]
aliases: [设备状态记录实现状态分析报告]
---

# LEPG 设备状态记录实现状态分析报告

> 适用范围：`internal/server/` 及其数据/连接基础设施（`cache/`、`cache/connections/`、`cache/migrations/`），并涵盖上游消息协议层 `internal/msg/`、数据模型 `internal/model/` 与客户端设备定义 `internal/client/`。
> 状态基准：`main` 分支，提交 `88f340b` 及后续（网关状态接线已完成）。

## 一、总体结论（先读这段）

| 能力 | 定义层 | 实现层 | 接入层 | 状态 |
|------|:---:|:---:|:---:|------|
| 网关在线/离线记录 | ✅ | ✅ | ✅ | **已接线（Redis），启动探活、注册/心跳/清理全贯通** |
| 网关接入登记（RegisterConnection） | ✅ | ✅ | ✅ | 握手成功后调用，SN 作 DeviceHash，uuid 作 ConnectionID |
| 心跳刷新（UpdateHeartbeat） | ✅ | ✅ | ✅ | `case MsgTypeHeartbeat` 分支已接入 `connMgr.UpdateHeartbeat` |
| 网关断开清理（RemoveConnection） | ✅ | ✅ | ✅ | 握手成功后注册 defer，连接退出时自动清理 Redis key |
| 同 SN 互斥 / 全局连接表 | ❌ | ❌ | ❌ | 无 |
| MQTT 上下线通知（`device/{sn}/status`） | ✅ | ❌ | ❌ | topic 已定义，从未发布 |
| Notify 协议（0x07 上线/离线/故障事件） | ✅ | ✅ | ❌ | 两端均不发不收 |
| 边缘设备数值记录（readings） | ✅ | ✅ | ✅ | **已实现并接线**（仅数值，非状态） |
| 边缘设备注册表（devices 表） | ✅ | ✅ | ✅ | **已实现**：`devices` 表 + `SaveReadings` upsert + `QueryDevices` 查询 |
| 边缘设备在线/离线检测 | ⚠️ | ❌ | ❌ | client 有 `OfflineThreshold`/`EnableMonitor` 字段，但轮询循环不消费 |
| 设备元数据（型号/连接/点位）上送 | ❌ | ❌ | ❌ | 设备定义只存在于 client 配置，server 无感知 |

**一句话总结**：边缘设备的**数值**这条记录通路是通的（readings 表）；**网关状态**（在线/离线/心跳）已通过 `RedisConnectionManager` 接线落地到 Redis（key `lepg:device:{SN}`）；**设备注册表**（devices 表 + SaveReadings upsert + QueryDevices）已实现，server 可枚举各网关下挂设备；边缘设备的**状态/存活**通路仍是"零件造好了（`NotifyPayload`、`OfflineThreshold` 字段）却没装上车"——离线检测与 Notify 协议尚未接线。

---

## 二、概念界定

本文把"设备状态记录"拆成两条独立的链路，分别评估：

- **网关状态（Gateway State）**：以 SN 标识的一条 TCP 连接的生命周期——在线/离线、接入时间（ConnectedAt）、最近心跳（LastHeartbeat）、客户端 IP、所在服务节点。一条连接 = 一个网关实例的当前会话。
- **边缘设备状态（Edge Device State）**：网关下挂的 Modbus 从站 / MQTT 虚拟设备——设备是否存在（注册）、是否存活（在线/离线）、最近一次上报、以及型号/连接方式/点位等元数据。

两者的记录现状截然不同：数值数据只走"边缘设备状态"这条线，且只覆盖"数值"不覆盖"状态"。

---

## 三、网关状态记录（已接线 ✅）

### 3.1 基础设施：双实现齐全

[internal/server/cache/connections/](../../internal/server/cache/connections/) 提供了完整的连接管理零件：

- [connections.go](../../internal/server/cache/connections/connections.go) 定义 `Connection` 结构体与 `ConnectionManager` 接口：

  ```go
  type Connection struct {
      DeviceHash    string
      ConnectionID  string
      ServerNode    string
      ClientIP      string
      ConnectedAt   utils.Timestamp
      LastHeartbeat utils.Timestamp
      NodeAddr      string // 默认用 pod ip，本机模式留空
  }

  type ConnectionManager interface {
      RegisterConnection(conn *Connection) error
      UpdateHeartbeat(connection_id string) error
      GetConnection(device_hash string) (*Connection, error)
      RemoveConnection(device_hash string) error
      ListConnections() ([]*Connection, error)
      GetDeviceConnection(device_hash string) (*Connection, error)
  }
  ```

- [memory.go](../../internal/server/cache/connections/memory.go)：`MemoryConnectionManager`，`sync.RWMutex` + `map[string]*Connection`，**六个方法全部实现**。
- [redis.go](../../internal/server/cache/connections/redis.go)：`RedisConnectionManager`，key 前缀 `lepg:device:`，用 `HSet`/Pipeline 写入 hash、`Keys("lepg:device:*")` 扫描列表，**六个方法全部实现**。

即"零件"层面：结构体字段齐全（已预留 `ConnectedAt`/`LastHeartbeat`），两套后端（内存/分布式）都造好了。

### 3.2 已接入（main 分支，commit 88f340b 及后续）

`ConnectionManager` 已接入 server 连接生命周期。接入点：

- [internal/server/server.go](../../internal/server/server.go) 的 `ReceiveLoop` 与 `HandleConnection` 已增加 `connMgr connections.ConnectionManager` 参数，从 `main.go` 注入 `RedisConnectionManager`。

- 握手鉴权成功后（`sendHandshakeResponse` 之后）：构建 `Connection`（`DeviceHash`=`hsPayload.Sn`、`ConnectionID`=`uuid.NewString()`、`ClientIP`=`remoteAddr`），调用 `connMgr.RegisterConnection`，并注册 `defer connMgr.RemoveConnection(hsPayload.Sn)` 确保断开时清理。注册失败为非致命（记 warn）。

- 消息循环的 `case msg.MsgTypeHeartbeat` 分支（[server.go:191](../../internal/server/server.go#L191)）：在发送心跳 ACK 前调用 `connMgr.UpdateHeartbeat(hsPayload.Sn)`，失败为非致命（记 warn）。

- [cmd/server/main.go](../../cmd/server/main.go)：从 `cfg.Redis` 构建 `redis.NewClient`，启动时 `Ping` 探活（失败则 `os.Exit(1)`），创建 `connections.NewRedisConnectionManager(rdb)`，传入 `ReceiveLoop`。

即网关的在线/离线记录已通过 Redis（key `lepg:device:{SN}`）落地，`ConnectedAt` / `LastHeartbeat` 会随连接生命周期自动更新。可回答"当前哪些网关在线""某网关何时上线""它多久没心跳了"等问题（通过 `redis-cli KEYS "lepg:device:*"` 或后续查询接口）。

### 3.3 出口通道也全是空的

即便有了状态，server 也无处对外广播：

- [internal/server/mqtt.go:17](../../internal/server/mqtt.go#L17) 定义了 `TopicStatus = "device/%s/status"`，但**全代码库无任何 `Publish` 调用指向它**。
- [internal/server/publisher.go](../../internal/server/publisher.go) 的 `EventPublisher` 接口只有 `PublishDeviceReadings`，**没有** `PublishDeviceStatus` / `PublishDeviceEvent`，也无对应实现。
- [internal/msg/msg.go:280](../../internal/msg/msg.go#L280) 的 `NotifyPayload` 已为状态事件预留了完整字段：

  ```go
  type NotifyPayload struct {
      EventCode  uint8  // 0x01=上线, 0x02=离线, 0x10=传感器故障
      DeviceHash string
      Timestamp  uint32
      Severity   uint8
      Message    string
      RawData    []byte
  }
  ```

  协议层已就绪（编解码已实现、`init()` 已注册），但 **client 不发、server 不解析**。这是为网关↔server 设备状态事件准备的协议，目前零使用。

---

## 四、边缘设备状态记录（数值已通，状态未通）

### 4.1 数值记录：已实现并接线 ✅

边缘设备数值是目前**已落地**的设备相关记录之一（另一项是网关在线/离线记录，见 §3.2）：

```
client 轮询 → model.Reading → UploadPayload.Readings（gob）→ server
server: parse Upload → SQLiteStore.SaveReadings
```

- 接入点：[internal/server/server.go:128-151](../../internal/server/server.go#L128)，解析 `UploadPayload` 后调用 `s.SaveReadings(...)`。
- 存储：[internal/server/cache/store.go](../../internal/server/cache/store.go) 的 `StoredReading`（含 `sn`/`device`/`device_name`/`point`/`point_name`/`value`/`quality`/`timestamp` 等）。
- 建表：[internal/server/cache/migrations/001_init.go](../../internal/server/cache/migrations/001_init.go) 创建 `readings` 表，含索引 `idx_readings_sn_ts`、`idx_readings_dev_pt`。
- 查询：[internal/server/cache/sqlite.go](../../internal/server/cache/sqlite.go) 的 `QueryReadings`，支持按 `sn`/`device`/时间范围过滤。

注意：记录的是**数值快照**，不是状态。但因为它携带了 `device`(hash) 和 `device_name`，所以**能从 readings 反推出"某网关下出现过哪些设备"**——这是目前 server 感知边缘设备存在的唯一途径。

### 4.2 设备注册表：已实现 ✅

server 侧现已具备设备目录能力：

- `devices` 表（[002_devices.go](../../internal/server/cache/migrations/002_devices.go)）：字段 `sn` / `device_hash` / `device_name` / `type` / `first_seen` / `last_seen` / `status`，UNIQUE(sn, device_hash)。
- `SaveReadings` 内自动 upsert（[sqlite.go](../../internal/server/cache/sqlite.go)）：每个 upload 批次去重后更新 `last_seen`，首次出现时记录 `first_seen`。
- `QueryDevices` 接口：按 `sn` 查询网关下所有设备，支持分页。
- `type` 字段当前为可空——server 无法从 reading 获知连接类型(rtu/tcp/mqtt)，留待后续元数据上送填充。

### 4.3 在线/离线检测：配置有、逻辑无 ⚠️

- [internal/client/config.go:229-230](../../internal/client/config.go#L229) 的 `DeviceConfig` 有 `OfflineThreshold`（默认 30s）与 `EnableMonitor`（默认 true）字段，看起来是给离线检测准备的。
- 但 [internal/client/modbus.go](../../internal/client/modbus.go) 的 `ModbusDevicePolling` 读取失败时只做一件事：

  ```go
  if err != nil {
      slog.Error("Failed to read holding registers", "error", err)
      continue   // 直接进入下一轮，无计时、无状态机、不发 Notify
  }
  ```

  **没有计时器、没有离线判定、不触发 `NotifyPayload`(0x02)**。这两个字段目前唯一用途是启动时 [formatModbusDevice](../../internal/client/config.go#L503) 打印设备列表（展示 `offline=30s monitor=true`）。

- server 侧更无从得知边缘设备存活——因为根本没有 `MsgTypeNotify` 接收逻辑。

### 4.4 数据出口（MQTT）：被桩实现挡住

- [cmd/server/main.go:90-91](../../cmd/server/main.go#L90)：

  ```go
  // TODO: 数据桥接阶段替换为 server.NewMqttPublisher(broker)
  var publisher server.EventPublisher = new(server.NopPublisher)
  ```

  即便数值已落库，`PublishDeviceReadings` 也被 `NopPublisher`（空实现）吞掉。所以 readings 目前**只进 SQLite、不进 MQTT**，下游订阅者（`device/{sn}/reading`）看不到任何数据。

---

## 五、现状数据流

```
                ┌─ SaveReadings → SQLite (readings 表)  ✅ 已落地
边缘设备 ─Modbus/MQTT→ client.Reading ─gob/Upload→ server
                └─ PublishDeviceReadings              ❌ NopPublisher 吞掉

网关上下线     ──✅──  握手 → RegisterConnection → Redis (lepg:device:{SN})
                                        心跳 → UpdateHeartbeat → 刷新 last_heartbeat
                                        断开 → RemoveConnection → DEL key            ✅
边缘设备上下线 ──×──  无任何记录 / 无离线判定 / 无 Notify                 ❌
```

简言之："边缘设备的数值"（readings）+ "网关在线/离线"（Redis `lepg:device:*`）两条线已落地；"边缘设备状态/存活/元数据"通路仍断裂。

---

## 六、缺口清单与改进方向（仅作梳理，不在本次范围）

按建议优先级：

1. **网关状态接线** ✅（已完成）
   - `cmd/server/main.go` 从 `RedisConfig` 构建 `redis.Client`，启动 Ping 探活，创建 `RedisConnectionManager` 并注入 `ReceiveLoop`。
   - `HandleConnection` 鉴权成功 → `RegisterConnection`（`DeviceHash`=SN、`ConnectionID`=uuid）；断开 → defer `RemoveConnection`。
   - 心跳接入：`case MsgTypeHeartbeat` 分支 → `UpdateHeartbeat`（刷新 `last_heartbeat`）。
   - 待补：`ListConnections` 的查询出口（HTTP/调试接口），让"在线网关列表"可观测。

2. **边缘设备注册表** ✅（已完成）
   - `devices` 迁移与表（`sn`/`device_hash`/`device_name`/`type`/`first_seen`/`last_seen`/`status`，UNIQUE(sn, device_hash)）——见 `internal/server/cache/migrations/002_devices.go`。
   - `SaveReadings` 时按批次去重 upsert 设备行（first_seen 仅首次写入、last_seen 每次更新），`QueryDevices` 可按 SN 枚举网关下所有设备——见 `internal/server/cache/sqlite.go`。
   - 测试覆盖：首次注册、重复更新、批内去重、多设备、跨网关隔离、空批次——见 `internal/server/cache/sqlite_test.go`。

3. **边缘设备离线检测 + Notify**
   - client：在 `ModbusDevicePolling` 内用 `OfflineThreshold` 计时，连续读取失败超过阈值 → 标记离线 → 通过 `MsgTypeNotify`(EventCode=0x02) 上送。
   - server：增 `MsgTypeNotify` 分支，落库（写入设备状态表 / readings 的 quality）+ 可选 MQTT 发布 `device/{sn}/status`。

4. **MQTT 出口接线**
   - `main.go` 改用 `server.NewMqttPublisher(broker)`，让 readings 真正发布到 `device/{sn}/reading`。
   - `EventPublisher` 扩 `PublishDeviceStatus(sn, status)`，发布 `device/{sn}/status`（上线/离线），与第 1、3 步联动。

> 这四项中，第 1、4 项几乎纯接线（零件已就绪），第 2、3 项需要新增表与逻辑。建议作为独立的小步迭代推进，每步可单独验证。

---

## 相关笔记

- [[handshake-heartbeat-analysis|握手与心跳逻辑分析报告]]（心跳未接入的根因）
- [[message-protocol|消息协议]]（NotifyPayload 0x07 定义）
- [[mqtt-broker-design|MQTT Broker 设计]]（status topic 与数据桥接方案）
- [[modbus-config|Modbus 设备配置]]（OfflineThreshold / EnableMonitor 字段）
- [[roadmap|开发路线图]]
