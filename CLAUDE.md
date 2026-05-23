# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

LEPG (Lightweight Edge Piercing Gateway) is a lightweight IoT edge gateway and NAT traversal system built in Go. It consists of two main components:

- **Client (`lepgc`)**: Edge-side gateway that collects data and connects to the server
- **Server (`lepgs`)**: Cloud-side gateway that receives connections and manages devices

The system features custom TCP tunneling for NAT traversal, TLV protocol parsing, SQLite local caching with断点续传, and real-time monitoring capabilities.

## Development Commands

### Building and Running
```bash
# Build both client and server
make build

# Run client (shortcuts: make run-client or make c)
go run cmd/client/main.go run

# Run server (shortcuts: make run-server or make s)
go run cmd/server/main.go run

# Clean build artifacts
make clean
```

### Testing
```bash
# Run all tests
make test

# Run config tests only
make test-config

# Run tests with coverage report
make test-coverage

# Run race condition tests
make test-race
```

### Configuration Initialization
```bash
# Initialize default client config
go run cmd/client/main.go init

# Initialize default server config
go run cmd/server/main.go init

# Run with custom config file
go run cmd/client/main.go run --config=/path/to/config.toml
go run cmd/server/main.go run -c /path/to/config.toml
```

## Architecture

### Configuration System (Provider Chain Pattern)

The configuration system uses a **Provider Chain pattern** with dependency injection, avoiding global singleton state. Configuration sources are prioritized from low to high:

1. **DefaultProvider** (0): Hardcoded defaults at compile time
2. **FileProvider** (1): TOML configuration files (`config/client.toml`, `config/server.toml`)
3. **EnvProvider** (2): Environment variables with `LEPG_` prefix
4. **FlagProvider** (3): Command-line arguments (highest priority)

Key interfaces:
- `IProvider`: Core configuration reading interface (`GetString`, `GetInt`, `GetBool`, etc.)
- `IUnmarshaler`: Optional structured config parsing via type assertion

**Important**: Always use the provider chain for configuration access. Never use global configuration or hardcoded values outside DefaultProvider.

### Message Protocol

The system uses a custom TLV (Type-Length-Value) protocol over TCP:

```
[Magic:2B][Version:1B][Flags:1B][Type:1B][MsgID:2B][PayloadLen:2B][Timestamp:4B][Payload:N][Checksum:2B]
```

Message types:
- `0x01`: Handshake
- `0x02`: Authentication  
- `0x03`: Heartbeat
- `0x04`: Error
- `0x05`: Data upload

Key components:
- `internal/msg/msg.go`: Message encoding/decoding with TLV format
- `internal/msg/msg_test.go`: Comprehensive protocol tests
- `internal/utils/checksum.go`: CRC16-CCITT checksum calculation
- `internal/utils/timestamp.go`: Custom epoch (2020-01-01) timestamps

### Client-Server Architecture

**Client** (`internal/client/`):
- `UploadLoop()`: Main data upload loop with retry logic
- Connection management with configurable retry (`max_retry`, `retry_interval`)
- Mock data generation for testing
- Modbus RTU/TCP slave structures (in development)

**Server** (`internal/server/`):
- `ReceiveLoop()`: TCP listener accepting client connections
- `HandleConnection()`: Per-connection message processing
- Client authentication via SN/token pairs from config
- Concurrent connection handling with goroutines

## Project Structure

```
LEPG/
├── cmd/
│   ├── client/main.go          # Client CLI entry point
│   └── server/main.go          # Server CLI entry point
├── internal/
│   ├── client/                 # Client implementation
│   │   ├── client.go           # Main client logic
│   │   ├── config.go           # Client config initialization
│   │   └── modbus.go           # Modbus slave definitions
│   ├── server/                 # Server implementation
│   │   ├── server.go           # Main server logic
│   │   └── config.go           # Server config initialization
│   ├── config/                 # Configuration system
│   │   ├── config.go           # Provider factory
│   │   ├── provider.go         # IProvider interface and chain
│   │   └── provider/           # Provider implementations
│   ├── msg/                    # Message protocol
│   │   ├── msg.go              # TLV encoding/decoding
│   │   └── msg_test.go         # Protocol tests
│   ├── utils/                  # Utilities
│   │   ├── checksum.go         # CRC16 implementation
│   │   └── timestamp.go        # Custom timestamp logic
│   └── errors/                 # Custom errors
├── config/                     # Configuration files
│   ├── client.toml             # Default client config
│   └── server.toml             # Default server config
└── makefile                    # Build automation
```

## Configuration Files

### Client Config (`config/client.toml`)
```toml
log_level = 'info'
max_retry = 10
port = 8883
retry_interval = 5000
server = '127.0.0.1'
sn = "CLIENT001"
token = "token123456"
```

### Server Config (`config/server.toml`)
```toml
log_level = 'info'
port = 8883

[[clients]]
sn = "CLIENT001"
token = "token123456"
description = "Test client 1"
```

## Key Design Patterns

1. **Dependency Injection**: Configuration is injected via `IProvider` interface, not global singletons
2. **Provider Chain**: Multiple configuration sources with clear priority
3. **Type Assertion**: Optional capabilities (like `Unmarshal`) obtained via type assertions
4. **Goroutine-per-connection**: Server handles each client connection concurrently
5. **TLV Protocol**: Custom binary protocol for efficient data transmission

## Common Tasks

### Adding New Configuration Fields

1. Add default value in the appropriate `defaultXxxValues` map
2. Read the field in `InitXxxConfig()` via provider
3. Add to config struct with validation if needed
4. Update config file examples

### Adding New Message Types

1. Define message type constant in `internal/msg/msg.go`
2. Update documentation in `docs/Message Design.md`
3. Add encoding/decoding logic if needed
4. Write tests in `msg_test.go`

### Running Single Tests

```bash
# Test specific package
go test -v ./internal/msg

# Test specific function
go test -v ./internal/msg -run TestMsgEncodeDecode
```

## Error Handling

Custom errors are defined in `internal/errors/errors.go`:
- `ErrConfigNotSet`: Required configuration missing
- `ErrConfigInvalid`: Configuration validation failed
- `ErrChecksumMismatch`: Message checksum validation failed

Always use these custom errors for consistent error handling across the codebase.

## Dependencies

Key external dependencies:
- `github.com/spf13/cobra`: CLI framework
- `github.com/spf13/viper`: Configuration file parsing (used internally by FileProvider)

## Testing Strategy

The project uses table-driven tests for comprehensive coverage. Key test files:
- `internal/msg/msg_test.go`: Protocol encoding/decoding tests
- `internal/utils/checksum_test.go`: CRC16 algorithm tests

Always ensure tests pass before committing changes.
