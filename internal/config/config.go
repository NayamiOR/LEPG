package config

import (
	"LEPG/internal/config/provider"
)

// ==================== 编译期常量 ====================
const (
	DefaultServerHost = "http://localhost"
	DefaultServerPort = 8883
	DefaultLogLevel   = "info"
)

// ==================== 运行时配置变量 ====================
var (
	BuildVersion = "dev"
	BuildTime    = "unknown"
	GitCommit    = "unknown"
)

// Providers 包含配置提供者链
type Providers struct {
	Chain IProvider
}

// NewProviders 创建配置提供者
// flagValues: 命令行参数，优先级最高
// cfgFile: 配置文件路径，为空时使用默认路径
func NewProviders(flagValues map[string]any, cfgFile string) *Providers {
	// 1. Flag provider - 最高优先级
	flagProv := provider.NewFlagProvider()
	for k, v := range flagValues {
		flagProv.Set(k, v)
	}

	// 2. Env provider
	envProv := provider.NewEnvProvider("")
	envProv.Load()

	// 3. File provider
	fileProv := provider.NewFileProvider("toml")
	if cfgFile != "" {
		fileProv.SetConfigFile(cfgFile)
	} else {
		fileProv.SetConfigName("config")
		fileProv.AddConfigPath(".")
		fileProv.AddConfigPath("./config")
	}

	// 尝试加载文件，忽略不存在错误
	_ = fileProv.Load()
	_ = fileProv.LoadDotEnv()

	// 4. Default provider - 最低优先级
	defaultProv := provider.NewDefaultProvider(map[string]any{
		"server":    DefaultServerHost,
		"port":      DefaultServerPort,
		"log_level": DefaultLogLevel,
	})

	// 构建提供者链：Default < File < Env < Flag
	chain := NewProviderChainWithFile(fileProv, defaultProv, fileProv, envProv, flagProv)

	return &Providers{
		Chain: chain,
	}
}

// InitConfigWithDefaults 初始化配置文件并写入默认值
func InitConfigWithDefaults(filename string, defaults map[string]any) error {
	return provider.InitConfig(filename, defaults)
}

// GetVersionInfo 返回应用的版本信息
func GetVersionInfo() string {
	return "Version: " + BuildVersion + "\nCommit: " + GitCommit + "\nBuilt at: " + BuildTime
}
