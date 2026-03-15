package config

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/spf13/viper"
)

type Side int

const (
	Server Side = iota
	Client
)

type ClientConfig struct {
	ServerUrl string `mapstructure:"server"`
	Port      int    `mapstructure:"port"`
	LogLevel  string `mapstructure:"log_level"`
	Sn        string `mapstructure:"sn"`
	Token     string `mapstructure:"token"`
}

type ServerConfig struct {
	Port     int         `mapstructure:"port"`
	LogLevel string      `mapstructure:"log_level"`
	Clients  []ClientDef `mapstructure:"clients"`
}

type ClientDef struct {
	Sn          string `mapstructure:"sn"`
	Token       string `mapstructure:"token"`
	Description string `mapstructure:"description"`
}

// ==================== 编译期常量 ====================
// 这些是应用的默认值，可以在编译时通过 ldflags 覆盖
const (
	DefaultServerHost = "http://localhost"
	DefaultServerPort = 8883
	DefaultLogLevel   = "info"
)

// ==================== 运行时配置变量 ====================
// 可以通过 ldflags 在编译时修改这些值
// 例如: go build -ldflags "-X 'LEPG/internal/config.BuildVersion=1.0.0'"
var (
	BuildVersion = "dev"     // 构建版本
	BuildTime    = "unknown" // 构建时间
	GitCommit    = "unknown" // Git提交哈希
)

// GetVersionInfo 返回应用的版本信息
func GetVersionInfo() string {
	return fmt.Sprintf("Version: %s\nCommit: %s\nBuilt at: %s",
		BuildVersion, GitCommit, BuildTime)
}

var defaultClientValues = map[string]any{
	"server":    DefaultServerHost,
	"port":      DefaultServerPort,
	"log_level": DefaultLogLevel,
	"sn":        "",
	"token":     "",
}

var defaultServerValues = map[string]any{
	"port":      DefaultServerPort,
	"log_level": DefaultLogLevel,
}

// ==================== 单例状态 ====================
var (
	clientConfigInstance *ClientConfig
	clientConfigOnce     sync.Once
	clientConfigErr      error

	serverConfigInstance *ServerConfig
	serverConfigOnce     sync.Once
	serverConfigErr      error
)

// SetFlagValues sets the values from command line flags before loading config
// This ensures flag values have higher priority than config file values
func SetFlagValues(serverUrl string, port int) {
	if serverUrl != "" {
		viper.Set("server", serverUrl)
	}
	if port != 0 {
		viper.Set("port", port)
	}
}

// loadDotEnv 尝试加载 .env 文件（如果存在）
// .env 文件的值会被环境变量覆盖，但会覆盖 TOML 配置文件的值
func loadDotEnv() {
	// 检查 .env 文件是否存在
	if _, err := os.Stat(".env"); os.IsNotExist(err) {
		// .env 文件不存在，跳过
		return
	}

	// 设置 .env 文件配置
	viper.SetConfigFile(".env")
	viper.SetConfigType("env")

	// 读取 .env 文件（非致命错误，文件不存在或格式错误不会中断程序）
	if err := viper.MergeInConfig(); err != nil {
		// 如果是配置文件未找到错误，忽略
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			// 其他错误（如格式错误）也忽略，.env 是可选的
			fmt.Printf("Warning: Error reading .env file: %v\n", err)
		}
	}
}

func LoadConfig() error {
	// 尝试加载 .env 文件（如果存在）
	loadDotEnv()

	viper.AutomaticEnv()
	viper.SetConfigName("config")
	viper.SetConfigType("toml")
	viper.AddConfigPath(".")
	viper.AddConfigPath("./config")
	err := viper.ReadInConfig()
	if err != nil {
		return err
	}
	return nil
}

func LoadConfigWithPath(path string) error {
	// 尝试加载 .env 文件（如果存在）
	loadDotEnv()

	viper.AutomaticEnv()
	viper.SetConfigName("config")
	viper.SetConfigType("toml")
	viper.AddConfigPath(path)
	err := viper.ReadInConfig()
	if err != nil {
		return err
	}
	return nil
}

func InitConfig(side Side) {
	var value map[string]any
	switch side {
	case Client:
		value = defaultClientValues
	case Server:
		value = defaultServerValues
	}
	for k, v := range value {
		viper.SetDefault(k, v)
	}

	viper.WriteConfigAs("config.toml")
}

type ConfigNotSetError struct {
	Code int
	Msg  string
}

func (e *ConfigNotSetError) Error() string {
	return e.Msg
}

func CheckConfig(side Side) error {
	var value map[string]any
	switch side {
	case Client:
		value = defaultClientValues
	case Server:
		value = defaultServerValues
	}
	var MissingConfigs []string
	for k := range value {
		if !viper.IsSet(k) {
			MissingConfigs = append(MissingConfigs, k)
		}
	}
	if len(MissingConfigs) > 0 {
		return &ConfigNotSetError{
			Code: 1,
			Msg:  "Missing configs: " + strings.Join(MissingConfigs, ", "),
		}
	}
	return nil
}

// ==================== 单例访问函数 ====================

// unmarshalClientConfig 从 viper 反序列化客户端配置
func unmarshalClientConfig() (*ClientConfig, error) {
	cfg := &ClientConfig{}
	if err := viper.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal client config: %w", err)
	}
	return cfg, nil
}

// unmarshalServerConfig 从 viper 反序列化服务端配置
func unmarshalServerConfig() (*ServerConfig, error) {
	cfg := &ServerConfig{}
	if err := viper.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal server config: %w", err)
	}
	return cfg, nil
}

// GetClientConfig 返回客户端配置单例
// 必须在 SetFlagValues() 和 LoadConfig() 之后调用
func GetClientConfig() (*ClientConfig, error) {
	clientConfigOnce.Do(func() {
		clientConfigInstance, clientConfigErr = unmarshalClientConfig()
	})
	return clientConfigInstance, clientConfigErr
}

// GetServerConfig 返回服务端配置单例
// 必须在 SetFlagValues() 和 LoadConfig() 之后调用
func GetServerConfig() (*ServerConfig, error) {
	serverConfigOnce.Do(func() {
		serverConfigInstance, serverConfigErr = unmarshalServerConfig()
	})
	return serverConfigInstance, serverConfigErr
}

// ResetSingletons 重置单例状态（仅用于测试）
func ResetSingletons() {
	clientConfigInstance = nil
	clientConfigOnce = sync.Once{}
	clientConfigErr = nil

	serverConfigInstance = nil
	serverConfigOnce = sync.Once{}
	serverConfigErr = nil
}
