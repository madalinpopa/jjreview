package tui_test

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/madalinpopa/jjreview/internal/diff"
	"github.com/madalinpopa/jjreview/internal/tui"
)

func wideText(prefix string, n int) string {
	var parts []string
	for i := 0; i < n; i += 5 {
		parts = append(parts, fmt.Sprintf("%s%03d", prefix, i))
	}
	return strings.Join(parts, " ")
}

func wideFile(lines ...diff.Line) diff.File {
	return diff.File{OldPath: "LONG.txt", NewPath: "LONG.txt", OldMode: "100644", NewMode: "100644", Hunks: []diff.Hunk{
		{OldStart: 1, OldLines: 2, NewStart: 1, NewLines: 3, Lines: lines},
	}}
}

func wideSource() *fakeSource {
	src := newSource()
	src.files = []diff.File{readmeFile(), wideFile(
		diff.Line{Kind: diff.LineContext, Text: "ctx", OldNo: 1, NewNo: 1},
		diff.Line{Kind: diff.LineRemoved, Text: wideText("r", 150), OldNo: 2},
		diff.Line{Kind: diff.LineAdded, Text: wideText("a", 200), NewNo: 2},
		diff.Line{Kind: diff.LineAdded, Text: "short", NewNo: 3},
	)}
	src.contents["parent1:LONG.txt"] = "ctx\n" + wideText("r", 150) + "\n"
	src.contents["commit1:LONG.txt"] = "ctx\n" + wideText("a", 200) + "\nshort\n"
	return src
}

func deepSource() *fakeSource {
	src := wideSource()
	head := strings.Repeat("x\n", 9998) + "ctx\n"
	src.files[1].Hunks = []diff.Hunk{{OldStart: 9999, OldLines: 1, NewStart: 9999, NewLines: 3, Lines: []diff.Line{
		{Kind: diff.LineContext, Text: "ctx", OldNo: 9999, NewNo: 9999},
		{Kind: diff.LineAdded, Text: wideText("a", 200), NewNo: 10000},
		{Kind: diff.LineAdded, Text: "short", NewNo: 10001},
	}}}
	src.contents["parent1:LONG.txt"] = head
	src.contents["commit1:LONG.txt"] = head + wideText("a", 200) + "\nshort\n"
	return src
}

func onWide(t *testing.T, src *fakeSource) tui.Model {
	t.Helper()
	m := loaded(t, src)
	if got := m.CurrentFile(); got != "LONG.txt" {
		t.Fatalf("CurrentFile() = %q, want LONG.txt", got)
	}
	return m
}

func scrolledEnd(t *testing.T, m tui.Model) tui.Model {
	t.Helper()
	for range 40 {
		m = press(t, m, "l")
	}
	return m
}

func TestScroll_success(t *testing.T) {
	t.Run("l scrolls the code and keeps the gutter", func(t *testing.T) {
		m := onWide(t, wideSource())
		assertContains(t, view(m), "   2 + a000 a005 a010")

		got := view(press(t, m, "l"))

		assertContains(t, got, "   2 +‹5 a010 a015", "@@ -1,2 +1,3 @@", "   3 +‹")
		assertMissing(t, got, "a000", "short")
	})

	t.Run("h scrolls back", func(t *testing.T) {
		m := onWide(t, wideSource())

		assertContains(t, view(press(t, m, "l", "l", "h")), "   2 +‹5 a010 a015")
		assertContains(t, view(press(t, m, "l", "l", "h", "h")), "   2 + a000 a005 a010", "   3 + short")
	})

	t.Run("h stops at the first column", func(t *testing.T) {
		m := press(t, onWide(t, wideSource()), "h")

		assertContains(t, view(m), "   2 + a000 a005 a010", "file 1/2 · 1 hunk")
	})

	t.Run("l stops when the longest line ends at the pane edge", func(t *testing.T) {
		m := scrolledEnd(t, onWide(t, wideSource()))

		got := lineWith(t, view(m), "a195")
		assertContains(t, got, "a190 a195│")
		assertMissing(t, got, "›")
		if again := lineWith(t, view(press(t, m, "l")), "a195"); again != got {
			t.Errorf("View() line after one more l = %q, want it unchanged %q", again, got)
		}
	})

	t.Run("marks the edges that hide text", func(t *testing.T) {
		m := onWide(t, wideSource())

		got := view(m)
		assertContains(t, lineWith(t, got, "a000"), "›│")
		assertMissing(t, lineWith(t, got, "a000"), "‹")
		assertMissing(t, lineWith(t, got, "short"), "‹", "›")

		got = view(press(t, m, "l"))
		assertContains(t, lineWith(t, got, "a010"), "+‹", "›│")
		assertMissing(t, lineWith(t, got, "   3 +"), "›")
		assertContains(t, lineWith(t, got, "   1 "), "   1  ‹")

		assertContains(t, lineWith(t, view(scrolledEnd(t, m)), "a195"), "+‹")
	})

	t.Run("keeps every row as wide as the screen", func(t *testing.T) {
		m := press(t, onWide(t, wideSource()), "l", "l", "l")

		for i, l := range strings.Split(m.View().Content, "\n") {
			if w := lipgloss.Width(l); w != 120 {
				t.Errorf("View() row %d is %d columns wide, want 120: %q", i, w, ansiPattern.ReplaceAllString(l, ""))
			}
		}
	})

	t.Run("scrolls the removed lines", func(t *testing.T) {
		m := press(t, onWide(t, wideSource()), "d", "l")

		assertContains(t, view(m), "     -‹5 r010 r015", "   2 +‹5 a010 a015")
	})

	t.Run("scrolls the full file", func(t *testing.T) {
		m := press(t, onWide(t, wideSource()), "f", "l")

		assertContains(t, view(m), "   2 +‹5 a010 a015")
	})

	t.Run("scrolls while a range is open", func(t *testing.T) {
		m := press(t, onWide(t, wideSource()), "v", "l")

		assertContains(t, view(m), "   2 +‹5 a010 a015", "selecting a range")
	})

	t.Run("a picked line keeps its gutter block", func(t *testing.T) {
		m := press(t, onWide(t, wideSource()), "s", "l")

		assertContains(t, view(m), "   2 +‹5 a010 a015")
		assertPicked(t, m, "a010")
	})

	t.Run("the cursor row is filled to the pane edge", func(t *testing.T) {
		m := press(t, onWide(t, wideSource()), "j", "l")

		raw := rawLineWith(t, m, "   3 +‹")
		if !regexp.MustCompile(`\x1b\[[0-9;]*100[0-9;]*m {20,}\x1b\[m\x1b\[[0-9;]*m│`).MatchString(raw) {
			t.Errorf("View() cursor row is not filled with the cursor background (SGR 100) up to the border: %q", raw)
		}
	})

	t.Run("the range rows are filled to the pane edge", func(t *testing.T) {
		m := press(t, onWide(t, wideSource()), "j", "v", "k", "l")

		raw := rawLineWith(t, m, "   3 +‹")
		if !regexp.MustCompile(`\x1b\[[0-9;]*100[0-9;]*m {20,}\x1b\[m\x1b\[[0-9;]*m│`).MatchString(raw) {
			t.Errorf("View() range row is not filled with the range background (SGR 100) up to the border: %q", raw)
		}
	})

	t.Run("the bar names the keys when a line is wider than the pane", func(t *testing.T) {
		m := onWide(t, wideSource())

		assertContains(t, view(m), "h l scroll")
		assertMissing(t, view(press(t, m, "tab")), "h l scroll")
		assertMissing(t, view(loaded(t, newSource())), "h l scroll")
	})

	t.Run("the help lists the keys", func(t *testing.T) {
		assertContains(t, view(press(t, start(t, newSource(), nil, 120, 50), "?")), "h l", "scroll the diff left / right")
	})
}

func TestScroll_kept(t *testing.T) {
	tests := map[string][]string{
		"moving the cursor":  {"j", "k"},
		"folding the hunk":   {"z", "z"},
		"selecting the line": {"s", "s"},
		"showing removed":    {"d"},
		"a commit":           {"s", "c", "ctrl+s"},
	}
	for name, keys := range tests {
		t.Run(name, func(t *testing.T) {
			m := press(t, onWide(t, wideSource()), "l")

			m = press(t, m, keys...)

			assertContains(t, view(m), "   2 +‹5 a010 a015")
		})
	}
}

func TestScroll_reset(t *testing.T) {
	tests := map[string][]string{
		"J and K change the file":     {"J", "K"},
		"the file pane changes it":    {"tab", "j", "k", "tab"},
		"R reloads the change":        {"R"},
		"f shows the full file":       {"f"},
		"f goes back to changes only": {"f", "l", "f"},
	}
	for name, keys := range tests {
		t.Run(name, func(t *testing.T) {
			m := press(t, onWide(t, wideSource()), "l", "l")

			m = press(t, m, keys...)

			assertContains(t, view(m), "   2 + a000 a005 a010")
		})
	}
}

func TestScroll_clamped(t *testing.T) {
	t.Run("a wider window pulls the end of the line back to the pane edge", func(t *testing.T) {
		m := scrolledEnd(t, start(t, wideSource(), nil, 100, 30))

		m, _ = update(t, m, tea.WindowSizeMsg{Width: 140, Height: 30})

		assertContains(t, lineWith(t, view(m), "a195"), "a190 a195│")
	})

	t.Run("hiding the longest line pulls the rest back", func(t *testing.T) {
		src := wideSource()
		src.files[1].Hunks[0].Lines[1].Text = wideText("r", 300)
		m := press(t, onWide(t, src), "d")

		m = press(t, scrolledEnd(t, m), "d")

		assertContains(t, lineWith(t, view(m), "a195"), "a190 a195│")
	})
}

func TestScroll_longFile(t *testing.T) {
	t.Run("a five-digit line number keeps the right edge mark", func(t *testing.T) {
		m := onWide(t, deepSource())

		got := view(m)
		assertContains(t, lineWith(t, got, "a000"), "10000 + a000 a005 a010", "›│")
		assertContains(t, got, "│  9999   ctx", "10001 + short", "│        ⋯ 9998 unchanged lines")
	})

	t.Run("l reaches the last column of a five-digit line", func(t *testing.T) {
		m := scrolledEnd(t, onWide(t, deepSource()))

		got := lineWith(t, view(m), "a195")
		assertContains(t, got, "10000 +‹", "a190 a195│")
		assertMissing(t, got, "›")
	})

	t.Run("keeps every row as wide as the screen", func(t *testing.T) {
		for name, keys := range map[string][]string{"changes": {"l"}, "full file": {"f", "l"}} {
			m := press(t, onWide(t, deepSource()), keys...)

			for i, l := range strings.Split(m.View().Content, "\n") {
				if w := lipgloss.Width(l); w != 120 {
					t.Errorf("View() %s row %d is %d columns wide, want 120: %q", name, i, w, ansiPattern.ReplaceAllString(l, ""))
				}
			}
		}
	})

	t.Run("the full file aligns short line numbers with long ones", func(t *testing.T) {
		m := press(t, onWide(t, deepSource()), "f")

		assertContains(t, view(m), "│  9998   x", "│  9999   ctx", "10000 + a000")
	})
}

func TestScroll_wideCharacters(t *testing.T) {
	t.Run("a wide character cut in half leaves a blank column", func(t *testing.T) {
		src := wideSource()
		src.files[1].Hunks[0].Lines[2].Text = "a" + strings.Repeat("世", 120)

		m := press(t, onWide(t, src), "l")

		assertContains(t, view(m), "   2 +‹ 世世")
		for i, l := range strings.Split(m.View().Content, "\n") {
			if w := lipgloss.Width(l); w != 120 {
				t.Errorf("View() row %d is %d columns wide, want 120: %q", i, w, ansiPattern.ReplaceAllString(l, ""))
			}
		}
	})

	t.Run("tabs scroll as the spaces they are drawn with", func(t *testing.T) {
		src := wideSource()
		src.files[1].Hunks[0].Lines[2].Text = "\t\t\tx" + wideText("a", 200)

		m := press(t, onWide(t, src), "l")

		assertContains(t, view(m), "   2 +‹    xa000 a005")
	})
}

func TestScroll_highlight(t *testing.T) {
	t.Run("keeps the syntax colours of the text left in view", func(t *testing.T) {
		src := goSource(
			diff.Line{Kind: diff.LineContext, Text: "package main", OldNo: 1, NewNo: 1},
			diff.Line{Kind: diff.LineAdded, Text: `func main() { println("hello") } // ` + wideText("c", 200), NewNo: 2},
		)

		m := press(t, loaded(t, src), "l")

		raw := m.View().Content
		assertContains(t, view(m), `   2 +‹n() { println("hello")`)
		if got := sgrBefore(t, raw, `"hello"`); !hasSGR(got, "32") {
			t.Errorf("SGR before the string = %q, want green 32", got)
		}
		if got := sgrBefore(t, raw, "n"+"\x1b"); !hasSGR(got, "34") {
			t.Errorf("SGR before the cut function name = %q, want blue 34", got)
		}
	})
}

func TestScroll_failed(t *testing.T) {
	tests := map[string]struct {
		keys []string
		want string
	}{
		"l in the file pane":     {keys: []string{"tab", "l"}, want: "l works in the diff pane; press tab"},
		"h in the file pane":     {keys: []string{"tab", "h"}, want: "h works in the diff pane; press tab"},
		"l when every line fits": {keys: []string{"J", "l"}, want: "every line fits the pane"},
		"h when every line fits": {keys: []string{"J", "h"}, want: "every line fits the pane"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			m := press(t, onWide(t, wideSource()), tt.keys...)

			assertContains(t, view(m), tt.want)
		})
	}
}
