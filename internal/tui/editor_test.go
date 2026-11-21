package tui_test

import (
	"context"
	"errors"
	"os/exec"
	"testing"

	"github.com/madalinpopa/jjreview/internal/diff"
	"github.com/madalinpopa/jjreview/internal/tui"
)

func withEditor(m tui.Model, opened *[]string) tui.Model {
	return m.WithEditor(func(ctx context.Context, path string) *exec.Cmd {
		*opened = append(*opened, path)
		return exec.CommandContext(ctx, "true")
	})
}

func TestEditor_success(t *testing.T) {
	t.Run("e opens the current file from the workspace root", func(t *testing.T) {
		var opened []string
		m := withEditor(press(t, loaded(t, newSource()), "tab", "j", "tab"), &opened)

		_, cmd := update(t, m, keyMsg("e"))

		if cmd == nil {
			t.Fatal("Update(e) cmd = nil, want an editor command")
		}
		if len(opened) != 1 || opened[0] != "/repo/src/main.go" {
			t.Errorf("opened = %v, want [/repo/src/main.go]", opened)
		}
	})

	t.Run("reloads the change after the editor exits and keeps the file", func(t *testing.T) {
		src := newSource()
		m := press(t, loaded(t, src), "tab", "j", "tab")

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
		m := press(t, loaded(t, newSource()), "tab", "j", "tab", "d", "j", "j", "z")

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

	t.Run("shows the editor error", func(t *testing.T) {
		m := loaded(t, newSource())

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

	t.Run("c does not open the commit modal until the reload arrives", func(t *testing.T) {
		var opened []string
		m := withEditor(answered(t, press(t, loaded(t, newSource()), "tab", "j", "tab", "space")), &opened)
		m = press(t, m, "e", "e")
		m, reload := update(t, m, tui.EditorFinishedMsg{})

		m = press(t, m, "c")

		if m.ModalOpen() {
			t.Fatal("ModalOpen() = true during reload, want false")
		}
		assertContains(t, view(m), "reloading change…")

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

		m = press(t, run(t, m, reload), "c")

		assertMissing(t, view(m), "reloading change…")
	})
}
