package tui

import (
	"fmt"
	"image/color"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const (
	maxListWidth = 32
	cardChrome   = 4
)

type keyHint struct {
	keys  []string
	label string
}

func keyHints(filesFocus, readOnly bool) []keyHint {
	move := keyHint{keys: []string{"j", "k"}, label: "line"}
	if filesFocus {
		move.label = "file"
	}
	hints := []keyHint{{keys: []string{"h", "l"}, label: "pane"}, move}
	if !readOnly {
		hints = append(hints, keyHint{keys: []string{"space"}, label: "select"}, keyHint{keys: []string{"c"}, label: "commit"})
	}
	if !filesFocus {
		hints = append(hints, keyHint{keys: []string{"d"}, label: "diff"})
	}
	return append(hints, keyHint{keys: []string{"e"}, label: "edit"}, keyHint{keys: []string{"?"}, label: "help"})
}

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
	return v
}

func fit(content string, width, height int) string {
	lines := strings.Split(content, "\n")
	lines = lines[:min(len(lines), max(height, 0))]
	for i, l := range lines {
		lines[i] = clip(l, width)
	}
	return strings.Join(lines, "\n")
}

func (m Model) screen() string {
	body := lipgloss.JoinHorizontal(lipgloss.Top,
		m.filesCard(),
		blank(1, m.pal.Page),
		m.diffCard(),
	)
	return strings.Join(append([]string{m.topBar(), body}, m.bottomBar()...), "\n")
}

func (m Model) listWidth() int {
	return min(maxListWidth, max(m.width/3, 1))
}

func (m Model) diffCardWidth() int {
	return max(m.width-m.listWidth()-1, 1)
}

func (m Model) bodyHeight() int {
	return max(m.height-1-len(m.bottomBar()), 0)
}

func (m Model) diffHeight() int {
	return max(m.bodyHeight()-cardChrome, 1)
}

func (m Model) pill(bg, fg color.Color, text string) seg {
	return span(lipgloss.NewStyle().Background(bg).Foreground(fg), " "+text+" ")
}

func (m Model) topBar() string {
	p := m.pal
	id := m.target
	if id == "" {
		id = m.rev
	}
	left := []seg{
		m.pill(p.Chip, p.ChipText, "@ "+shortID(id)),
		span(lipgloss.NewStyle(), " "),
		span(fg(p.Title).Bold(true), "jj review"),
	}
	n := pickedUnits(m.picks())
	badge := m.pill(p.BadgeIdle, p.BadgeIdleFg, "nothing selected")
	if n > 0 {
		badge = m.pill(p.BadgeSel, p.Picked, fmt.Sprintf("%d %s selected", n, plural(n, "hunk")))
	}
	badge.style = badge.style.Bold(true)
	right := []seg{m.pill(p.Chip, p.Key, m.modeLabel()), span(lipgloss.NewStyle(), " "), badge}
	if m.change.Immutable {
		right = append([]seg{m.pill(p.Chip, p.Removed, "read only"), span(lipgloss.NewStyle(), " ")}, right...)
	}
	return paint(m.width, p.Page, left, right)
}

func (m Model) modeLabel() string {
	if m.full {
		return "full file"
	}
	return "changes only"
}

func (m Model) hintSegs(h keyHint) []seg {
	p := m.pal
	var segs []seg
	for j, k := range h.keys {
		if j > 0 {
			segs = append(segs, span(fg(p.Stats), " "))
		}
		segs = append(segs, span(lipgloss.NewStyle().Background(p.Chip).Foreground(p.Key), k))
	}
	return append(segs, span(fg(p.Stats), " "+h.label))
}

func (m Model) bottomBar() []string {
	p := m.pal
	sep := span(fg(p.Faint), " · ")
	var lines [][]seg
	var line []seg
	used := 0
	for _, h := range keyHints(m.filesFocus, m.change.Immutable) {
		segs := m.hintSegs(h)
		w := lipgloss.Width(render(p.Page, segs))
		switch {
		case len(line) == 0:
		case used+lipgloss.Width(sep.text)+w <= m.width:
			line = append(line, sep)
			used += lipgloss.Width(sep.text)
		default:
			lines = append(lines, line)
			line, used = nil, 0
		}
		line = append(line, segs...)
		used += w
	}
	status := []seg{span(fg(p.Status), m.statusText())}
	out := make([]string, 0, len(lines)+2)
	for _, l := range lines {
		out = append(out, paint(m.width, p.Page, l, nil))
	}
	if used+1+lipgloss.Width(m.statusText()) <= m.width {
		return append(out, paint(m.width, p.Page, line, status))
	}
	return append(out, paint(m.width, p.Page, line, nil), paint(m.width, p.Page, nil, status))
}

func (m Model) statusText() string {
	if m.status != "" {
		return m.status
	}
	if !m.ready() {
		return ""
	}
	n := len(m.files[m.fileIdx].Hunks)
	text := fmt.Sprintf("file %d/%d · %d %s", m.fileIdx+1, len(m.files), n, plural(n, "hunk"))
	if folded := m.currentReview().foldedCount(); folded > 0 && n > 0 {
		text += fmt.Sprintf(" (%d folded)", folded)
	}
	return text
}

func (m Model) card(width int, title string, focused bool, value []seg, body []string) string {
	p := m.pal
	inner := max(width-2, 0)
	edge := fg(p.Border)
	if focused {
		edge = fg(p.Selection)
	}
	side := edge.Background(p.Card).Render("│")
	lines := []string{
		edge.Render("╭" + strings.Repeat("─", inner) + "╮"),
		side + paint(inner, p.Card, []seg{span(fg(p.Header), " "+strings.ToUpper(title))}, append(value, span(lipgloss.NewStyle(), " "))) + side,
		edge.Background(p.Card).Render("├" + strings.Repeat("─", inner) + "┤"),
	}
	for i := range max(m.bodyHeight()-cardChrome, 0) {
		content := blank(inner, p.Card)
		if i < len(body) {
			content = clip(body[i], inner)
			content += blank(inner-lipgloss.Width(content), p.Card)
		}
		lines = append(lines, side+content+side)
	}
	lines = append(lines, edge.Render("╰"+strings.Repeat("─", inner)+"╯"))
	return strings.Join(lines[:min(len(lines), m.bodyHeight())], "\n")
}

func (m Model) filesCard() string {
	done := 0
	for _, r := range m.reviews {
		if r.state() == stateDone {
			done++
		}
	}
	value := []seg{span(fg(m.pal.Title), fmt.Sprintf("%d/%d ✓", done, len(m.files)))}
	rows := m.treeRows()
	inner := max(m.listWidth()-2, 0)
	var body []string
	for i := m.listTop; i < len(rows) && len(body) < m.diffHeight(); i++ {
		body = append(body, m.treeLine(rows[i], inner))
	}
	return m.card(m.listWidth(), "Files", m.filesFocus, value, body)
}

func (m Model) treeLine(r treeRow, width int) string {
	p := m.pal
	if r.dir {
		left := []seg{span(lipgloss.NewStyle(), strings.Repeat(" ", 1+r.depth*2)), span(fg(p.Muted), "▾ "), span(fg(p.DirName), r.name)}
		return paint(width, p.Card, left, []seg{span(fg(p.Muted), fmt.Sprint(r.count)), span(lipgloss.NewStyle(), " ")})
	}
	name, bg, bar := fg(p.Strong), p.Card, p.Card
	if r.file == m.fileIdx {
		name, bg, bar = fg(p.Selection).Bold(true), p.CursorBg, p.Selection
	}
	st := m.reviews[r.file].state()
	left := []seg{
		span(fg(bar), barGlyph(bar)),
		span(lipgloss.NewStyle(), strings.Repeat(" ", r.depth*2)),
		span(m.glyphStyle(st), st.glyph()),
		span(lipgloss.NewStyle(), " "),
		span(name, r.name),
	}
	return paint(width, bg, left, []seg{span(fg(p.Muted), m.fileSummary(r.file)), span(lipgloss.NewStyle(), " ")})
}

func (m Model) glyphStyle(st fileState) lipgloss.Style {
	switch st {
	case stateDone:
		return fg(m.pal.Added)
	case stateNone:
		return fg(m.pal.Muted)
	}
	return fg(m.pal.Picked)
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
	var value []seg
	if file, ok := m.currentFile(); ok {
		value = []seg{span(fg(m.pal.Title), file.Path())}
	}
	rows := m.rows()
	width := m.diffCardWidth()
	inner := max(width-2, 0)
	var body []string
	for i := m.diffTop; i < len(rows) && len(body) < m.diffHeight(); i++ {
		body = append(body, m.diffLine(rows[i], i == m.cursor, inner))
	}
	return m.card(width, "Unified diff", !m.filesFocus, value, body)
}
