---
title: 敏感字段处理
description: token、sn、redis_password 等敏感字段的允许来源限制与安全建议
type: reference
tags: [config, security, reference]
---

# 敏感字段处理

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

## 相关笔记

- [[overview|配置系统总览]]
- [[server-config|服务器配置]]
- [[client-config|客户端配置]]
- [[cli-and-env|CLI 命令与环境变量]]
- [[config-examples|完整配置示例]]
- [[config-migration|迁移指南]]
- [[config-dev-guide|开发者指南]]
- [[config-internals|内部机制]]
- [[modbus-config|Modbus 设备配置]]
