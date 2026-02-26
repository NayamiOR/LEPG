package config

import (
	"github.com/spf13/viper"
)

type Side int

const (
	Server Side = iota
	Client
)

type ClientConfig struct {
	ServerUrl string `mapstructure:"server"`
	Port      int    `mapstructure:"port"`
	LogLevel  string `mapstructure:"log_level"`
}

type ServerConfig struct {
	Port     string `mapstructure:"port"`
	LogLevel string `mapstructure:"log_level"`
}

var defaultClientValues = map[string]any{
	"server":    "http://localhost",
	"port":      8883,
	"log_level": "info",
}

var defaultServerValues = map[string]any{
	"port":      "8883",
	"log_level": "info",
}

func InitConfig(side Side) {
	var value map[string]any
	switch side {
	case Client:
		value = defaultClientValues
	case Server:
		value = defaultServerValues
	}
	for k, v := range value {
		viper.SetDefault(k, v)
	}
}

func CheckConfig(side Side) {
	var value map[string]any
	switch side {
	case Client:
		value = defaultClientValues
	case Server:
		value = defaultServerValues
	}
	for k := range value {
		if !viper.IsSet(k) {
			panic("Config value " + k + " is not set")
		}
	}
}
