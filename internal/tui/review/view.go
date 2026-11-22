package review

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/madalinpopa/jjreview/internal/theme"
	"github.com/madalinpopa/jjreview/internal/tui/ui"
)

const maxListWidth = 60

func (m Model) View(env Env) string {
	return lipgloss.JoinHorizontal(lipgloss.Top,
		m.filesCard(env),
		ui.Blank(1, env.Theme.Page),
		m.diffCard(env),
	)
}

func (m Model) Overlay(env Env) string {
	if m.modal == nil {
		return ""
	}
	return m.modalView(env)
}

func (m Model) Status(env Env) string {
	if m.ranging {
		return rangeStatus
	}
	if _, ok := m.currentFile(); !env.Loaded || env.Err != nil || !ok {
		return ""
	}
	n := len(m.files[m.fileIdx].Hunks)
	text := fmt.Sprintf("file %d/%d · %d %s", m.fileIdx+1, len(m.files), n, ui.Plural(n, "hunk"))
	if folded := m.currentReview().foldedCount(); folded > 0 && n > 0 {
		text += fmt.Sprintf(" (%d folded)", folded)
	}
	if m.needle != "" {
		text += " · " + m.searchStatus(env)
	}
	return text
}

func (m Model) listWidth(width int) int {
	return max(min(m.treeWidth+2, maxListWidth, (width-1)/2), 1)
}

func (m Model) measureTree(p theme.Theme) int {
	w := ui.RowWidth(ui.CardHeader(p, "Files", m.filesValue(p, len(m.files))))
	for _, r := range m.treeRows() {
		w = max(w, ui.RowWidth(m.treeSegs(p, r)))
	}
	return w
}

func (m Model) diffCardWidth(width int) int {
	return max(width-m.listWidth(width)-1, 1)
}

func (m Model) filesValue(p theme.Theme, done int) []ui.Seg {
	return []ui.Seg{ui.Span(ui.Fg(p.Title), fmt.Sprintf("%d/%d ✓", done, len(m.files)))}
}

func (m Model) filesCard(env Env) string {
	done := 0
	for _, r := range m.reviews {
		if r.state() == stateDone {
			done++
		}
	}
	rows := m.treeRows()
	width := m.listWidth(env.Width)
	inner := max(width-2, 0)
	var body []string
	for i := m.listTop; i < len(rows) && len(body) < env.ListHeight; i++ {
		body = append(body, m.treeLine(env.Theme, rows[i], inner))
	}
	return ui.Card(env.Theme, width, env.BodyHeight, "Files", m.filesFocus, m.filesValue(env.Theme, done), body)
}

func (m Model) treeLine(p theme.Theme, r treeRow, width int) string {
	bg := p.Card
	if !r.dir && r.file == m.fileIdx {
		bg = p.CursorBg
	}
	left, right := m.treeSegs(p, r)
	return ui.Paint(width, bg, left, right)
}

func (m Model) treeSegs(p theme.Theme, r treeRow) ([]ui.Seg, []ui.Seg) {
	if r.dir {
		left := []ui.Seg{ui.Span(lipgloss.NewStyle(), strings.Repeat(" ", 1+r.depth*2)), ui.Span(ui.Fg(p.Muted), "▾ "), ui.Span(ui.Fg(p.DirName), ui.EscapePath(r.name))}
		return left, []ui.Seg{ui.Span(ui.Fg(p.Muted), fmt.Sprint(r.count)), ui.Span(lipgloss.NewStyle(), " ")}
	}
	name, bar := ui.Fg(p.Strong), p.Card
	if r.file == m.fileIdx {
		name, bar = ui.Fg(p.Selection).Bold(true), p.Selection
	}
	st := m.reviews[r.file].state()
	left := []ui.Seg{
		ui.Span(ui.Fg(bar), ui.BarGlyph(bar)),
		ui.Span(lipgloss.NewStyle(), strings.Repeat(" ", r.depth*2)),
		ui.Span(glyphStyle(p, st), st.glyph()),
		ui.Span(lipgloss.NewStyle(), " "),
		ui.Span(name, ui.EscapePath(r.name)),
	}
	return left, []ui.Seg{ui.Span(ui.Fg(p.Muted), m.fileSummary(r.file)), ui.Span(lipgloss.NewStyle(), " ")}
}

func glyphStyle(p theme.Theme, st fileState) lipgloss.Style {
	switch st {
	case stateDone:
		return ui.Fg(p.Added)
	case stateNone:
		return ui.Fg(p.Muted)
	}
	return ui.Fg(p.Picked)
}

func (m Model) fileSummary(i int) string {
	f := m.files[i]
	if f.Binary {
		return "bin"
	}
	a, d := fileStats(f)
	return fmt.Sprintf("+%d −%d", a, d)
}

func (m Model) diffCard(env Env) string {
	var value []ui.Seg
	if file, ok := m.currentFile(); ok {
		value = []ui.Seg{ui.Span(ui.Fg(env.Theme.Title), ui.EscapePath(file.Path()))}
	}
	rows := m.rows(env)
	width := m.diffCardWidth(env.Width)
	inner := max(width-2, 0)
	number := numberWidth(rows)
	var body []string
	if m.searching {
		q := m.query
		q.SetStyles(ui.SearchStyles(env.Theme))
		body = []string{ui.Paint(inner, env.Theme.Card, []ui.Seg{ui.Span(lipgloss.NewStyle(), " "), ui.Span(lipgloss.NewStyle(), q.View())}, nil)}
	}
	for i := m.diffTop; i < len(rows) && len(body) < env.ListHeight; i++ {
		body = append(body, m.diffLine(env.Theme, rows[i], i == m.cursor, m.inRange(i), number, inner))
	}
	return ui.Card(env.Theme, width, env.BodyHeight, "Unified diff", !m.filesFocus, value, body)
}
