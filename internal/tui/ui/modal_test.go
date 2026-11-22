package ui_test

import (
	"slices"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/madalinpopa/jjreview/internal/theme"
	"github.com/madalinpopa/jjreview/internal/tui/ui"
)

func stripped(lines []string) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = ansi.Strip(l)
	}
	return out
}

func TestWrapLines_success(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		width int
		text  string
		want  []string
	}{
		"pads a short line to the width": {width: 6, text: "ab", want: []string{"ab    "}},
		"wraps long text on word breaks": {width: 5, text: "ab cd ef", want: []string{"ab cd", "ef   "}},
		"keeps explicit line breaks":     {width: 4, text: "ab\ncd", want: []string{"ab  ", "cd  "}},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := stripped(ui.WrapLines(theme.Default(), tt.width, lipgloss.NewStyle(), tt.text))

			if !slices.Equal(got, tt.want) {
				t.Errorf("WrapLines(%d, %q) = %q, want %q", tt.width, tt.text, got, tt.want)
			}
		})
	}
}

func TestTruncateLines_success(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		lines []string
		room  int
		want  []string
	}{
		"keeps lines that fit":             {lines: []string{"a", "b"}, room: 2, want: []string{"a", "b"}},
		"ends cut lines with the ellipsis": {lines: []string{"a", "b", "c"}, room: 2, want: []string{"a", "…"}},
		"keeps only the ellipsis in a row": {lines: []string{"a", "b"}, room: 1, want: []string{"…"}},
		"drops everything without room":    {lines: []string{"a", "b"}, room: 0, want: []string{}},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			before := slices.Clone(tt.lines)

			got := ui.TruncateLines(tt.lines, tt.room, "…")

			if !slices.Equal(got, tt.want) {
				t.Errorf("TruncateLines(%q, %d) = %q, want %q", tt.lines, tt.room, got, tt.want)
			}
			if !slices.Equal(tt.lines, before) {
				t.Errorf("TruncateLines changed its input to %q, want %q", tt.lines, before)
			}
		})
	}
}

func TestEllipsis_success(t *testing.T) {
	t.Parallel()

	got := ansi.Strip(ui.Ellipsis(theme.Default(), 4))

	if got != "…   " {
		t.Errorf("Ellipsis(4) = %q, want %q", got, "…   ")
	}
}

func TestModalBox_success(t *testing.T) {
	t.Parallel()

	got := strings.Split(ansi.Strip(ui.ModalBox(theme.Default(), []string{"ab", "cd"})), "\n")

	want := []string{"╭────╮", "│ ab │", "│ cd │", "╰────╯"}
	if !slices.Equal(got, want) {
		t.Errorf("ModalBox([ab cd]) = %q, want %q", got, want)
	}
}
