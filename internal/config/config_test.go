package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/madalinpopa/jjreview/internal/config"
	"github.com/madalinpopa/jjreview/internal/theme"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestPath_success(t *testing.T) {
	t.Run("uses XDG_CONFIG_HOME", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", dir)

		got, err := config.Path()
		if err != nil {
			t.Fatalf("Path() error = %v", err)
		}

		if want := filepath.Join(dir, "jjr", "config.toml"); got != want {
			t.Errorf("Path() = %q, want %q", got, want)
		}
	})

	t.Run("falls back to ~/.config", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", "")
		t.Setenv("HOME", home)

		got, err := config.Path()
		if err != nil {
			t.Fatalf("Path() error = %v", err)
		}

		if want := filepath.Join(home, ".config", "jjr", "config.toml"); got != want {
			t.Errorf("Path() = %q, want %q", got, want)
		}
	})
}

func TestLoad_success(t *testing.T) {
	t.Run("returns defaults without a config file", func(t *testing.T) {
		got, err := config.Load(filepath.Join(t.TempDir(), "missing.toml"))
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		if got.Provider != "anthropic" || got.Anthropic.APIKey != "" || got.Anthropic.Model != "" {
			t.Errorf("Load() = %+v, want the anthropic provider without key or model", got)
		}
		if got.Theme != theme.Default() {
			t.Errorf("Load().Theme differs from theme.Default()")
		}
	})

	t.Run("reads the provider and theme", func(t *testing.T) {
		path := writeConfig(t, `
provider = "anthropic"

[providers.anthropic]
api_key = "sk-test"
model = "claude-sonnet-5-5"

[theme]
border = "magenta"

[theme.syntax]
comment = "white"
`)

		got, err := config.Load(path)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		if got.Anthropic.APIKey != "sk-test" || got.Anthropic.Model != "claude-sonnet-5-5" {
			t.Errorf("Load().Anthropic = %+v", got.Anthropic)
		}
		if got.Theme.Border != lipgloss.Magenta || got.Theme.Syntax.Comment != lipgloss.White {
			t.Errorf("Load().Theme border = %v, comment = %v", got.Theme.Border, got.Theme.Syntax.Comment)
		}
		if got.Theme.Added != theme.Default().Added {
			t.Errorf("Load().Theme.Added = %v, want the default", got.Theme.Added)
		}
	})

	t.Run("reads the copilot provider", func(t *testing.T) {
		path := writeConfig(t, `
provider = "copilot"

[providers.copilot]
model = "gpt-5-mini"
`)

		got, err := config.Load(path)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		if got.Provider != config.ProviderCopilot || got.Copilot.Model != "gpt-5-mini" {
			t.Errorf("Load() provider = %q, copilot = %+v", got.Provider, got.Copilot)
		}
	})
}

func TestLoad_failure(t *testing.T) {
	for name, tc := range map[string]struct{ body, want string }{
		"invalid toml":      {body: "provider = ", want: "config.toml"},
		"unknown provider":  {body: `provider = "openai"`, want: "openai"},
		"copilot api key":   {body: "[providers.copilot]\napi_key = \"secret\"\n", want: "api_key"},
		"unknown theme key": {body: "[theme]\nbordr = \"red\"\n", want: "bordr"},
		"non-string colour": {body: "[theme]\nborder = 1\n", want: "border"},
		"invalid colour":    {body: "[theme.syntax]\nkeyword = \"nope\"\n", want: "syntax.keyword"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := config.Load(writeConfig(t, tc.body))

			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Load() error = %v, want one mentioning %q", err, tc.want)
			}
		})
	}
}
