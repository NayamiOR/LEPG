package server

import (
	"LEPG/internal/config"
	"LEPG/internal/errors"
)

const DefaultConfigFile = "/etc/lepgs/config.toml"

// NewProviders 创建带有服务端默认值的配置提供者
func NewProviders(flagValues map[string]any, cfgFile string) *config.Providers {
	return config.NewProviders(flagValues, cfgFile, DefaultConfigFile, defaultServerValues)
}

type ServerConfig struct {
	Port     int
	LogLevel string
	DataPath string
	Clients  []ClientDef
	Mqtt     MqttConfig
	Redis    RedisConfig
}

type RedisConfig struct {
	Addr     string
	Password string
	DB       int
}

type MqttConfig struct {
	TCPAddr string
	WSAddr  string
}

type ClientDef struct {
	Sn          string
	Token       string
	Description string
}

var defaultServerValues = map[string]any{
	"port":           8883,
	"log_level":      "info",
	"mqtt_tcp":       "127.0.0.1:1883",
	"mqtt_ws":        "127.0.0.1:8083",
	"redis_addr":     "127.0.0.1:6379",
	"redis_password": "",
	"redis_db":       0,
}

// InitServerConfig 初始化服务端配置
func InitServerConfig(provider config.IProvider) (*ServerConfig, error) {
	cfg := &ServerConfig{}

	// 简单字段从 provider 获取（DefaultProvider 兜底）
	cfg.Port = provider.GetInt("port")
	cfg.LogLevel = provider.GetString("log_level")
	cfg.DataPath = "/var/cache/lepgs/lepgs.db"
	cfg.Mqtt = MqttConfig{
		TCPAddr: provider.GetString("mqtt_tcp"),
		WSAddr:  provider.GetString("mqtt_ws"),
	}
	cfg.Redis = RedisConfig{
		Addr:     provider.GetString("redis_addr"),
		Password: provider.GetString("redis_password"),
		DB:       provider.GetInt("redis_db"),
	}

	// 复杂嵌套结构通过类型断言获取 unmarshal 能力
	if u, ok := provider.(config.IUnmarshaler); ok {
		var clientsWrapper struct {
			Clients []ClientDef `mapstructure:"clients"`
		}
		if err := u.Unmarshal(&clientsWrapper); err != nil {
			return nil, errors.Wrap(err, "failed to unmarshal clients")
		}
		cfg.Clients = clientsWrapper.Clients
	}

	return cfg, nil
}

// GetDefaultValues 返回服务端默认配置值
func GetDefaultValues() map[string]any {
	return defaultServerValues
}
