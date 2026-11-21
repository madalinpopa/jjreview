package ui

import (
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/lipgloss/v2"

	"github.com/madalinpopa/jjreview/internal/theme"
)

type KeyHint struct {
	Keys  []string
	Label string
}

func Hint(label string, bindings ...key.Binding) KeyHint {
	h := KeyHint{Label: label}
	for _, b := range bindings {
		h.Keys = append(h.Keys, strings.Fields(b.Help().Key)...)
	}
	return h
}

func Describe(b key.Binding) KeyHint {
	return Hint(b.Help().Desc, b)
}

func HintSegs(p theme.Theme, h KeyHint) []Seg {
	var segs []Seg
	for j, k := range h.Keys {
		if j > 0 {
			segs = append(segs, Span(Fg(p.Stats), " "))
		}
		segs = append(segs, Span(lipgloss.NewStyle().Background(p.Chip).Foreground(p.Key), k))
	}
	return append(segs, Span(Fg(p.Stats), " "+h.Label))
}
