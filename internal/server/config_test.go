package server

import (
	"testing"
)

func validServerConfig() *ServerConfig {
	return &ServerConfig{
		Port:          8883,
		LogLevel:      "info",
		LogPath:       "logs/",
		LogMaxSize:    10,
		LogMaxBackups: 3,
		LogMaxAge:     28,
		Pg: PgConfig{
			Host:    "127.0.0.1",
			Port:    5432,
			User:    "lepgs",
			DBName:  "lepgs",
			SSLMode: "disable",
		},
	}
}

func TestServerConfig_Validate_Valid(t *testing.T) {
	cfg := validServerConfig()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestServerConfig_Validate_PortOutOfRange(t *testing.T) {
	tests := []struct {
		port int
		ok   bool
	}{
		{0, false},
		{-1, false},
		{65536, false},
		{1, true},
		{65535, true},
		{8883, true},
	}

	for _, tt := range tests {
		cfg := validServerConfig()
		cfg.Port = tt.port
		err := cfg.Validate()
		if (err == nil) != tt.ok {
			t.Errorf("Port=%d: err=%v, want ok=%v", tt.port, err, tt.ok)
		}
	}
}

func TestServerConfig_Validate_LogLevel(t *testing.T) {
	tests := []struct {
		level string
		ok    bool
	}{
		{"debug", true},
		{"info", true},
		{"warn", true},
		{"error", true},
		{"TRACE", false},
		{"verbose", false},
		{"", false},
	}

	for _, tt := range tests {
		cfg := validServerConfig()
		cfg.LogLevel = tt.level
		err := cfg.Validate()
		if (err == nil) != tt.ok {
			t.Errorf("LogLevel=%q: err=%v, want ok=%v", tt.level, err, tt.ok)
		}
	}
}

func TestServerConfig_Validate_PgHost(t *testing.T) {
	cfg := validServerConfig()
	cfg.Pg.Host = ""
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for empty PgHost")
	}
}

func TestServerConfig_Validate_PgDBName(t *testing.T) {
	cfg := validServerConfig()
	cfg.Pg.DBName = ""
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for empty PgDBName")
	}
}
