package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

// OPCUAConfig holder OPC UA-relaterte innstillinger.
type OPCUAConfig struct {
	Endpoint string `mapstructure:"endpoint"`
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
}

// LoggingConfig definerer loggnivå og andre loggerinnstillinger.
type LoggingConfig struct {
	Level string `mapstructure:"level"`
}

// FermentationConfig – alt som gjelder gjæring.
type FermentationConfig struct {
	Enabled bool `mapstructure:"enabled"`

	Hysteresis struct {
		Cooling float64 `mapstructure:"cooling"`
		Heating float64 `mapstructure:"heating"`
	} `mapstructure:"hysteresis"`

	StepCheckInterval string `mapstructure:"step_check_interval"`
	StabilizationTime string `mapstructure:"stabilization_time"`

	// Hvor vi lagrer lokal fermenterings-DB.
	DatabasePath string `mapstructure:"database_path"`
}

// BrewhouseConfig - oppsett for bryggeprosessen
type BrewhouseConfig struct {
	Enabled      bool   `mapstructure:"enabled"`
	DatabasePath string `mapstructure:"database_path"`
}

// BrewfatherConfig – API-nøklene.
type BrewfatherConfig struct {
	UserID string `mapstructure:"user_id"`
	APIKey string `mapstructure:"api_key"`
}

// ServerConfig definerer innstillinger for HTTP-serveren.
type ServerConfig struct {
	ListenAddr string `mapstructure:"listen_addr"`
}

// Config er toppnivåstrukturen for YAML-konfigurasjonen.
type Config struct {
	OPCUA        OPCUAConfig        `mapstructure:"opcua"`
	Logging      LoggingConfig      `mapstructure:"logging"`
	Fermentation FermentationConfig `mapstructure:"fermentation"`
	Brewhouse    BrewhouseConfig    `mapstructure:"brewhouse"`
	Brewfather   BrewfatherConfig   `mapstructure:"brewfather"`
	Server       ServerConfig       `mapstructure:"server"`
}

// Load leser konfigurasjon fra fil og overstyrer med miljøvariabler.
// Eksempel: `opcua.endpoint` kan settes med env-var `GOTOV_OPCUA_ENDPOINT`.
func Load(path string) (*Config, error) {
	v := viper.New()

	// 1. Sett standardverdier
	v.SetDefault("logging.level", "info")
	v.SetDefault("server.listen_addr", ":8085")
	v.SetDefault("fermentation.enabled", true)
	v.SetDefault("fermentation.database_path", "data/fermentation.db")
	v.SetDefault("fermentation.hysteresis.cooling", 0.3)
	v.SetDefault("fermentation.hysteresis.heating", 0.3)
	v.SetDefault("fermentation.step_check_interval", "30s")
	v.SetDefault("fermentation.stabilization_time", "10m")
	v.SetDefault("brewhouse.enabled", true)
	v.SetDefault("brewhouse.database_path", "data/brewhouse.db")

	// 2. Konfigurer lesing fra fil
	if path != "" {
		v.SetConfigFile(path) // Bruk spesifikk fil hvis oppgitt
	} else {
		v.AddConfigPath("/app/config") // For Docker
		v.AddConfigPath("config")      // For lokal kjøring
		v.AddConfigPath(".")           // For enkelhet
		v.SetConfigName("config")
		v.SetConfigType("yaml")
	}

	// 3. Konfigurer lesing fra miljøvariabler
	v.SetEnvPrefix("GOTOV")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// 4. Les konfigurasjonen
	if err := v.ReadInConfig(); err != nil {
		// Feil hvis filen er funnet, men ikke kan leses. Ignorer hvis filen ikke finnes.
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
	}

	// 5. Unmarshal til struct
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return &cfg, nil
}
