package tui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/madalinpopa/jjreview/internal/diff"
	"github.com/madalinpopa/jjreview/internal/jj"
)

type stubLogSource struct {
	lines []jj.LogLine
}

func (s stubLogSource) Log(context.Context) ([]jj.LogLine, error) {
	return s.lines, nil
}

func (s stubLogSource) Diff(context.Context, string) ([]diff.File, error) {
	return nil, nil
}

func loadedLog(t *testing.T, lines ...jj.LogLine) logModel {
	t.Helper()
	l := newLogModel(t.Context(), stubLogSource{lines: lines})
	env := logEnv{list: 10, head: "c1"}
	msg := l.load()()
	if _, in := l.update(msg, env); in != (logIntent{}) {
		t.Fatalf("update(logMsg) intent = %+v, want none while inactive", in)
	}
	return l
}

func twoEntries() []jj.LogLine {
	return []jj.LogLine{
		{Graph: "@  ", Entry: &jj.LogEntry{ChangeID: "aaaaaaaaaaaa", CommitID: "c1", WorkingCopy: true}},
		{Graph: "○  ", Entry: &jj.LogEntry{ChangeID: "bbbbbbbbbbbb", CommitID: "c2", Immutable: true}},
	}
}

func TestLogModelKey_success(t *testing.T) {
	t.Parallel()

	t.Run("enter asks root to open the selected revision", func(t *testing.T) {
		t.Parallel()
		l := loadedLog(t, twoEntries()...)

		_, in := l.key(tea.KeyPressMsg{Code: tea.KeyEnter}, logEnv{list: 10})

		if in.open != "c1" {
			t.Errorf("key(enter) open = %q, want c1", in.open)
		}
	})

	t.Run("a asks root to arm the abandon without arming it", func(t *testing.T) {
		t.Parallel()
		l := loadedLog(t, twoEntries()...)

		_, in := l.key(tea.KeyPressMsg{Code: 'a', Text: "a"}, logEnv{list: 10})

		if !in.arm || l.confirming() {
			t.Errorf("key(a) arm = %v, confirming = %v, want arm requested and nothing armed", in.arm, l.confirming())
		}
	})

	t.Run("arm refuses an immutable revision", func(t *testing.T) {
		t.Parallel()
		l := loadedLog(t, twoEntries()...)
		l.move(1)

		status := l.arm()

		if status != "bbbbbbbb is immutable" || l.confirming() {
			t.Errorf("arm() = %q, confirming = %v, want the immutable status and nothing armed", status, l.confirming())
		}
	})

	t.Run("y after arming returns the entry to abandon", func(t *testing.T) {
		t.Parallel()
		l := loadedLog(t, twoEntries()...)
		l.arm()

		_, in := l.key(tea.KeyPressMsg{Code: 'y', Text: "y"}, logEnv{list: 10})

		if in.abandon == nil || in.abandon.CommitID != "c1" || l.confirming() {
			t.Errorf("key(y) abandon = %+v, confirming = %v, want c1 and the confirmation closed", in.abandon, l.confirming())
		}
	})

	t.Run("search captures input until applied", func(t *testing.T) {
		t.Parallel()
		l := loadedLog(t, twoEntries()...)

		l.key(tea.KeyPressMsg{Code: '/', Text: "/"}, logEnv{list: 10})
		captured := l.capturing()
		l.key(tea.KeyPressMsg{Code: tea.KeyEnter}, logEnv{list: 10})

		if !captured || l.capturing() {
			t.Errorf("capturing() = %v then %v, want true while searching and false after enter", captured, l.capturing())
		}
	})

	t.Run("reload on the active view after going stale", func(t *testing.T) {
		t.Parallel()
		l := loadedLog(t, twoEntries()...)
		cmd := l.load()
		l.stale = true

		next, _ := l.update(cmd(), logEnv{list: 10, head: "c1", active: true})

		if next == nil {
			t.Error("update(logMsg) cmd = nil, want a reload of the stale log")
		}
	})
}

func TestLogModelKey_failed(t *testing.T) {
	t.Parallel()

	t.Run("y does not abandon when the selection moved", func(t *testing.T) {
		t.Parallel()
		l := loadedLog(t, twoEntries()...)
		l.arm()
		l.move(1)

		_, in := l.key(tea.KeyPressMsg{Code: 'y', Text: "y"}, logEnv{list: 10})

		if in.abandon != nil {
			t.Errorf("key(y) abandon = %+v, want nil", in.abandon)
		}
	})

	t.Run("a is ignored while the review reloads", func(t *testing.T) {
		t.Parallel()
		l := loadedLog(t, twoEntries()...)

		if !l.ignores(tea.KeyPressMsg{Code: 'a', Text: "a"}, true) {
			t.Error("ignores(a, reloading) = false, want true")
		}
	})
}
