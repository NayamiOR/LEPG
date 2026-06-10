package config

import (
	"LEPG/internal/config/provider"
	"testing"
)

// ========== Test structs ==========

type flatConfig struct {
	Name    string `config:"name"    default:"hello" sources:"file,flag,env,default"`
	Count   int    `config:"count"   default:"42"   sources:"file,env,default"`
	Enabled bool   `config:"enabled" default:"true" sources:"file,env,default"`
}

type nestedConfig struct {
	Port int `config:"port" default:"8080" sources:"file,flag,env,default"`
	Sub  subConfig
}

type subConfig struct {
	Host string `config:"host" default:"localhost" sources:"file,env,default"`
}

type noSourcesConfig struct {
	Name  string `config:"name"  default:"hello"`
	Items []string
}

type sensitiveConfig struct {
	ApiKey   string `config:"api_key"   default:"default-key" sources:"file,env,default"`
	ApiToken string `config:"api_token"                       sources:"file,env"`
}

// ========== PopulateFromProvider tests ==========

func TestPopulateFromProvider_FlatFields(t *testing.T) {
	flagProv := provider.NewFlagProvider()
	flagProv.Set("name", "from-flag")

	chain := NewProviderChainWithFile(
		nil,
		provider.NewDefaultProvider(map[string]any{
			"name":    "from-default",
			"count":   99,
			"enabled": false,
		}),
		provider.NewEnvProvider(""),
		flagProv,
	)

	cfg := &flatConfig{}
	if err := PopulateFromProvider(cfg, chain); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Name != "from-flag" {
		t.Errorf("Name = %q, want %q (flag overrides default)", cfg.Name, "from-flag")
	}
	// Count and Enabled come from DefaultProvider (which has them set)
	if cfg.Count != 99 {
		t.Errorf("Count = %d, want %d (from DefaultProvider)", cfg.Count, 99)
	}
	if cfg.Enabled != false {
		t.Errorf("Enabled = %v, want %v (from DefaultProvider)", cfg.Enabled, false)
	}
}

func TestPopulateFromProvider_DefaultFallback(t *testing.T) {
	chain := NewProviderChainWithFile(
		nil,
		provider.NewDefaultProvider(map[string]any{}),
	)

	cfg := &flatConfig{}
	if err := PopulateFromProvider(cfg, chain); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Name != "hello" {
		t.Errorf("Name = %q, want %q (from default tag)", cfg.Name, "hello")
	}
	if cfg.Count != 42 {
		t.Errorf("Count = %d, want %d (from default tag)", cfg.Count, 42)
	}
}

func TestPopulateFromProvider_NestedStruct(t *testing.T) {
	chain := NewProviderChainWithFile(
		nil,
		provider.NewDefaultProvider(map[string]any{
			"port": 9999,
			"host": "example.com",
		}),
	)

	cfg := &nestedConfig{}
	if err := PopulateFromProvider(cfg, chain); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Port != 9999 {
		t.Errorf("Port = %d, want %d", cfg.Port, 9999)
	}
	if cfg.Sub.Host != "example.com" {
		t.Errorf("Sub.Host = %q, want %q", cfg.Sub.Host, "example.com")
	}
}

func TestPopulateFromProvider_NoSourcesTag(t *testing.T) {
	chain := NewProviderChainWithFile(
		nil,
		provider.NewDefaultProvider(map[string]any{
			"name": "should-not-appear",
		}),
	)

	cfg := &noSourcesConfig{}
	if err := PopulateFromProvider(cfg, chain); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Name != "" {
		t.Errorf("Name = %q, want empty (no sources tag means no provider reads)", cfg.Name)
	}
}

func TestPopulateFromProvider_SourceFiltering(t *testing.T) {
	flagProv := provider.NewFlagProvider()
	flagProv.Set("api_token", "from-flag-should-be-ignored")

	chain := NewProviderChainWithFile(
		nil,
		provider.NewDefaultProvider(map[string]any{
			"api_key":   "from-default",
			"api_token": "from-default-should-be-ignored", // default not in sources for api_token
		}),
		provider.NewEnvProvider(""),
		flagProv,
	)

	cfg := &sensitiveConfig{}
	if err := PopulateFromProvider(cfg, chain); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.ApiKey != "from-default" {
		t.Errorf("ApiKey = %q, want %q", cfg.ApiKey, "from-default")
	}
	// ApiToken has sources:"file,env" — flag is excluded, default is excluded
	if cfg.ApiToken != "" {
		t.Errorf("ApiToken = %q, want empty (flag excluded, default excluded, no file/env value)", cfg.ApiToken)
	}
}

func TestPopulateFromProvider_InvalidInput(t *testing.T) {
	// Non-pointer
	err := PopulateFromProvider(flatConfig{}, &ProviderChain{})
	if err == nil {
		t.Error("expected error for non-pointer input")
	}

	// Non-ProviderChain
	err = PopulateFromProvider(&flatConfig{}, provider.NewDefaultProvider(map[string]any{}))
	if err == nil {
		t.Error("expected error for non-ProviderChain provider")
	}
}

// ========== ExtractDefaults tests ==========

func TestExtractDefaults_FlatFields(t *testing.T) {
	defaults := ExtractDefaults(&flatConfig{})

	if defaults["name"] != "hello" {
		t.Errorf("name = %v, want %q", defaults["name"], "hello")
	}
	if defaults["count"] != 42 {
		t.Errorf("count = %v, want %d", defaults["count"], 42)
	}
	if defaults["enabled"] != true {
		t.Errorf("enabled = %v, want %v", defaults["enabled"], true)
	}
}

func TestExtractDefaults_NestedStruct(t *testing.T) {
	defaults := ExtractDefaults(&nestedConfig{})

	if defaults["port"] != 8080 {
		t.Errorf("port = %v, want %d", defaults["port"], 8080)
	}
	if defaults["host"] != "localhost" {
		t.Errorf("host = %v, want %q", defaults["host"], "localhost")
	}
}

func TestExtractDefaults_SkipsNoDefaultFields(t *testing.T) {
	defaults := ExtractDefaults(&sensitiveConfig{})

	if _, ok := defaults["api_token"]; ok {
		t.Error("api_token should not be in defaults (no default tag)")
	}
	if defaults["api_key"] != "default-key" {
		t.Errorf("api_key = %v, want %q", defaults["api_key"], "default-key")
	}
}

func TestExtractDefaults_IncludesFieldsWithoutSources(t *testing.T) {
	defaults := ExtractDefaults(&noSourcesConfig{})

	// ExtractDefaults doesn't look at sources tag, only config+default
	if defaults["name"] != "hello" {
		t.Errorf("name = %v, want %q (ExtractDefaults ignores sources tag)", defaults["name"], "hello")
	}
}

func TestExtractDefaults_MultipleStructs(t *testing.T) {
	defaults := ExtractDefaults(&flatConfig{}, &nestedConfig{})

	if defaults["name"] != "hello" {
		t.Errorf("name from flatConfig = %v, want %q", defaults["name"], "hello")
	}
	if defaults["host"] != "localhost" {
		t.Errorf("host from nestedConfig = %v, want %q", defaults["host"], "localhost")
	}
}

// ========== parseSources tests ==========

func TestParseSources(t *testing.T) {
	tests := []struct {
		tag  string
		want Source
	}{
		{"file,env", SourceFile | SourceEnv},
		{"file,flag,env,default", SourceFile | SourceFlag | SourceEnv | SourceDefault},
		{"default", SourceDefault},
		{"", 0},
	}

	for _, tt := range tests {
		got := parseSources(tt.tag)
		if got != tt.want {
			t.Errorf("parseSources(%q) = %v, want %v", tt.tag, got, tt.want)
		}
	}
}
