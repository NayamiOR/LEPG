package provider

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/spf13/viper"
)

// FileProvider 配置文件提供者
type FileProvider struct {
	mu       sync.RWMutex
	v        *viper.Viper
	loaded   bool
	fileType string
}

func NewFileProvider(fileType string) *FileProvider {
	return &FileProvider{
		v:        viper.New(),
		fileType: fileType,
	}
}

// SetConfigName 设置配置文件名（不含扩展名）
func (p *FileProvider) SetConfigName(name string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.v.SetConfigName(name)
}

// SetConfigFile 设置完整配置文件路径
func (p *FileProvider) SetConfigFile(path string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.v.SetConfigFile(path)
}

// SetConfigType 设置配置文件类型
func (p *FileProvider) SetConfigType(typ string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.v.SetConfigType(typ)
}

// AddConfigPath 添加配置文件搜索路径
func (p *FileProvider) AddConfigPath(path string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.v.AddConfigPath(path)
}

// SetDefault 设置默认值
func (p *FileProvider) SetDefault(key string, value any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.v.SetDefault(key, value)
}

// Load 加载配置文件
func (p *FileProvider) Load() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.fileType != "" {
		p.v.SetConfigType(p.fileType)
	}

	err := p.v.ReadInConfig()
	p.loaded = err == nil
	return err
}

// LoadDotEnv 加载 .env 文件（可选）
func (p *FileProvider) LoadDotEnv() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// 检查 .env 文件是否存在
	if _, err := os.Stat(".env"); os.IsNotExist(err) {
		return nil
	}

	// 使用独立的 viper 实例加载 .env
	vEnv := viper.New()
	vEnv.SetConfigFile(".env")
	vEnv.SetConfigType("env")

	err := vEnv.MergeInConfig()

	// 如果加载成功，合并到主 viper
	if err == nil {
		for _, key := range vEnv.AllKeys() {
			p.v.Set(key, vEnv.Get(key))
		}
	}

	return err
}

func (p *FileProvider) GetString(key string) string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.v.GetString(key)
}

func (p *FileProvider) GetInt(key string) int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.v.GetInt(key)
}

func (p *FileProvider) GetBool(key string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.v.GetBool(key)
}

func (p *FileProvider) GetStringSlice(key string) []string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.v.GetStringSlice(key)
}

func (p *FileProvider) Get(key string) any {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.v.Get(key)
}

func (p *FileProvider) IsSet(key string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.v.IsSet(key)
}

func (p *FileProvider) Unmarshal(rawVal any) error {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.v.Unmarshal(rawVal)
}

// WriteConfig 写入配置文件
func (p *FileProvider) WriteConfig() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.v.WriteConfig()
}

// WriteConfigAs 写入到指定路径
func (p *FileProvider) WriteConfigAs(filename string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.v.WriteConfigAs(filename)
}

// InitConfig 初始化配置文件
func InitConfig(filename string, defaults map[string]any) error {
	v := viper.New()
	v.SetConfigFile(filename)

	// 设置扩展名
	ext := filepath.Ext(filename)
	if ext != "" {
		v.SetConfigType(ext[1:])
	}

	// 设置默认值
	for k, val := range defaults {
		v.SetDefault(k, val)
	}

	// 确保目录存在
	dir := filepath.Dir(filename)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create config directory: %w", err)
		}
	}

	return v.SafeWriteConfigAs(filename)
}
