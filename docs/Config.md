# Configuration System

## Overview

LEPG 的配置系统采用 **Provider Chain 模式**，支持多层级配置来源，按优先级自动覆盖。核心设计原则：

- **依赖注入**：配置通过接口注入，而非全局单例
- **优先级链**：配置来源按优先级排列，高优先级覆盖低优先级
- **默认值兜底**：链底端的 DefaultProvider 确保配置永远有值
- **可选能力分离**：复杂能力（如结构化解析）通过类型断言获取，不污染主接口

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                        Application Layer                     │
│                    (cmd/server, cmd/client)                  │
└────────────────────────────┬────────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────┐
│                     config.NewProviders()                   │
│  ┌──────────────────────────────────────────────────────┐   │
│  │                   ProviderChain                      │   │
│  │  ┌─────────┐ ┌─────────┐ ┌─────────┐ ┌─────────┐     │   │
│  │  │ Default │ │  File   │ │   Env   │ │  Flag   │     │   │
│  │  │   (0)   │ │   (1)   │ │   (2)   │ │   (3)   │     │   │
│  │  └─────────┘ └─────────┘ └─────────┘ └─────────┘     │   │
│  │       │           │          │          │            │   │
│  │       └───────────┴──────────┴──────────┘            │   │
│  │                    │   ▲                             │   │
│  │                    └───┴──────► IProvider            │   │
│  └──────────────────────────────────────────────────────┘   │
│                          │                                  │
│                          ▼                                  │
│                    IUnmarshaler                             │
│                  (optional, via type assert)                │
└─────────────────────────────────────────────────────────────┘
```

## Priority Chain

配置来源按以下优先级（从低到高）：

| Priority | Source      | Description                          |
|----------|-------------|--------------------------------------|
| 0        | Default     | 编译期硬编码的默认值，永远兜底        |
| 1        | File        | TOML 配置文件，可选 `.env` 文件      |
| 2        | Environment | 环境变量，支持前缀（如 `LEPG_`）     |
| 3        | Flag        | 命令行参数，优先级最高                |

## Core Interfaces

### IProvider

配置读取的核心接口，所有 provider 必须实现：

```go
type IProvider interface {
    GetString(key string) string
    GetInt(key string) int
    GetBool(key string) bool
    GetStringSlice(key string) []string
    Get(key string) any
    IsSet(key string) bool
}
```

### IUnmarshaler

**可选**的结构化解析接口，通过类型断言获取：

```go
type IUnmarshaler interface {
    Unmarshal(rawVal any) error
}
```

只有 `FileProvider` 实现此接口，`ProviderChain` 会委托给它。Env/Flag provider 不支持此操作。

## Provider Implementations

### DefaultProvider

提供编译期硬编码的默认值，位于链的最末端：

```go
defaultProv := provider.NewDefaultProvider(map[string]any{
    "server":    "http://localhost",
    "port":      8883,
    "log_level": "info",
})
```

### FileProvider

从 TOML 文件读取配置，自动加载 `.env` 文件（如果存在）：

```go
fileProv := provider.NewFileProvider("toml")
fileProv.SetConfigFile("config.toml")
fileProv.Load()           // 加载 TOML
fileProv.LoadDotEnv()     // 加载 .env（可选）
```

支持的配置文件路径：
- 显式指定：`--config=/path/to/config.toml`
- 默认搜索：`./config.toml` 或 `./config/config.toml`

### EnvProvider

从环境变量读取，支持前缀：

```go
envProv := provider.NewEnvProvider("LEPG_") // LEPG_PORT, LEPG_LOG_LEVEL
envProv.Load()
```

### FlagProvider

命令行参数，优先级最高：

```go
flagProv := provider.NewFlagProvider()
flagProv.Set("port", 9999)
flagProv.Set("log_level", "debug")
```

## Provider Chain

`ProviderChain` 按优先级顺序组合多个 provider：

```go
chain := NewProviderChainWithFile(
    fileProv,       // FileProvider 引用，用于 Unmarshal
    defaultProv,    // 0: 默认值
    fileProv,       // 1: 配置文件
    envProv,        // 2: 环境变量
    flagProv,       // 3: 命令行参数
)
```

读取规则：从高优先级向低优先级查找，返回第一个非空值。

## Usage Patterns

### Reading Simple Values

```go
func InitClientConfig(provider config.IProvider) (*ClientConfig, error) {
    cfg := &ClientConfig{}

    // Direct reads, DefaultProvider ensures fallback
    cfg.ServerUrl = provider.GetString("server")
    cfg.Port = provider.GetInt("port")
    cfg.LogLevel = provider.GetString("log_level")

    return cfg, nil
}
```

**无需手动判断零值** — DefaultProvider 已在链底兜底。

### Reading Structured Data

```go
func InitServerConfig(provider config.IProvider) (*ServerConfig, error) {
    cfg := &ServerConfig{}

    // Simple fields
    cfg.Port = provider.GetInt("port")
    cfg.LogLevel = provider.GetString("log_level")

    // Structured data via type assertion
    if u, ok := provider.(config.IUnmarshaler); ok {
        var wrapper struct {
            Clients []ClientDef `mapstructure:"clients"`
        }
        if err := u.Unmarshal(&wrapper); err != nil {
            return nil, err
        }
        cfg.Clients = wrapper.Clients
    }

    return cfg, nil
}
```

**类型断言** 是 Go 处理可选能力的标准模式。

## Configuration File Format

### TOML Structure

```toml
# config.toml
port = 8883
log_level = "info"

[[clients]]
sn = "device001"
token = "abc123"
description = "Gateway at warehouse"

[[clients]]
sn = "device002"
token = "def456"
description = "Gateway at office"
```

### Environment Variables

```bash
# .env file or shell environment
LEPG_PORT=9999
LEPG_LOG_LEVEL=debug
```

### Command Line Flags

```bash
# Override any config
lepgs run --port=7777
lepgc run --url=https://example.com --port=7777
```

## Initialization Flow

```go
// 1. Collect flag values
flagValues := map[string]any{}
if flagPort != 0 {
    flagValues["port"] = flagPort
}

// 2. Create provider chain
providers := config.NewProviders(flagValues, cfgFile)

// 3. Initialize config (single interface)
cfg, err := server.InitServerConfig(providers.Chain)
```

## Key Design Decisions

### Why Type Assertion for Unmarshal?

`Unmarshal` 不是"配置提供"的核心抽象：
- `EnvProvider` 和 `FlagProvider` 不支持解析嵌套结构
- 强行加入 `IProvider` 会让所有 provider 实现无意义的方法
- 违反接口隔离原则 (ISP)

类型断言是 Go 处理可选能力的标准模式。

### Why DefaultProvider in Chain?

默认值是配置来源之一，应该与其他 provider 平等对待：
- 统一的读取路径，无需业务层特殊处理
- 支持运行时扩展默认值（未来可从远程获取）
- 符合开闭原则

### Why Providers Holds Only Chain?

`Providers.FileProvider` 字段已被移除：
- cmd 层不需要知道 server 需要 unmarshal 能力
- 这是 server 包的实现细节，不应外泄
- 需要写入配置时直接调用 `provider.InitConfig()`

## File Structure

```
internal/config/
├── config.go          # Providers factory, constants
├── provider.go        # IProvider, IUnmarshaler, ProviderChain
└── provider/
    ├── default.go     # DefaultProvider (chain bottom)
    ├── file.go        # FileProvider (TOML + .env)
    ├── env.go         # EnvProvider (environment variables)
    └── flag.go        # FlagProvider (command line flags)

internal/server/
└── config.go          # InitServerConfig, ServerConfig

internal/client/
└── config.go          # InitClientConfig, ClientConfig
```

## Common Patterns

### Adding a New Config Field

1. Add default value in `config.NewProviders()`
2. Read in `InitXxxConfig()` via provider
3. Add to config struct and validate if needed

### Supporting Nested Configuration

Use type assertion to get `IUnmarshaler`, then unmarshal:

```go
if u, ok := provider.(config.IUnmarshaler); ok {
    u.Unmarshal(&nestedStruct)
}
```

### Validation

Validate in config init, return error for missing required fields:

```go
func (c *ClientConfig) Validate() error {
    var missing []string
    if c.Sn == "" {
        missing = append(missing, "sn")
    }
    if len(missing) > 0 {
        return errors.NewConfigNotSetError(missing)
    }
    return nil
}
```

## Error Handling

- `ConfigNotSetError`: Required configuration field is missing
- `UnmarshalNotSupportedError`: Provider doesn't support unmarshal
- Viper errors propagated from `FileProvider.Load()`

## Migration from Viper Singleton

The old pattern used a global viper instance:

```go
// Old (anti-pattern)
viper.SetDefault("port", 8883)
port := viper.GetInt("port")
```

New pattern uses dependency injection:

```go
// New (correct)
providers := config.NewProviders(flagValues, cfgFile)
port := providers.Chain.GetInt("port")  // DefaultProvider ensures fallback
```

Benefits:
- Testable: can inject mock providers
- Isolated: no global state
- Flexible: easy to add new providers
