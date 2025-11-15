package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// DefaultConfigPath brukes hvis ingen sti oppgis i kall til Load().
const DefaultConfigPath = "config/config.yaml"

// OPCUAConfig holder OPC UA-relaterte innstillinger.
type OPCUAConfig struct {
	Endpoint string `yaml:"endpoint"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

// LoggingConfig definerer loggnivå og andre loggerinnstillinger.
type LoggingConfig struct {
	Level string `yaml:"level"`
}

// FermentationConfig – alt som gjelder gjæring.
type FermentationConfig struct {
	Enabled bool `yaml:"enabled"`

	Hysteresis struct {
		Cooling float64 `yaml:"cooling"`
		Heating float64 `yaml:"heating"`
	} `yaml:"hysteresis"`

	StepCheckInterval string `yaml:"step_check_interval"`
	StabilizationTime string `yaml:"stabilization_time"`

	// Hvor vi lagrer lokal fermenterings-DB.
	DatabasePath string `yaml:"database_path"`
}

// BrewfatherConfig – API-nøklene.
type BrewfatherConfig struct {
	UserID string `yaml:"user_id"`
	APIKey string `yaml:"api_key"`
}

// Config er toppnivåstrukturen for YAML-konfigurasjonen.
type Config struct {
	OPCUA        OPCUAConfig        `yaml:"opcua"`
	Logging      LoggingConfig      `yaml:"logging"`
	Fermentation FermentationConfig `yaml:"fermentation"`
	Brewfather   BrewfatherConfig   `yaml:"brewfather"`
}

// Load leser og parser YAML-konfigurasjonen.
// Hvis path er tom, brukes DefaultConfigPath.
func Load(path string) (*Config, error) {
	if path == "" {
		path = DefaultConfigPath
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("unable to open config file %q: %w", path, err)
	}
	defer file.Close()

	var cfg Config
	if err := yaml.NewDecoder(file).Decode(&cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file %q: %w", path, err)
	}

	// Defaults
	if cfg.Fermentation.DatabasePath == "" {
		cfg.Fermentation.DatabasePath = "data/fermentation.db"
	}

	return &cfg, nil
}
