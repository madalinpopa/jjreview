package logview_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/madalinpopa/jjreview/internal/diff"
	"github.com/madalinpopa/jjreview/internal/tui/logview"
	"github.com/madalinpopa/jjreview/internal/tui/ui"
)

func labels(m logview.Model) []string {
	var out []string
	for _, h := range m.Hints(ui.KeyHint{Label: "tabs"}, ui.KeyHint{Label: "help"}) {
		out = append(out, strings.Join(h.Keys, " ")+" "+h.Label)
	}
	return out
}

func TestFetch_success(t *testing.T) {
	t.Parallel()

	t.Run("f asks root to fetch", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, &fakeSource{lines: stack()})

		if in := keys(&m, "f"); !in.Fetch {
			t.Errorf("Key(f) = %+v, want a fetch", in)
		}
	})
}

func TestPush_success(t *testing.T) {
	t.Parallel()

	t.Run("P asks root to arm the push without arming it", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, &fakeSource{lines: stack()})

		in := keys(&m, "P")

		if !in.ArmPush || m.OverlayOpen() {
			t.Errorf("Key(P) ArmPush = %v, OverlayOpen = %v, want a request and no prompt yet", in.ArmPush, m.OverlayOpen())
		}
	})

	t.Run("y pushes the revision the push was armed on", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, &fakeSource{lines: stack()})

		m.ArmPush()
		in := keys(&m, "y")

		if in.Push == nil || in.Push.CommitID != "c1" || in.Abandon != nil {
			t.Errorf("Key(y) = %+v, want a push of c1", in)
		}
	})

	t.Run("an immutable revision can be pushed", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, &fakeSource{lines: stack()})
		keys(&m, "G")

		m.ArmPush()
		in := keys(&m, "y")

		if in.Push == nil || in.Push.CommitID != "c4" {
			t.Errorf("Key(y) = %+v, want a push of c4", in)
		}
	})

	t.Run("the prompt names the bookmarks to push", func(t *testing.T) {
		t.Parallel()
		lines := stack()
		lines[0].Entry.Bookmarks = []string{"main", "feat"}
		m := loaded(t, &fakeSource{lines: lines})

		m.ArmPush()

		got := m.Overlay(env(true))
		for _, want := range []string{"Push aaaaaaaa c1", "Pushes bookmarks main, feat", "y push · any other key cancels"} {
			if !strings.Contains(got, want) {
				t.Errorf("Overlay() = %q, want it to contain %q", got, want)
			}
		}
	})

	t.Run("the prompt says a revision without bookmarks gets one", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, &fakeSource{lines: stack()})

		m.ArmPush()

		if got := m.Overlay(env(true)); !strings.Contains(got, "Creates a bookmark for the change") || strings.Contains(got, "Drops") {
			t.Errorf("Overlay() = %q, want the new bookmark named and nothing dropped", got)
		}
	})
}

func TestPush_cancelled(t *testing.T) {
	t.Parallel()

	t.Run("any other key closes the prompt without pushing", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, &fakeSource{lines: stack()})

		m.ArmPush()
		in := keys(&m, "n", "y")

		if in.Push != nil || m.OverlayOpen() {
			t.Errorf("Key(n, y) Push = %+v, OverlayOpen = %v, want no push and no prompt", in.Push, m.OverlayOpen())
		}
	})
}

func TestReset_success(t *testing.T) {
	t.Parallel()

	t.Run("R clears the selection and folds every revision", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, &fakeSource{lines: stack(), files: []diff.File{{NewPath: "a.go"}}})
		keys(&m, "space", "l", "j", "l")

		in := keys(&m, "R")

		got := m.View(env(true))
		if in.Refresh || strings.Contains(got, "●") || strings.Contains(got, "selected") || strings.Contains(got, "loading…") {
			t.Errorf("Key(R) = %+v, View() = %q, want no refresh, no selection and no open files", in, got)
		}
	})

	t.Run("ctrl+r refreshes the log", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, &fakeSource{lines: stack()})

		if in := keys(&m, "ctrl+r"); !in.Refresh {
			t.Errorf("Key(ctrl+r) = %+v, want a refresh", in)
		}
	})
}

func TestHints_success(t *testing.T) {
	t.Parallel()

	t.Run("lists fetch and push without squash or reset", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, &fakeSource{lines: stack()})

		got := labels(m)

		for _, want := range []string{"space select", "f fetch", "P push"} {
			if !slices.Contains(got, want) {
				t.Errorf("Hints() = %v, want %q", got, want)
			}
		}
		for _, missing := range []string{"s squash", "R reset", "ctrl+r refresh"} {
			if slices.Contains(got, missing) {
				t.Errorf("Hints() = %v, want no %q", got, missing)
			}
		}
	})

	t.Run("a selection adds squash and reset", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, &fakeSource{lines: stack()})

		keys(&m, "space")

		got := labels(m)
		if !slices.Contains(got, "s squash") || !slices.Contains(got, "R reset") {
			t.Errorf("Hints() = %v, want squash and reset", got)
		}
	})

	t.Run("open files add reset", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, &fakeSource{lines: stack()})

		keys(&m, "l")

		got := labels(m)
		if !slices.Contains(got, "R reset") || slices.Contains(got, "s squash") {
			t.Errorf("Hints() = %v, want reset without squash", got)
		}
	})

	t.Run("open files of a commit that left the log drop reset", func(t *testing.T) {
		t.Parallel()
		src := &fakeSource{lines: stack()}
		m := loaded(t, src)
		keys(&m, "l")
		src.lines = stack()[1:]

		m.Update(run(t, m.Load()), env(true))

		if got := labels(m); slices.Contains(got, "R reset") {
			t.Errorf("Hints() = %v, want no reset", got)
		}
	})
}
