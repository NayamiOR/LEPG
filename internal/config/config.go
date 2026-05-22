package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/viper"
)

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
	return viper.ReadInConfig()
}

func LoadConfigWithPath(path string) error {
	// 尝试加载 .env 文件（如果存在）
	loadDotEnv()

	viper.AutomaticEnv()
	viper.SetConfigName("config")
	viper.SetConfigType("toml")
	viper.AddConfigPath(path)
	return viper.ReadInConfig()
}

// InitConfig 初始化配置文件，设置默认值并写入 config.toml
func InitConfig(defaultValues map[string]any) {
	for k, v := range defaultValues {
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

// CheckConfig 检查配置是否完整
func CheckConfig(requiredKeys []string) error {
	var missingConfigs []string
	for _, key := range requiredKeys {
		if !viper.IsSet(key) {
			missingConfigs = append(missingConfigs, key)
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
