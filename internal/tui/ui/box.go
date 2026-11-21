package ui

import (
	"image/color"

	"charm.land/lipgloss/v2"

	"github.com/madalinpopa/jjreview/internal/theme"
)

func InputBoxInner(width int) int {
	return max(width-4, 1)
}

func InputBox(p theme.Theme, width int, border color.Color, content string) string {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(border).
		BorderBackground(p.Card).
		Background(p.Input).
		Padding(0, 1).
		Width(width).
		Render(content)
}
