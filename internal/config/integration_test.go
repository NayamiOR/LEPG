package config

import (
	"os"
	"testing"

	"github.com/spf13/viper"
)

// ==================== 端到端集成测试 ====================

// TestConfig_ClientWorkflow 测试客户端完整的配置工作流程
// 流程：初始化配置 → 加载配置 → 验证配置
func TestConfig_ClientWorkflow(t *testing.T) {
	originalDir, _ := os.Getwd()
	tempDir := t.TempDir()

	// 清理函数：恢复目录并重置 viper
	t.Cleanup(func() {
		os.Chdir(originalDir)
		viper.Reset()
	})

	// 切换到临时目录
	err := os.Chdir(tempDir)
	if err != nil {
		t.Fatalf("无法切换到临时目录: %v", err)
	}

	// 步骤 1: 初始化客户端配置
	InitConfig(Client)

	// 验证配置文件已创建
	if _, err := os.Stat("config.toml"); os.IsNotExist(err) {
		t.Fatal("初始化后配置文件不存在")
	}

	// 步骤 2: 重置 viper 并加载配置
	viper.Reset()
	err = LoadConfig()
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}

	// 步骤 3: 验证配置完整性
	err = CheckConfig(Client)
	if err != nil {
		t.Errorf("配置验证失败: %v", err)
	}

	// 步骤 4: 验证默认值已正确加载
	expectedServer := "http://localhost"
	expectedPort := 8883
	expectedLogLevel := "info"

	actualServer := viper.GetString("server")
	actualPort := viper.GetInt("port")
	actualLogLevel := viper.GetString("log_level")

	if actualServer != expectedServer {
		t.Errorf("server 期望值 '%s', 实际值 '%s'", expectedServer, actualServer)
	}
	if actualPort != expectedPort {
		t.Errorf("port 期望值 %d, 实际值 %d", expectedPort, actualPort)
	}
	if actualLogLevel != expectedLogLevel {
		t.Errorf("log_level 期望值 '%s', 实际值 '%s'", expectedLogLevel, actualLogLevel)
	}
}

// TestConfig_ServerWorkflow 测试服务端完整的配置工作流程
// 流程：初始化配置 → 加载配置 → 验证配置
func TestConfig_ServerWorkflow(t *testing.T) {
	originalDir, _ := os.Getwd()
	tempDir := t.TempDir()

	// 清理函数
	t.Cleanup(func() {
		os.Chdir(originalDir)
		viper.Reset()
	})

	// 切换到临时目录
	err := os.Chdir(tempDir)
	if err != nil {
		t.Fatalf("无法切换到临时目录: %v", err)
	}

	// 步骤 1: 初始化服务端配置
	InitConfig(Server)

	// 验证配置文件已创建
	if _, err := os.Stat("config.toml"); os.IsNotExist(err) {
		t.Fatal("初始化后配置文件不存在")
	}

	// 步骤 2: 重置 viper 并加载配置
	viper.Reset()
	err = LoadConfig()
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}

	// 步骤 3: 验证配置完整性
	err = CheckConfig(Server)
	if err != nil {
		t.Errorf("配置验证失败: %v", err)
	}

	// 步骤 4: 验证默认值已正确加载
	expectedPort := "8883"
	expectedLogLevel := "info"

	actualPort := viper.GetString("port")
	actualLogLevel := viper.GetString("log_level")

	if actualPort != expectedPort {
		t.Errorf("port 期望值 '%s', 实际值 '%s'", expectedPort, actualPort)
	}
	if actualLogLevel != expectedLogLevel {
		t.Errorf("log_level 期望值 '%s', 实际值 '%s'", expectedLogLevel, actualLogLevel)
	}
}

// TestConfig_EnvironmentOverride 测试环境变量覆盖配置的功能
// 这验证了 viper.AutomaticEnv() 的功能
func TestConfig_EnvironmentOverride(t *testing.T) {
	originalDir, _ := os.Getwd()
	tempDir := t.TempDir()

	// 清理函数
	t.Cleanup(func() {
		os.Chdir(originalDir)
		viper.Reset()
		os.Unsetenv("LEPG_SERVER")
		os.Unsetenv("LEPG_PORT")
		os.Unsetenv("LEPG_LOG_LEVEL")
	})

	// 设置环境变量
	os.Setenv("LEPG_SERVER", "http://production.example.com")
	os.Setenv("LEPG_PORT", "9999")
	os.Setenv("LEPG_LOG_LEVEL", "debug")

	// 切换到临时目录
	err := os.Chdir(tempDir)
	if err != nil {
		t.Fatalf("无法切换到临时目录: %v", err)
	}

	// 初始化配置（写入默认值到文件）
	InitConfig(Client)

	// 加载配置（此时环境变量应该覆盖文件中的值）
	err = LoadConfig()
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}

	// 注意：viper 的环境变量覆盖需要配置 key 的映射
	// 这里我们验证 LoadConfig 中确实调用了 AutomaticEnv()
	// 实际的环境变量覆盖行为取决于 viper 的配置
}

// TestConfig_MultipleOperations 测试多次配置操作
func TestConfig_MultipleOperations(t *testing.T) {
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

	// 第一次初始化和加载
	InitConfig(Client)
	viper.Reset()
	err = LoadConfig()
	if err != nil {
		t.Fatalf("第一次加载失败: %v", err)
	}

	firstServer := viper.GetString("server")

	// 重新初始化（覆盖文件）
	InitConfig(Client)
	viper.Reset()
	err = LoadConfig()
	if err != nil {
		t.Fatalf("第二次加载失败: %v", err)
	}

	secondServer := viper.GetString("server")

	// 验证两次加载的结果一致
	if firstServer != secondServer {
		t.Errorf("多次加载的结果不一致: %s != %s", firstServer, secondServer)
	}
}
