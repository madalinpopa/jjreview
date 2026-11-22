package tui

import (
	"fmt"
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/madalinpopa/jjreview/internal/tui/ui"
)

const maxListWidth = 60

func (m Model) View() tea.View {
	content := m.screen()
	if box := m.overlay(); box != "" {
		x := max((m.width-lipgloss.Width(box))/2, 0)
		y := max((m.height-lipgloss.Height(box))/2, 0)
		content = lipgloss.NewCompositor(
			lipgloss.NewLayer(content),
			lipgloss.NewLayer(box).X(x).Y(y).Z(1),
		).Render()
	}
	v := tea.NewView(fit(content, m.width, m.height))
	v.AltScreen = true
	v.ReportFocus = m.autoTheme()
	return v
}

func fit(content string, width, height int) string {
	lines := strings.Split(content, "\n")
	lines = lines[:min(len(lines), max(height, 0))]
	for i, l := range lines {
		lines[i] = ui.Clip(l, width)
	}
	return strings.Join(lines, "\n")
}

func (m Model) screen() string {
	var body string
	switch m.view {
	case logView:
		body = m.log.View(m.logEnv())
	case bookmarksView:
		body = m.bookmarks.View(m.bookmarkEnv())
	case operationsView:
		body = m.operations.View(m.operationEnv())
	case reviewView:
		body = lipgloss.JoinHorizontal(lipgloss.Top,
			m.filesCard(),
			ui.Blank(1, m.pal.Page),
			m.diffCard(),
		)
	}
	return strings.Join(append([]string{m.topBar(), body}, m.bottomBar()...), "\n")
}

func (m Model) listWidth() int {
	return max(min(m.treeWidth+2, maxListWidth, (m.width-1)/2), 1)
}

func (m Model) measureTree() int {
	w := ui.RowWidth(ui.CardHeader(m.pal, "Files", m.filesValue(len(m.files))))
	for _, r := range m.treeRows() {
		w = max(w, ui.RowWidth(m.treeSegs(r)))
	}
	return w
}

func (m Model) diffCardWidth() int {
	return max(m.width-m.listWidth()-1, 1)
}

func (m Model) bodyHeight() int {
	return max(m.height-1-len(m.bottomBar()), 0)
}

func (m Model) diffHeight() int {
	return max(m.bodyHeight()-ui.CardChrome, 1)
}

func (m Model) topBar() string {
	p := m.pal
	id := m.target
	if id == "" {
		id = m.rev
	}
	gap := ui.Span(lipgloss.NewStyle(), " ")
	revision := ui.Pill(p.Chip, p.ChipText, "@ "+ui.ShortID(id))
	n := pickedUnits(m.picks())
	var indicators, optional []ui.Seg
	if m.change.Immutable {
		indicators = append(indicators, ui.Pill(p.Chip, p.Removed, "read only"))
	}
	switch {
	case m.view == reviewView && n > 0:
		indicators = append(indicators, bold(ui.Pill(p.BadgeSel, p.Picked, fmt.Sprintf("%d %s selected", n, ui.Plural(n, "hunk")))))
	case m.view == reviewView:
		optional = append(optional, bold(ui.Pill(p.BadgeIdle, p.BadgeIdleFg, "nothing selected")))
	case n > 0:
		indicators = append(indicators, bold(ui.Pill(p.BadgeSel, p.Picked, fmt.Sprintf("● %d", n))))
	}
	var title []ui.Seg
	if subject, _, _ := strings.Cut(strings.TrimSpace(m.change.Description), "\n"); subject != "" {
		title = []ui.Seg{gap, ui.Span(ui.Fg(p.Title).Bold(true), subject)}
	}
	fits := func(left, right, title []ui.Seg) bool {
		return ui.RowWidth(slices.Concat(left, title), spaced(right))+1 <= m.width
	}
	bar := func(left, right []ui.Seg) string {
		return ui.Paint(m.width, p.Page, slices.Concat(left, title), spaced(right))
	}
	left := spaced(append(m.tabs(false), revision))
	chips := slices.Concat(optional, indicators)
	for drop := range len(optional) + 1 {
		if fits(left, chips[drop:], title) {
			return bar(left, chips[drop:])
		}
	}
	if fits(left, indicators, nil) {
		return bar(left, indicators)
	}
	left = spaced(append(m.tabs(true), revision))
	for drop := range len(indicators) {
		if fits(left, indicators[drop:], nil) {
			return bar(left, indicators[drop:])
		}
	}
	return bar(left, nil)
}

func bold(s ui.Seg) ui.Seg {
	s.Style = s.Style.Bold(true)
	return s
}

func spaced(segs []ui.Seg) []ui.Seg {
	var out []ui.Seg
	for i, s := range segs {
		if i > 0 {
			out = append(out, ui.Span(lipgloss.NewStyle(), " "))
		}
		out = append(out, s)
	}
	return out
}

func (m Model) tabs(short bool) []ui.Seg {
	tabs := []struct {
		view  view
		key   key.Binding
		label string
	}{
		{reviewView, m.keys.reviewTab, "Review"},
		{logView, m.keys.logTab, "Log"},
		{bookmarksView, m.keys.bookmarksTab, "Bookmarks"},
		{operationsView, m.keys.operationsTab, "Operations"},
	}
	segs := make([]ui.Seg, len(tabs))
	for i, t := range tabs {
		label := t.label
		if short {
			label = ""
		}
		segs[i] = m.tab(t.view, t.key, label)
	}
	return segs
}

func (m Model) tab(v view, b key.Binding, label string) ui.Seg {
	text := strings.TrimSpace(b.Help().Key + " " + label)
	if v == m.view {
		s := ui.Pill(m.pal.Chip, m.pal.Key, text)
		s.Style = s.Style.Bold(true)
		return s
	}
	return ui.Pill(m.pal.Chip, m.pal.ChipText, text)
}

func (m Model) bottomBar() []string {
	p := m.pal
	sep := ui.Span(ui.Fg(p.Faint), " · ")
	var lines [][]ui.Seg
	var line []ui.Seg
	used := 0
	for _, h := range m.keyHints() {
		segs := ui.HintSegs(m.pal, h)
		w := lipgloss.Width(ui.Render(p.Page, segs))
		switch {
		case len(line) == 0:
		case used+lipgloss.Width(sep.Text)+w <= m.width:
			line = append(line, sep)
			used += lipgloss.Width(sep.Text)
		default:
			lines = append(lines, line)
			line, used = nil, 0
		}
		line = append(line, segs...)
		used += w
	}
	status := []ui.Seg{ui.Span(ui.Fg(p.Status), m.statusText())}
	out := make([]string, 0, len(lines)+2)
	for _, l := range lines {
		out = append(out, ui.Paint(m.width, p.Page, l, nil))
	}
	if used+1+lipgloss.Width(m.statusText()) <= m.width {
		return append(out, ui.Paint(m.width, p.Page, line, status))
	}
	return append(out, ui.Paint(m.width, p.Page, line, nil), ui.Paint(m.width, p.Page, nil, status))
}

func (m Model) statusText() string {
	if m.status == "" && m.outdated {
		return outdatedStatus
	}
	if m.status != "" || m.view != reviewView {
		return m.status
	}
	if !m.ready() {
		return ""
	}
	n := len(m.files[m.fileIdx].Hunks)
	text := fmt.Sprintf("file %d/%d · %d %s", m.fileIdx+1, len(m.files), n, ui.Plural(n, "hunk"))
	if folded := m.currentReview().foldedCount(); folded > 0 && n > 0 {
		text += fmt.Sprintf(" (%d folded)", folded)
	}
	return text
}

func (m Model) filesValue(done int) []ui.Seg {
	return []ui.Seg{ui.Span(ui.Fg(m.pal.Title), fmt.Sprintf("%d/%d ✓", done, len(m.files)))}
}

func (m Model) filesCard() string {
	done := 0
	for _, r := range m.reviews {
		if r.state() == stateDone {
			done++
		}
	}
	rows := m.treeRows()
	width := m.listWidth()
	inner := max(width-2, 0)
	var body []string
	for i := m.listTop; i < len(rows) && len(body) < m.diffHeight(); i++ {
		body = append(body, m.treeLine(rows[i], inner))
	}
	return ui.Card(m.pal, width, m.bodyHeight(), "Files", m.filesFocus, m.filesValue(done), body)
}

func (m Model) treeLine(r treeRow, width int) string {
	bg := m.pal.Card
	if !r.dir && r.file == m.fileIdx {
		bg = m.pal.CursorBg
	}
	left, right := m.treeSegs(r)
	return ui.Paint(width, bg, left, right)
}

func (m Model) treeSegs(r treeRow) ([]ui.Seg, []ui.Seg) {
	p := m.pal
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
		ui.Span(m.glyphStyle(st), st.glyph()),
		ui.Span(lipgloss.NewStyle(), " "),
		ui.Span(name, ui.EscapePath(r.name)),
	}
	return left, []ui.Seg{ui.Span(ui.Fg(p.Muted), m.fileSummary(r.file)), ui.Span(lipgloss.NewStyle(), " ")}
}

func (m Model) glyphStyle(st fileState) lipgloss.Style {
	switch st {
	case stateDone:
		return ui.Fg(m.pal.Added)
	case stateNone:
		return ui.Fg(m.pal.Muted)
	}
	return ui.Fg(m.pal.Picked)
}

func (m Model) fileSummary(i int) string {
	f := m.files[i]
	if f.Binary {
		return "bin"
	}
	a, d := fileStats(f)
	return fmt.Sprintf("+%d −%d", a, d)
}

func (m Model) diffCard() string {
	var value []ui.Seg
	if file, ok := m.currentFile(); ok {
		value = []ui.Seg{ui.Span(ui.Fg(m.pal.Title), ui.EscapePath(file.Path()))}
	}
	rows := m.rows()
	width := m.diffCardWidth()
	inner := max(width-2, 0)
	var body []string
	for i := m.diffTop; i < len(rows) && len(body) < m.diffHeight(); i++ {
		body = append(body, m.diffLine(rows[i], i == m.cursor, inner))
	}
	return ui.Card(m.pal, width, m.bodyHeight(), "Unified diff", !m.filesFocus, value, body)
}

func (m Model) layout() ui.Layout {
	return ui.Layout{
		Theme:      m.pal,
		Width:      m.width,
		Height:     m.height,
		BodyHeight: m.bodyHeight(),
		ListHeight: m.diffHeight(),
		ModalWidth: m.modalWidth(),
	}
}
