package tui

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

type seg struct {
	style lipgloss.Style
	text  string
}

func span(style lipgloss.Style, text string) seg {
	return seg{style: style, text: text}
}

func render(bg color.Color, segs []seg) string {
	var b strings.Builder
	for _, s := range segs {
		style := s.style
		if _, unset := style.GetBackground().(lipgloss.NoColor); unset {
			style = style.Background(bg)
		}
		b.WriteString(style.Render(s.text))
	}
	return b.String()
}

func clip(s string, width int) string {
	if width <= 0 {
		return ""
	}
	return lipgloss.NewStyle().MaxWidth(width).Render(s)
}

func blank(width int, bg color.Color) string {
	if width <= 0 {
		return ""
	}
	return lipgloss.NewStyle().Background(bg).Render(strings.Repeat(" ", width))
}

func paint(width int, bg color.Color, left, right []seg) string {
	r := clip(render(bg, right), width)
	avail := width - lipgloss.Width(r)
	if len(right) > 0 {
		avail--
	}
	l := clip(render(bg, left), avail)
	fill := width - lipgloss.Width(l) - lipgloss.Width(r)
	return clip(l+blank(fill, bg)+r, width)
}

func scrollTop(top, cursor, height, total int) int {
	if height <= 0 {
		return 0
	}
	top = min(top, max(total-height, 0))
	switch {
	case cursor < top:
		top = cursor
	case cursor >= top+height:
		top = cursor - height + 1
	}
	return max(top, 0)
}

func plural(n int, word string) string {
	if n == 1 {
		return word
	}
	return word + "s"
}
