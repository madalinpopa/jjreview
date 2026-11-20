package config

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"

	"github.com/spf13/viper"

	"github.com/madalinpopa/jjreview/internal/theme"
)

const (
	appName           = "jjr"
	fileName          = "config.toml"
	ProviderAnthropic = "anthropic"
	ProviderCopilot   = "copilot"
)

type Provider struct {
	APIKey string
	Model  string
}

type Copilot struct {
	Model string
}

type Config struct {
	Provider  string
	Anthropic Provider
	Copilot   Copilot
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
		Copilot: Copilot{
			Model: v.GetString("providers.copilot.model"),
		},
	}
	switch cfg.Provider {
	case ProviderAnthropic, ProviderCopilot:
	default:
		return Config{}, fmt.Errorf("config: %s: unknown provider %q; supported: %s, %s", path, cfg.Provider, ProviderAnthropic, ProviderCopilot)
	}
	if v.IsSet("providers.copilot.api_key") {
		return Config{}, fmt.Errorf("config: %s: providers.copilot does not take api_key; sign in by running copilot", path)
	}

	sections, err := themeSections(v.GetStringMap("theme"))
	if err != nil {
		return Config{}, fmt.Errorf("config: %s: %w", path, err)
	}
	if cfg.Theme, err = theme.New(sections); err != nil {
		return Config{}, fmt.Errorf("config: %s: %w", path, err)
	}
	return cfg, nil
}

func themeSections(raw map[string]any) (map[string]map[string]string, error) {
	sections := make(map[string]map[string]string, len(raw))
	var flat []error
	for _, name := range slices.Sorted(maps.Keys(raw)) {
		table, ok := raw[name].(map[string]any)
		if !ok {
			if moved, known := theme.Moved(name); known {
				flat = append(flat, fmt.Errorf("theme.%s moved to theme.%s", name, moved))
			} else {
				flat = append(flat, fmt.Errorf("unknown theme key theme.%s; use a section: %s", name, theme.Sections()))
			}
			continue
		}
		values, err := stringValues(table, "theme."+name+".")
		if err != nil {
			return nil, err
		}
		sections[name] = values
	}
	if err := errors.Join(flat...); err != nil {
		return nil, err
	}
	return sections, nil
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
