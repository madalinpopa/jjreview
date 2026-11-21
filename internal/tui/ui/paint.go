package ui

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

type Seg struct {
	Style lipgloss.Style
	Text  string
}

func Span(style lipgloss.Style, text string) Seg {
	return Seg{Style: style, Text: text}
}

func Render(bg color.Color, segs []Seg) string {
	var b strings.Builder
	for _, s := range segs {
		style := s.Style
		if _, unset := style.GetBackground().(lipgloss.NoColor); unset {
			style = style.Background(bg)
		}
		b.WriteString(style.Render(s.Text))
	}
	return b.String()
}

func Clip(s string, width int) string {
	if width <= 0 {
		return ""
	}
	return lipgloss.NewStyle().MaxWidth(width).Render(s)
}

func Blank(width int, bg color.Color) string {
	if width <= 0 {
		return ""
	}
	return lipgloss.NewStyle().Background(bg).Render(strings.Repeat(" ", width))
}

func Paint(width int, bg color.Color, left, right []Seg) string {
	r := Clip(Render(bg, right), width)
	avail := width - lipgloss.Width(r) - gapWidth(right)
	l := Clip(Render(bg, left), avail)
	fill := width - lipgloss.Width(l) - lipgloss.Width(r)
	return Clip(l+Blank(fill, bg)+r, width)
}

func gapWidth(right []Seg) int {
	if len(right) > 0 {
		return 1
	}
	return 0
}

func Width(segs []Seg) int {
	w := 0
	for _, s := range segs {
		w += lipgloss.Width(s.Text)
	}
	return w
}

func RowWidth(left, right []Seg) int {
	return Width(left) + gapWidth(right) + Width(right)
}

func ScrollTop(top, cursor, height, total int) int {
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

func Plural(n int, word string) string {
	if n == 1 {
		return word
	}
	return word + "s"
}
