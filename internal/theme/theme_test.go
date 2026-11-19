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
		got, err := theme.New(
			map[string]string{"border": "red", "cursor_bg": "bright-black", "added": "#00ff00", "removed": "196", "hunk": "14", "page": "none"},
			map[string]string{"keyword": "Bright_Blue"},
		)
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		for name, pair := range map[string][2]color.Color{
			"border":         {got.Border, lipgloss.Red},
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
		colors, syntax map[string]string
		want           string
	}{
		"unknown colour key":  {colors: map[string]string{"bordr": "red"}, want: "bordr"},
		"unknown syntax key":  {syntax: map[string]string{"keywrd": "red"}, want: "syntax.keywrd"},
		"invalid name":        {colors: map[string]string{"border": "redish"}, want: "border"},
		"invalid hex":         {colors: map[string]string{"border": "#12345"}, want: "border"},
		"number out of range": {colors: map[string]string{"border": "256"}, want: "border"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := theme.New(tc.colors, tc.syntax)

			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("New() error = %v, want one naming %q", err, tc.want)
			}
		})
	}
}
