package config

import (
	"errors"
	"fmt"

	"github.com/go-playground/validator/v10"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/confmap"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

type User struct {
	Token              string `koanf:"token" validate:"required"`
	ReadeckAccessToken string `koanf:"readeck_access_token" validate:"required"`
}

type ConfigReadeck struct {
	Host string `koanf:"host" validate:"required,url"`
}

type Config struct {
	Readeck  ConfigReadeck `koanf:"readeck"`
	Server   struct {
		Port int `koanf:"port" validate:"min=1,max=65535"`
	} `koanf:"server"`
	Users    []User        `koanf:"users" validate:"required,min=1,dive"`
	LogLevel string        `koanf:"log_level" validate:"oneof=error warn info debug"`
}

func (c *Config) Validate() error {
	validate := validator.New()
	err := validate.Struct(c)
	if err == nil {
		return nil
	}

	var validationErrors validator.ValidationErrors
	if errors.As(err, &validationErrors) {
		return fmt.Errorf("configuration validation failed: %v", validationErrors)
	}

	return err
}

func Load(path string) (*Config, error) {
	k := koanf.New(".")
	parser := yaml.Parser()

	if err := setDefaultValues(k); err != nil {
		return nil, err
	}

	if err := k.Load(file.Provider(path), parser); err != nil {
		return nil, err
	}

	cfg := &Config{}
	if err := k.Unmarshal("", &cfg); err != nil {
		return nil, err
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func setDefaultValues(k *koanf.Koanf) error {
	return k.Load(confmap.Provider(map[string]any{
		"server.port": 8080,
		"log_level":   "info",
	}, "."), nil)
}
