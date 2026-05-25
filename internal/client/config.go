package client

import (
	"LEPG/internal/config"
	"LEPG/internal/errors"
	"time"
)

const DefaultConfigFile = "config/client"

// NewProviders 创建带有客户端默认值的配置提供者
func NewProviders(flagValues map[string]any, cfgFile string) *config.Providers {
	return config.NewProviders(flagValues, cfgFile, DefaultConfigFile, defaultClientValues)
}

type ClientConfig struct {
	ServerUrl     string
	Port          int
	LogLevel      string
	Sn            string
	Token         string
	MaxRetry      int
	RetryInterval int
}

var defaultClientValues = map[string]any{
	"server":         "http://localhost",
	"port":           8883,
	"log_level":      "info",
	"max_retry":      10,
	"retry_interval": 5000,
}

// InitClientConfig 初始化客户端配置
func InitClientConfig(provider config.IProvider) (*ClientConfig, error) {
	cfg := &ClientConfig{}

	// 从 provider 获取（DefaultProvider 兜底）
	cfg.ServerUrl = provider.GetString("server")
	cfg.Port = provider.GetInt("port")
	cfg.LogLevel = provider.GetString("log_level")
	cfg.Sn = provider.GetString("sn")
	cfg.Token = provider.GetString("token")
	cfg.MaxRetry = provider.GetInt("max_retry")
	cfg.RetryInterval = provider.GetInt("retry_interval")

	// 验证必需配置
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Validate 验证配置
func (c *ClientConfig) Validate() error {
	var missing []string
	var errs []error

	if c.Sn == "" {
		missing = append(missing, "sn")
	}
	if c.Token == "" {
		missing = append(missing, "token")
	}

	if len(missing) > 0 {
		errs = append(errs, errors.NewConfigNotSetError(missing))
	}

	if c.MaxRetry < 0 {
		errs = append(errs, errors.NewConfigInvalidError("max_retry", "must be non-negative"))
	}
	if c.RetryInterval < 0 {
		errs = append(errs, errors.NewConfigInvalidError("retry_interval", "must be non-negative"))
	}

	if len(errs) > 0 {
		return errors.NewConfigValidationErrors(errs)
	}
	return nil
}

// GetDefaultValues 返回客户端默认配置值
func GetDefaultValues() map[string]any {
	return defaultClientValues
}

/// START: MODBUS CONFIGURATION STRUCTS

// RtuSlaveConfig contains RTU-specific connection parameters
type RtuSlaveConfig struct {
	Port     string `toml:"port"`      // Serial port (e.g., "/dev/ttyS0" or "COM3")
	BaudRate int    `toml:"baud_rate"` // Baud rate (e.g., 9600, 19200)
	DataBits int    `toml:"data_bits"` // Data bits (default 8)
	Parity   string `toml:"parity"`    // Parity: "N", "E", "O" (default "N")
	StopBits int    `toml:"stop_bits"` // Stop bits (default 1)
}

// TcpSlaveConfig contains TCP-specific connection parameters
type TcpSlaveConfig struct {
	Host string `toml:"host"` // IP address or hostname
	Port int    `toml:"port"` // Port number (default 502)
}

// PointConfig defines a single data point on a Modbus device
type PointConfig struct {
	Name         string     `toml:"name"`          // Point identifier (JSON field name)
	FunctionCode int        `toml:"function_code"` // Modbus function code (1/2/3/4/5/6/16)
	Address      uint16     `toml:"address"`       // Register starting address (decimal)
	Quantity     uint16     `toml:"quantity"`      // Number of registers
	DataType     DataType   `toml:"data_type"`     // Data type for parsing
	Scale        float64    `toml:"scale"`         // Scaling factor (default 1.0)
	Offset       float64    `toml:"offset"`        // Offset value (default 0.0)
	Unit         string     `toml:"unit"`          // Engineering unit (e.g., "°C", "%", "V")
	Access       AccessType `toml:"access"`        // Access permission: "ro", "rw", "wo" (default "ro")
	// NOTE：目前Access没有用到
	CacheEnabled bool `toml:"cache_enabled"` // Enable local caching for resume (default true)
}

// DeviceConfig defines a Modbus device configuration
type DeviceConfig struct {
	Name             string         `toml:"name"`              // Device unique identifier
	Type             ConnectionType `toml:"type"`              // Connection type: "rtu" or "tcp"
	Timeout          time.Duration  `toml:"timeout"`           // Request timeout (default "5s")
	OfflineThreshold time.Duration  `toml:"offline_threshold"` // Offline detection threshold (default "30s")
	EnableMonitor    bool           `toml:"enable_monitor"`    // Enable health monitoring (default true)

	// RTU-specific (only when type = "rtu")
	RTU *RtuSlaveConfig `toml:"rtu,omitempty"`

	// TCP-specific (only when type = "tcp")
	TCP *TcpSlaveConfig `toml:"tcp,omitempty"`

	// Common addressing
	SlaveID byte `toml:"slave_id"` // Modbus slave address (required for RTU, usually 1 for TCP)

	// Data points
	Points       []*PointConfig `toml:"points"`        // List of data points to read/write
	PollInterval time.Duration  `toml:"poll_interval"` // Polling interval (e.g., "10s")
}

// Validate checks if the device configuration is valid
func (d *DeviceConfig) Validate() error {
	if d.Name == "" {
		return &ValidationError{Field: "name", Message: "device name cannot be empty"}
	}

	if d.Type != ConnectionTypeRTU && d.Type != ConnectionTypeTCP {
		return &ValidationError{Field: "type", Message: "must be 'rtu' or 'tcp'"}
	}

	if d.Type == ConnectionTypeRTU && d.RTU == nil {
		return &ValidationError{Field: "rtu", Message: "RTU config required when type=rtu"}
	}

	if d.Type == ConnectionTypeTCP && d.TCP == nil {
		return &ValidationError{Field: "tcp", Message: "TCP config required when type=tcp"}
	}

	// NOTE：slave id的类型改过，从int到byte
	if d.SlaveID <= 0 || d.SlaveID > 247 {
		return &ValidationError{Field: "slave_id", Message: "must be between 1 and 247"}
	}

	if len(d.Points) == 0 {
		return &ValidationError{Field: "points", Message: "at least one point required"}
	}

	// Validate each point
	for i, point := range d.Points {
		if err := point.Validate(); err != nil {
			return &ValidationError{Field: "points[" + string(rune('0'+i)) + "]", Message: err.Error()}
		}
	}

	return nil
}

// Validate checks if the point configuration is valid
func (p *PointConfig) Validate() error {
	if p.Name == "" {
		return &ValidationError{Field: "name", Message: "point name cannot be empty"}
	}

	// Validate function code
	validFunctionCodes := map[int]bool{
		1:  true, // Read Coils
		2:  true, // Read Discrete Inputs
		3:  true, // Read Holding Registers
		4:  true, // Read Input Registers
		5:  true, // Write Single Coil
		6:  true, // Write Single Register
		16: true, // Write Multiple Registers
	}

	if !validFunctionCodes[p.FunctionCode] {
		return &ValidationError{
			Field:   "function_code",
			Message: "must be 1, 2, 3, 4, 5, 6, or 16",
		}
	}

	// Validate access vs function code compatibility
	if p.Access == AccessReadOnly && (p.FunctionCode == 5 || p.FunctionCode == 6 || p.FunctionCode == 16) {
		return &ValidationError{
			Field:   "access",
			Message: "read-only access incompatible with write function codes",
		}
	}

	// Validate data type
	validDataTypes := map[DataType]bool{
		DataTypeBool:    true,
		DataTypeInt16:   true,
		DataTypeUint16:  true,
		DataTypeInt32:   true,
		DataTypeUint32:  true,
		DataTypeFloat32: true,
	}

	if !validDataTypes[p.DataType] {
		return &ValidationError{
			Field:   "data_type",
			Message: "must be bool, int16, uint16, int32, uint32, or float32",
		}
	}

	// Validate register count for multi-register types
	if p.Quantity <= 0 {
		return &ValidationError{
			Field:   "quantity",
			Message: "must be greater than 0",
		}
	}

	return nil
}

// ValidationError represents a configuration validation error
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return e.Field + ": " + e.Message
}

/// END: MODBUS CONFIGURATION STRUCTS
