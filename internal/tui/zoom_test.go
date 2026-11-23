package tui_test

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

func TestZoom_success(t *testing.T) {
	t.Run("w hides the file pane and widens the diff", func(t *testing.T) {
		m := onWide(t, wideSource())
		assertMissing(t, lineWith(t, view(m), "a000"), "a100")

		m = press(t, m, "w")

		got := view(m)
		assertContains(t, got, "UNIFIED DIFF", "LONG.txt")
		assertMissing(t, got, "FILES")
		line := lineWith(t, got, "a000")
		assertContains(t, line, "a100", "›│")
		if !strings.HasPrefix(line, "│▌   2 + a000") {
			t.Errorf("View() diff line = %q, want it to start at the left edge", line)
		}
		for i, l := range strings.Split(m.View().Content, "\n") {
			if w := lipgloss.Width(l); w != 120 {
				t.Errorf("View() row %d is %d columns wide, want 120: %q", i, w, ansiPattern.ReplaceAllString(l, ""))
			}
		}
	})

	t.Run("w again brings the file pane back", func(t *testing.T) {
		m := press(t, onWide(t, wideSource()), "w", "w")

		got := view(m)
		assertContains(t, got, "FILES", "UNIFIED DIFF")
		assertMissing(t, lineWith(t, got, "a000"), "a100")
	})

	t.Run("w from the file pane focuses the diff", func(t *testing.T) {
		m := press(t, onWide(t, wideSource()), "tab", "w")

		if m.FilesFocused() {
			t.Error("FilesFocused() = true, want the diff pane focused")
		}
		assertContains(t, view(press(t, m, "l")), "   2 +‹5 a010 a015")
	})

	t.Run("tab leaves the zoom and focuses the file pane", func(t *testing.T) {
		m := press(t, onWide(t, wideSource()), "w", "tab")

		if !m.FilesFocused() {
			t.Error("FilesFocused() = false, want the file pane focused")
		}
		assertContains(t, view(m), "FILES", "UNIFIED DIFF")
	})

	t.Run("J and K keep the zoom", func(t *testing.T) {
		m := press(t, onWide(t, wideSource()), "w", "J")

		if got := m.CurrentFile(); got != "README.md" {
			t.Errorf("CurrentFile() = %q, want README.md", got)
		}
		assertMissing(t, view(m), "FILES")
		assertMissing(t, view(press(t, m, "K")), "FILES")
	})

	t.Run("R keeps the zoom", func(t *testing.T) {
		m := press(t, onWide(t, wideSource()), "w", "R")

		assertMissing(t, view(m), "FILES")
		assertContains(t, view(m), "UNIFIED DIFF")
	})

	t.Run("opening another revision leaves the zoom", func(t *testing.T) {
		m := press(t, loaded(t, revisionSource()), "w")
		assertMissing(t, view(m), "FILES")

		m = press(t, m, reviewParent()...)

		assertContains(t, view(m), "FILES", "README.md")
	})

	t.Run("the search query spans the zoomed pane", func(t *testing.T) {
		m := press(t, onWide(t, wideSource()), "w")

		m = press(t, m, append(append([]string{"/"}, typeKeys(strings.Repeat("x", 150))...), "y")...)

		got := lineWith(t, view(m), "/ x")
		assertContains(t, got, "xy │")
		if n := strings.Count(got, "x"); n < 100 {
			t.Errorf("View() query line shows %d x, want the full pane width: %q", n, got)
		}
	})

	t.Run("the bar offers zoom when a line is wider than the pane", func(t *testing.T) {
		m := onWide(t, wideSource())

		assertContains(t, view(m), "w zoom")
		assertContains(t, view(press(t, m, "w")), "w unzoom")
		assertMissing(t, view(loaded(t, newSource())), "w zoom")
	})

	t.Run("the bar offers zoom from the file pane", func(t *testing.T) {
		m := press(t, onWide(t, wideSource()), "tab")

		assertContains(t, view(m), "w zoom")
	})

	t.Run("the help lists the key", func(t *testing.T) {
		assertContains(t, view(press(t, start(t, newSource(), nil, 120, 50), "?")), "zoom the diff pane in / out")
	})
}

func TestZoom_clamped(t *testing.T) {
	t.Run("zooming pulls the scroll back so leaving it shows the same column", func(t *testing.T) {
		m := scrolledEnd(t, onWide(t, wideSource()))
		assertMissing(t, lineWith(t, view(m), "a195"), "›")

		m = press(t, m, "w")
		assertContains(t, lineWith(t, view(m), "a195"), "a190 a195│")

		m = press(t, m, "w")

		got := lineWith(t, view(m), "   2 +‹")
		assertContains(t, got, "›│")
		assertMissing(t, got, "a195")
	})
}
