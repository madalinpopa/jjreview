package bookmarks

import (
	"context"
	"errors"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/madalinpopa/jjreview/internal/jj"
	"github.com/madalinpopa/jjreview/internal/theme"
	"github.com/madalinpopa/jjreview/internal/tui/tab"
	"github.com/madalinpopa/jjreview/internal/tui/ui"
)

const pickerChrome = 4

type picker struct {
	changeID string
	commitID string
	title    string
	cursor   int
	top      int
	moving   bool
	pending  *jj.Bookmark
}

type pickedMsg struct {
	bookmark jj.Bookmark
	change   string
	warnings string
	refused  bool
	err      error
}

func (b *Model) Pick(e jj.LogEntry) tea.Cmd {
	b.picker = &picker{changeID: e.ChangeID, commitID: e.CommitID, title: e.Description}
	return b.loadDue()
}

func (b *Model) pickKey(msg tea.KeyPressMsg, env tab.Env) (tea.Cmd, tab.Effect) {
	pk := b.picker
	pending := pk.pending
	pk.pending = nil
	last := len(b.items)
	switch it := b.keys.resolvePicker(msg); {
	case it == cancelIntent:
		b.picker = nil
	case pk.moving || !b.listed():
	case it == confirmIntent && pending != nil:
		return b.movePicked(pk, *pending, true), tab.Effect{}
	case it == moveDownIntent:
		pk.cursor = min(pk.cursor+1, last)
	case it == moveUpIntent:
		pk.cursor = max(pk.cursor-1, 0)
	case it == firstIntent:
		pk.cursor = 0
	case it == lastIntent:
		pk.cursor = last
	case it == openIntent:
		return b.choose(pk, env)
	}
	pk.top = ui.ScrollTop(pk.top, pk.cursor, b.pickerRoom(env), last+1)
	return nil, tab.Effect{}
}

func (b *Model) choose(pk *picker, env tab.Env) (tea.Cmd, tab.Effect) {
	if pk.cursor = min(pk.cursor, len(b.items)); pk.cursor == 0 {
		b.picker = nil
		env.Head.CommitID, env.Head.ChangeID, env.Head.Title = pk.commitID, pk.changeID, pk.title
		cmd := b.openModal(nil, env)
		b.modal.picked = true
		return cmd, tab.Effect{}
	}
	s := b.items[pk.cursor-1]
	switch {
	case s.Conflict:
		b.picker = nil
		return nil, tab.Effect{Status: s.Name + " is conflicted; resolve it with jj bookmark set"}
	case s.CommitID == pk.commitID:
		b.picker = nil
		return nil, tab.Effect{Status: s.Name + " is already on " + ui.ShortID(pk.changeID)}
	}
	return b.movePicked(pk, s, false), tab.Effect{}
}

func (b *Model) movePicked(pk *picker, s jj.Bookmark, allowBackwards bool) tea.Cmd {
	pk.moving = true
	ctx, src, change, to := b.ctx, b.src, pk.changeID, pk.commitID
	return func() tea.Msg {
		return moveBookmark(ctx, src, s, change, to, allowBackwards)
	}
}

func moveBookmark(ctx context.Context, src Source, s jj.Bookmark, change, to string, allowBackwards bool) pickedMsg {
	msg := pickedMsg{bookmark: s, change: change}
	ref := refOf(s)
	if _, err := checkBookmark(ctx, src, ref, "the list loaded"); err != nil {
		msg.err = fmt.Errorf("%w; nothing moved", err)
		return msg
	}
	warnings, err := src.MoveBookmark(ctx, s.Name, []string{s.CommitID}, to, allowBackwards)
	switch {
	case errors.Is(err, jj.ErrBookmarkBackwards):
		msg.refused = true
	case errors.Is(err, jj.ErrBookmarkChanged):
		msg.err = fmt.Errorf("%s changed since the list loaded (%s)", s.Name, observe(ctx, src, s.Name))
	case err != nil:
		msg.err = fmt.Errorf("move bookmark %s: %w (%s)", s.Name, err, observe(ctx, src, s.Name))
	default:
		msg.warnings = warnings
	}
	return msg
}

func (b *Model) applyPicked(msg pickedMsg) tab.Effect {
	pk := b.picker
	open := pk != nil && pk.moving
	name := msg.bookmark.Name
	if msg.refused {
		if !open {
			return tab.Effect{Status: name + " would move backwards or sideways; not moved"}
		}
		pk.moving = false
		pk.pending = new(msg.bookmark)
		return tab.Effect{}
	}
	if open {
		b.picker = nil
	}
	eff := tab.Effect{Status: "moved " + name + " to " + ui.ShortID(msg.change), Refresh: true}
	switch {
	case msg.err != nil:
		eff.Status, _, _ = strings.Cut(msg.err.Error(), "\n")
	case msg.warnings != "":
		eff.Status += " · " + strings.Join(strings.Fields(msg.warnings), " ")
	}
	return eff
}

func (b Model) pickerView(env tab.Env) string {
	p := env.Theme
	pk := b.picker
	inner := modalInner(env)
	lines := []string{ui.Paint(inner, p.Card, []ui.Seg{ui.Span(ui.Fg(p.Title).Bold(true), "Bookmark "+ui.ShortID(pk.changeID))}, nil)}
	switch {
	case !b.list.Loaded() || b.list.Loading():
		lines = append(lines, ui.Paint(inner, p.Card, []ui.Seg{ui.Span(ui.Fg(p.Muted).Italic(true), "loading…")}, nil))
	case b.list.Err() != nil:
		lines = append(lines, ui.Paint(inner, p.Card, []ui.Seg{ui.Span(ui.Fg(p.Removed), "error: "+b.list.Err().Error())}, nil))
	default:
		room := b.pickerRoom(env)
		top := ui.ScrollTop(pk.top, pk.cursor, room, len(b.items)+1)
		for i := top; i <= len(b.items) && i < top+room; i++ {
			lines = append(lines, b.pickerLine(p, i, inner))
		}
	}
	footer := []ui.Seg{ui.Span(ui.Fg(p.Key), "enter"), ui.Span(ui.Fg(p.Muted), " choose · "), ui.Span(ui.Fg(p.Key), "esc"), ui.Span(ui.Fg(p.Muted), " cancel")}
	note := ui.Fg(p.Removed)
	if pk.moving {
		note = ui.Fg(p.Status)
	}
	for _, line := range b.pickerNotes(inner) {
		lines = append(lines, ui.Paint(inner, p.Card, []ui.Seg{ui.Span(note, line)}, nil))
	}
	if pk.pending != nil {
		footer = []ui.Seg{ui.Span(ui.Fg(p.Key), "y"), ui.Span(ui.Fg(p.Muted), " move anyway · "), ui.Span(ui.Fg(p.Key), "esc"), ui.Span(ui.Fg(p.Muted), " cancel")}
	}
	lines = append(lines, ui.Paint(inner, p.Card, nil, footer))
	return ui.ModalBox(p, lines)
}

func (b Model) listed() bool {
	return b.list.Loaded() && !b.list.Loading() && b.list.Err() == nil
}

func (b Model) pickerNotes(width int) []string {
	switch pk := b.picker; {
	case pk == nil:
		return nil
	case pk.moving:
		return []string{"moving…"}
	case pk.pending != nil:
		return strings.Split(ansi.Wrap(pk.pending.Name+" would move backwards or sideways; press y to move anyway", width, ""), "\n")
	}
	return nil
}

func (b Model) pickerRoom(env tab.Env) int {
	return max(env.Height-pickerChrome-len(b.pickerNotes(modalInner(env))), 1)
}

func (b Model) pickerLine(p theme.Theme, i, width int) string {
	pk := b.picker
	bg := p.Card
	if i == pk.cursor {
		bg = p.CursorBg
	}
	if i == 0 {
		return ui.Paint(width, bg, []ui.Seg{ui.Span(ui.Fg(p.Added), "+ new bookmark…")}, nil)
	}
	s := b.items[i-1]
	var note ui.Seg
	switch {
	case s.Conflict:
		note = ui.Span(ui.Fg(p.Removed), "conflicted")
	case s.CommitID == pk.commitID:
		note = ui.Span(ui.Fg(p.Muted), "already here")
	default:
		note = ui.Span(ui.Fg(p.Muted), "move from "+ui.ShortID(s.ChangeID))
	}
	return ui.Paint(width, bg, []ui.Seg{ui.Span(lipgloss.NewStyle(), "  "), ui.Pill(p.Chip, p.Key, s.Name)}, []ui.Seg{note})
}
