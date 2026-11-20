package tui_test

import (
	"regexp"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/madalinpopa/jjreview/internal/diff"
	"github.com/madalinpopa/jjreview/internal/theme"
	"github.com/madalinpopa/jjreview/internal/tui"
)

func sgrBefore(t *testing.T, raw, text string) string {
	t.Helper()
	re := regexp.MustCompile(`\x1b\[([0-9;]*)m` + regexp.QuoteMeta(text))
	match := re.FindStringSubmatch(raw)
	if match == nil {
		t.Fatalf("View() has no escape sequence directly before %q:\n%q", text, raw)
	}
	return match[1]
}

func hasSGR(params, code string) bool {
	for p := range strings.SplitSeq(params, ";") {
		if p == code {
			return true
		}
	}
	return false
}

func goSource(lines ...diff.Line) *fakeSource {
	src := newSource()
	added, old := 0, 0
	for _, l := range lines {
		if l.Kind != diff.LineRemoved {
			added++
		}
		if l.Kind != diff.LineAdded {
			old++
		}
	}
	src.files = []diff.File{{
		OldPath: "cmd/app/main.go", NewPath: "cmd/app/main.go", OldMode: "100644", NewMode: "100644",
		Hunks: []diff.Hunk{{OldStart: 1, OldLines: old, NewStart: 1, NewLines: added, Lines: lines}},
	}}
	return src
}

func TestHighlight_success(t *testing.T) {
	t.Run("colours Go keywords and strings while the sign stays green", func(t *testing.T) {
		src := goSource(
			diff.Line{Kind: diff.LineContext, Text: "package main", OldNo: 1, NewNo: 1},
			diff.Line{Kind: diff.LineAdded, Text: `func main() { println("hello") }`, NewNo: 2},
			diff.Line{Kind: diff.LineContext, Text: "var end = 1", OldNo: 2, NewNo: 3},
		)

		raw := press(t, loaded(t, src), "j").View().Content

		if got := sgrBefore(t, raw, "package"); !hasSGR(got, "35") {
			t.Errorf("SGR before package = %q, want magenta 35", got)
		}
		if got := sgrBefore(t, raw, "func"); !hasSGR(got, "35") {
			t.Errorf("SGR before func = %q, want magenta 35", got)
		}
		if got := sgrBefore(t, raw, `"hello"`); !hasSGR(got, "32") {
			t.Errorf("SGR before string = %q, want green 32", got)
		}
		if got := sgrBefore(t, raw, "+ "); !hasSGR(got, "32") {
			t.Errorf("SGR before sign = %q, want green 32", got)
		}
		for _, seq := range []string{"38;2;", "48;2;", "38;5;", "48;5;"} {
			if strings.Contains(raw, seq) {
				t.Errorf("View() contains fixed colour sequence %q", seq)
			}
		}
	})

	t.Run("uses the colours of a custom theme", func(t *testing.T) {
		src := goSource(
			diff.Line{Kind: diff.LineContext, Text: "package main", OldNo: 1, NewNo: 1},
			diff.Line{Kind: diff.LineAdded, Text: `var s = "hello"`, NewNo: 2},
		)
		th := theme.Default()
		th.Syntax.Keyword = lipgloss.BrightRed
		th.Added = lipgloss.BrightYellow
		m := tui.New(t.Context(), src, nil, "@").WithTheme(th)
		m, _ = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 30})
		m = press(t, run(t, m, m.Init()), "j")

		raw := m.View().Content

		if got := sgrBefore(t, raw, "package"); !hasSGR(got, "91") {
			t.Errorf("SGR before package = %q, want bright red 91", got)
		}
		if got := sgrBefore(t, raw, "+ "); !hasSGR(got, "93") {
			t.Errorf("SGR before sign = %q, want bright yellow 93", got)
		}
	})

	t.Run("renders unknown file types plain", func(t *testing.T) {
		src := newSource()
		src.files = []diff.File{{OldPath: "notes.zzqx", NewPath: "notes.zzqx", OldMode: "100644", NewMode: "100644",
			Hunks: []diff.Hunk{{OldStart: 1, OldLines: 0, NewStart: 1, NewLines: 1, Lines: []diff.Line{
				{Kind: diff.LineAdded, Text: "plain words here", NewNo: 1},
			}}}}}

		raw := loaded(t, src).View().Content

		if got := sgrBefore(t, raw, "plain words here"); got != "" && got != "0" && got != "100" {
			t.Errorf("SGR before plain text = %q, want no foreground colour", got)
		}
	})

	t.Run("colours the second line of a block comment in changes mode", func(t *testing.T) {
		src := goSource(
			diff.Line{Kind: diff.LineAdded, Text: "/* opening", NewNo: 1},
			diff.Line{Kind: diff.LineAdded, Text: "closing */", NewNo: 2},
			diff.Line{Kind: diff.LineAdded, Text: "var x = 1", NewNo: 3},
		)

		raw := loaded(t, src).View().Content

		if got := sgrBefore(t, raw, "closing */"); !hasSGR(got, "90") {
			t.Errorf("SGR before comment continuation = %q, want bright black 90", got)
		}
	})

	t.Run("colours a block comment opened before the hunk in full mode", func(t *testing.T) {
		src := goSource(
			diff.Line{Kind: diff.LineContext, Text: "inside comment", OldNo: 3, NewNo: 3},
			diff.Line{Kind: diff.LineAdded, Text: "end */", NewNo: 4},
		)
		src.files[0].Hunks[0].OldStart, src.files[0].Hunks[0].NewStart = 3, 3
		src.contents["commit1:cmd/app/main.go"] = "package main\n/* start\ninside comment\nend */\n"

		raw := press(t, loaded(t, src), "f").View().Content

		if got := sgrBefore(t, raw, "inside comment"); !hasSGR(got, "90") {
			t.Errorf("SGR before comment line = %q, want bright black 90", got)
		}
	})

	t.Run("committed lines stay dimmed without syntax colours", func(t *testing.T) {
		src := goSource(
			diff.Line{Kind: diff.LineContext, Text: "package main", OldNo: 1, NewNo: 1},
			diff.Line{Kind: diff.LineAdded, Text: `func main() {}`, NewNo: 2},
			diff.Line{Kind: diff.LineContext, Text: "var end = 1", OldNo: 2, NewNo: 3},
		)
		src.contents["parent1:cmd/app/main.go"] = "package main\nvar end = 1\n"
		m := press(t, loaded(t, src), "S", "c", "ctrl+s", "z")
		if len(src.splits) != 1 {
			t.Fatalf("splits = %d, want 1", len(src.splits))
		}

		raw := m.View().Content

		if got := sgrBefore(t, raw, "func main() {}"); !hasSGR(got, "90") {
			t.Errorf("SGR before committed line = %q, want dim bright black 90 for the whole text", got)
		}
	})
}
