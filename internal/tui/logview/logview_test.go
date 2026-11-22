package logview_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/madalinpopa/jjreview/internal/diff"
	"github.com/madalinpopa/jjreview/internal/jj"
	"github.com/madalinpopa/jjreview/internal/tui/logview"
	"github.com/madalinpopa/jjreview/internal/tui/tab"
)

type squashCall struct {
	from []string
	into string
}

type fakeSource struct {
	lines      []jj.LogLine
	files      []diff.File
	abandoned  []string
	abandonErr error
	squashes   []squashCall
	warnings   string
	pushed     []string
	pushResult []jj.PushResult
	pushErr    error
	fetched    int
	unchanged  bool
}

func (s *fakeSource) Log(context.Context) ([]jj.LogLine, error) {
	return append([]jj.LogLine(nil), s.lines...), nil
}

func (s *fakeSource) Diff(context.Context, string) ([]diff.File, error) {
	return s.files, nil
}

func (s *fakeSource) Abandon(_ context.Context, commitID string, _ []string) (bool, error) {
	s.abandoned = append(s.abandoned, commitID)
	return false, s.abandonErr
}

func (s *fakeSource) Squash(_ context.Context, from []jj.LogEntry, into jj.LogEntry) (string, error) {
	s.squashes = append(s.squashes, squashCall{from: commits(from), into: into.CommitID})
	return s.warnings, nil
}

func (s *fakeSource) Push(_ context.Context, e jj.LogEntry) ([]jj.PushResult, error) {
	s.pushed = append(s.pushed, e.CommitID)
	return s.pushResult, s.pushErr
}

func (s *fakeSource) Fetch(context.Context) (bool, error) {
	s.fetched++
	return !s.unchanged, nil
}

func rewrite(t *testing.T, fx tab.Effect) (tab.Result, error) {
	t.Helper()

	if fx.Rewrite == nil {
		t.Fatalf("Effect = %+v, want a rewrite", fx)
	}
	return fx.Rewrite.Run(t.Context(), tab.Head{CommitID: "c1", WorkingCopy: true})
}

func locked(l tab.Lock) tab.Env {
	e := env(true)
	e.Lock = l
	return e
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

var _ tab.View = (*logview.Model)(nil)

func env(active bool) tab.Env {
	return tab.Env{Width: 80, Height: 24, BodyHeight: 20, ListHeight: 10, ModalWidth: 64, Head: tab.Head{CommitID: "c1"}, Active: active}
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
	if _, fx, ok := m.Update(run(t, m.Show(env(true))), env(false)); !ok || fx != (tab.Effect{}) {
		t.Fatalf("Update(log result) = %+v, %v, want no effect and handled while inactive", fx, ok)
	}
	return m
}

func press(k string) tea.KeyPressMsg {
	switch k {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "space":
		return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	case "ctrl+r":
		return tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl}
	}
	return tea.KeyPressMsg{Code: rune(k[0]), Text: k}
}

func TestUpdate_success(t *testing.T) {
	t.Parallel()

	t.Run("handles the result of its own log load", func(t *testing.T) {
		t.Parallel()
		m := logview.New(t.Context(), &fakeSource{lines: twoEntries()})
		msg := run(t, m.Show(env(true)))

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

func TestUpdateSearch_success(t *testing.T) {
	t.Parallel()

	t.Run("forwards a foreign message to the search input while active", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, &fakeSource{lines: twoEntries()})
		m.Key(press("/"), env(true))

		_, fx, ok := m.Update(tea.PasteMsg{Content: "c2"}, env(true))

		if !ok || fx != (tab.Effect{}) || m.SelectedChange() != "bbbbbbbbbbbb" {
			t.Errorf("Update(PasteMsg) = %+v, %v, selected = %q, want no effect, handled and the matching bbbbbbbbbbbb", fx, ok, m.SelectedChange())
		}
	})
}

func TestPaste_success(t *testing.T) {
	t.Parallel()

	t.Run("types into the search", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, &fakeSource{lines: twoEntries()})
		m.Key(press("/"), env(true))

		m.Paste(tea.PasteMsg{Content: "c2"})

		if got := m.SelectedChange(); got != "bbbbbbbbbbbb" {
			t.Errorf("SelectedChange() = %q, want the matching bbbbbbbbbbbb", got)
		}
	})

	t.Run("is ignored without a search", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, &fakeSource{lines: twoEntries()})

		cmd := m.Paste(tea.PasteMsg{Content: "c2"})

		if cmd != nil || m.SelectedChange() != "aaaaaaaaaaaa" || m.Captures() {
			t.Errorf("Paste() cmd = %v, selected = %q, Captures = %v, want nothing", cmd != nil, m.SelectedChange(), m.Captures())
		}
	})
}

func TestUpdate_failed(t *testing.T) {
	t.Parallel()

	t.Run("leaves foreign messages to the caller while inactive", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, &fakeSource{lines: twoEntries()})
		m.Key(press("/"), env(true))

		cmd, fx, ok := m.Update(tea.PasteMsg{Content: "c2"}, env(false))

		if ok || cmd != nil || fx != (tab.Effect{}) || m.SelectedChange() != "aaaaaaaaaaaa" {
			t.Errorf("Update(PasteMsg) = %v, %+v, %v, selected = %q, want nothing, unhandled and the head", cmd != nil, fx, ok, m.SelectedChange())
		}
	})

	t.Run("leaves foreign messages to the caller", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, &fakeSource{lines: twoEntries()})

		cmd, fx, ok := m.Update(tea.FocusMsg{}, env(true))

		if ok || cmd != nil || fx != (tab.Effect{}) {
			t.Errorf("Update(FocusMsg) = %v, %+v, %v, want nothing and unhandled", cmd != nil, fx, ok)
		}
	})
}

func TestMarkStale_success(t *testing.T) {
	t.Parallel()

	t.Run("show reloads a stale log", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, &fakeSource{lines: twoEntries()})
		m.MarkStale()

		cmd := m.Show(env(true))

		if cmd == nil {
			t.Error("Show() after MarkStale() = nil, want a reload")
		}
	})

	t.Run("show keeps a fresh log", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, &fakeSource{lines: twoEntries()})

		cmd := m.Show(env(true))

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

	t.Run("a targeted change wins over the held row", func(t *testing.T) {
		t.Parallel()
		src := &fakeSource{lines: []jj.LogLine{entry("@  ", "aaaa", "c1"), entry("○  ", "bbbb", "c2"), entry("○  ", "cccc", "c3")}}
		m := loaded(t, src)
		m.Key(press("j"), env(true))
		keep := m.HoldPosition().Targeting("bbbb")
		src.lines = []jj.LogLine{entry("@  ", "bbbb", "c2b"), entry("○  ", "cccc", "c3")}

		m.RestorePosition(keep)
		m.Update(run(t, m.Load()), env(true))

		if got := m.SelectedChange(); got != "bbbb" {
			t.Errorf("SelectedChange() = %q, want the targeted bbbb", got)
		}
	})

	t.Run("a missing targeted change keeps the held row", func(t *testing.T) {
		t.Parallel()
		src := &fakeSource{lines: []jj.LogLine{entry("@  ", "aaaa", "c1"), entry("○  ", "bbbb", "c2"), entry("○  ", "cccc", "c3")}}
		m := loaded(t, src)
		m.Key(press("j"), env(true))
		keep := m.HoldPosition().Targeting("gone")
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

		_, fx := m.Key(press("enter"), env(true))

		if fx != (tab.Effect{Open: "c1", Reopen: true}) {
			t.Errorf("Key(enter) = %+v, want an open of c1 that keeps a pending drop armed", fx)
		}
	})

	t.Run("b asks root to pick a bookmark for the selected revision", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, &fakeSource{lines: twoEntries()})

		_, fx := m.Key(press("b"), env(true))

		if fx.Pick == nil || fx.Pick.CommitID != "c1" {
			t.Errorf("Key(b) = %+v, want a pick of c1", fx)
		}
	})

	t.Run("a arms the abandon of the selected revision", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, &fakeSource{lines: twoEntries()})

		_, fx := m.Key(press("a"), env(true))

		if fx != (tab.Effect{}) || !m.OverlayOpen() || m.Overlay(env(true)) == "" {
			t.Errorf("Key(a) = %+v, OverlayOpen = %v, want no effect and the confirmation open", fx, m.OverlayOpen())
		}
	})

	t.Run("y after arming abandons the armed revision", func(t *testing.T) {
		t.Parallel()
		src := &fakeSource{lines: twoEntries()}
		m := loaded(t, src)
		m.Key(press("a"), env(true))

		_, fx := m.Key(press("y"), env(true))
		res, err := rewrite(t, fx)

		if err != nil || len(src.abandoned) != 1 || src.abandoned[0] != "c1" || m.OverlayOpen() {
			t.Fatalf("Key(y) abandoned %v, err = %v, OverlayOpen = %v, want c1 and the confirmation closed", src.abandoned, err, m.OverlayOpen())
		}
		if res.Done != "abandoned aaaaaaaa" || !res.Replaced || fx.Rewrite.Pending != "" || fx.Rewrite.Focus != "" {
			t.Errorf("abandon = %+v, rewrite = %+v, want the done status and the head replaced", res, *fx.Rewrite)
		}
	})

	t.Run("search captures input until applied", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, &fakeSource{lines: twoEntries()})

		m.Key(press("/"), env(true))
		captured := m.Captures()
		m.Key(press("enter"), env(true))

		if !captured || m.Captures() {
			t.Errorf("Captures() = %v then %v, want true while searching and false after enter", captured, m.Captures())
		}
	})
}

func TestKey_failed(t *testing.T) {
	t.Parallel()

	t.Run("y does not abandon when a reload moved the selection", func(t *testing.T) {
		t.Parallel()
		src := &fakeSource{lines: twoEntries()}
		m := loaded(t, src)
		m.Key(press("a"), env(true))
		src.lines = []jj.LogLine{
			{Graph: "@  ", Entry: &jj.LogEntry{ChangeID: "cccccccccccc", CommitID: "c3", WorkingCopy: true}},
			{Graph: "○  ", Entry: &jj.LogEntry{ChangeID: "bbbbbbbbbbbb", CommitID: "c2", Immutable: true}},
		}
		m.Update(run(t, m.Load()), env(true))
		open := m.OverlayOpen()

		_, fx := m.Key(press("y"), env(true))

		if !open || m.SelectedChange() != "cccccccccccc" || fx.Rewrite != nil {
			t.Errorf("after reload OverlayOpen = %v, selected = %q; Key(y) = %+v, want open, cccccccccccc and no rewrite", open, m.SelectedChange(), fx)
		}
	})

	t.Run("a is ignored while the review reloads", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, &fakeSource{lines: twoEntries()})

		if !m.Ignores(press("a"), locked(tab.Lock{Busy: true})) {
			t.Error("Ignores(a, busy) = false, want true")
		}
	})

	t.Run("a is not ignored while idle", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, &fakeSource{lines: twoEntries()})

		if m.Ignores(press("a"), env(true)) {
			t.Error("Ignores(a, idle) = true, want false")
		}
	})

	t.Run("a refuses an immutable revision", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, &fakeSource{lines: twoEntries()})
		m.Key(press("j"), env(true))

		_, fx := m.Key(press("a"), env(true))

		if fx.Status != "bbbbbbbb is immutable" || m.OverlayOpen() || m.Overlay(env(true)) != "" {
			t.Errorf("Key(a) = %+v, OverlayOpen = %v, want the immutable status and nothing armed", fx, m.OverlayOpen())
		}
	})

	t.Run("a does nothing while busy", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, &fakeSource{lines: twoEntries()})

		_, fx := m.Key(press("a"), locked(tab.Lock{Busy: true}))

		if fx != (tab.Effect{}) || m.OverlayOpen() {
			t.Errorf("Key(a) = %+v, OverlayOpen = %v, want nothing", fx, m.OverlayOpen())
		}
	})

	t.Run("a reports why the log is blocked", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, &fakeSource{lines: twoEntries()})

		_, fx := m.Key(press("a"), locked(tab.Lock{Reason: "blocked"}))

		if fx != (tab.Effect{Status: "blocked"}) || m.OverlayOpen() {
			t.Errorf("Key(a) = %+v, OverlayOpen = %v, want the reason and nothing armed", fx, m.OverlayOpen())
		}
	})

	t.Run("y closes the confirmation without abandoning while busy", func(t *testing.T) {
		t.Parallel()
		src := &fakeSource{lines: twoEntries()}
		m := loaded(t, src)
		m.Key(press("a"), env(true))

		_, fx := m.Key(press("y"), locked(tab.Lock{Busy: true}))

		if fx != (tab.Effect{}) || m.OverlayOpen() {
			t.Errorf("Key(y) = %+v, OverlayOpen = %v, want nothing and the confirmation closed", fx, m.OverlayOpen())
		}
	})

	t.Run("y reports why the log is blocked instead of abandoning", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, &fakeSource{lines: twoEntries()})
		m.Key(press("a"), env(true))

		_, fx := m.Key(press("y"), locked(tab.Lock{Reason: "blocked"}))

		if fx != (tab.Effect{Status: "blocked"}) {
			t.Errorf("Key(y) = %+v, want the reason", fx)
		}
	})
}

func TestAbandon_failed(t *testing.T) {
	t.Parallel()

	t.Run("changed bookmarks ask to review the abandon again", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, &fakeSource{lines: twoEntries(), abandonErr: jj.ErrBookmarksChanged})
		m.Key(press("a"), env(true))

		_, fx := m.Key(press("y"), env(true))
		_, err := rewrite(t, fx)

		if err == nil || err.Error() != "bookmarks changed; press a to review the abandon again" {
			t.Errorf("abandon error = %v, want the retry hint", err)
		}
	})

	t.Run("a jj failure names the revision", func(t *testing.T) {
		t.Parallel()
		errJJ := errors.New("jj abandon: exit status 1")
		m := loaded(t, &fakeSource{lines: twoEntries(), abandonErr: errJJ})
		m.Key(press("a"), env(true))

		_, fx := m.Key(press("y"), env(true))
		_, err := rewrite(t, fx)

		if !errors.Is(err, errJJ) || !strings.HasPrefix(err.Error(), "abandon aaaaaaaa: ") {
			t.Errorf("abandon error = %v, want it wrapped with the revision", err)
		}
	})
}
