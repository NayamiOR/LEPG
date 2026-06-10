package client

import (
	"testing"
)

func validClientConfig() *ClientConfig {
	return &ClientConfig{
		ServerUrl:       "127.0.0.1",
		Port:            8883,
		LogLevel:        "info",
		Sn:              "DEVICE001",
		Token:           "secret",
		MaxRetry:        10,
		RetryInterval:   5000,
		BufferSize:      1000,
		UploadBatchSize: 100,
		UploadInterval:  5000,
		Paths: PathsConfig{
			LogPath:       "logs/",
			DataPath:      "./data/data.db",
			LogMaxSize:    10,
			LogMaxBackups: 3,
			LogMaxAge:     28,
		},
	}
}

func TestClientConfig_Validate_Valid(t *testing.T) {
	cfg := validClientConfig()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestClientConfig_Validate_RequiredFields(t *testing.T) {
	tests := []struct {
		name string
		mod  func(*ClientConfig)
	}{
		{"empty sn", func(c *ClientConfig) { c.Sn = "" }},
		{"empty token", func(c *ClientConfig) { c.Token = "" }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validClientConfig()
			tt.mod(cfg)
			if err := cfg.Validate(); err == nil {
				t.Error("expected error")
			}
		})
	}
}

func TestClientConfig_Validate_Port(t *testing.T) {
	tests := []struct {
		port int
		ok   bool
	}{
		{0, false},
		{-1, false},
		{65536, false},
		{1, true},
		{8883, true},
	}

	for _, tt := range tests {
		cfg := validClientConfig()
		cfg.Port = tt.port
		err := cfg.Validate()
		if (err == nil) != tt.ok {
			t.Errorf("Port=%d: err=%v, want ok=%v", tt.port, err, tt.ok)
		}
	}
}

func TestClientConfig_Validate_LogLevel(t *testing.T) {
	cfg := validClientConfig()
	cfg.LogLevel = "verbose"
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for invalid LogLevel")
	}
}

func TestClientConfig_Validate_NumericRanges(t *testing.T) {
	tests := []struct {
		name string
		mod  func(*ClientConfig)
		ok   bool
	}{
		{"negative MaxRetry", func(c *ClientConfig) { c.MaxRetry = -1 }, false},
		{"negative RetryInterval", func(c *ClientConfig) { c.RetryInterval = -1 }, false},
		{"zero BufferSize", func(c *ClientConfig) { c.BufferSize = 0 }, false},
		{"zero UploadBatchSize", func(c *ClientConfig) { c.UploadBatchSize = 0 }, false},
		{"zero UploadInterval", func(c *ClientConfig) { c.UploadInterval = 0 }, false},
		{"zero MaxRetry (valid)", func(c *ClientConfig) { c.MaxRetry = 0 }, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validClientConfig()
			tt.mod(cfg)
			err := cfg.Validate()
			if (err == nil) != tt.ok {
				t.Errorf("err=%v, want ok=%v", err, tt.ok)
			}
		})
	}
}

func TestClientConfig_Validate_DataPath(t *testing.T) {
	cfg := validClientConfig()
	cfg.Paths.DataPath = ""
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for empty DataPath")
	}
}
