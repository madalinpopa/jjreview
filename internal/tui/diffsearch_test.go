package tui_test

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/madalinpopa/jjreview/internal/diff"
	"github.com/madalinpopa/jjreview/internal/tui"
)

func searchFile(path string, lines ...diff.Line) diff.File {
	added, old := 0, 0
	for _, l := range lines {
		if l.Kind != diff.LineRemoved {
			added++
		}
		if l.Kind != diff.LineAdded {
			old++
		}
	}
	return diff.File{OldPath: path, NewPath: path, OldMode: "100644", NewMode: "100644", Hunks: []diff.Hunk{
		{OldStart: 1, OldLines: old, NewStart: 1, NewLines: added, Lines: lines},
	}}
}

func searchSource() *fakeSource {
	src := newSource()
	src.files = []diff.File{
		searchFile("a.txt",
			diff.Line{Kind: diff.LineContext, Text: "alpha", OldNo: 1, NewNo: 1},
			diff.Line{Kind: diff.LineRemoved, Text: "old Foo", OldNo: 2},
			diff.Line{Kind: diff.LineAdded, Text: "new Foo fOO", NewNo: 2},
			diff.Line{Kind: diff.LineAdded, Text: "bar", NewNo: 3},
		),
		searchFile("b.txt",
			diff.Line{Kind: diff.LineContext, Text: "same", OldNo: 1, NewNo: 1},
			diff.Line{Kind: diff.LineAdded, Text: "a.b", NewNo: 2},
		),
		searchFile("c.txt",
			diff.Line{Kind: diff.LineAdded, Text: "zzz", NewNo: 1},
			diff.Line{Kind: diff.LineContext, Text: "keep", OldNo: 1, NewNo: 2},
			diff.Line{Kind: diff.LineAdded, Text: "foo again", NewNo: 3},
		),
	}
	src.contents["commit1:a.txt"] = "alpha\nnew Foo fOO\nbar\n"
	src.contents["commit1:b.txt"] = "same\na.b\n"
	src.contents["commit1:c.txt"] = "zzz\nkeep\nfoo again\n"
	return src
}

func findKeys(query string) []string {
	return append(append([]string{"/"}, typeKeys(query)...), "enter")
}

func onSearch(t *testing.T) tui.Model {
	t.Helper()
	m := loaded(t, searchSource())
	if got := m.CurrentFile(); got != "a.txt" {
		t.Fatalf("CurrentFile() = %q, want a.txt", got)
	}
	return m
}

func assertAt(t *testing.T, m tui.Model, file string, cursor int) {
	t.Helper()
	if got := m.CurrentFile(); got != file {
		t.Errorf("CurrentFile() = %q, want %q", got, file)
	}
	if got := m.Cursor(); got != cursor {
		t.Errorf("Cursor() = %d, want %d", got, cursor)
	}
}

func TestDiffSearch_success(t *testing.T) {
	t.Run("enter jumps to the first match and shows its position", func(t *testing.T) {
		m := press(t, onSearch(t), "j")

		m = press(t, m, findKeys("foo")...)

		assertAt(t, m, "c.txt", 3)
		assertContains(t, view(m), "/foo 1/1")
	})

	t.Run("enter counts the match under the cursor", func(t *testing.T) {
		m := press(t, onSearch(t), findKeys("foo")...)

		assertAt(t, m, "a.txt", 2)
		assertContains(t, view(m), "/foo 1/2")
	})

	t.Run("ignores case", func(t *testing.T) {
		m := press(t, onSearch(t), findKeys("FOO")...)

		assertAt(t, m, "a.txt", 2)
		assertContains(t, view(m), "/FOO 1/2")
	})

	t.Run("matches the query literally", func(t *testing.T) {
		m := press(t, onSearch(t), findKeys(".")...)

		assertAt(t, m, "b.txt", 2)
		assertContains(t, view(m), "/. 1/1")
	})

	t.Run("n moves to the next match on the same line", func(t *testing.T) {
		m := press(t, onSearch(t), findKeys("foo")...)

		m = press(t, m, "n")

		assertAt(t, m, "a.txt", 2)
		assertContains(t, view(m), "/foo 2/2")
	})

	t.Run("n after the last match goes to the next file with a match", func(t *testing.T) {
		m := press(t, onSearch(t), findKeys("foo")...)

		m = press(t, m, "n", "n")

		assertAt(t, m, "c.txt", 3)
		assertContains(t, view(m), "/foo 1/1", "file 3/3")
	})

	t.Run("n after the last file wraps to the first", func(t *testing.T) {
		m := press(t, onSearch(t), findKeys("foo")...)

		m = press(t, m, "n", "n", "n")

		assertAt(t, m, "a.txt", 2)
		assertContains(t, view(m), "/foo 1/2")
	})

	t.Run("N goes back to the previous file with a match", func(t *testing.T) {
		m := press(t, onSearch(t), findKeys("foo")...)

		m = press(t, m, "n", "n", "N")

		assertAt(t, m, "a.txt", 2)
		assertContains(t, view(m), "/foo 2/2")
	})

	t.Run("N before the first match wraps to the last file", func(t *testing.T) {
		m := press(t, onSearch(t), findKeys("foo")...)

		m = press(t, m, "N")

		assertAt(t, m, "c.txt", 3)
	})

	t.Run("n works from the file pane and the tree follows", func(t *testing.T) {
		m := press(t, onSearch(t), findKeys("foo")...)

		m = press(t, m, "tab", "n", "n")

		assertAt(t, m, "c.txt", 3)
		if !m.FilesFocused() {
			t.Error("FilesFocused() = false, want the file pane kept")
		}
	})

	t.Run("n moves on from the cursor after moving it", func(t *testing.T) {
		m := press(t, onSearch(t), findKeys("foo")...)

		m = press(t, m, "j", "n")

		assertAt(t, m, "c.txt", 3)
	})

	t.Run("another file counts its matches", func(t *testing.T) {
		m := press(t, onSearch(t), findKeys("foo")...)

		m = press(t, m, "J")

		assertContains(t, view(m), "/foo no matches")
		assertContains(t, view(press(t, m, "J")), "/foo 1 match")
	})

	t.Run("the full file is searched", func(t *testing.T) {
		m := press(t, onSearch(t), "f")

		m = press(t, m, findKeys("keep")...)

		assertAt(t, m, "c.txt", 1)
		assertContains(t, view(m), "/keep 1/1")
	})

	t.Run("the full file of the next match is loaded first", func(t *testing.T) {
		m := press(t, onSearch(t), "f")
		m = press(t, m, findKeys("foo")...)

		m = press(t, m, "n", "n")

		assertAt(t, m, "c.txt", 2)
		assertContains(t, view(m), "/foo 1/1")
	})

	t.Run("pasted text goes to the query", func(t *testing.T) {
		m := press(t, onSearch(t), "j", "/")

		m = run(t, m, func() tea.Msg { return tea.PasteMsg{Content: "FOO"} })
		m = press(t, m, "enter")

		assertAt(t, m, "c.txt", 3)
	})

	t.Run("typed keys go to the query instead of running commands", func(t *testing.T) {
		m := press(t, onSearch(t), "/", "q", "2", "?", "j", "n")

		assertAt(t, m, "a.txt", 2)
		assertContains(t, view(m), "/ q2?jn", "UNIFIED DIFF")
	})

	t.Run("the query shows while typing", func(t *testing.T) {
		m := press(t, onSearch(t), "/", "f", "o")

		assertContains(t, view(m), "/ fo", "enter search", "esc cancel")
	})

	t.Run("the bar shows the match keys once a search is active", func(t *testing.T) {
		m := onSearch(t)

		assertContains(t, view(press(t, m, findKeys("foo")...)), "n N match")
		assertMissing(t, view(m), "n N match")
	})

	t.Run("the help lists the keys", func(t *testing.T) {
		assertContains(t, view(press(t, start(t, newSource(), nil, 120, 60), "?")), "search the diff; next / previous match")
	})
}

func TestDiffSearch_visible(t *testing.T) {
	t.Run("scrolls down to a match below the window", func(t *testing.T) {
		m := start(t, bigSource(100), nil, 80, 24)

		m = press(t, m, findKeys("line 90")...)

		lineWith(t, view(m), "line 90")
	})

	t.Run("scrolls right to a match past the pane edge", func(t *testing.T) {
		m := onWide(t, wideSource())

		m = press(t, m, findKeys("a190")...)

		lineWith(t, view(m), "a190")
	})

	t.Run("scrolls back left to a match before the pane edge", func(t *testing.T) {
		m := scrolledEnd(t, onWide(t, wideSource()))

		m = press(t, m, findKeys("a005")...)

		lineWith(t, view(m), "a005")
	})
}

func TestDiffSearch_removedLines(t *testing.T) {
	t.Run("hidden removed lines do not match", func(t *testing.T) {
		m := press(t, onSearch(t), findKeys("old")...)

		assertAt(t, m, "a.txt", 2)
		assertContains(t, view(m), "/old no matches")
	})

	t.Run("shown removed lines match", func(t *testing.T) {
		m := press(t, onSearch(t), findKeys("old")...)

		m = press(t, m, "d", "n")

		assertAt(t, m, "a.txt", 2)
		assertContains(t, view(m), "/old 1/1", "old Foo")
	})
}

func TestDiffSearch_highlight(t *testing.T) {
	t.Run("marks every match and the current one apart", func(t *testing.T) {
		m := press(t, onSearch(t), findKeys("foo")...)

		raw := rawLineWith(t, m, "new Foo fOO")
		current, other, plain := sgrBefore(t, raw, "Foo"), sgrBefore(t, raw, "fOO"), sgrBefore(t, raw, "new")

		if current == other {
			t.Errorf("SGR of the current match = %q, want it apart from the other match", current)
		}
		if other == plain || current == plain {
			t.Errorf("SGR of the matches = %q and %q, want them apart from the plain text %q", current, other, plain)
		}
	})

	t.Run("n moves the current mark", func(t *testing.T) {
		m := press(t, onSearch(t), findKeys("foo")...)
		before := sgrBefore(t, rawLineWith(t, m, "new Foo fOO"), "Foo")

		m = press(t, m, "n")

		raw := rawLineWith(t, m, "new Foo fOO")
		if got := sgrBefore(t, raw, "fOO"); got != before {
			t.Errorf("SGR of the second match after n = %q, want the current mark %q", got, before)
		}
	})
}

func TestDiffSearch_cancel(t *testing.T) {
	t.Run("esc while typing closes the query", func(t *testing.T) {
		m := press(t, onSearch(t), "/", "f", "o", "o", "esc")

		got := view(m)
		assertMissing(t, got, "/ foo", "/foo")
		assertAt(t, m, "a.txt", 2)
	})

	t.Run("esc while typing keeps the active search", func(t *testing.T) {
		m := press(t, onSearch(t), findKeys("foo")...)

		m = press(t, m, "/", "b", "a", "r", "esc")

		assertContains(t, view(m), "/foo 1/2")
	})

	t.Run("esc clears the active search", func(t *testing.T) {
		m := press(t, onSearch(t), findKeys("foo")...)

		m = press(t, m, "esc")

		assertMissing(t, view(m), "/foo", "n N match")
	})

	t.Run("esc in a range cancels the range and keeps the search", func(t *testing.T) {
		m := press(t, onSearch(t), findKeys("foo")...)

		m = press(t, m, "v", "esc")

		got := view(m)
		assertContains(t, got, "/foo 1/2")
		assertMissing(t, got, "selecting a range")
	})

	t.Run("an empty query clears the search", func(t *testing.T) {
		m := press(t, onSearch(t), findKeys("foo")...)

		m = press(t, m, "/", "enter")

		assertMissing(t, view(m), "/foo")
	})

	t.Run("enter does not fold the hunk", func(t *testing.T) {
		m := press(t, onSearch(t), findKeys("foo")...)

		assertMissing(t, view(m), "folded")
	})
}

func TestDiffSearch_failed(t *testing.T) {
	t.Run("n without a search names the key", func(t *testing.T) {
		m := press(t, onSearch(t), "n")

		assertContains(t, view(m), "press / to search")
		assertAt(t, m, "a.txt", 2)
	})
}
