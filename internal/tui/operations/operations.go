package operations

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/madalinpopa/jjreview/internal/jj"
	"github.com/madalinpopa/jjreview/internal/theme"
	"github.com/madalinpopa/jjreview/internal/tui/tab"
	"github.com/madalinpopa/jjreview/internal/tui/ui"
)

type Source interface {
	Operations(ctx context.Context) ([]jj.Operation, error)
	RestoreOperation(ctx context.Context, id string) error
	RevertOperation(ctx context.Context, id string) error
	AbandonOperation(ctx context.Context, id string) error
}

type keyMap struct {
	ui.NavKeys
	restore  key.Binding
	undo     key.Binding
	abandon  key.Binding
	expand   key.Binding
	collapse key.Binding
	confirm  key.Binding
	reload   key.Binding
}

type intent int

const (
	noIntent intent = iota
	moveDownIntent
	moveUpIntent
	halfPageDownIntent
	halfPageUpIntent
	topIntent
	bottomIntent
	expandIntent
	collapseIntent
	restoreIntent
	undoIntent
	abandonIntent
	reloadIntent
	confirmIntent
)

func (i intent) verb() string {
	switch i {
	case restoreIntent:
		return "restore"
	case undoIntent:
		return "undo"
	}
	return "abandon"
}

func (i intent) action() bool {
	return i == restoreIntent || i == undoIntent || i == abandonIntent
}

func (k keyMap) resolve(msg tea.KeyPressMsg) intent {
	switch {
	case key.Matches(msg, k.Down):
		return moveDownIntent
	case key.Matches(msg, k.Up):
		return moveUpIntent
	case key.Matches(msg, k.HalfDown):
		return halfPageDownIntent
	case key.Matches(msg, k.HalfUp):
		return halfPageUpIntent
	case key.Matches(msg, k.Top):
		return topIntent
	case key.Matches(msg, k.Bottom):
		return bottomIntent
	case key.Matches(msg, k.expand):
		return expandIntent
	case key.Matches(msg, k.collapse):
		return collapseIntent
	case key.Matches(msg, k.restore):
		return restoreIntent
	case key.Matches(msg, k.undo):
		return undoIntent
	case key.Matches(msg, k.abandon):
		return abandonIntent
	case key.Matches(msg, k.reload):
		return reloadIntent
	}
	return noIntent
}

func (k keyMap) resolveConfirm(msg tea.KeyPressMsg) intent {
	if !msg.IsRepeat && key.Matches(msg, k.confirm) {
		return confirmIntent
	}
	return noIntent
}

func newKeyMap() keyMap {
	return keyMap{
		NavKeys:  ui.NewNavKeys("next operation", "previous operation", "newest operation", "oldest operation"),
		restore:  key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "restore the repo to the operation (y confirms)")),
		undo:     key.NewBinding(key.WithKeys("u"), key.WithHelp("u", "undo the operation (y confirms)")),
		abandon:  key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "abandon the operation from the history (y confirms)")),
		expand:   key.NewBinding(key.WithKeys("l"), key.WithHelp("l", "show the command of the operation")),
		collapse: key.NewBinding(key.WithKeys("h"), key.WithHelp("h", "hide the command of the operation")),
		confirm:  key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "confirm")),
		reload:   key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "refresh the operations")),
	}
}

type pending struct {
	action intent
	op     jj.Operation
}

type Model struct {
	ctx      context.Context
	src      Source
	keys     keyMap
	items    []jj.Operation
	list     ui.Loader
	cursor   int
	top      int
	confirm  *pending
	expanded map[string]bool
}

func New(ctx context.Context, src Source) Model {
	return Model{ctx: ctx, src: src, keys: newKeyMap(), expanded: map[string]bool{}}
}

const entryRow = -1

type row struct {
	op   int
	line int
}

type loadedMsg struct {
	seq   int
	items []jj.Operation
	err   error
}

func (o Model) selected() *jj.Operation {
	if o.cursor < 0 || o.cursor >= len(o.items) {
		return nil
	}
	return &o.items[o.cursor]
}

func (o Model) SelectedID() string {
	if s := o.selected(); s != nil {
		return s.ID
	}
	return ""
}

func (o Model) OverlayOpen() bool {
	return o.confirm != nil
}

func (o Model) Captures() bool {
	return o.OverlayOpen()
}

func (o Model) Ignores(msg tea.KeyPressMsg, env tab.Env) bool {
	return o.confirm == nil && o.keys.resolve(msg).action() && (msg.IsRepeat || env.Lock.Busy || o.list.Loading())
}

func (o *Model) move(delta int) {
	o.cursor = max(min(o.cursor+delta, len(o.items)-1), 0)
}

func indent(op jj.Operation) int {
	return 2 + len(ui.ShortID(op.ID))
}

func details(op jj.Operation, width int) []string {
	if op.Args == "" {
		return []string{"no command recorded"}
	}
	width = max(width-indent(op), 1)
	var out []string
	for line := range strings.SplitSeq(op.Args, "\n") {
		out = append(out, strings.Split(ansi.Hardwrap(ansi.Wrap(printable(line), width, ""), width, true), "\n")...)
	}
	return out
}

func printable(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsControl(r) {
			q := strconv.QuoteRune(r)
			b.WriteString(q[1 : len(q)-1])
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func (o Model) rows(width int) []row {
	var rows []row
	for i, op := range o.items {
		rows = append(rows, row{op: i, line: entryRow})
		if !o.expanded[op.ID] {
			continue
		}
		for j := range details(op, width) {
			rows = append(rows, row{op: i, line: j})
		}
	}
	return rows
}

func (o Model) cursorRow(rows []row) int {
	return max(slices.Index(rows, row{op: o.cursor, line: entryRow}), 0)
}

func (o Model) cursorSpan(rows []row) (int, int) {
	start := o.cursorRow(rows)
	end := start
	for end+1 < len(rows) && rows[end+1].op == o.cursor {
		end++
	}
	return start, end
}

func listWidth(env tab.Env) int {
	return max(env.Width-2, 0)
}

func (o *Model) KeepVisible(env tab.Env) {
	height := env.ListHeight
	rows := o.rows(listWidth(env))
	top := min(o.top, max(len(rows)-height, 0))
	start, end := o.cursorSpan(rows)
	switch {
	case end < top:
		top = start
	case start >= top+height:
		top = start - height + 1
	}
	o.top = max(top, 0)
}

func (o *Model) expand(height, width int) {
	s := o.selected()
	if s == nil {
		return
	}
	o.expanded[s.ID] = true
	start, end := o.cursorSpan(o.rows(width))
	if end >= o.top+height {
		o.top = max(min(start, end-height+1), 0)
	}
}

func (o *Model) jump(delta, width int) {
	o.move(delta)
	o.top = min(o.top, o.cursorRow(o.rows(width)))
}

func (o *Model) scrollPage(delta, height, width int) {
	rows := o.rows(width)
	if len(rows) == 0 {
		return
	}
	offset := min(max(o.cursorRow(rows)-o.top, 0), height-1)
	target := min(max(o.top+offset+delta, 0), len(rows)-1)
	prevTop, prevCursor := o.top, o.cursor
	o.top = min(max(o.top+delta, 0), max(len(rows)-height, 0))
	o.cursor = rows[target].op
	if o.top == prevTop && o.cursor == prevCursor {
		o.move(max(min(delta, 1), -1))
	}
}

func (o *Model) Show(tab.Env) tea.Cmd {
	if o.list.Due() {
		return o.Load()
	}
	return nil
}

func (o *Model) Load() tea.Cmd {
	seq, ctx, src := o.list.Begin(), o.ctx, o.src
	return func() tea.Msg {
		items, err := src.Operations(ctx)
		if err != nil {
			err = fmt.Errorf("load operations: %w", err)
		}
		return loadedMsg{seq: seq, items: items, err: err}
	}
}

func (o *Model) MarkStale() {
	o.list.MarkStale()
}

func (o *Model) Update(msg tea.Msg, env tab.Env) (tea.Cmd, tab.Effect, bool) {
	loaded, ok := msg.(loadedMsg)
	if !ok {
		return nil, tab.Effect{}, false
	}
	want := o.SelectedID()
	if !o.list.Finish(loaded.seq, loaded.err) {
		return nil, tab.Effect{}, true
	}
	o.items = loaded.items
	maps.DeleteFunc(o.expanded, func(id string, _ bool) bool {
		return !slices.ContainsFunc(o.items, func(op jj.Operation) bool { return op.ID == id })
	})
	if i := slices.IndexFunc(o.items, func(op jj.Operation) bool { return op.ID == want }); i >= 0 {
		o.cursor = i
	} else {
		o.cursor = max(min(o.cursor, len(o.items)-1), 0)
	}
	if o.list.Stale() && env.Active {
		return o.Load(), tab.Effect{}, true
	}
	return nil, tab.Effect{}, true
}

func (o *Model) Key(msg tea.KeyPressMsg, env tab.Env) (tea.Cmd, tab.Effect) {
	if o.confirm != nil {
		return nil, o.confirmKey(msg, env)
	}
	half := max(env.ListHeight/2, 1)
	width := listWidth(env)
	switch in := o.keys.resolve(msg); in {
	case moveDownIntent:
		o.jump(1, width)
	case moveUpIntent:
		o.jump(-1, width)
	case halfPageDownIntent:
		o.scrollPage(half, env.ListHeight, width)
	case halfPageUpIntent:
		o.scrollPage(-half, env.ListHeight, width)
	case topIntent:
		o.jump(-len(o.items), width)
	case bottomIntent:
		o.jump(len(o.items), width)
	case expandIntent:
		o.expand(env.ListHeight, width)
	case collapseIntent:
		if s := o.selected(); s != nil {
			delete(o.expanded, s.ID)
		}
	case restoreIntent, undoIntent, abandonIntent:
		return nil, o.ask(in)
	case reloadIntent:
		return nil, tab.Effect{Refresh: !o.list.Loading()}
	}
	return nil, tab.Effect{}
}

func (o *Model) ask(action intent) tab.Effect {
	s := o.selected()
	switch {
	case s == nil:
		return tab.Effect{}
	case action == restoreIntent && s.Current:
		return tab.Effect{Status: "already at operation " + ui.ShortID(s.ID)}
	case action == undoIntent && s.Root:
		return tab.Effect{Status: "the root operation cannot be undone"}
	case action == abandonIntent && s.Root:
		return tab.Effect{Status: "the root operation cannot be abandoned"}
	case action == abandonIntent && s.Current:
		return tab.Effect{Status: "the current operation cannot be abandoned; press " + o.keys.undo.Help().Key + " to undo it first"}
	}
	o.confirm = &pending{action: action, op: *s}
	return tab.Effect{}
}

func (o *Model) confirmKey(msg tea.KeyPressMsg, env tab.Env) tab.Effect {
	c := o.confirm
	o.confirm = nil
	switch {
	case o.keys.resolveConfirm(msg) != confirmIntent, o.SelectedID() != c.op.ID, env.Lock.Busy:
		return tab.Effect{}
	case env.Lock.Reason != "":
		return tab.Effect{Status: env.Lock.Reason}
	}
	rw := o.rewrite(*c)
	return tab.Effect{Rewrite: &rw}
}

func (o Model) rewrite(c pending) tab.Rewrite {
	src, id, short := o.src, c.op.ID, ui.ShortID(c.op.ID)
	return tab.Rewrite{Run: func(ctx context.Context, head tab.Head) (tab.Result, error) {
		switch c.action {
		case restoreIntent:
			if err := src.RestoreOperation(ctx, id); err != nil {
				return tab.Result{KeepPicks: true}, fmt.Errorf("restore operation %s: %w", short, err)
			}
			return tab.Result{Done: "restored operation " + short, Replaced: head.WorkingCopy, Restored: true}, nil
		case undoIntent:
			if err := src.RevertOperation(ctx, id); err != nil {
				return tab.Result{KeepPicks: true}, fmt.Errorf("undo operation %s: %w", short, err)
			}
			return tab.Result{Done: "undid operation " + short, Replaced: head.WorkingCopy, Restored: true}, nil
		}
		if err := src.AbandonOperation(ctx, id); err != nil {
			return tab.Result{KeepPicks: true}, fmt.Errorf("abandon operation %s: %w", short, err)
		}
		return tab.Result{Done: "abandoned operation " + short, KeepPicks: true}, nil
	}}
}

func (o Model) Overlay(env tab.Env) string {
	if o.confirm == nil {
		return ""
	}
	p := env.Theme
	c := o.confirm
	inner := ui.InputBoxInner(env.ModalWidth)
	wrap := func(style lipgloss.Style, text string) []string {
		return ui.WrapLines(p, inner, style, text)
	}
	verb := c.action.verb()
	lines := wrap(ui.Fg(p.Title).Bold(true), strings.ToUpper(verb[:1])+verb[1:]+" operation "+ui.ShortID(c.op.ID))
	lines = append(lines, wrap(ui.Fg(p.Strong), describe(c.op))...)
	var args []string
	if c.op.Args != "" {
		args = wrap(ui.Fg(p.Muted), c.op.Args)
	}
	tail := wrap(ui.Fg(p.Muted), c.op.Time+" by "+c.op.User)
	switch c.action {
	case restoreIntent:
		tail = append(tail, wrap(ui.Fg(p.Picked), "Returns the repo to its state after this operation")...)
	case undoIntent:
		tail = append(tail, wrap(ui.Fg(p.Picked), "Applies the inverse of this operation")...)
	case abandonIntent:
		tail = append(tail, wrap(ui.Fg(p.Removed), "Removes the operation from the history; the repo is unchanged")...)
	}
	if d := env.Dropped; d > 0 && c.action != abandonIntent {
		tail = append(tail, wrap(ui.Fg(p.Removed), fmt.Sprintf("Drops %d selected %s", d, ui.Plural(d, "hunk")))...)
	}
	footer := wrap(ui.Fg(p.Muted), o.keys.confirm.Help().Key+" "+verb+" · any other key cancels")
	ellipsis := ui.Ellipsis(p, inner)
	args = ui.TruncateLines(args, max(env.Height-2-len(footer)-len(lines)-len(tail), 1), ellipsis)
	lines = append(append(lines, args...), tail...)
	lines = ui.TruncateLines(lines, env.Height-2-len(footer), ellipsis)
	return ui.ModalBox(p, append(lines, footer...))
}

func describe(op jj.Operation) string {
	if op.Root {
		return "root"
	}
	return op.Description
}

func (o Model) Hints(tabs, help ui.KeyHint) []ui.KeyHint {
	return []ui.KeyHint{
		tabs,
		ui.Hint("operation", o.keys.Down, o.keys.Up),
		ui.Hint("command", o.keys.expand, o.keys.collapse),
		ui.Hint("restore", o.keys.restore),
		ui.Hint("undo", o.keys.undo),
		ui.Hint("abandon", o.keys.abandon),
		ui.Hint("refresh", o.keys.reload),
		help,
	}
}

func (o Model) Paste(tea.PasteMsg) tea.Cmd {
	return nil
}

func (o Model) Help() []ui.KeyHint {
	return []ui.KeyHint{
		ui.Hint("next / previous operation", o.keys.Down, o.keys.Up),
		ui.Hint("half page down / up", o.keys.HalfDown, o.keys.HalfUp),
		ui.Hint("newest / oldest operation", o.keys.Top, o.keys.Bottom),
		ui.Describe(o.keys.expand),
		ui.Describe(o.keys.collapse),
		ui.Describe(o.keys.restore),
		ui.Describe(o.keys.undo),
		ui.Describe(o.keys.abandon),
		ui.Describe(o.keys.reload),
	}
}

func (o Model) View(env tab.Env) string {
	p := env.Theme
	inner := listWidth(env)
	var body []string
	var value []ui.Seg
	switch {
	case o.list.Err() != nil:
		body = []string{ui.Paint(inner, p.Card, []ui.Seg{ui.Span(ui.Fg(p.Removed), "  error: "+o.list.Err().Error())}, nil)}
	case !o.list.Loaded():
		body = []string{ui.Message(p, "loading…", inner)}
	case len(o.items) == 0:
		body = []string{ui.Message(p, "no operations", inner)}
	default:
		value = []ui.Seg{ui.Span(ui.Fg(p.Title), fmt.Sprintf("operation %d/%d", o.cursor+1, len(o.items)))}
		rows := o.rows(inner)
		for i := o.top; i < len(rows) && len(body) < env.ListHeight; i++ {
			if rows[i].line == entryRow {
				body = append(body, o.line(p, rows[i].op, inner))
				continue
			}
			body = append(body, o.detail(p, rows[i], inner))
		}
	}
	return ui.Card(p, env.Width, env.BodyHeight, "Operations", true, value, body)
}

func (o Model) line(p theme.Theme, i, width int) string {
	op := o.items[i]
	bg := p.Card
	if i == o.cursor {
		bg = p.CursorBg
	}
	bar := bg
	if op.Current {
		bar = p.Selection
	}
	text := ui.Fg(p.Strong)
	if op.Snapshot || op.Root {
		text = ui.Fg(p.Muted)
	}
	gap := ui.Span(lipgloss.NewStyle(), " ")
	left := []ui.Seg{ui.Span(ui.Fg(bar), ui.BarGlyph(bar)), ui.Span(ui.Fg(p.Title), ui.ShortID(op.ID)), gap}
	if op.Current {
		left = append(left, ui.Pill(p.Chip, p.Key, "@"), gap)
	}
	left = append(left, ui.Span(text, firstLine(describe(op))))
	right := []ui.Seg{ui.Span(ui.Fg(p.Muted), op.Time), gap}
	return ui.Paint(width, bg, left, right)
}

func (o Model) detail(p theme.Theme, r row, width int) string {
	op := o.items[r.op]
	style := ui.Fg(p.Strong)
	if op.Args == "" {
		style = ui.Fg(p.Muted)
	}
	return ui.Paint(width, p.Card, []ui.Seg{ui.Span(lipgloss.NewStyle(), strings.Repeat(" ", indent(op))), ui.Span(style, details(op, width)[r.line])}, nil)
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}
