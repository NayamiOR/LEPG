package client

import (
	"fmt"
	"strings"
	"sync"

	"github.com/spf13/viper"
)

type ClientConfig struct {
	ServerUrl string `mapstructure:"server"`
	Port      int    `mapstructure:"port"`
	LogLevel  string `mapstructure:"log_level"`
	Sn        string `mapstructure:"sn"`
	Token     string `mapstructure:"token"`
}


var defaultClientValues = map[string]any{
	"server":    "http://localhost",
	"port":      8883,
	"log_level": "info",
	"sn":        "",
	"token":     "",
}

var clientConfigInstance *ClientConfig
var clientConfigOnce sync.Once

func GetClientConfig() *ClientConfig {
	return clientConfigInstance
}

func SetClientConfig(cfg *ClientConfig) {
	clientConfigOnce.Do(func() {
		clientConfigInstance = cfg
	})
}

func UnmarshalClientConfig(unmarshal func(any) error) (*ClientConfig, error) {
	cfg := &ClientConfig{}
	if err := unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal client config: %w", err)
	}
	return cfg, nil
}

// UnmarshalClientConfigFromViper 从 viper 反序列化客户端配置
func UnmarshalClientConfigFromViper() (*ClientConfig, error) {
	return UnmarshalClientConfig(func(rawVal any) error {
		return viper.Unmarshal(rawVal)
	})
}

// GetDefaultValues 返回客户端默认配置值
func GetDefaultValues() map[string]any {
	return defaultClientValues
}

// GetRequiredKeys 返回必需的配置键
func GetRequiredKeys() []string {
	keys := make([]string, 0, len(defaultClientValues))
	for k := range defaultClientValues {
		keys = append(keys, k)
	}
	return keys
}

// CheckConfigNotSet 检查配置是否缺失
func CheckConfigNotSet() error {
	var missingConfigs []string
	for k := range defaultClientValues {
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
