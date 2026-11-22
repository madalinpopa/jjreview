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
	"github.com/madalinpopa/jjreview/internal/tui/ui"
)

type Source interface {
	Operations(ctx context.Context) ([]jj.Operation, error)
}

type Kind int

const (
	Restore Kind = iota
	Undo
	Abandon
)

func (k Kind) verb() string {
	switch k {
	case Restore:
		return "restore"
	case Undo:
		return "undo"
	}
	return "abandon"
}

type Action struct {
	Kind Kind
	Op   jj.Operation
}

type keyMap struct {
	down     key.Binding
	up       key.Binding
	halfDown key.Binding
	halfUp   key.Binding
	top      key.Binding
	bottom   key.Binding
	restore  key.Binding
	undo     key.Binding
	abandon  key.Binding
	expand   key.Binding
	collapse key.Binding
	confirm  key.Binding
	reload   key.Binding
}

func newKeyMap() keyMap {
	return keyMap{
		down:     key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("j", "next operation")),
		up:       key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("k", "previous operation")),
		halfDown: key.NewBinding(key.WithKeys("ctrl+d", "pgdown"), key.WithHelp("^d", "half page down")),
		halfUp:   key.NewBinding(key.WithKeys("ctrl+u", "pgup"), key.WithHelp("^u", "half page up")),
		top:      key.NewBinding(key.WithKeys("g", "home"), key.WithHelp("g", "newest operation")),
		bottom:   key.NewBinding(key.WithKeys("G", "end"), key.WithHelp("G", "oldest operation")),
		restore:  key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "restore the repo to the operation (y confirms)")),
		undo:     key.NewBinding(key.WithKeys("u"), key.WithHelp("u", "undo the operation (y confirms)")),
		abandon:  key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "abandon the operation from the history (y confirms)")),
		expand:   key.NewBinding(key.WithKeys("l"), key.WithHelp("l", "show the command of the operation")),
		collapse: key.NewBinding(key.WithKeys("h"), key.WithHelp("h", "hide the command of the operation")),
		confirm:  key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "confirm")),
		reload:   key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "refresh the operations")),
	}
}

type Env struct {
	ui.Layout
	Dropped int
}

type Intent struct {
	Status  string
	Refresh bool
	Run     *Action
}

type Model struct {
	ctx      context.Context
	src      Source
	keys     keyMap
	items    []jj.Operation
	loaded   bool
	loading  bool
	stale    bool
	err      error
	cursor   int
	top      int
	seq      int
	confirm  *Action
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

func (o Model) Ignores(msg tea.KeyPressMsg, reloading bool) bool {
	return o.confirm == nil && key.Matches(msg, o.keys.restore, o.keys.undo, o.keys.abandon) && (msg.IsRepeat || reloading || o.loading)
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

func listWidth(env Env) int {
	return max(env.Width-2, 0)
}

func (o *Model) KeepVisible(env Env) {
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

func (o *Model) Show() tea.Cmd {
	if !o.loading && (!o.loaded || o.stale) {
		return o.Load()
	}
	return nil
}

func (o *Model) Load() tea.Cmd {
	o.seq++
	o.loading = true
	o.stale = false
	seq, ctx, src := o.seq, o.ctx, o.src
	return func() tea.Msg {
		items, err := src.Operations(ctx)
		if err != nil {
			err = fmt.Errorf("load operations: %w", err)
		}
		return loadedMsg{seq: seq, items: items, err: err}
	}
}

func (o *Model) MarkStale() {
	o.stale = true
}

func (o *Model) Update(msg tea.Msg, active bool) (tea.Cmd, bool) {
	loaded, ok := msg.(loadedMsg)
	if !ok {
		return nil, false
	}
	if loaded.seq != o.seq {
		return nil, true
	}
	want := o.SelectedID()
	o.loading = false
	o.loaded = true
	o.err = loaded.err
	o.items = loaded.items
	maps.DeleteFunc(o.expanded, func(id string, _ bool) bool {
		return !slices.ContainsFunc(o.items, func(op jj.Operation) bool { return op.ID == id })
	})
	if i := slices.IndexFunc(o.items, func(op jj.Operation) bool { return op.ID == want }); i >= 0 {
		o.cursor = i
	} else {
		o.cursor = max(min(o.cursor, len(o.items)-1), 0)
	}
	if o.stale && active {
		return o.Load(), true
	}
	return nil, true
}

func (o *Model) Key(msg tea.KeyPressMsg, env Env) Intent {
	if o.confirm != nil {
		return o.confirmKey(msg)
	}
	half := max(env.ListHeight/2, 1)
	width := listWidth(env)
	switch {
	case key.Matches(msg, o.keys.down):
		o.jump(1, listWidth(env))
	case key.Matches(msg, o.keys.up):
		o.jump(-1, listWidth(env))
	case key.Matches(msg, o.keys.halfDown):
		o.scrollPage(half, env.ListHeight, width)
	case key.Matches(msg, o.keys.halfUp):
		o.scrollPage(-half, env.ListHeight, width)
	case key.Matches(msg, o.keys.top):
		o.jump(-len(o.items), width)
	case key.Matches(msg, o.keys.bottom):
		o.jump(len(o.items), width)
	case key.Matches(msg, o.keys.expand):
		o.expand(env.ListHeight, width)
	case key.Matches(msg, o.keys.collapse):
		if s := o.selected(); s != nil {
			delete(o.expanded, s.ID)
		}
	case key.Matches(msg, o.keys.restore):
		return o.ask(Restore)
	case key.Matches(msg, o.keys.undo):
		return o.ask(Undo)
	case key.Matches(msg, o.keys.abandon):
		return o.ask(Abandon)
	case key.Matches(msg, o.keys.reload):
		return Intent{Refresh: !o.loading}
	}
	return Intent{}
}

func (o *Model) ask(kind Kind) Intent {
	s := o.selected()
	switch {
	case s == nil:
		return Intent{}
	case kind == Restore && s.Current:
		return Intent{Status: "already at operation " + ui.ShortID(s.ID)}
	case kind == Undo && s.Root:
		return Intent{Status: "the root operation cannot be undone"}
	case kind == Abandon && s.Root:
		return Intent{Status: "the root operation cannot be abandoned"}
	case kind == Abandon && s.Current:
		return Intent{Status: "the current operation cannot be abandoned; press " + o.keys.undo.Help().Key + " to undo it first"}
	}
	o.confirm = &Action{Kind: kind, Op: *s}
	return Intent{}
}

func (o *Model) confirmKey(msg tea.KeyPressMsg) Intent {
	c := o.confirm
	o.confirm = nil
	if msg.IsRepeat || !key.Matches(msg, o.keys.confirm) || o.SelectedID() != c.Op.ID {
		return Intent{}
	}
	return Intent{Run: c}
}

func (o Model) Overlay(env Env) string {
	if o.confirm == nil {
		return ""
	}
	p := env.Theme
	c := o.confirm
	inner := ui.InputBoxInner(env.ModalWidth)
	wrap := func(style lipgloss.Style, text string) []string {
		var out []string
		for line := range strings.SplitSeq(ansi.Wrap(text, inner, ""), "\n") {
			out = append(out, ui.Paint(inner, p.Card, []ui.Seg{ui.Span(style, line)}, nil))
		}
		return out
	}
	verb := c.Kind.verb()
	lines := wrap(ui.Fg(p.Title).Bold(true), strings.ToUpper(verb[:1])+verb[1:]+" operation "+ui.ShortID(c.Op.ID))
	lines = append(lines, wrap(ui.Fg(p.Strong), describe(c.Op))...)
	var args []string
	if c.Op.Args != "" {
		args = wrap(ui.Fg(p.Muted), c.Op.Args)
	}
	tail := wrap(ui.Fg(p.Muted), c.Op.Time+" by "+c.Op.User)
	switch c.Kind {
	case Restore:
		tail = append(tail, wrap(ui.Fg(p.Picked), "Returns the repo to its state after this operation")...)
	case Undo:
		tail = append(tail, wrap(ui.Fg(p.Picked), "Applies the inverse of this operation")...)
	case Abandon:
		tail = append(tail, wrap(ui.Fg(p.Removed), "Removes the operation from the history; the repo is unchanged")...)
	}
	if d := env.Dropped; d > 0 && c.Kind != Abandon {
		tail = append(tail, wrap(ui.Fg(p.Removed), fmt.Sprintf("Drops %d selected %s", d, ui.Plural(d, "hunk")))...)
	}
	footer := wrap(ui.Fg(p.Muted), o.keys.confirm.Help().Key+" "+verb+" · any other key cancels")
	if room := max(env.Height-2-len(footer)-len(lines)-len(tail), 1); len(args) > room {
		args = append(args[:room-1], ui.Paint(inner, p.Card, []ui.Seg{ui.Span(ui.Fg(p.Muted), "…")}, nil))
	}
	lines = append(append(lines, args...), tail...)
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

func describe(op jj.Operation) string {
	if op.Root {
		return "root"
	}
	return op.Description
}

func (o Model) Hints() []ui.KeyHint {
	return []ui.KeyHint{
		ui.Hint("operation", o.keys.down, o.keys.up),
		ui.Hint("command", o.keys.expand, o.keys.collapse),
		ui.Hint("restore", o.keys.restore),
		ui.Hint("undo", o.keys.undo),
		ui.Hint("abandon", o.keys.abandon),
		ui.Hint("refresh", o.keys.reload),
	}
}

func (o Model) Help() []ui.KeyHint {
	return []ui.KeyHint{
		ui.Hint("next / previous operation", o.keys.down, o.keys.up),
		ui.Hint("half page down / up", o.keys.halfDown, o.keys.halfUp),
		ui.Hint("newest / oldest operation", o.keys.top, o.keys.bottom),
		ui.Describe(o.keys.expand),
		ui.Describe(o.keys.collapse),
		ui.Describe(o.keys.restore),
		ui.Describe(o.keys.undo),
		ui.Describe(o.keys.abandon),
		ui.Describe(o.keys.reload),
	}
}

func (o Model) View(env Env) string {
	p := env.Theme
	inner := listWidth(env)
	var body []string
	var value []ui.Seg
	switch {
	case o.err != nil:
		body = []string{ui.Paint(inner, p.Card, []ui.Seg{ui.Span(ui.Fg(p.Removed), "  error: "+o.err.Error())}, nil)}
	case !o.loaded:
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
