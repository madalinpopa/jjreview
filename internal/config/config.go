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
	ProviderNone      = "none"
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
	Theme     theme.Set
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
	v.SetDefault("provider", ProviderNone)
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
	case ProviderNone, ProviderAnthropic, ProviderCopilot:
	default:
		return Config{}, fmt.Errorf("config: %s: unknown provider %q; supported: %s, %s, %s", path, cfg.Provider, ProviderNone, ProviderAnthropic, ProviderCopilot)
	}
	if v.IsSet("providers.copilot.api_key") {
		return Config{}, fmt.Errorf("config: %s: providers.copilot does not take api_key; sign in by running copilot", path)
	}

	raw, ok := v.Get("theme").(map[string]any)
	if !ok && v.IsSet("theme") {
		return Config{}, fmt.Errorf(`config: %s: theme must be a table; use [theme] mode = "light"`, path)
	}
	var err error
	if cfg.Theme, err = loadTheme(raw); err != nil {
		return Config{}, fmt.Errorf("config: %s: %w", path, err)
	}
	return cfg, nil
}

func loadTheme(raw map[string]any) (theme.Set, error) {
	mode := theme.ModeAuto
	sections := make(map[string]map[string]string, len(raw))
	profiles := make(map[string]map[string]map[string]string, 2)
	var flat []error
	for _, name := range slices.Sorted(maps.Keys(raw)) {
		if name == "mode" {
			s, ok := raw[name].(string)
			if !ok {
				return theme.Set{}, errors.New("theme.mode must be a string")
			}
			mode = theme.Mode(s)
			continue
		}
		table, ok := raw[name].(map[string]any)
		if !ok {
			flat = append(flat, flatKeyError(name))
			continue
		}
		if isProfile(name) {
			profile, err := profileSections(table, "theme."+name+".")
			if err != nil {
				return theme.Set{}, err
			}
			profiles[name] = profile
			continue
		}
		if !theme.IsSection(name) {
			return theme.Set{}, fmt.Errorf("unknown section theme.%s; use %s", name, themeKeys())
		}
		values, err := stringValues(table, "theme."+name+".")
		if err != nil {
			return theme.Set{}, err
		}
		sections[name] = values
	}
	if err := errors.Join(flat...); err != nil {
		return theme.Set{}, err
	}
	return theme.NewSet(mode, sections, profiles[string(theme.ModeLight)], profiles[string(theme.ModeDark)])
}

func isProfile(name string) bool {
	return name == string(theme.ModeLight) || name == string(theme.ModeDark)
}

func flatKeyError(name string) error {
	if isProfile(name) {
		return fmt.Errorf("theme.%s must be a table of sections: %s", name, theme.Sections())
	}
	if moved, known := theme.Moved(name); known {
		return fmt.Errorf("theme.%s moved to theme.%s", name, moved)
	}
	return fmt.Errorf("unknown theme key theme.%s; use %s", name, themeKeys())
}

func themeKeys() string {
	return "mode, light, dark or a section: " + theme.Sections()
}

func profileSections(table map[string]any, prefix string) (map[string]map[string]string, error) {
	sections := make(map[string]map[string]string, len(table))
	for _, name := range slices.Sorted(maps.Keys(table)) {
		section, ok := table[name].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("unknown theme key %s%s; use a section: %s", prefix, name, theme.Sections())
		}
		if !theme.IsSection(name) {
			return nil, fmt.Errorf("unknown section %s%s; use %s", prefix, name, theme.Sections())
		}
		values, err := stringValues(section, prefix+name+".")
		if err != nil {
			return nil, err
		}
		sections[name] = values
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
