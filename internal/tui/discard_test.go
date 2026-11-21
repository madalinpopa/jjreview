package tui_test

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/madalinpopa/jjreview/internal/tui"
)

func quits(t *testing.T, m tui.Model, k string) bool {
	t.Helper()
	_, cmd := update(t, m, keyMsg(k))
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

func TestDiscard_quit(t *testing.T) {
	t.Run("q with selected hunks asks before quitting", func(t *testing.T) {
		m := press(t, loaded(t, newSource()), "space")

		if quits(t, m, "q") {
			t.Fatal("q quit with a selected hunk, want a warning first")
		}
		m = press(t, m, "q")
		assertContains(t, view(m), "press q again to drop 1 selected hunk", "1 hunk selected")
		if !quits(t, m, "q") {
			t.Error("second q did not quit")
		}
	})

	t.Run("another key cancels the pending quit", func(t *testing.T) {
		m := press(t, loaded(t, newSource()), "space", "q", "j")

		if quits(t, m, "q") {
			t.Error("q after j quit, want a new warning")
		}
	})

	t.Run("ctrl+c quits with selected hunks", func(t *testing.T) {
		m := press(t, loaded(t, newSource()), "space")

		if !quits(t, m, "ctrl+c") {
			t.Error("ctrl+c did not quit")
		}
	})
}

func TestDiscard_load(t *testing.T) {
	for _, tc := range []struct{ k, label string }{{"@", "@"}, {"R", "R"}} {
		k, label := tc.k, tc.label
		t.Run(k+" with selected hunks asks before loading", func(t *testing.T) {
			src := revisionSource()
			m := press(t, loaded(t, src), "space")
			calls := len(src.changeRevs)

			m = press(t, m, k)

			if got := len(src.changeRevs); got != calls {
				t.Fatalf("Change() calls = %d after one %s, want %d", got, k, calls)
			}
			assertContains(t, view(m), "press "+label+" again to drop 1 selected hunk", "1 hunk selected")

			m = press(t, m, k)

			if got := len(src.changeRevs); got != calls+1 {
				t.Errorf("Change() calls = %d after two %s, want %d", got, k, calls+1)
			}
			assertContains(t, view(m), "nothing selected")
		})
	}

	t.Run("loads at once without selected hunks", func(t *testing.T) {
		src := revisionSource()
		m := loaded(t, src)
		calls := len(src.changeRevs)

		press(t, m, "R")

		if got := len(src.changeRevs); got != calls+1 {
			t.Errorf("Change() calls = %d, want %d", got, calls+1)
		}
	})
}

func TestDiscard_edit(t *testing.T) {
	t.Run("e with selected hunks asks before opening the editor", func(t *testing.T) {
		var opened []string
		m := withEditor(answered(t, press(t, loaded(t, newSource()), "space")), &opened)

		m = press(t, m, "e")

		if len(opened) != 0 {
			t.Fatalf("opened = %v after one e, want nothing", opened)
		}
		assertContains(t, view(m), "press e again to drop 1 selected hunk")

		press(t, m, "e")

		if len(opened) != 1 {
			t.Errorf("opened = %v after two e, want one file", opened)
		}
	})

	t.Run("e where editing is impossible does not ask", func(t *testing.T) {
		var opened []string
		m := withEditor(press(t, press(t, loaded(t, revisionSource()), reviewParent()...), "space"), &opened)

		got := view(press(t, m, "e"))

		assertContains(t, got, "not the working copy", "1 hunk selected")
		assertMissing(t, got, "press e again")
	})
}
