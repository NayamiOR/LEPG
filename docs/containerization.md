# LEPG Server 容器化部署

## 文件说明

| 文件 | 用途 |
|------|------|
| `Dockerfile` | 多阶段构建：`golang:1.26-alpine` 编译 → `alpine` 运行 |
| `docker-compose.yml` | 编排 PostgreSQL + LEPG Server，带健康检查 |

## 构建与启动

```bash
docker compose up -d
```

首次启动会自动构建镜像。之后代码有改动需要重建：

```bash
docker compose up -d --build
```

## 配置

### server.toml

部署时需准备一份面向容器环境的 `config/server.toml`，关键差异：

```toml
# 数据库——主机名用 compose service name
pg_host = "postgres"
pg_port = 5432
pg_user = "lepgs"
pg_password = "lepgs123"    # 与 POSTGRES_PASSWORD 一致
pg_dbname = "lepgs"
pg_sslmode = "disable"

# MQTT broker——绑定容器内所有接口
mqtt_tcp = "0.0.0.0:1883"
mqtt_ws = "0.0.0.0:8083"

# 路径用容器内约定（也可不写，靠环境变量 LOG_PATH / DATA_PATH）
log_path = "/app/data/logs"
data_path = "/app/data/lepgs.db"

# [[clients]] 和 [[outputs]] 保持原样
```

> **注意**：Provider Chain 优先级为 Flag > File > Env > Default。TOML 中的值会覆盖同名环境变量。`LOG_PATH` / `DATA_PATH` 在 docker-compose.yml 中以环境变量给出，是因为容器内路径约定固定，不想在每个部署环境都改 TOML。如需 TOML 全权控制，删掉 compose 中的 environment 块即可。

### 环境变量

docker-compose.yml 中仅保留容器环境特定的变量：

| 变量 | 值 | 说明 |
|------|-----|------|
| `LOG_PATH` | `/app/data/logs` | 日志目录 |
| `DATA_PATH` | `/app/data/lepgs.db` | 持久化数据文件 |

### PostgreSQL

PostgreSQL 使用官方 `postgres:17-alpine` 镜像，通过环境变量自动创建用户和数据库：

```yaml
POSTGRES_USER: lepgs
POSTGRES_PASSWORD: lepgs123
POSTGRES_DB: lepgs
```

数据持久化在命名 volume `pgdata`。

## 端口

| 容器端口 | 宿主机 | 用途 |
|----------|--------|------|
| 8883 | 8883 | TLV 隧道（LEPG Client 连接入口） |
| 1883 | 1883 | 嵌入式 MQTT Broker TCP（下游消费者订阅） |
| 8083 | 8083 | 嵌入式 MQTT Broker WebSocket（浏览器仪表盘） |

## 数据持久化

| Volume | 挂载点 | 内容 |
|--------|--------|------|
| `pgdata` | `/var/lib/postgresql/data` | PostgreSQL 数据 |
| `lepg-data` | `/app/data` | LEPG 日志 + lepgs.db |
