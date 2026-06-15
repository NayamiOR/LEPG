---
title: 服务器配置 (lepgs)
description: lepgs 的 MetaConfig、业务配置字段（端口/日志/MQTT/Redis/客户端列表）与验证规则
type: reference
tags: [config, server, reference]
---

# 服务器配置 (lepgs)

## MetaConfig（元设置）

| 属性 | 值 |
|------|----|
| SearchPath | `config/server.toml` |
| InitPath | `/etc/lepgs/config.toml` |

- **SearchPath**：运行时未指定 `--config` 时的默认搜索路径。
- **InitPath**：`init` 命令默认写入的配置文件路径。

## 业务配置字段

### 基本设置

| TOML 键 | 类型 | 默认值 | 必填 | 允许来源 | 说明 |
|---------|------|--------|------|---------|------|
| `port` | int | `8883` | 否 | file, flag, env, default | TCP 监听端口，范围 1-65535 |
| `log_level` | string | `"info"` | 否 | file, env, default | 日志级别：`debug` / `info` / `warn` / `error` |
| `data_path` | string | `"/var/cache/lepgs/lepgs.db"` | 否 | file, env, default | [已废弃] 旧 SQLite 数据库路径，已迁移至 PostgreSQL |

### PostgreSQL 设置

| TOML 键 | 类型 | 默认值 | 必填 | 允许来源 | 说明 |
|---------|------|--------|------|---------|------|
| `pg_host` | string | `"127.0.0.1"` | 否 | file, env, default | PostgreSQL 主机地址 |
| `pg_port` | int | `5432` | 否 | file, env, default | PostgreSQL 端口 |
| `pg_user` | string | `"lepgs"` | 否 | file, env, default | PostgreSQL 用户名 |
| `pg_password` | string | — | 否 | file, env | **敏感字段**，无默认值 |
| `pg_dbname` | string | `"lepgs"` | 否 | file, env, default | PostgreSQL 数据库名 |
| `pg_sslmode` | string | `"disable"` | 否 | file, env, default | SSL 模式：`disable` / `require` / `verify-ca` / `verify-full` |

### MQTT Broker 设置

| TOML 键 | 类型 | 默认值 | 必填 | 允许来源 | 说明 |
|---------|------|--------|------|---------|------|
| `mqtt_tcp` | string | `"127.0.0.1:1883"` | 否 | file, env, default | MQTT broker TCP 监听地址 |
| `mqtt_ws` | string | `"127.0.0.1:8083"` | 否 | file, env, default | MQTT broker WebSocket 监听地址 |

### Redis 设置（预留）

| TOML 键 | 类型 | 默认值 | 必填 | 允许来源 | 说明 |
|---------|------|--------|------|---------|------|
| `redis_addr` | string | `"127.0.0.1:6379"` | 否 | file, env, default | Redis 连接地址 |
| `redis_password` | string | — | 否 | file, env | **敏感字段**，无默认值，`init` 命令不会生成此键 |
| `redis_db` | int | `0` | 否 | file, env, default | Redis 数据库索引 |

### 客户端列表（Unmarshal 填充）

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

## 验证规则

`ServerConfig.Validate()` 在配置加载后自动执行，检查以下约束：

| 字段 | 规则 |
|------|------|
| `port` | 必须在 1-65535 范围内 |
| `log_level` | 必须是 `debug`、`info`、`warn`、`error` 之一 |
| `pg_host` | 不能为空 |
| `pg_dbname` | 不能为空 |

---

## 相关笔记

- [[overview|配置系统总览]]
- [[client-config|客户端配置]]
- [[config-examples|完整配置示例]]
- [[cli-and-env|CLI 命令与环境变量]]
- [[sensitive-fields|敏感字段处理]]
- [[config-migration|迁移指南]]
- [[config-dev-guide|开发者指南]]
- [[config-internals|内部机制]]
- [[modbus-config|Modbus 设备配置]]
