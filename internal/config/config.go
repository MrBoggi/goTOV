package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type OPCUAConfig struct {
	Endpoint string `yaml:"endpoint"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

type LoggingConfig struct {
	Level string `yaml:"level"`
}

type Config struct {
	OPCUA   OPCUAConfig   `yaml:"opcua"`
	Logging LoggingConfig `yaml:"logging"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
