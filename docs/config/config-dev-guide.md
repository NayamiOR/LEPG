---
title: 开发者指南：添加新配置项
description: 通过 config/default/sources 结构体标签添加新配置字段并自动纳入 Provider Chain 的步骤
type: reference
tags: [config, development, reference]
---

# 开发者指南：添加新配置项

添加一个新的配置字段只需以下步骤：

## 步骤 1：在结构体中添加字段

在 `internal/server/config.go`（`ServerConfig`）或 `internal/client/config.go`（`ClientConfig` / 子结构体）中添加字段，携带 `config`、`default`、`sources` 三个标签：

```go
// 简单字段示例
MaxConnections int `config:"max_connections" default:"100" sources:"file,env,default"`

// 敏感字段（无 default，限制来源）
ApiKey string `config:"api_key" sources:"file,env"`
```

标签说明：

| 标签 | 必填 | 说明 |
|------|------|------|
| `config` | 是（有 `sources` 时） | TOML 键名 / Provider 查找键 |
| `default` | 否 | 默认值字符串，不设则无默认值 |
| `sources` | 是（参与 Provider Chain 的字段） | 允许的来源列表，逗号分隔：`file,env,flag,default` |

## 步骤 2：添加验证（如需要）

在对应的 `Validate()` 方法中添加校验逻辑：

```go
if c.MaxConnections <= 0 {
    errs = append(errs, errors.NewConfigInvalidError("max_connections", "must be positive"))
}
```

## 步骤 3：完成

`PopulateFromProvider` 和 `ExtractDefaults` 通过反射自动处理新字段，无需修改任何框架代码。

如果新字段是复杂嵌套结构（数组、对象），则需要：
1. 不加 `sources` 标签（不参与 Provider Chain 逐项填充）。
2. 使用 `mapstructure` 标签定义 TOML 映射。
3. 在 `InitXxxConfig` 中通过 `IUnmarshaler` 反序列化。
4. 在 `Validate()` 中添加验证。

---

## 相关笔记

- [[overview|配置系统总览]]
- [[config-internals|内部机制]]（反射填充与 ExtractDefaults 的实现细节）
- [[server-config|服务器配置]]
- [[client-config|客户端配置]]
- [[sensitive-fields|敏感字段处理]]
- [[config-migration|迁移指南]]
- [[config-examples|完整配置示例]]
- [[cli-and-env|CLI 命令与环境变量]]
- [[modbus-config|Modbus 设备配置]]
