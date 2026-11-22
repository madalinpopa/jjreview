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

func (m Model) diffLine(p theme.Theme, r row, selected, ranged bool, number, width int) string {
	switch r.kind {
	case rowHunk:
		return m.hunkLine(p, r.hunk, selected, width)
	case rowLine:
		return m.codeLine(p, r, selected, ranged, number, width)
	case rowGap:
		return ui.Paint(width, p.Card, []ui.Seg{ui.Span(ui.Fg(p.Faint), strings.Repeat(" ", number-minNumberWidth)+gapIndent+r.text)}, nil)
	case rowError:
		return ui.Paint(width, p.Card, []ui.Seg{ui.Span(ui.Fg(p.Removed), "  "+r.text)}, nil)
	}
	return ui.Paint(width, p.Card, []ui.Seg{ui.Span(ui.Fg(p.Muted).Italic(true), "  "+r.text)}, nil)
}

func (m Model) hunkLine(p theme.Theme, k int, selected bool, width int) string {
	file := m.files[m.fileIdx]
	review := m.currentReview()
	h := file.Hunks[k]
	st := review.unitState(k)
	folded := review.folded[k]

	bar, text, arrow := p.HunkBar, p.Hunk, "▾"
	if folded {
		bar, text, arrow = p.FoldedBar, p.Stats, "▸"
	}
	bg := p.HunkBg
	muted, stat, mark := p.Muted, p.Stats, p.Added
	check, checkStyle := st.glyph(), ui.Fg(p.Muted)
	switch st {
	case stateDone:
		bar, checkStyle = p.Added, ui.Fg(p.Added)
	case stateAll:
		bg, text, muted, stat, mark = p.Selection, p.OnSelection, p.OnSelection, p.OnSelection, p.OnSelection
		bar, checkStyle = p.Selection, ui.Fg(p.OnSelection).Bold(true)
	case stateSome:
		checkStyle = ui.Fg(p.Picked)
	}
	if selected {
		bar = p.Selection
	}
	stats := statsText(h, review.lines[k])
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
	if note := committedNote(h, review, k); note != "" {
		right = append(right, ui.Span(ui.Fg(mark), " "+note))
	}
	right = append(right, ui.Span(lipgloss.NewStyle(), " "))
	return ui.Paint(width, bg, left, right)
}

func committedNote(h diff.Hunk, review fileReview, k int) string {
	a, d := hunkStats(h)
	pending, _ := tally(review.lines[k])
	switch done := a + d - pending; {
	case done == 0 || review.committed[k] == "":
		return ""
	case pending == 0:
		return review.committed[k]
	default:
		return fmt.Sprintf("✓ %d/%d %s", done, a+d, review.committed[k])
	}
}

func (m Model) lineState(r row) (lineState, bool) {
	if r.hunk < 0 {
		return lineContext, false
	}
	review := m.currentReview()
	done := review.unitState(r.hunk) == stateDone
	if r.index < 0 {
		return lineContext, done
	}
	st := review.lines[r.hunk][r.index]
	return st, done || st == lineDone
}

func (m Model) codeLine(p theme.Theme, r row, selected, ranged bool, number, width int) string {
	l := r.line
	bg, bar, sign, text := p.Card, p.Card, p.Muted, p.Text
	switch l.Kind {
	case diff.LineAdded:
		bg, bar, sign = p.AddedBg, p.Added, p.Added
	case diff.LineRemoved:
		bg, bar, sign = p.RemovedBg, p.Removed, p.Removed
	}
	code := m.codeSegs(r)
	gutter := p.Faint
	st, dim := m.lineState(r)
	if dim {
		bar, sign, text, gutter, code = dimmed(bar, p), p.Dim, p.Dim, p.Dim, nil
	}
	if ranged {
		bg = p.CursorBg
	}
	if selected {
		bg, bar = p.CursorBg, p.Selection
	}
	if code == nil {
		code = []ui.Seg{ui.Span(ui.Fg(text), l.Text)}
	}
	code, before, after := m.visibleCode(code, width-gutterWidth(number))
	left := append([]ui.Seg{ui.Span(ui.Fg(bar), ui.BarGlyph(bar))}, gutterSegs(p, l, number, gutter, sign, st == linePicked, before)...)
	left = append(left, code...)
	if after {
		left = append(left, ui.Span(ui.Fg(p.Muted), hiddenRight))
	}
	return ui.Paint(width, bg, left, nil)
}

func gutterSegs(p theme.Theme, l diff.Line, width int, gutter, sign color.Color, picked, hidden bool) []ui.Seg {
	number, signText := "", " "
	switch l.Kind {
	case diff.LineAdded:
		signText = "+"
	case diff.LineRemoved:
		signText = "-"
	}
	if l.Kind != diff.LineRemoved && l.NewNo > 0 {
		number = fmt.Sprint(l.NewNo)
	}
	number = fmt.Sprintf("%*s", width, number)
	edge := ui.Span(ui.Fg(sign), " ")
	if hidden {
		edge = ui.Span(ui.Fg(p.Muted), hiddenLeft)
	}
	if picked {
		return []ui.Seg{ui.Span(ui.Fg(p.Picked).Reverse(true), number+" "+signText), edge}
	}
	segs := []ui.Seg{ui.Span(ui.Fg(gutter), number), ui.Span(lipgloss.NewStyle(), " ")}
	if hidden {
		return append(segs, ui.Span(ui.Fg(sign), signText), edge)
	}
	return append(segs, ui.Span(ui.Fg(sign), signText+" "))
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
