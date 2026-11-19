package tui

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

func barGlyph(c color.Color) string {
	if _, unset := c.(lipgloss.NoColor); unset {
		return " "
	}
	return "▌"
}

func fg(c color.Color) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(c)
}
