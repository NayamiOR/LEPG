---
title: 握手与心跳逻辑分析报告
description: 基于 main/f32820a 的 LEPG 握手鉴权（已实现）与心跳协议（协议就绪、业务未接入）现状分析
type: report
tags: [handshake, heartbeat, report, connection, auth]
aliases: [握手与心跳逻辑分析报告]
---

# LEPG Server 与 Client 握手/心跳逻辑分析报告

> 适用范围：`internal/server/` 与 `internal/client/`，并涵盖其依赖的消息协议层 `internal/msg/`。
> 状态基准：`main` 分支，提交 `f32820a`。

## 一、总体结论（先读这段）

| 能力 | 协议层定义 | 业务层实现 | 状态 |
|------|:---:|:---:|------|
| 握手（Handshake / HandshakeAck） | ✅ | ✅ | **已完整实现** |
| 鉴权（SN + Token） | ✅ | ✅ | **已实现**（白名单线性匹配） |
| 心跳（Heartbeat / HeartbeatAck） | ✅ | ❌ | **协议就绪，业务未接入** |
| 心跳超时检测 | — | ❌ | **未实现**（无 ReadDeadline / 无计时器） |
| 连接状态机 | — | ❌ | **未实现**（状态隐含在执行流中） |
| 首次连接重试 | — | ✅ | **已实现**（指数退避，上限 60s） |
| 运行时断连重连 | — | ❌ | **未实现**（断连即退出） |
| ConnectionManager | ✅ | ❌ | **已定义并实现，但未接入** |

**一句话总结**：握手这条路是通的、可用的；心跳这条路只修好了"路标（协议定义）"和"车辆（编解码）"，但两端都没有发车，也没有看表（超时检测）。

---

## 二、消息协议层基础（握手/心跳的底层支撑）

### 2.1 TLV 帧格式

每条消息的线上格式（大端序）：

```
[Magic:2B][Version:1B][Flags:1B][Type:1B][MsgID:2B][PayloadLen:2B][Timestamp:4B][Payload:N][Checksum:2B]
 偏移0     偏移2      偏移3      偏移4    偏移5      偏移7          偏移9        偏移13     末尾
```

- 固定帧头 13 字节，末尾 2 字节 CRC16，总长 = `13 + PayloadLen + 2`。
- `Magic = 0x4E59`（ASCII "NY"），`Version` 当前硬编码为 `1`，`Flags` 预留为 `0`。
- 定义位置：[internal/msg/msg.go](../../internal/msg/msg.go) 字段尺寸常量与 `Msg` 结构体。

### 2.2 消息类型常量

[internal/msg/msg.go](../../internal/msg/msg.go)（`iota + 1` 自增）：

```go
MsgTypeHandshake    = 0x01  // 握手      client -> server
MsgTypeHandshakeAck = 0x02  // 握手回应  server -> client
MsgTypeUpload       = 0x03  // 上传数据  client -> server
MsgTypeUploadAck    = 0x04  // 上传回应  server -> client（注：当前 server 未发）
MsgTypeHeartbeat    = 0x05  // 心跳      client -> server
MsgTypeHeartbeatAck = 0x06  // 心跳回应  server -> client
MsgTypeNotify       = 0x07  // 通知      server -> client
```

### 2.3 Reason Code（AckPayload.Code）

```go
Ok       = 1   // 成功
Failed   = 2   // 通用失败（如首条消息不是 Handshake、payload 解析失败）
BadSn    = 3   // SN 不在白名单
BadToken = 4   // SN 匹配但 Token 不一致
```

### 2.4 关键 Payload 结构

| Payload | 用途 | 线上格式 | 实现位置 |
|---------|------|----------|----------|
| `HandshakePayload` | 握手请求 | `[FirmwareVersion:1B][SnLen:1B][Sn:nB][TokenLen:1B][Token:nB]` | [msg.go](../../internal/msg/msg.go) + [payload.go](../../internal/msg/payload.go) |
| `HeartbeatPayload` | 心跳请求 | **空（0 字节）**，解码时严格要求 `len==0` | 同上 |
| `AckPayload` | 握手/上传/心跳的通用回应 | `[MsgID:2B][Code:1B]`（固定 3 字节） | 同上 |

要点：
- `HeartbeatPayload` 是**空结构体**，`Encode()` 返回 `nil`，`Decode()` 校验长度必须为 0。
- `AckPayload` 被**三种 Ack 类型复用**（HandshakeAck / UploadAck / HeartbeatAck 共用同一个 `decodeAck`）。
- `AckPayload.MsgID` 回写"被回复的请求消息 ID"，便于请求方配对。

### 2.5 消息工厂与注册机制

- **工厂**：`MsgFactory.NewMsg(type, payload)` 一次性完成"编码 payload → 填充帧头 → 自动生成自增 MsgID（atomic，模 65536）→ 计算 CRC16"。调用方无需手动拼帧。
- **注册表**：`RegisterPacketType(type, decodeFn)` + `ParseMsg(msg)` 实现反向解析。`payload.go` 的 `init()` 注册了 7 种类型的解码器。
- **接口**：所有 Payload 实现 `Packable{ Encode(); Decode() }`。

### 2.6 校验与时间戳

- **CRC16-CCITT**（[internal/utils/checksum.go](../../internal/utils/checksum.go)）：多项式 `0x1021`，初值 `0xFFFF`。**校验范围 = 帧头 13B + Payload**，不含自身 2B。编码时由工厂自动算，解码时由 `DecodeFrame` 重算并比对，不符返回 `ErrChecksumMismatch`。
- **时间戳**（[internal/utils/timestamp.go](../../internal/utils/timestamp.go)）：自定义 epoch = `2020-01-01`（节省表示范围），`uint32`，每条消息由工厂自动打戳。**当前未用于超时/防重放判断，仅作为消息元信息记录。**

---

## 三、握手流程（已完整实现）

### 3.1 端到端时序

```
        Client                                    Server
          │                                         │
          │  ── Handshake (0x01) ─────────────────►  │  HandleConnection: 阻塞读首帧
          │   Payload: {FW=1, Sn, Token}            │  校验类型==0x01，解析 payload
          │                                         │  线性匹配 Sn → 比对 Token
          │                                         │
          │  ◄──────────── HandshakeAck (0x02) ───  │  Code = Ok / BadSn / BadToken / Failed
          │   Payload: {MsgID, Code}                │
          │                                         │
   Code==Ok?  否 → 握手失败，uploadLoop 退出        │
          │  是                                     │
          │  ── Upload (0x03) ───────────────────►  │  进入消息循环
          │  ...                                    │
```

### 3.2 Client 发起握手

[internal/client/client.go](../../internal/client/client.go) `performHandshake`（行 271–312）：

```go
func performHandshake(conn net.Conn, cfg *ClientConfig, factory *msg.MsgFactory) error {
    // 1. 构造并发送 Handshake（FW 硬编码为 1，Sn/Token 来自配置）
    hsMsg, _ := factory.NewMsg(msg.MsgTypeHandshake, &msg.HandshakePayload{
        FirmwareVersion: 1,
        Sn:              cfg.Sn,
        Token:           cfg.Token,
    })
    encoded, _ := hsMsg.Encode()
    conn.Write(encoded)

    // 2. 阻塞读取 HandshakeAck
    respMsg, _ := msg.DecodeFrame(conn)
    if respMsg.Type != msg.MsgTypeHandshakeAck { ... }

    // 3. 解析 AckPayload，校验 Code == Ok
    ack, _ := msg.ParseMsg(&respMsg).(*msg.AckPayload)
    if ack.Code != msg.Ok {
        return fmt.Errorf("handshake rejected (code=%d): %w", ack.Code, errors.ErrHandshakeRejected)
    }
    return nil
}
```

调用点：`uploadLoop`（行 140）成功 dial 之后立即调用，**握手失败直接 `return`，不重连**。

### 3.3 Server 处理握手与鉴权

[internal/server/server.go](../../internal/server/server.go) `HandleConnection`（行 34–90），握手是连接建立的**第一步阻塞操作**，鉴权失败即关闭连接：

```go
// 1. 阻塞等待首帧
hsMsg, err := msg.DecodeFrame(conn)
// 2. 必须是 Handshake
if hsMsg.Type != msg.MsgTypeHandshake {
    sendHandshakeResponse(conn, factory, hsMsg.MsgID, msg.Failed) // Code=2
    return
}
// 3. 解析 payload
hsPayload, _ := msg.ParseMsg(&hsMsg).(*msg.HandshakePayload)
// 4. SN 白名单线性匹配（O(n)）
var matched *ClientDef
for i := range clients {
    if clients[i].Sn == hsPayload.Sn { matched = &clients[i]; break }
}
if matched == nil { sendHandshakeResponse(..., msg.BadSn);  return } // Code=3
if matched.Token != hsPayload.Token { sendHandshakeResponse(..., msg.BadToken); return } // Code=4
// 5. 鉴权通过
sendHandshakeResponse(conn, factory, hsMsg.MsgID, msg.Ok)            // Code=1
```

白名单来源：`ServerConfig.Clients []ClientDef`（TOML `[[clients]]` 节，含 `sn/token/description`），定义于 [internal/server/config.go](../../internal/server/config.go)。

回应函数 `sendHandshakeResponse`（行 156–170）：用工厂构造 `HandshakeAck`（`AckPayload.MsgID` = 请求方 MsgID，`Code` = 鉴权结果），`conn.Write` 发出。

### 3.4 握手鉴权特点

- **白名单匹配**：只有配置文件中预先登记的 `Sn` 才允许连接。
- **SN 与 Token 分级失败码**：未登记 → `BadSn`；登记但 Token 错 → `BadToken`，便于客户端/运维定位。
- **无超时保护**：`DecodeFrame` 是阻塞 `io.ReadFull`，若客户端建连后不发握手，Server goroutine 将一直阻塞到 OS 级 TCP keepalive 超时。**握手阶段同样无 ReadDeadline。**

---

## 四、心跳流程（协议层就绪，业务层未接入）

### 4.1 协议层：已完备

- 类型常量 `MsgTypeHeartbeat=0x05` / `MsgTypeHeartbeatAck=0x06` 已定义。
- `HeartbeatPayload`（空）已实现编解码。
- `decodeHeartbeat` 与 `decodeAck`（用于 HeartbeatAck）已在 `init()` 注册。
- 即：**心跳消息可以被正确地创建、发送、接收、解析**。

### 4.2 Server 端：不处理心跳

`HandleConnection` 的消息循环（行 94–153）**只识别 `MsgTypeUpload`**：

```go
for {
    message, err := msg.DecodeFrame(conn)
    if err != nil { ...; return }
    // 只有这一个分支：
    if message.Type == msg.MsgTypeUpload { ... 处理上传 ... }
    // 没有 if message.Type == msg.MsgTypeHeartbeat 的分支
}
```

后果：若 Client 真发来心跳，Server 仅打一条 `"received message" type=heartbeat` 日志，**不会回复 HeartbeatAck，不会更新任何心跳时间戳**。

### 4.3 Client 端：不发送心跳

`uploadLoop`（行 126–175）只有一个 `timer`，周期由 `UploadInterval` 驱动，用于**上传数据**，**没有独立的心跳定时器**，全代码库无 `HeartbeatInterval` 配置项。

### 4.4 未实现的心跳能力清单

1. **Client 定时发送 Heartbeat**（缺定时器、缺配置项 `HeartbeatInterval`）。
2. **Server 接收 Heartbeat 并回复 HeartbeatAck**（缺 `MsgTypeHeartbeat` 分支，缺 `sendHeartbeatResponse`）。
3. **心跳超时检测**：Server 侧无 `SetReadDeadline`、无 `time.Ticker`、无 `select-timeout`，无法发现"静默断连"的客户端。
4. **心跳失败重连**：Client 侧无心跳失败 → 重连的触发路径。

> 注：[internal/client/upload_loop_example.go](../../internal/client/upload_loop_example.go) 中出现的字符串 `"heartbeat check"` 只是 mock 数据，与心跳机制无关。

---

## 五、连接生命周期与重连

### 5.1 Server 端：goroutine-per-connection

[internal/server/server.go](../../internal/server/server.go) `ReceiveLoop`（行 14–32）：

```go
for {
    conn, err := ln.Accept()
    go HandleConnection(conn, cfg.Clients, s, publisher)  // 每连接一个 goroutine
}
```

- `HandleConnection` 开头 `defer conn.Close()`（行 35）是**唯一的清理动作**。
- **无连接状态机**：状态隐含在流程中（等待握手 → 鉴权中 → 已认证 → 断开）。
- **无全局连接表**：`HandleConnection` 无状态，不维护"当前在线设备"映射。
- **无断线通知**：断开时仅打日志，不发 MQTT 上下线通知，不从连接管理器移除。
- **无同 SN 重复连接检测**：同一 SN 可多连接并存。

### 5.2 Client 端：uploadLoop 生命周期

[internal/client/client.go](../../internal/client/client.go)：

```
MainFunc（行 16）
 ├─ goroutine: MQTT broker（可选）
 ├─ goroutine: Modbus 轮询（可选）  ┐
 ├─ goroutine: producerWg.Wait → close(ch)
 ├─ goroutine: consumeAndWrite：ch → SQLite
 └─ goroutine: uploadLoop：SQLite → TCP Server  ◄── 握手+上传在此
        └─ mainWg.Wait() 阻塞，任一 goroutine 退出都会收敛
```

数据流：`设备/MQTT → channel → SQLite → uploadLoop → TCP → Server`。所有 goroutine 通过 `ctx.Done()` 协作退出。

### 5.3 重连机制：仅首次连接

`dialWithRetry`（行 233–269）提供**首次连接**的指数退避：

```go
for attempt := 0; attempt < cfg.MaxRetry; attempt++ {
    // 每轮先检查 ctx.Done()，支持优雅退出
    conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
    if err == nil { return conn, nil }
    backoff := baseInterval * (1 << attempt)   // baseInterval = RetryInterval
    if backoff > 60*time.Second { backoff = 60 * time.Second }  // 上限 60s
    // 等 backoff 或 ctx.Done()
}
```

配置项（[internal/client/config.go](../../internal/client/config.go)）：`MaxRetry`（默认 10）、`RetryInterval`（默认 5000ms）。

**关键缺口——运行时无重连**：`dialWithRetry` 仅在 `uploadLoop` 开头调用**一次**。一旦进入上传循环：
- `conn.Write` 失败（`uploadReadings` 返回 error）→ `uploadLoop` 直接 `return`（行 169–172）。
- **不会重新 `dialWithRetry` + `performHandshake`**。
- 即：**网络抖动一次，上传就永久停止，整个客户端随之退出。**

### 5.4 握手失败同样不重连

`performHandshake` 返回 error 时，`uploadLoop`（行 140–143）直接 `return`，不重试握手、不重连。

---

## 六、ConnectionManager（已定义并实现，但完全未接入）

[internal/server/cache/connections/](../../internal/server/cache/connections/) 定义了连接管理基础设施：

- **接口** `ConnectionManager`：含 `RegisterConnection` / `UpdateHeartbeat` / `GetConnection` / `RemoveConnection` / `ListConnections` 等。
- **`Connection` 结构体**：已预留 `ConnectedAt`、`LastHeartbeat` 字段。
- **两种实现**：`MemoryConnectionManager`（内存 map）、`RedisConnectionManager`（Pipeline 更新）。

但经全项目搜索，**`ConnectionManager` 未被任何代码导入**：`ReceiveLoop` / `HandleConnection` 的函数签名中都没有它，`UpdateHeartbeat` 从未被调用。属于"造好了零件但还没装上车"。

---

## 七、现状评估与风险点

### 7.1 已实现且可用
1. ✅ 握手双向链路（Handshake ↔ HandshakeAck）。
2. ✅ SN/Token 白名单鉴权，分级失败码（BadSn/BadToken/Failed/Ok）。
3. ✅ 协议层心跳编解码（含空 Payload、通用 Ack 复用）。
4. ✅ 首次连接指数退避重试（上限 60s，响应 ctx 优雅退出）。
5. ✅ CRC16 帧校验、自定义 epoch 时间戳。

### 7.2 未实现 / 风险点
1. ⚠️ **心跳业务层完全缺失**：协议就绪但两端不发不收，连接活性检测形同虚设。
2. ⚠️ **无读超时**：Server 侧无 `SetReadDeadline`，半开连接（客户端崩溃/拔网线）会留下长期阻塞的 goroutine，直至 OS keepalive 超时（分钟级）。
3. ⚠️ **运行时断连不重连**：上传循环中任一 `Write` 失败即终止，一次抖动导致客户端永久离线，需人工重启。
4. ⚠️ **无连接状态机 / 全局连接表**：无法感知在线设备、无法触发上下线事件、无法做同 SN 互斥。
5. ⚠️ **ConnectionManager 未接入**：`LastHeartbeat`/`UpdateHeartbeat` 形同虚设。
6. ⚠️ **无心跳间隔配置项**：`ClientConfig` 中缺 `HeartbeatInterval`/`HeartbeatTimeout`。

### 7.3 改进方向（仅作梳理，不在本次范围）
- 心跳落地：Client 加心跳定时器 + 配置项；Server 加 `MsgTypeHeartbeat` 分支回 Ack + `SetReadDeadline` 超时清理。
- 运行时重连：把 `dialWithRetry + performHandshake` 包成可重入的"建立会话"函数，`uploadLoop` 捕获 Write/握手失败后进入重连循环。
- 接入 ConnectionManager：鉴权成功 `RegisterConnection`，心跳到达 `UpdateHeartbeat`，断开 `RemoveConnection`。

---

## 相关笔记

- [[message-protocol|消息协议]]（TLV 帧与消息类型定义）
- [[device-state-analysis|设备状态记录实现状态分析报告]]（ConnectionManager 未接入的下游影响）
- [[authentication|网关认证逻辑]]
- [[roadmap|开发路线图]]
- [[client-config|客户端配置]]
