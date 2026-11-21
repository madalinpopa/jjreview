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
	"github.com/madalinpopa/jjreview/internal/tui/ui"
)

type bookmarkState struct {
	items   []jj.Bookmark
	loaded  bool
	loading bool
	stale   bool
	err     error
	cursor  int
	top     int
	seq     int
	want    string
	armed   bookmarkRef
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

func (m Model) BookmarkCursor() string {
	return m.bookmarks.selectedName()
}

func (b bookmarkState) selected() *jj.Bookmark {
	if b.cursor < 0 || b.cursor >= len(b.items) {
		return nil
	}
	return &b.items[b.cursor]
}

func (b bookmarkState) selectedName() string {
	if s := b.selected(); s != nil {
		return s.Name
	}
	return ""
}

func (b bookmarkState) selectedRef() bookmarkRef {
	if s := b.selected(); s != nil {
		return refOf(*s)
	}
	return bookmarkRef{}
}

func (b *bookmarkState) move(delta int) {
	b.cursor = max(min(b.cursor+delta, len(b.items)-1), 0)
}

func (b *bookmarkState) keepVisible(height int) {
	b.top = ui.ScrollTop(b.top, b.cursor, height, len(b.items))
}

func (b *bookmarkState) scrollPage(delta, height int) {
	b.top = min(max(b.top+delta, 0), max(len(b.items)-height, 0))
	b.move(delta)
}

func (m *Model) showBookmarks() tea.Cmd {
	m.view = bookmarksView
	if !m.bookmarks.loading && (!m.bookmarks.loaded || m.bookmarks.stale) {
		return m.loadBookmarks()
	}
	return nil
}

func (m *Model) loadBookmarks() tea.Cmd {
	m.bookmarks.seq++
	m.bookmarks.loading = true
	m.bookmarks.stale = false
	seq, ctx, src := m.bookmarks.seq, m.ctx, m.src
	return func() tea.Msg {
		items, err := src.Bookmarks(ctx)
		if err != nil {
			err = fmt.Errorf("load bookmarks: %w", err)
		}
		return bookmarksMsg{seq: seq, items: items, err: err}
	}
}

func (m *Model) markStale() {
	m.log.stale = true
	m.bookmarks.stale = true
}

func (m *Model) invalidate() tea.Cmd {
	m.markStale()
	switch m.view {
	case logView:
		return m.loadLog()
	case bookmarksView:
		return m.loadBookmarks()
	}
	return nil
}

func (m *Model) refreshBookmarks(name string) tea.Cmd {
	m.bookmarks.want = name
	return m.invalidate()
}

func (m *Model) applyBookmarks(msg bookmarksMsg) tea.Cmd {
	if msg.seq != m.bookmarks.seq {
		return nil
	}
	b := &m.bookmarks
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
	if m.view == bookmarksView && b.selectedRef() != b.armed && slices.Contains(m.keys.open.Keys(), m.armed) {
		m.armed = ""
		m.status = ""
	}
	if b.stale && m.view == bookmarksView {
		return m.loadBookmarks()
	}
	return nil
}

func (m *Model) bookmarksKey(msg tea.KeyPressMsg) tea.Cmd {
	m.status = ""
	armed := m.armed
	m.armed = ""
	switch {
	case key.Matches(msg, m.keys.reviewTab):
		m.view = reviewView
	case key.Matches(msg, m.keys.logTab):
		return m.showLog()
	case key.Matches(msg, m.keys.quit):
		if m.confirmDrop(msg, armed) {
			return m.quit()
		}
	case key.Matches(msg, m.keys.down):
		m.bookmarks.move(1)
	case key.Matches(msg, m.keys.up):
		m.bookmarks.move(-1)
	case key.Matches(msg, m.keys.halfDown):
		m.bookmarks.scrollPage(m.halfPage(), m.diffHeight())
	case key.Matches(msg, m.keys.halfUp):
		m.bookmarks.scrollPage(-m.halfPage(), m.diffHeight())
	case key.Matches(msg, m.keys.top):
		m.bookmarks.move(-len(m.bookmarks.items))
	case key.Matches(msg, m.keys.bottom):
		m.bookmarks.move(len(m.bookmarks.items))
	case key.Matches(msg, m.keys.open):
		return m.openBookmark(msg, armed)
	case key.Matches(msg, m.keys.newBookmark):
		return m.openBookmarkModal(nil)
	case key.Matches(msg, m.keys.editBookmark):
		b := m.bookmarks.selected()
		switch {
		case b == nil:
		case b.Conflict:
			m.status = b.Name + " is conflicted; resolve it with jj bookmark set"
		default:
			return m.openBookmarkModal(b)
		}
	case key.Matches(msg, m.keys.deleteBookmark):
		m.confirmDelete()
	case key.Matches(msg, m.keys.reload):
		if !m.bookmarks.loading {
			return m.invalidate()
		}
	case key.Matches(msg, m.keys.help):
		m.help = true
		m.helpTop = 0
	}
	return nil
}

func (m *Model) openBookmark(msg tea.KeyPressMsg, armed string) tea.Cmd {
	b := m.bookmarks.selected()
	if b == nil {
		return nil
	}
	if b.CommitID == "" {
		m.status = b.Name + " is conflicted: it has no single target"
		return nil
	}
	ref := m.bookmarks.selectedRef()
	if ref != m.bookmarks.armed {
		armed = ""
	}
	cmd := m.openCommit(msg, armed, b.CommitID)
	m.bookmarks.armed = ref
	return cmd
}

func (m *Model) confirmDelete() {
	if b := m.bookmarks.selected(); b != nil {
		m.bmDelete = new(*b)
	}
}

func (m *Model) bookmarkDeleteKey(msg tea.KeyPressMsg) tea.Cmd {
	b := *m.bmDelete
	m.bmDelete = nil
	if msg.String() != "y" {
		return nil
	}
	ref, ctx, src := refOf(b), m.ctx, m.src
	return func() tea.Msg {
		return removeBookmark(ctx, src, ref)
	}
}

func (m Model) bookmarkDeleteView() string {
	p := m.pal
	b := m.bmDelete
	inner := m.modalInner()
	lines := []string{ui.Paint(inner, p.Card, []ui.Seg{ui.Span(ui.Fg(p.Title).Bold(true), "Delete bookmark")}, nil)}
	for line := range strings.SplitSeq(ansi.Hardwrap(ui.EscapePath(b.Name), inner, true), "\n") {
		lines = append(lines, ui.Paint(inner, p.Card, []ui.Seg{ui.Span(ui.Fg(p.Key).Bold(true), line)}, nil))
	}
	target := []ui.Seg{ui.Span(ui.Fg(p.Removed), "conflicted")}
	if !b.Conflict {
		gap := ui.Span(lipgloss.NewStyle(), " ")
		target = []ui.Seg{ui.Span(ui.Fg(p.Title), ui.ShortID(b.ChangeID)), gap, ui.Span(ui.Fg(p.Muted), ui.ShortID(b.CommitID)), gap, ui.Description(p, b.Description, false)}
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

func removeBookmark(ctx context.Context, src Source, ref bookmarkRef) bookmarkDeletedMsg {
	if _, err := checkBookmark(ctx, src, ref, "you pressed d"); err != nil {
		return bookmarkDeletedMsg{name: ref.name, refresh: true, err: fmt.Errorf("%w; nothing deleted", err)}
	}
	warnings, err := src.DeleteBookmark(ctx, ref.name)
	if err != nil {
		return bookmarkDeletedMsg{name: ref.name, refresh: true, err: fmt.Errorf("delete bookmark %s: %w (%s)", ref.name, err, observe(ctx, src, ref.name))}
	}
	return bookmarkDeletedMsg{name: ref.name, warnings: warnings, refresh: true}
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

func (m *Model) applyBookmarkDeleted(msg bookmarkDeletedMsg) tea.Cmd {
	m.status = "✓ deleted " + msg.name
	if msg.err != nil {
		m.status = msg.err.Error()
	} else if msg.warnings != "" {
		m.status += " · " + strings.Join(strings.Fields(msg.warnings), " ")
	}
	if !msg.refresh {
		return nil
	}
	return m.refreshBookmarks("")
}

func (m Model) bookmarksCard() string {
	p := m.pal
	inner := max(m.width-2, 0)
	var body []string
	var value []ui.Seg
	b := m.bookmarks
	switch {
	case b.err != nil:
		body = []string{ui.Paint(inner, p.Card, []ui.Seg{ui.Span(ui.Fg(p.Removed), "  error: "+b.err.Error())}, nil)}
	case !b.loaded:
		body = []string{ui.Message(m.pal, "loading…", inner)}
	case len(b.items) == 0:
		body = []string{ui.Message(m.pal, "no bookmarks", inner)}
	default:
		value = []ui.Seg{ui.Span(ui.Fg(p.Title), fmt.Sprintf("bookmark %d/%d", b.cursor+1, len(b.items)))}
		names := 0
		for _, item := range b.items {
			names = max(names, min(lipgloss.Width(item.Name), bookmarkNameWidth))
		}
		for i := b.top; i < len(b.items) && len(body) < m.diffHeight(); i++ {
			body = append(body, m.bookmarkLine(i, names, inner))
		}
	}
	return ui.Card(m.pal, m.width, m.bodyHeight(), "Bookmarks", true, value, body)
}

func (m Model) bookmarkLine(i, names, width int) string {
	p := m.pal
	b := m.bookmarks.items[i]
	bg := p.Card
	if i == m.bookmarks.cursor {
		bg = p.CursorBg
	}
	bar := bg
	if b.CommitID != "" && b.CommitID == m.targetHead {
		bar = p.Selection
	}
	gap := ui.Span(lipgloss.NewStyle(), " ")
	pad := strings.Repeat(" ", max(names-lipgloss.Width(b.Name), 0)+1)
	left := []ui.Seg{ui.Span(ui.Fg(bar), ui.BarGlyph(bar)), ui.Pill(p.Chip, p.Key, b.Name), ui.Span(lipgloss.NewStyle(), pad)}
	right := clipSegs(m.bookmarkMarkers(b), width-ui.Width(left[:2])-1)
	if b.CommitID != "" {
		left = append(left,
			ui.Span(ui.Fg(p.Title), ui.ShortID(b.ChangeID)), gap,
			ui.Span(ui.Fg(p.Muted), ui.ShortID(b.CommitID)), gap,
			ui.Description(p, b.Description, false),
		)
	}
	return ui.Paint(width, bg, left, right)
}

func (m Model) bookmarkMarkers(b jj.Bookmark) []ui.Seg {
	p := m.pal
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
