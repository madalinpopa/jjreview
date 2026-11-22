package ui_test

import (
	"errors"
	"testing"

	"github.com/madalinpopa/jjreview/internal/tui/ui"
)

func TestLoader_success(t *testing.T) {
	t.Parallel()

	t.Run("is due before the first load", func(t *testing.T) {
		t.Parallel()
		var l ui.Loader

		if !l.Due() {
			t.Error("Due() = false before any load, want true")
		}
	})

	t.Run("is busy and not due while loading", func(t *testing.T) {
		t.Parallel()
		var l ui.Loader

		l.Begin()

		if !l.Loading() || l.Due() {
			t.Errorf("after Begin Loading() = %v, Due() = %v, want true, false", l.Loading(), l.Due())
		}
	})

	t.Run("accepts the latest load and keeps its error", func(t *testing.T) {
		t.Parallel()
		var l ui.Loader
		errLoad := errors.New("boom")
		seq := l.Begin()

		ok := l.Finish(seq, errLoad)

		if !ok || l.Loading() || !l.Loaded() || !errors.Is(l.Err(), errLoad) || l.Due() {
			t.Errorf("Finish = %v, Loading %v, Loaded %v, Err %v, Due %v; want true, false, true, %v, false", ok, l.Loading(), l.Loaded(), l.Err(), l.Due(), errLoad)
		}
	})

	t.Run("is due again once stale", func(t *testing.T) {
		t.Parallel()
		var l ui.Loader
		l.Finish(l.Begin(), nil)

		l.MarkStale()

		if !l.Stale() || !l.Due() {
			t.Errorf("after MarkStale Stale() = %v, Due() = %v, want true, true", l.Stale(), l.Due())
		}
	})

	t.Run("a new load clears the stale mark", func(t *testing.T) {
		t.Parallel()
		var l ui.Loader
		l.MarkStale()

		l.Begin()

		if l.Stale() {
			t.Error("Stale() = true after Begin, want false")
		}
	})
}

func TestLoader_failed(t *testing.T) {
	t.Parallel()
	var l ui.Loader
	old := l.Begin()
	l.Begin()

	ok := l.Finish(old, nil)

	if ok || !l.Loading() || l.Loaded() {
		t.Errorf("Finish(outdated) = %v, Loading %v, Loaded %v; want false, true, false", ok, l.Loading(), l.Loaded())
	}
}
