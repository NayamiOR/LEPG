# MQTT Broker 设计文档

LEPG Server 内嵌 MQTT Broker 的后续设计方案。当前框架已完成基础搭建（comqtt v2 集成、MqttBroker 封装、EventPublisher 接口），以下是需要逐步落地的功能模块及其设计取舍。

---

## 1. Topic 设计

### 已确定的命名规范

```
device/{SN}/reading   — 设备采集数据上报
device/{SN}/event     — 设备事件（告警、状态变更等）
device/{SN}/status    — 设备在线/离线状态（Retain 消息）
device/{SN}/command   — 下行指令（云端→设备，预留）
```

### 还需要决定的

| 决策点 | 选项 | 取舍 |
|--------|------|------|
| reading 的 payload 格式 | A) 单条 JSON B) 批量 JSON 数组 C) MessagePack | JSON 易调试、通用性强；MessagePack 省带宽但调试困难。建议先用 JSON，有性能问题再切换 |
| reading 是用单条还是批量 | 一条 Reading 一条消息 vs 一个 Upload 消息对应多条 | 批量更贴合当前 TLV 协议（一个 Upload 包含多条 Reading），减少消息数，但单条更利于下游过滤。建议批量，topic 加 suffix 区分：`device/{SN}/reading/batch` |
| 是否支持通配符订阅 | `device/+/reading` 或 `device/#` | 框架阶段 AllowAll ACL 不限制。后续 ACL Hook 中决定是否允许跨 SN 订阅 |

### 建议的最终 topic 层级

```
device/{SN}/reading           # 采集数据（JSON 批量）
device/{SN}/event             # 设备事件
device/{SN}/status            # 在线状态（Retain，payload: online/offline）
device/{SN}/command           # 下行指令（预留）
$SYS/broker/#                 # comqtt 内置系统主题
```

---

## 2. 数据桥接方案（TLV → MQTT）

### 当前数据流

```
Client → [TLV over TCP] → HandleConnection → gob.Decode → []Reading → SQLite
```

### 目标数据流

```
Client → [TLV over TCP] → HandleConnection → gob.Decode → []Reading
                                                        ├→ SQLite（不变）
                                                        └→ MqttPublisher.PublishDeviceReadings()
                                                           └→ broker.Publish("device/{SN}/reading", jsonPayload)
```

### 实现步骤

1. **定义 JSON 序列化格式** — 在 `internal/server/publisher.go` 中增加 `serializeReadings` 函数：

```go
type MqttReading struct {
    DeviceName string  `json:"device_name"`
    PointName  string  `json:"point_name"`
    DataType   string  `json:"data_type"`
    Value      any     `json:"value"`     // bool 或 float64
    Unit       string  `json:"unit,omitempty"`
    Timestamp  int64   `json:"timestamp"` // 毫秒
}
```

2. **取消 server.go 中的 TODO 注释**，启用 publisher 调用
3. **在 cmd/server/main.go 中** 将 `NopPublisher` 替换为 `MqttPublisher`

### Publish 失败处理

当前 `MqttBroker.Publish` 会 log.Error 但不重试。失败策略：

| 方案 | 复杂度 | 数据安全性 |
|------|--------|-----------|
| A) 只记日志，不重试 | 低 | 丢失 MQTT 侧数据，SQLite 侧不受影响 |
| B) 失败写入本地重试队列 | 中 | 不丢数据，但需要额外 goroutine 和存储 |
| C) 失败时回写 SQLite 标记，由后台任务补发 | 高 | 最可靠，但改表结构 |

**建议**：先用方案 A。SQLite 已经是完整数据源，MQTT 丢失的数据可以从 SQLite 补查。后续如需要可靠性，加一个简单的内存重试队列（带最大重试次数）即可。

---

## 3. 认证方案

### 当前状态

使用 `auth.AllowHook`，任何客户端都能连接。

### 目标：基于 SN/Token 的自定义 Hook

comqtt 的认证通过 `OnConnectAuthenticate` hook 实现。连接时 MQTT 客户端提供 username/password，我们将其映射到现有的 ClientDef（SN/Token）。

```go
// internal/server/mqtt_auth_hook.go
type AuthHook struct {
    mqtt.HookBase
    clients map[string]ClientDef  // key: SN
}

func (h *AuthHook) ID() string { return "lepg-auth" }

func (h *AuthHook) Provides(b byte) bool {
    return b == mqtt.OnConnectAuthenticate || b == mqtt.OnACLCheck
}

func (h *AuthHook) OnConnectAuthenticate(cl *mqtt.Client, pk packets.Packet) bool {
    // username → SN, password → Token
    username := string(pk.Connect.Username)
    password := string(pk.Connect.Password)
    if client, ok := h.clients[username]; ok {
        return client.Token == password
    }
    return false
}

func (h *AuthHook) OnACLCheck(cl *mqtt.Client, topic string, write bool) bool {
    // 订阅时：只允许订阅自己的 device/{SN}/# 或 device/#
    // 发布时：外部客户端不允许发布到 device/ 前缀
    // 详见下一节 ACL 规则
    return true
}
```

### 连接方式

外部系统用 MQTT 客户端连接时：
```
username = 设备 SN（如 "CLIENT001"）
password = 设备 Token（如 "token123456"）
```

### 安全启动检查

当 `MqttConfig.TCPAddr` 不是本地地址（非 `127.x` 或 `localhost`）时：
- 如果未配置自定义 AuthHook → 启动时打印 WARN（当前行为）
- 后续改为：非本地绑定时，如果没有注册认证 Hook，**拒绝启动**

---

## 4. ACL 规则设计

### 角色划分

| 角色 | 订阅权限 | 发布权限 |
|------|---------|---------|
| 设备客户端（SN 认证） | `device/{自己的SN}/command` | `device/{自己的SN}/reading`, `event`, `status` |
| 管理员/SCADA | `device/#`（通配） | `device/{任意SN}/command` |
| 匿名 | 无 | 无 |

### 实现位置

在 `AuthHook.OnACLCheck` 中实现。需要区分"设备客户端"和"管理员"——可以用一个 `admin_tokens` 配置项或单独的用户列表。

### comqtt ACL check 签名

```go
OnACLCheck(cl *mqtt.Client, topic string, write bool) bool
```
- `write=true` → 客户端要 Publish 到该 topic
- `write=false` → 客户端要 Subscribe 到该 topic

---

## 5. QoS 策略

| 场景 | QoS | 原因 |
|------|-----|------|
| reading 数据 | QoS 0 | 实时数据，丢一两帧无所谓，SQLite 有完整记录 |
| status（在线/离线） | QoS 1 + Retain | 状态消息不能丢，新订阅者需要立即拿到当前状态 |
| event（告警） | QoS 1 | 告警不能丢 |
| command（下行） | QoS 1 | 指令不能丢 |

当前 `MqttPublisher.PublishDeviceReadings` 硬编码 QoS 1，后续应根据消息类型传入不同 QoS。

---

## 6. 性能与基准测试

### 需要测试的场景

1. **连接容量**：同时 100+ MQTT 客户端保持连接的内存占用
2. **消息吞吐**：每秒 Publish 1000 条 reading 的 CPU 和延迟
3. **长期运行稳定性**：24 小时不间断运行，观察 goroutine 和内存是否泄漏

### 测试工具

- `mosquitto_pub` / `mosquitto_sub` 基础验证
- comqtt 自带的 benchmark 示例：`mqtt/examples/benchmark/`
- Go pprof 内存/goroutine 分析

### comqtt 调优参数

```go
&mqtt.Options{
    InlineClient: true,
    ClientNetWriteBufferSize: 2048,  // 默认值，可根据实际流量调大
    ClientNetReadBufferSize:  2048,
}
```

---

## 7. 与 SQLite 缓存的协作

当前 SQLite 是唯一持久化存储，MQTT 是实时数据通道。两者关系：

```
Client → TLV → HandleConnection
                  ├→ SQLite.SaveReadings()    （持久化，可靠）
                  └→ MqttPublisher.Publish()   （实时推送，尽力而为）
```

SQLite 不依赖 MQTT，MQTT 失败不影响数据完整性。

### 后续可扩展：MQTT 消息持久化

comqtt 支持 Bolt/Badger/Redis 存储 Hook，可以持久化 MQTT 的 session、subscription、retained message。如果需要 MQTT broker 重启后恢复状态：

```go
import "github.com/wind-c/comqtt/v2/mqtt/hooks/storage/bolt"

server.AddHook(new(bolt.Hook), &bolt.Options{
    Path: "data/mqtt.db",
})
```

框架阶段不需要，但接口已预留。

---

## 8. 实施优先级

| 阶段 | 内容 | 依赖 |
|------|------|------|
| P0（已完成） | comqtt v2 集成、MqttBroker、EventPublisher 接口、NopPublisher | - |
| P1 | JSON 序列化、数据桥接、取消 NopPublisher | P0 |
| P2 | 自定义 AuthHook（SN/Token 认证） | P1 |
| P3 | ACL 规则、QoS 分级 | P2 |
| P4 | 性能测试、comqtt 调优 | P1 |
| P5 | MQTT 消息持久化（Bolt Hook） | P2 |
| P6 | WebSocket TLS | P2 |

每个阶段完成后确保 `go build ./...` 通过、现有测试不退化。
