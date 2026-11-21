package tui_test

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/madalinpopa/jjreview/internal/diff"
	"github.com/madalinpopa/jjreview/internal/tui"
)

func assertContains(t *testing.T, got string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(got, want) {
			t.Errorf("View() missing %q:\n%s", want, got)
		}
	}
}

func lineWith(t *testing.T, screen, needle string) string {
	t.Helper()
	for l := range strings.SplitSeq(screen, "\n") {
		if strings.Contains(l, needle) {
			return l
		}
	}
	t.Fatalf("View() has no line with %q:\n%s", needle, screen)
	return ""
}

func TestLoad_success(t *testing.T) {
	t.Run("renders top bar, cards and key bar", func(t *testing.T) {
		m := loaded(t, newSource())

		got := view(m)

		assertContains(t, got,
			"@ kxyzabcd", "changes only", "nothing selected",
			"FILES", "0/4 ✓", "UNIFIED DIFF", "src/ui/view.go",
			"tab pane · j k line · space select · c commit · d diff · e edit · ? help", "file 1/4 · 1 hunk",
		)
	})

	t.Run("shows the revision description in the top bar", func(t *testing.T) {
		src := newSource()
		src.change.Description = "feat: add log tab\n\nlonger body\n"

		got := view(loaded(t, src))

		top := lineWith(t, got, "@ kxyzabcd")
		assertContains(t, top, "@ kxyzabcd  feat: add log tab")
		assertMissing(t, top, "jj review", "longer body")
	})

	t.Run("leaves the title empty without a description", func(t *testing.T) {
		src := newSource()
		src.change.Description = ""

		top := lineWith(t, view(loaded(t, src)), "@ kxyzabcd")

		assertMissing(t, top, "jj review")
	})

	t.Run("sorts the file tree with directories first", func(t *testing.T) {
		m := loaded(t, newSource())

		got := view(m)

		order := []string{"▾ src/", "▾ ui/", "○ view.go", "○ main.go", "○ README.md", "○ img.png"}
		last := -1
		for _, want := range order {
			i := strings.Index(got, want)
			if i <= last {
				t.Fatalf("View() %q at %d, want after %d:\n%s", want, i, last, got)
			}
			last = i
		}
		if l := lineWith(t, got, "▾ src/"); !strings.Contains(l, "2") {
			t.Errorf("src/ row = %q, want file count 2", l)
		}
		if l := lineWith(t, got, "main.go"); !strings.Contains(l, "+2 −1") {
			t.Errorf("main.go row = %q, want +2 −1", l)
		}
	})

	t.Run("highlights the current file row like the diff cursor", func(t *testing.T) {
		m := loaded(t, newSource())

		got, raw := view(m), m.View().Content

		if l := lineWith(t, got, "○ view.go"); !strings.HasPrefix(l, "│▌") {
			t.Errorf("current file row = %q, want a selection bar", l)
		}
		if l := lineWith(t, got, "○ main.go"); strings.HasPrefix(l, "│▌") {
			t.Errorf("other file row = %q, want no selection bar", l)
		}
		if params := sgrBefore(t, raw, "view.go"); !hasSGR(params, "100") {
			t.Errorf("SGR before current file = %q, want cursor background 100", params)
		}
	})

	t.Run("renders hunk headers, gaps and one new line number gutter", func(t *testing.T) {
		m := press(t, loaded(t, newSource()), "tab", "j", "tab", "d")

		got := view(m)

		assertContains(t, got, "⋯ 2 unchanged lines", "⋯ 5 unchanged lines", "▾ ○ @@ -3,2 +3,2 @@", "+1 −1")
		if l := lineWith(t, got, "old line"); !strings.Contains(l, "▌     - old line") {
			t.Errorf("removed row = %q, want blank gutter", l)
		}
		if l := lineWith(t, got, "new line"); !strings.Contains(l, "▌   4 + new line") {
			t.Errorf("added row = %q, want new line number 4", l)
		}
	})

	t.Run("colours added, removed and hunk lines with the terminal's ANSI palette", func(t *testing.T) {
		m := press(t, loaded(t, newSource()), "tab", "j", "tab", "d")

		raw := m.View().Content

		for name, code := range map[string]string{"green": "32", "red": "31", "cyan": "36"} {
			if !regexp.MustCompile(`\x1b\[([0-9]+;)*` + code + `(;[0-9]+)*m`).MatchString(raw) {
				t.Errorf("View() missing ANSI %s foreground (SGR %s)", name, code)
			}
		}
	})

	t.Run("leaves background and colours to the terminal theme", func(t *testing.T) {
		m := press(t, loaded(t, newSource()), "tab", "j", "tab")

		v := m.View()

		for _, seq := range []string{"38;2;", "48;2;", "38;5;", "48;5;"} {
			if strings.Contains(v.Content, seq) {
				t.Errorf("View() contains fixed colour sequence %q", seq)
			}
		}
		for _, bg := range []string{"[4", ";4"} {
			for d := '0'; d <= '7'; d++ {
				if strings.Contains(v.Content, bg+string(d)+"m") {
					t.Errorf("View() sets an ANSI background (%s%cm)", bg, d)
				}
			}
		}
		if v.BackgroundColor != nil || v.ForegroundColor != nil {
			t.Errorf("View() colours = %v / %v, want terminal defaults", v.BackgroundColor, v.ForegroundColor)
		}
	})

	t.Run("tints only the cursor line with the subtle grey of the hunk headers", func(t *testing.T) {
		m := press(t, loaded(t, newSource()), "tab", "j", "tab")
		tinted := regexp.MustCompile(`\x1b\[([0-9]+;)*100(;[0-9]+)*m`)

		var hits []string
		for line := range strings.SplitSeq(m.View().Content, "\n") {
			if tinted.MatchString(line) && !strings.Contains(line, "@@") {
				hits = append(hits, line)
			}
		}

		if len(hits) != 1 || !strings.Contains(ansiPattern.ReplaceAllString(hits[0], ""), "+ new line") {
			t.Errorf("tinted lines = %q, want only the cursor line (new line)", hits)
		}
	})

	t.Run("describes binary files", func(t *testing.T) {
		m := press(t, loaded(t, newSource()), "tab", "j", "j", "j", "tab")

		assertContains(t, view(m), "binary file", "file 4/4 · 0 hunks")
	})

	t.Run("shows no changes for an empty diff", func(t *testing.T) {
		src := newSource()
		src.files = nil

		assertContains(t, view(loaded(t, src)), "no changes", "0/0 ✓")
	})

	t.Run("loads the diff of the resolved commit", func(t *testing.T) {
		src := newSource()

		loaded(t, src)

		if want := []string{"commit1"}; !slices.Equal(src.diffRevs, want) {
			t.Errorf("Diff revs = %q, want %q", src.diffRevs, want)
		}
	})
}

func TestLoad_failed(t *testing.T) {
	src := newSource()
	src.err = errors.New("revision not found")

	m := loaded(t, src)

	assertContains(t, view(m), "revision not found")
}

func TestCursor_success(t *testing.T) {
	tests := map[string]struct {
		keys []string
		want int
	}{
		"starts on the first changed line": {keys: nil, want: 3},
		"j skips gap rows":                 {keys: []string{"j"}, want: 5},
		"j moves down":                     {keys: []string{"j", "j"}, want: 6},
		"k skips gap rows":                 {keys: []string{"j", "k"}, want: 3},
		"k stops below the leading gap":    {keys: []string{"k", "k", "k", "k"}, want: 1},
		"j clamps at the last row":         {keys: slices.Repeat([]string{"down"}, 20), want: 7},
		"up moves up":                      {keys: []string{"up"}, want: 2},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			m := press(t, loaded(t, newSource()), "tab", "j", "tab")

			m = press(t, m, tt.keys...)

			if got := m.Cursor(); got != tt.want {
				t.Errorf("Cursor() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestFileNavigation_success(t *testing.T) {
	tests := map[string]struct {
		keys []string
		want string
	}{
		"starts at the first tree file": {keys: nil, want: "src/ui/view.go"},
		"plain n and p do not switch":   {keys: []string{"n", "n", "p"}, want: "src/ui/view.go"},
		"j moves to the next file":      {keys: []string{"tab", "j"}, want: "src/main.go"},
		"j clamps at the last file":     {keys: []string{"tab", "j", "j", "j", "j", "j"}, want: "img.png"},
		"k moves back":                  {keys: []string{"tab", "j", "j", "k"}, want: "src/main.go"},
		"k clamps at the first file":    {keys: []string{"tab", "k"}, want: "src/ui/view.go"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			m := press(t, loaded(t, newSource()), tt.keys...)

			if got := m.CurrentFile(); got != tt.want {
				t.Errorf("CurrentFile() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSpaceSelect_success(t *testing.T) {
	t.Run("space selects the hunk under the cursor in the diff pane", func(t *testing.T) {
		m := press(t, loaded(t, newSource()), "tab", "j", "tab", "space")

		assertContains(t, view(m), "▾ ● @@ -3,2 +3,2 @@", "1 hunk selected", "◐ main.go")
	})

	t.Run("space selects the whole file in the file pane", func(t *testing.T) {
		m := press(t, loaded(t, newSource()), "tab", "j", "space")

		assertContains(t, view(m), "2 hunks selected", "● main.go")
	})

	t.Run("a selected hunk header gets a blue background", func(t *testing.T) {
		m := press(t, loaded(t, newSource()), "tab", "j", "tab", "space")

		if !regexp.MustCompile(`\x1b\[([0-9]+;)*44(;[0-9]+)*m[^\x1b]*@@ -3,2 \+3,2 @@`).MatchString(m.View().Content) {
			t.Error("View() selected hunk header has no ANSI blue (SGR 44) background")
		}
	})
}

func TestPaneFocus_success(t *testing.T) {
	t.Run("starts with the diff pane focused", func(t *testing.T) {
		m := loaded(t, newSource())

		if m.FilesFocused() {
			t.Error("FilesFocused() = true, want false")
		}
	})

	t.Run("only tab moves the focus", func(t *testing.T) {
		for _, k := range []string{"h", "ctrl+h", "left"} {
			if m := press(t, loaded(t, newSource()), k); m.FilesFocused() {
				t.Errorf("FilesFocused() after %s = true, want false", k)
			}
		}
		for _, k := range []string{"l", "ctrl+l", "right"} {
			if m := press(t, loaded(t, newSource()), "tab", k); !m.FilesFocused() {
				t.Errorf("FilesFocused() after tab %s = false, want true", k)
			}
		}
	})

	t.Run("j and k move between files in the file pane", func(t *testing.T) {
		tests := map[string]struct {
			keys []string
			want string
		}{
			"j moves to the next file":   {keys: []string{"tab", "j"}, want: "src/main.go"},
			"j clamps at the last file":  {keys: []string{"tab", "j", "j", "j", "j", "j"}, want: "img.png"},
			"k moves back":               {keys: []string{"tab", "j", "j", "k"}, want: "src/main.go"},
			"k clamps at the first file": {keys: []string{"tab", "k"}, want: "src/ui/view.go"},
			"down and up work too":       {keys: []string{"tab", "down", "down", "up"}, want: "src/main.go"},
		}
		for name, tt := range tests {
			t.Run(name, func(t *testing.T) {
				m := press(t, loaded(t, newSource()), tt.keys...)

				if got := m.CurrentFile(); got != tt.want {
					t.Errorf("CurrentFile() = %q, want %q", got, tt.want)
				}
			})
		}
	})

	t.Run("j in the file pane resets the diff cursor of the new file", func(t *testing.T) {
		m := press(t, loaded(t, newSource()), "tab", "j")

		if got := m.Cursor(); got != 3 {
			t.Errorf("Cursor() = %d, want 3", got)
		}
	})

	t.Run("j and k move diff lines again after l", func(t *testing.T) {
		m := press(t, loaded(t, newSource()), "tab", "j", "tab", "j")

		if got, want := m.CurrentFile(), "src/main.go"; got != want {
			t.Errorf("CurrentFile() = %q, want %q", got, want)
		}
		if got := m.Cursor(); got != 5 {
			t.Errorf("Cursor() = %d, want 5", got)
		}
	})
}

func TestFold_success(t *testing.T) {
	t.Run("z folds the hunk under the cursor and moves to its header", func(t *testing.T) {
		m := press(t, loaded(t, newSource()), "tab", "j", "tab", "z")

		got := view(m)

		assertContains(t, got, "▸ ○ @@ -3,2 +3,2 @@", "+1 −1 · folded", "(1 folded)")
		if strings.Contains(got, "new line") {
			t.Errorf("View() shows folded lines:\n%s", got)
		}
		if m.Cursor() != 1 {
			t.Errorf("Cursor() = %d, want 1", m.Cursor())
		}
	})

	t.Run("enter unfolds again", func(t *testing.T) {
		m := press(t, loaded(t, newSource()), "tab", "j", "tab", "z", "enter")

		assertContains(t, view(m), "▾ ○ @@ -3,2 +3,2 @@", "new line")
	})

	t.Run("Z folds all hunks then unfolds them", func(t *testing.T) {
		m := press(t, loaded(t, newSource()), "tab", "j", "tab", "Z")

		assertContains(t, view(m), "(2 folded)")

		m = press(t, m, "Z")

		if got := view(m); strings.Contains(got, "folded") {
			t.Errorf("View() still folded:\n%s", got)
		}
	})
}

func repeatHunk(oldStart, newStart int) diff.Hunk {
	return diff.Hunk{OldStart: oldStart, OldLines: 1, NewStart: newStart, NewLines: 2, Lines: []diff.Line{
		{Kind: diff.LineContext, Text: "x", OldNo: oldStart, NewNo: newStart},
		{Kind: diff.LineAdded, Text: "y", NewNo: newStart + 1},
	}}
}

func repeatSource(hunks ...diff.Hunk) *fakeSource {
	src := newSource()
	src.files = []diff.File{{OldPath: "a.go", NewPath: "a.go", OldMode: "100644", NewMode: "100644", Hunks: hunks}}
	return src
}

func reload(t *testing.T, m tui.Model) tui.Model {
	t.Helper()
	m, cmd := update(t, launched(t, m), tui.EditorFinishedMsg{})
	return run(t, m, cmd)
}

func TestFold_reload(t *testing.T) {
	t.Run("keeps the fold of only the folded hunk among identical hunks", func(t *testing.T) {
		m := press(t, loaded(t, repeatSource(repeatHunk(3, 3), repeatHunk(20, 20))), "z")

		got := view(reload(t, m))

		assertContains(t, got, "▸ ○ @@ -3,1 +3,2 @@", "▾ ○ @@ -20,1 +20,2 @@", "(1 folded)")
	})

	t.Run("keeps the fold of a hunk shifted by lines added above", func(t *testing.T) {
		src := repeatSource(repeatHunk(20, 20))
		m := press(t, loaded(t, src), "z")
		src.files[0].Hunks = []diff.Hunk{
			{OldStart: 2, OldLines: 0, NewStart: 2, NewLines: 1, Lines: []diff.Line{
				{Kind: diff.LineAdded, Text: "top", NewNo: 2},
			}},
			repeatHunk(20, 21),
		}

		got := view(reload(t, m))

		assertContains(t, got, "▾ ○ @@ -2,0 +2,1 @@", "▸ ○ @@ -20,1 +21,2 @@", "(1 folded)")
	})
}

func TestSelect_success(t *testing.T) {
	t.Run("shows the resulting file without removed lines", func(t *testing.T) {
		src := newSource()
		src.contents["commit1:README.md"] = "# jjreview\n\nTUI for jujutsu\n"

		got := view(press(t, loaded(t, src), "tab", "j", "j", "tab", "f"))

		assertContains(t, got, "# jjreview", "+ TUI for jujutsu")
		if strings.Contains(got, "TUI for git") {
			t.Error("View() in full mode shows the removed line")
		}
	})

	t.Run("says a deleted file has no resulting content", func(t *testing.T) {
		src := newSource()
		src.files = []diff.File{{OldPath: "gone.txt", OldMode: "100644", Status: diff.StatusDeleted, Hunks: []diff.Hunk{
			{OldStart: 1, OldLines: 1, Lines: []diff.Line{{Kind: diff.LineRemoved, Text: "bye", OldNo: 1}}},
		}}}

		got := view(press(t, loaded(t, src), "f"))

		assertContains(t, got, "file deleted")
		if strings.Contains(got, "bye") {
			t.Error("View() in full mode shows the deleted file's lines")
		}
	})

	t.Run("space selects the hunk under the cursor", func(t *testing.T) {
		m := press(t, loaded(t, newSource()), "tab", "j", "tab", "space")

		got := view(m)

		assertContains(t, got, "▾ ● @@ -3,2 +3,2 @@", "1 hunk selected", "◐ main.go")
	})

	t.Run("space twice clears the selection", func(t *testing.T) {
		m := press(t, loaded(t, newSource()), "tab", "j", "tab", "space", "space")

		assertContains(t, view(m), "○ main.go", "nothing selected")
	})

	t.Run("s does not select", func(t *testing.T) {
		m := press(t, loaded(t, newSource()), "tab", "j", "tab", "s")

		assertContains(t, view(m), "○ main.go", "nothing selected")
	})

	t.Run("S selects every hunk of the file then clears them", func(t *testing.T) {
		m := press(t, loaded(t, newSource()), "tab", "j", "tab", "S")

		assertContains(t, view(m), "● main.go", "2 hunks selected")

		m = press(t, m, "S")

		assertContains(t, view(m), "○ main.go", "nothing selected")
	})

	t.Run("S on a file without hunks selects it as one unit", func(t *testing.T) {
		m := press(t, loaded(t, newSource()), "tab", "j", "j", "j", "tab", "space")

		assertContains(t, view(m), "○ img.png", "nothing selected")

		m = press(t, m, "S")

		assertContains(t, view(m), "● img.png", "1 hunk selected")
	})
}

func TestFullMode_success(t *testing.T) {
	t.Run("f requests the file at the commit and switches the mode chip", func(t *testing.T) {
		src := newSource()
		src.contents["commit1:src/ui/view.go"] = "package ui\nfunc View() {}\n"

		m := press(t, loaded(t, src), "f")

		if !m.FullFile() {
			t.Error("FullFile() = false, want true")
		}
		if !slices.Contains(src.requested, "commit1:src/ui/view.go") {
			t.Errorf("requested = %v, want commit1:src/ui/view.go", src.requested)
		}
		assertContains(t, view(m), "full file")
	})

	t.Run("shows the file lines without hunk headers", func(t *testing.T) {
		src := newSource()
		src.contents["commit1:src/ui/view.go"] = "package ui\nfunc View() {}\n"

		got := view(press(t, loaded(t, src), "f"))

		assertContains(t, got, "+ package ui", "+ func View() {}")
		if strings.Contains(got, "@@") {
			t.Error("View() in full mode shows a hunk header")
		}
	})

	t.Run("shows the resulting file without removed lines", func(t *testing.T) {
		src := newSource()
		src.contents["commit1:README.md"] = "# jjreview\n\nTUI for jujutsu\n"

		got := view(press(t, loaded(t, src), "tab", "j", "j", "tab", "f"))

		assertContains(t, got, "# jjreview", "+ TUI for jujutsu")
		if strings.Contains(got, "TUI for git") {
			t.Error("View() in full mode shows the removed line")
		}
	})

	t.Run("says a deleted file has no resulting content", func(t *testing.T) {
		src := newSource()
		src.files = []diff.File{{OldPath: "gone.txt", OldMode: "100644", Status: diff.StatusDeleted, Hunks: []diff.Hunk{
			{OldStart: 1, OldLines: 1, Lines: []diff.Line{{Kind: diff.LineRemoved, Text: "bye", OldNo: 1}}},
		}}}

		got := view(press(t, loaded(t, src), "f"))

		assertContains(t, got, "file deleted")
		if strings.Contains(got, "bye") {
			t.Error("View() in full mode shows the deleted file's lines")
		}
	})

	t.Run("space selects the hunk under the cursor", func(t *testing.T) {
		src := newSource()
		src.contents["commit1:src/ui/view.go"] = "package ui\nfunc View() {}\n"

		got := view(press(t, loaded(t, src), "f", "space"))

		assertContains(t, got, "1 hunk selected")
	})

	t.Run("caches content per file", func(t *testing.T) {
		src := newSource()

		press(t, loaded(t, src), "f", "f", "f")

		if got := len(src.requested); got != 1 {
			t.Errorf("requested = %v, want one request", src.requested)
		}
	})

	t.Run("f before load requests content once loaded", func(t *testing.T) {
		src := newSource()
		m := tui.New(t.Context(), src, nil, "@")
		m, _ = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 30})
		init := m.Init()

		m = press(t, m, "f")
		run(t, m, init)

		if !slices.Equal(src.requested, []string{"commit1:src/ui/view.go"}) {
			t.Errorf("requested = %v, want [commit1:src/ui/view.go]", src.requested)
		}
	})

	t.Run("cached content adds the trailing gap in changes mode", func(t *testing.T) {
		src := newSource()
		src.contents["commit1:src/main.go"] = strings.Repeat("line\n", 15)

		m := press(t, loaded(t, src), "tab", "j", "tab", "f", "f")

		assertContains(t, view(m), "⋯ 4 unchanged lines")
	})
}

func TestFullMode_failed(t *testing.T) {
	src := newSource()
	src.contentErr["commit1:src/ui/view.go"] = errors.New("disk gone")

	m := press(t, loaded(t, src), "f")

	assertContains(t, view(m), "disk gone")

	delete(src.contentErr, "commit1:src/ui/view.go")
	m = press(t, m, "f", "f")

	if got := view(m); strings.Contains(got, "disk gone") {
		t.Errorf("View() still shows the old error after retry:\n%s", got)
	}
	if got := len(src.requested); got != 2 {
		t.Errorf("requested = %v, want a retry", src.requested)
	}
}

func TestFullMode_escapedError(t *testing.T) {
	src := newSource()
	f := viewFile()
	f.NewPath = "src/ui/a\nb.go"
	src.files = []diff.File{f}
	src.contentErr["commit1:src/ui/a\nb.go"] = errors.New("open src/ui/a\nb.go: no such file")

	m := press(t, loaded(t, src), "f")

	assertContains(t, view(m), `error: read src/ui/a\nb.go: open src/ui/a\nb.go: no such file`)
	if got, want := src.requested, []string{"commit1:src/ui/a\nb.go"}; !slices.Equal(got, want) {
		t.Errorf("requested = %q, want %q", got, want)
	}
}

func TestFullMode_staleError(t *testing.T) {
	src := newSource()
	src.contentErr["commit1:src/ui/view.go"] = errors.New("disk gone")
	m, stale := update(t, loaded(t, src), keyMsg("f"))
	m = press(t, m, "tab", "j", "tab", "j", "j")
	want := m.Cursor()

	m = run(t, m, stale)

	if got := m.Cursor(); got != want {
		t.Errorf("Cursor() = %d after a stale content error, want %d", got, want)
	}
}

func TestLayout_success(t *testing.T) {
	src := newSource()
	src.files = nil
	for i := range 40 {
		src.files = append(src.files, diff.File{NewPath: fmt.Sprintf("f%02d.go", i), OldPath: fmt.Sprintf("f%02d.go", i), Binary: true})
	}
	m := start(t, src, nil, 80, 12)

	m = press(t, m, slices.Concat([]string{"tab"}, slices.Repeat([]string{"j"}, 39))...)

	got := view(m)
	if lines := strings.Count(got, "\n") + 1; lines > 12 {
		t.Errorf("View() has %d lines, want at most 12", lines)
	}
	assertContains(t, got, "f39.go")
	if strings.Contains(got, "f00.go") {
		t.Errorf("View() still shows the first file:\n%s", got)
	}
}

func TestFilesPaneWidth_success(t *testing.T) {
	long := "a_really_long_file_name_that_used_to_be_cut.go"
	longFile := func() diff.File {
		return diff.File{OldPath: "internal/" + long, NewPath: "internal/" + long, Binary: true}
	}

	t.Run("grows to fit the longest file row", func(t *testing.T) {
		src := newSource()
		src.files = append(src.files, longFile())
		m := start(t, src, nil, 160, 30)

		line := lineWith(t, view(m), "○ "+long)

		assertContains(t, line, "○ "+long+" bin │")
	})

	t.Run("shrinks to fit short file rows", func(t *testing.T) {
		src := newSource()
		src.files = []diff.File{imageFile()}
		m := start(t, src, nil, 160, 30)

		got := view(m)

		assertContains(t, lineWith(t, got, "○ img.png"), "│▌○ img.png bin │")
		if col := diffCardColumn(t, got); col != 18 {
			t.Errorf("diff card starts at column %d, want 18", col)
		}
	})

	t.Run("reserves room for every file being done when the header is the widest row", func(t *testing.T) {
		src := newSource()
		src.files = nil
		for i := range 10 {
			name := fmt.Sprintf("a%d.go", i)
			src.files = append(src.files, diff.File{OldPath: name, NewPath: name, Binary: true})
		}
		m := start(t, src, nil, 160, 30)

		got := view(m)

		assertContains(t, got, "0/10 ✓")
		if col := diffCardColumn(t, got); col != 18 {
			t.Errorf("diff card starts at column %d, want 18 to fit 10/10 ✓", col)
		}
	})

	t.Run("keeps at least half of the screen for the diff", func(t *testing.T) {
		src := newSource()
		src.files = append(src.files, longFile())
		m := start(t, src, nil, 80, 30)

		if got := diffCardColumn(t, view(m)); got != 40 {
			t.Errorf("diff card starts at column %d, want 40", got)
		}
	})

	t.Run("caps its width on wide screens", func(t *testing.T) {
		src := newSource()
		name := strings.Repeat("vendor/", 10) + long
		src.files = append(src.files, diff.File{OldPath: name, NewPath: name, Binary: true})
		m := start(t, src, nil, 300, 30)

		if got := diffCardColumn(t, view(m)); got != 61 {
			t.Errorf("diff card starts at column %d, want 61", got)
		}
	})
}

func diffCardColumn(t *testing.T, screen string) int {
	t.Helper()
	border := lineWith(t, screen, "╭")
	return utf8.RuneCountInString(border[:strings.LastIndex(border, "╭")])
}

func TestKeyBar_success(t *testing.T) {
	t.Run("keeps only the essential diff keys", func(t *testing.T) {
		m := start(t, newSource(), nil, 60, 30)

		got := view(m)

		assertContains(t, got, "tab pane", "j k line", "space select", "c commit", "d diff", "e edit", "? help", "file 1/4 · 1 hunk")
		for _, hint := range []string{"half page", "fold", "S select file", "full / changes"} {
			if strings.Contains(got, hint) {
				t.Errorf("View() key bar shows %q", hint)
			}
		}
	})

	t.Run("lists file keys when the file pane is focused", func(t *testing.T) {
		m := press(t, start(t, newSource(), nil, 60, 30), "tab")

		got := view(m)

		assertContains(t, got, "tab pane", "j k file", "space select", "c commit", "e edit", "? help", "file 1/4 · 1 hunk")
		for _, hint := range []string{"j k line", "d diff"} {
			if strings.Contains(got, hint) {
				t.Errorf("View() with the file pane focused shows %q", hint)
			}
		}
	})
}

func TestKeyBar_notWorkingCopy(t *testing.T) {
	m := press(t, loaded(t, revisionSource()), reviewParent()...)

	assertContains(t, view(m), "? help")
	assertMissing(t, view(m), "e edit")
	assertMissing(t, view(press(t, m, "?")), "open in $EDITOR")
}

func TestHelp_success(t *testing.T) {
	t.Run("? opens a popup with every key", func(t *testing.T) {
		m := press(t, start(t, newSource(), nil, 120, 50), "?")

		got := view(m)

		assertContains(t, got, "Keys", "^d ^u", "half page", "@", "working-copy revision", "S", "select every hunk in the file",
			"Z", "fold all hunks", "R", "reload the change", "open in $EDITOR", "full file / changes only", "q", "quit")
	})

	t.Run("lists the jump keys", func(t *testing.T) {
		got := view(press(t, start(t, newSource(), nil, 120, 40), "?"))

		assertContains(t, got, "tab", "switch pane", "g G", "first / last line or file", "] [", "next / previous hunk", "J K", "next / previous file")
	})

	t.Run("scrolls with j and k when taller than the screen", func(t *testing.T) {
		m := press(t, start(t, newSource(), nil, 100, 20), "?")

		assertContains(t, view(m), "switch pane", "j k scroll")
		assertMissing(t, view(m), "quit")

		m = press(t, m, slices.Repeat([]string{"j"}, 30)...)

		assertContains(t, view(m), "quit")
		assertMissing(t, view(m), "switch pane")

		m = press(t, m, slices.Repeat([]string{"k"}, 30)...)

		assertContains(t, view(m), "switch pane")
	})

	t.Run("fits a terminal shorter than its chrome", func(t *testing.T) {
		got := view(press(t, start(t, newSource(), nil, 100, 4), "?"))

		if n := strings.Count(got, "\n") + 1; n > 4 {
			t.Errorf("View() is %d lines tall, want at most 4", n)
		}
		assertContains(t, got, "╰")
	})

	t.Run("does not scroll when it fits", func(t *testing.T) {
		got := view(press(t, start(t, newSource(), nil, 120, 50), "?", "j"))

		assertContains(t, got, "switch pane", "quit")
		assertMissing(t, got, "j k scroll")
	})

	for _, k := range []string{"?", "esc", "q"} {
		t.Run(k+" closes the popup without quitting", func(t *testing.T) {
			m := press(t, loaded(t, newSource()), "?")

			next, cmd := update(t, m, keyMsg(k))

			if cmd != nil {
				t.Errorf("Update(%s) cmd = non-nil, want nil", k)
			}
			if strings.Contains(view(next), "fold all hunks") {
				t.Errorf("View() after %s still shows the help popup", k)
			}
		})
	}

	t.Run("ignores review keys while open", func(t *testing.T) {
		m := press(t, loaded(t, newSource()), "?", "space", "j")

		assertContains(t, view(m), "nothing selected", "fold all hunks")
	})

	t.Run("hides select and commit keys for an immutable revision", func(t *testing.T) {
		src := newSource()
		src.change.Immutable = true

		got := view(press(t, loaded(t, src), "?"))

		assertContains(t, got, "fold all hunks")
		for _, hint := range []string{"select every hunk", "commit the selected"} {
			if strings.Contains(got, hint) {
				t.Errorf("help popup of an immutable revision shows %q", hint)
			}
		}
	})
}

func TestQuit_success(t *testing.T) {
	for _, k := range []string{"q", "ctrl+c"} {
		t.Run(k+" quits", func(t *testing.T) {
			m := loaded(t, newSource())

			_, cmd := update(t, m, keyMsg(k))

			if cmd == nil {
				t.Fatalf("Update(%s) cmd = nil, want tea.Quit", k)
			}
			if _, ok := cmd().(tea.QuitMsg); !ok {
				t.Errorf("Update(%s) cmd() = %T, want tea.QuitMsg", k, cmd())
			}
		})
	}
}

func bigSource(lines int) *fakeSource {
	h := diff.Hunk{NewStart: 1, NewLines: lines}
	for i := range lines {
		h.Lines = append(h.Lines, diff.Line{Kind: diff.LineAdded, Text: fmt.Sprintf("line %d", i+1), NewNo: i + 1})
	}
	src := newSource()
	src.files = []diff.File{{NewPath: "big.txt", Status: diff.StatusAdded, Hunks: []diff.Hunk{h}}}
	return src
}

func TestHalfPage_success(t *testing.T) {
	t.Run("ctrl+d moves the diff cursor by half a page", func(t *testing.T) {
		m := start(t, bigSource(100), nil, 80, 24)
		before := m.Cursor()

		m = press(t, m, "ctrl+d")

		if moved := m.Cursor() - before; moved < 5 || moved > 12 {
			t.Errorf("ctrl+d moved the cursor %d rows, want about half of a 24-row window", moved)
		}
	})

	t.Run("ctrl+d scrolls the view along with the cursor", func(t *testing.T) {
		m := start(t, bigSource(100), nil, 80, 24)

		m = press(t, m, "ctrl+d")

		if regexp.MustCompile(`\+ line 1\s*│`).MatchString(view(m)) {
			t.Error("View() after ctrl+d still shows line 1, want the view scrolled down")
		}
	})

	t.Run("ctrl+u undoes ctrl+d", func(t *testing.T) {
		m := start(t, bigSource(100), nil, 80, 24)
		before := m.Cursor()

		m = press(t, m, "ctrl+d", "ctrl+d", "ctrl+u", "ctrl+u")

		if got := m.Cursor(); got != before {
			t.Errorf("Cursor() = %d, want %d", got, before)
		}
	})

	t.Run("ctrl+d clamps at the last row and ctrl+u at the first", func(t *testing.T) {
		m := start(t, bigSource(100), nil, 80, 24)

		m = press(t, m, slices.Repeat([]string{"ctrl+d"}, 30)...)
		if got := m.Cursor(); got != 100 {
			t.Errorf("Cursor() after many ctrl+d = %d, want 100", got)
		}

		m = press(t, m, slices.Repeat([]string{"ctrl+u"}, 30)...)
		if got := m.Cursor(); got != 0 {
			t.Errorf("Cursor() after many ctrl+u = %d, want 0", got)
		}
	})

	t.Run("ctrl+d and ctrl+u move between files in the file pane", func(t *testing.T) {
		src := newSource()
		src.files = nil
		for i := range 40 {
			src.files = append(src.files, diff.File{NewPath: fmt.Sprintf("f%02d.txt", i), Status: diff.StatusAdded})
		}
		m := start(t, src, nil, 80, 24)

		m = press(t, m, "tab", "ctrl+d")
		down := m.CurrentFile()
		m = press(t, m, "ctrl+u")

		if down == "f00.txt" || down == "f01.txt" {
			t.Errorf("CurrentFile() after ctrl+d = %q, want several files down", down)
		}
		if got := m.CurrentFile(); got != "f00.txt" {
			t.Errorf("CurrentFile() after ctrl+d ctrl+u = %q, want f00.txt", got)
		}
	})
}

func TestHunkHeader_success(t *testing.T) {
	m := press(t, loaded(t, newSource()), "tab", "j", "tab", "j")

	raw := m.View().Content

	if !regexp.MustCompile(`\x1b\[([0-9]+;)*100(;[0-9]+)*m`).MatchString(raw) {
		t.Error("View() has no bright-black (SGR 100) background for hunk headers")
	}
}
