package bookmarks_test

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/madalinpopa/jjreview/internal/jj"
	"github.com/madalinpopa/jjreview/internal/tui/bookmarks"
)

type fakeSource struct {
	bookmarks.Source
	items []jj.Bookmark
}

func (s *fakeSource) Bookmarks(context.Context) ([]jj.Bookmark, error) {
	return append([]jj.Bookmark(nil), s.items...), nil
}

func (s *fakeSource) Change(context.Context, string) (jj.Change, error) {
	return jj.Change{CommitID: "c0ffee"}, nil
}

func (s *fakeSource) CreateBookmark(_ context.Context, name, _ string) (string, error) {
	s.items = append(s.items, jj.Bookmark{Name: name, CommitID: "c0ffee"})
	return "", nil
}

func (s *fakeSource) DeleteBookmark(context.Context, string) (string, error) {
	s.items = s.items[:0]
	return "", nil
}

func newSource() *fakeSource {
	return &fakeSource{items: []jj.Bookmark{{Name: "main", ChangeID: "kkkk", CommitID: "aaaa"}}}
}

func env() bookmarks.Env {
	return bookmarks.Env{Width: 80, Height: 24, BodyHeight: 20, ListHeight: 16, ModalWidth: 64, Head: "aaaa", Active: true}
}

func run(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()

	if cmd == nil {
		t.Fatal("cmd = nil, want a command")
	}
	return cmd()
}

func loaded(t *testing.T, src *fakeSource) bookmarks.Model {
	t.Helper()

	m := bookmarks.New(t.Context(), src)
	if _, _, ok := m.Update(run(t, m.Show()), env()); !ok {
		t.Fatal("Update(list result) handled = false, want true")
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

	t.Run("handles the result of its own list load", func(t *testing.T) {
		t.Parallel()
		m := bookmarks.New(t.Context(), newSource())
		msg := run(t, m.Show())

		_, _, ok := m.Update(msg, env())

		if !ok {
			t.Errorf("Update(%T) handled = false, want true", msg)
		}
		if m.SelectedName() != "main" {
			t.Errorf("SelectedName() = %q, want %q", m.SelectedName(), "main")
		}
	})

	t.Run("handles the result of a delete", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, newSource())
		m.Key(press("d"), env())
		cmd, _ := m.Key(press("y"), env())
		msg := run(t, cmd)

		_, eff, ok := m.Update(msg, env())

		if !ok {
			t.Errorf("Update(%T) handled = false, want true", msg)
		}
		if eff.Status != "✓ deleted main" || !eff.Refresh {
			t.Errorf("Update(%T) effect = %+v, want deleted status and a refresh", msg, eff)
		}
	})

	t.Run("handles the result of a save", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, newSource())
		m.Key(press("n"), env())
		m.Paste(tea.PasteMsg{Content: "feature"})
		cmd, _ := m.Key(press("enter"), env())
		msg := run(t, cmd)

		_, eff, ok := m.Update(msg, env())

		if !ok {
			t.Errorf("Update(%T) handled = false, want true", msg)
		}
		if eff.Status != "✓ saved feature" || !eff.Refresh {
			t.Errorf("Update(%T) effect = %+v, want saved status and a refresh", msg, eff)
		}
	})
}

func TestUpdate_failed(t *testing.T) {
	t.Parallel()

	tests := map[string]tea.Msg{
		"window size": tea.WindowSizeMsg{Width: 10, Height: 10},
		"focus":       tea.FocusMsg{},
		"key press":   press("j"),
	}

	for name, msg := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			m := loaded(t, newSource())

			cmd, eff, ok := m.Update(msg, env())

			if ok || cmd != nil || eff != (bookmarks.Effect{}) {
				t.Errorf("Update(%T) = %v, %+v, %v, want nil, zero effect, false", msg, cmd, eff, ok)
			}
		})
	}
}

func TestKeyOpen_success(t *testing.T) {
	t.Parallel()

	t.Run("reports the first open of a target as a new target", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, newSource())

		_, eff := m.Key(press("enter"), env())

		if eff.Open != "aaaa" || eff.Reopen {
			t.Errorf("Key(enter) effect = %+v, want Open aaaa without Reopen", eff)
		}
	})

	t.Run("reports a repeated open of the same target as a reopen", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, newSource())
		m.Key(press("enter"), env())

		_, eff := m.Key(press("enter"), env())

		if eff.Open != "aaaa" || !eff.Reopen {
			t.Errorf("Key(enter) effect = %+v, want Open aaaa and Reopen", eff)
		}
	})

	t.Run("reports an open after the target moved as a new target", func(t *testing.T) {
		t.Parallel()
		src := newSource()
		m := loaded(t, src)
		m.Key(press("enter"), env())
		src.items[0].CommitID = "bbbb"
		m.Update(run(t, m.Load()), env())

		_, eff := m.Key(press("enter"), env())

		if eff.Open != "bbbb" || eff.Reopen {
			t.Errorf("Key(enter) effect = %+v, want Open bbbb without Reopen", eff)
		}
	})
}

func TestReloadRetarget_success(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		moved  bool
		active bool
		want   bool
	}{
		"reports a retarget when the opened target moved":         {moved: true, active: true, want: true},
		"reports no retarget when the opened target did not move": {moved: false, active: true, want: false},
		"reports no retarget while the tab is not visible":        {moved: true, active: false, want: false},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			src := newSource()
			m := loaded(t, src)
			m.Key(press("enter"), env())
			if tt.moved {
				src.items[0].CommitID = "bbbb"
			}
			e := env()
			e.Active = tt.active

			_, eff, _ := m.Update(run(t, m.Load()), e)

			if eff.Retarget != tt.want {
				t.Errorf("Update(reload) Retarget = %v, want %v", eff.Retarget, tt.want)
			}
		})
	}
}

func TestOverlayOpen_success(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		open func(m *bookmarks.Model)
		want bool
	}{
		"is closed without an overlay": {open: func(*bookmarks.Model) {}, want: false},
		"is open while the dialog is open": {open: func(m *bookmarks.Model) {
			m.Key(press("n"), env())
		}, want: true},
		"is open while the delete confirmation is open": {open: func(m *bookmarks.Model) {
			m.Key(press("d"), env())
		}, want: true},
		"is open while the picker is open": {open: func(m *bookmarks.Model) {
			m.Pick(jj.LogEntry{ChangeID: "kkkk", CommitID: "aaaa"})
		}, want: true},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			m := loaded(t, newSource())
			tt.open(&m)

			got := m.OverlayOpen()

			if got != tt.want {
				t.Errorf("OverlayOpen() = %v, want %v", got, tt.want)
			}
		})
	}
}
