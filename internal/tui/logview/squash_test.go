package logview_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/madalinpopa/jjreview/internal/jj"
	"github.com/madalinpopa/jjreview/internal/tui/logview"
)

func stack() []jj.LogLine {
	return []jj.LogLine{
		{Graph: "@  ", Entry: &jj.LogEntry{ChangeID: "aaaaaaaaaaaa", CommitID: "c1", Parents: []string{"c2"}, WorkingCopy: true}},
		{Graph: "○  ", Entry: &jj.LogEntry{ChangeID: "bbbbbbbbbbbb", CommitID: "c2", Parents: []string{"c3"}}},
		{Graph: "○  ", Entry: &jj.LogEntry{ChangeID: "cccccccccccc", CommitID: "c3", Parents: []string{"c4"}}},
		{Graph: "◆  ", Entry: &jj.LogEntry{ChangeID: "dddddddddddd", CommitID: "c4", Immutable: true}},
	}
}

func keys(m *logview.Model, ks ...string) logview.Intent {
	var in logview.Intent
	for _, k := range ks {
		_, in = m.Key(press(k), env(true))
	}
	return in
}

func commits(entries []jj.LogEntry) []string {
	var ids []string
	for _, e := range entries {
		ids = append(ids, e.CommitID)
	}
	return ids
}

func TestSquash_success(t *testing.T) {
	t.Parallel()

	t.Run("s asks root to start the squash without starting it", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, &fakeSource{lines: stack()})

		in := keys(&m, "s")

		if !in.Squash || m.Capturing() {
			t.Errorf("Key(s) Squash = %v, Capturing = %v, want squash requested and no mode yet", in.Squash, m.Capturing())
		}
	})

	t.Run("without a selection squashes the cursor revision into its parent", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, &fakeSource{lines: stack()})

		status := m.StartSquash()
		in := keys(&m, "y")

		if status != "" || in.Squashed == nil {
			t.Fatalf("StartSquash() = %q, Key(y) Squashed = %+v, want no status and a squash", status, in.Squashed)
		}
		if got := commits(in.Squashed.From); !slices.Equal(got, []string{"c1"}) || in.Squashed.Into.CommitID != "c2" {
			t.Errorf("Squashed = %v into %s, want [c1] into c2", got, in.Squashed.Into.CommitID)
		}
	})

	t.Run("selected revisions default into the latest parent outside the selection", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, &fakeSource{lines: stack()})
		keys(&m, "space", "j", "space")

		m.StartSquash()
		in := keys(&m, "y")

		if in.Squashed == nil {
			t.Fatal("Key(y) Squashed = nil, want a squash")
		}
		if got := commits(in.Squashed.From); !slices.Equal(got, []string{"c1", "c2"}) || in.Squashed.Into.CommitID != "c3" {
			t.Errorf("Squashed = %v into %s, want [c1 c2] into c3", got, in.Squashed.Into.CommitID)
		}
	})

	t.Run("the destination follows the cursor", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, &fakeSource{lines: stack()})
		keys(&m, "j", "j")

		m.StartSquash()
		in := keys(&m, "k", "k", "k", "y")

		if in.Squashed == nil || in.Squashed.Into.CommitID != "c1" {
			t.Errorf("Key(y) Squashed = %+v, want c3 into c1", in.Squashed)
		}
	})

	t.Run("space toggles the selection", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, &fakeSource{lines: stack()})
		keys(&m, "space", "space", "j", "space")

		m.StartSquash()
		in := keys(&m, "y")

		if in.Squashed == nil || !slices.Equal(commits(in.Squashed.From), []string{"c2"}) {
			t.Errorf("Key(y) Squashed = %+v, want only c2", in.Squashed)
		}
	})

	t.Run("esc cancels the squash and keeps the selection", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, &fakeSource{lines: stack()})
		keys(&m, "space")
		m.StartSquash()

		keys(&m, "esc")

		if m.Capturing() || !strings.Contains(m.View(env(true)), "●") {
			t.Errorf("after esc Capturing = %v, want the squash cancelled and the selection kept", m.Capturing())
		}
	})

	t.Run("finishing clears the mode and the selection", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, &fakeSource{lines: stack()})
		keys(&m, "space")
		m.StartSquash()

		m.FinishSquash()

		if m.Capturing() || strings.Contains(m.View(env(true)), "●") {
			t.Errorf("after FinishSquash Capturing = %v, want no mode and no selection", m.Capturing())
		}
	})

	t.Run("starting clears the search so the parent is the destination", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, &fakeSource{lines: stack()})
		keys(&m, "space", "/", "c", "3", "enter")

		m.StartSquash()
		in := keys(&m, "y")

		if in.Squashed == nil || in.Squashed.Into.CommitID != "c2" {
			t.Errorf("Key(y) Squashed = %+v, want c1 into c2", in.Squashed)
		}
		if got := m.View(env(true)); strings.Contains(got, "/ c3") {
			t.Errorf("View() = %q, want the search cleared", got)
		}
	})

	t.Run("clearing the search keeps the cursor on its revision without a parent", func(t *testing.T) {
		t.Parallel()
		lines := stack()
		lines[2].Entry.Parents = nil
		m := loaded(t, &fakeSource{lines: lines})
		keys(&m, "/", "c", "c", "enter")

		m.StartSquash()

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
		m.StartSquash()
		in := keys(&m, "y")

		if in.Squashed == nil || !slices.Equal(commits(in.Squashed.From), []string{"c1"}) || in.Squashed.Into.CommitID != "c2b" {
			t.Errorf("Key(y) Squashed = %+v, want the cursor revision c1 into c2b", in.Squashed)
		}
	})
}

func TestSquash_failed(t *testing.T) {
	t.Parallel()

	t.Run("refuses an immutable source", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, &fakeSource{lines: stack()})
		keys(&m, "G")

		status := m.StartSquash()

		if status != "dddddddd is immutable" || m.Capturing() {
			t.Errorf("StartSquash() = %q, Capturing = %v, want the immutable status and no mode", status, m.Capturing())
		}
	})

	t.Run("refuses an immutable destination", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, &fakeSource{lines: stack()})
		m.StartSquash()

		in := keys(&m, "G", "y")

		if in.Squashed != nil || in.Status != "dddddddd is immutable" || !m.Capturing() {
			t.Errorf("Key(y) = %+v, Capturing = %v, want the immutable status and the mode kept", in, m.Capturing())
		}
	})

	t.Run("refuses a source as the destination", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, &fakeSource{lines: stack()})
		m.StartSquash()

		in := keys(&m, "k", "y")

		if in.Squashed != nil || in.Status == "" || !m.Capturing() {
			t.Errorf("Key(y) = %+v, Capturing = %v, want a status and the mode kept", in, m.Capturing())
		}
	})

	t.Run("enter neither squashes nor opens the revision", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, &fakeSource{lines: stack()})
		m.StartSquash()

		in := keys(&m, "enter")

		if in.Squashed != nil || in.Open != "" || !m.Capturing() {
			t.Errorf("Key(enter) = %+v, Capturing = %v, want nothing and the mode kept", in, m.Capturing())
		}
	})

	t.Run("y is ignored while the review reloads", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, &fakeSource{lines: stack()})
		m.StartSquash()

		if !m.Ignores(press("y"), true) {
			t.Error("Ignores(y, reloading) = false, want true")
		}
	})
}
