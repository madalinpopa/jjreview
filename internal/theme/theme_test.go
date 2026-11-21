package theme_test

import (
	"image/color"
	"reflect"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/madalinpopa/jjreview/internal/theme"
)

func sameColor(a, b color.Color) bool {
	_, an := a.(lipgloss.NoColor)
	_, bn := b.(lipgloss.NoColor)
	if an || bn {
		return an && bn
	}
	if a == nil || b == nil {
		return a == b
	}
	ar, ag, ab, aa := a.RGBA()
	br, bg, bb, ba := b.RGBA()
	return ar == br && ag == bg && ab == bb && aa == ba
}

func TestDefault_success(t *testing.T) {
	t.Run("uses only terminal colours", func(t *testing.T) {
		d := theme.Default()

		for name, c := range map[string]color.Color{
			"border": d.Border, "added": d.Added, "removed": d.Removed, "hunk_bg": d.HunkBg,
			"selection": d.Selection, "syntax.keyword": d.Syntax.Keyword, "page": d.Page,
		} {
			_, none := c.(lipgloss.NoColor)
			if !none && reflect.TypeOf(c) != reflect.TypeOf(lipgloss.Black) {
				t.Errorf("Default().%s = %T, want an ANSI colour or none", name, c)
			}
		}
	})
}

func TestNewSetCommon_success(t *testing.T) {
	t.Run("overrides only the given colours", func(t *testing.T) {
		set, err := theme.NewSet(theme.ModeAuto, map[string]map[string]string{
			"base":   {"border": "red", "cursor_bg": "bright-black", "page": "none"},
			"diff":   {"added": "#00ff00", "removed": "196", "hunk": "14"},
			"modal":  {"border": "blue"},
			"syntax": {"keyword": "Bright_Blue"},
		}, nil, nil)
		if err != nil {
			t.Fatalf("NewSet() error = %v", err)
		}
		got := set.Dark

		for name, pair := range map[string][2]color.Color{
			"border":         {got.Border, lipgloss.Red},
			"modal.border":   {got.ModalBorder, lipgloss.Blue},
			"cursor_bg":      {got.CursorBg, lipgloss.BrightBlack},
			"added":          {got.Added, lipgloss.Color("#00ff00")},
			"removed":        {got.Removed, lipgloss.ANSIColor(196)},
			"hunk":           {got.Hunk, lipgloss.BrightCyan},
			"page":           {got.Page, lipgloss.NoColor{}},
			"syntax.keyword": {got.Syntax.Keyword, lipgloss.BrightBlue},
			"selection":      {got.Selection, theme.Default().Selection},
			"syntax.string":  {got.Syntax.String, theme.Default().Syntax.String},
		} {
			if !sameColor(pair[0], pair[1]) {
				t.Errorf("%s = %v, want %v", name, pair[0], pair[1])
			}
		}
	})

	t.Run("maps 0-15 to the terminal's basic colours", func(t *testing.T) {
		got, err := theme.ParseColor("9")
		if err != nil {
			t.Fatalf("ParseColor() error = %v", err)
		}

		if got != lipgloss.BrightRed {
			t.Errorf("ParseColor(9) = %#v, want lipgloss.BrightRed", got)
		}
	})
}

func TestNewSetCommon_failure(t *testing.T) {
	for name, tc := range map[string]struct {
		sections map[string]map[string]string
		want     string
	}{
		"unknown section":      {sections: map[string]map[string]string{"colors": {"border": "red"}}, want: "unknown section theme.colors; use bar, base, diff, files, modal or syntax"},
		"key in wrong section": {sections: map[string]map[string]string{"diff": {"border": "red"}}, want: "unknown colour theme.diff.border"},
		"unknown syntax key":   {sections: map[string]map[string]string{"syntax": {"keywrd": "red"}}, want: "unknown colour theme.syntax.keywrd"},
		"invalid name":         {sections: map[string]map[string]string{"base": {"border": "redish"}}, want: "theme.base.border: "},
		"invalid hex":          {sections: map[string]map[string]string{"base": {"border": "#12345"}}, want: "theme.base.border: "},
		"number out of range":  {sections: map[string]map[string]string{"base": {"border": "256"}}, want: "theme.base.border: "},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := theme.NewSet(theme.ModeAuto, tc.sections, nil, nil)

			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("NewSet() error = %v, want one naming %q", err, tc.want)
			}
		})
	}
}

func TestMoved_success(t *testing.T) {
	for key, want := range map[string]string{
		"border":         "base.border",
		"added":          "diff.added",
		"dir_name":       "files.dir_name",
		"badge_selected": "bar.badge_selected",
		"modal_border":   "modal.border",
		"input":          "modal.input",
	} {
		t.Run(key, func(t *testing.T) {
			got, ok := theme.Moved(key)

			if !ok || got != want {
				t.Errorf("Moved(%q) = %q, %v, want %q, true", key, got, ok, want)
			}
		})
	}
}

func TestMoved_everyFlatKey(t *testing.T) {
	flat := []string{
		"page", "text", "card", "border", "chip", "chip_text", "title", "header", "strong", "dir_name",
		"muted", "faint", "dim", "added", "added_bg", "removed", "removed_bg", "hunk", "hunk_bg",
		"hunk_bar", "folded_bar", "stats", "selection", "cursor_bg", "key", "status", "picked",
		"on_selection", "badge_idle", "badge_idle_fg", "badge_selected", "modal_border", "input",
	}

	for _, key := range flat {
		t.Run(key, func(t *testing.T) {
			moved, ok := theme.Moved(key)
			if !ok {
				t.Fatalf("Moved(%q) = false, want the new path", key)
			}
			section, name, _ := strings.Cut(moved, ".")

			if _, err := theme.NewSet(theme.ModeAuto, map[string]map[string]string{section: {name: "red"}}, nil, nil); err != nil {
				t.Errorf("Moved(%q) = %q, which NewSet rejects: %v", key, moved, err)
			}
		})
	}
}

func TestMoved_failed(t *testing.T) {
	for _, key := range []string{"bordr", "type", "keyword"} {
		t.Run(key, func(t *testing.T) {
			if got, ok := theme.Moved(key); ok {
				t.Errorf("Moved(%q) = %q, true, want false", key, got)
			}
		})
	}
}

func TestIsSection_success(t *testing.T) {
	for _, name := range []string{"bar", "base", "diff", "files", "modal", "syntax"} {
		t.Run(name, func(t *testing.T) {
			if !theme.IsSection(name) {
				t.Errorf("IsSection(%q) = false, want true", name)
			}
		})
	}
}

func TestIsSection_failed(t *testing.T) {
	for _, name := range []string{"colors", "light", "dark", "mode", ""} {
		t.Run(name, func(t *testing.T) {
			if theme.IsSection(name) {
				t.Errorf("IsSection(%q) = true, want false", name)
			}
		})
	}
}

func TestTheme_WithBackground(t *testing.T) {
	t.Run("derives a subtle cursor background from the terminal", func(t *testing.T) {
		dark := theme.Default().WithBackground(color.RGBA{R: 0x20, G: 0x20, B: 0x20, A: 0xff}, true)
		light := theme.Default().WithBackground(color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}, false)

		if want := (color.RGBA{R: 0x34, G: 0x34, B: 0x34, A: 0xff}); !sameColor(dark.CursorBg, want) {
			t.Errorf("dark CursorBg = %v, want %v", dark.CursorBg, want)
		}
		if want := (color.RGBA{R: 0xef, G: 0xef, B: 0xef, A: 0xff}); !sameColor(light.CursorBg, want) {
			t.Errorf("light CursorBg = %v, want %v", light.CursorBg, want)
		}
	})

	t.Run("derives a darker hunk background on a light terminal only", func(t *testing.T) {
		dark := theme.Default().WithBackground(color.RGBA{R: 0x20, G: 0x20, B: 0x20, A: 0xff}, true)
		light := theme.Default().WithBackground(color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}, false)

		if dark.HunkBg != theme.Default().HunkBg {
			t.Errorf("dark HunkBg = %v, want the default", dark.HunkBg)
		}
		if want := (color.RGBA{R: 0xe0, G: 0xe0, B: 0xe0, A: 0xff}); !sameColor(light.HunkBg, want) {
			t.Errorf("light HunkBg = %v, want %v", light.HunkBg, want)
		}
	})

	t.Run("restores the default hunk background when the terminal turns dark", func(t *testing.T) {
		light := theme.Default().WithBackground(color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}, false)

		got := light.WithBackground(color.RGBA{R: 0x20, G: 0x20, B: 0x20, A: 0xff}, true)

		if got.HunkBg != theme.Default().HunkBg {
			t.Errorf("HunkBg = %v, want the default", got.HunkBg)
		}
	})

	t.Run("keeps the default hunk background over a configured card", func(t *testing.T) {
		set, err := theme.NewSet(theme.ModeAuto, map[string]map[string]string{"base": {"card": "#d8d8d8"}}, nil, nil)
		if err != nil {
			t.Fatalf("NewSet() error = %v", err)
		}
		th := set.Dark

		got := th.WithBackground(color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}, false)

		if got.HunkBg != theme.Default().HunkBg {
			t.Errorf("HunkBg = %v, want the default", got.HunkBg)
		}
	})

	t.Run("keeps a configured hunk background", func(t *testing.T) {
		set, err := theme.NewSet(theme.ModeAuto, map[string]map[string]string{"diff": {"hunk_bg": "blue"}}, nil, nil)
		if err != nil {
			t.Fatalf("NewSet() error = %v", err)
		}
		th := set.Dark

		got := th.WithBackground(color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}, false)

		if got.HunkBg != lipgloss.Blue {
			t.Errorf("HunkBg = %v, want lipgloss.Blue", got.HunkBg)
		}
	})

	t.Run("keeps a configured cursor background", func(t *testing.T) {
		set, err := theme.NewSet(theme.ModeAuto, map[string]map[string]string{"base": {"cursor_bg": "blue"}}, nil, nil)
		if err != nil {
			t.Fatalf("NewSet() error = %v", err)
		}
		th := set.Dark

		got := th.WithBackground(color.RGBA{A: 0xff}, true)

		if got.CursorBg != lipgloss.Blue {
			t.Errorf("CursorBg = %v, want lipgloss.Blue", got.CursorBg)
		}
	})

	t.Run("keeps the default cursor background over a configured page", func(t *testing.T) {
		set, err := theme.NewSet(theme.ModeAuto, map[string]map[string]string{"base": {"page": "#1e1e2e"}}, nil, nil)
		if err != nil {
			t.Fatalf("NewSet() error = %v", err)
		}
		th := set.Dark

		got := th.WithBackground(color.RGBA{A: 0xff}, true)

		if got.CursorBg != theme.Default().CursorBg {
			t.Errorf("CursorBg = %v, want the default", got.CursorBg)
		}
	})
}

func TestNewSet_success(t *testing.T) {
	t.Run("layers the active profile over the common sections", func(t *testing.T) {
		got, err := theme.NewSet(theme.ModeAuto,
			map[string]map[string]string{"base": {"border": "magenta"}, "diff": {"added": "green", "removed": "red"}},
			map[string]map[string]string{"diff": {"added": "#166534"}},
			map[string]map[string]string{"diff": {"added": "#86efac", "removed": "none"}},
		)
		if err != nil {
			t.Fatalf("NewSet() error = %v", err)
		}

		for name, pair := range map[string][2]color.Color{
			"light.border":  {got.Light.Border, lipgloss.Magenta},
			"light.added":   {got.Light.Added, lipgloss.Color("#166534")},
			"light.removed": {got.Light.Removed, lipgloss.Red},
			"light.hunk":    {got.Light.Hunk, theme.Default().Hunk},
			"dark.border":   {got.Dark.Border, lipgloss.Magenta},
			"dark.added":    {got.Dark.Added, lipgloss.Color("#86efac")},
			"dark.removed":  {got.Dark.Removed, lipgloss.NoColor{}},
		} {
			if !sameColor(pair[0], pair[1]) {
				t.Errorf("%s = %v, want %v", name, pair[0], pair[1])
			}
		}
		if got.Mode != theme.ModeAuto {
			t.Errorf("Mode = %q, want %q", got.Mode, theme.ModeAuto)
		}
	})

	t.Run("applies the common sections to both profiles when none is configured", func(t *testing.T) {
		common := map[string]map[string]string{"base": {"border": "red", "page": "#1e1e2e"}}

		got, err := theme.NewSet(theme.ModeDark, common, nil, nil)
		if err != nil {
			t.Fatalf("NewSet() error = %v", err)
		}

		if got.Light != got.Dark || got.Dark.Border != lipgloss.Red || !sameColor(got.Dark.Page, lipgloss.Color("#1e1e2e")) {
			t.Errorf("NewSet() = %+v, want equal profiles with the common colours", got)
		}
	})

	t.Run("disables auto shading only for the profile that sets the colour", func(t *testing.T) {
		got, err := theme.NewSet(theme.ModeAuto, nil,
			map[string]map[string]string{"diff": {"hunk_bg": "blue"}},
			map[string]map[string]string{"base": {"cursor_bg": "blue"}},
		)
		if err != nil {
			t.Fatalf("NewSet() error = %v", err)
		}
		bg := color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}

		light := got.Light.WithBackground(bg, false)
		dark := got.Dark.WithBackground(bg, false)

		if light.HunkBg != lipgloss.Blue || sameColor(light.CursorBg, theme.Default().CursorBg) {
			t.Errorf("light HunkBg = %v, CursorBg = %v, want blue and a derived cursor", light.HunkBg, light.CursorBg)
		}
		if dark.CursorBg != lipgloss.Blue || sameColor(dark.HunkBg, theme.Default().HunkBg) {
			t.Errorf("dark CursorBg = %v, HunkBg = %v, want blue and a derived hunk", dark.CursorBg, dark.HunkBg)
		}
	})

	t.Run("computes auto shading on the merged page", func(t *testing.T) {
		got, err := theme.NewSet(theme.ModeAuto,
			map[string]map[string]string{"base": {"page": "#1e1e2e"}},
			map[string]map[string]string{"base": {"page": "none"}},
			nil,
		)
		if err != nil {
			t.Fatalf("NewSet() error = %v", err)
		}

		light := got.Light.WithBackground(color.RGBA{A: 0xff}, true)
		dark := got.Dark.WithBackground(color.RGBA{A: 0xff}, true)

		if sameColor(light.CursorBg, theme.Default().CursorBg) {
			t.Errorf("light CursorBg = %v, want a derived cursor", light.CursorBg)
		}
		if dark.CursorBg != theme.Default().CursorBg {
			t.Errorf("dark CursorBg = %v, want the default", dark.CursorBg)
		}
	})
}

func TestNewSet_failure(t *testing.T) {
	valid := map[string]map[string]string{"diff": {"added": "green"}}
	for name, tc := range map[string]struct {
		mode                theme.Mode
		common, light, dark map[string]map[string]string
		want                string
	}{
		"unknown mode":            {mode: "dusk", want: `unknown theme mode "dusk"; supported: auto, light, dark`},
		"invalid common colour":   {mode: theme.ModeAuto, common: map[string]map[string]string{"diff": {"added": "nope"}}, want: "theme.diff.added: unknown colour"},
		"invalid inactive colour": {mode: theme.ModeDark, light: map[string]map[string]string{"diff": {"added": "nope"}}, dark: valid, want: "theme.light.diff.added: unknown colour"},
		"unknown profile colour":  {mode: theme.ModeLight, dark: map[string]map[string]string{"diff": {"addd": "red"}}, want: "unknown colour theme.dark.diff.addd"},
		"unknown profile section": {mode: theme.ModeAuto, dark: map[string]map[string]string{"foo": {"added": "red"}}, want: "unknown section theme.dark.foo; use bar, base, diff, files, modal or syntax"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := theme.NewSet(tc.mode, tc.common, tc.light, tc.dark)

			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("NewSet() error = %v, want one naming %q", err, tc.want)
			}
		})
	}
}

func TestSet_Resolve(t *testing.T) {
	light := theme.Default()
	light.Added = lipgloss.Red
	dark := theme.Default()
	dark.Added = lipgloss.Blue

	for name, tc := range map[string]struct {
		mode theme.Mode
		dark bool
		want color.Color
	}{
		"auto on light terminal":  {mode: theme.ModeAuto, dark: false, want: lipgloss.Red},
		"auto on dark terminal":   {mode: theme.ModeAuto, dark: true, want: lipgloss.Blue},
		"light on dark terminal":  {mode: theme.ModeLight, dark: true, want: lipgloss.Red},
		"light on light terminal": {mode: theme.ModeLight, dark: false, want: lipgloss.Red},
		"dark on light terminal":  {mode: theme.ModeDark, dark: false, want: lipgloss.Blue},
		"dark on dark terminal":   {mode: theme.ModeDark, dark: true, want: lipgloss.Blue},
	} {
		t.Run(name, func(t *testing.T) {
			s := theme.Set{Light: light, Dark: dark, Mode: tc.mode}

			got := s.Resolve(tc.dark)

			if got.Added != tc.want {
				t.Errorf("Resolve(%v).Added = %v, want %v", tc.dark, got.Added, tc.want)
			}
		})
	}
}

func TestDefaultSet_success(t *testing.T) {
	got := theme.DefaultSet()

	if got.Light != theme.Default() || got.Dark != theme.Default() || got.Mode != theme.ModeAuto {
		t.Errorf("DefaultSet() = %+v, want both profiles Default() in auto mode", got)
	}
}
