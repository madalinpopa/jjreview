package ui

import (
	"slices"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/madalinpopa/jjreview/internal/theme"
)

const CardChrome = 4

func Card(p theme.Theme, width, height int, title string, focused bool, value []Seg, body []string) string {
	inner := max(width-2, 0)
	edge := Fg(p.Border)
	if focused {
		edge = Fg(p.Selection)
	}
	side := edge.Background(p.Card).Render("│")
	header, values := CardHeader(p, title, value)
	lines := []string{
		edge.Render("╭" + strings.Repeat("─", inner) + "╮"),
		side + Paint(inner, p.Card, header, values) + side,
		edge.Background(p.Card).Render("├" + strings.Repeat("─", inner) + "┤"),
	}
	for i := range max(height-CardChrome, 0) {
		content := Blank(inner, p.Card)
		if i < len(body) {
			content = Clip(body[i], inner)
			content += Blank(inner-lipgloss.Width(content), p.Card)
		}
		lines = append(lines, side+content+side)
	}
	lines = append(lines, edge.Render("╰"+strings.Repeat("─", inner)+"╯"))
	return strings.Join(lines[:min(len(lines), height)], "\n")
}

func CardHeader(p theme.Theme, title string, value []Seg) ([]Seg, []Seg) {
	return []Seg{Span(Fg(p.Header), " "+strings.ToUpper(title))}, slices.Concat(value, []Seg{Span(lipgloss.NewStyle(), " ")})
}

func Message(p theme.Theme, text string, width int) string {
	return Paint(width, p.Card, []Seg{Span(Fg(p.Muted).Italic(true), "  "+text)}, nil)
}

func Description(p theme.Theme, text string, immutable bool) Seg {
	title, _, _ := strings.Cut(text, "\n")
	switch {
	case text == "":
		return Span(Fg(p.Muted).Italic(true), "(no description set)")
	case immutable:
		return Span(Fg(p.Muted), title)
	}
	return Span(Fg(p.Strong), title)
}
