package operations_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/madalinpopa/jjreview/internal/jj"
	"github.com/madalinpopa/jjreview/internal/theme"
	"github.com/madalinpopa/jjreview/internal/tui/operations"
	"github.com/madalinpopa/jjreview/internal/tui/tab"
	"github.com/madalinpopa/jjreview/internal/tui/ui"
)

type fakeSource struct {
	items  []jj.Operation
	err    error
	calls  int
	runs   []string
	runErr error
}

func (s *fakeSource) Operations(context.Context) ([]jj.Operation, error) {
	s.calls++
	return slices.Clone(s.items), s.err
}

func (s *fakeSource) RestoreOperation(_ context.Context, id string) error {
	s.runs = append(s.runs, "restore "+id)
	return s.runErr
}

func (s *fakeSource) RevertOperation(_ context.Context, id string) error {
	s.runs = append(s.runs, "revert "+id)
	return s.runErr
}

func (s *fakeSource) AbandonOperation(_ context.Context, id string) error {
	s.runs = append(s.runs, "abandon "+id)
	return s.runErr
}

func newSource() *fakeSource {
	return &fakeSource{items: []jj.Operation{
		{ID: "aaaa1111cccc", Description: "describe commit 123", Args: "jj describe -m wip", User: "me@host", Time: "1 minute ago", Current: true},
		{ID: "bbbb2222dddd", Description: "snapshot working copy", Args: "jj status", User: "me@host", Time: "2 minutes ago", Snapshot: true},
		{ID: "cccc3333eeee", Description: "new empty commit", Args: "jj new", User: "me@host", Time: "3 minutes ago"},
		{ID: "000000000000", Root: true},
	}}
}

func env(width, listHeight int) operations.Env {
	return operations.Env{
		Theme:      theme.Default(),
		Width:      width,
		Height:     listHeight + ui.CardChrome + 2,
		BodyHeight: listHeight + ui.CardChrome,
		ListHeight: listHeight,
		ModalWidth: 64,
		Active:     true,
	}
}

func run(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	if cmd == nil {
		t.Fatal("cmd = nil, want a command")
	}
	return cmd()
}

func loaded(t *testing.T, src *fakeSource) operations.Model {
	t.Helper()
	m := operations.New(t.Context(), src)
	if _, _, ok := m.Update(run(t, m.Show()), env(80, 10)); !ok {
		t.Fatal("Update(list result) handled = false, want true")
	}
	return m
}

func keyMsg(k string) tea.KeyPressMsg {
	switch k {
	case "ctrl+d", "ctrl+u":
		return tea.KeyPressMsg{Code: rune(k[len(k)-1]), Mod: tea.ModCtrl}
	case "home":
		return tea.KeyPressMsg{Code: tea.KeyHome}
	case "end":
		return tea.KeyPressMsg{Code: tea.KeyEnd}
	}
	return tea.KeyPressMsg{Code: rune(k[0]), Text: k}
}

func press(m *operations.Model, e operations.Env, keys ...string) operations.Effect {
	var eff operations.Effect
	for _, k := range keys {
		_, eff = m.Key(keyMsg(k), e)
		m.KeepVisible(e)
	}
	return eff
}

func screen(m operations.Model, e operations.Env) string {
	return ansi.Strip(m.View(e))
}

func overlay(m operations.Model, e operations.Env) string {
	return ansi.Strip(m.Overlay(e))
}

func TestLoad_success(t *testing.T) {
	t.Parallel()

	t.Run("Show loads the operations once", func(t *testing.T) {
		t.Parallel()
		src := newSource()
		m := loaded(t, src)

		cmd := m.Show()

		if cmd != nil {
			t.Error("Show() after a load = a command, want nil")
		}
		if src.calls != 1 {
			t.Errorf("Operations() calls = %d, want 1", src.calls)
		}
		if got := m.SelectedID(); got != "aaaa1111cccc" {
			t.Errorf("SelectedID() = %q, want the newest operation", got)
		}
	})

	t.Run("Show reloads stale operations", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, newSource())
		m.MarkStale()

		cmd := m.Show()

		if cmd == nil {
			t.Error("Show() on stale operations = nil, want a load")
		}
	})

	t.Run("a reload keeps the selection on the same operation", func(t *testing.T) {
		t.Parallel()
		src := newSource()
		m := loaded(t, src)
		press(&m, env(80, 10), "j", "j")
		src.items = append([]jj.Operation{{ID: "ffff9999ffff", Description: "restore to operation cccc3333eeee", Current: true}}, src.items...)

		m.Update(run(t, m.Load()), env(80, 10))

		if got := m.SelectedID(); got != "cccc3333eeee" {
			t.Errorf("SelectedID() = %q, want cccc3333eeee", got)
		}
	})

	t.Run("a reload keeps the position when the operation is gone", func(t *testing.T) {
		t.Parallel()
		src := newSource()
		m := loaded(t, src)
		press(&m, env(80, 10), "j")
		src.items = slices.Delete(src.items, 1, 2)

		m.Update(run(t, m.Load()), env(80, 10))

		if got := m.SelectedID(); got != "cccc3333eeee" {
			t.Errorf("SelectedID() = %q, want cccc3333eeee", got)
		}
	})

	t.Run("an outdated load result is ignored", func(t *testing.T) {
		t.Parallel()
		src := newSource()
		m := operations.New(t.Context(), src)
		first := run(t, m.Load())
		src.items = src.items[:1]
		second := run(t, m.Load())

		m.Update(second, env(80, 10))
		_, _, ok := m.Update(first, env(80, 10))

		if !ok {
			t.Error("Update(outdated result) handled = false, want true")
		}
		if got := screen(m, env(80, 10)); !strings.Contains(got, "operation 1/1") {
			t.Errorf("View() = %q, want only the newer result", got)
		}
	})

	t.Run("a result that went stale while loading reloads when active", func(t *testing.T) {
		t.Parallel()
		m := operations.New(t.Context(), newSource())
		msg := run(t, m.Load())
		m.MarkStale()

		cmd, _, _ := m.Update(msg, env(80, 10))

		if cmd == nil {
			t.Error("Update(stale result, active) = nil, want a reload")
		}
	})

	t.Run("a result that went stale while loading waits when inactive", func(t *testing.T) {
		t.Parallel()
		m := operations.New(t.Context(), newSource())
		msg := run(t, m.Load())
		m.MarkStale()

		inactive := env(80, 10)
		inactive.Active = false

		cmd, _, _ := m.Update(msg, inactive)

		if cmd != nil {
			t.Error("Update(stale result, inactive) = a command, want nil")
		}
	})

	t.Run("the list shows each operation on one row", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, newSource())

		got := screen(m, env(80, 10))

		for _, want := range []string{"OPERATIONS", "operation 1/4", "aaaa1111", "describe commit 123", "1 minute ago", "bbbb2222", "cccc3333", "00000000", "root"} {
			if !strings.Contains(got, want) {
				t.Errorf("View() missing %q:\n%s", want, got)
			}
		}
		if strings.Contains(got, "jj describe -m wip") {
			t.Errorf("View() shows the command before l:\n%s", got)
		}
	})
}

func TestLoad_failed(t *testing.T) {
	t.Parallel()

	t.Run("a failed load shows the error", func(t *testing.T) {
		t.Parallel()
		src := newSource()
		src.err = errors.New("boom")

		m := loaded(t, src)

		if got := screen(m, env(80, 10)); !strings.Contains(got, "error: load operations: boom") {
			t.Errorf("View() = %q, want the load error", got)
		}
	})

	t.Run("Update ignores other messages", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, newSource())

		cmd, eff, ok := m.Update(tea.WindowSizeMsg{Width: 10, Height: 10}, env(80, 10))

		if ok || cmd != nil || eff != (operations.Effect{}) {
			t.Errorf("Update(WindowSizeMsg) = (%v, %+v, %t), want (nil, {}, false)", cmd, eff, ok)
		}
	})
}

func TestKey_success(t *testing.T) {
	t.Parallel()

	t.Run("j, G and g move between operations", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, newSource())
		e := env(80, 10)

		press(&m, e, "j")
		afterJ := m.SelectedID()
		press(&m, e, "G")
		afterG := m.SelectedID()
		press(&m, e, "g")

		if afterJ != "bbbb2222dddd" || afterG != "000000000000" || m.SelectedID() != "aaaa1111cccc" {
			t.Errorf("SelectedID() after j, G, g = %q, %q, %q, want bbbb2222dddd, 000000000000, aaaa1111cccc", afterJ, afterG, m.SelectedID())
		}
	})

	t.Run("R asks for a refresh", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, newSource())

		eff := press(&m, env(80, 10), "R")

		if !eff.Refresh {
			t.Errorf("Key(R) = %+v, want a refresh", eff)
		}
	})

	t.Run("R while loading does not ask for another refresh", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, newSource())
		m.Load()

		eff := press(&m, env(80, 10), "R")

		if eff.Refresh {
			t.Errorf("Key(R) while loading = %+v, want no refresh", eff)
		}
	})

	tests := map[string]struct {
		key  string
		run  string
		want tab.Result
	}{
		"r then y restores": {key: "r", run: "restore cccc3333eeee", want: tab.Result{Done: "restored operation cccc3333", Replaced: true, Restored: true}},
		"u then y undoes":   {key: "u", run: "revert cccc3333eeee", want: tab.Result{Done: "undid operation cccc3333", Replaced: true, Restored: true}},
		"a then y abandons": {key: "a", run: "abandon cccc3333eeee", want: tab.Result{Done: "abandoned operation cccc3333", KeepPicks: true}},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			src := newSource()
			m := loaded(t, src)
			e := env(80, 10)
			press(&m, e, "j", "j", tt.key)
			opened := m.OverlayOpen()

			eff := press(&m, e, "y")

			if !opened {
				t.Errorf("OverlayOpen() after %s = false, want true", tt.key)
			}
			if m.OverlayOpen() {
				t.Error("OverlayOpen() after y = true, want false")
			}
			if eff.Rewrite == nil {
				t.Fatalf("Key(y) = %+v, want a rewrite", eff)
			}
			if len(src.runs) != 0 {
				t.Errorf("runs before the rewrite = %v, want none", src.runs)
			}
			got, err := eff.Rewrite.Run(t.Context(), tab.Head{CommitID: "commit1", WorkingCopy: true})
			if err != nil {
				t.Fatalf("Rewrite.Run() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("Rewrite.Run() = %+v, want %+v", got, tt.want)
			}
			if !slices.Equal(src.runs, []string{tt.run}) {
				t.Errorf("runs = %v, want [%s]", src.runs, tt.run)
			}
		})
	}

	for _, k := range []string{"r", "u", "a"} {
		t.Run(k+" off the working copy does not replace the reviewed commit", func(t *testing.T) {
			t.Parallel()
			m := loaded(t, newSource())

			eff := press(&m, env(80, 10), "j", "j", k, "y")

			if eff.Rewrite == nil {
				t.Fatalf("Key(y) = %+v, want a rewrite", eff)
			}
			got, err := eff.Rewrite.Run(t.Context(), tab.Head{CommitID: "commit1"})
			if err != nil {
				t.Fatalf("Rewrite.Run() error = %v", err)
			}
			if got.Replaced {
				t.Error("Rewrite.Run() Replaced = true, want false")
			}
		})
	}

	t.Run("any other key cancels the confirmation", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, newSource())
		e := env(80, 10)
		press(&m, e, "j", "j", "r")

		eff := press(&m, e, "n")

		if eff != (operations.Effect{}) || m.OverlayOpen() {
			t.Errorf("Key(n) = %+v, OverlayOpen() = %t, want nothing to run and the prompt closed", eff, m.OverlayOpen())
		}
	})

	t.Run("a held y does not confirm", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, newSource())
		e := env(80, 10)
		press(&m, e, "j", "j", "r")

		_, eff := m.Key(tea.KeyPressMsg{Code: 'y', Text: "y", IsRepeat: true}, e)

		if eff.Rewrite != nil {
			t.Errorf("Key(repeated y) = %+v, want nothing to run", eff)
		}
	})

	t.Run("action keys are ignored while reloading or held", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, newSource())
		held := tea.KeyPressMsg{Code: 'r', Text: "r", IsRepeat: true}
		idle := env(80, 10)
		busy := env(80, 10)
		busy.Lock = tab.Lock{Busy: true}

		if !m.Ignores(keyMsg("r"), busy) {
			t.Error("Ignores(r, busy) = false, want true")
		}
		if !m.Ignores(held, idle) {
			t.Error("Ignores(held r) = false, want true")
		}
		if m.Ignores(keyMsg("r"), idle) {
			t.Error("Ignores(r) = true, want false")
		}
		if m.Ignores(keyMsg("j"), busy) {
			t.Error("Ignores(j, busy) = true, want false")
		}
	})
}

func TestCaptures_success(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		keys []string
		want bool
	}{
		"browsing the list":               {keys: []string{"j"}, want: false},
		"the confirmation is open":        {keys: []string{"j", "u"}, want: true},
		"the confirmation was answered":   {keys: []string{"j", "u", "n"}, want: false},
		"an action refused with a status": {keys: []string{"r"}, want: false},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			m := loaded(t, newSource())

			press(&m, env(80, 10), tt.keys...)

			if got := m.Captures(); got != tt.want {
				t.Errorf("Captures() after %v = %t, want %t", tt.keys, got, tt.want)
			}
		})
	}
}

func TestKey_failed(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		keys   []string
		status string
	}{
		"restoring the current operation":  {keys: []string{"r"}, status: "already at operation aaaa1111"},
		"abandoning the current operation": {keys: []string{"a"}, status: "the current operation cannot be abandoned; press u to undo it first"},
		"undoing the root operation":       {keys: []string{"G", "u"}, status: "the root operation cannot be undone"},
		"abandoning the root operation":    {keys: []string{"G", "a"}, status: "the root operation cannot be abandoned"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			m := loaded(t, newSource())

			eff := press(&m, env(80, 10), tt.keys...)

			if eff.Status != tt.status || eff.Rewrite != nil {
				t.Errorf("Key(%v) = %+v, want status %q", tt.keys, eff, tt.status)
			}
			if m.OverlayOpen() {
				t.Error("OverlayOpen() = true, want no prompt")
			}
		})
	}

	t.Run("actions on an empty list do nothing", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, &fakeSource{})

		eff := press(&m, env(80, 10), "r")

		if eff != (operations.Effect{}) || m.OverlayOpen() {
			t.Errorf("Key(r) on no operations = %+v, OverlayOpen() = %t, want nothing", eff, m.OverlayOpen())
		}
	})
}

func TestConfirm_failed(t *testing.T) {
	t.Parallel()

	t.Run("y while the review is busy closes the prompt without a rewrite", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, newSource())
		e := env(80, 10)
		press(&m, e, "j", "j", "r")
		e.Lock = tab.Lock{Busy: true}

		eff := press(&m, e, "y")

		if eff != (operations.Effect{}) || m.OverlayOpen() {
			t.Errorf("Key(y) while busy = %+v, OverlayOpen() = %t, want nothing and the prompt closed", eff, m.OverlayOpen())
		}
	})

	t.Run("y while the review is blocked reports why", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, newSource())
		e := env(80, 10)
		press(&m, e, "j", "j", "r")
		e.Lock = tab.Lock{Reason: "blocked"}

		eff := press(&m, e, "y")

		if eff.Status != "blocked" || eff.Rewrite != nil || m.OverlayOpen() {
			t.Errorf("Key(y) while blocked = %+v, OverlayOpen() = %t, want status blocked and the prompt closed", eff, m.OverlayOpen())
		}
	})

	t.Run("y after the selected operation is gone does not rewrite", func(t *testing.T) {
		t.Parallel()
		src := newSource()
		m := loaded(t, src)
		e := env(80, 10)
		press(&m, e, "j", "j", "r")
		src.items = slices.Delete(src.items, 2, 3)
		m.Update(run(t, m.Load()), e)

		eff := press(&m, e, "y")

		if eff.Rewrite != nil {
			t.Errorf("Key(y) = %+v, want no rewrite", eff)
		}
	})

	errs := map[string]struct {
		key  string
		want string
	}{
		"restore": {key: "r", want: "restore operation cccc3333: boom"},
		"undo":    {key: "u", want: "undo operation cccc3333: boom"},
		"abandon": {key: "a", want: "abandon operation cccc3333: boom"},
	}
	for name, tt := range errs {
		t.Run("a failed "+name+" keeps the picks and names the operation", func(t *testing.T) {
			t.Parallel()
			boom := errors.New("boom")
			src := newSource()
			src.runErr = boom
			m := loaded(t, src)

			eff := press(&m, env(80, 10), "j", "j", tt.key, "y")

			if eff.Rewrite == nil {
				t.Fatalf("Key(y) = %+v, want a rewrite", eff)
			}
			got, err := eff.Rewrite.Run(t.Context(), tab.Head{CommitID: "commit1", WorkingCopy: true})
			if !errors.Is(err, boom) || err.Error() != tt.want {
				t.Errorf("Rewrite.Run() error = %v, want %q wrapping boom", err, tt.want)
			}
			if got != (tab.Result{KeepPicks: true}) {
				t.Errorf("Rewrite.Run() = %+v, want only KeepPicks", got)
			}
		})
	}
}

func TestOverlay_success(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		key     string
		title   string
		summary string
	}{
		"restore": {key: "r", title: "Restore operation cccc3333", summary: "Returns the repo to its state after this operation"},
		"undo":    {key: "u", title: "Undo operation cccc3333", summary: "Applies the inverse of this operation"},
		"abandon": {key: "a", title: "Abandon operation cccc3333", summary: "Removes the operation from the history"},
	}
	for name, tt := range tests {
		t.Run(name+" names the operation and what it does", func(t *testing.T) {
			t.Parallel()
			m := loaded(t, newSource())
			e := env(80, 10)
			press(&m, e, "j", "j", tt.key)

			got := overlay(m, e)

			for _, want := range []string{tt.title, "new empty commit", "jj new", "3 minutes ago by me@host", tt.summary, "y " + name + " · any other key cancels"} {
				if !strings.Contains(got, want) {
					t.Errorf("Overlay() missing %q:\n%s", want, got)
				}
			}
		})
	}

	dropped := map[string]struct {
		key  string
		want bool
	}{
		"restore": {key: "r", want: true},
		"undo":    {key: "u", want: true},
		"abandon": {key: "a", want: false},
	}
	for name, tt := range dropped {
		t.Run(name+" warns about selected hunks only when it drops them", func(t *testing.T) {
			t.Parallel()
			m := loaded(t, newSource())
			e := env(80, 10)
			e.Dropped = 2
			press(&m, e, "j", "j", tt.key)

			got := strings.Contains(overlay(m, e), "Drops 2 selected hunks")

			if got != tt.want {
				t.Errorf("Overlay() warns = %t, want %t", got, tt.want)
			}
		})
	}

	t.Run("a long command does not push the warning out of the prompt", func(t *testing.T) {
		t.Parallel()
		src := newSource()
		src.items[2].Args = "jj describe -m \"" + strings.Repeat("a long line of the commit message body\n", 40) + "\""
		m := loaded(t, src)
		e := env(80, 10)
		e.Dropped = 1
		press(&m, e, "j", "j", "r")

		got := overlay(m, e)

		for _, want := range []string{"Restore operation cccc3333", "Returns the repo to its state", "Drops 1 selected hunk", "y restore", "…"} {
			if !strings.Contains(got, want) {
				t.Errorf("Overlay() missing %q:\n%s", want, got)
			}
		}
		if h := lipgloss.Height(got); h > e.Height {
			t.Errorf("Overlay() height = %d, want at most %d", h, e.Height)
		}
	})

	t.Run("the prompt keeps every character of a long command", func(t *testing.T) {
		t.Parallel()
		src := newSource()
		src.items[2].Args = "jj squash --from feature/operations --into feature/review-operations -- internal/tui/model.go"
		m := loaded(t, src)
		e := env(80, 20)
		press(&m, e, "j", "j", "r")

		got := overlay(m, e)

		if n := strings.Count(got, "--"); n != 3 {
			t.Errorf("Overlay() shows %d of the 3 \"--\" in the command:\n%s", n, got)
		}
		if !strings.Contains(got, "internal/tui/model.go") {
			t.Errorf("Overlay() missing the last argument:\n%s", got)
		}
	})

	t.Run("no prompt renders nothing", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, newSource())

		if got := m.Overlay(env(80, 10)); got != "" {
			t.Errorf("Overlay() = %q, want empty", got)
		}
	})
}

func TestExpand_success(t *testing.T) {
	t.Parallel()

	t.Run("l shows the command under the operation and h hides it", func(t *testing.T) {
		t.Parallel()
		src := newSource()
		src.items[0].Args = "jj describe -m \"first\nsecond\""
		m := loaded(t, src)
		e := env(80, 10)

		press(&m, e, "l")
		shown := screen(m, e)
		press(&m, e, "h")
		hidden := screen(m, e)

		if !strings.Contains(shown, "jj describe -m \"first") || !strings.Contains(shown, "second\"") {
			t.Errorf("View() after l missing the command lines:\n%s", shown)
		}
		if strings.Contains(hidden, "jj describe") {
			t.Errorf("View() after h still shows the command:\n%s", hidden)
		}
	})

	t.Run("j and k still move by operation over an expanded command", func(t *testing.T) {
		t.Parallel()
		src := newSource()
		src.items[0].Args = "jj a\nb\nc"
		m := loaded(t, src)
		e := env(80, 10)

		press(&m, e, "l", "j")

		if got := m.SelectedID(); got != "bbbb2222dddd" {
			t.Errorf("SelectedID() = %q, want bbbb2222dddd", got)
		}
	})

	t.Run("an operation without a command says so", func(t *testing.T) {
		t.Parallel()
		m := loaded(t, newSource())
		e := env(80, 10)

		press(&m, e, "G", "l")

		if got := screen(m, e); !strings.Contains(got, "no command recorded") {
			t.Errorf("View() = %q, want no command recorded", got)
		}
	})

	t.Run("a long command wraps to the width without losing characters", func(t *testing.T) {
		t.Parallel()
		src := newSource()
		src.items[0].Args = "jj squash --from feature/operations --into feature/review-operations -- internal/tui/model.go"
		m := loaded(t, src)
		e := env(80, 10)

		press(&m, e, "l")
		got := screen(m, e)

		if n := strings.Count(got, "--"); n != 3 {
			t.Errorf("View() shows %d of the 3 \"--\" in the command:\n%s", n, got)
		}
		if !strings.Contains(got, "internal/tui/model.go") {
			t.Errorf("View() missing the last argument:\n%s", got)
		}
		for line := range strings.Lines(got) {
			if w := lipgloss.Width(strings.TrimSuffix(line, "\n")); w > e.Width {
				t.Errorf("View() line width = %d, want at most %d: %q", w, e.Width, line)
			}
		}
	})

	t.Run("backslashes in the command are shown unchanged", func(t *testing.T) {
		t.Parallel()
		src := newSource()
		src.items[0].Args = `jj describe -m 'don'\''t'`
		m := loaded(t, src)
		e := env(80, 10)

		press(&m, e, "l")

		if got := screen(m, e); !strings.Contains(got, `jj describe -m 'don'\''t'`) {
			t.Errorf("View() does not show the command unchanged:\n%s", got)
		}
	})

	t.Run("control characters in the command are escaped", func(t *testing.T) {
		t.Parallel()
		src := newSource()
		src.items[0].Args = "jj describe -m \"a\tb\x1b[31m\""
		m := loaded(t, src)
		e := env(80, 10)

		press(&m, e, "l")

		if got := screen(m, e); !strings.Contains(got, `a\tb\x1b[31m`) {
			t.Errorf("View() does not escape control characters:\n%s", got)
		}
	})

	t.Run("an expanded command stays open after a reload", func(t *testing.T) {
		t.Parallel()
		src := newSource()
		m := loaded(t, src)
		e := env(80, 10)
		press(&m, e, "j", "j", "l")

		m.Update(run(t, m.Load()), env(80, 10))

		if got := screen(m, e); !strings.Contains(got, "jj new") {
			t.Errorf("View() after reload = %q, want the command still shown", got)
		}
	})

	t.Run("a reload forgets commands of operations that are gone", func(t *testing.T) {
		t.Parallel()
		src := newSource()
		m := loaded(t, src)
		e := env(80, 10)
		press(&m, e, "j", "j", "l")
		all := slices.Clone(src.items)
		src.items = slices.Delete(src.items, 2, 3)
		m.Update(run(t, m.Load()), env(80, 10))
		src.items = all

		m.Update(run(t, m.Load()), env(80, 10))

		if got := screen(m, e); strings.Contains(got, "jj new") {
			t.Errorf("View() = %q, want the forgotten command hidden", got)
		}
	})

	t.Run("expanding the last operation scrolls its command into view", func(t *testing.T) {
		t.Parallel()
		src := newSource()
		src.items[3].Args = "jj a\nb\nc\nd\nlast line"
		m := loaded(t, src)
		e := env(80, 6)

		press(&m, e, "G", "l")
		got := screen(m, e)

		if !strings.Contains(got, "00000000") || !strings.Contains(got, "last line") {
			t.Errorf("View() = %q, want the root operation and its whole command", got)
		}
	})

	t.Run("half page down pages through a long command, then moves on", func(t *testing.T) {
		t.Parallel()
		src := newSource()
		src.items[0].Args = strings.Repeat("line\n", 30) + "end"
		m := loaded(t, src)
		e := env(80, 10)
		press(&m, e, "l", "ctrl+d")
		inside := m.SelectedID()

		for range 10 {
			if m.SelectedID() != "aaaa1111cccc" {
				break
			}
			press(&m, e, "ctrl+d")
		}

		if inside != "aaaa1111cccc" {
			t.Errorf("SelectedID() after one page = %q, want aaaa1111cccc", inside)
		}
		if got := m.SelectedID(); got != "bbbb2222dddd" {
			t.Errorf("SelectedID() after paging = %q, want bbbb2222dddd", got)
		}
	})

	reveal := map[string]struct {
		setup  []string
		key    string
		header string
	}{
		"g reveals the newest operation":    {setup: []string{"l", "ctrl+d"}, key: "g", header: "aaaa1111"},
		"home reveals the newest operation": {setup: []string{"l", "ctrl+d"}, key: "home", header: "aaaa1111"},
		"k reveals the expanded operation":  {setup: []string{"l", "ctrl+d", "ctrl+d", "ctrl+d", "ctrl+d", "ctrl+d", "ctrl+d"}, key: "k", header: "aaaa1111"},
	}
	for name, tt := range reveal {
		t.Run(name+" header from inside its command", func(t *testing.T) {
			t.Parallel()
			src := newSource()
			src.items[0].Args = strings.Repeat("line\n", 30) + "end"
			m := loaded(t, src)
			e := env(80, 10)
			press(&m, e, tt.setup...)
			if strings.Contains(screen(m, e), tt.header) {
				t.Fatalf("View() before %s already shows %s:\n%s", tt.key, tt.header, screen(m, e))
			}

			press(&m, e, tt.key)

			if got := screen(m, e); !strings.Contains(got, tt.header) {
				t.Errorf("View() after %s missing %s:\n%s", tt.key, tt.header, got)
			}
		})
	}
}

func TestHelp_success(t *testing.T) {
	t.Parallel()
	m := operations.New(t.Context(), newSource())

	var got []string
	for _, h := range slices.Concat(m.Hints(), m.Help()) {
		got = append(got, h.Label)
	}

	for _, want := range []string{"command", "restore", "undo", "abandon", "refresh", "show the command of the operation", "hide the command of the operation", "restore the repo to the operation (y confirms)"} {
		if !slices.Contains(got, want) {
			t.Errorf("Hints() and Help() labels = %v, missing %q", got, want)
		}
	}
}
