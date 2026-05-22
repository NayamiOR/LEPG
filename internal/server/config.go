package server

import (
	"fmt"
	"strings"
	"github.com/spf13/viper"
)

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

var defaultServerValues = map[string]any{
	"port":      8883,
	"log_level": "info",
}

var serverConfigInstance *ServerConfig

func GetServerConfig() *ServerConfig {
	return serverConfigInstance
}

func SetServerConfig(cfg *ServerConfig) {
	serverConfigInstance = cfg
}

func UnmarshalServerConfig(unmarshal func(any) error) (*ServerConfig, error) {
	cfg := &ServerConfig{}
	if err := unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal server config: %w", err)
	}
	return cfg, nil
}

// UnmarshalServerConfigFromViper 从 viper 反序列化服务端配置
func UnmarshalServerConfigFromViper() (*ServerConfig, error) {
	return UnmarshalServerConfig(func(rawVal any) error {
		return viper.Unmarshal(rawVal)
	})
}

// GetDefaultValues 返回服务端默认配置值
func GetDefaultValues() map[string]any {
	return defaultServerValues
}

// GetRequiredKeys 返回必需的配置键
func GetRequiredKeys() []string {
	keys := make([]string, 0, len(defaultServerValues))
	for k := range defaultServerValues {
		keys = append(keys, k)
	}
	return keys
}

// CheckConfigNotSet 检查配置是否缺失
func CheckConfigNotSet() error {
	var missingConfigs []string
	for k := range defaultServerValues {
		if !viper.IsSet(k) {
			missingConfigs = append(missingConfigs, k)
		}
	}
	if len(missingConfigs) > 0 {
		return &ConfigNotSetError{
			Code: 1,
			Msg:  "Missing configs: " + strings.Join(missingConfigs, ", "),
		}
	}
	return nil
}

type ConfigNotSetError struct {
	Code int
	Msg  string
}

func (e *ConfigNotSetError) Error() string {
	return e.Msg
}
