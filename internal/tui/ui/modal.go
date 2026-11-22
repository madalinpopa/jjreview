package ui

import (
	"slices"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/madalinpopa/jjreview/internal/theme"
)

func WrapLines(p theme.Theme, width int, style lipgloss.Style, text string) []string {
	var out []string
	for line := range strings.SplitSeq(ansi.Wrap(text, width, ""), "\n") {
		out = append(out, Paint(width, p.Card, []Seg{Span(style, line)}, nil))
	}
	return out
}

func TruncateLines(lines []string, room int, ellipsis string) []string {
	if len(lines) <= room {
		return lines
	}
	if room <= 0 {
		return []string{}
	}
	return append(slices.Clone(lines[:room-1]), ellipsis)
}

func Ellipsis(p theme.Theme, width int) string {
	return Paint(width, p.Card, []Seg{Span(Fg(p.Muted), "…")}, nil)
}

func ModalBox(p theme.Theme, lines []string) string {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(p.ModalBorder).
		BorderBackground(p.Card).
		Background(p.Card).
		Padding(0, 1).
		Render(strings.Join(lines, "\n"))
}
