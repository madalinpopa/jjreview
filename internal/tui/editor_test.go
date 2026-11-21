package tui_test

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"slices"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/madalinpopa/jjreview/internal/diff"
	"github.com/madalinpopa/jjreview/internal/jj"
	"github.com/madalinpopa/jjreview/internal/theme"
	"github.com/madalinpopa/jjreview/internal/tui"
)

func withEditor(m tui.Model, opened *[]string) tui.Model {
	return m.WithEditor(func(ctx context.Context, path string) *exec.Cmd {
		*opened = append(*opened, path)
		return exec.CommandContext(ctx, "true")
	})
}

func edit(t *testing.T, m tui.Model) (tui.Model, tea.Cmd) {
	t.Helper()
	m, check := update(t, m, keyMsg("e"))
	if check == nil {
		return m, nil
	}
	return update(t, m, check())
}

func launched(t *testing.T, m tui.Model) tui.Model {
	t.Helper()
	var opened []string
	m, cmd := edit(t, withEditor(answered(t, m), &opened))
	if got := messages(t, cmd); !slices.ContainsFunc(got, isExec(t)) {
		t.Fatalf("Update(e) messages = %v, want the editor", got)
	}
	return m
}

func TestEditor_success(t *testing.T) {
	t.Run("e opens the current file from the workspace root", func(t *testing.T) {
		var opened []string
		m := withEditor(answered(t, press(t, loaded(t, newSource()), "tab", "j", "tab")), &opened)

		_, cmd := edit(t, m)

		if cmd == nil {
			t.Fatal("edit cmd = nil, want an editor command")
		}
		if len(opened) != 1 || opened[0] != "/repo/src/main.go" {
			t.Errorf("opened = %v, want [/repo/src/main.go]", opened)
		}
	})

	t.Run("opens the file e was pressed on after moving to another file", func(t *testing.T) {
		var opened []string
		m := withEditor(answered(t, press(t, loaded(t, newSource()), "tab", "j", "tab")), &opened)
		m, check := update(t, m, keyMsg("e"))
		m = press(t, m, "J")
		if got := m.CurrentFile(); got == "src/main.go" {
			t.Fatalf("CurrentFile() = %q after J, want another file", got)
		}

		_, cmd := update(t, m, check())

		if cmd == nil {
			t.Fatal("edit cmd = nil, want an editor command")
		}
		if len(opened) != 1 || opened[0] != "/repo/src/main.go" {
			t.Errorf("opened = %v, want [/repo/src/main.go]", opened)
		}
	})

	t.Run("reloads the change after the editor exits and keeps the file", func(t *testing.T) {
		src := newSource()
		m := launched(t, press(t, loaded(t, src), "tab", "j", "tab"))

		m, cmd := update(t, m, tui.EditorFinishedMsg{})
		m = run(t, m, cmd)

		if src.diffCalls != 2 {
			t.Errorf("Diff calls = %d, want 2", src.diffCalls)
		}
		if got, want := m.CurrentFile(), "src/main.go"; got != want {
			t.Errorf("CurrentFile() = %q, want %q", got, want)
		}
	})

	t.Run("keeps shown removed lines and folds of unchanged hunks after the reload", func(t *testing.T) {
		m := launched(t, press(t, loaded(t, newSource()), "tab", "j", "tab", "d", "j", "j", "z"))

		m, cmd := update(t, m, tui.EditorFinishedMsg{})
		m = run(t, m, cmd)

		assertContains(t, view(m), "- old line", "(1 folded)")
	})
}

func TestEditor_failed(t *testing.T) {
	t.Run("a deleted file cannot be edited", func(t *testing.T) {
		var opened []string
		src := newSource()
		src.files = []diff.File{{OldPath: "gone.txt", Status: diff.StatusDeleted}}
		m := withEditor(loaded(t, src), &opened)

		m = press(t, m, "e")

		if len(opened) != 0 {
			t.Errorf("opened = %v, want nothing", opened)
		}
		assertContains(t, view(m), "file deleted; nothing to edit")
	})

	t.Run("a revision other than the working copy cannot be edited", func(t *testing.T) {
		var opened []string
		m := withEditor(press(t, loaded(t, revisionSource()), reviewParent()...), &opened)

		m = press(t, m, "e")

		if len(opened) != 0 {
			t.Errorf("opened = %v, want nothing", opened)
		}
		assertContains(t, view(m), "not the working copy; press @ to edit files")
	})

	t.Run("the working copy moved since the last load", func(t *testing.T) {
		var opened []string
		src := newSource()
		m := withEditor(press(t, loaded(t, src), "tab", "j", "tab"), &opened)
		src.revErrs = map[string]error{jj.WorkingCopyRevset("kxyzabcdefgh"): fmt.Errorf("jj log: %w", jj.ErrNoRevision)}

		m = press(t, m, "e")

		if len(opened) != 0 {
			t.Errorf("opened = %v, want nothing", opened)
		}
		assertContains(t, view(m), "working copy moved; press @ to review it")
		assertContains(t, view(press(t, m, "e")), "not the working copy; press @ to edit files")
	})

	t.Run("the working-copy check fails", func(t *testing.T) {
		var opened []string
		src := newSource()
		m := withEditor(press(t, loaded(t, src), "tab", "j", "tab"), &opened)
		src.revErrs = map[string]error{jj.WorkingCopyRevset("kxyzabcdefgh"): errors.New("jj exploded")}

		m = press(t, m, "e")

		if len(opened) != 0 {
			t.Errorf("opened = %v, want nothing", opened)
		}
		assertContains(t, view(m), "jj exploded")

		src.revErrs = nil
		press(t, m, "e")

		if len(opened) != 1 {
			t.Errorf("opened = %v after retrying, want one file", opened)
		}
	})

	t.Run("shows the editor error", func(t *testing.T) {
		m := launched(t, press(t, loaded(t, newSource()), "tab", "j", "tab"))

		m, _ = update(t, m, tui.EditorFinishedMsg{Err: errors.New("exit status 2")})

		assertContains(t, view(m), "editor: exit status 2")
	})
}

func TestEditor_reloading(t *testing.T) {
	t.Run("e does not open the editor while a revision loads", func(t *testing.T) {
		var opened []string
		m := withEditor(loaded(t, revisionSource()), &opened)
		m, _ = update(t, m, keyMsg("@"))

		press(t, m, "e")

		if len(opened) != 0 {
			t.Errorf("opened = %v, want nothing", opened)
		}
	})

	t.Run("c does not open the commit modal before the editor finishes", func(t *testing.T) {
		var opened []string
		m := withEditor(answered(t, press(t, loaded(t, newSource()), "tab", "j", "tab", "space")), &opened)

		m = press(t, m, "e", "e", "c")

		if m.ModalOpen() {
			t.Fatal("ModalOpen() = true while editing, want false")
		}
	})

	t.Run("c opens the commit modal again after an editor error", func(t *testing.T) {
		var opened []string
		m := withEditor(answered(t, press(t, loaded(t, newSource()), "tab", "j", "tab", "space")), &opened)
		m = press(t, m, "e", "e")
		m, _ = update(t, m, tui.EditorFinishedMsg{Err: errors.New("exit status 2")})

		m = press(t, m, "c")

		if !m.ModalOpen() {
			t.Error("ModalOpen() = false after editor error, want true")
		}
	})

	t.Run("c pressed during the reload waits for it", func(t *testing.T) {
		var opened []string
		m := withEditor(answered(t, press(t, loaded(t, newSource()), "tab", "j", "tab", "space")), &opened)
		m = press(t, m, "e", "e")
		m, reload := update(t, m, tui.EditorFinishedMsg{})

		m = press(t, m, "c")

		if m.ModalOpen() {
			t.Fatal("ModalOpen() = true during reload, want false")
		}

		m = press(t, run(t, m, reload), "space", "c")

		if !m.ModalOpen() {
			t.Error("ModalOpen() = false after reload, want true")
		}
	})

	t.Run("a failed reload stops reloading", func(t *testing.T) {
		var opened []string
		src := newSource()
		m := withEditor(answered(t, press(t, loaded(t, src), "tab", "j", "tab", "space")), &opened)
		m = press(t, m, "e", "e")
		m, reload := update(t, m, tui.EditorFinishedMsg{})
		src.err = errors.New("revision not found")
		m = run(t, m, reload)
		src.err = nil
		calls := src.diffCalls

		m = press(t, m, "R", "R")

		if got := src.diffCalls; got <= calls {
			t.Fatalf("Diff calls = %d after R, want a retry after %d", got, calls)
		}
		got := view(m)
		assertMissing(t, got, "reloading change…", "revision not found")
		assertContains(t, got, "src/main.go")
	})
}

func TestEditorQuit_success(t *testing.T) {
	t.Parallel()

	t.Run("q during the hold does not start the editor", func(t *testing.T) {
		t.Parallel()
		var opened []string
		m, hold := update(t, pendingEditor(t, &opened), keyMsg("e"))
		m, cmd := update(t, m, keyMsg("q"))
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Fatalf("Update(q) cmd() = %T, want tea.QuitMsg", cmd())
		}

		var got []tea.Msg
		for _, msg := range []tea.Msg{darkBg(), hold()} {
			m, cmd = update(t, m, msg)
			got = append(got, messages(t, cmd)...)
		}

		if len(got) != 0 || len(opened) != 0 {
			t.Errorf("messages after quitting = %v, opened = %v, want nothing", got, opened)
		}
	})

	t.Run("q during the working-copy check does not start the editor", func(t *testing.T) {
		t.Parallel()
		var opened []string
		m := withEditor(answered(t, press(t, loaded(t, newSource()), "tab", "j", "tab")), &opened)
		m, check := update(t, m, keyMsg("e"))
		m, _ = update(t, m, keyMsg("q"))

		_, cmd := update(t, m, check())

		if got := messages(t, cmd); len(got) != 0 || len(opened) != 0 {
			t.Errorf("messages after the check = %v, opened = %v, want nothing", got, opened)
		}
	})

}

func handingBack(t *testing.T, phase string, keys ...string) tui.Model {
	t.Helper()
	var opened []string
	m := withEditor(answered(t, press(t, loaded(t, newSource()), append([]string{"tab", "j", "tab"}, keys...)...)), &opened)
	m = press(t, m, "e")
	if len(opened) == 0 {
		m = press(t, m, "e")
	}
	if len(opened) != 1 {
		t.Fatalf("opened = %v, want the editor launched", opened)
	}
	if phase == "during the reload" {
		m, _ = update(t, m, tui.EditorFinishedMsg{})
	}
	return m
}

func TestEditorHandback_ignoresKeys(t *testing.T) {
	t.Parallel()
	for _, phase := range []string{"while the editor runs", "during the reload"} {
		t.Run(phase, func(t *testing.T) {
			t.Parallel()
			m := handingBack(t, phase, "space")
			before, cursor := view(m), m.Cursor()
			keys := []string{"j", "c", "space", "?", "e"}
			if phase == "while the editor runs" {
				keys = append(keys, "q")
			}

			for _, k := range keys {
				var cmd tea.Cmd
				m, cmd = update(t, m, keyMsg(k))
				if cmd != nil {
					t.Errorf("Update(%s) cmd = %T, want nil", k, cmd())
				}
			}

			if got := m.Cursor(); got != cursor {
				t.Errorf("Cursor() = %d, want %d", got, cursor)
			}
			if m.ModalOpen() {
				t.Error("ModalOpen() = true, want false")
			}
			if got := view(m); got != before {
				t.Errorf("View() changed:\n%s\nwant:\n%s", got, before)
			}
		})
	}
}

func TestEditorHandback_quitDuringReload(t *testing.T) {
	t.Parallel()

	t.Run("q without a selection quits", func(t *testing.T) {
		t.Parallel()
		m := handingBack(t, "during the reload")

		_, cmd := update(t, m, keyMsg("q"))

		if got := messages(t, cmd); !slices.Contains(got, tea.Msg(tea.QuitMsg{})) {
			t.Errorf("Update(q) messages = %v, want the quit", got)
		}
	})

	t.Run("q with a selection asks first", func(t *testing.T) {
		t.Parallel()
		m := handingBack(t, "during the reload", "space")

		m, cmd := update(t, m, keyMsg("q"))

		if cmd != nil {
			t.Fatalf("Update(q) cmd() = %T, want nil until confirmed", cmd())
		}
		assertContains(t, view(m), "press q again to drop 1 selected hunk")
		_, cmd = update(t, m, keyMsg("q"))
		if got := messages(t, cmd); !slices.Contains(got, tea.Msg(tea.QuitMsg{})) {
			t.Errorf("second Update(q) messages = %v, want the quit", got)
		}
	})
}

func TestEditorHandback_completionAfterQuit(t *testing.T) {
	t.Parallel()
	src := newSource()
	var opened []string
	m := withEditor(answered(t, press(t, themed(t, src, nil, theme.ModeAuto), "tab", "j", "tab")), &opened)
	m = press(t, m, "e")
	m, _ = update(t, m, keyMsg("ctrl+c"))
	calls := src.diffCalls

	_, cmd := update(t, m, tui.EditorFinishedMsg{})

	if got := messages(t, cmd); len(got) != 0 || src.diffCalls != calls {
		t.Errorf("Update(EditorFinishedMsg) messages = %v, Diff calls = %d, want nothing after quitting", got, src.diffCalls)
	}
}

func TestEditorHandback_reloadingStatus(t *testing.T) {
	t.Parallel()
	failures := map[string]error{
		"the reload lands": nil,
		"the reload fails": errors.New("revision not found"),
	}

	for name, err := range failures {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			src := newSource()
			m := launched(t, press(t, loaded(t, src), "tab", "j", "tab"))

			m, reload := update(t, m, tui.EditorFinishedMsg{})

			assertContains(t, view(m), "reloading change…")
			src.err = err
			assertMissing(t, view(run(t, m, reload)), "reloading change…")
		})
	}
}

func TestEditorHandback_ctrlCQuits(t *testing.T) {
	t.Parallel()
	for _, phase := range []string{"while the editor runs", "during the reload"} {
		t.Run(phase, func(t *testing.T) {
			t.Parallel()
			m := handingBack(t, phase)

			_, cmd := update(t, m, keyMsg("ctrl+c"))

			if got := messages(t, cmd); !slices.Contains(got, tea.Msg(tea.QuitMsg{})) {
				t.Errorf("Update(ctrl+c) messages = %v, want the quit", got)
			}
		})
	}
}

func TestEditorFinished_ignored(t *testing.T) {
	t.Parallel()

	t.Run("without a launched editor", func(t *testing.T) {
		t.Parallel()
		src := newSource()
		m := press(t, loaded(t, src), "tab", "j", "tab")

		_, cmd := update(t, m, tui.EditorFinishedMsg{})

		if got := messages(t, cmd); len(got) != 0 || src.diffCalls != 1 {
			t.Errorf("Update(EditorFinishedMsg) messages = %v, Diff calls = %d, want nothing", got, src.diffCalls)
		}
	})

	t.Run("a second completion for the same launch", func(t *testing.T) {
		t.Parallel()
		src := newSource()
		m := launched(t, press(t, loaded(t, src), "tab", "j", "tab"))
		m, _ = update(t, m, tui.EditorFinishedMsg{Err: exec.ErrNotFound})

		_, cmd := update(t, m, tui.EditorFinishedMsg{})

		if got := messages(t, cmd); len(got) != 0 || src.diffCalls != 1 {
			t.Errorf("Update(EditorFinishedMsg) messages = %v, Diff calls = %d, want nothing", got, src.diffCalls)
		}
	})
}
