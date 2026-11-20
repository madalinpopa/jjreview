package tui_test

import (
	"errors"
	"testing"

	"github.com/madalinpopa/jjreview/internal/diff"
)

func TestReload_success(t *testing.T) {
	t.Run("R loads the new changes of the revision", func(t *testing.T) {
		src := newSource()
		m := loaded(t, src)
		src.files = append(src.files, diff.File{OldPath: "notes.txt", NewPath: "notes.txt", Binary: true})

		m = press(t, m, "R")

		assertContains(t, view(m), "notes.txt")
	})

	t.Run("R keeps the current file", func(t *testing.T) {
		m := press(t, loaded(t, newSource()), "ctrl+h", "j", "ctrl+l", "R")

		if got := m.CurrentFile(); got != "src/main.go" {
			t.Errorf("CurrentFile() = %q, want %q", got, "src/main.go")
		}
	})

	t.Run("R reloads the revision left after a commit", func(t *testing.T) {
		src := newSource()
		m := press(t, loaded(t, src), "ctrl+h", "j", "ctrl+l", "space", "c", "ctrl+s")

		press(t, m, "R")

		if got := src.changeRevs[len(src.changeRevs)-1]; got != "rest1zzzzzzzzzz" {
			t.Errorf("Change() last rev = %q, want %q", got, "rest1zzzzzzzzzz")
		}
	})

	t.Run("R retries after a failed load", func(t *testing.T) {
		src := newSource()
		src.err = errors.New("revision not found")
		m := loaded(t, src)
		src.err = nil

		m = press(t, m, "R")

		assertMissing(t, view(m), "revision not found")
		if got := m.CurrentFile(); got != "src/ui/view.go" {
			t.Errorf("CurrentFile() = %q, want %q", got, "src/ui/view.go")
		}
	})
}

func TestReload_reloading(t *testing.T) {
	t.Run("c does not open the commit modal until the reload arrives", func(t *testing.T) {
		m := press(t, loaded(t, newSource()), "ctrl+h", "j", "ctrl+l", "space", "R")
		m, reload := update(t, m, keyMsg("R"))

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
}
