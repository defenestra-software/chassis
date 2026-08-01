package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/adrg/xdg"
	"github.com/spf13/viper"
)

type Config struct {
	Debug      bool   `mapstructure:"debug"`
	ChrootPath string `mapstructure:"chroot_path"`
	SkipSetup  bool   `mapstructure:"skip_setup"`
}

// Load reads ~/.config/chassis/config.toml; CHASSIS_DEBUG and CHASSIS_CHROOT override.
func Load() (*Config, error) {
	v := viper.New()
	v.SetConfigType("toml")

	v.SetDefault("debug", false)
	v.SetDefault("chroot_path", "")
	v.SetDefault("skip_setup", false)

	v.SetEnvPrefix("CHASSIS")
	v.BindEnv("debug")
	v.BindEnv("chroot_path", "CHASSIS_CHROOT")

	configPath, err := xdg.ConfigFile("chassis/config.toml")
	if err != nil {
		slog.Debug("could not resolve config path, using defaults", "error", err)
	} else {
		v.SetConfigFile(configPath)
		if err := v.ReadInConfig(); err != nil {
			if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
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

func SaveSkipSetup(skip bool) {
	path, err := xdg.ConfigFile("chassis/config.toml")
	if err != nil {
		slog.Error("resolving config path", "error", err)
		return
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		slog.Error("creating config directory", "error", err)
		return
	}

	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("toml")

	if err := v.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if !errors.As(err, &notFound) && !errors.Is(err, os.ErrNotExist) {
			slog.Error("reading config", "error", err)
			return
		}
	}

	v.Set("skip_setup", skip)

	if err := v.WriteConfig(); err != nil {
		if err := v.WriteConfigAs(path); err != nil {
			slog.Error("writing config", "error", err)
		}
	}
}
