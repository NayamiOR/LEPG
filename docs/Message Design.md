# 消息设计

## 消息包结构

```
1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|   Magic (2B)  |  Version (1B) |  Flags (1B)   |  MsgType (1B) |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|          Message ID (2B)      |       Payload Length (2B)     |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                       Timestamp (4B)                          |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                                                               |
~                         Payload (N Bytes)                     ~
|                                                               |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|   Checksum (2B, CRC16)        |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
```

Timestamp：4B的Unix时间戳，单位为毫秒，从2020年1月1日00:00:00 UTC开始计算。

Payload上限64KB（2B Payload Length 的表达上限）

## Message Type

目前定义了八种消息类型

| 值   | 名称                  | 用途         |
| ---- | --------------------- | ------------ |
| 0x01 | `MsgTypeHandshake`    | 握手消息     |
| 0x02 | `MsgTypeHandshakeAck` | 握手ACK      |
| 0x03 | `MsgTypeUpload`       | 上传数据消息 |
| 0x04 | `MsgTypeUploadAck`    | 上传ACK      |
| 0x05 | `MsgTypeHeartbeat`    | 心跳消息     |
| 0x06 | `MsgTypeHeartbeatAck` | 心跳ACK      |
| 0x07 | `MsgTypeNotify`       | 消息通知     |

### ACK 响应载荷

ACK 消息的 Payload 格式：

```
[Code:1B][MsgLen:1B][Msg:nB]
```

- Code：状态码，`0x00` 成功，`0x01` 失败
- MsgLen：消息长度
- Msg：可选的描述信息

## Flags

暂未定义
