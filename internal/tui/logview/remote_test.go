package logview_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/madalinpopa/jjreview/internal/diff"
	"github.com/madalinpopa/jjreview/internal/jj"
	"github.com/madalinpopa/jjreview/internal/tui/logview"
	"github.com/madalinpopa/jjreview/internal/tui/tab"
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

	t.Run("f fetches from the remote", func(t *testing.T) {
		t.Parallel()
		src := &fakeSource{lines: stack()}
		m := loaded(t, src)

		fx := keys(&m, "f")
		res, err := rewrite(t, fx)

		if err != nil || src.fetched != 1 || res != (tab.Result{Done: "fetched", KeepPicks: true}) || fx.Rewrite.Pending != "fetching…" {
			t.Errorf("Key(f) = %+v, err = %v, fetched %d times, want a fetch reported as fetched", res, err, src.fetched)
		}
	})

	t.Run("an unchanged remote reports it is up to date", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, &fakeSource{lines: stack(), unchanged: true})

		res, err := rewrite(t, keys(&m, "f"))

		if err != nil || res.Done != "already up to date" {
			t.Errorf("Key(f) = %+v, err = %v, want already up to date", res, err)
		}
	})
}

func TestFetch_failed(t *testing.T) {
	t.Parallel()

	t.Run("f reports why the log is blocked", func(t *testing.T) {
		t.Parallel()
		src := &fakeSource{lines: stack()}
		m := loaded(t, src)

		_, fx := m.Key(press("f"), locked(tab.Lock{Reason: "blocked"}))

		if fx != (tab.Effect{Status: "blocked"}) {
			t.Errorf("Key(f) = %+v, want the reason", fx)
		}
	})

	t.Run("f is ignored while the review reloads", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, &fakeSource{lines: stack()})

		if !m.Ignores(press("f"), locked(tab.Lock{Busy: true})) {
			t.Error("Ignores(f, busy) = false, want true")
		}
	})
}

func TestPush_success(t *testing.T) {
	t.Parallel()

	t.Run("P arms the push of the selected revision", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, &fakeSource{lines: stack()})

		fx := keys(&m, "P")

		if fx != (tab.Effect{}) || !m.OverlayOpen() {
			t.Errorf("Key(P) = %+v, OverlayOpen = %v, want no effect and the prompt open", fx, m.OverlayOpen())
		}
	})

	t.Run("y pushes the revision the push was armed on", func(t *testing.T) {
		t.Parallel()
		src := &fakeSource{lines: stack(), pushResult: []jj.PushResult{{Remote: "origin", Bookmarks: []string{"main", "feat"}}, {Remote: "fork", Bookmarks: []string{"feat"}}}}
		m := loaded(t, src)

		fx := keys(&m, "P", "y")
		res, err := rewrite(t, fx)

		if err != nil || !slices.Equal(src.pushed, []string{"c1"}) || len(src.abandoned) != 0 {
			t.Fatalf("Key(y) pushed %v, abandoned %v, err = %v, want a push of c1", src.pushed, src.abandoned, err)
		}
		if res != (tab.Result{Done: "pushed main, feat to origin and feat to fork", KeepPicks: true}) || fx.Rewrite.Pending != "pushing aaaaaaaa…" || fx.Rewrite.Focus != "aaaaaaaaaaaa" {
			t.Errorf("push = %+v, rewrite = %+v, want the pushed bookmarks focused on the change", res, *fx.Rewrite)
		}
	})

	t.Run("nothing to push is reported", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, &fakeSource{lines: stack()})

		res, err := rewrite(t, keys(&m, "P", "y"))

		if err != nil || res.Done != "nothing to push" {
			t.Errorf("push = %+v, err = %v, want nothing to push", res, err)
		}
	})

	t.Run("an immutable revision can be pushed", func(t *testing.T) {
		t.Parallel()
		src := &fakeSource{lines: stack()}
		m := loaded(t, src)

		if _, err := rewrite(t, keys(&m, "G", "P", "y")); err != nil || !slices.Equal(src.pushed, []string{"c4"}) {
			t.Errorf("pushed %v, err = %v, want a push of c4", src.pushed, err)
		}
	})

	t.Run("the prompt names the bookmarks to push", func(t *testing.T) {
		t.Parallel()
		lines := stack()
		lines[0].Entry.Bookmarks = []string{"main", "feat"}
		m := loaded(t, &fakeSource{lines: lines})

		keys(&m, "P")

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

		keys(&m, "P")

		if got := m.Overlay(env(true)); !strings.Contains(got, "Creates a bookmark for the change") || strings.Contains(got, "Drops") {
			t.Errorf("Overlay() = %q, want the new bookmark named and nothing dropped", got)
		}
	})
}

func TestPush_failed(t *testing.T) {
	t.Parallel()

	t.Run("changed bookmarks ask to review the push again", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, &fakeSource{lines: stack(), pushErr: jj.ErrBookmarksChanged})

		res, err := rewrite(t, keys(&m, "P", "y"))

		if err == nil || err.Error() != "bookmarks changed; press P to review the push again" || !res.KeepPicks {
			t.Errorf("push = %+v, err = %v, want the retry hint and the picks kept", res, err)
		}
	})

	t.Run("a jj failure names the revision", func(t *testing.T) {
		t.Parallel()
		errJJ := errors.New("jj git push: exit status 1")
		m := loaded(t, &fakeSource{lines: stack(), pushErr: errJJ})

		_, err := rewrite(t, keys(&m, "P", "y"))

		if !errors.Is(err, errJJ) || !strings.HasPrefix(err.Error(), "push aaaaaaaa: ") {
			t.Errorf("push error = %v, want it wrapped with the revision", err)
		}
	})

	t.Run("P does nothing while busy", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, &fakeSource{lines: stack()})

		_, fx := m.Key(press("P"), locked(tab.Lock{Busy: true}))

		if fx != (tab.Effect{}) || m.OverlayOpen() {
			t.Errorf("Key(P) = %+v, OverlayOpen = %v, want nothing", fx, m.OverlayOpen())
		}
	})
}

func TestPush_cancelled(t *testing.T) {
	t.Parallel()

	t.Run("any other key closes the prompt without pushing", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, &fakeSource{lines: stack()})

		fx := keys(&m, "P", "n", "y")

		if fx.Rewrite != nil || m.OverlayOpen() {
			t.Errorf("Key(P, n, y) = %+v, OverlayOpen = %v, want no push and no prompt", fx, m.OverlayOpen())
		}
	})
}

func TestReset_success(t *testing.T) {
	t.Parallel()

	t.Run("R clears the selection and folds every revision", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, &fakeSource{lines: stack(), files: []diff.File{{NewPath: "a.go"}}})
		keys(&m, "space", "l", "j", "l")

		fx := keys(&m, "R")

		got := m.View(env(true))
		if fx.Refresh || strings.Contains(got, "●") || strings.Contains(got, "selected") || strings.Contains(got, "loading…") {
			t.Errorf("Key(R) = %+v, View() = %q, want no refresh, no selection and no open files", fx, got)
		}
	})

	t.Run("ctrl+r refreshes the log", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, &fakeSource{lines: stack()})

		if fx := keys(&m, "ctrl+r"); !fx.Refresh {
			t.Errorf("Key(ctrl+r) = %+v, want a refresh", fx)
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
