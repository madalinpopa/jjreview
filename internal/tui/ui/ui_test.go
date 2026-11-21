package ui_test

import (
	"image/color"
	"slices"
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/madalinpopa/jjreview/internal/theme"
	"github.com/madalinpopa/jjreview/internal/tui/ui"
)

func plain(text string) ui.Seg {
	return ui.Span(lipgloss.NewStyle(), text)
}

func TestPaint_success(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		width       int
		left, right []ui.Seg
		want        string
	}{
		"aligns the right segments to the edge": {width: 10, left: []ui.Seg{plain("ab")}, right: []ui.Seg{plain("xy")}, want: "ab      xy"},
		"pads a lone left side to the width":    {width: 5, left: []ui.Seg{plain("ab")}, want: "ab   "},
		"truncates the left side first":         {width: 6, left: []ui.Seg{plain("abcdef")}, right: []ui.Seg{plain("xy")}, want: "abc xy"},
		"clips the right side to the width":     {width: 3, right: []ui.Seg{plain("wxyz")}, want: "wxy"},
		"renders nothing at zero width":         {width: 0, left: []ui.Seg{plain("ab")}, want: ""},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := ansi.Strip(ui.Paint(tt.width, lipgloss.NoColor{}, tt.left, tt.right))

			if got != tt.want {
				t.Errorf("Paint(%d) = %q, want %q", tt.width, got, tt.want)
			}
		})
	}
}

func TestClip_success(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		text  string
		width int
		want  string
	}{
		"cuts text wider than the width": {text: "hello", width: 3, want: "hel"},
		"keeps text that fits":           {text: "hi", width: 5, want: "hi"},
		"renders nothing at zero width":  {text: "hello", width: 0, want: ""},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := ansi.Strip(ui.Clip(tt.text, tt.width))

			if got != tt.want {
				t.Errorf("Clip(%q, %d) = %q, want %q", tt.text, tt.width, got, tt.want)
			}
		})
	}
}

func TestBlank_success(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		width int
		want  string
	}{
		"fills the width with spaces":   {width: 3, want: "   "},
		"renders nothing at zero width": {width: 0, want: ""},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := ansi.Strip(ui.Blank(tt.width, lipgloss.NoColor{}))

			if got != tt.want {
				t.Errorf("Blank(%d) = %q, want %q", tt.width, got, tt.want)
			}
		})
	}
}

func TestRowWidth_success(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		left, right []ui.Seg
		want        int
	}{
		"adds a gap before the right side": {left: []ui.Seg{plain("ab"), plain("c")}, right: []ui.Seg{plain("xy")}, want: 6},
		"adds no gap without a right side": {left: []ui.Seg{plain("ab")}, want: 2},
		"counts wide runes by cell":        {left: []ui.Seg{plain("✓→")}, want: 2},
		"counts double-width runes twice":  {left: []ui.Seg{plain("a界")}, want: 3},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := ui.RowWidth(tt.left, tt.right)

			if got != tt.want {
				t.Errorf("RowWidth() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestScrollTop_success(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		top, cursor, height, total int
		want                       int
	}{
		"keeps a visible cursor in place":    {top: 2, cursor: 4, height: 5, total: 20, want: 2},
		"scrolls up to a cursor above":       {top: 5, cursor: 3, height: 5, total: 20, want: 3},
		"scrolls down to a cursor below":     {top: 0, cursor: 9, height: 5, total: 20, want: 5},
		"pulls back a top past the end":      {top: 18, cursor: 19, height: 5, total: 20, want: 15},
		"stays at zero for a short list":     {top: 3, cursor: 1, height: 5, total: 2, want: 0},
		"stays at zero without a viewport":   {top: 3, cursor: 9, height: 0, total: 20, want: 0},
		"never scrolls above the first line": {top: 0, cursor: -1, height: 5, total: 20, want: 0},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := ui.ScrollTop(tt.top, tt.cursor, tt.height, tt.total)

			if got != tt.want {
				t.Errorf("ScrollTop(%d, %d, %d, %d) = %d, want %d", tt.top, tt.cursor, tt.height, tt.total, got, tt.want)
			}
		})
	}
}

func TestPlural_success(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		n    int
		want string
	}{
		"keeps the word for one": {n: 1, want: "hunk"},
		"adds an s for zero":     {n: 0, want: "hunks"},
		"adds an s for many":     {n: 3, want: "hunks"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := ui.Plural(tt.n, "hunk")

			if got != tt.want {
				t.Errorf("Plural(%d, %q) = %q, want %q", tt.n, "hunk", got, tt.want)
			}
		})
	}
}

func TestShortID_success(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		id   string
		want string
	}{
		"cuts a long ID to eight characters": {id: "abcdef0123456789", want: "abcdef01"},
		"keeps a short ID":                   {id: "abc", want: "abc"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := ui.ShortID(tt.id)

			if got != tt.want {
				t.Errorf("ShortID(%q) = %q, want %q", tt.id, got, tt.want)
			}
		})
	}
}

func TestEscapePath_success(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		path string
		want string
	}{
		"keeps a plain path":           {path: "internal/ui/é.go", want: "internal/ui/é.go"},
		"escapes control characters":   {path: "a\tb\x1b[31m", want: `a\tb\x1b[31m`},
		"doubles existing backslashes": {path: `a\tb`, want: `a\\tb`},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := ui.EscapePath(tt.path)

			if got != tt.want {
				t.Errorf("EscapePath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestPill_success(t *testing.T) {
	t.Parallel()

	got := ui.Pill(lipgloss.Black, lipgloss.White, "main")

	if got.Text != " main " {
		t.Errorf("Pill() Text = %q, want %q", got.Text, " main ")
	}
	if got.Style.GetBackground() != lipgloss.Black {
		t.Errorf("Pill() background = %v, want %v", got.Style.GetBackground(), lipgloss.Black)
	}
}

func TestBarGlyph_success(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		c    lipgloss.Style
		want string
	}{
		"draws a bar for a colour":       {c: lipgloss.NewStyle().Foreground(lipgloss.Blue), want: "▌"},
		"draws a space without a colour": {c: lipgloss.NewStyle(), want: " "},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := ui.BarGlyph(tt.c.GetForeground())

			if got != tt.want {
				t.Errorf("BarGlyph() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCard_success(t *testing.T) {
	t.Parallel()
	p := theme.Default()

	t.Run("frames the body at the given size", func(t *testing.T) {
		t.Parallel()

		got := strings.Split(ansi.Strip(ui.Card(p, 12, 7, "Log", true, []ui.Seg{plain("1/2")}, []string{"one"})), "\n")

		want := []string{
			"╭──────────╮",
			"│ LOG  1/2 │",
			"├──────────┤",
			"│one       │",
			"│          │",
			"│          │",
			"╰──────────╯",
		}
		if !slices.Equal(got, want) {
			t.Errorf("Card() = %q, want %q", got, want)
		}
	})

	t.Run("clips the body to the card width", func(t *testing.T) {
		t.Parallel()

		got := strings.Split(ansi.Strip(ui.Card(p, 6, 5, "Log", false, nil, []string{"abcdefgh"})), "\n")

		if got[3] != "│abcd│" {
			t.Errorf("Card() body = %q, want %q", got[3], "│abcd│")
		}
	})

	t.Run("cuts the frame to the height", func(t *testing.T) {
		t.Parallel()

		got := strings.Split(ansi.Strip(ui.Card(p, 12, 2, "Log", true, nil, []string{"one"})), "\n")

		if len(got) != 2 {
			t.Errorf("Card() lines = %d, want 2", len(got))
		}
	})
}

func TestCardBorder_success(t *testing.T) {
	t.Parallel()
	p := theme.Default()
	p.Border = lipgloss.Red
	p.Selection = lipgloss.Green
	tests := map[string]struct {
		focused bool
		want    string
	}{
		"draws a focused card in the selection colour": {focused: true, want: ui.Fg(lipgloss.Green).Render("╭────╮")},
		"draws a blurred card in the border colour":    {focused: false, want: ui.Fg(lipgloss.Red).Render("╭────╮")},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, _, _ := strings.Cut(ui.Card(p, 6, 5, "Log", tt.focused, nil, nil), "\n")

			if got != tt.want {
				t.Errorf("Card(focused=%t) top edge = %q, want %q", tt.focused, got, tt.want)
			}
		})
	}
}

func TestHint_success(t *testing.T) {
	t.Parallel()
	fold := key.NewBinding(key.WithKeys("z", "enter"), key.WithHelp("z enter", "fold the hunk"))
	quit := key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit"))

	t.Run("collects the keys of every binding", func(t *testing.T) {
		t.Parallel()

		got := ui.Hint("fold", fold, quit)

		if want := []string{"z", "enter", "q"}; !slices.Equal(got.Keys, want) || got.Label != "fold" {
			t.Errorf("Hint() = %+v, want keys %q and label %q", got, want, "fold")
		}
	})

	t.Run("describes a binding with its help text", func(t *testing.T) {
		t.Parallel()

		got := ui.Describe(fold)

		if got.Label != "fold the hunk" {
			t.Errorf("Describe() Label = %q, want %q", got.Label, "fold the hunk")
		}
	})

	t.Run("renders keys apart and the label after", func(t *testing.T) {
		t.Parallel()

		got := ansi.Strip(ui.Render(lipgloss.NoColor{}, ui.HintSegs(theme.Default(), ui.Hint("fold", fold))))

		if got != "z enter fold" {
			t.Errorf("HintSegs() = %q, want %q", got, "z enter fold")
		}
	})
}

func TestDescription_success(t *testing.T) {
	t.Parallel()
	p := theme.Default()
	p.Strong = lipgloss.Red
	p.Muted = lipgloss.Green
	tests := map[string]struct {
		text       string
		immutable  bool
		want       string
		wantFg     color.Color
		wantItalic bool
	}{
		"shows the first line of a mutable revision strongly":  {text: "feat: add\n\nbody", want: "feat: add", wantFg: lipgloss.Red},
		"mutes the first line of an immutable revision":        {text: "feat: add\n\nbody", immutable: true, want: "feat: add", wantFg: lipgloss.Green},
		"marks a missing description with a muted placeholder": {text: "", want: "(no description set)", wantFg: lipgloss.Green, wantItalic: true},
		"marks a missing immutable description the same way":   {text: "", immutable: true, want: "(no description set)", wantFg: lipgloss.Green, wantItalic: true},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := ui.Description(p, tt.text, tt.immutable)

			if got.Text != tt.want {
				t.Errorf("Description(%q, %t) Text = %q, want %q", tt.text, tt.immutable, got.Text, tt.want)
			}
			if fg := got.Style.GetForeground(); fg != tt.wantFg {
				t.Errorf("Description(%q, %t) foreground = %v, want %v", tt.text, tt.immutable, fg, tt.wantFg)
			}
			if italic := got.Style.GetItalic(); italic != tt.wantItalic {
				t.Errorf("Description(%q, %t) italic = %t, want %t", tt.text, tt.immutable, italic, tt.wantItalic)
			}
		})
	}
}

func TestMessage_success(t *testing.T) {
	t.Parallel()

	got := ansi.Strip(ui.Message(theme.Default(), "loading…", 12))

	if got != "  loading…  " {
		t.Errorf("Message() = %q, want %q", got, "  loading…  ")
	}
}
