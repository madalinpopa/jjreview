package logview_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/madalinpopa/jjreview/internal/jj"
	"github.com/madalinpopa/jjreview/internal/tui/logview"
	"github.com/madalinpopa/jjreview/internal/tui/tab"
)

func stack() []jj.LogLine {
	return []jj.LogLine{
		{Graph: "@  ", Entry: &jj.LogEntry{ChangeID: "aaaaaaaaaaaa", CommitID: "c1", Parents: []string{"c2"}, WorkingCopy: true}},
		{Graph: "○  ", Entry: &jj.LogEntry{ChangeID: "bbbbbbbbbbbb", CommitID: "c2", Parents: []string{"c3"}}},
		{Graph: "○  ", Entry: &jj.LogEntry{ChangeID: "cccccccccccc", CommitID: "c3", Parents: []string{"c4"}}},
		{Graph: "◆  ", Entry: &jj.LogEntry{ChangeID: "dddddddddddd", CommitID: "c4", Immutable: true}},
	}
}

func keys(m *logview.Model, ks ...string) logview.Effect {
	var fx logview.Effect
	for _, k := range ks {
		_, fx = m.Key(press(k), env(true))
	}
	return fx
}

func commits(entries []jj.LogEntry) []string {
	var ids []string
	for _, e := range entries {
		ids = append(ids, e.CommitID)
	}
	return ids
}

func squashed(t *testing.T, src *fakeSource, fx logview.Effect) (tab.Result, squashCall) {
	t.Helper()

	res, err := rewrite(t, fx)
	if err != nil {
		t.Fatalf("squash error = %v, want none", err)
	}
	if len(src.squashes) != 1 {
		t.Fatalf("Squash() calls = %+v, want one", src.squashes)
	}
	return res, src.squashes[0]
}

func TestSquash_success(t *testing.T) {
	t.Parallel()

	t.Run("s starts the squash", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, &fakeSource{lines: stack()})

		fx := keys(&m, "s")

		if fx != (logview.Effect{}) || !m.Captures() {
			t.Errorf("Key(s) = %+v, Captures = %v, want no effect and the squash mode", fx, m.Captures())
		}
	})

	t.Run("without a selection squashes the cursor revision into its parent", func(t *testing.T) {
		t.Parallel()
		src := &fakeSource{lines: stack()}
		m := loaded(t, src)

		fx := keys(&m, "s", "y")
		res, got := squashed(t, src, fx)

		if !slices.Equal(got.from, []string{"c1"}) || got.into != "c2" {
			t.Errorf("Squash() = %v into %s, want [c1] into c2", got.from, got.into)
		}
		if res != (tab.Result{Done: "squashed aaaaaaaa into bbbbbbbb", Replaced: true}) || fx.Rewrite.Focus != "bbbbbbbbbbbb" || fx.Rewrite.Pending != "" {
			t.Errorf("squash = %+v, rewrite = %+v, want the done status, the head replaced and the destination focused", res, *fx.Rewrite)
		}
	})

	t.Run("selected revisions default into the latest parent outside the selection", func(t *testing.T) {
		t.Parallel()
		src := &fakeSource{lines: stack()}
		m := loaded(t, src)
		keys(&m, "space", "j", "space")

		res, got := squashed(t, src, keys(&m, "s", "y"))

		if !slices.Equal(got.from, []string{"c1", "c2"}) || got.into != "c3" {
			t.Errorf("Squash() = %v into %s, want [c1 c2] into c3", got.from, got.into)
		}
		if res.Done != "squashed 2 revisions into cccccccc" {
			t.Errorf("squash Done = %q, want squashed 2 revisions into cccccccc", res.Done)
		}
	})

	t.Run("a squash that leaves the head keeps it", func(t *testing.T) {
		t.Parallel()
		src := &fakeSource{lines: stack(), warnings: "New conflicts"}
		m := loaded(t, src)

		res, _ := squashed(t, src, keys(&m, "j", "s", "y"))

		if res != (tab.Result{Done: "squashed bbbbbbbb into cccccccc", Warnings: "New conflicts"}) {
			t.Errorf("squash = %+v, want the warnings and the head kept", res)
		}
	})

	t.Run("the destination follows the cursor", func(t *testing.T) {
		t.Parallel()
		src := &fakeSource{lines: stack()}
		m := loaded(t, src)
		keys(&m, "j", "j")

		_, got := squashed(t, src, keys(&m, "s", "k", "k", "k", "y"))

		if got.into != "c1" {
			t.Errorf("Squash() into %s, want c3 into c1", got.into)
		}
	})

	t.Run("space toggles the selection", func(t *testing.T) {
		t.Parallel()
		src := &fakeSource{lines: stack()}
		m := loaded(t, src)
		keys(&m, "space", "space", "j", "space")

		_, got := squashed(t, src, keys(&m, "s", "y"))

		if !slices.Equal(got.from, []string{"c2"}) {
			t.Errorf("Squash() from %v, want only c2", got.from)
		}
	})

	t.Run("esc cancels the squash and keeps the selection", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, &fakeSource{lines: stack()})
		keys(&m, "space", "s")

		keys(&m, "esc")

		if m.Captures() || !strings.Contains(m.View(env(true)), "●") {
			t.Errorf("after esc Captures = %v, want the squash cancelled and the selection kept", m.Captures())
		}
	})

	t.Run("squashing clears the mode and the selection", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, &fakeSource{lines: stack()})
		keys(&m, "space", "s")

		fx := keys(&m, "j", "y")

		if fx.Rewrite == nil || m.Captures() || strings.Contains(m.View(env(true)), "●") {
			t.Errorf("Key(y) = %+v, Captures = %v, want a squash, no mode and no selection", fx, m.Captures())
		}
	})

	t.Run("selected hunks ask for a second y before squashing", func(t *testing.T) {
		t.Parallel()
		src := &fakeSource{lines: stack()}
		m := loaded(t, src)
		dropping := env(true)
		dropping.Dropped = 2
		keys(&m, "s")

		_, first := m.Key(press("y"), dropping)
		_, second := m.Key(press("y"), dropping)

		if first != (logview.Effect{Status: "press y again to drop 2 selected hunks"}) {
			t.Errorf("first Key(y) = %+v, want the drop prompt", first)
		}
		if _, got := squashed(t, src, second); got.into != "c2" || m.Captures() {
			t.Errorf("second Key(y) squashed into %s, Captures = %v, want c2 and the mode closed", got.into, m.Captures())
		}
	})

	t.Run("another key between the two y presses asks again", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, &fakeSource{lines: stack()})
		dropping := env(true)
		dropping.Dropped = 1
		keys(&m, "s")

		m.Key(press("y"), dropping)
		m.Key(press("x"), dropping)
		_, fx := m.Key(press("y"), dropping)

		if fx != (logview.Effect{Status: "press y again to drop 1 selected hunk"}) || !m.Captures() {
			t.Errorf("Key(y, x, y) = %+v, Captures = %v, want the drop prompt again and the mode kept", fx, m.Captures())
		}
	})

	t.Run("starting clears the search so the parent is the destination", func(t *testing.T) {
		t.Parallel()
		src := &fakeSource{lines: stack()}
		m := loaded(t, src)
		keys(&m, "space", "/", "c", "3", "enter")

		fx := keys(&m, "s", "y")

		if got := m.View(env(true)); strings.Contains(got, "/ c3") {
			t.Errorf("View() = %q, want the search cleared", got)
		}
		if _, got := squashed(t, src, fx); got.into != "c2" {
			t.Errorf("Squash() into %s, want c1 into c2", got.into)
		}
	})

	t.Run("clearing the search keeps the cursor on its revision without a parent", func(t *testing.T) {
		t.Parallel()
		lines := stack()
		lines[2].Entry.Parents = nil
		m := loaded(t, &fakeSource{lines: lines})
		keys(&m, "/", "c", "c", "enter")

		keys(&m, "s")

		if got := m.SelectedChange(); got != "cccccccccccc" {
			t.Errorf("SelectedChange() = %q, want cccccccccccc kept under the cursor", got)
		}
	})

	t.Run("a reload that drops a selected revision clears it", func(t *testing.T) {
		t.Parallel()
		src := &fakeSource{lines: stack()}
		m := loaded(t, src)
		keys(&m, "j", "space")
		src.lines = stack()
		src.lines[0].Entry.Parents = []string{"c2b"}
		src.lines[1].Entry.CommitID = "c2b"

		m.Update(run(t, m.Load()), env(true))
		_, got := squashed(t, src, keys(&m, "s", "y"))

		if !slices.Equal(got.from, []string{"c1"}) || got.into != "c2b" {
			t.Errorf("Squash() = %v into %s, want the cursor revision c1 into c2b", got.from, got.into)
		}
	})
}

func TestSquash_failed(t *testing.T) {
	t.Parallel()

	t.Run("refuses an immutable source", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, &fakeSource{lines: stack()})

		fx := keys(&m, "G", "s")

		if fx != (logview.Effect{Status: "dddddddd is immutable"}) || m.Captures() {
			t.Errorf("Key(s) = %+v, Captures = %v, want the immutable status and no mode", fx, m.Captures())
		}
	})

	t.Run("refuses an immutable destination", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, &fakeSource{lines: stack()})

		fx := keys(&m, "s", "G", "y")

		if fx != (logview.Effect{Status: "dddddddd is immutable"}) || !m.Captures() {
			t.Errorf("Key(y) = %+v, Captures = %v, want the immutable status and the mode kept", fx, m.Captures())
		}
	})

	t.Run("refuses a source as the destination", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, &fakeSource{lines: stack()})

		fx := keys(&m, "s", "k", "y")

		if fx.Rewrite != nil || fx.Status == "" || !m.Captures() {
			t.Errorf("Key(y) = %+v, Captures = %v, want a status and the mode kept", fx, m.Captures())
		}
	})

	t.Run("enter neither squashes nor opens the revision", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, &fakeSource{lines: stack()})

		fx := keys(&m, "s", "enter")

		if fx != (logview.Effect{}) || !m.Captures() {
			t.Errorf("Key(enter) = %+v, Captures = %v, want nothing and the mode kept", fx, m.Captures())
		}
	})

	t.Run("y is ignored while the review reloads", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, &fakeSource{lines: stack()})
		keys(&m, "s")

		if !m.Ignores(press("y"), locked(tab.Lock{Busy: true})) {
			t.Error("Ignores(y, busy) = false, want true")
		}
	})

	t.Run("s does nothing while busy", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, &fakeSource{lines: stack()})

		_, fx := m.Key(press("s"), locked(tab.Lock{Busy: true}))

		if fx != (logview.Effect{}) || m.Captures() {
			t.Errorf("Key(s) = %+v, Captures = %v, want nothing", fx, m.Captures())
		}
	})

	t.Run("y reports why the log is blocked and keeps the mode", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, &fakeSource{lines: stack()})
		keys(&m, "s")

		_, fx := m.Key(press("y"), locked(tab.Lock{Reason: "blocked"}))

		if fx != (logview.Effect{Status: "blocked"}) || !m.Captures() {
			t.Errorf("Key(y) = %+v, Captures = %v, want the reason and the mode kept", fx, m.Captures())
		}
	})
}
