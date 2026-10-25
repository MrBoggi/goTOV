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

// ADSConfig configures the optional, read-only ADS V1 input reader.
// It intentionally contains no output or control settings.
type ADSConfig struct {
	Enabled        bool   `mapstructure:"enabled"`
	Host           string `mapstructure:"host"`
	Port           int    `mapstructure:"port"`
	TargetNetID    string `mapstructure:"target_net_id"`
	TargetPort     uint16 `mapstructure:"target_port"`
	LocalNetID     string `mapstructure:"local_net_id"`
	LocalPort      uint16 `mapstructure:"local_port"`
	RequestTimeout string `mapstructure:"request_timeout"`
	PollInterval   string `mapstructure:"poll_interval"`
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
	ADS          ADSConfig          `mapstructure:"ads"`
	Logging      LoggingConfig      `mapstructure:"logging"`
	Fermentation FermentationConfig `mapstructure:"fermentation"`
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
	v.SetDefault("ads.enabled", false)
	v.SetDefault("ads.port", 48898)
	v.SetDefault("ads.target_port", 851)
	v.SetDefault("ads.local_port", 10500)
	v.SetDefault("ads.request_timeout", "8s")
	v.SetDefault("ads.poll_interval", "5s")

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
