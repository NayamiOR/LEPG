---
title: 迁移指南（旧系统 → 新系统）
description: 从旧版全局 Viper 单例到 Provider Chain 的破坏性变更、API 变化与配置文件兼容性
type: reference
tags: [config, migration, reference]
---

# 迁移指南（旧系统 → 新系统）

以下是配置系统重构后的主要破坏性变更。重构的背景、设计动机与完整实施过程见 [[config-refactor-report|配置系统重构案例报告]]。

## 常量与映射变更

| 旧系统 | 新系统 | 影响 |
|--------|--------|------|
| `DefaultConfigFile` 常量 | `ServerMetaConfig.SearchPath` / `ClientMetaConfig.SearchPath` | 配置文件默认路径逻辑不变，但实现方式改变 |
| `defaultServerValues` / `defaultClientValues` 硬编码 map | 结构体 `default` 标签 + `ExtractDefaults` 自动提取 | 默认值现在与结构体定义同位，更易维护 |
| `config_path` 在 defaults map 中运行时使用 | `config_path` 仅在 `GetDefaultValues()` 中为 `init` 命令注入 | 运行时不再有 `config_path` 键 |
| `PathsConfig.ConfigPath` 字段 | 已移除（原为死代码） | 无功能影响 |

## API 变更

| 旧系统 | 新系统 |
|--------|--------|
| `NewProviders("server")` — 传配置名（无扩展名） | `NewProviders(flagValues, cfgFile)` — `cfgFile` 为完整文件路径，默认路径由 `MetaConfig.SearchPath` 控制 |
| 服务器无 `Validate()` | 服务器新增 `Validate()`，启动时自动校验配置 |

## 行为变更

| 变更项 | 说明 |
|--------|------|
| `DataPath` 可配置 | 服务器端 `data_path` 现在可通过 Provider Chain 设置（原为硬编码） |
| `sources` 白名单机制 | 每个字段显式声明允许的来源，未声明的不参与解析 |
| 敏感字段保护 | `sn`、`token` 禁止通过 CLI flag 传入；`redis_password` 无默认值且 `init` 不生成 |

## 配置文件兼容性

TOML 配置文件的键名 **完全不变**，现有配置文件无需修改即可在新系统上使用。

---

## 相关笔记

- [[overview|配置系统总览]]
- [[config-refactor-report|配置系统重构案例报告]]
- [[server-config|服务器配置]]
- [[client-config|客户端配置]]
- [[config-dev-guide|开发者指南]]
- [[config-internals|内部机制]]
- [[cli-and-env|CLI 命令与环境变量]]
- [[sensitive-fields|敏感字段处理]]
- [[config-examples|完整配置示例]]
- [[modbus-config|Modbus 设备配置]]
