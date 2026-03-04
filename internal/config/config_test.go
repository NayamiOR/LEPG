package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

// ==================== 辅助函数 ====================

// setupTempDir 创建临时目录并切换，返回清理函数
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

// createConfigFile 在指定目录创建配置文件
func createConfigFile(t *testing.T, dir, content string) {
	configPath := filepath.Join(dir, "config.toml")
	err := os.WriteFile(configPath, []byte(content), 0644)
	if err != nil {
		t.Fatalf("无法创建配置文件: %v", err)
	}
}

// ==================== 配置测试套件 ====================
// 按照核心业务场景组织的完整测试
//
// 核心场景：
//   1. init覆盖文件 - InitConfig 会覆盖已存在的配置文件
//   2. 读不到任何配置 - 配置文件不存在时的错误处理
//   3. 只能读到配置文件 - 从配置文件读取配置
//   4. flag覆盖配置文件 - flag 覆盖配置文件，未设置的从配置读取
//   5. 配置缺失报错 - 配置项缺失时正确报错

// ==================== 场景1: InitConfig 覆盖文件 ====================

// TestScenario1_InitOverwrite 测试 InitConfig 会覆盖已存在的配置文件
func TestScenario1_InitOverwrite(t *testing.T) {
	tempDir, _ := setupTempDir(t)

	// 先创建一个旧的配置文件
	createConfigFile(t, tempDir, `old_setting = "should_be_removed"
server = "http://old.com"
port = 1111
`)

	// 执行 InitConfig，应该覆盖旧文件
	InitConfig(Client)

	// 验证文件被覆盖
	content, _ := os.ReadFile("config.toml")
	contentStr := string(content)

	// 旧内容应该不存在
	if strings.Contains(contentStr, "old_setting") {
		t.Error("旧配置文件未被覆盖，仍包含 old_setting")
	}
	if strings.Contains(contentStr, "http://old.com") {
		t.Error("旧配置文件未被覆盖，仍包含旧 server")
	}

	// 新的默认值应该存在
	if !strings.Contains(contentStr, "server =") {
		t.Error("新配置文件缺少 server 字段")
	}
	if !strings.Contains(contentStr, "port = 8883") {
		t.Error("新配置文件缺少默认 port")
	}
}

// ==================== 场景2: 读不到任何配置 ====================

// TestScenario2_NoConfig 测试配置文件不存在时的错误处理
func TestScenario2_NoConfig(t *testing.T) {
	setupTempDir(t)

	// 不创建配置文件，直接尝试加载
	err := LoadConfig()
	if err == nil {
		t.Fatal("配置文件不存在时应该返回错误")
	}

	// 验证无法通过配置检查
	checkErr := CheckConfig(Client)
	if checkErr == nil {
		t.Fatal("没有配置时应该验证失败")
	}

	// 验证错误信息包含所有缺失的配置项
	configErr, ok := checkErr.(*ConfigNotSetError)
	if !ok {
		t.Fatalf("错误类型应该是 ConfigNotSetError")
	}

	expectedMissing := []string{"server", "port", "log_level"}
	for _, expected := range expectedMissing {
		if !strings.Contains(configErr.Msg, expected) {
			t.Errorf("错误信息应该包含 '%s', 实际: %s", expected, configErr.Msg)
		}
	}
}

// ==================== 场景3: 只能读到配置文件 ====================

// TestScenario3_OnlyConfigFile 测试从配置文件读取配置
func TestScenario3_OnlyConfigFile(t *testing.T) {
	tests := []struct {
		name         string
		configContent string
		setupFunc     func(string) string
		verifyFunc   func()
	}{
		{
			name: "从当前目录读取",
			configContent: `server = "http://config.com"
port = 8080
log_level = "debug"
`,
			setupFunc: func(tempDir string) string {
				createConfigFile(t, tempDir, `server = "http://config.com"
port = 8080
log_level = "debug"
`)
				return ""
			},
			verifyFunc: func() {
				// 验证所有值都来自配置文件
				if viper.GetString("server") != "http://config.com" {
					t.Errorf("server 应该是 'http://config.com', 实际是 '%s'", viper.GetString("server"))
				}
				if viper.GetInt("port") != 8080 {
					t.Errorf("port 应该是 8080, 实际是 %d", viper.GetInt("port"))
				}
				if viper.GetString("log_level") != "debug" {
					t.Errorf("log_level 应该是 'debug', 实际是 '%s'", viper.GetString("log_level"))
				}

				// 验证配置检查通过
				if err := CheckConfig(Client); err != nil {
					t.Errorf("完整配置应该验证通过: %v", err)
				}
			},
		},
		{
			name: "从config子目录读取",
			setupFunc: func(tempDir string) string {
				configDir := filepath.Join(tempDir, "config")
				os.Mkdir(configDir, 0755)
				createConfigFile(t, configDir, `server = "http://subdir.com"
port = 9999
log_level = "warn"
`)
				return ""
			},
			verifyFunc: func() {
				if viper.GetString("server") != "http://subdir.com" {
					t.Errorf("应该从子目录读取配置")
				}
			},
		},
		{
			name: "从指定路径读取",
			setupFunc: func(tempDir string) string {
				customDir := filepath.Join(tempDir, "custom")
				os.Mkdir(customDir, 0755)
				createConfigFile(t, customDir, `server = "http://custom.com"
port = 7777
log_level = "trace"
`)
				return customDir
			},
			verifyFunc: func() {
				if viper.GetString("server") != "http://custom.com" {
					t.Errorf("应该从指定路径读取配置")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir, _ := setupTempDir(t)
			customPath := tt.setupFunc(tempDir)

			var err error
			if customPath != "" {
				err = LoadConfigWithPath(customPath)
			} else {
				err = LoadConfig()
			}

			if err != nil {
				t.Fatalf("加载配置失败: %v", err)
			}

			if tt.verifyFunc != nil {
				tt.verifyFunc()
			}
		})
	}
}

// ==================== 场景4: Flag 覆盖配置文件 ====================

// TestScenario4_FlagOverride 测试 flag 覆盖配置文件，未设置的从配置读取
func TestScenario4_FlagOverride(t *testing.T) {
	tests := []struct {
		name          string
		configContent string
		flagServer    string
		flagPort      int
		manualSet     map[string]string
		expectServer  string
		expectPort    int
		expectLog     string
	}{
		{
			name: "只有flag（没有配置文件）",
			flagServer: "http://flag-only.com",
			flagPort:   9999,
			manualSet:  map[string]string{"log_level": "debug"},
			expectServer: "http://flag-only.com",
			expectPort:   9999,
			expectLog:    "debug",
		},
		{
			name: "flag覆盖配置文件中的所有字段",
			configContent: `server = "http://config.com"
port = 8080
log_level = "info"
`,
			flagServer: "http://flag.com",
			flagPort:   9999,
			manualSet:  map[string]string{"log_level": "debug"},
			expectServer: "http://flag.com",
			expectPort:   9999,
			expectLog:    "debug",
		},
		{
			name: "flag只覆盖server，port和log_level从配置读取",
			configContent: `server = "http://config.com"
port = 8080
log_level = "warn"
`,
			flagServer: "http://flag.com",
			// flagPort = 0 表示不设置
			expectServer: "http://flag.com",
			expectPort:   8080, // 来自配置文件
			expectLog:    "warn", // 来自配置文件
		},
		{
			name: "flag只覆盖port，server和log_level从配置读取",
			configContent: `server = "http://config.com"
port = 8080
log_level = "info"
`,
			flagPort:   9999,
			expectServer: "http://config.com", // 来自配置文件
			expectPort:   9999, // 来自 flag
			expectLog:    "info", // 来自配置文件
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir, _ := setupTempDir(t)

			// 创建配置文件（如果有）
			if tt.configContent != "" {
				createConfigFile(t, tempDir, tt.configContent)
				if err := LoadConfig(); err != nil {
					t.Fatalf("加载配置文件失败: %v", err)
				}
			}

			// 设置 flag 值
			SetFlagValues(tt.flagServer, tt.flagPort)
			for k, v := range tt.manualSet {
				viper.Set(k, v)
			}

			// 验证 server
			if viper.GetString("server") != tt.expectServer {
				t.Errorf("server: 期望 '%s', 实际 '%s'", tt.expectServer, viper.GetString("server"))
			}

			// 验证 port
			if viper.GetInt("port") != tt.expectPort {
				t.Errorf("port: 期望 %d, 实际 %d", tt.expectPort, viper.GetInt("port"))
			}

			// 验证 log_level
			if viper.GetString("log_level") != tt.expectLog {
				t.Errorf("log_level: 期望 '%s', 实际 '%s'", tt.expectLog, viper.GetString("log_level"))
			}

			// 验证配置检查通过
			if err := CheckConfig(Client); err != nil {
				t.Errorf("配置应该完整: %v", err)
			}
		})
	}
}

// ==================== 场景5: 配置缺失报错 ====================

// TestScenario5_MissingConfigError 测试配置项缺失时正确报错
func TestScenario5_MissingConfigError(t *testing.T) {
	tests := []struct {
		name          string
		configType    Side
		setupFunc     func()
		expectError   bool
		expectedInMsg []string
	}{
		{
			name:       "客户端完整配置-应该通过",
			configType: Client,
			setupFunc: func() {
				viper.Set("server", "http://localhost")
				viper.Set("port", 8883)
				viper.Set("log_level", "info")
			},
			expectError: false,
		},
		{
			name:       "服务端完整配置-应该通过",
			configType: Server,
			setupFunc: func() {
				viper.Set("port", "8883")
				viper.Set("log_level", "info")
			},
			expectError: false,
		},
		{
			name:       "客户端缺少server和port-应该报错",
			configType: Client,
			setupFunc: func() {
				viper.Set("log_level", "debug")
				// 缺少 server 和 port
			},
			expectError:   true,
			expectedInMsg: []string{"server", "port"},
		},
		{
			name:       "客户端缺少port和log_level-应该报错",
			configType: Client,
			setupFunc: func() {
				viper.Set("server", "http://localhost")
				// 缺少 port 和 log_level
			},
			expectError:   true,
			expectedInMsg: []string{"port", "log_level"},
		},
		{
			name:       "服务端缺少port-应该报错",
			configType: Server,
			setupFunc: func() {
				viper.Set("log_level", "debug")
				// 缺少 port
			},
			expectError:   true,
			expectedInMsg: []string{"port"},
		},
		{
			name:       "全部配置缺失-应该报错",
			configType: Client,
			setupFunc: func() {
				// 什么都没设置
			},
			expectError:   true,
			expectedInMsg: []string{"server", "port", "log_level"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupTempDir(t)

			if tt.setupFunc != nil {
				tt.setupFunc()
			}

			err := CheckConfig(tt.configType)

			if tt.expectError {
				if err == nil {
					t.Fatal("期望返回错误，但没有")
				}

				// 验证错误类型
				configErr, ok := err.(*ConfigNotSetError)
				if !ok {
					t.Fatalf("错误类型应该是 ConfigNotSetError，实际是 %T", err)
				}

				// 验证错误信息包含所有缺失的配置项
				for _, expected := range tt.expectedInMsg {
					if !strings.Contains(configErr.Msg, expected) {
						t.Errorf("错误信息应该包含 '%s'，实际: %s", expected, configErr.Msg)
					}
				}
			} else {
				if err != nil {
					t.Errorf("完整配置应该通过验证，但返回错误: %v", err)
				}
			}
		})
	}
}
