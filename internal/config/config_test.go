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

		if got.Provider != config.ProviderNone || got.Anthropic.APIKey != "" || got.Anthropic.Model != "" {
			t.Errorf("Load() = %+v, want no provider without key or model", got)
		}
		if got.Theme != theme.DefaultSet() {
			t.Errorf("Load().Theme differs from theme.DefaultSet()")
		}
	})

	t.Run("reads the provider and theme", func(t *testing.T) {
		path := writeConfig(t, `
provider = "anthropic"

[providers.anthropic]
api_key = "sk-test"
model = "claude-sonnet-5-5"

[theme.base]
border = "magenta"

[theme.modal]
border = "cyan"

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
		if got.Theme.Mode != theme.ModeAuto || got.Theme.Light != got.Theme.Dark {
			t.Errorf("Load().Theme mode = %q, want auto with identical profiles", got.Theme.Mode)
		}
		th := got.Theme.Dark
		if th.Border != lipgloss.Magenta || th.Syntax.Comment != lipgloss.White {
			t.Errorf("Load().Theme border = %v, comment = %v", th.Border, th.Syntax.Comment)
		}
		if th.ModalBorder != lipgloss.Cyan {
			t.Errorf("Load().Theme.ModalBorder = %v, want lipgloss.Cyan", th.ModalBorder)
		}
		if th.Added != theme.Default().Added {
			t.Errorf("Load().Theme.Added = %v, want the default", th.Added)
		}
	})

	t.Run("reads the theme mode and profiles", func(t *testing.T) {
		path := writeConfig(t, `
[theme]
mode = "dark"

[theme.base]
border = "magenta"

[theme.light.diff]
added = "#166534"

[theme.dark.diff]
added = "#86efac"
removed = "none"
`)

		got, err := config.Load(path)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		if got.Theme.Mode != theme.ModeDark {
			t.Errorf("Load().Theme.Mode = %q, want %q", got.Theme.Mode, theme.ModeDark)
		}
		if got.Theme.Light.Added != lipgloss.Color("#166534") || got.Theme.Light.Border != lipgloss.Magenta {
			t.Errorf("Load().Theme.Light added = %v, border = %v", got.Theme.Light.Added, got.Theme.Light.Border)
		}
		if got.Theme.Dark.Added != lipgloss.Color("#86efac") || got.Theme.Dark.Border != lipgloss.Magenta {
			t.Errorf("Load().Theme.Dark added = %v, border = %v", got.Theme.Dark.Added, got.Theme.Dark.Border)
		}
		if got.Theme.Dark.Removed != (lipgloss.NoColor{}) || got.Theme.Light.Removed != theme.Default().Removed {
			t.Errorf("Load().Theme removed dark = %v, light = %v", got.Theme.Dark.Removed, got.Theme.Light.Removed)
		}
	})

	t.Run("reads the none provider", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.toml")
		if err := os.WriteFile(path, []byte("provider = \"none\"\n"), 0o600); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}

		got, err := config.Load(path)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		if got.Provider != config.ProviderNone {
			t.Errorf("Load() provider = %q, want %q", got.Provider, config.ProviderNone)
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

	t.Run("reads the openai provider", func(t *testing.T) {
		path := writeConfig(t, `
provider = "openai"

[providers.openai]
api_key = "sk-test"
model = "gpt-5.4"
`)

		got, err := config.Load(path)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		want := config.Provider{APIKey: "sk-test", Model: "gpt-5.4"}
		if got.Provider != config.ProviderOpenAI || got.OpenAI != want {
			t.Errorf("Load() provider = %q, openai = %+v, want %q, %+v", got.Provider, got.OpenAI, config.ProviderOpenAI, want)
		}
	})

	t.Run("reads the gemini provider", func(t *testing.T) {
		path := writeConfig(t, `
provider = "gemini"

[providers.gemini]
api_key = "gm-test"
model = "gemini-3.6-pro"
`)

		got, err := config.Load(path)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		want := config.Provider{APIKey: "gm-test", Model: "gemini-3.6-pro"}
		if got.Provider != config.ProviderGemini || got.Gemini != want {
			t.Errorf("Load() provider = %q, gemini = %+v, want %q, %+v", got.Provider, got.Gemini, config.ProviderGemini, want)
		}
	})
}

func TestLoad_failure(t *testing.T) {
	for name, tc := range map[string]struct{ body, want string }{
		"invalid toml":               {body: "provider = ", want: "config.toml"},
		"unknown provider":           {body: `provider = "mistral"`, want: `unknown provider "mistral"; supported: none, anthropic, copilot, openai, gemini`},
		"copilot api key":            {body: "[providers.copilot]\napi_key = \"secret\"\n", want: "api_key"},
		"unknown theme key":          {body: "[theme.base]\nbordr = \"red\"\n", want: "unknown colour theme.base.bordr"},
		"unknown section":            {body: "[theme.colors]\nborder = \"red\"\n", want: "unknown section theme.colors"},
		"non-string colour":          {body: "[theme.base]\nborder = 1\n", want: "theme.base.border"},
		"flat theme key":             {body: "[theme]\nadded = \"red\"\n", want: "theme.added moved to theme.diff.added"},
		"renamed flat key":           {body: "[theme]\nmodal_border = \"red\"\n", want: "theme.modal_border moved to theme.modal.border"},
		"unknown flat key":           {body: "[theme]\nbordr = \"red\"\n", want: "unknown theme key theme.bordr; use mode, light, dark or a section: bar, base, diff, files, modal or syntax"},
		"mode typo":                  {body: "[theme]\nmdoe = \"light\"\n", want: "unknown theme key theme.mdoe; use mode, light, dark or a section: bar, base, diff, files, modal or syntax"},
		"profile typo":               {body: "[theme.ligth.base]\ncursor_bg = \"#e8e4df\"\n", want: "unknown section theme.ligth; use mode, light, dark or a section: bar, base, diff, files, modal or syntax"},
		"string theme":               {body: "theme = \"light\"\n", want: `theme must be a table; use [theme] mode = "light"`},
		"json string theme":          {body: "theme = '{\"mode\":\"light\"}'\n", want: `theme must be a table; use [theme] mode = "light"`},
		"number theme":               {body: "theme = 1\n", want: `theme must be a table; use [theme] mode = "light"`},
		"array theme":                {body: "theme = [\"light\"]\n", want: `theme must be a table; use [theme] mode = "light"`},
		"every flat key":             {body: "[theme]\nadded = \"red\"\nborder = \"red\"\n", want: "theme.added moved to theme.diff.added\ntheme.border moved to theme.base.border"},
		"invalid colour":             {body: "[theme.syntax]\nkeyword = \"nope\"\n", want: "theme.syntax.keyword: "},
		"unknown mode":               {body: "[theme]\nmode = \"dusk\"\n", want: `unknown theme mode "dusk"; supported: auto, light, dark`},
		"non-string mode":            {body: "[theme]\nmode = 1\n", want: "theme.mode must be a string"},
		"inactive profile":           {body: "[theme]\nmode = \"dark\"\n[theme.light.diff]\nadded = \"nope\"\n", want: "theme.light.diff.added: unknown colour"},
		"non-table profile":          {body: "[theme]\nlight = \"red\"\n", want: "theme.light must be a table of sections: bar, base, diff, files, modal or syntax"},
		"flat profile key":           {body: "[theme.dark]\nadded = \"red\"\n", want: "unknown theme key theme.dark.added; use a section: bar, base, diff, files, modal or syntax"},
		"profile section":            {body: "[theme.dark.foo]\nadded = \"red\"\n", want: "unknown section theme.dark.foo"},
		"profile section non-string": {body: "[theme.dark.foo]\nadded = 1\n", want: "unknown section theme.dark.foo; use bar, base, diff, files, modal or syntax"},
		"profile non-string":         {body: "[theme.light.base]\nborder = 1\n", want: "theme.light.base.border must be a string"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := config.Load(writeConfig(t, tc.body))

			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Load() error = %v, want one mentioning %q", err, tc.want)
			}
		})
	}
}
