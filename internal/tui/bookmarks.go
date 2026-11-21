package tui

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/madalinpopa/jjreview/internal/jj"
	"github.com/madalinpopa/jjreview/internal/theme"
	"github.com/madalinpopa/jjreview/internal/tui/ui"
)

type bookmarkSource interface {
	Bookmarks(ctx context.Context) ([]jj.Bookmark, error)
	CreateBookmark(ctx context.Context, name, rev string) (string, error)
	RenameBookmark(ctx context.Context, oldName, newName string) (string, error)
	MoveBookmark(ctx context.Context, name string, from []string, to string, allowBackwards bool) (string, error)
	DeleteBookmark(ctx context.Context, name string) (string, error)
	Change(ctx context.Context, rev string) (jj.Change, error)
}

type bookmarkKeys struct {
	down     key.Binding
	up       key.Binding
	halfDown key.Binding
	halfUp   key.Binding
	top      key.Binding
	bottom   key.Binding
	open     key.Binding
	create   key.Binding
	edit     key.Binding
	remove   key.Binding
	reload   key.Binding
}

func newBookmarkKeys() bookmarkKeys {
	return bookmarkKeys{
		down:     key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("j", "next bookmark")),
		up:       key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("k", "previous bookmark")),
		halfDown: key.NewBinding(key.WithKeys("ctrl+d", "pgdown"), key.WithHelp("^d", "half page down")),
		halfUp:   key.NewBinding(key.WithKeys("ctrl+u", "pgup"), key.WithHelp("^u", "half page up")),
		top:      key.NewBinding(key.WithKeys("g", "home"), key.WithHelp("g", "first bookmark")),
		bottom:   key.NewBinding(key.WithKeys("G", "end"), key.WithHelp("G", "last bookmark")),
		open:     key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "review the target revision")),
		create:   key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "new bookmark on a revision")),
		edit:     key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "edit the bookmark: rename or move it")),
		remove:   key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "delete the bookmark (y confirms)")),
		reload:   key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "refresh the bookmarks")),
	}
}

type bookmarkEnv struct {
	pal    theme.Theme
	width  int
	height int
	body   int
	list   int
	modal  int
	head   string
	active bool
}

type bookmarkIntent struct {
	status     string
	refresh    bool
	retarget   bool
	open       string
	sameTarget bool
}

type bookmarksModel struct {
	ctx      context.Context
	src      bookmarkSource
	keys     bookmarkKeys
	items    []jj.Bookmark
	loaded   bool
	loading  bool
	stale    bool
	err      error
	cursor   int
	top      int
	seq      int
	want     string
	armed    bookmarkRef
	modal    *bookmarkModal
	deleting *jj.Bookmark
}

func newBookmarksModel(ctx context.Context, src bookmarkSource) bookmarksModel {
	return bookmarksModel{ctx: ctx, src: src, keys: newBookmarkKeys()}
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

const bookmarkNameWidth = 32

type bookmarksMsg struct {
	seq   int
	items []jj.Bookmark
	err   error
}

type bookmarkDeletedMsg struct {
	name     string
	warnings string
	refresh  bool
	err      error
}

func (b bookmarksModel) selected() *jj.Bookmark {
	if b.cursor < 0 || b.cursor >= len(b.items) {
		return nil
	}
	return &b.items[b.cursor]
}

func (b bookmarksModel) selectedName() string {
	if s := b.selected(); s != nil {
		return s.Name
	}
	return ""
}

func (b bookmarksModel) selectedRef() bookmarkRef {
	if s := b.selected(); s != nil {
		return refOf(*s)
	}
	return bookmarkRef{}
}

func (b bookmarksModel) overlayOpen() bool {
	return b.modal != nil || b.deleting != nil
}

func (b *bookmarksModel) move(delta int) {
	b.cursor = max(min(b.cursor+delta, len(b.items)-1), 0)
}

func (b *bookmarksModel) keepVisible(height int) {
	b.top = ui.ScrollTop(b.top, b.cursor, height, len(b.items))
}

func (b *bookmarksModel) scrollPage(delta, height int) {
	b.top = min(max(b.top+delta, 0), max(len(b.items)-height, 0))
	b.move(delta)
}

func (b *bookmarksModel) show() tea.Cmd {
	if !b.loading && (!b.loaded || b.stale) {
		return b.load()
	}
	return nil
}

func (b *bookmarksModel) load() tea.Cmd {
	b.seq++
	b.loading = true
	b.stale = false
	seq, ctx, src := b.seq, b.ctx, b.src
	return func() tea.Msg {
		items, err := src.Bookmarks(ctx)
		if err != nil {
			err = fmt.Errorf("load bookmarks: %w", err)
		}
		return bookmarksMsg{seq: seq, items: items, err: err}
	}
}

func (b *bookmarksModel) markStale() {
	b.stale = true
}

func (b *bookmarksModel) update(msg tea.Msg, env bookmarkEnv) (tea.Cmd, bookmarkIntent) {
	switch msg := msg.(type) {
	case bookmarksMsg:
		return b.applyList(msg, env)
	case bookmarkSavedMsg:
		return nil, b.applySaved(msg)
	case bookmarkDeletedMsg:
		return nil, b.applyDeleted(msg)
	}
	return nil, bookmarkIntent{}
}

func (b *bookmarksModel) applyList(msg bookmarksMsg, env bookmarkEnv) (tea.Cmd, bookmarkIntent) {
	if msg.seq != b.seq {
		return nil, bookmarkIntent{}
	}
	before := b.selectedName()
	want := cmp.Or(b.want, before)
	b.loading = false
	b.loaded = true
	b.err = msg.err
	b.items = msg.items
	if i := slices.IndexFunc(b.items, func(item jj.Bookmark) bool { return item.Name == want }); i >= 0 {
		b.cursor = i
	} else {
		b.cursor = max(min(b.cursor, len(b.items)-1), 0)
	}
	if !b.stale {
		b.want = ""
	}
	in := bookmarkIntent{retarget: env.active && b.selectedRef() != b.armed}
	if b.stale && env.active {
		return b.load(), in
	}
	return nil, in
}

func (b *bookmarksModel) key(msg tea.KeyPressMsg, env bookmarkEnv) (tea.Cmd, bookmarkIntent) {
	switch {
	case b.deleting != nil:
		return b.deleteKey(msg), bookmarkIntent{}
	case b.modal != nil:
		return b.modalKey(msg), bookmarkIntent{}
	}
	half := max(env.list/2, 1)
	switch {
	case key.Matches(msg, b.keys.down):
		b.move(1)
	case key.Matches(msg, b.keys.up):
		b.move(-1)
	case key.Matches(msg, b.keys.halfDown):
		b.scrollPage(half, env.list)
	case key.Matches(msg, b.keys.halfUp):
		b.scrollPage(-half, env.list)
	case key.Matches(msg, b.keys.top):
		b.move(-len(b.items))
	case key.Matches(msg, b.keys.bottom):
		b.move(len(b.items))
	case key.Matches(msg, b.keys.open):
		return nil, b.open()
	case key.Matches(msg, b.keys.create):
		return b.openModal(nil, env), bookmarkIntent{}
	case key.Matches(msg, b.keys.edit):
		s := b.selected()
		switch {
		case s == nil:
		case s.Conflict:
			return nil, bookmarkIntent{status: s.Name + " is conflicted; resolve it with jj bookmark set"}
		default:
			return b.openModal(s, env), bookmarkIntent{}
		}
	case key.Matches(msg, b.keys.remove):
		if s := b.selected(); s != nil {
			b.deleting = new(*s)
		}
	case key.Matches(msg, b.keys.reload):
		return nil, bookmarkIntent{refresh: !b.loading}
	}
	return nil, bookmarkIntent{}
}

func (b *bookmarksModel) open() bookmarkIntent {
	s := b.selected()
	switch {
	case s == nil:
		return bookmarkIntent{}
	case s.CommitID == "":
		return bookmarkIntent{status: s.Name + " is conflicted: it has no single target"}
	}
	ref := b.selectedRef()
	in := bookmarkIntent{open: s.CommitID, sameTarget: ref == b.armed}
	b.armed = ref
	return in
}

func (b *bookmarksModel) deleteKey(msg tea.KeyPressMsg) tea.Cmd {
	d := *b.deleting
	b.deleting = nil
	if msg.String() != "y" {
		return nil
	}
	ref, ctx, src := refOf(d), b.ctx, b.src
	return func() tea.Msg {
		return removeBookmark(ctx, src, ref)
	}
}

func (b bookmarksModel) overlay(env bookmarkEnv) string {
	switch {
	case b.modal != nil:
		return b.modalView(env)
	case b.deleting != nil:
		return b.deleteView(env)
	}
	return ""
}

func (b bookmarksModel) deleteView(env bookmarkEnv) string {
	p := env.pal
	d := b.deleting
	inner := env.modal
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
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(p.ModalBorder).
		BorderBackground(p.Card).
		Background(p.Card).
		Padding(0, 1).
		Render(strings.Join(lines, "\n"))
}

func (b bookmarksModel) hints() []ui.KeyHint {
	return []ui.KeyHint{
		ui.Hint("bookmark", b.keys.down, b.keys.up),
		ui.Hint("review", b.keys.open),
		ui.Hint("new", b.keys.create),
		ui.Hint("edit", b.keys.edit),
		ui.Hint("delete", b.keys.remove),
		ui.Hint("refresh", b.keys.reload),
	}
}

func (b bookmarksModel) help() []ui.KeyHint {
	return []ui.KeyHint{
		ui.Hint("next / previous bookmark", b.keys.down, b.keys.up),
		ui.Hint("half page down / up", b.keys.halfDown, b.keys.halfUp),
		ui.Hint("first / last bookmark", b.keys.top, b.keys.bottom),
		ui.Describe(b.keys.open),
		ui.Describe(b.keys.create),
		ui.Describe(b.keys.edit),
		ui.Describe(b.keys.remove),
		ui.Describe(b.keys.reload),
	}
}

func removeBookmark(ctx context.Context, src bookmarkSource, ref bookmarkRef) bookmarkDeletedMsg {
	if _, err := checkBookmark(ctx, src, ref, "you pressed d"); err != nil {
		return bookmarkDeletedMsg{name: ref.name, refresh: true, err: fmt.Errorf("%w; nothing deleted", err)}
	}
	warnings, err := src.DeleteBookmark(ctx, ref.name)
	if err != nil {
		return bookmarkDeletedMsg{name: ref.name, refresh: true, err: fmt.Errorf("delete bookmark %s: %w (%s)", ref.name, err, observe(ctx, src, ref.name))}
	}
	return bookmarkDeletedMsg{name: ref.name, warnings: warnings, refresh: true}
}

func checkBookmark(ctx context.Context, src bookmarkSource, ref bookmarkRef, since string) ([]jj.Bookmark, error) {
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

func observe(ctx context.Context, src bookmarkSource, names ...string) string {
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

func (b *bookmarksModel) applyDeleted(msg bookmarkDeletedMsg) bookmarkIntent {
	in := bookmarkIntent{status: "✓ deleted " + msg.name}
	if msg.err != nil {
		in.status = msg.err.Error()
	} else if msg.warnings != "" {
		in.status += " · " + strings.Join(strings.Fields(msg.warnings), " ")
	}
	if msg.refresh {
		b.want = ""
		in.refresh = true
	}
	return in
}

func (b bookmarksModel) view(env bookmarkEnv) string {
	p := env.pal
	inner := max(env.width-2, 0)
	var body []string
	var value []ui.Seg
	switch {
	case b.err != nil:
		body = []string{ui.Paint(inner, p.Card, []ui.Seg{ui.Span(ui.Fg(p.Removed), "  error: "+b.err.Error())}, nil)}
	case !b.loaded:
		body = []string{ui.Message(p, "loading…", inner)}
	case len(b.items) == 0:
		body = []string{ui.Message(p, "no bookmarks", inner)}
	default:
		value = []ui.Seg{ui.Span(ui.Fg(p.Title), fmt.Sprintf("bookmark %d/%d", b.cursor+1, len(b.items)))}
		names := 0
		for _, item := range b.items {
			names = max(names, min(lipgloss.Width(item.Name), bookmarkNameWidth))
		}
		for i := b.top; i < len(b.items) && len(body) < env.list; i++ {
			body = append(body, b.line(env, i, names, inner))
		}
	}
	return ui.Card(p, env.width, env.body, "Bookmarks", true, value, body)
}

func (b bookmarksModel) line(env bookmarkEnv, i, names, width int) string {
	p := env.pal
	item := b.items[i]
	bg := p.Card
	if i == b.cursor {
		bg = p.CursorBg
	}
	bar := bg
	if item.CommitID != "" && item.CommitID == env.head {
		bar = p.Selection
	}
	gap := ui.Span(lipgloss.NewStyle(), " ")
	pad := strings.Repeat(" ", max(names-lipgloss.Width(item.Name), 0)+1)
	left := []ui.Seg{ui.Span(ui.Fg(bar), ui.BarGlyph(bar)), ui.Pill(p.Chip, p.Key, item.Name), ui.Span(lipgloss.NewStyle(), pad)}
	right := clipSegs(bookmarkMarkers(p, item), width-ui.Width(left[:2])-1)
	if item.CommitID != "" {
		left = append(left,
			ui.Span(ui.Fg(p.Title), ui.ShortID(item.ChangeID)), gap,
			ui.Span(ui.Fg(p.Muted), ui.ShortID(item.CommitID)), gap,
			ui.Description(p, item.Description, false),
		)
	}
	return ui.Paint(width, bg, left, right)
}

func bookmarkMarkers(p theme.Theme, b jj.Bookmark) []ui.Seg {
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
