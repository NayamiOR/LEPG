package provider

import "sync"

// FlagProvider 命令行参数提供者
type FlagProvider struct {
	mu   sync.RWMutex
	data map[string]any
}

func NewFlagProvider() *FlagProvider {
	return &FlagProvider{
		data: make(map[string]any),
	}
}

// Set 设置命令行参数值
func (p *FlagProvider) Set(key string, value any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.data[key] = value
}

func (p *FlagProvider) GetString(key string) string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if v, ok := p.data[key].(string); ok {
		return v
	}
	return ""
}

func (p *FlagProvider) GetInt(key string) int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if v, ok := p.data[key].(int); ok {
		return v
	}
	return 0
}

func (p *FlagProvider) GetBool(key string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if v, ok := p.data[key].(bool); ok {
		return v
	}
	return false
}

func (p *FlagProvider) GetStringSlice(key string) []string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if v, ok := p.data[key].([]string); ok {
		return v
	}
	return nil
}

func (p *FlagProvider) Get(key string) any {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.data[key]
}

func (p *FlagProvider) IsSet(key string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	_, ok := p.data[key]
	return ok
}

func (p *FlagProvider) Unmarshal(_ any) error {
	// Flag provider 通常只用于单个值，不支持反序列化整个结构
	return nil
}
