package bookmarks

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/madalinpopa/jjreview/internal/jj"
	"github.com/madalinpopa/jjreview/internal/theme"
	"github.com/madalinpopa/jjreview/internal/tui/ui"
)

type Source interface {
	Bookmarks(ctx context.Context) ([]jj.Bookmark, error)
	CreateBookmark(ctx context.Context, name, rev string) (string, error)
	RenameBookmark(ctx context.Context, oldName, newName string) (string, error)
	MoveBookmark(ctx context.Context, name string, from []string, to string, allowBackwards bool) (string, error)
	DeleteBookmark(ctx context.Context, name string) (string, error)
	Change(ctx context.Context, rev string) (jj.Change, error)
}

type Env struct {
	ui.Layout
	Head       string
	HeadChange string
	HeadTitle  string
	Active     bool
}

func (e Env) modalInner() int {
	return ui.InputBoxInner(e.ModalWidth)
}

type Effect struct {
	Status   string
	Refresh  bool
	Retarget bool
	Reopen   bool
	Open     string
}

type Model struct {
	ctx      context.Context
	src      Source
	keys     keyMap
	items    []jj.Bookmark
	list     ui.Loader
	cursor   int
	top      int
	want     string
	opened   bookmarkRef
	modal    *dialog
	deleting *jj.Bookmark
	picker   *picker
}

func New(ctx context.Context, src Source) Model {
	return Model{ctx: ctx, src: src, keys: newKeyMap()}
}

type bookmarkRef struct {
	name   string
	target string
}

func refOf(b jj.Bookmark) bookmarkRef {
	if b.Conflict {
		return bookmarkRef{name: b.Name, target: "conflict " + strings.Join(b.Added, ",") + " / " + strings.Join(b.Removed, ",")}
	}
	return bookmarkRef{name: b.Name, target: b.CommitID}
}

const nameWidth = 32

type loadedMsg struct {
	seq   int
	items []jj.Bookmark
	err   error
}

type deletedMsg struct {
	name     string
	warnings string
	refresh  bool
	err      error
}

func (b Model) selected() *jj.Bookmark {
	if b.cursor < 0 || b.cursor >= len(b.items) {
		return nil
	}
	return &b.items[b.cursor]
}

func (b Model) SelectedName() string {
	if s := b.selected(); s != nil {
		return s.Name
	}
	return ""
}

func (b Model) selectedRef() bookmarkRef {
	if s := b.selected(); s != nil {
		return refOf(*s)
	}
	return bookmarkRef{}
}

func (b Model) OverlayOpen() bool {
	return b.modal != nil || b.deleting != nil || b.picker != nil
}

func (b *Model) move(delta int) {
	b.cursor = max(min(b.cursor+delta, len(b.items)-1), 0)
}

func (b *Model) KeepVisible(height int) {
	b.top = ui.ScrollTop(b.top, b.cursor, height, len(b.items))
}

func (b *Model) scrollPage(delta, height int) {
	b.top = min(max(b.top+delta, 0), max(len(b.items)-height, 0))
	b.move(delta)
}

func (b *Model) Show() tea.Cmd {
	if b.list.Due() {
		return b.Load()
	}
	return nil
}

func (b *Model) Load() tea.Cmd {
	seq, ctx, src := b.list.Begin(), b.ctx, b.src
	return func() tea.Msg {
		items, err := src.Bookmarks(ctx)
		if err != nil {
			err = fmt.Errorf("load bookmarks: %w", err)
		}
		return loadedMsg{seq: seq, items: items, err: err}
	}
}

func (b *Model) MarkStale() {
	b.list.MarkStale()
}

func (b *Model) Update(msg tea.Msg, env Env) (tea.Cmd, Effect, bool) {
	switch msg := msg.(type) {
	case loadedMsg:
		cmd, eff := b.applyList(msg, env)
		return cmd, eff, true
	case savedMsg:
		return nil, b.applySaved(msg), true
	case pickedMsg:
		return nil, b.applyPicked(msg), true
	case deletedMsg:
		return nil, b.applyDeleted(msg), true
	}
	return nil, Effect{}, false
}

func (b *Model) applyList(msg loadedMsg, env Env) (tea.Cmd, Effect) {
	want := cmp.Or(b.want, b.SelectedName())
	if !b.list.Finish(msg.seq, msg.err) {
		return nil, Effect{}
	}
	b.items = msg.items
	if i := slices.IndexFunc(b.items, func(item jj.Bookmark) bool { return item.Name == want }); i >= 0 {
		b.cursor = i
	} else {
		b.cursor = max(min(b.cursor, len(b.items)-1), 0)
	}
	if !b.list.Stale() {
		b.want = ""
	}
	eff := Effect{Retarget: env.Active && b.selectedRef() != b.opened}
	if b.list.Stale() && env.Active {
		return b.Load(), eff
	}
	return nil, eff
}

func (b *Model) Key(msg tea.KeyPressMsg, env Env) (tea.Cmd, Effect) {
	switch {
	case b.deleting != nil:
		return b.deleteKey(msg), Effect{}
	case b.modal != nil:
		return b.modalKey(msg), Effect{}
	case b.picker != nil:
		return b.pickKey(msg, env)
	}
	half := max(env.ListHeight/2, 1)
	switch b.keys.resolve(msg) {
	case moveDownIntent:
		b.move(1)
	case moveUpIntent:
		b.move(-1)
	case halfDownIntent:
		b.scrollPage(half, env.ListHeight)
	case halfUpIntent:
		b.scrollPage(-half, env.ListHeight)
	case firstIntent:
		b.move(-len(b.items))
	case lastIntent:
		b.move(len(b.items))
	case openIntent:
		return nil, b.open()
	case createIntent:
		return b.openModal(nil, env), Effect{}
	case editIntent:
		s := b.selected()
		switch {
		case s == nil:
		case s.Conflict:
			return nil, Effect{Status: s.Name + " is conflicted; resolve it with jj bookmark set"}
		default:
			return b.openModal(s, env), Effect{}
		}
	case deleteIntent:
		if s := b.selected(); s != nil {
			b.deleting = new(*s)
		}
	case reloadIntent:
		return nil, Effect{Refresh: !b.list.Loading()}
	}
	return nil, Effect{}
}

func (b *Model) open() Effect {
	s := b.selected()
	switch {
	case s == nil:
		return Effect{}
	case s.CommitID == "":
		return Effect{Status: s.Name + " is conflicted: it has no single target"}
	}
	ref := b.selectedRef()
	eff := Effect{Open: s.CommitID, Reopen: ref == b.opened}
	b.opened = ref
	return eff
}

func (b *Model) deleteKey(msg tea.KeyPressMsg) tea.Cmd {
	d := *b.deleting
	b.deleting = nil
	if b.keys.resolveDelete(msg) != confirmIntent {
		return nil
	}
	ref, ctx, src := refOf(d), b.ctx, b.src
	return func() tea.Msg {
		return removeBookmark(ctx, src, ref)
	}
}

func (b Model) Overlay(env Env) string {
	switch {
	case b.modal != nil:
		return b.modalView(env)
	case b.deleting != nil:
		return b.deleteView(env)
	case b.picker != nil:
		return b.pickerView(env)
	}
	return ""
}

func (b Model) deleteView(env Env) string {
	p := env.Theme
	d := b.deleting
	inner := env.modalInner()
	lines := []string{ui.Paint(inner, p.Card, []ui.Seg{ui.Span(ui.Fg(p.Title).Bold(true), "Delete bookmark")}, nil)}
	for line := range strings.SplitSeq(ansi.Hardwrap(ui.EscapePath(d.Name), inner, true), "\n") {
		lines = append(lines, ui.Paint(inner, p.Card, []ui.Seg{ui.Span(ui.Fg(p.Key).Bold(true), line)}, nil))
	}
	target := []ui.Seg{ui.Span(ui.Fg(p.Removed), "conflicted")}
	if !d.Conflict {
		gap := ui.Span(lipgloss.NewStyle(), " ")
		target = []ui.Seg{ui.Span(ui.Fg(p.Title), ui.ShortID(d.ChangeID)), gap, ui.Span(ui.Fg(p.Muted), ui.ShortID(d.CommitID)), gap, ui.Description(p, d.Description, false)}
	}
	lines = append(lines,
		ui.Paint(inner, p.Card, target, nil),
		ui.Paint(inner, p.Card, nil, []ui.Seg{ui.Span(ui.Fg(p.Key), "y"), ui.Span(ui.Fg(p.Muted), " delete · any other key cancels")}),
	)
	return ui.ModalBox(p, lines)
}

func (b Model) Hints() []ui.KeyHint {
	return []ui.KeyHint{
		ui.Hint("bookmark", b.keys.Down, b.keys.Up),
		ui.Hint("review", b.keys.open),
		ui.Hint("new", b.keys.create),
		ui.Hint("edit", b.keys.edit),
		ui.Hint("delete", b.keys.remove),
		ui.Hint("refresh", b.keys.reload),
	}
}

func (b Model) Help() []ui.KeyHint {
	return []ui.KeyHint{
		ui.Hint("next / previous bookmark", b.keys.Down, b.keys.Up),
		ui.Hint("half page down / up", b.keys.HalfDown, b.keys.HalfUp),
		ui.Hint("first / last bookmark", b.keys.Top, b.keys.Bottom),
		ui.Describe(b.keys.open),
		ui.Describe(b.keys.create),
		ui.Describe(b.keys.edit),
		ui.Describe(b.keys.remove),
		ui.Describe(b.keys.reload),
	}
}

func removeBookmark(ctx context.Context, src Source, ref bookmarkRef) deletedMsg {
	if _, err := checkBookmark(ctx, src, ref, "you pressed d"); err != nil {
		return deletedMsg{name: ref.name, refresh: true, err: fmt.Errorf("%w; nothing deleted", err)}
	}
	warnings, err := src.DeleteBookmark(ctx, ref.name)
	if err != nil {
		return deletedMsg{name: ref.name, refresh: true, err: fmt.Errorf("delete bookmark %s: %w (%s)", ref.name, err, observe(ctx, src, ref.name))}
	}
	return deletedMsg{name: ref.name, warnings: warnings, refresh: true}
}

func checkBookmark(ctx context.Context, src Source, ref bookmarkRef, since string) ([]jj.Bookmark, error) {
	items, err := src.Bookmarks(ctx)
	if err != nil {
		return nil, fmt.Errorf("check bookmark %s: %w", ref.name, err)
	}
	i := slices.IndexFunc(items, func(b jj.Bookmark) bool { return b.Name == ref.name })
	switch {
	case i < 0:
		return nil, fmt.Errorf("%s no longer exists", ref.name)
	case refOf(items[i]) != ref:
		return nil, fmt.Errorf("%s changed since %s", ref.name, since)
	}
	return items, nil
}

func observe(ctx context.Context, src Source, names ...string) string {
	items, err := src.Bookmarks(ctx)
	if err != nil {
		return "could not re-read the bookmarks: " + err.Error()
	}
	parts := make([]string, len(names))
	for i, name := range names {
		j := slices.IndexFunc(items, func(b jj.Bookmark) bool { return b.Name == name })
		switch {
		case j < 0:
			parts[i] = name + " does not exist"
		case items[j].Conflict:
			parts[i] = name + " is conflicted"
		default:
			parts[i] = name + " → " + ui.ShortID(items[j].CommitID)
		}
	}
	return "now: " + strings.Join(parts, ", ")
}

func (b *Model) applyDeleted(msg deletedMsg) Effect {
	eff := Effect{Status: "✓ deleted " + msg.name}
	if msg.err != nil {
		eff.Status = msg.err.Error()
	} else if msg.warnings != "" {
		eff.Status += " · " + strings.Join(strings.Fields(msg.warnings), " ")
	}
	if msg.refresh {
		b.want = ""
		eff.Refresh = true
	}
	return eff
}

func (b Model) View(env Env) string {
	p := env.Theme
	inner := max(env.Width-2, 0)
	var body []string
	var value []ui.Seg
	switch {
	case b.list.Err() != nil:
		body = []string{ui.Paint(inner, p.Card, []ui.Seg{ui.Span(ui.Fg(p.Removed), "  error: "+b.list.Err().Error())}, nil)}
	case !b.list.Loaded():
		body = []string{ui.Message(p, "loading…", inner)}
	case len(b.items) == 0:
		body = []string{ui.Message(p, "no bookmarks", inner)}
	default:
		value = []ui.Seg{ui.Span(ui.Fg(p.Title), fmt.Sprintf("bookmark %d/%d", b.cursor+1, len(b.items)))}
		names := 0
		for _, item := range b.items {
			names = max(names, min(lipgloss.Width(item.Name), nameWidth))
		}
		for i := b.top; i < len(b.items) && len(body) < env.ListHeight; i++ {
			body = append(body, b.line(env, i, names, inner))
		}
	}
	return ui.Card(p, env.Width, env.BodyHeight, "Bookmarks", true, value, body)
}

func (b Model) line(env Env, i, names, width int) string {
	p := env.Theme
	item := b.items[i]
	bg := p.Card
	if i == b.cursor {
		bg = p.CursorBg
	}
	bar := bg
	if item.CommitID != "" && item.CommitID == env.Head {
		bar = p.Selection
	}
	gap := ui.Span(lipgloss.NewStyle(), " ")
	pad := strings.Repeat(" ", max(names-lipgloss.Width(item.Name), 0)+1)
	left := []ui.Seg{ui.Span(ui.Fg(bar), ui.BarGlyph(bar)), ui.Pill(p.Chip, p.Key, item.Name), ui.Span(lipgloss.NewStyle(), pad)}
	right := clipSegs(markers(p, item), width-ui.Width(left[:2])-1)
	if item.CommitID != "" {
		left = append(left,
			ui.Span(ui.Fg(p.Title), ui.ShortID(item.ChangeID)), gap,
			ui.Span(ui.Fg(p.Muted), ui.ShortID(item.CommitID)), gap,
			ui.Description(p, item.Description, false),
		)
	}
	return ui.Paint(width, bg, left, right)
}

func markers(p theme.Theme, b jj.Bookmark) []ui.Seg {
	gap := ui.Span(lipgloss.NewStyle(), " ")
	var segs []ui.Seg
	if b.Conflict {
		segs = append(segs, ui.Span(ui.Fg(p.Removed), "conflicted"), gap)
	}
	for _, r := range b.Remotes {
		if r.Synced {
			segs = append(segs, ui.Span(ui.Fg(p.Added), r.Remote+" ✓"), gap)
			continue
		}
		var parts []string
		if r.Ahead > 0 || r.AheadAtLeast {
			parts = append(parts, countLabel("↑", r.Ahead, r.AheadAtLeast))
		}
		if r.Behind > 0 || r.BehindAtLeast {
			parts = append(parts, countLabel("↓", r.Behind, r.BehindAtLeast))
		}
		if len(parts) == 0 {
			parts = append(parts, "≠")
		}
		segs = append(segs, ui.Span(ui.Fg(p.Picked), r.Remote+" "+strings.Join(parts, " ")), gap)
	}
	return segs
}

func countLabel(arrow string, n int, atLeast bool) string {
	if atLeast {
		return fmt.Sprintf("%s%d+", arrow, n)
	}
	return fmt.Sprintf("%s%d", arrow, n)
}

func clipSegs(segs []ui.Seg, width int) []ui.Seg {
	var out []ui.Seg
	used := 0
	for _, s := range segs {
		w := lipgloss.Width(s.Text)
		if used+w > width {
			if rest := width - used; rest > 1 {
				out = append(out, ui.Span(s.Style, ansi.Truncate(s.Text, rest-1, "")+"…"))
			}
			return out
		}
		out = append(out, s)
		used += w
	}
	return out
}
