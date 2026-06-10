package client

import (
	"LEPG/internal/config"
	"LEPG/internal/errors"
	"LEPG/internal/model"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// ClientMetaConfig 元设置：控制 provider 链构建和 initCmd 行为。
// MetaConfig 在 provider 链构建之前即已确定，且其值在后续填充业务配置过程中不再变更。
type ClientMetaConfig struct {
	SearchPath string // 运行时默认搜索路径
	InitPath   string // initCmd 写入目标
}

func newClientMetaConfig() *ClientMetaConfig {
	return &ClientMetaConfig{
		SearchPath: "config/client.toml",
		InitPath:   "/etc/lepgc/config.toml",
	}
}

// NewProviders 创建带有客户端默认值的配置提供者
func NewProviders(flagValues map[string]any, cfgFile string) *config.Providers {
	meta := newClientMetaConfig()
	defaults := config.ExtractDefaults(&ClientConfig{})
	return config.NewProviders(flagValues, cfgFile, meta.SearchPath, defaults)
}

// ClientConfig 业务设置：由 provider 链解析
type ClientConfig struct {
	ServerUrl       string `config:"server"            default:"http://localhost" sources:"file,flag,env,default"`
	Port            int    `config:"port"              default:"8883"             sources:"file,flag,env,default"`
	LogLevel        string `config:"log_level"         default:"info"             sources:"file,env,default"`
	Sn              string `config:"sn"                                           sources:"file,env"` // 凭据，禁止 flag
	Token           string `config:"token"                                        sources:"file,env"` // 凭据，禁止 flag
	MaxRetry        int    `config:"max_retry"         default:"10"               sources:"file,env,default"`
	RetryInterval   int    `config:"retry_interval"    default:"5000"              sources:"file,env,default"`
	BufferSize      int    `config:"buffer_size"       default:"1000"              sources:"file,env,default"`
	UploadBatchSize int    `config:"upload_batch_size" default:"100"               sources:"file,env,default"`
	UploadInterval  int    `config:"upload_interval"   default:"5000"              sources:"file,env,default"`

	Paths PathsConfig // 子结构体，递归填充

	// Unmarshal 字段 — 无 sources tag，由 IUnmarshaler 填充
	Devices []*DeviceConfig
	Mqtt    *MqttConfig
}

// PathsConfig 文件路径配置
type PathsConfig struct {
	LogPath  string `config:"log_path"  default:"./logs/client.log" sources:"file,env,default"`
	DataPath string `config:"data_path" default:"./data/data.db"   sources:"file,env,default"`
}

// InitClientConfig 初始化客户端配置
func InitClientConfig(provider config.IProvider) (*ClientConfig, error) {
	cfg := &ClientConfig{}
	if err := config.PopulateFromProvider(cfg, provider); err != nil {
		return nil, err
	}

	// 复杂嵌套结构通过类型断言获取 unmarshal 能力
	if u, ok := provider.(config.IUnmarshaler); ok {
		var devicesWrapper struct {
			Devices []*DeviceConfig `mapstructure:"devices"`
		}
		if err := u.Unmarshal(&devicesWrapper); err != nil {
			return nil, errors.Wrap(err, "failed to unmarshal devices")
		}

		// 验证每个设备配置
		for i, device := range devicesWrapper.Devices {
			if err := device.Validate(); err != nil {
				return nil, fmt.Errorf("device[%d] validation failed: %w", i, err)
			}
		}
		cfg.Devices = devicesWrapper.Devices

		// 检查设备名称唯一性
		seen := make(map[string]bool, len(cfg.Devices))
		for _, d := range cfg.Devices {
			if seen[d.Name] {
				return nil, fmt.Errorf("duplicate device name: %s", d.Name)
			}
			seen[d.Name] = true
		}
	}

	// MQTT broker + virtual device definitions
	if u, ok := provider.(config.IUnmarshaler); ok {
		var mqttWrapper struct {
			Mqtt *MqttConfig `mapstructure:"mqtt"`
		}
		if err := u.Unmarshal(&mqttWrapper); err != nil {
			return nil, errors.Wrap(err, "failed to unmarshal mqtt config")
		}
		if mqttWrapper.Mqtt != nil {
			if err := mqttWrapper.Mqtt.Validate(); err != nil {
				return nil, fmt.Errorf("mqtt config validation failed: %w", err)
			}
			cfg.Mqtt = mqttWrapper.Mqtt
		}
	}

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

	if c.Port <= 0 || c.Port > 65535 {
		errs = append(errs, errors.NewConfigInvalidError("port", "must be 1-65535"))
	}

	validLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if !validLevels[c.LogLevel] {
		errs = append(errs, errors.NewConfigInvalidError("log_level", "must be debug/info/warn/error"))
	}

	if c.MaxRetry < 0 {
		errs = append(errs, errors.NewConfigInvalidError("max_retry", "must be non-negative"))
	}
	if c.RetryInterval < 0 {
		errs = append(errs, errors.NewConfigInvalidError("retry_interval", "must be non-negative"))
	}

	if c.BufferSize <= 0 {
		errs = append(errs, errors.NewConfigInvalidError("buffer_size", "must be positive"))
	}
	if c.UploadBatchSize <= 0 {
		errs = append(errs, errors.NewConfigInvalidError("upload_batch_size", "must be positive"))
	}
	if c.UploadInterval <= 0 {
		errs = append(errs, errors.NewConfigInvalidError("upload_interval", "must be positive"))
	}

	if c.Paths.DataPath == "" {
		errs = append(errs, errors.NewConfigInvalidError("data_path", "cannot be empty"))
	}

	if len(errs) > 0 {
		return errors.NewConfigValidationErrors(errs)
	}
	return nil
}

// GetDefaultValues 返回客户端默认配置值，供 initCmd 使用
func GetDefaultValues() map[string]any {
	meta := newClientMetaConfig()
	defaults := config.ExtractDefaults(&ClientConfig{})
	defaults["config_path"] = meta.InitPath
	return defaults
}

/// START: MODBUS CONFIGURATION STRUCTS

// RtuSlaveConfig contains RTU-specific connection parameters
type RtuSlaveConfig struct {
	Port     string `toml:"port" mapstructure:"port"`           // Serial port (e.g., "/dev/ttyS0" or "COM3")
	BaudRate int    `toml:"baud_rate" mapstructure:"baud_rate"` // Baud rate (e.g., 9600, 19200)
	DataBits int    `toml:"data_bits" mapstructure:"data_bits"` // Data bits (default 8)
	Parity   string `toml:"parity" mapstructure:"parity"`       // Parity: "N", "E", "O" (default "N")
	StopBits int    `toml:"stop_bits" mapstructure:"stop_bits"` // Stop bits (default 1)
}

// TcpSlaveConfig contains TCP-specific connection parameters
type TcpSlaveConfig struct {
	Host string `toml:"host" mapstructure:"host"` // IP address or hostname
	Port int    `toml:"port" mapstructure:"port"` // Port number (default 502)
}

// ModbusPointConfig defines a single data point on a Modbus device
type ModbusPointConfig struct {
	Name         string           `toml:"name" mapstructure:"name"`                   // Point identifier (JSON field name)
	FunctionCode int              `toml:"function_code" mapstructure:"function_code"` // Modbus function code (1/2/3/4/5/6/16)
	Address      uint16           `toml:"address" mapstructure:"address"`             // Register starting address (decimal)
	Quantity     uint16           `toml:"quantity" mapstructure:"quantity"`           // Number of registers
	DataType     model.DataType   `toml:"data_type" mapstructure:"data_type"`         // Data type for parsing
	ByteOrder    model.ByteOrder  `toml:"byte_order" mapstructure:"byte_order"`       // Byte order for multi-register types (default "abcd")
	Scale        float64          `toml:"scale" mapstructure:"scale"`                 // Scaling factor (default 1.0)
	Offset       float64          `toml:"offset" mapstructure:"offset"`               // Offset value (default 0.0)
	Unit         string           `toml:"unit" mapstructure:"unit"`                   // Engineering unit (e.g., "°C", "%", "V")
	Access       model.AccessType `toml:"access" mapstructure:"access"`               // Access permission: "ro", "rw", "wo" (default "ro")
	// NOTE：目前Access没有用到
	// NOTE：目前CacheEnabled没有用到
	CacheEnabled bool `toml:"cache_enabled" mapstructure:"cache_enabled"` // Enable local caching for resume (default true)
}

type TopicConfig struct {
	Topic     string `toml:"topic" mapstructure:"topic"`
	QoS       byte   `toml:"qos" mapstructure:"qos"`
	PointName string `toml:"point_name" mapstructure:"point_name"`
	Unit      string `toml:"unit" mapstructure:"unit"`
	Retain    bool   `toml:"retain" mapstructure:"retain"`
}

// DeviceConfig defines a Modbus device configuration
type DeviceConfig struct {
	Name             string               `toml:"name" mapstructure:"name"`                           // Device unique identifier
	Type             model.ConnectionType `toml:"type" mapstructure:"type"`                           // Connection type: "rtu" or "tcp"
	Timeout          time.Duration        `toml:"timeout" mapstructure:"timeout"`                     // Request timeout (default "5s")
	OfflineThreshold time.Duration        `toml:"offline_threshold" mapstructure:"offline_threshold"` // Offline detection threshold (default "30s")
	EnableMonitor    bool                 `toml:"enable_monitor" mapstructure:"enable_monitor"`       // Enable health monitoring (default true)

	// RTU-specific (only when type = "rtu")
	RTU *RtuSlaveConfig `toml:"rtu,omitempty" mapstructure:"rtu"`

	// TCP-specific (only when type = "tcp")
	TCP *TcpSlaveConfig `toml:"tcp,omitempty" mapstructure:"tcp"`

	// Common addressing
	SlaveID byte `toml:"slave_id" mapstructure:"slave_id"` // Modbus slave address (required for RTU, usually 1 for TCP)

	// Data points
	Points       []*ModbusPointConfig `toml:"points" mapstructure:"points"`               // List of data points to read/write
	PollInterval time.Duration        `toml:"poll_interval" mapstructure:"poll_interval"` // Polling interval (e.g., "10s")
}

// Validate checks if the device configuration is valid
func (d *DeviceConfig) Validate() error {
	if d.Name == "" {
		return &ValidationError{Field: "name", Message: "device name cannot be empty"}
	}

	if d.Type != model.ConnectionTypeRTU && d.Type != model.ConnectionTypeTCP {
		return &ValidationError{Field: "type", Message: "must be 'rtu' or 'tcp'"}
	}

	if d.Type == model.ConnectionTypeRTU && d.RTU == nil {
		return &ValidationError{Field: "rtu", Message: "RTU config required when type=rtu"}
	}

	if d.Type == model.ConnectionTypeTCP && d.TCP == nil {
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

// Hash generates a deterministic 16-char hex identifier for the device.
// RTU: port + slaveID; TCP: host + port + slaveID.
func (d *DeviceConfig) Hash() (string, error) {
	var input string // Hash 只包含设备连接方式
	switch d.Type {
	case model.ConnectionTypeRTU:
		if d.RTU == nil {
			return "", fmt.Errorf("rtu config is nil")
		}
		input = fmt.Sprintf("rtu:%s:%d", d.RTU.Port, d.SlaveID)
	case model.ConnectionTypeTCP:
		if d.TCP == nil {
			return "", fmt.Errorf("tcp config is nil")
		}
		input = fmt.Sprintf("tcp:%s:%d:%d", d.TCP.Host, d.TCP.Port, d.SlaveID)
	default:
		return "", fmt.Errorf("unsupported connection type: %s", d.Type)
	}
	h := sha256.Sum256([]byte(input))
	return hex.EncodeToString(h[:8]), nil
}

// Validate checks if the point configuration is valid
func (p *ModbusPointConfig) Validate() error {
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
	if p.Access == model.AccessReadOnly && (p.FunctionCode == 5 || p.FunctionCode == 6 || p.FunctionCode == 16) {
		return &ValidationError{
			Field:   "access",
			Message: "read-only access incompatible with write function codes",
		}
	}

	// Validate data type
	validDataTypes := map[model.DataType]bool{
		model.DataTypeBool:    true,
		model.DataTypeInt16:   true,
		model.DataTypeUint16:  true,
		model.DataTypeInt32:   true,
		model.DataTypeUint32:  true,
		model.DataTypeFloat32: true,
	}

	if !validDataTypes[p.DataType] {
		return &ValidationError{
			Field:   "data_type",
			Message: "must be bool, int16, uint16, int32, uint32, or float32",
		}
	}

	// Validate byte order for multi-register types
	if p.DataType == model.DataTypeInt32 || p.DataType == model.DataTypeUint32 || p.DataType == model.DataTypeFloat32 {
		// Set default to Big-Endian if not specified
		if p.ByteOrder == "" {
			p.ByteOrder = model.ByteOrderBigEndian
		}

		validByteOrders := map[model.ByteOrder]bool{
			model.ByteOrderBigEndian:       true,
			model.ByteOrderLittleEndian:    true,
			model.ByteOrderMidLittleEndian: true,
			model.ByteOrderMidBigEndian:    true,
		}

		if !validByteOrders[p.ByteOrder] {
			return &ValidationError{
				Field:   "byte_order",
				Message: "must be abcd, dcba, badc, or cdab",
			}
		}
	} else {
		// For single-register types, byte order should not be set
		if p.ByteOrder != "" && p.ByteOrder != model.ByteOrderBigEndian {
			return &ValidationError{
				Field:   "byte_order",
				Message: "only applicable for multi-register types (int32, uint32, float32)",
			}
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

/// START: MQTT CONFIGURATION STRUCTS

type MQTTDeviceConfig struct {
	Name             string         `toml:"name" mapstructure:"name"`
	ClientID         string         `toml:"client_id" mapstructure:"client_id"`
	Username         string         `toml:"username" mapstructure:"username"`
	Password         string         `toml:"password" mapstructure:"password"`
	KeepAlive        time.Duration  `toml:"keep_alive" mapstructure:"keep_alive"`
	CleanSession     bool           `toml:"clean_session" mapstructure:"clean_session"`
	OfflineThreshold time.Duration  `toml:"offline_threshold" mapstructure:"offline_threshold"`
	EnableMonitor    bool           `toml:"enable_monitor" mapstructure:"enable_monitor"`
	Topics           []*TopicConfig `toml:"topics" mapstructure:"topics"`
}

func (d *MQTTDeviceConfig) Validate() error {
	if d.Name == "" {
		return &ValidationError{Field: "name", Message: "cannot be empty"}
	}
	if len(d.Topics) == 0 {
		return &ValidationError{Field: "topics", Message: "at least one topic required"}
	}
	seen := make(map[string]bool, len(d.Topics))
	for i, t := range d.Topics {
		if err := t.Validate(); err != nil {
			return fmt.Errorf("topics[%d]: %w", i, err)
		}
		if seen[t.Topic] {
			return &ValidationError{Field: "topics", Message: fmt.Sprintf("duplicate topic: %s", t.Topic)}
		}
		seen[t.Topic] = true
	}
	return nil
}

func (t *TopicConfig) Validate() error {
	if t.Topic == "" {
		return &ValidationError{Field: "topic", Message: "cannot be empty"}
	}
	if t.PointName == "" {
		return &ValidationError{Field: "point_name", Message: "cannot be empty"}
	}
	if t.QoS > 2 {
		return &ValidationError{Field: "qos", Message: "must be 0, 1, or 2"}
	}
	return nil
}

type MqttConfig struct {
	BrokerAddr string               `toml:"broker_addr" mapstructure:"broker_addr"`
	Devices    []*MQTTDeviceConfig `toml:"devices" mapstructure:"devices"`
}

func (m *MqttConfig) Validate() error {
	if m.BrokerAddr == "" {
		return &ValidationError{Field: "mqtt.broker_addr", Message: "cannot be empty"}
	}
	if len(m.Devices) == 0 {
		return &ValidationError{Field: "mqtt.devices", Message: "at least one device required when mqtt is configured"}
	}
	seen := make(map[string]bool, len(m.Devices))
	for i, d := range m.Devices {
		if err := d.Validate(); err != nil {
			return fmt.Errorf("mqtt.devices[%d]: %w", i, err)
		}
		if seen[d.Name] {
			return &ValidationError{Field: "mqtt.devices", Message: fmt.Sprintf("duplicate device name: %s", d.Name)}
		}
		seen[d.Name] = true
	}
	return nil
}

/// END: MQTT CONFIGURATION STRUCTS

/// START: DEVICE LIST FORMATTING

func formatDeviceList(devices []*DeviceConfig) string {
	var buf strings.Builder
	fmt.Fprintf(&buf, "[Modbus Devices] (%d devices)\n", len(devices))
	for i, d := range devices {
		buf.WriteString(formatModbusDevice(i+1, d))
	}
	return buf.String()
}

func formatModbusDevice(idx int, d *DeviceConfig) string {
	var buf strings.Builder

	var conn string
	switch d.Type {
	case model.ConnectionTypeTCP:
		conn = fmt.Sprintf("tcp %s:%d", d.TCP.Host, d.TCP.Port)
	case model.ConnectionTypeRTU:
		conn = fmt.Sprintf("rtu %s:%d-%d%s%d", d.RTU.Port, d.RTU.BaudRate, d.RTU.DataBits, d.RTU.Parity, d.RTU.StopBits)
	}

	fmt.Fprintf(&buf, " #%-2d %-15s | %-25s | slave=%-3d | poll=%-5s | timeout=%-5s | offline=%-5s | monitor=%v\n",
		idx, `"`+d.Name+`"`, conn, d.SlaveID,
		durShort(d.PollInterval), durShort(d.Timeout), durShort(d.OfflineThreshold), d.EnableMonitor)

	for j, p := range d.Points {
		prefix := "    ├─ "
		if j == len(d.Points)-1 {
			prefix = "    └─ "
		}
		bo := string(p.ByteOrder)
		if bo == "" {
			bo = "-"
		}
		fmt.Fprintf(&buf, "%s%-14s fc=%02d addr=%-4d qty=%-2d %-8s %-5s scale=%-6g off=%-6g unit=%-4s %s\n",
			prefix, p.Name, p.FunctionCode, p.Address, p.Quantity,
			p.DataType, bo, p.Scale, p.Offset, p.Unit, p.Access)
	}

	return buf.String()
}

func formatMqttDeviceList(brokerAddr string, devices []*MQTTDeviceConfig) string {
	var buf strings.Builder
	fmt.Fprintf(&buf, "[MQTT Devices] (%d devices, broker=%s)\n", len(devices), brokerAddr)
	for i, d := range devices {
		buf.WriteString(formatMqttDevice(i+1, d))
	}
	return buf.String()
}

func formatMqttDevice(idx int, d *MQTTDeviceConfig) string {
	var buf strings.Builder

	fmt.Fprintf(&buf, " #%-2d %-15s | client=%-10s | user=%-10s | keepalive=%-5s | clean=%-5v | offline=%-5s | monitor=%v\n",
		idx, `"`+d.Name+`"`, d.ClientID, d.Username,
		durShort(d.KeepAlive), d.CleanSession, durShort(d.OfflineThreshold), d.EnableMonitor)

	for j, t := range d.Topics {
		prefix := "    ├─ "
		if j == len(d.Topics)-1 {
			prefix = "    └─ "
		}
		fmt.Fprintf(&buf, "%s%-20s qos=%-2d point=%-16s unit=%-6s retain=%v\n",
			prefix, t.Topic, t.QoS, t.PointName, t.Unit, t.Retain)
	}

	return buf.String()
}

// durShort returns a short duration string (e.g. "10s", "5ms", "1m30s").
func durShort(d time.Duration) string {
	if d == 0 {
		return "0"
	}
	if d%time.Minute == 0 {
		return fmt.Sprintf("%dm", d/time.Minute)
	}
	if d%time.Second == 0 {
		return fmt.Sprintf("%ds", d/time.Second)
	}
	return d.Truncate(time.Millisecond).String()
}

/// END: DEVICE LIST FORMATTING
