package config

import (
	"fmt"
	"log/slog"

	"github.com/adrg/xdg"
	"github.com/spf13/viper"
)

// Config holds user-facing settings for the Chassis GUI.
type Config struct {
	Debug      bool   `mapstructure:"debug"`
	ChrootPath string `mapstructure:"chroot_path"`
}

// Load reads config from ~/.config/chassis/config.toml, falling back to defaults.
// Environment variables (CHASSIS_DEBUG, CHASSIS_CHROOT) override file values.
func Load() (*Config, error) {
	v := viper.New()
	v.SetConfigType("toml")

	// Defaults
	v.SetDefault("debug", false)
	v.SetDefault("chroot_path", "")

	// Env var overrides
	v.SetEnvPrefix("CHASSIS")
	v.BindEnv("debug")
	v.BindEnv("chroot_path", "CHASSIS_CHROOT")

	// Config file location
	configPath, err := xdg.ConfigFile("chassis/config.toml")
	if err != nil {
		slog.Debug("could not resolve config path, using defaults", "error", err)
	} else {
		v.SetConfigFile(configPath)
		if err := v.ReadInConfig(); err != nil {
			if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
				// File exists but couldn't be parsed
				return nil, fmt.Errorf("reading config: %w", err)
			}
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	return &cfg, nil
}
