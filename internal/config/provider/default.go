package provider

import "sync"

// DefaultProvider 提供默认值，位于 provider 链的最末端
type DefaultProvider struct {
	mu      sync.RWMutex
	defaults map[string]any
}

func NewDefaultProvider(defaults map[string]any) *DefaultProvider {
	return &DefaultProvider{defaults: defaults}
}

func (p *DefaultProvider) GetString(key string) string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if v, ok := p.defaults[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func (p *DefaultProvider) GetInt(key string) int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if v, ok := p.defaults[key]; ok {
		switch val := v.(type) {
		case int:
			return val
		case float64: // JSON 数字解析为 float64
			return int(val)
		}
	}
	return 0
}

func (p *DefaultProvider) GetBool(key string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if v, ok := p.defaults[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

func (p *DefaultProvider) GetStringSlice(key string) []string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if v, ok := p.defaults[key]; ok {
		if s, ok := v.([]string); ok {
			return s
		}
	}
	return nil
}

func (p *DefaultProvider) Get(key string) any {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.defaults[key]
}

func (p *DefaultProvider) IsSet(key string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	_, ok := p.defaults[key]
	return ok
}
