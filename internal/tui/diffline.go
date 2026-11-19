package tui

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/madalinpopa/jjreview/internal/diff"
	"github.com/madalinpopa/jjreview/internal/theme"
)

const gapIndent = "       "

func (m Model) diffLine(r row, selected bool, width int) string {
	p := m.pal
	switch r.kind {
	case rowHunk:
		return m.hunkLine(r.hunk, selected, width)
	case rowLine:
		return m.codeLine(r, selected, width)
	case rowGap:
		return paint(width, p.Card, []seg{span(fg(p.Faint), gapIndent+r.text)}, nil)
	case rowError:
		return paint(width, p.Card, []seg{span(fg(p.Removed), "  "+r.text)}, nil)
	}
	return paint(width, p.Card, []seg{span(fg(p.Muted).Italic(true), "  "+r.text)}, nil)
}

func (m Model) hunkLine(k int, selected bool, width int) string {
	p := m.pal
	file := m.files[m.fileIdx]
	review := m.currentReview()
	h := file.Hunks[k]
	committed := review.committed[k]
	picked := committed == "" && review.selected[k]
	folded := review.folded[k]

	bar, text, arrow := p.HunkBar, p.Hunk, "▾"
	if folded {
		bar, text, arrow = p.FoldedBar, p.Stats, "▸"
	}
	bg := p.HunkBg
	muted, stat := p.Muted, p.Stats
	check, checkStyle := "○", fg(p.Muted)
	switch {
	case committed != "":
		bar, check, checkStyle = p.Added, "✓", fg(p.Added)
	case picked:
		bg, text, muted, stat = p.Selection, p.OnSelection, p.OnSelection, p.OnSelection
		bar, check, checkStyle = p.Selection, "●", fg(p.OnSelection).Bold(true)
	}
	if selected {
		bar = p.Selection
	}
	a, d := hunkStats(h)
	stats := fmt.Sprintf("+%d −%d", a, d)
	if folded {
		stats += " · folded"
	}
	left := []seg{
		span(fg(bar), barGlyph(bar)+" "),
		span(fg(muted), arrow+" "),
		span(checkStyle, check+" "),
		span(fg(text), hunkHeader(h)),
	}
	right := []seg{span(fg(stat), stats)}
	if committed != "" {
		right = append(right, span(fg(p.Added), " "+committed))
	}
	right = append(right, span(lipgloss.NewStyle(), " "))
	return paint(width, bg, left, right)
}

func (m Model) codeLine(r row, selected bool, width int) string {
	p := m.pal
	l := r.line
	bg, bar, sign, text, signText := p.Card, p.Card, p.Muted, p.Text, "  "
	switch l.Kind {
	case diff.LineAdded:
		bg, bar, sign, signText = p.AddedBg, p.Added, p.Added, "+ "
	case diff.LineRemoved:
		bg, bar, sign, signText = p.RemovedBg, p.Removed, p.Removed, "- "
	}
	code := m.codeSegs(r)
	gutter := p.Faint
	if r.hunk >= 0 && m.currentReview().committed[r.hunk] != "" {
		bar, sign, text, gutter, code = dimmed(bar, p), p.Dim, p.Dim, p.Dim, nil
	}
	if selected {
		bg, bar = p.CursorBg, p.Selection
	}
	number := ""
	if l.Kind != diff.LineRemoved && l.NewNo > 0 {
		number = fmt.Sprint(l.NewNo)
	}
	left := []seg{
		span(fg(bar), barGlyph(bar)),
		span(fg(gutter), fmt.Sprintf("%4s ", number)),
		span(fg(sign), signText),
	}
	if code == nil {
		code = []seg{span(fg(text), l.Text)}
	}
	for _, c := range code {
		left = append(left, span(c.style, expandTabs(c.text)))
	}
	return paint(width, bg, left, nil)
}

func expandTabs(s string) string {
	return strings.ReplaceAll(s, "\t", "    ")
}

func dimmed(c color.Color, p theme.Theme) color.Color {
	if c == p.Card {
		return c
	}
	return p.Dim
}
