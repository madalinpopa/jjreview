package bookmarks_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/madalinpopa/jjreview/internal/jj"
	"github.com/madalinpopa/jjreview/internal/tui/bookmarks"
	"github.com/madalinpopa/jjreview/internal/tui/tab"
	"github.com/madalinpopa/jjreview/internal/tui/ui"
)

var _ tab.View = (*bookmarks.Model)(nil)

func TestShow_success(t *testing.T) {
	t.Parallel()

	t.Run("loads the list on the first show", func(t *testing.T) {
		t.Parallel()
		m := bookmarks.New(t.Context(), newSource())

		cmd := m.Show(env())

		if _, _, ok := m.Update(run(t, cmd), env()); !ok {
			t.Error("Update(Show() result) handled = false, want true")
		}
	})

	t.Run("does not reload a loaded list", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, newSource())

		cmd := m.Show(env())

		if cmd != nil {
			t.Error("Show() = a command, want nil")
		}
	})
}

func TestKeepVisible_success(t *testing.T) {
	t.Parallel()
	src := &fakeSource{items: []jj.Bookmark{
		{Name: "alpha", CommitID: "a1"},
		{Name: "bravo", CommitID: "b1"},
		{Name: "charlie", CommitID: "c1"},
		{Name: "delta", CommitID: "d1"},
	}}
	m := loaded(t, src)
	e := env()
	e.ListHeight = 2
	for range 3 {
		m.Key(press("j"), e)
	}

	m.KeepVisible(e)

	if got := m.View(e); !strings.Contains(got, "delta") || strings.Contains(got, "alpha") {
		t.Errorf("View() after KeepVisible = %q, want the selected delta row in a two-row window", got)
	}
}

func TestIgnores_failed(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		key  string
		open bool
	}{
		"ignores no list key":        {key: "j"},
		"ignores no open key":        {key: "enter"},
		"ignores no key in a dialog": {key: "q", open: true},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			m := loaded(t, newSource())
			if tt.open {
				m.Key(press("n"), env())
			}

			if m.Ignores(press(tt.key), env()) {
				t.Errorf("Ignores(%q) = true, want false", tt.key)
			}
		})
	}
}

func TestCaptures_success(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		open func(m *bookmarks.Model)
		want bool
	}{
		"does not capture without an overlay": {open: func(*bookmarks.Model) {}, want: false},
		"captures while the dialog is open": {open: func(m *bookmarks.Model) {
			m.Key(press("n"), env())
		}, want: true},
		"captures while the delete confirmation is open": {open: func(m *bookmarks.Model) {
			m.Key(press("d"), env())
		}, want: true},
		"captures while the picker is open": {open: func(m *bookmarks.Model) {
			m.Pick(jj.LogEntry{ChangeID: "kkkk", CommitID: "aaaa"})
		}, want: true},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			m := loaded(t, newSource())
			tt.open(&m)

			got := m.Captures()

			if got != tt.want {
				t.Errorf("Captures() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHints_success(t *testing.T) {
	t.Parallel()
	m := loaded(t, newSource())
	tabs := ui.KeyHint{Keys: []string{"1-3"}, Label: "tabs"}
	help := ui.KeyHint{Keys: []string{"?"}, Label: "help"}

	got := m.Hints(tabs, help)

	labels := make([]string, len(got))
	for i, h := range got {
		labels[i] = h.Label
	}
	want := []string{"tabs", "bookmark", "review", "new", "edit", "delete", "refresh", "help"}
	if !slices.Equal(labels, want) {
		t.Errorf("Hints() labels = %v, want %v", labels, want)
	}
}
