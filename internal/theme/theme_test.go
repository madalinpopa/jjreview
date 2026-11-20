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

func TestNew_success(t *testing.T) {
	t.Run("overrides only the given colours", func(t *testing.T) {
		got, err := theme.New(map[string]map[string]string{
			"base":   {"border": "red", "cursor_bg": "bright-black", "page": "none"},
			"diff":   {"added": "#00ff00", "removed": "196", "hunk": "14"},
			"modal":  {"border": "blue"},
			"syntax": {"keyword": "Bright_Blue"},
		})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

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

func TestNew_failure(t *testing.T) {
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
			_, err := theme.New(tc.sections)

			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("New() error = %v, want one naming %q", err, tc.want)
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

			if _, err := theme.New(map[string]map[string]string{section: {name: "red"}}); err != nil {
				t.Errorf("Moved(%q) = %q, which New rejects: %v", key, moved, err)
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
		th, err := theme.New(map[string]map[string]string{"base": {"card": "#d8d8d8"}})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		got := th.WithBackground(color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}, false)

		if got.HunkBg != theme.Default().HunkBg {
			t.Errorf("HunkBg = %v, want the default", got.HunkBg)
		}
	})

	t.Run("keeps a configured hunk background", func(t *testing.T) {
		th, err := theme.New(map[string]map[string]string{"diff": {"hunk_bg": "blue"}})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		got := th.WithBackground(color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}, false)

		if got.HunkBg != lipgloss.Blue {
			t.Errorf("HunkBg = %v, want lipgloss.Blue", got.HunkBg)
		}
	})

	t.Run("keeps a configured cursor background", func(t *testing.T) {
		th, err := theme.New(map[string]map[string]string{"base": {"cursor_bg": "blue"}})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		got := th.WithBackground(color.RGBA{A: 0xff}, true)

		if got.CursorBg != lipgloss.Blue {
			t.Errorf("CursorBg = %v, want lipgloss.Blue", got.CursorBg)
		}
	})

	t.Run("keeps the default cursor background over a configured page", func(t *testing.T) {
		th, err := theme.New(map[string]map[string]string{"base": {"page": "#1e1e2e"}})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		got := th.WithBackground(color.RGBA{A: 0xff}, true)

		if got.CursorBg != theme.Default().CursorBg {
			t.Errorf("CursorBg = %v, want the default", got.CursorBg)
		}
	})
}
