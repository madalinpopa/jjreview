package ui

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

func BarGlyph(c color.Color) string {
	if _, unset := c.(lipgloss.NoColor); unset {
		return " "
	}
	return "▌"
}

func Fg(c color.Color) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(c)
}

func Pill(bg, fg color.Color, text string) Seg {
	return Span(lipgloss.NewStyle().Background(bg).Foreground(fg), " "+text+" ")
}
