package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

// 辅助函数：创建临时目录并切换
func setupTempDir(t *testing.T) (string, string) {
	originalDir, _ := os.Getwd()
	tempDir := t.TempDir()

	t.Cleanup(func() {
		os.Chdir(originalDir)
		viper.Reset()
	})

	err := os.Chdir(tempDir)
	if err != nil {
		t.Fatalf("无法切换到临时目录: %v", err)
	}

	return tempDir, originalDir
}

// 辅助函数：在指定目录创建配置文件
func createConfigFile(t *testing.T, dir, content string) {
	configPath := filepath.Join(dir, "config.toml")
	err := os.WriteFile(configPath, []byte(content), 0644)
	if err != nil {
		t.Fatalf("无法创建配置文件: %v", err)
	}
}

// ==================== InitConfig 测试 ====================

func TestInitConfig_ClientSide(t *testing.T) {
	setupTempDir(t)

	InitConfig(Client)

	// 验证文件存在
	if _, err := os.Stat("config.toml"); os.IsNotExist(err) {
		t.Fatal("配置文件未创建")
	}

	// 读取文件内容验证
	content, err := os.ReadFile("config.toml")
	if err != nil {
		t.Fatalf("无法读取配置文件: %v", err)
	}

	contentStr := string(content)

	// 验证默认值（viper 使用单引号生成 TOML）
	if !strings.Contains(contentStr, "server =") {
		t.Error("客户端配置中缺少 server 默认值")
	}
	if !strings.Contains(contentStr, "port = 8883") {
		t.Error("客户端配置中缺少 port 默认值")
	}
	if !strings.Contains(contentStr, "log_level =") {
		t.Error("客户端配置中缺少 log_level 默认值")
	}
}

func TestInitConfig_ServerSide(t *testing.T) {
	setupTempDir(t)

	InitConfig(Server)

	// 验证文件存在
	if _, err := os.Stat("config.toml"); os.IsNotExist(err) {
		t.Fatal("配置文件未创建")
	}

	// 读取文件内容验证
	content, err := os.ReadFile("config.toml")
	if err != nil {
		t.Fatalf("无法读取配置文件: %v", err)
	}

	contentStr := string(content)

	// 验证默认值（viper 使用单引号生成 TOML，服务端没有 server 字段）
	if !strings.Contains(contentStr, "port =") {
		t.Error("服务端配置中缺少 port 默认值")
	}
	if !strings.Contains(contentStr, "log_level =") {
		t.Error("服务端配置中缺少 log_level 默认值")
	}
}

func TestInitConfig_FileOverwrite(t *testing.T) {
	tempDir, _ := setupTempDir(t)

	// 先创建一个配置文件
	createConfigFile(t, tempDir, "old_content = true")

	// 再次初始化
	InitConfig(Client)

	// 验证文件被覆盖
	content, err := os.ReadFile("config.toml")
	if err != nil {
		t.Fatalf("无法读取配置文件: %v", err)
	}

	contentStr := string(content)
	if strings.Contains(contentStr, "old_content") {
		t.Error("配置文件未被覆盖")
	}
}

// ==================== LoadConfig 测试 ====================

func TestLoadConfig_CurrentDir(t *testing.T) {
	tempDir, _ := setupTempDir(t)

	// 创建配置文件
	clientContent := `server = "http://test.com"
port = 8080
log_level = "debug"
`
	createConfigFile(t, tempDir, clientContent)

	// 加载配置
	err := LoadConfig()
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}

	// 验证配置值
	if viper.GetString("server") != "http://test.com" {
		t.Errorf("期望 server 为 'http://test.com', 实际为 '%s'", viper.GetString("server"))
	}
	if viper.GetInt("port") != 8080 {
		t.Errorf("期望 port 为 8080, 实际为 %d", viper.GetInt("port"))
	}
	if viper.GetString("log_level") != "debug" {
		t.Errorf("期望 log_level 为 'debug', 实际为 '%s'", viper.GetString("log_level"))
	}
}

func TestLoadConfig_ConfigDir(t *testing.T) {
	tempDir, _ := setupTempDir(t)

	// 创建 config 子目录
	configDir := filepath.Join(tempDir, "config")
	err := os.Mkdir(configDir, 0755)
	if err != nil {
		t.Fatalf("无法创建 config 目录: %v", err)
	}

	// 在 config 目录创建配置文件
	serverContent := `port = "9999"
log_level = "warn"
`
	createConfigFile(t, configDir, serverContent)

	// 加载配置
	err = LoadConfig()
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}

	// 验证配置值
	if viper.GetString("port") != "9999" {
		t.Errorf("期望 port 为 '9999', 实际为 '%s'", viper.GetString("port"))
	}
	if viper.GetString("log_level") != "warn" {
		t.Errorf("期望 log_level 为 'warn', 实际为 '%s'", viper.GetString("log_level"))
	}
}

func TestLoadConfig_NotFound(t *testing.T) {
	setupTempDir(t)

	// 不创建配置文件，直接尝试加载
	err := LoadConfig()
	if err == nil {
		t.Fatal("期望返回错误，但没有")
	}

	if !strings.Contains(err.Error(), "Not Found") && !strings.Contains(err.Error(), "no such file") {
		t.Errorf("错误类型不正确: %v", err)
	}
}

// ==================== LoadConfigWithPath 测试 ====================

func TestLoadConfigWithPath_Valid(t *testing.T) {
	tempDir, _ := setupTempDir(t)

	// 创建子目录和配置文件
	subDir := filepath.Join(tempDir, "custom")
	err := os.Mkdir(subDir, 0755)
	if err != nil {
		t.Fatalf("无法创建子目录: %v", err)
	}

	clientContent := `server = "http://custom.com"
port = 7777
log_level = "trace"
`
	createConfigFile(t, subDir, clientContent)

	// 从指定路径加载
	err = LoadConfigWithPath(subDir)
	if err != nil {
		t.Fatalf("从指定路径加载配置失败: %v", err)
	}

	// 验证配置值
	if viper.GetString("server") != "http://custom.com" {
		t.Errorf("期望 server 为 'http://custom.com', 实际为 '%s'", viper.GetString("server"))
	}
	if viper.GetInt("port") != 7777 {
		t.Errorf("期望 port 为 7777, 实际为 %d", viper.GetInt("port"))
	}
}

func TestLoadConfigWithPath_NotFound(t *testing.T) {
	setupTempDir(t)

	// 尝试从不存在的路径加载
	err := LoadConfigWithPath("/nonexistent/path")
	if err == nil {
		t.Fatal("期望返回错误，但没有")
	}
}

// ==================== CheckConfig 测试 ====================

func TestCheckConfig_ClientValid(t *testing.T) {
	setupTempDir(t)

	// 设置完整的客户端配置
	viper.Set("server", "http://localhost")
	viper.Set("port", 8883)
	viper.Set("log_level", "info")

	err := CheckConfig(Client)
	if err != nil {
		t.Errorf("完整配置应该验证通过: %v", err)
	}
}

func TestCheckConfig_ServerValid(t *testing.T) {
	setupTempDir(t)

	// 设置完整的服务端配置
	viper.Set("port", "8883")
	viper.Set("log_level", "info")

	err := CheckConfig(Server)
	if err != nil {
		t.Errorf("完整配置应该验证通过: %v", err)
	}
}

func TestCheckConfig_ClientMissing(t *testing.T) {
	setupTempDir(t)

	// 只设置部分客户端配置
	viper.Set("server", "http://localhost")
	// 缺少 port 和 log_level

	err := CheckConfig(Client)
	if err == nil {
		t.Fatal("缺少配置项应该返回错误")
	}

	configErr, ok := err.(*ConfigNotSetError)
	if !ok {
		t.Fatalf("错误类型应该是 ConfigNotSetError")
	}

	if !strings.Contains(configErr.Msg, "port") {
		t.Errorf("错误信息应该包含 'port': %s", configErr.Msg)
	}
	if !strings.Contains(configErr.Msg, "log_level") {
		t.Errorf("错误信息应该包含 'log_level': %s", configErr.Msg)
	}
}

func TestCheckConfig_ServerMissing(t *testing.T) {
	setupTempDir(t)

	// 只设置部分服务端配置
	viper.Set("log_level", "debug")
	// 缺少 port

	err := CheckConfig(Server)
	if err == nil {
		t.Fatal("缺少配置项应该返回错误")
	}

	configErr, ok := err.(*ConfigNotSetError)
	if !ok {
		t.Fatalf("错误类型应该是 ConfigNotSetError")
	}

	if !strings.Contains(configErr.Msg, "port") {
		t.Errorf("错误信息应该包含 'port': %s", configErr.Msg)
	}
}

func TestCheckConfig_AllMissing(t *testing.T) {
	setupTempDir(t)

	// 不设置任何配置
	err := CheckConfig(Client)
	if err == nil {
		t.Fatal("缺少所有配置项应该返回错误")
	}

	configErr, ok := err.(*ConfigNotSetError)
	if !ok {
		t.Fatalf("错误类型应该是 ConfigNotSetError")
	}

	// 应该包含所有缺失的配置项
	if !strings.Contains(configErr.Msg, "server") {
		t.Errorf("错误信息应该包含 'server': %s", configErr.Msg)
	}
	if !strings.Contains(configErr.Msg, "port") {
		t.Errorf("错误信息应该包含 'port': %s", configErr.Msg)
	}
	if !strings.Contains(configErr.Msg, "log_level") {
		t.Errorf("错误信息应该包含 'log_level': %s", configErr.Msg)
	}
}

// ==================== ConfigNotSetError 测试 ====================

func TestConfigNotSetError_Error(t *testing.T) {
	err := &ConfigNotSetError{
		Code: 1,
		Msg:  "Test error message",
	}

	if err.Error() != "Test error message" {
		t.Errorf("Error() 方法返回值不正确: %s", err.Error())
	}

	if err.Code != 1 {
		t.Errorf("Code 应该为 1, 实际为 %d", err.Code)
	}
}
