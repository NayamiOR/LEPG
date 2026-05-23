package client

import (
	"LEPG/internal/config"
	"LEPG/internal/errors"
)

// NewProviders 创建带有客户端默认值的配置提供者
func NewProviders(flagValues map[string]any, cfgFile string) *config.Providers {
	return config.NewProviders(flagValues, cfgFile, defaultClientValues)
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
		errs = append(errs, errors.NewConfigInvalidError("max_retry","must be non-negative"))
	}
	if c.RetryInterval < 0 {
		errs = append(errs, errors.NewConfigInvalidError("retry_interval","must be non-negative"))
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
