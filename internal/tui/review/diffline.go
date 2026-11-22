package review

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/madalinpopa/jjreview/internal/diff"
	"github.com/madalinpopa/jjreview/internal/theme"
	"github.com/madalinpopa/jjreview/internal/tui/ui"
)

const gapIndent = "       "

func (m Model) diffLine(p theme.Theme, r row, selected bool, width int) string {
	switch r.kind {
	case rowHunk:
		return m.hunkLine(p, r.hunk, selected, width)
	case rowLine:
		return m.codeLine(p, r, selected, width)
	case rowGap:
		return ui.Paint(width, p.Card, []ui.Seg{ui.Span(ui.Fg(p.Faint), gapIndent+r.text)}, nil)
	case rowError:
		return ui.Paint(width, p.Card, []ui.Seg{ui.Span(ui.Fg(p.Removed), "  "+r.text)}, nil)
	}
	return ui.Paint(width, p.Card, []ui.Seg{ui.Span(ui.Fg(p.Muted).Italic(true), "  "+r.text)}, nil)
}

func (m Model) hunkLine(p theme.Theme, k int, selected bool, width int) string {
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
	check, checkStyle := "○", ui.Fg(p.Muted)
	switch {
	case committed != "":
		bar, check, checkStyle = p.Added, "✓", ui.Fg(p.Added)
	case picked:
		bg, text, muted, stat = p.Selection, p.OnSelection, p.OnSelection, p.OnSelection
		bar, check, checkStyle = p.Selection, "●", ui.Fg(p.OnSelection).Bold(true)
	}
	if selected {
		bar = p.Selection
	}
	a, d := hunkStats(h)
	stats := fmt.Sprintf("+%d −%d", a, d)
	if folded {
		stats += " · folded"
	}
	left := []ui.Seg{
		ui.Span(ui.Fg(bar), ui.BarGlyph(bar)+" "),
		ui.Span(ui.Fg(muted), arrow+" "),
		ui.Span(checkStyle, check+" "),
		ui.Span(ui.Fg(text), hunkHeader(h)),
	}
	right := []ui.Seg{ui.Span(ui.Fg(stat), stats)}
	if committed != "" {
		right = append(right, ui.Span(ui.Fg(p.Added), " "+committed))
	}
	right = append(right, ui.Span(lipgloss.NewStyle(), " "))
	return ui.Paint(width, bg, left, right)
}

func (m Model) codeLine(p theme.Theme, r row, selected bool, width int) string {
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
	left := []ui.Seg{
		ui.Span(ui.Fg(bar), ui.BarGlyph(bar)),
		ui.Span(ui.Fg(gutter), fmt.Sprintf("%4s ", number)),
		ui.Span(ui.Fg(sign), signText),
	}
	if code == nil {
		code = []ui.Seg{ui.Span(ui.Fg(text), l.Text)}
	}
	for _, c := range code {
		left = append(left, ui.Span(c.Style, expandTabs(c.Text)))
	}
	return ui.Paint(width, bg, left, nil)
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
