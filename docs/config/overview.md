---
title: 配置系统总览
description: Provider Chain 模式概览、四层配置来源与优先级、环境变量与文件定位规则
type: reference
tags: [config, provider-chain, reference]
---

# 配置系统总览

> 本组笔记面向两类读者：**运维人员**（部署、调参）和 **开发人员**（扩展配置项、理解内部机制）。

## 配置笔记索引

- [[server-config|服务器配置 (lepgs)]]
- [[client-config|客户端配置 (lepgc)]]
- [[config-examples|完整配置示例]]
- [[cli-and-env|CLI 命令与环境变量参考]]
- [[sensitive-fields|敏感字段处理]]
- [[config-migration|迁移指南（旧系统 → 新系统）]]
- [[config-dev-guide|开发者指南：添加新配置项]]
- [[config-internals|内部机制]]
- [[modbus-config|Modbus 设备配置]]

---

## 概述

LEPG 的配置系统采用 **Provider Chain（提供者链）** 模式，通过依赖注入组装多个配置来源，避免全局单例状态。

核心设计原则：

- **多层来源**：默认值 < 环境变量 < 配置文件 < 命令行参数，优先级由低到高。
- **严格白名单**：每个配置字段通过 `sources` 标签显式声明允许的来源，未声明的来源不会读取该字段。
- **两类字段**：简单字段（`string`/`int`/`bool`）通过 Provider Chain 逐项填充；复杂嵌套结构（数组、对象）通过 `Unmarshal` 一次性反序列化。
- **元设置与业务设置分离**：`MetaConfig`（搜索路径、初始化路径）在 Provider Chain 构建之前即已确定，且运行期间不再变更。

配置文件格式为 **TOML**。服务器默认配置文件为 `config/server.toml`，客户端默认配置文件为 `config/client.toml`。

## 配置来源与优先级

Provider Chain 按以下顺序组装，**编号越大优先级越高**：

| 优先级 | 来源 | 实现类 | 说明 |
|--------|------|--------|------|
| 0（最低） | 默认值 | `DefaultProvider` | 从结构体 `default` 标签提取，由 `ExtractDefaults` 自动生成 |
| 1 | 环境变量 | `EnvProvider` | 无前缀，键名小写匹配。例如 `PORT=9999` 对应键 `port` |
| 2 | 配置文件 | `FileProvider` | 支持 TOML 格式 + `.env` 文件合并 |
| 3（最高） | 命令行参数 | `FlagProvider` | 仅限特定字段（见 CLI 参考） |

解析时，系统反向遍历 Provider Chain（从高到低），命中第一个有效值即停止。每个字段的 `sources` 标签控制允许参与解析的 Provider 子集。

### 环境变量规则

- **无前缀**：`EnvProvider` 初始化时 `prefix` 为空字符串。
- **键名转换**：环境变量名小写化后直接作为配置键。例如 `LOG_LEVEL=debug` 对应键 `log_level`，`REDIS_ADDR=localhost:6379` 对应键 `redis_addr`。
- **类型推断**：字符串值按需转换为 `int`（`strconv.Atoi`）或 `bool`（`strconv.ParseBool`）。

### 配置文件定位顺序

运行时按以下逻辑确定配置文件路径：

1. 若通过 `--config/-c` 指定了路径 → 使用该路径。
2. 否则 → 使用 `MetaConfig.SearchPath`（服务器：`config/server.toml`，客户端：`config/client.toml`）。

配置文件不存在时不会报错（静默忽略），字段将回退到更低优先级的来源。

### `.env` 文件

`FileProvider` 在加载 TOML 配置文件后，会尝试加载工作目录下的 `.env` 文件。`.env` 文件中的值会合并到 FileProvider 中，优先级等同于 TOML 文件。`.env` 文件不存在时不报错。
