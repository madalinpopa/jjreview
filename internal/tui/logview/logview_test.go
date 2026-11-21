package logview_test

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/madalinpopa/jjreview/internal/diff"
	"github.com/madalinpopa/jjreview/internal/jj"
	"github.com/madalinpopa/jjreview/internal/tui/logview"
)

type fakeSource struct {
	lines []jj.LogLine
	files []diff.File
}

func (s *fakeSource) Log(context.Context) ([]jj.LogLine, error) {
	return append([]jj.LogLine(nil), s.lines...), nil
}

func (s *fakeSource) Diff(context.Context, string) ([]diff.File, error) {
	return s.files, nil
}

func entry(graph, changeID, commitID string) jj.LogLine {
	return jj.LogLine{Graph: graph, Entry: &jj.LogEntry{ChangeID: changeID, CommitID: commitID}}
}

func twoEntries() []jj.LogLine {
	return []jj.LogLine{
		{Graph: "@  ", Entry: &jj.LogEntry{ChangeID: "aaaaaaaaaaaa", CommitID: "c1", WorkingCopy: true}},
		{Graph: "○  ", Entry: &jj.LogEntry{ChangeID: "bbbbbbbbbbbb", CommitID: "c2", Immutable: true}},
	}
}

func env(active bool) logview.Env {
	return logview.Env{Width: 80, Height: 24, BodyHeight: 20, ListHeight: 10, ModalWidth: 64, Head: "c1", Active: active}
}

func run(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()

	if cmd == nil {
		t.Fatal("cmd = nil, want a command")
	}
	return cmd()
}

func loaded(t *testing.T, src *fakeSource) logview.Model {
	t.Helper()

	m := logview.New(t.Context(), src)
	if _, in, ok := m.Update(run(t, m.Show("c1")), env(false)); !ok || in != (logview.Intent{}) {
		t.Fatalf("Update(log result) = %+v, %v, want no intent and handled while inactive", in, ok)
	}
	return m
}

func press(k string) tea.KeyPressMsg {
	if k == "enter" {
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	}
	return tea.KeyPressMsg{Code: rune(k[0]), Text: k}
}

func TestUpdate_success(t *testing.T) {
	t.Parallel()

	t.Run("handles the result of its own log load", func(t *testing.T) {
		t.Parallel()
		m := logview.New(t.Context(), &fakeSource{lines: twoEntries()})
		msg := run(t, m.Show("c1"))

		_, _, ok := m.Update(msg, env(true))

		if !ok || m.SelectedChange() != "aaaaaaaaaaaa" {
			t.Errorf("Update(log result) handled = %v, selected = %q, want true and the head", ok, m.SelectedChange())
		}
	})

	t.Run("handles the files of a revision while inactive", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, &fakeSource{lines: twoEntries(), files: []diff.File{{NewPath: "a.go"}}})
		cmd, _ := m.Key(press("l"), env(true))

		_, _, ok := m.Update(run(t, cmd), env(false))

		if !ok {
			t.Error("Update(files result) handled = false, want true")
		}
	})

	t.Run("reloads on the active view after going stale", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, &fakeSource{lines: twoEntries()})
		cmd := m.Load()
		m.MarkStale()

		next, _, _ := m.Update(cmd(), env(true))

		if next == nil {
			t.Error("Update(log result) cmd = nil, want a reload of the stale log")
		}
	})
}

func TestUpdate_failed(t *testing.T) {
	t.Parallel()

	t.Run("leaves foreign messages to the caller", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, &fakeSource{lines: twoEntries()})

		cmd, in, ok := m.Update(tea.FocusMsg{}, env(true))

		if ok || cmd != nil || in != (logview.Intent{}) {
			t.Errorf("Update(FocusMsg) = %v, %+v, %v, want nothing and unhandled", cmd != nil, in, ok)
		}
	})
}

func TestMarkStale_success(t *testing.T) {
	t.Parallel()

	t.Run("show reloads a stale log", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, &fakeSource{lines: twoEntries()})
		m.MarkStale()

		cmd := m.Show("c1")

		if cmd == nil {
			t.Error("Show() after MarkStale() = nil, want a reload")
		}
	})

	t.Run("show keeps a fresh log", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, &fakeSource{lines: twoEntries()})

		cmd := m.Show("c1")

		if cmd != nil {
			t.Error("Show() on a fresh log = a command, want nil")
		}
	})
}

func TestRestorePosition_success(t *testing.T) {
	t.Parallel()

	t.Run("keeps the row number after the held revision disappears", func(t *testing.T) {
		t.Parallel()
		src := &fakeSource{lines: []jj.LogLine{entry("@  ", "aaaa", "c1"), entry("○  ", "bbbb", "c2"), entry("○  ", "cccc", "c3")}}
		m := loaded(t, src)
		m.Key(press("j"), env(true))
		keep := m.HoldPosition()
		src.lines = []jj.LogLine{entry("@  ", "aaaa", "c1"), entry("○  ", "cccc", "c3")}

		m.RestorePosition(keep)
		m.Update(run(t, m.Load()), env(true))

		if got := m.SelectedChange(); got != "cccc" {
			t.Errorf("SelectedChange() = %q, want cccc in the held row", got)
		}
	})

	t.Run("without a restore the reload follows the head", func(t *testing.T) {
		t.Parallel()
		src := &fakeSource{lines: []jj.LogLine{entry("@  ", "aaaa", "c1"), entry("○  ", "bbbb", "c2"), entry("○  ", "cccc", "c3")}}
		m := loaded(t, src)
		m.Key(press("j"), env(true))
		m.HoldPosition()
		src.lines = []jj.LogLine{entry("@  ", "aaaa", "c1"), entry("○  ", "cccc", "c3")}

		m.Update(run(t, m.Load()), env(true))

		if got := m.SelectedChange(); got != "aaaa" {
			t.Errorf("SelectedChange() = %q, want the head aaaa", got)
		}
	})
}

func TestKey_success(t *testing.T) {
	t.Parallel()

	t.Run("enter asks root to open the selected revision", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, &fakeSource{lines: twoEntries()})

		_, in := m.Key(press("enter"), env(true))

		if in.Open != "c1" {
			t.Errorf("Key(enter) Open = %q, want c1", in.Open)
		}
	})

	t.Run("a asks root to arm the abandon without arming it", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, &fakeSource{lines: twoEntries()})

		_, in := m.Key(press("a"), env(true))

		if !in.Arm || m.OverlayOpen() {
			t.Errorf("Key(a) Arm = %v, OverlayOpen = %v, want arm requested and nothing armed", in.Arm, m.OverlayOpen())
		}
	})

	t.Run("y after arming returns the entry to abandon", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, &fakeSource{lines: twoEntries()})
		m.Arm()

		_, in := m.Key(press("y"), env(true))

		if in.Abandon == nil || in.Abandon.CommitID != "c1" || m.OverlayOpen() {
			t.Errorf("Key(y) Abandon = %+v, OverlayOpen = %v, want c1 and the confirmation closed", in.Abandon, m.OverlayOpen())
		}
	})

	t.Run("search captures input until applied", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, &fakeSource{lines: twoEntries()})

		m.Key(press("/"), env(true))
		captured := m.Capturing()
		m.Key(press("enter"), env(true))

		if !captured || m.Capturing() {
			t.Errorf("Capturing() = %v then %v, want true while searching and false after enter", captured, m.Capturing())
		}
	})
}

func TestKey_failed(t *testing.T) {
	t.Parallel()

	t.Run("y does not abandon when a reload moved the selection", func(t *testing.T) {
		t.Parallel()
		src := &fakeSource{lines: twoEntries()}
		m := loaded(t, src)
		m.Arm()
		src.lines = []jj.LogLine{
			{Graph: "@  ", Entry: &jj.LogEntry{ChangeID: "cccccccccccc", CommitID: "c3", WorkingCopy: true}},
			{Graph: "○  ", Entry: &jj.LogEntry{ChangeID: "bbbbbbbbbbbb", CommitID: "c2", Immutable: true}},
		}
		m.Update(run(t, m.Load()), env(true))
		open := m.OverlayOpen()

		_, in := m.Key(press("y"), env(true))

		if !open || m.SelectedChange() != "cccccccccccc" || in.Abandon != nil {
			t.Errorf("after reload OverlayOpen = %v, selected = %q; Key(y) Abandon = %+v, want open, cccccccccccc and nil", open, m.SelectedChange(), in.Abandon)
		}
	})

	t.Run("a is ignored while the review reloads", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, &fakeSource{lines: twoEntries()})

		if !m.Ignores(press("a"), true) {
			t.Error("Ignores(a, reloading) = false, want true")
		}
	})
}

func TestArm_success(t *testing.T) {
	t.Parallel()

	t.Run("arms the selected mutable revision", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, &fakeSource{lines: twoEntries()})

		status := m.Arm()

		if status != "" || !m.OverlayOpen() || m.Overlay(env(true)) == "" {
			t.Errorf("Arm() = %q, OverlayOpen = %v, want no status and the confirmation open", status, m.OverlayOpen())
		}
	})
}

func TestArm_failed(t *testing.T) {
	t.Parallel()

	t.Run("refuses an immutable revision", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, &fakeSource{lines: twoEntries()})
		m.Key(press("j"), env(true))

		status := m.Arm()

		if status != "bbbbbbbb is immutable" || m.OverlayOpen() || m.Overlay(env(true)) != "" {
			t.Errorf("Arm() = %q, OverlayOpen = %v, want the immutable status and nothing armed", status, m.OverlayOpen())
		}
	})
}

func TestAbandonKey_success(t *testing.T) {
	t.Parallel()

	m := logview.New(t.Context(), &fakeSource{})

	if got := m.AbandonKey(); got != "a" {
		t.Errorf("AbandonKey() = %q, want a", got)
	}
}
