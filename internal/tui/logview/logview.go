package logview

import (
	"context"
	"fmt"
	"maps"
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

type Source interface {
	Log(ctx context.Context) ([]jj.LogLine, error)
	Diff(ctx context.Context, rev string) ([]diff.File, error)
}

type keyMap struct {
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
	reset       key.Binding
	fetch       key.Binding
	push        key.Binding
	bookmark    key.Binding
	mark        key.Binding
	squash      key.Binding
	cancel      key.Binding
}

func newKeyMap() keyMap {
	return keyMap{
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
		reload:      key.NewBinding(key.WithKeys("ctrl+r"), key.WithHelp("ctrl+r", "refresh the log")),
		reset:       key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "clear the selection and hide all files")),
		fetch:       key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "fetch from the remote")),
		push:        key.NewBinding(key.WithKeys("P"), key.WithHelp("P", "push the bookmarks of the revision, or the change (y confirms)")),
		bookmark:    key.NewBinding(key.WithKeys("b"), key.WithHelp("b", "create or move a bookmark to the revision")),
		mark:        key.NewBinding(key.WithKeys("space"), key.WithHelp("space", "select the revision")),
		squash:      key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "squash the selected revisions (y confirms)")),
		cancel:      key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
	}
}

type Env struct {
	Theme      theme.Theme
	Width      int
	Height     int
	BodyHeight int
	ListHeight int
	ModalWidth int
	Head       string
	Active     bool
	Dropped    int
}

func (e Env) modalInner() int {
	return ui.InputBoxInner(e.ModalWidth)
}

type Intent struct {
	Open     string
	Arm      bool
	Abandon  *jj.LogEntry
	ArmPush  bool
	Bookmark *jj.LogEntry
	Push     *jj.LogEntry
	Fetch    bool
	Squash   bool
	Squashed *Squash
	Status   string
	Refresh  bool
	Retarget bool
}

type action int

const (
	abandonAction action = iota
	pushAction
)

type confirmation struct {
	action action
	entry  jj.LogEntry
}

type Squash struct {
	From []jj.LogEntry
	Into jj.LogEntry
}

type Model struct {
	ctx       context.Context
	src       Source
	keys      keyMap
	lines     []jj.LogLine
	shown     []jj.LogLine
	query     textinput.Model
	searching bool
	expanded  map[string]bool
	files     map[string]fileList
	loaded    bool
	loading   bool
	stale     bool
	err       error
	cursor    int
	followed  string
	keep      Position
	confirm   *confirmation
	marked    map[string]bool
	squashing []jj.LogEntry
	top       int
	seq       int
}

type Position struct {
	at     int
	cursor int
	query  string
	change string
}

type fileList struct {
	files  []diff.File
	err    error
	loaded bool
}

type filesMsg struct {
	commitID string
	files    []diff.File
	err      error
}

type row struct {
	line int
	file int
}

const (
	entryRow   = -1
	messageRow = -2
)

const minDescription = 20

const defaultWorkspace = "default"

func New(ctx context.Context, src Source) Model {
	return Model{ctx: ctx, src: src, keys: newKeyMap(), query: newSearchInput(), expanded: map[string]bool{}, files: map[string]fileList{}, marked: map[string]bool{}}
}

type loadedMsg struct {
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

func (l *Model) Resize(width int) {
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

func (l Model) filtered() bool {
	return l.searching || l.query.Value() != ""
}

func (l Model) OverlayOpen() bool {
	return l.confirm != nil
}

func (l Model) Capturing() bool {
	return l.searching || l.confirm != nil || l.squashing != nil
}

func (l *Model) refilter() {
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

func (l Model) selected() *jj.LogEntry {
	if l.cursor < 0 || l.cursor >= len(l.shown) {
		return nil
	}
	return l.shown[l.cursor].Entry
}

func (l Model) SelectedChange() string {
	if e := l.selected(); e != nil {
		return e.ChangeID
	}
	return ""
}

func (l Model) AbandonKey() string {
	return l.keys.abandon.Help().Key
}

func (l Model) PushKey() string {
	return l.keys.push.Help().Key
}

func (l Model) selectedCommit() string {
	if e := l.selected(); e != nil {
		return e.CommitID
	}
	return ""
}

func (l *Model) focusWhere(pred func(*jj.LogEntry) bool) bool {
	i := slices.IndexFunc(l.shown, func(line jj.LogLine) bool { return line.Entry != nil && pred(line.Entry) })
	if i < 0 {
		return false
	}
	l.cursor = i
	return true
}

func (l *Model) focus(commitID string) {
	switch {
	case l.focusWhere(func(e *jj.LogEntry) bool { return e.CommitID == commitID }):
	case l.focusWhere(func(e *jj.LogEntry) bool { return e.WorkingCopy }):
	case l.focusWhere(func(*jj.LogEntry) bool { return true }):
	default:
		l.cursor = 0
	}
	l.followed = l.selectedCommit()
}

func (l *Model) focusNth(n int) {
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

func (l *Model) Follow(commitID string) {
	if l.loaded && l.selectedCommit() == l.followed {
		l.focus(commitID)
	}
}

func (l *Model) move(delta int) {
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

func (l *Model) jump(delta int) {
	l.move(delta)
	l.top = min(l.top, l.cursorRow(l.rows()))
}

func (l Model) position() (int, int) {
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

func (l *Model) HoldPosition() Position {
	at, _ := l.position()
	l.followed = ""
	return Position{at: at, cursor: l.cursor, query: l.query.Value()}
}

func (p Position) Targeting(changeID string) Position {
	p.change = changeID
	return p
}

func (l *Model) RestorePosition(keep Position) {
	l.keep = keep
}

func (l *Model) Show(head string) tea.Cmd {
	if !l.loading && (!l.loaded || l.stale) {
		return l.Load()
	}
	l.focus(head)
	return nil
}

func (l *Model) Load() tea.Cmd {
	l.seq++
	l.loading = true
	l.stale = false
	seq, ctx, src := l.seq, l.ctx, l.src
	return func() tea.Msg {
		lines, err := src.Log(ctx)
		if err != nil {
			err = fmt.Errorf("load log: %w", err)
		}
		return loadedMsg{seq: seq, lines: lines, err: err}
	}
}

func (l *Model) MarkStale() {
	l.stale = true
}

func (l *Model) Update(msg tea.Msg, env Env) (tea.Cmd, Intent, bool) {
	switch msg := msg.(type) {
	case loadedMsg:
		cmd, in := l.applyList(msg, env)
		return cmd, in, true
	case filesMsg:
		l.files[msg.commitID] = fileList{files: msg.files, err: msg.err, loaded: true}
		return nil, Intent{}, true
	}
	return nil, Intent{}, false
}

func (l *Model) applyList(msg loadedMsg, env Env) (tea.Cmd, Intent) {
	if msg.seq != l.seq {
		return nil, Intent{}
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
	l.prune()
	keep := l.keep
	restore := keep.at > 0 && msg.err == nil && l.cursor == keep.cursor && l.query.Value() == keep.query
	l.keep = Position{}
	l.refilter()
	switch {
	case restore && keep.change != "" && l.focusWhere(func(e *jj.LogEntry) bool { return e.ChangeID == keep.change }):
		l.followed = ""
	case restore:
		l.focusNth(keep.at)
	case keep.at > 0 && before != "" && l.selectedCommit() == before:
	default:
		l.focus(env.Head)
	}
	in := Intent{Retarget: env.Active && l.selectedCommit() != before}
	if l.stale && env.Active {
		return l.Load(), in
	}
	return nil, in
}

func (l Model) Ignores(msg tea.KeyPressMsg, reloading bool) bool {
	if l.squashing != nil {
		return key.Matches(msg, l.keys.confirm) && (msg.IsRepeat || reloading || l.loading)
	}
	return l.confirm == nil && !l.searching && key.Matches(msg, l.keys.abandon, l.keys.confirm, l.keys.squash, l.keys.fetch, l.keys.push) && (msg.IsRepeat || reloading || l.loading)
}

func (l *Model) Key(msg tea.KeyPressMsg, env Env) (tea.Cmd, Intent) {
	if l.confirm != nil {
		return nil, l.confirmKey(msg)
	}
	if l.searching {
		return l.searchKey(msg), Intent{}
	}
	if l.squashing != nil {
		return nil, l.squashKey(msg, env)
	}
	if l.navigate(msg, env) {
		return nil, Intent{}
	}
	return l.browseKey(msg)
}

func (l *Model) confirmKey(msg tea.KeyPressMsg) Intent {
	c := l.confirm
	l.confirm = nil
	if e := l.selected(); msg.IsRepeat || !key.Matches(msg, l.keys.confirm) || e == nil || e.CommitID != c.entry.CommitID {
		return Intent{}
	}
	if c.action == pushAction {
		return Intent{Push: &c.entry}
	}
	return Intent{Abandon: &c.entry}
}

func (l *Model) browseKey(msg tea.KeyPressMsg) (tea.Cmd, Intent) {
	switch {
	case key.Matches(msg, l.keys.search):
		if !l.loaded || l.err != nil {
			return nil, Intent{}
		}
		l.searching = true
		return l.query.Focus(), Intent{}
	case key.Matches(msg, l.keys.clearSearch) && l.filtered():
		l.clearSearch()
	case key.Matches(msg, l.keys.cancel):
		clear(l.marked)
	case key.Matches(msg, l.keys.mark):
		if e := l.selected(); e != nil {
			l.toggleMark(e.CommitID)
		}
	case key.Matches(msg, l.keys.squash):
		return nil, Intent{Squash: l.selected() != nil || len(l.marked) > 0}
	case key.Matches(msg, l.keys.open):
		if e := l.selected(); e != nil {
			return nil, Intent{Open: e.CommitID}
		}
	case key.Matches(msg, l.keys.expand):
		return l.expandFiles(), Intent{}
	case key.Matches(msg, l.keys.abandon):
		return nil, Intent{Arm: l.selected() != nil}
	case key.Matches(msg, l.keys.push):
		return nil, Intent{ArmPush: l.selected() != nil}
	case key.Matches(msg, l.keys.fetch):
		return nil, Intent{Fetch: true}
	case key.Matches(msg, l.keys.bookmark):
		if e := l.selected(); e != nil {
			return nil, Intent{Bookmark: new(*e)}
		}
	case key.Matches(msg, l.keys.reset):
		clear(l.marked)
		clear(l.expanded)
	case key.Matches(msg, l.keys.collapse):
		if e := l.selected(); e != nil {
			delete(l.expanded, e.CommitID)
		}
	case key.Matches(msg, l.keys.reload):
		return nil, Intent{Refresh: !l.loading}
	}
	return nil, Intent{}
}

func (l *Model) navigate(msg tea.KeyPressMsg, env Env) bool {
	half := max(env.ListHeight/2, 1)
	switch {
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
	default:
		return false
	}
	return true
}

func (l *Model) toggleMark(commitID string) {
	if l.marked[commitID] {
		delete(l.marked, commitID)
		return
	}
	l.marked[commitID] = true
}

func (l Model) marks() []jj.LogEntry {
	var out []jj.LogEntry
	for _, line := range l.lines {
		if line.Entry != nil && l.marked[line.Entry.CommitID] {
			out = append(out, *line.Entry)
		}
	}
	return out
}

func (l *Model) prune() {
	present := map[string]bool{}
	for _, line := range l.lines {
		if line.Entry != nil {
			present[line.Entry.CommitID] = true
		}
	}
	gone := func(id string, _ bool) bool { return !present[id] }
	maps.DeleteFunc(l.marked, gone)
	maps.DeleteFunc(l.expanded, gone)
	maps.DeleteFunc(l.files, func(id string, _ fileList) bool { return !present[id] })
	if slices.ContainsFunc(l.squashing, func(e jj.LogEntry) bool { return !present[e.CommitID] }) {
		l.squashing = nil
	}
}

func (l Model) squashed(commitID string) bool {
	return slices.ContainsFunc(l.squashing, func(e jj.LogEntry) bool { return e.CommitID == commitID })
}

func (l *Model) StartSquash() string {
	from := l.marks()
	if len(from) == 0 {
		if e := l.selected(); e != nil {
			from = []jj.LogEntry{*e}
		}
	}
	if len(from) == 0 {
		return ""
	}
	if i := slices.IndexFunc(from, func(e jj.LogEntry) bool { return e.Immutable }); i >= 0 {
		return ui.ShortID(from[i].ChangeID) + " is immutable"
	}
	l.squashing = from
	l.clearSearch()
	parents := map[string]bool{}
	for _, e := range from {
		for _, p := range e.Parents {
			parents[p] = !l.squashed(p)
		}
	}
	l.focusWhere(func(e *jj.LogEntry) bool { return parents[e.CommitID] })
	return ""
}

func (l *Model) FinishSquash() {
	l.squashing = nil
	clear(l.marked)
}

func (l *Model) squashKey(msg tea.KeyPressMsg, env Env) Intent {
	switch {
	case l.navigate(msg, env):
	case key.Matches(msg, l.keys.cancel):
		l.squashing = nil
	case key.Matches(msg, l.keys.confirm):
		e := l.selected()
		switch {
		case e == nil:
		case l.squashed(e.CommitID):
			return Intent{Status: "pick a revision that is not being squashed"}
		case e.Immutable:
			return Intent{Status: ui.ShortID(e.ChangeID) + " is immutable"}
		default:
			return Intent{Squashed: &Squash{From: slices.Clone(l.squashing), Into: *e}}
		}
	}
	return Intent{}
}

func (l *Model) Arm() string {
	if e := l.selected(); e != nil && e.Immutable {
		return ui.ShortID(e.ChangeID) + " is immutable"
	}
	l.arm(abandonAction)
	return ""
}

func (l *Model) ArmPush() {
	l.arm(pushAction)
}

func (l *Model) arm(a action) {
	e := l.selected()
	if e == nil {
		return
	}
	target := *e
	target.Bookmarks = slices.Clone(e.Bookmarks)
	l.confirm = &confirmation{action: a, entry: target}
}

func (l *Model) expandFiles() tea.Cmd {
	e := l.selected()
	if e == nil {
		return nil
	}
	id := e.CommitID
	l.expanded[id] = true
	if f, ok := l.files[id]; ok && (!f.loaded || f.err == nil) {
		return nil
	}
	l.files[id] = fileList{}
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
		return filesMsg{commitID: id, files: paths, err: err}
	}
}

func (l Model) rows() []row {
	var rows []row
	for i, line := range l.shown {
		rows = append(rows, row{line: i, file: entryRow})
		if line.Entry == nil || !l.expanded[line.Entry.CommitID] {
			continue
		}
		f := l.files[line.Entry.CommitID]
		if !f.loaded || f.err != nil || len(f.files) == 0 {
			rows = append(rows, row{line: i, file: messageRow})
			continue
		}
		for j := range f.files {
			rows = append(rows, row{line: i, file: j})
		}
	}
	return rows
}

func (l Model) cursorRow(rows []row) int {
	return max(slices.Index(rows, row{line: l.cursor, file: entryRow}), 0)
}

func (l Model) height(env Env) int {
	if l.filtered() {
		return max(env.ListHeight-1, 1)
	}
	return env.ListHeight
}

func (l *Model) KeepVisible(env Env) {
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

func (l *Model) scrollPage(delta, height int) {
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

func (l *Model) searchKey(msg tea.KeyPressMsg) tea.Cmd {
	switch {
	case key.Matches(msg, l.keys.clearSearch):
		l.clearSearch()
	case key.Matches(msg, l.keys.open):
		l.searching = false
		l.query.Blur()
	default:
		return l.Input(msg)
	}
	return nil
}

func (l *Model) Input(msg tea.Msg) tea.Cmd {
	if !l.searching {
		return nil
	}
	var cmd tea.Cmd
	l.query, cmd = l.query.Update(msg)
	l.refilter()
	return cmd
}

func (l *Model) clearSearch() {
	l.searching = false
	l.query.Blur()
	l.query.Reset()
	l.refilter()
}

func (l Model) Hints(tabs, help ui.KeyHint) []ui.KeyHint {
	if l.searching {
		return []ui.KeyHint{ui.Hint("apply", l.keys.open), ui.Hint("clear", l.keys.clearSearch)}
	}
	if l.squashing != nil {
		return []ui.KeyHint{ui.Hint("into", l.keys.down, l.keys.up), ui.Hint("squash", l.keys.confirm), ui.Hint("cancel", l.keys.cancel)}
	}
	hints := []ui.KeyHint{
		tabs,
		ui.Hint("revision", l.keys.down, l.keys.up),
		ui.Hint("review", l.keys.open),
		ui.Hint("select", l.keys.mark),
	}
	if len(l.marked) > 0 {
		hints = append(hints, ui.Hint("squash", l.keys.squash))
	}
	hints = append(hints,
		ui.Hint("files", l.keys.expand, l.keys.collapse),
		ui.Hint("search", l.keys.search),
		ui.Hint("fetch", l.keys.fetch),
		ui.Hint("push", l.keys.push),
		ui.Hint("bookmark", l.keys.bookmark),
	)
	if l.filtered() {
		hints = append(hints, ui.Hint("clear", l.keys.clearSearch))
	}
	if len(l.marked) > 0 || len(l.expanded) > 0 {
		hints = append(hints, ui.Hint("reset", l.keys.reset))
	}
	return append(hints, help)
}

func (l Model) Help() []ui.KeyHint {
	return []ui.KeyHint{
		ui.Hint("next / previous revision", l.keys.down, l.keys.up),
		ui.Hint("half page down / up", l.keys.halfDown, l.keys.halfUp),
		ui.Hint("first / last revision", l.keys.top, l.keys.bottom),
		ui.Describe(l.keys.open),
		ui.Describe(l.keys.expand),
		ui.Describe(l.keys.collapse),
		ui.Describe(l.keys.mark),
		ui.Describe(l.keys.squash),
		ui.Describe(l.keys.abandon),
		ui.Describe(l.keys.fetch),
		ui.Describe(l.keys.push),
		ui.Describe(l.keys.bookmark),
		ui.Describe(l.keys.search),
		ui.Describe(l.keys.clearSearch),
		ui.Describe(l.keys.reset),
		ui.Describe(l.keys.reload),
	}
}

func (l Model) View(env Env) string {
	p := env.Theme
	inner := max(env.Width-2, 0)
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
		if n := len(l.marked); n > 0 {
			value = append(value, ui.Span(ui.Fg(p.Picked), fmt.Sprintf(" · %d selected", n)))
		}
		rows := l.rows()
		for i := l.top; i < len(rows) && len(body) < env.ListHeight; i++ {
			if rows[i].file == entryRow {
				body = append(body, l.line(env, rows[i].line, inner))
			} else {
				body = append(body, l.fileLine(p, rows[i], inner))
			}
		}
	}
	return ui.Card(p, env.Width, env.BodyHeight, "Log", true, value, body)
}

func (l Model) line(env Env, i, width int) string {
	p := env.Theme
	line := l.shown[i]
	bg := p.Card
	if i == l.cursor {
		bg = p.CursorBg
	}
	e := line.Entry
	bar := bg
	if e != nil && e.CommitID == env.Head {
		bar = p.Selection
	}
	left := []ui.Seg{ui.Span(ui.Fg(bar), ui.BarGlyph(bar)), ui.Span(ui.Fg(p.Muted), line.Graph)}
	if e == nil {
		return ui.Paint(width, bg, left, nil)
	}
	switch {
	case l.squashing == nil:
		if l.marked[e.CommitID] {
			left = append(left, ui.Span(ui.Fg(p.Picked).Bold(true), "● "))
		}
	case l.squashed(e.CommitID):
		left = append(left, ui.Pill(p.Chip, p.Removed, "squash"), ui.Span(lipgloss.NewStyle(), " "))
	case i == l.cursor:
		left = append(left, ui.Pill(p.Chip, p.Added, "into"), ui.Span(lipgloss.NewStyle(), " "))
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
	if ui.RowWidth(left, right)+min(lipgloss.Width(desc.Text), minDescription) > width {
		right = nil
	}
	return ui.Paint(width, bg, append(left, desc), right)
}

func (l Model) fileLine(p theme.Theme, r row, width int) string {
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

func (l Model) Overlay(env Env) string {
	p := env.Theme
	if l.confirm == nil {
		return ""
	}
	c := l.confirm
	e := c.entry
	inner := env.modalInner()
	wrap := func(style lipgloss.Style, text string) []string {
		var out []string
		for line := range strings.SplitSeq(ansi.Wrap(text, inner, ""), "\n") {
			out = append(out, ui.Paint(inner, p.Card, []ui.Seg{ui.Span(style, line)}, nil))
		}
		return out
	}
	verb := "abandon"
	if c.action == pushAction {
		verb = "push"
	}
	lines := wrap(ui.Fg(p.Title).Bold(true), strings.ToUpper(verb[:1])+verb[1:]+" "+ui.ShortID(e.ChangeID)+" "+ui.ShortID(e.CommitID))
	if title, _, _ := strings.Cut(e.Description, "\n"); title != "" {
		lines = append(lines, wrap(ui.Fg(p.Strong), title)...)
	}
	n := len(e.Bookmarks)
	switch {
	case c.action == pushAction && n > 0:
		lines = append(lines, wrap(ui.Fg(p.Added), "Pushes "+ui.Plural(n, "bookmark")+" "+strings.Join(e.Bookmarks, ", "))...)
	case c.action == pushAction:
		lines = append(lines, wrap(ui.Fg(p.Added), "Creates a bookmark for the change and pushes it")...)
	case n > 0:
		lines = append(lines, wrap(ui.Fg(p.Removed), "Deletes "+ui.Plural(n, "bookmark")+" "+strings.Join(e.Bookmarks, ", "))...)
	}
	if d := env.Dropped; d > 0 && c.action == abandonAction {
		lines = append(lines, wrap(ui.Fg(p.Removed), fmt.Sprintf("Drops %d selected %s", d, ui.Plural(d, "hunk")))...)
	}
	footer := wrap(ui.Fg(p.Muted), l.keys.confirm.Help().Key+" "+verb+" · any other key cancels")
	if room := max(env.Height-2-len(footer), 0); len(lines) > room {
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
