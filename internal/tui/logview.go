package tui

import (
	"fmt"
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/madalinpopa/jjreview/internal/diff"
	"github.com/madalinpopa/jjreview/internal/jj"
	"github.com/madalinpopa/jjreview/internal/theme"
)

type view int

const (
	reviewView view = iota
	logView
)

type logState struct {
	lines     []jj.LogLine
	shown     []jj.LogLine
	query     textinput.Model
	searching bool
	expanded  map[string]bool
	files     map[string]logFiles
	loaded    bool
	loading   bool
	stale     bool
	err       error
	cursor    int
	followed  string
	top       int
	seq       int
}

type logFiles struct {
	files  []diff.File
	err    error
	loaded bool
}

type logFilesMsg struct {
	commitID string
	files    []diff.File
	err      error
}

type logRow struct {
	line int
	file int
}

const (
	logEntryRow   = -1
	logMessageRow = -2
)

const logMinDescription = 20

func newLogState() logState {
	return logState{query: newSearchInput(), expanded: map[string]bool{}, files: map[string]logFiles{}}
}

type logMsg struct {
	seq   int
	lines []jj.LogLine
	err   error
}

func (m Model) LogCursor() string {
	if e := m.log.selected(); e != nil {
		return e.ChangeID
	}
	return ""
}

func newSearchInput() textinput.Model {
	in := textinput.New()
	in.Prompt = "/ "
	styles := in.Styles()
	styles.Cursor.Blink = false
	in.SetStyles(styles)
	return in
}

func (m *Model) resizeSearch() {
	q := &m.log.query
	value, pos := q.Value(), q.Position()
	q.SetWidth(max(m.width-6, 1))
	q.SetValue("")
	q.SetValue(value)
	q.SetCursor(pos)
}

func searchStyles(p theme.Theme) textinput.Styles {
	base := lipgloss.NewStyle().Background(p.Card)
	state := textinput.StyleState{
		Text:        base.Foreground(p.Strong),
		Placeholder: base.Foreground(p.Muted),
		Suggestion:  base.Foreground(p.Muted),
		Prompt:      base.Foreground(p.Key),
	}
	return textinput.Styles{
		Focused: state,
		Blurred: state,
		Cursor:  textinput.CursorStyle{Color: p.Selection, Shape: tea.CursorBar},
	}
}

func (l logState) filtered() bool {
	return l.searching || l.query.Value() != ""
}

func (l *logState) refilter() {
	keep := l.selected()
	q := strings.ToLower(strings.TrimSpace(l.query.Value()))
	if q == "" {
		l.shown = l.lines
	} else {
		l.shown = nil
		for _, line := range l.lines {
			if line.Entry != nil && matches(*line.Entry, q) {
				l.shown = append(l.shown, line)
			}
		}
	}
	if keep == nil || !l.focusWhere(func(e *jj.LogEntry) bool { return e.CommitID == keep.CommitID }) {
		l.focusWhere(func(*jj.LogEntry) bool { return true })
	}
}

func matches(e jj.LogEntry, query string) bool {
	return strings.HasPrefix(strings.ToLower(e.ChangeID), query) ||
		strings.HasPrefix(strings.ToLower(e.CommitID), query) ||
		strings.Contains(strings.ToLower(e.Description), query)
}

func (l logState) selected() *jj.LogEntry {
	if l.cursor < 0 || l.cursor >= len(l.shown) {
		return nil
	}
	return l.shown[l.cursor].Entry
}

func (l logState) selectedCommit() string {
	if e := l.selected(); e != nil {
		return e.CommitID
	}
	return ""
}

func (l *logState) focusWhere(pred func(*jj.LogEntry) bool) bool {
	i := slices.IndexFunc(l.shown, func(line jj.LogLine) bool { return line.Entry != nil && pred(line.Entry) })
	if i < 0 {
		return false
	}
	l.cursor = i
	return true
}

func (l *logState) focus(commitID string) {
	switch {
	case l.focusWhere(func(e *jj.LogEntry) bool { return e.CommitID == commitID }):
	case l.focusWhere(func(e *jj.LogEntry) bool { return e.WorkingCopy }):
	case l.focusWhere(func(*jj.LogEntry) bool { return true }):
	default:
		l.cursor = 0
	}
	l.followed = l.selectedCommit()
}

func (l *logState) follow(commitID string) {
	if l.loaded && l.selectedCommit() == l.followed {
		l.focus(commitID)
	}
}

func (l *logState) move(delta int) {
	step := 1
	if delta < 0 {
		step = -1
	}
	for i := l.cursor + step; delta != 0 && i >= 0 && i < len(l.shown); i += step {
		if l.shown[i].Entry != nil {
			l.cursor = i
			delta -= step
		}
	}
}

func (l *logState) jump(delta int) {
	l.move(delta)
	l.top = min(l.top, l.cursorRow(l.rows()))
}

func (l logState) position() (int, int) {
	at, total := 0, 0
	for i, line := range l.shown {
		if line.Entry == nil {
			continue
		}
		total++
		if i <= l.cursor {
			at = total
		}
	}
	return at, total
}

func (m *Model) showLog() tea.Cmd {
	m.view = logView
	if !m.log.loading && (!m.log.loaded || m.log.stale) {
		return m.loadLog()
	}
	m.log.focus(m.targetHead)
	return nil
}

func (m *Model) loadLog() tea.Cmd {
	m.log.seq++
	m.log.loading = true
	m.log.stale = false
	seq, ctx, src := m.log.seq, m.ctx, m.src
	return func() tea.Msg {
		lines, err := src.Log(ctx)
		if err != nil {
			err = fmt.Errorf("load log: %w", err)
		}
		return logMsg{seq: seq, lines: lines, err: err}
	}
}

func (m *Model) applyLog(msg logMsg) tea.Cmd {
	if msg.seq != m.log.seq {
		return nil
	}
	before := m.log.selectedCommit()
	m.log.loading = false
	m.log.loaded = true
	m.log.err = msg.err
	if msg.err != nil {
		m.log.lines = nil
		m.log.searching = false
		m.log.query.Blur()
	} else {
		m.log.lines = msg.lines
	}
	m.log.refilter()
	m.log.focus(m.targetHead)
	if m.log.selectedCommit() != before && slices.Contains(m.keys.open.Keys(), m.armed) {
		m.armed = ""
		m.status = ""
	}
	if m.log.stale && m.view == logView {
		return m.loadLog()
	}
	return nil
}

func (m *Model) logKey(msg tea.KeyPressMsg) tea.Cmd {
	m.status = ""
	if m.log.searching {
		m.armed = ""
		return m.searchKey(msg)
	}
	armed := m.armed
	m.armed = ""
	switch {
	case key.Matches(msg, m.keys.reviewTab):
		m.view = reviewView
	case key.Matches(msg, m.keys.search):
		if !m.log.loaded || m.log.err != nil {
			return nil
		}
		m.log.searching = true
		return m.log.query.Focus()
	case key.Matches(msg, m.keys.clearSearch):
		m.clearSearch()
	case key.Matches(msg, m.keys.quit):
		if m.confirmDrop(msg, armed) {
			return m.quit()
		}
	case key.Matches(msg, m.keys.down):
		m.log.move(1)
	case key.Matches(msg, m.keys.up):
		m.log.move(-1)
	case key.Matches(msg, m.keys.halfDown):
		m.log.scrollPage(m.halfPage(), m.logHeight())
	case key.Matches(msg, m.keys.halfUp):
		m.log.scrollPage(-m.halfPage(), m.logHeight())
	case key.Matches(msg, m.keys.top):
		m.log.jump(-len(m.log.shown))
	case key.Matches(msg, m.keys.bottom):
		m.log.jump(len(m.log.shown))
	case key.Matches(msg, m.keys.open):
		return m.openPicked(msg, armed)
	case key.Matches(msg, m.keys.expand):
		return m.expandFiles()
	case key.Matches(msg, m.keys.collapse):
		if e := m.log.selected(); e != nil {
			delete(m.log.expanded, e.CommitID)
		}
	case key.Matches(msg, m.keys.reload):
		if !m.log.loading {
			return m.loadLog()
		}
	case key.Matches(msg, m.keys.help):
		m.help = true
		m.helpTop = 0
	}
	return nil
}

func (m *Model) expandFiles() tea.Cmd {
	e := m.log.selected()
	if e == nil {
		return nil
	}
	id := e.CommitID
	m.log.expanded[id] = true
	if f, ok := m.log.files[id]; ok && (!f.loaded || f.err == nil) {
		return nil
	}
	m.log.files[id] = logFiles{}
	ctx, src := m.ctx, m.src
	return func() tea.Msg {
		files, err := src.Diff(ctx, id)
		if err != nil {
			err = fmt.Errorf("load files of %s: %w", shortID(id), err)
		}
		paths := make([]diff.File, 0, len(files))
		for _, f := range files {
			paths = append(paths, diff.File{OldPath: f.OldPath, NewPath: f.NewPath, Status: f.Status})
		}
		return logFilesMsg{commitID: id, files: paths, err: err}
	}
}

func (m *Model) applyLogFiles(msg logFilesMsg) {
	m.log.files[msg.commitID] = logFiles{files: msg.files, err: msg.err, loaded: true}
}

func (l logState) rows() []logRow {
	var rows []logRow
	for i, line := range l.shown {
		rows = append(rows, logRow{line: i, file: logEntryRow})
		if line.Entry == nil || !l.expanded[line.Entry.CommitID] {
			continue
		}
		f := l.files[line.Entry.CommitID]
		if !f.loaded || f.err != nil || len(f.files) == 0 {
			rows = append(rows, logRow{line: i, file: logMessageRow})
			continue
		}
		for j := range f.files {
			rows = append(rows, logRow{line: i, file: j})
		}
	}
	return rows
}

func (l logState) cursorRow(rows []logRow) int {
	return max(slices.Index(rows, logRow{line: l.cursor, file: logEntryRow}), 0)
}

func (l *logState) keepVisible(height int) {
	rows := l.rows()
	top := min(l.top, max(len(rows)-height, 0))
	start := l.cursorRow(rows)
	end := start
	for end+1 < len(rows) && rows[end+1].line == l.cursor {
		end++
	}
	switch {
	case end < top:
		top = start
	case start >= top+height:
		top = start - height + 1
	}
	l.top = max(top, 0)
}

func (l *logState) scrollPage(delta, height int) {
	rows := l.rows()
	if len(rows) == 0 {
		return
	}
	offset := min(max(l.cursorRow(rows)-l.top, 0), height-1)
	target := min(max(l.top+offset+delta, 0), len(rows)-1)
	l.top = min(max(l.top+delta, 0), max(len(rows)-height, 0))
	l.cursor = rows[target].line
	if l.selected() != nil {
		return
	}
	step := 1
	if delta < 0 {
		step = -1
	}
	l.move(step)
	if l.selected() == nil {
		l.move(-step)
	}
}

func (m *Model) searchKey(msg tea.KeyPressMsg) tea.Cmd {
	switch {
	case key.Matches(msg, m.keys.clearSearch):
		m.clearSearch()
	case key.Matches(msg, m.keys.open):
		m.log.searching = false
		m.log.query.Blur()
	default:
		return m.updateQuery(msg)
	}
	return nil
}

func (m *Model) updateQuery(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	m.log.query, cmd = m.log.query.Update(msg)
	m.log.refilter()
	return cmd
}

func (m *Model) clearSearch() {
	m.log.searching = false
	m.log.query.Blur()
	m.log.query.Reset()
	m.log.refilter()
}

func (m *Model) openPicked(msg tea.KeyPressMsg, armed string) tea.Cmd {
	e := m.log.selected()
	if e == nil {
		return nil
	}
	if e.CommitID == m.targetHead {
		m.view = reviewView
		return nil
	}
	if !m.canLoad() || !m.confirmDrop(msg, armed) {
		return nil
	}
	return m.loadRevision(pickedStep, jj.CommitRevset(e.CommitID))
}

func (m Model) logCard() string {
	p := m.pal
	inner := max(m.width-2, 0)
	var body []string
	var value []seg
	switch {
	case m.log.err != nil:
		body = []string{paint(inner, p.Card, []seg{span(fg(p.Removed), "  error: "+m.log.err.Error())}, nil)}
	case !m.log.loaded:
		body = []string{m.logMessage("loading…", inner)}
	default:
		if m.log.filtered() {
			q := m.log.query
			q.SetStyles(searchStyles(p))
			body = []string{paint(inner, p.Card, []seg{span(lipgloss.NewStyle(), " "), span(lipgloss.NewStyle(), q.View())}, nil)}
		}
		at, total := m.log.position()
		if total == 0 {
			empty := "no revisions"
			if m.log.filtered() {
				empty = "no matching revisions"
			}
			body = append(body, m.logMessage(empty, inner))
			break
		}
		value = []seg{span(fg(p.Title), fmt.Sprintf("rev %d/%d", at, total))}
		rows := m.log.rows()
		for i := m.log.top; i < len(rows) && len(body) < m.diffHeight(); i++ {
			if rows[i].file == logEntryRow {
				body = append(body, m.logLine(rows[i].line, inner))
			} else {
				body = append(body, m.logFileLine(rows[i], inner))
			}
		}
	}
	return m.card(m.width, "Log", true, value, body)
}

func (m Model) logHeight() int {
	if m.log.filtered() {
		return max(m.diffHeight()-1, 1)
	}
	return m.diffHeight()
}

func (m Model) logMessage(text string, width int) string {
	return paint(width, m.pal.Card, []seg{span(fg(m.pal.Muted).Italic(true), "  "+text)}, nil)
}

func (m Model) logLine(i, width int) string {
	p := m.pal
	line := m.log.shown[i]
	bg := p.Card
	if i == m.log.cursor {
		bg = p.CursorBg
	}
	e := line.Entry
	bar := bg
	if e != nil && e.CommitID == m.targetHead {
		bar = p.Selection
	}
	left := []seg{span(fg(bar), barGlyph(bar)), span(fg(p.Muted), line.Graph)}
	if e == nil {
		return paint(width, bg, left, nil)
	}
	id := fg(p.Title)
	if e.WorkingCopy {
		id = id.Bold(true)
	}
	left = append(left, span(id, shortID(e.ChangeID)), span(lipgloss.NewStyle(), " "))
	for _, b := range e.Bookmarks {
		left = append(left, m.pill(p.Chip, p.Key, b), span(lipgloss.NewStyle(), " "))
	}
	desc := m.logDescription(*e)
	right := []seg{span(fg(p.Muted), strings.Join(slices.DeleteFunc([]string{e.Author, e.Timestamp}, func(s string) bool { return s == "" }), " · ")), span(lipgloss.NewStyle(), " ")}
	if rowWidth(left, right)+min(lipgloss.Width(desc.text), logMinDescription) > width {
		right = nil
	}
	return paint(width, bg, append(left, desc), right)
}

func (m Model) logFileLine(r logRow, width int) string {
	p := m.pal
	e := m.log.shown[r.line].Entry
	left := []seg{span(fg(p.Card), barGlyph(p.Card)), span(fg(p.Muted), continuation(m.log.shown[r.line].Graph, r.line == len(m.log.shown)-1)), span(lipgloss.NewStyle(), "  ")}
	f := m.log.files[e.CommitID]
	switch {
	case !f.loaded:
		left = append(left, span(fg(p.Muted).Italic(true), "loading…"))
	case f.err != nil:
		left = append(left, span(fg(p.Removed), "error: "+f.err.Error()))
	case len(f.files) == 0:
		left = append(left, span(fg(p.Muted).Italic(true), "no changes"))
	default:
		file := f.files[r.file]
		letter, style := m.fileStatus(file.Status)
		name := escapePath(file.Path())
		if file.Status == diff.StatusRenamed || file.Status == diff.StatusCopied {
			name = escapePath(file.OldPath) + " → " + escapePath(file.NewPath)
		}
		left = append(left, span(style, letter), span(lipgloss.NewStyle(), " "), span(fg(p.Strong), name))
	}
	return paint(width, p.Card, left, nil)
}

func (m Model) fileStatus(s diff.Status) (string, lipgloss.Style) {
	switch s {
	case diff.StatusAdded:
		return "A", fg(m.pal.Added)
	case diff.StatusDeleted:
		return "D", fg(m.pal.Removed)
	case diff.StatusRenamed:
		return "R", fg(m.pal.Key)
	case diff.StatusCopied:
		return "C", fg(m.pal.Key)
	}
	return "M", fg(m.pal.Picked)
}

func continuation(graph string, last bool) string {
	var b strings.Builder
	for _, r := range graph {
		switch {
		case r == '│' || r == ' ':
			b.WriteRune(r)
		case strings.ContainsRune("├┤─╮╭╯╰~", r) || last:
			b.WriteRune(' ')
		default:
			b.WriteRune('│')
		}
	}
	return b.String()
}

func (m Model) logDescription(e jj.LogEntry) seg {
	title, _, _ := strings.Cut(e.Description, "\n")
	switch {
	case e.Description == "":
		return span(fg(m.pal.Muted).Italic(true), "(no description set)")
	case e.Immutable:
		return span(fg(m.pal.Muted), title)
	}
	return span(fg(m.pal.Strong), title)
}
