package client

import (
	"LEPG/internal/config"
	"LEPG/internal/errors"
)

type ClientConfig struct {
	ServerUrl string
	Port      int
	LogLevel  string
	Sn        string
	Token     string
}

var defaultClientValues = map[string]any{
	"server":    "http://localhost",
	"port":      8883,
	"log_level": "info",
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

	// 验证必需配置
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Validate 验证配置
func (c *ClientConfig) Validate() error {
	var missing []string

	if c.Sn == "" {
		missing = append(missing, "sn")
	}
	if c.Token == "" {
		missing = append(missing, "token")
	}

	if len(missing) > 0 {
		return errors.NewConfigNotSetError(missing)
	}
	return nil
}

// GetDefaultValues 返回客户端默认配置值
func GetDefaultValues() map[string]any {
	return defaultClientValues
}
