package server

import (
	"LEPG/internal/config"
	"LEPG/internal/errors"
	"LEPG/internal/output"
)

// ServerMetaConfig 元设置：控制 provider 链构建和 initCmd 行为。
// MetaConfig 在 provider 链构建之前即已确定，且其值在后续填充业务配置过程中不再变更。
type ServerMetaConfig struct {
	SearchPath string // 运行时默认搜索路径
	InitPath   string // initCmd 写入目标
}

func newServerMetaConfig() *ServerMetaConfig {
	return &ServerMetaConfig{
		SearchPath: "config/server.toml",
		InitPath:   "/etc/lepgs/config.toml",
	}
}

// ServerConfig 业务设置：由 provider 链解析
type ServerConfig struct {
	Port     int    `config:"port"      default:"8883"                    sources:"file,flag,env,default"`
	LogLevel string `config:"log_level" default:"info"                    sources:"file,env,default"`
	LogPath  string `config:"log_path"  default:"logs/"                   sources:"file,env,default"`
	DataPath string `config:"data_path" default:"/var/cache/lepgs/lepgs.db" sources:"file,env,default"`

	LogMaxSize    int `config:"log_max_size"    default:"10" sources:"file,env,default"`
	LogMaxBackups int `config:"log_max_backups" default:"3"  sources:"file,env,default"`
	LogMaxAge     int `config:"log_max_age"     default:"28" sources:"file,env,default"`

	Mqtt  MqttConfig  // 子结构体，PopulateFromProvider 递归填充
	Redis RedisConfig // 同上
	Pg    PgConfig    // PostgreSQL 连接配置

	Clients []ClientDef           // 无 sources tag，通过 Unmarshal 填充
	Outputs []output.OutputConfig // [[outputs]] 数组，通过 Unmarshal 填充
}

// MqttConfig MQTT broker 监听配置
type MqttConfig struct {
	TCPAddr string `config:"mqtt_tcp" default:"127.0.0.1:1883" sources:"file,env,default"`
	WSAddr  string `config:"mqtt_ws"  default:"127.0.0.1:8083" sources:"file,env,default"`
}

// RedisConfig Redis 连接配置
type RedisConfig struct {
	Addr     string `config:"redis_addr"     default:"127.0.0.1:6379" sources:"file,env,default"`
	Password string `config:"redis_password"                          sources:"file,env"` // 敏感字段，无 default，init 不生成此 key
	DB       int    `config:"redis_db"       default:"0"              sources:"file,env,default"`
}

// PgConfig PostgreSQL 连接配置
type PgConfig struct {
	Host     string `config:"pg_host"     default:"127.0.0.1" sources:"file,env,default"`
	Port     int    `config:"pg_port"     default:"5432"      sources:"file,env,default"`
	User     string `config:"pg_user"     default:"lepgs"     sources:"file,env,default"`
	Password string `config:"pg_password"                     sources:"file,env"` // 敏感字段，无 default
	DBName   string `config:"pg_dbname"   default:"lepgs"     sources:"file,env,default"`
	SSLMode  string `config:"pg_sslmode"  default:"disable"   sources:"file,env,default"`
}

// ClientDef 客户端定义，通过 Unmarshal 从 [[clients]] 填充
type ClientDef struct {
	Sn          string `mapstructure:"sn"`
	Token       string `mapstructure:"token"`
	Description string `mapstructure:"description"`
}

// NewProviders 创建带有服务端默认值的配置提供者
func NewProviders(flagValues map[string]any, cfgFile string) *config.Providers {
	meta := newServerMetaConfig()
	defaults := config.ExtractDefaults(&ServerConfig{})
	return config.NewProviders(flagValues, cfgFile, meta.SearchPath, defaults)
}

// InitServerConfig 初始化服务端配置
func InitServerConfig(provider config.IProvider) (*ServerConfig, error) {
	cfg := &ServerConfig{}
	if err := config.PopulateFromProvider(cfg, provider); err != nil {
		return nil, err
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

		var outputsWrapper struct {
			Outputs []output.OutputConfig `mapstructure:"outputs"`
		}
		if err := u.Unmarshal(&outputsWrapper); err != nil {
			return nil, errors.Wrap(err, "failed to unmarshal outputs")
		}
		cfg.Outputs = outputsWrapper.Outputs
	}

	for _, o := range cfg.Outputs {
		if err := o.Validate(); err != nil {
			return nil, err
		}
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Validate 验证服务端业务配置
func (c *ServerConfig) Validate() error {
	var errs []error

	if c.Port <= 0 || c.Port > 65535 {
		errs = append(errs, errors.NewConfigInvalidError("port", "must be 1-65535"))
	}

	validLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if !validLevels[c.LogLevel] {
		errs = append(errs, errors.NewConfigInvalidError("log_level", "must be debug/info/warn/error"))
	}

	if c.Pg.Host == "" {
		errs = append(errs, errors.NewConfigInvalidError("pg_host", "cannot be empty"))
	}
	if c.Pg.DBName == "" {
		errs = append(errs, errors.NewConfigInvalidError("pg_dbname", "cannot be empty"))
	}

	return errors.NewConfigValidationErrors(errs)
}

// GetDefaultValues 返回服务端默认配置值，供 initCmd 使用
func GetDefaultValues() map[string]any {
	meta := newServerMetaConfig()
	defaults := config.ExtractDefaults(&ServerConfig{})
	defaults["config_path"] = meta.InitPath
	return defaults
}
