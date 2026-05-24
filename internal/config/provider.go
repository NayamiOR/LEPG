package config

import "LEPG/internal/config/provider"

// IProvider 配置提供者接口
type IProvider interface {
	// GetString 获取字符串值
	GetString(key string) string
	// GetInt 获取整数值
	GetInt(key string) int
	// GetBool 获取布尔值
	GetBool(key string) bool
	// GetStringSlice 获取字符串数组
	GetStringSlice(key string) []string
	// Get 获取任意类型值
	Get(key string) any
	// IsSet 检查键是否设置
	IsSet(key string) bool
}

// IUnmarshaler 可选的结构化解析接口
// 通过类型断言获取，只有 FileProvider 实现
type IUnmarshaler interface {
	Unmarshal(rawVal any) error
}

// ProviderChain 多提供者链，按优先级顺序查找
// 优先级：传入顺序 Default < Env < File < Flag（Flag 最高）
type ProviderChain struct {
	providers []IProvider
	fileProvider *provider.FileProvider
}

func NewProviderChain(providers ...IProvider) *ProviderChain {
	return &ProviderChain{providers: providers}
}

// NewProviderChainWithFile 创建包含 FileProvider 引用的链
func NewProviderChainWithFile(fileProv *provider.FileProvider, providers ...IProvider) *ProviderChain {
	return &ProviderChain{
		providers:    providers,
		fileProvider: fileProv,
	}
}

// Unmarshal 实现结构化解析，委托给 FileProvider
func (c *ProviderChain) Unmarshal(rawVal any) error {
	if c.fileProvider != nil {
		return c.fileProvider.Unmarshal(rawVal)
	}
	return &UnmarshalNotSupportedError{}
}

// UnmarshalNotSupportedError 表示 unmarshal 操作不支持
type UnmarshalNotSupportedError struct{}

func (e *UnmarshalNotSupportedError) Error() string {
	return "unmarshal not supported"
}

// GetString 按优先级获取字符串值
func (c *ProviderChain) GetString(key string) string {
	// 从后往前找（优先级从高到低）
	for i := len(c.providers) - 1; i >= 0; i-- {
		if v := c.providers[i].GetString(key); v != "" {
			return v
		}
	}
	return ""
}

// GetInt 按优先级获取整数值
func (c *ProviderChain) GetInt(key string) int {
	// 从后往前找（优先级从高到低）
	for i := len(c.providers) - 1; i >= 0; i-- {
		if c.providers[i].IsSet(key) {
			return c.providers[i].GetInt(key)
		}
	}
	return 0
}

// GetBool 按优先级获取布尔值
func (c *ProviderChain) GetBool(key string) bool {
	// 从后往前找（优先级从高到低）
	for i := len(c.providers) - 1; i >= 0; i-- {
		if c.providers[i].IsSet(key) {
			return c.providers[i].GetBool(key)
		}
	}
	return false
}

// GetStringSlice 按优先级获取字符串数组
func (c *ProviderChain) GetStringSlice(key string) []string {
	// 从后往前找（优先级从高到低）
	for i := len(c.providers) - 1; i >= 0; i-- {
		if v := c.providers[i].GetStringSlice(key); v != nil {
			return v
		}
	}
	return nil
}

// Get 按优先级获取任意类型值
func (c *ProviderChain) Get(key string) any {
	// 从后往前找（优先级从高到低）
	for i := len(c.providers) - 1; i >= 0; i-- {
		if v := c.providers[i].Get(key); v != nil {
			return v
		}
	}
	return nil
}

// IsSet 检查任意 provider 是否设置了该键
func (c *ProviderChain) IsSet(key string) bool {
	for _, p := range c.providers {
		if p.IsSet(key) {
			return true
		}
	}
	return false
}
