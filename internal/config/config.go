package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/spf13/viper"

	"github.com/madalinpopa/jjreview/internal/theme"
)

const (
	appName           = "jjr"
	fileName          = "config.toml"
	ProviderAnthropic = "anthropic"
)

type Provider struct {
	APIKey string
	Model  string
}

type Config struct {
	Provider  string
	Anthropic Provider
	Theme     theme.Theme
}

func Path() (string, error) {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("config: find home directory: %w", err)
		}
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, appName, fileName), nil
}

func Load(path string) (Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("toml")
	v.SetDefault("provider", ProviderAnthropic)
	if _, err := os.Stat(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return Config{}, fmt.Errorf("config: %w", err)
	} else if err == nil {
		if err := v.ReadInConfig(); err != nil {
			return Config{}, fmt.Errorf("config: read %s: %w", path, err)
		}
	}

	cfg := Config{
		Provider: v.GetString("provider"),
		Anthropic: Provider{
			APIKey: v.GetString("providers.anthropic.api_key"),
			Model:  v.GetString("providers.anthropic.model"),
		},
	}
	if cfg.Provider != ProviderAnthropic {
		return Config{}, fmt.Errorf("config: %s: unknown provider %q; supported: %s", path, cfg.Provider, ProviderAnthropic)
	}

	colors, syntax, err := themeValues(v.GetStringMap("theme"))
	if err != nil {
		return Config{}, fmt.Errorf("config: %s: %w", path, err)
	}
	if cfg.Theme, err = theme.New(colors, syntax); err != nil {
		return Config{}, fmt.Errorf("config: %s: %w", path, err)
	}
	return cfg, nil
}

func themeValues(raw map[string]any) (colors, syntax map[string]string, err error) {
	colors = map[string]string{}
	for key, value := range raw {
		if key == "syntax" {
			table, ok := value.(map[string]any)
			if !ok {
				return nil, nil, errors.New("theme.syntax must be a table")
			}
			if syntax, err = stringValues(table, "theme.syntax."); err != nil {
				return nil, nil, err
			}
			continue
		}
		s, ok := value.(string)
		if !ok {
			return nil, nil, fmt.Errorf("theme.%s must be a string", key)
		}
		colors[key] = s
	}
	return colors, syntax, nil
}

func stringValues(table map[string]any, prefix string) (map[string]string, error) {
	out := make(map[string]string, len(table))
	for key, value := range table {
		s, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("%s%s must be a string", prefix, key)
		}
		out[key] = s
	}
	return out, nil
}
