package tui

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/madalinpopa/jjreview/internal/diff"
	"github.com/madalinpopa/jjreview/internal/jj"
	"github.com/madalinpopa/jjreview/internal/theme"
	"github.com/madalinpopa/jjreview/internal/tui/ui"
)

type logSource interface {
	Log(ctx context.Context) ([]jj.LogLine, error)
	Diff(ctx context.Context, rev string) ([]diff.File, error)
}

type logKeys struct {
	down        key.Binding
	up          key.Binding
	halfDown    key.Binding
	halfUp      key.Binding
	top         key.Binding
	bottom      key.Binding
	open        key.Binding
	search      key.Binding
	clearSearch key.Binding
	expand      key.Binding
	collapse    key.Binding
	abandon     key.Binding
	confirm     key.Binding
	reload      key.Binding
}

func newLogKeys() logKeys {
	return logKeys{
		down:        key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("j", "next revision")),
		up:          key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("k", "previous revision")),
		halfDown:    key.NewBinding(key.WithKeys("ctrl+d", "pgdown"), key.WithHelp("^d", "half page down")),
		halfUp:      key.NewBinding(key.WithKeys("ctrl+u", "pgup"), key.WithHelp("^u", "half page up")),
		top:         key.NewBinding(key.WithKeys("g", "home"), key.WithHelp("g", "first revision")),
		bottom:      key.NewBinding(key.WithKeys("G", "end"), key.WithHelp("G", "last revision")),
		open:        key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "review the revision")),
		search:      key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search by ID or description")),
		clearSearch: key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "clear the search")),
		expand:      key.NewBinding(key.WithKeys("l"), key.WithHelp("l", "show the files of the revision")),
		collapse:    key.NewBinding(key.WithKeys("h"), key.WithHelp("h", "hide the files of the revision")),
		abandon:     key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "abandon the revision (y confirms)")),
		confirm:     key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "confirm")),
		reload:      key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "refresh the log")),
	}
}

type logEnv struct {
	pal     theme.Theme
	width   int
	height  int
	body    int
	list    int
	modal   int
	head    string
	active  bool
	dropped int
}

type logIntent struct {
	open     string
	arm      bool
	abandon  *jj.LogEntry
	refresh  bool
	retarget bool
}

type logModel struct {
	ctx       context.Context
	src       logSource
	keys      logKeys
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
	keep      logPosition
	confirm   *jj.LogEntry
	top       int
	seq       int
}

type logPosition struct {
	at     int
	cursor int
	query  string
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

const defaultWorkspace = "default"

func newLogModel(ctx context.Context, src logSource) logModel {
	return logModel{ctx: ctx, src: src, keys: newLogKeys(), query: newSearchInput(), expanded: map[string]bool{}, files: map[string]logFiles{}}
}

type logMsg struct {
	seq   int
	lines []jj.LogLine
	err   error
}

func newSearchInput() textinput.Model {
	in := textinput.New()
	in.Prompt = "/ "
	styles := in.Styles()
	styles.Cursor.Blink = false
	in.SetStyles(styles)
	return in
}

func (l *logModel) resize(width int) {
	q := &l.query
	value, pos := q.Value(), q.Position()
	q.SetWidth(max(width-6, 1))
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

func (l logModel) filtered() bool {
	return l.searching || l.query.Value() != ""
}

func (l logModel) confirming() bool {
	return l.confirm != nil
}

func (l logModel) capturing() bool {
	return l.searching || l.confirm != nil
}

func (l *logModel) refilter() {
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

func (l logModel) selected() *jj.LogEntry {
	if l.cursor < 0 || l.cursor >= len(l.shown) {
		return nil
	}
	return l.shown[l.cursor].Entry
}

func (l logModel) selectedCommit() string {
	if e := l.selected(); e != nil {
		return e.CommitID
	}
	return ""
}

func (l *logModel) focusWhere(pred func(*jj.LogEntry) bool) bool {
	i := slices.IndexFunc(l.shown, func(line jj.LogLine) bool { return line.Entry != nil && pred(line.Entry) })
	if i < 0 {
		return false
	}
	l.cursor = i
	return true
}

func (l *logModel) focus(commitID string) {
	switch {
	case l.focusWhere(func(e *jj.LogEntry) bool { return e.CommitID == commitID }):
	case l.focusWhere(func(e *jj.LogEntry) bool { return e.WorkingCopy }):
	case l.focusWhere(func(*jj.LogEntry) bool { return true }):
	default:
		l.cursor = 0
	}
	l.followed = l.selectedCommit()
}

func (l *logModel) focusNth(n int) {
	seen := 0
	for i, line := range l.shown {
		if line.Entry == nil {
			continue
		}
		l.cursor = i
		if seen++; seen == n {
			break
		}
	}
	l.followed = ""
}

func (l *logModel) follow(commitID string) {
	if l.loaded && l.selectedCommit() == l.followed {
		l.focus(commitID)
	}
}

func (l *logModel) move(delta int) {
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

func (l *logModel) jump(delta int) {
	l.move(delta)
	l.top = min(l.top, l.cursorRow(l.rows()))
}

func (l logModel) position() (int, int) {
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

func (l *logModel) holdPosition() logPosition {
	at, _ := l.position()
	l.followed = ""
	return logPosition{at: at, cursor: l.cursor, query: l.query.Value()}
}

func (l *logModel) restorePosition(keep logPosition) {
	l.keep = keep
}

func (l *logModel) show(head string) tea.Cmd {
	if !l.loading && (!l.loaded || l.stale) {
		return l.load()
	}
	l.focus(head)
	return nil
}

func (l *logModel) load() tea.Cmd {
	l.seq++
	l.loading = true
	l.stale = false
	seq, ctx, src := l.seq, l.ctx, l.src
	return func() tea.Msg {
		lines, err := src.Log(ctx)
		if err != nil {
			err = fmt.Errorf("load log: %w", err)
		}
		return logMsg{seq: seq, lines: lines, err: err}
	}
}

func (l *logModel) update(msg tea.Msg, env logEnv) (tea.Cmd, logIntent) {
	switch msg := msg.(type) {
	case logMsg:
		return l.applyList(msg, env)
	case logFilesMsg:
		l.files[msg.commitID] = logFiles{files: msg.files, err: msg.err, loaded: true}
	}
	return nil, logIntent{}
}

func (l *logModel) applyList(msg logMsg, env logEnv) (tea.Cmd, logIntent) {
	if msg.seq != l.seq {
		return nil, logIntent{}
	}
	before := l.selectedCommit()
	l.loading = false
	l.loaded = true
	l.err = msg.err
	if msg.err != nil {
		l.lines = nil
		l.searching = false
		l.query.Blur()
	} else {
		l.lines = msg.lines
	}
	keep := l.keep
	restore := keep.at > 0 && msg.err == nil && l.cursor == keep.cursor && l.query.Value() == keep.query
	l.keep = logPosition{}
	l.refilter()
	switch {
	case restore:
		l.focusNth(keep.at)
	case keep.at > 0 && before != "" && l.selectedCommit() == before:
	default:
		l.focus(env.head)
	}
	in := logIntent{retarget: env.active && l.selectedCommit() != before}
	if l.stale && env.active {
		return l.load(), in
	}
	return nil, in
}

func (l logModel) ignores(msg tea.KeyPressMsg, reloading bool) bool {
	return l.confirm == nil && !l.searching && key.Matches(msg, l.keys.abandon, l.keys.confirm) && (msg.IsRepeat || reloading || l.loading)
}

func (l *logModel) key(msg tea.KeyPressMsg, env logEnv) (tea.Cmd, logIntent) {
	if target := l.confirm; target != nil {
		l.confirm = nil
		if e := l.selected(); !msg.IsRepeat && key.Matches(msg, l.keys.confirm) && e != nil && e.CommitID == target.CommitID {
			return nil, logIntent{abandon: target}
		}
		return nil, logIntent{}
	}
	if l.searching {
		return l.searchKey(msg), logIntent{}
	}
	half := max(env.list/2, 1)
	switch {
	case key.Matches(msg, l.keys.search):
		if !l.loaded || l.err != nil {
			return nil, logIntent{}
		}
		l.searching = true
		return l.query.Focus(), logIntent{}
	case key.Matches(msg, l.keys.clearSearch):
		l.clearSearch()
	case key.Matches(msg, l.keys.down):
		l.move(1)
	case key.Matches(msg, l.keys.up):
		l.move(-1)
	case key.Matches(msg, l.keys.halfDown):
		l.scrollPage(half, l.height(env))
	case key.Matches(msg, l.keys.halfUp):
		l.scrollPage(-half, l.height(env))
	case key.Matches(msg, l.keys.top):
		l.jump(-len(l.shown))
	case key.Matches(msg, l.keys.bottom):
		l.jump(len(l.shown))
	case key.Matches(msg, l.keys.open):
		if e := l.selected(); e != nil {
			return nil, logIntent{open: e.CommitID}
		}
	case key.Matches(msg, l.keys.expand):
		return l.expandFiles(), logIntent{}
	case key.Matches(msg, l.keys.abandon):
		return nil, logIntent{arm: l.selected() != nil}
	case key.Matches(msg, l.keys.collapse):
		if e := l.selected(); e != nil {
			delete(l.expanded, e.CommitID)
		}
	case key.Matches(msg, l.keys.reload):
		return nil, logIntent{refresh: !l.loading}
	}
	return nil, logIntent{}
}

func (l *logModel) arm() string {
	e := l.selected()
	if e == nil {
		return ""
	}
	if e.Immutable {
		return ui.ShortID(e.ChangeID) + " is immutable"
	}
	target := *e
	target.Bookmarks = slices.Clone(e.Bookmarks)
	l.confirm = &target
	return ""
}

func (l *logModel) expandFiles() tea.Cmd {
	e := l.selected()
	if e == nil {
		return nil
	}
	id := e.CommitID
	l.expanded[id] = true
	if f, ok := l.files[id]; ok && (!f.loaded || f.err == nil) {
		return nil
	}
	l.files[id] = logFiles{}
	ctx, src := l.ctx, l.src
	return func() tea.Msg {
		files, err := src.Diff(ctx, id)
		if err != nil {
			err = fmt.Errorf("load files of %s: %w", ui.ShortID(id), err)
		}
		paths := make([]diff.File, 0, len(files))
		for _, f := range files {
			paths = append(paths, diff.File{OldPath: f.OldPath, NewPath: f.NewPath, Status: f.Status})
		}
		return logFilesMsg{commitID: id, files: paths, err: err}
	}
}

func (l logModel) rows() []logRow {
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

func (l logModel) cursorRow(rows []logRow) int {
	return max(slices.Index(rows, logRow{line: l.cursor, file: logEntryRow}), 0)
}

func (l logModel) height(env logEnv) int {
	if l.filtered() {
		return max(env.list-1, 1)
	}
	return env.list
}

func (l *logModel) keepVisible(env logEnv) {
	height := l.height(env)
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

func (l *logModel) scrollPage(delta, height int) {
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

func (l *logModel) searchKey(msg tea.KeyPressMsg) tea.Cmd {
	switch {
	case key.Matches(msg, l.keys.clearSearch):
		l.clearSearch()
	case key.Matches(msg, l.keys.open):
		l.searching = false
		l.query.Blur()
	default:
		return l.input(msg)
	}
	return nil
}

func (l *logModel) input(msg tea.Msg) tea.Cmd {
	if !l.searching {
		return nil
	}
	var cmd tea.Cmd
	l.query, cmd = l.query.Update(msg)
	l.refilter()
	return cmd
}

func (l *logModel) clearSearch() {
	l.searching = false
	l.query.Blur()
	l.query.Reset()
	l.refilter()
}

func (l logModel) hints(tabs, help ui.KeyHint) []ui.KeyHint {
	if l.searching {
		return []ui.KeyHint{ui.Hint("apply", l.keys.open), ui.Hint("clear", l.keys.clearSearch)}
	}
	hints := []ui.KeyHint{
		tabs,
		ui.Hint("revision", l.keys.down, l.keys.up),
		ui.Hint("review", l.keys.open),
		ui.Hint("files", l.keys.expand, l.keys.collapse),
		ui.Hint("search", l.keys.search),
	}
	if l.filtered() {
		hints = append(hints, ui.Hint("clear", l.keys.clearSearch))
	}
	return append(hints, ui.Hint("refresh", l.keys.reload), help)
}

func (l logModel) help() []ui.KeyHint {
	return []ui.KeyHint{
		ui.Hint("next / previous revision", l.keys.down, l.keys.up),
		ui.Hint("half page down / up", l.keys.halfDown, l.keys.halfUp),
		ui.Hint("first / last revision", l.keys.top, l.keys.bottom),
		ui.Describe(l.keys.open),
		ui.Describe(l.keys.expand),
		ui.Describe(l.keys.collapse),
		ui.Describe(l.keys.abandon),
		ui.Describe(l.keys.search),
		ui.Describe(l.keys.clearSearch),
		ui.Describe(l.keys.reload),
	}
}

func (l logModel) view(env logEnv) string {
	p := env.pal
	inner := max(env.width-2, 0)
	var body []string
	var value []ui.Seg
	switch {
	case l.err != nil:
		body = []string{ui.Paint(inner, p.Card, []ui.Seg{ui.Span(ui.Fg(p.Removed), "  error: "+l.err.Error())}, nil)}
	case !l.loaded:
		body = []string{ui.Message(p, "loading…", inner)}
	default:
		if l.filtered() {
			q := l.query
			q.SetStyles(searchStyles(p))
			body = []string{ui.Paint(inner, p.Card, []ui.Seg{ui.Span(lipgloss.NewStyle(), " "), ui.Span(lipgloss.NewStyle(), q.View())}, nil)}
		}
		at, total := l.position()
		if total == 0 {
			empty := "no revisions"
			if l.filtered() {
				empty = "no matching revisions"
			}
			body = append(body, ui.Message(p, empty, inner))
			break
		}
		value = []ui.Seg{ui.Span(ui.Fg(p.Title), fmt.Sprintf("rev %d/%d", at, total))}
		rows := l.rows()
		for i := l.top; i < len(rows) && len(body) < env.list; i++ {
			if rows[i].file == logEntryRow {
				body = append(body, l.line(env, rows[i].line, inner))
			} else {
				body = append(body, l.fileLine(p, rows[i], inner))
			}
		}
	}
	return ui.Card(p, env.width, env.body, "Log", true, value, body)
}

func (l logModel) line(env logEnv, i, width int) string {
	p := env.pal
	line := l.shown[i]
	bg := p.Card
	if i == l.cursor {
		bg = p.CursorBg
	}
	e := line.Entry
	bar := bg
	if e != nil && e.CommitID == env.head {
		bar = p.Selection
	}
	left := []ui.Seg{ui.Span(ui.Fg(bar), ui.BarGlyph(bar)), ui.Span(ui.Fg(p.Muted), line.Graph)}
	if e == nil {
		return ui.Paint(width, bg, left, nil)
	}
	id := ui.Fg(p.Title)
	if e.WorkingCopy {
		id = id.Bold(true)
	}
	left = append(left, ui.Span(id, ui.ShortID(e.ChangeID)), ui.Span(lipgloss.NewStyle(), " "))
	for _, w := range e.Workspaces {
		if w != defaultWorkspace {
			left = append(left, ui.Pill(p.Chip, p.Added, w+"@"), ui.Span(lipgloss.NewStyle(), " "))
		}
	}
	for _, b := range e.Bookmarks {
		left = append(left, ui.Pill(p.Chip, p.Key, b), ui.Span(lipgloss.NewStyle(), " "))
	}
	desc := ui.Description(p, e.Description, e.Immutable)
	right := []ui.Seg{ui.Span(ui.Fg(p.Muted), strings.Join(slices.DeleteFunc([]string{e.Author, e.Timestamp}, func(s string) bool { return s == "" }), " · ")), ui.Span(lipgloss.NewStyle(), " ")}
	if ui.RowWidth(left, right)+min(lipgloss.Width(desc.Text), logMinDescription) > width {
		right = nil
	}
	return ui.Paint(width, bg, append(left, desc), right)
}

func (l logModel) fileLine(p theme.Theme, r logRow, width int) string {
	e := l.shown[r.line].Entry
	left := []ui.Seg{ui.Span(ui.Fg(p.Card), ui.BarGlyph(p.Card)), ui.Span(ui.Fg(p.Muted), continuation(l.shown[r.line].Graph, r.line == len(l.shown)-1)), ui.Span(lipgloss.NewStyle(), "  ")}
	f := l.files[e.CommitID]
	switch {
	case !f.loaded:
		left = append(left, ui.Span(ui.Fg(p.Muted).Italic(true), "loading…"))
	case f.err != nil:
		left = append(left, ui.Span(ui.Fg(p.Removed), "error: "+f.err.Error()))
	case len(f.files) == 0:
		left = append(left, ui.Span(ui.Fg(p.Muted).Italic(true), "no changes"))
	default:
		file := f.files[r.file]
		letter, style := fileStatus(p, file.Status)
		name := ui.EscapePath(file.Path())
		if file.Status == diff.StatusRenamed || file.Status == diff.StatusCopied {
			name = ui.EscapePath(file.OldPath) + " → " + ui.EscapePath(file.NewPath)
		}
		left = append(left, ui.Span(style, letter), ui.Span(lipgloss.NewStyle(), " "), ui.Span(ui.Fg(p.Strong), name))
	}
	return ui.Paint(width, p.Card, left, nil)
}

func fileStatus(p theme.Theme, s diff.Status) (string, lipgloss.Style) {
	switch s {
	case diff.StatusAdded:
		return "A", ui.Fg(p.Added)
	case diff.StatusDeleted:
		return "D", ui.Fg(p.Removed)
	case diff.StatusRenamed:
		return "R", ui.Fg(p.Key)
	case diff.StatusCopied:
		return "C", ui.Fg(p.Key)
	}
	return "M", ui.Fg(p.Picked)
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

func (l logModel) confirmView(env logEnv) string {
	p := env.pal
	e := l.confirm
	inner := env.modal
	wrap := func(style lipgloss.Style, text string) []string {
		var out []string
		for line := range strings.SplitSeq(ansi.Wrap(text, inner, ""), "\n") {
			out = append(out, ui.Paint(inner, p.Card, []ui.Seg{ui.Span(style, line)}, nil))
		}
		return out
	}
	lines := wrap(ui.Fg(p.Title).Bold(true), "Abandon "+ui.ShortID(e.ChangeID)+" "+ui.ShortID(e.CommitID))
	if title, _, _ := strings.Cut(e.Description, "\n"); title != "" {
		lines = append(lines, wrap(ui.Fg(p.Strong), title)...)
	}
	if n := len(e.Bookmarks); n > 0 {
		lines = append(lines, wrap(ui.Fg(p.Removed), "Deletes "+ui.Plural(n, "bookmark")+" "+strings.Join(e.Bookmarks, ", "))...)
	}
	if n := env.dropped; n > 0 {
		lines = append(lines, wrap(ui.Fg(p.Removed), fmt.Sprintf("Drops %d selected %s", n, ui.Plural(n, "hunk")))...)
	}
	footer := wrap(ui.Fg(p.Muted), l.keys.confirm.Help().Key+" abandon · any other key cancels")
	if room := max(env.height-2-len(footer), 0); len(lines) > room {
		lines = lines[:room]
		if room > 0 {
			lines[room-1] = ui.Paint(inner, p.Card, []ui.Seg{ui.Span(ui.Fg(p.Muted), "…")}, nil)
		}
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(p.ModalBorder).
		BorderBackground(p.Card).
		Background(p.Card).
		Padding(0, 1).
		Render(strings.Join(append(lines, footer...), "\n"))
}
