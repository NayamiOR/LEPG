package config

import (
	"strings"

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

func LoadConfig() error {
	viper.AutomaticEnv()
	viper.SetConfigName("config")
	viper.SetConfigType("toml")
	viper.AddConfigPath(".")
	viper.AddConfigPath("./config")
	err := viper.ReadInConfig()
	if err != nil {
		return err
	}
	return nil
}

func LoadConfigWithPath(path string) error {
	viper.AutomaticEnv()
	viper.SetConfigName("config")
	viper.SetConfigType("toml")
	viper.AddConfigPath(path)
	err := viper.ReadInConfig()
	if err != nil {
		return err
	}
	return nil
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

	viper.WriteConfigAs("config.toml")
}

type ConfigNotSetError struct {
	Code int
	Msg  string
}

func (e *ConfigNotSetError) Error() string {
	return e.Msg
}

func CheckConfig(side Side) error {
	var value map[string]any
	switch side {
	case Client:
		value = defaultClientValues
	case Server:
		value = defaultServerValues
	}
	var MissingConfigs []string
	for k := range value {
		if !viper.IsSet(k) {
			MissingConfigs = append(MissingConfigs, k)
		}
	}
	if len(MissingConfigs) > 0 {
		return &ConfigNotSetError{
			Code: 1,
			Msg:  "Missing configs: " + strings.Join(MissingConfigs, ", "),
		}
	}
	return nil
}
