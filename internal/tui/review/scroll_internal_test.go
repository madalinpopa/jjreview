package review

import (
	"reflect"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/madalinpopa/jjreview/internal/diff"
	"github.com/madalinpopa/jjreview/internal/theme"
	"github.com/madalinpopa/jjreview/internal/tui/ui"
)

func numberedRows(numbers ...int) []row {
	rows := []row{{kind: rowHunk, hunk: 0, index: -1}}
	for _, n := range numbers {
		rows = append(rows, row{kind: rowLine, hunk: -1, index: -1, line: diff.Line{Kind: diff.LineContext, OldNo: n, NewNo: n}})
	}
	return rows
}

func TestNumberWidth_success(t *testing.T) {
	t.Parallel()
	removed := row{kind: rowLine, hunk: -1, index: -1, line: diff.Line{Kind: diff.LineRemoved, OldNo: 123456}}
	tests := map[string]struct {
		rows []row
		want int
	}{
		"no rows":                           {rows: nil, want: 4},
		"a short file":                      {rows: numberedRows(1, 2, 3), want: 4},
		"the last four-digit line":          {rows: numberedRows(9998, 9999), want: 4},
		"the first five-digit line":         {rows: numberedRows(9999, 10000), want: 5},
		"a six-digit line among short ones": {rows: numberedRows(7, 100000, 8), want: 6},
		"a removed line shows no number":    {rows: append(numberedRows(12), removed), want: 4},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := numberWidth(tt.rows)

			if got != tt.want {
				t.Errorf("numberWidth() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestCodeLine_numberWidth(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		number int
		want   string
	}{
		"a four-digit line number": {number: 9999, want: "▌9999 + abc"},
		"a five-digit line number": {number: 10000, want: "▌10000 + abc"},
		"a six-digit line number":  {number: 100000, want: "▌100000 + abc"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			text := strings.Repeat("abcdefghi ", 5)[:49] + "Z"
			file := diff.File{OldPath: "f.txt", NewPath: "f.txt", Hunks: []diff.Hunk{
				{OldStart: tt.number - 1, OldLines: 0, NewStart: tt.number, NewLines: 1, Lines: []diff.Line{
					{Kind: diff.LineAdded, Text: text, NewNo: tt.number},
				}},
			}}
			m := Model{files: []diff.File{file}, reviews: []fileReview{newReview(file)}}
			env := Env{Loaded: true}
			env.Theme, env.Width, env.ListHeight = theme.Default(), 45, 10
			inner := m.diffCardWidth(env.Width) - 2
			rows := m.rows(env)
			last := rows[len(rows)-1]
			m.KeepVisible(env)

			start := ansi.Strip(m.diffLine(env.Theme, last, false, false, numberWidth(rows), inner))
			m.diffLeft = m.overflow
			end := ansi.Strip(m.diffLine(env.Theme, last, false, false, numberWidth(rows), inner))

			if inner != 40 {
				t.Fatalf("diff pane inner width = %d, want 40", inner)
			}
			if !strings.Contains(start, tt.want) || !strings.HasSuffix(start, hiddenRight) {
				t.Errorf("diffLine() at the first column = %q, want %q and a closing %s", start, tt.want, hiddenRight)
			}
			if !strings.HasSuffix(end, "Z") {
				t.Errorf("diffLine() at the last column = %q, want it to end with the last character Z", end)
			}
			for _, got := range []string{start, end} {
				if w := lipgloss.Width(got); w != inner {
					t.Errorf("diffLine() = %q is %d columns wide, want %d", got, w, inner)
				}
			}
		})
	}
}

func TestCutText_success(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		text     string
		from, to int
		want     string
	}{
		"the whole text":                          {text: "hello", from: 0, to: 5, want: "hello"},
		"a window in the middle":                  {text: "hello world", from: 3, to: 8, want: "lo wo"},
		"a window past the end":                   {text: "hello", from: 3, to: 20, want: "lo"},
		"a window after the text":                 {text: "hello", from: 5, to: 9, want: ""},
		"a start before the text":                 {text: "hello", from: -4, to: 2, want: "he"},
		"an end before the text":                  {text: "hello", from: -4, to: 0, want: ""},
		"wide characters on both edges":           {text: "世界世界", from: 2, to: 6, want: "界世"},
		"a wide character cut by the left edge":   {text: "世界世界", from: 1, to: 6, want: " 界世"},
		"a wide character cut by the right edge":  {text: "世界世界", from: 2, to: 5, want: "界"},
		"a wide character cut by both edges":      {text: "世界世", from: 1, to: 5, want: " 界"},
		"a window inside one wide character":      {text: "世", from: 1, to: 2, want: " "},
		"an emoji cut by the left edge":           {text: "a👍b", from: 2, to: 4, want: " b"},
		"a combining mark stays on its character": {text: "aéx", from: 1, to: 3, want: "éx"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := cutText(tt.text, tt.from, tt.to)

			if got != tt.want {
				t.Errorf("cutText(%q, %d, %d) = %q, want %q", tt.text, tt.from, tt.to, got, tt.want)
			}
		})
	}
}

func TestWindow_success(t *testing.T) {
	t.Parallel()
	red, blue := ui.Fg(lipgloss.Red), ui.Fg(lipgloss.Blue)
	segs := []ui.Seg{ui.Span(red, "func "), ui.Span(blue, "世界"), ui.Span(red, "() {}")}
	tests := map[string]struct {
		left, width int
		want        []ui.Seg
	}{
		"the start":                        {left: 0, width: 7, want: []ui.Seg{ui.Span(red, "func "), ui.Span(blue, "世")}},
		"a cut inside the first segment":   {left: 3, width: 4, want: []ui.Seg{ui.Span(red, "c "), ui.Span(blue, "世")}},
		"a wide character cut on the left": {left: 6, width: 5, want: []ui.Seg{ui.Span(blue, " 界"), ui.Span(red, "()")}},
		"a window past the end":            {left: 12, width: 9, want: []ui.Seg{ui.Span(red, "{}")}},
		"a window after the text":          {left: 30, width: 9, want: nil},
		"no width":                         {left: 0, width: 0, want: nil},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := window(segs, tt.left, tt.width)

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("window(segs, %d, %d) = %+v, want %+v", tt.left, tt.width, got, tt.want)
			}
		})
	}
}
