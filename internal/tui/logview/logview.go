package logview

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/madalinpopa/jjreview/internal/diff"
	"github.com/madalinpopa/jjreview/internal/jj"
	"github.com/madalinpopa/jjreview/internal/theme"
	"github.com/madalinpopa/jjreview/internal/tui/tab"
	"github.com/madalinpopa/jjreview/internal/tui/ui"
)

type Source interface {
	Log(ctx context.Context) ([]jj.LogLine, error)
	Diff(ctx context.Context, rev string) ([]diff.File, error)
	Abandon(ctx context.Context, commitID string, bookmarks []string) (workingCopy bool, err error)
	Squash(ctx context.Context, from []jj.LogEntry, into jj.LogEntry) (string, error)
	Push(ctx context.Context, e jj.LogEntry) ([]jj.PushResult, error)
	Fetch(ctx context.Context) (bool, error)
}

type keyMap struct {
	ui.NavKeys
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
		NavKeys:     ui.NewNavKeys("next revision", "previous revision", "first revision", "last revision"),
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

type intent int

const (
	noIntent intent = iota
	moveDownIntent
	moveUpIntent
	halfDownIntent
	halfUpIntent
	topIntent
	bottomIntent
	openIntent
	searchIntent
	dismissIntent
	expandIntent
	collapseIntent
	abandonIntent
	confirmIntent
	reloadIntent
	resetIntent
	fetchIntent
	pushIntent
	bookmarkIntent
	markIntent
	squashIntent
)

func (k keyMap) resolve(msg tea.KeyPressMsg) intent {
	switch {
	case key.Matches(msg, k.Down):
		return moveDownIntent
	case key.Matches(msg, k.Up):
		return moveUpIntent
	case key.Matches(msg, k.HalfDown):
		return halfDownIntent
	case key.Matches(msg, k.HalfUp):
		return halfUpIntent
	case key.Matches(msg, k.Top):
		return topIntent
	case key.Matches(msg, k.Bottom):
		return bottomIntent
	case key.Matches(msg, k.open):
		return openIntent
	case key.Matches(msg, k.search):
		return searchIntent
	case key.Matches(msg, k.clearSearch, k.cancel):
		return dismissIntent
	case key.Matches(msg, k.expand):
		return expandIntent
	case key.Matches(msg, k.collapse):
		return collapseIntent
	case key.Matches(msg, k.abandon):
		return abandonIntent
	case key.Matches(msg, k.confirm):
		return confirmIntent
	case key.Matches(msg, k.reload):
		return reloadIntent
	case key.Matches(msg, k.reset):
		return resetIntent
	case key.Matches(msg, k.fetch):
		return fetchIntent
	case key.Matches(msg, k.push):
		return pushIntent
	case key.Matches(msg, k.bookmark):
		return bookmarkIntent
	case key.Matches(msg, k.mark):
		return markIntent
	case key.Matches(msg, k.squash):
		return squashIntent
	}
	return noIntent
}

func blocked(e tab.Env) (tab.Effect, bool) {
	switch {
	case e.Lock.Busy:
		return tab.Effect{}, true
	case e.Lock.Reason != "":
		return tab.Effect{Status: e.Lock.Reason}, true
	}
	return tab.Effect{}, false
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
	list      ui.Loader
	cursor    int
	followed  string
	keep      Position
	confirm   *confirmation
	marked    map[string]bool
	squashing []jj.LogEntry
	dropArmed bool
	top       int
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
	return Model{ctx: ctx, src: src, keys: newKeyMap(), query: ui.NewSearchInput(), expanded: map[string]bool{}, files: map[string]fileList{}, marked: map[string]bool{}}
}

type loadedMsg struct {
	seq   int
	lines []jj.LogLine
	err   error
}

func (l *Model) Resize(width int) {
	q := &l.query
	value, pos := q.Value(), q.Position()
	q.SetWidth(max(width-6, 1))
	q.SetValue("")
	q.SetValue(value)
	q.SetCursor(pos)
}

func (l Model) filtered() bool {
	return l.searching || l.query.Value() != ""
}

func (l Model) OverlayOpen() bool {
	return l.confirm != nil
}

func (l Model) Captures() bool {
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
	if l.list.Loaded() && l.selectedCommit() == l.followed {
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

func (l *Model) Show(env tab.Env) tea.Cmd {
	if l.list.Due() {
		return l.Load()
	}
	l.focus(env.Head.CommitID)
	return nil
}

func (l *Model) Load() tea.Cmd {
	seq, ctx, src := l.list.Begin(), l.ctx, l.src
	return func() tea.Msg {
		lines, err := src.Log(ctx)
		if err != nil {
			err = fmt.Errorf("load log: %w", err)
		}
		return loadedMsg{seq: seq, lines: lines, err: err}
	}
}

func (l *Model) MarkStale() {
	l.list.MarkStale()
}

func (l *Model) Update(msg tea.Msg, env tab.Env) (tea.Cmd, tab.Effect, bool) {
	switch msg := msg.(type) {
	case loadedMsg:
		cmd, eff := l.applyList(msg, env)
		return cmd, eff, true
	case filesMsg:
		l.files[msg.commitID] = fileList{files: msg.files, err: msg.err, loaded: true}
		return nil, tab.Effect{}, true
	}
	if env.Active && l.searching {
		return l.input(msg), tab.Effect{}, true
	}
	return nil, tab.Effect{}, false
}

func (l *Model) applyList(msg loadedMsg, env tab.Env) (tea.Cmd, tab.Effect) {
	before := l.selectedCommit()
	if !l.list.Finish(msg.seq, msg.err) {
		return nil, tab.Effect{}
	}
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
		l.focus(env.Head.CommitID)
	}
	eff := tab.Effect{Retarget: env.Active && l.selectedCommit() != before}
	if l.list.Stale() && env.Active {
		return l.Load(), eff
	}
	return nil, eff
}

func (l Model) Ignores(msg tea.KeyPressMsg, env tab.Env) bool {
	busy := msg.IsRepeat || env.Lock.Busy || l.list.Loading()
	in := l.keys.resolve(msg)
	if l.squashing != nil {
		return in == confirmIntent && busy
	}
	guarded := []intent{abandonIntent, confirmIntent, squashIntent, fetchIntent, pushIntent}
	return l.confirm == nil && !l.searching && slices.Contains(guarded, in) && busy
}

func (l *Model) Key(msg tea.KeyPressMsg, env tab.Env) (tea.Cmd, tab.Effect) {
	in := l.keys.resolve(msg)
	if l.confirm != nil {
		return nil, l.confirmKey(in, msg.IsRepeat, env)
	}
	if l.searching {
		return l.searchKey(in, msg), tab.Effect{}
	}
	if l.squashing != nil {
		return nil, l.squashKey(in, env)
	}
	if l.navigate(in, env) {
		return nil, tab.Effect{}
	}
	return l.browseKey(in, env)
}

func (l *Model) confirmKey(in intent, repeat bool, env tab.Env) tab.Effect {
	c := l.confirm
	l.confirm = nil
	if e := l.selected(); repeat || in != confirmIntent || e == nil || e.CommitID != c.entry.CommitID {
		return tab.Effect{}
	}
	if fx, held := blocked(env); held {
		return fx
	}
	if c.action == pushAction {
		return l.push(c.entry)
	}
	return l.abandon(c.entry)
}

func (l *Model) browseKey(in intent, env tab.Env) (tea.Cmd, tab.Effect) {
	switch in {
	case searchIntent:
		if !l.list.Loaded() || l.list.Err() != nil {
			return nil, tab.Effect{}
		}
		l.searching = true
		return l.query.Focus(), tab.Effect{}
	case dismissIntent:
		if l.filtered() {
			l.clearSearch()
		} else {
			clear(l.marked)
		}
	case markIntent:
		if e := l.selected(); e != nil {
			l.toggleMark(e.CommitID)
		}
	case squashIntent:
		return nil, l.startSquash(env)
	case openIntent:
		if e := l.selected(); e != nil {
			return nil, tab.Effect{Open: e.CommitID, KeepArmed: true}
		}
	case expandIntent:
		return l.expandFiles(), tab.Effect{}
	case abandonIntent:
		return nil, l.armAbandon(env)
	case pushIntent:
		return nil, l.armPush(env)
	case fetchIntent:
		return nil, l.fetch(env)
	case bookmarkIntent:
		if e := l.selected(); e != nil {
			return nil, tab.Effect{Pick: new(*e)}
		}
	case resetIntent:
		clear(l.marked)
		clear(l.expanded)
	case collapseIntent:
		if e := l.selected(); e != nil {
			delete(l.expanded, e.CommitID)
		}
	case reloadIntent:
		return nil, tab.Effect{Refresh: !l.list.Loading()}
	}
	return nil, tab.Effect{}
}

func (l *Model) navigate(in intent, env tab.Env) bool {
	half := max(env.ListHeight/2, 1)
	switch in {
	case moveDownIntent:
		l.move(1)
	case moveUpIntent:
		l.move(-1)
	case halfDownIntent:
		l.scrollPage(half, l.height(env))
	case halfUpIntent:
		l.scrollPage(-half, l.height(env))
	case topIntent:
		l.jump(-len(l.shown))
	case bottomIntent:
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

func (l *Model) startSquash(env tab.Env) tab.Effect {
	from := l.marks()
	if len(from) == 0 {
		if e := l.selected(); e != nil {
			from = []jj.LogEntry{*e}
		}
	}
	if len(from) == 0 {
		return tab.Effect{}
	}
	if fx, held := blocked(env); held {
		return fx
	}
	if i := slices.IndexFunc(from, func(e jj.LogEntry) bool { return e.Immutable }); i >= 0 {
		return tab.Effect{Status: ui.ShortID(from[i].ChangeID) + " is immutable"}
	}
	l.squashing = from
	l.dropArmed = false
	l.clearSearch()
	parents := map[string]bool{}
	for _, e := range from {
		for _, p := range e.Parents {
			parents[p] = !l.squashed(p)
		}
	}
	l.focusWhere(func(e *jj.LogEntry) bool { return parents[e.CommitID] })
	return tab.Effect{}
}

func (l *Model) squashKey(in intent, env tab.Env) tab.Effect {
	armed := l.dropArmed
	l.dropArmed = false
	switch {
	case l.navigate(in, env):
	case in == dismissIntent:
		l.squashing = nil
	case in == confirmIntent:
		e := l.selected()
		switch {
		case e == nil:
		case l.squashed(e.CommitID):
			return tab.Effect{Status: "pick a revision that is not being squashed"}
		case e.Immutable:
			return tab.Effect{Status: ui.ShortID(e.ChangeID) + " is immutable"}
		default:
			if fx, held := blocked(env); held {
				return fx
			}
			if env.Dropped > 0 && !armed {
				l.dropArmed = true
				return tab.Effect{Status: tab.DropPrompt(l.keys.confirm.Help().Key, env.Dropped)}
			}
			from := l.squashing
			l.squashing = nil
			clear(l.marked)
			return l.squash(from, *e)
		}
	}
	return tab.Effect{}
}

func (l *Model) armAbandon(env tab.Env) tab.Effect {
	e := l.selected()
	if e == nil {
		return tab.Effect{}
	}
	if fx, held := blocked(env); held {
		return fx
	}
	if e.Immutable {
		return tab.Effect{Status: ui.ShortID(e.ChangeID) + " is immutable"}
	}
	l.arm(abandonAction)
	return tab.Effect{}
}

func (l *Model) armPush(env tab.Env) tab.Effect {
	if l.selected() == nil {
		return tab.Effect{}
	}
	if fx, held := blocked(env); held {
		return fx
	}
	l.arm(pushAction)
	return tab.Effect{}
}

func (l Model) abandon(e jj.LogEntry) tab.Effect {
	id := ui.ShortID(e.ChangeID)
	retry := "bookmarks changed; press " + l.keys.abandon.Help().Key + " to review the abandon again"
	src := l.src
	return tab.Effect{Rewrite: new(tab.Rewrite{Run: func(ctx context.Context, head tab.Head) (tab.Result, error) {
		workingCopy, err := src.Abandon(ctx, e.CommitID, e.Bookmarks)
		switch {
		case errors.Is(err, jj.ErrBookmarksChanged):
			return tab.Result{}, errors.New(retry)
		case err != nil:
			return tab.Result{}, fmt.Errorf("abandon %s: %w", id, err)
		}
		return tab.Result{Done: "abandoned " + id, Replaced: workingCopy || e.CommitID == head.CommitID}, nil
	}})}
}

func (l Model) squash(from []jj.LogEntry, into jj.LogEntry) tab.Effect {
	done := "squashed " + ui.ShortID(from[0].ChangeID)
	if n := len(from); n > 1 {
		done = fmt.Sprintf("squashed %d revisions", n)
	}
	target := ui.ShortID(into.ChangeID)
	src := l.src
	return tab.Effect{Rewrite: new(tab.Rewrite{Focus: into.ChangeID, Run: func(ctx context.Context, head tab.Head) (tab.Result, error) {
		warnings, err := src.Squash(ctx, from, into)
		if err != nil {
			return tab.Result{}, fmt.Errorf("squash into %s: %w", target, err)
		}
		replaced := slices.ContainsFunc(from, func(e jj.LogEntry) bool { return e.CommitID == head.CommitID })
		return tab.Result{Done: done + " into " + target, Replaced: replaced, Warnings: warnings}, nil
	}})}
}

func (l Model) push(e jj.LogEntry) tab.Effect {
	id := ui.ShortID(e.ChangeID)
	retry := "bookmarks changed; press " + l.keys.push.Help().Key + " to review the push again"
	src := l.src
	return tab.Effect{Rewrite: new(tab.Rewrite{Pending: "pushing " + id + "…", Focus: e.ChangeID, Run: func(ctx context.Context, _ tab.Head) (tab.Result, error) {
		r, err := src.Push(ctx, e)
		res := tab.Result{KeepPicks: true}
		switch {
		case errors.Is(err, jj.ErrBookmarksChanged):
			return res, errors.New(retry)
		case err != nil:
			return res, fmt.Errorf("push %s: %w", id, err)
		case len(r) == 0:
			res.Done = "nothing to push"
		default:
			res.Done = "pushed " + pushSummary(r)
		}
		return res, nil
	}})}
}

func pushSummary(results []jj.PushResult) string {
	parts := make([]string, len(results))
	for i, r := range results {
		parts[i] = strings.Join(r.Bookmarks, ", ") + " to " + r.Remote
	}
	return strings.Join(parts, " and ")
}

func (l Model) fetch(env tab.Env) tab.Effect {
	if fx, held := blocked(env); held {
		return fx
	}
	src := l.src
	return tab.Effect{Rewrite: new(tab.Rewrite{Pending: "fetching…", Run: func(ctx context.Context, _ tab.Head) (tab.Result, error) {
		changed, err := src.Fetch(ctx)
		res := tab.Result{KeepPicks: true, Done: "fetched"}
		switch {
		case err != nil:
			return res, fmt.Errorf("fetch: %w", err)
		case !changed:
			res.Done = "already up to date"
		}
		return res, nil
	}})}
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

func (l Model) height(env tab.Env) int {
	if l.filtered() {
		return max(env.ListHeight-1, 1)
	}
	return env.ListHeight
}

func (l *Model) KeepVisible(env tab.Env) {
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

func (l *Model) searchKey(in intent, msg tea.KeyPressMsg) tea.Cmd {
	switch in {
	case dismissIntent:
		l.clearSearch()
	case openIntent:
		l.searching = false
		l.query.Blur()
	default:
		return l.input(msg)
	}
	return nil
}

func (l *Model) Paste(msg tea.PasteMsg) tea.Cmd {
	return l.input(msg)
}

func (l *Model) input(msg tea.Msg) tea.Cmd {
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
		return []ui.KeyHint{ui.Hint("into", l.keys.Down, l.keys.Up), ui.Hint("squash", l.keys.confirm), ui.Hint("cancel", l.keys.cancel)}
	}
	hints := []ui.KeyHint{
		tabs,
		ui.Hint("revision", l.keys.Down, l.keys.Up),
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
		ui.Hint("next / previous revision", l.keys.Down, l.keys.Up),
		ui.Hint("half page down / up", l.keys.HalfDown, l.keys.HalfUp),
		ui.Hint("first / last revision", l.keys.Top, l.keys.Bottom),
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

func (l Model) View(env tab.Env) string {
	p := env.Theme
	inner := max(env.Width-2, 0)
	var body []string
	var value []ui.Seg
	switch {
	case l.list.Err() != nil:
		body = []string{ui.Paint(inner, p.Card, []ui.Seg{ui.Span(ui.Fg(p.Removed), "  error: "+l.list.Err().Error())}, nil)}
	case !l.list.Loaded():
		body = []string{ui.Message(p, "loading…", inner)}
	default:
		if l.filtered() {
			q := l.query
			q.SetStyles(ui.SearchStyles(p))
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

func (l Model) line(env tab.Env, i, width int) string {
	p := env.Theme
	line := l.shown[i]
	bg := p.Card
	if i == l.cursor {
		bg = p.CursorBg
	}
	e := line.Entry
	bar := bg
	if e != nil && e.CommitID == env.Head.CommitID {
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

func (l Model) Overlay(env tab.Env) string {
	p := env.Theme
	if l.confirm == nil {
		return ""
	}
	c := l.confirm
	e := c.entry
	inner := ui.InputBoxInner(env.ModalWidth)
	wrap := func(style lipgloss.Style, text string) []string {
		return ui.WrapLines(p, inner, style, text)
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
	lines = ui.TruncateLines(lines, env.Height-2-len(footer), ui.Ellipsis(p, inner))
	return ui.ModalBox(p, append(lines, footer...))
}
