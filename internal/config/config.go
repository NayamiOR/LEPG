package config

import (
	"fmt"
	"strings"

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
}

type ServerConfig struct {
	Port     string `mapstructure:"port"`
	LogLevel string `mapstructure:"log_level"`
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
	BuildVersion = "dev"       // 构建版本
	BuildTime   = "unknown"   // 构建时间
	GitCommit   = "unknown"   // Git提交哈希
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
}

var defaultServerValues = map[string]any{
	"port":      DefaultServerPort,
	"log_level": DefaultLogLevel,
}

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

func LoadConfig() error {
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
