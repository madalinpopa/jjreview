package tui_test

import (
	"strings"
	"testing"

	"github.com/madalinpopa/jjreview/internal/diff"
)

func assertMissing(t *testing.T, got string, unwanted ...string) {
	t.Helper()
	for _, u := range unwanted {
		if strings.Contains(got, u) {
			t.Errorf("View() shows %q:\n%s", u, got)
		}
	}
}

func trimSource() *fakeSource {
	src := newSource()
	src.files = []diff.File{{OldPath: "src/trim.go", NewPath: "src/trim.go", OldMode: "100644", NewMode: "100644", Hunks: []diff.Hunk{
		{OldStart: 2, OldLines: 2, NewStart: 1, NewLines: 0, Lines: []diff.Line{
			{Kind: diff.LineRemoved, Text: "drop a", OldNo: 2},
			{Kind: diff.LineRemoved, Text: "drop b", OldNo: 3},
		}},
	}}}
	src.contents["commit1:src/trim.go"] = "a\nz\n"
	return src
}

func TestDiffView_success(t *testing.T) {
	t.Run("hides removed lines by default", func(t *testing.T) {
		m := press(t, loaded(t, newSource()), "ctrl+h", "j", "ctrl+l")

		got := view(m)

		assertContains(t, got, "@@ -3,2 +3,2 @@", "keep", "+ new line")
		assertMissing(t, got, "old line")
	})

	t.Run("d shows the removed lines of the hunk under the cursor", func(t *testing.T) {
		m := press(t, loaded(t, newSource()), "ctrl+h", "j", "ctrl+l", "d")

		assertContains(t, view(m), "- old line", "+ new line")
	})

	t.Run("d twice hides them again", func(t *testing.T) {
		m := press(t, loaded(t, newSource()), "ctrl+h", "j", "ctrl+l", "d", "d")

		assertMissing(t, view(m), "old line")
	})

	t.Run("d keeps the cursor on the same line", func(t *testing.T) {
		m := press(t, loaded(t, newSource()), "ctrl+h", "j", "ctrl+l")
		before := m.Cursor()

		m = press(t, m, "d")

		if got := m.Cursor(); got != before+1 {
			t.Errorf("Cursor() = %d, want %d, still on the new line below the shown removed line", got, before+1)
		}
	})

	t.Run("d leaves other hunks alone", func(t *testing.T) {
		m := press(t, loaded(t, newSource()), "ctrl+h", "j", "ctrl+l", "j", "j", "j", "d")

		assertMissing(t, view(m), "old line")
	})

	t.Run("d does nothing in the file pane", func(t *testing.T) {
		m := press(t, loaded(t, newSource()), "ctrl+h", "j", "d")

		assertMissing(t, view(m), "old line")
	})

	t.Run("d shows removed lines in full file mode", func(t *testing.T) {
		m := press(t, loaded(t, newSource()), "ctrl+h", "j", "ctrl+l", "f")
		assertMissing(t, view(m), "old line")

		m = press(t, m, "d")

		assertContains(t, view(m), "- old line", "+ new line")
	})

	t.Run("shows the lines of a deleted file", func(t *testing.T) {
		src := newSource()
		src.files = []diff.File{{OldPath: "gone.txt", OldMode: "100644", Status: diff.StatusDeleted, Hunks: []diff.Hunk{
			{OldStart: 1, OldLines: 1, NewStart: 0, NewLines: 0, Lines: []diff.Line{{Kind: diff.LineRemoved, Text: "bye", OldNo: 1}}},
		}}}

		assertContains(t, view(loaded(t, src)), "- bye")
	})

	t.Run("shows the lines of a hunk that only deletes", func(t *testing.T) {
		m := loaded(t, trimSource())

		assertContains(t, view(m), "- drop a", "- drop b")
	})

	t.Run("shows the lines of a hunk that only deletes in full file mode", func(t *testing.T) {
		m := press(t, loaded(t, trimSource()), "f")

		assertContains(t, view(m), "a", "- drop a", "- drop b", "z")
	})

	t.Run("d does nothing on a hunk that only deletes", func(t *testing.T) {
		m := press(t, loaded(t, trimSource()), "d")

		assertContains(t, view(m), "- drop a", "- drop b")
	})

	t.Run("d does nothing on a folded hunk", func(t *testing.T) {
		m := press(t, loaded(t, newSource()), "ctrl+h", "j", "ctrl+l", "z", "d", "enter")

		assertMissing(t, view(m), "old line")
	})

	t.Run("d shows removed lines of a folded hunk in full file mode", func(t *testing.T) {
		m := press(t, loaded(t, newSource()), "ctrl+h", "j", "ctrl+l", "z", "f", "d")

		assertContains(t, view(m), "- old line", "+ new line")

		m = press(t, m, "d")

		assertMissing(t, view(m), "old line")
	})

	t.Run("hiding removed lines moves the cursor to the next line still shown", func(t *testing.T) {
		m := press(t, loaded(t, newSource()), "ctrl+h", "j", "ctrl+l", "d", "k", "d")

		if got := m.Cursor(); got != 3 {
			t.Errorf("Cursor() = %d, want 3, on the new line after the hidden removed line", got)
		}
	})

	t.Run("hiding trailing removed lines moves the cursor to the line shown before them", func(t *testing.T) {
		src := newSource()
		src.files = []diff.File{{OldPath: "tail.go", NewPath: "tail.go", OldMode: "100644", NewMode: "100644", Hunks: []diff.Hunk{
			{OldStart: 1, OldLines: 2, NewStart: 1, NewLines: 2, Lines: []diff.Line{
				{Kind: diff.LineAdded, Text: "new", NewNo: 1},
				{Kind: diff.LineContext, Text: "keep", OldNo: 1, NewNo: 2},
				{Kind: diff.LineRemoved, Text: "old", OldNo: 2},
			}},
		}}}
		m := press(t, loaded(t, src), "d", "j", "j", "j", "j", "d")

		if got := m.Cursor(); got != 2 {
			t.Errorf("Cursor() = %d, want 2, on the line before the hidden removed line", got)
		}
	})

	t.Run("d in the file pane points to the diff pane", func(t *testing.T) {
		m := press(t, loaded(t, newSource()), "ctrl+h", "j", "d")

		assertContains(t, view(m), "d works in the diff pane; press l")
		assertMissing(t, view(m), "old line")
	})

	t.Run("help lists d", func(t *testing.T) {
		got := view(press(t, loaded(t, newSource()), "?"))

		assertContains(t, got, "show removed lines of the hunk")
	})
}

func BenchmarkToggleRemoved(b *testing.B) {
	const n = 32000
	lines := make([]diff.Line, 0, 2*n)
	for i := range n {
		lines = append(lines, diff.Line{Kind: diff.LineContext, Text: "same", OldNo: i + 1, NewNo: i + 1})
	}
	for i := range n {
		lines = append(lines, diff.Line{Kind: diff.LineRemoved, Text: "gone", OldNo: n + i + 1})
	}
	src := newSource()
	src.files = []diff.File{{OldPath: "big.go", NewPath: "big.go", OldMode: "100644", NewMode: "100644", Hunks: []diff.Hunk{
		{OldStart: 1, OldLines: 2 * n, NewStart: 1, NewLines: n, Lines: lines},
	}}}
	m := start(b, src, &fakeGenerator{}, 120, 4*n)
	m, _ = update(b, m, keyMsg("ctrl+d"))

	for b.Loop() {
		for _, k := range []string{"d", "j", "d"} {
			m, _ = update(b, m, keyMsg(k))
		}
	}
	if got := m.Cursor(); got != n {
		b.Fatalf("Cursor() = %d, want %d", got, n)
	}
}
