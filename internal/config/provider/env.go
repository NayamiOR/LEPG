package provider

import (
	"os"
	"strconv"
	"strings"
	"sync"
)

// EnvProvider 环境变量提供者
type EnvProvider struct {
	mu      sync.RWMutex
	prefix  string
	loaded  bool
	data    map[string]string
	mapping map[string]string // config key -> env key mapping
}

func NewEnvProvider(prefix string) *EnvProvider {
	return &EnvProvider{
		prefix:  prefix,
		data:    make(map[string]string),
		mapping: make(map[string]string),
	}
}

// SetMapping 设置配置键到环境变量名的映射
func (p *EnvProvider) SetMapping(configKey, envKey string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.mapping[configKey] = envKey
}

// Load 加载环境变量
func (p *EnvProvider) Load() {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, env := range os.Environ() {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) == 2 {
			key := parts[0]
			if p.prefix == "" || strings.HasPrefix(key, p.prefix) {
				configKey := strings.TrimPrefix(key, p.prefix)
				configKey = strings.ToLower(configKey)
				p.data[configKey] = parts[1]
			}
		}
	}
	p.loaded = true
}

func (p *EnvProvider) ensureLoaded() {
	if !p.loaded {
		p.Load()
	}
}

func (p *EnvProvider) getEnvKey(key string) string {
	if envKey, ok := p.mapping[key]; ok {
		return envKey
	}
	if p.prefix != "" {
		return p.prefix + strings.ToUpper(key)
	}
	return strings.ToUpper(key)
}

func (p *EnvProvider) GetString(key string) string {
	p.ensureLoaded()
	p.mu.RLock()
	defer p.mu.RUnlock()

	// 先查 mapping
	if envKey := p.getEnvKey(key); envKey != "" {
		if v := os.Getenv(envKey); v != "" {
			return v
		}
	}
	return p.data[key]
}

func (p *EnvProvider) GetInt(key string) int {
	s := p.GetString(key)
	if s == "" {
		return 0
	}
	i, _ := strconv.Atoi(s)
	return i
}

func (p *EnvProvider) GetBool(key string) bool {
	s := p.GetString(key)
	if s == "" {
		return false
	}
	b, _ := strconv.ParseBool(s)
	return b
}

func (p *EnvProvider) GetStringSlice(key string) []string {
	s := p.GetString(key)
	if s == "" {
		return nil
	}
	return strings.Split(s, ",")
}

func (p *EnvProvider) Get(key string) any {
	return p.GetString(key)
}

func (p *EnvProvider) IsSet(key string) bool {
	p.ensureLoaded()
	p.mu.RLock()
	defer p.mu.RUnlock()

	if envKey := p.getEnvKey(key); envKey != "" {
		return os.Getenv(envKey) != ""
	}
	_, ok := p.data[key]
	return ok
}

func (p *EnvProvider) Unmarshal(_ any) error {
	// Env provider 不支持直接反序列化
	return nil
}
