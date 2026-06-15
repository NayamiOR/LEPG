---
title: CLI 命令与环境变量参考
description: lepgs/lepgc 的 run/init 子命令、命令行标志，以及环境变量到配置键的映射
type: reference
tags: [config, cli, env, reference]
---

# CLI 命令与环境变量参考

## CLI 命令参考

### 服务器 (lepgs)

```
lepgs [command]
```

| 命令 | 说明 |
|------|------|
| `run` | 启动服务器 |
| `init` | 初始化配置文件 |
| `help` | 帮助信息 |

**全局标志：**

| 标志 | 短标志 | 默认值 | 说明 |
|------|--------|--------|------|
| `--config` | `-c` | — | 配置文件路径（默认搜索 `config/server.toml`） |

**`run` 命令标志：**

| 标志 | 短标志 | 默认值 | 说明 |
|------|--------|--------|------|
| `--port` | `-p` | `0`（不覆盖） | 覆盖 TCP 监听端口 |

**`init` 命令：**

默认写入路径为 `/etc/lepgs/config.toml`。可通过 `-c` 指定其他路径。文件已存在时会拒绝覆盖。

### 客户端 (lepgc)

```
lepgc [command]
```

| 命令 | 说明 |
|------|------|
| `run` | 启动客户端 |
| `init` | 初始化配置文件 |
| `help` | 帮助信息 |

**全局标志：**

| 标志 | 短标志 | 默认值 | 说明 |
|------|--------|--------|------|
| `--config` | `-c` | — | 配置文件路径（默认搜索 `config/client.toml`） |

**`run` 命令标志：**

| 标志 | 短标志 | 默认值 | 说明 |
|------|--------|--------|------|
| `--url` | `-u` | —（不覆盖） | 覆盖服务器地址 |
| `--port` | `-p` | `0`（不覆盖） | 覆盖服务器端口 |

**`init` 命令：**

默认写入路径为 `/etc/lepgc/config.toml`。可通过 `-c` 指定其他路径。文件已存在时会拒绝覆盖。

## 环境变量参考

环境变量无前缀，变量名小写后直接映射为配置键。

### 服务器环境变量

| 环境变量 | 对应键 | 类型 | 示例 |
|---------|--------|------|------|
| `PORT` | `port` | int | `PORT=9999` |
| `LOG_LEVEL` | `log_level` | string | `LOG_LEVEL=debug` |
| `DATA_PATH` | `data_path` | string | `DATA_PATH=/data/lepgs.db` | [已废弃] |
| `PG_HOST` | `pg_host` | string | `PG_HOST=postgres` |
| `PG_PORT` | `pg_port` | int | `PG_PORT=5433` |
| `PG_USER` | `pg_user` | string | `PG_USER=lepgs` |
| `PG_PASSWORD` | `pg_password` | string | `PG_PASSWORD=***` |
| `PG_DBNAME` | `pg_dbname` | string | `PG_DBNAME=lepgs` |
| `PG_SSLMODE` | `pg_sslmode` | string | `PG_SSLMODE=require` |
| `MQTT_TCP` | `mqtt_tcp` | string | `MQTT_TCP=0.0.0.0:1883` |
| `MQTT_WS` | `mqtt_ws` | string | `MQTT_WS=0.0.0.0:8083` |
| `REDIS_ADDR` | `redis_addr` | string | `REDIS_ADDR=redis:6379` |
| `REDIS_PASSWORD` | `redis_password` | string | `REDIS_PASSWORD=***` |
| `REDIS_DB` | `redis_db` | int | `REDIS_DB=1` |

> 注意：`[[clients]]` 数组无法通过环境变量配置，只能通过 TOML 文件。

### 客户端环境变量

| 环境变量 | 对应键 | 类型 | 示例 |
|---------|--------|------|------|
| `SERVER` | `server` | string | `SERVER=192.168.1.100` |
| `PORT` | `port` | int | `PORT=9999` |
| `LOG_LEVEL` | `log_level` | string | `LOG_LEVEL=debug` |
| `SN` | `sn` | string | `SN=DEVICE-001` |
| `TOKEN` | `token` | string | `TOKEN=***` |
| `MAX_RETRY` | `max_retry` | int | `MAX_RETRY=20` |
| `RETRY_INTERVAL` | `retry_interval` | int | `RETRY_INTERVAL=10000` |
| `BUFFER_SIZE` | `buffer_size` | int | `BUFFER_SIZE=2000` |
| `UPLOAD_BATCH_SIZE` | `upload_batch_size` | int | `UPLOAD_BATCH_SIZE=50` |
| `UPLOAD_INTERVAL` | `upload_interval` | int | `UPLOAD_INTERVAL=3000` |
| `LOG_PATH` | `log_path` | string | `LOG_PATH=/var/log/lepgc.log` |
| `DATA_PATH` | `data_path` | string | `DATA_PATH=/data/lepgc.db` |

> 注意：`[[devices]]`、`[mqtt]` 及其子项无法通过环境变量配置，只能通过 TOML 文件。

---

## 相关笔记

- [[overview|配置系统总览]]
- [[server-config|服务器配置]]
- [[client-config|客户端配置]]
- [[config-examples|完整配置示例]]
- [[sensitive-fields|敏感字段处理]]
- [[config-migration|迁移指南]]
- [[config-dev-guide|开发者指南]]
- [[config-internals|内部机制]]
- [[modbus-config|Modbus 设备配置]]
