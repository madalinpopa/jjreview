package bookmarks

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/madalinpopa/jjreview/internal/jj"
	"github.com/madalinpopa/jjreview/internal/theme"
	"github.com/madalinpopa/jjreview/internal/tui/ui"
)

const (
	fullChrome    = 12
	compactChrome = 6
	minimalChrome = 4
	labelWidth    = 7
)

var (
	errEmptyBookmark = errors.New("bookmark name is empty")
	errEmptyTarget   = errors.New("target revision is empty")
)

type bookmarkMove struct {
	name string
	from string
	to   string
}

type dialog struct {
	editing    bool
	oldName    string
	nameEdited bool
	oldRef     bookmarkRef
	from       []string
	oldTarget  string
	inputs     [2]textinput.Model
	field      int
	pending    bookmarkMove
	saving     bool
	done       bool
	notice     string
	observed   string
	err        error
}

type bookmarkRequest struct {
	editing  bool
	oldName  string
	oldRef   bookmarkRef
	from     []string
	name     string
	target   string
	rename   bool
	move     bool
	approved bookmarkMove
}

type savedMsg struct {
	name     string
	observed string
	refresh  bool
	moved    string
	warnings string
	refused  bookmarkMove
	err      error
}

func newInput(value string) textinput.Model {
	in := textinput.New()
	in.Prompt = ""
	in.KeyMap.Paste.SetEnabled(false)
	styles := in.Styles()
	styles.Cursor.Blink = false
	in.SetStyles(styles)
	in.SetValue(value)
	return in
}

func (b *Model) openModal(s *jj.Bookmark, env Env) tea.Cmd {
	mo := &dialog{}
	if s == nil {
		target := ""
		if env.Head != "" {
			target = jj.CommitRevset(env.Head)
		}
		mo.inputs = [2]textinput.Model{newInput(""), newInput(target)}
	} else {
		mo.editing, mo.oldName, mo.oldRef, mo.from = true, s.Name, refOf(*s), []string{s.CommitID}
		if s.CommitID != "" {
			mo.oldTarget = jj.CommitRevset(s.CommitID)
		}
		mo.inputs = [2]textinput.Model{newInput(s.Name), newInput(mo.oldTarget)}
	}
	b.modal = mo
	b.Resize(env)
	return mo.inputs[0].Focus()
}

func (b *Model) modalKey(msg tea.KeyPressMsg) tea.Cmd {
	mo := b.modal
	if mo.saving {
		return nil
	}
	k := msg.String()
	if mo.done {
		if slices.Contains([]string{"esc", "enter", "ctrl+enter", "ctrl+s"}, k) {
			b.modal = nil
		}
		return nil
	}
	switch k {
	case "esc":
		b.modal = nil
		return nil
	case "tab", "shift+tab", "up", "down":
		mo.inputs[mo.field].Blur()
		mo.field = 1 - mo.field
		return mo.inputs[mo.field].Focus()
	case "enter", "ctrl+enter", "ctrl+s":
		return b.save()
	}
	return b.input(msg)
}

func (b *Model) Paste(msg tea.PasteMsg) tea.Cmd {
	return b.input(msg)
}

func (b *Model) input(msg tea.Msg) tea.Cmd {
	mo := b.modal
	if mo == nil || mo.saving || mo.done {
		return nil
	}
	in := &mo.inputs[mo.field]
	before := in.Value()
	var cmd tea.Cmd
	*in, cmd = in.Update(msg)
	if in.Value() != before {
		mo.pending = bookmarkMove{}
		mo.nameEdited = mo.nameEdited || mo.field == 0
	}
	return cmd
}

func (b *Model) save() tea.Cmd {
	mo := b.modal
	r := bookmarkRequest{
		editing:  mo.editing,
		oldName:  mo.oldName,
		oldRef:   mo.oldRef,
		from:     mo.from,
		name:     mo.inputs[0].Value(),
		target:   strings.TrimSpace(mo.inputs[1].Value()),
		approved: mo.pending,
	}
	if mo.editing && !mo.nameEdited {
		r.name = mo.oldName
	}
	r.rename = mo.editing && r.name != mo.oldName
	r.move = !mo.editing || r.target != mo.oldTarget
	switch {
	case strings.TrimSpace(r.name) == "":
		mo.err = errEmptyBookmark
		return nil
	case r.move && r.target == "":
		mo.err = errEmptyTarget
		return nil
	case !r.rename && !r.move:
		b.modal = nil
		return nil
	}
	mo.err = nil
	mo.notice = ""
	mo.saving = true
	ctx, src := b.ctx, b.src
	return func() tea.Msg {
		return storeBookmark(ctx, src, r)
	}
}

func resolveTarget(ctx context.Context, src Source, target string) (string, error) {
	c, err := src.Change(ctx, jj.VisibleRevset(target))
	if errors.Is(err, jj.ErrNoRevision) {
		return "", fmt.Errorf("%s is no longer visible; press R in the review or choose another revision", target)
	}
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", target, err)
	}
	return c.CommitID, nil
}

func storeBookmark(ctx context.Context, src Source, r bookmarkRequest) savedMsg {
	var to string
	if r.move {
		var err error
		if to, err = resolveTarget(ctx, src, r.target); err != nil {
			return savedMsg{err: err}
		}
	}
	if !r.editing {
		warnings, err := src.CreateBookmark(ctx, r.name, jj.CommitRevset(to))
		if err != nil {
			return savedMsg{name: r.name, refresh: true, observed: observe(ctx, src, r.name), err: err}
		}
		return savedMsg{name: r.name, refresh: true, warnings: warnings}
	}
	items, err := checkBookmark(ctx, src, r.oldRef, "the dialog opened")
	if err != nil {
		return savedMsg{refresh: true, err: err}
	}
	if r.rename && slices.ContainsFunc(items, func(b jj.Bookmark) bool { return b.Name == r.name }) {
		return savedMsg{err: fmt.Errorf("%s already exists", r.name)}
	}
	var warnings []string
	var moved string
	if r.move && to != r.oldRef.target {
		mv := bookmarkMove{name: r.oldName, from: r.oldRef.target, to: to}
		w, err := src.MoveBookmark(ctx, r.oldName, r.from, to, mv == r.approved)
		switch {
		case errors.Is(err, jj.ErrBookmarkBackwards):
			return savedMsg{refused: mv}
		case errors.Is(err, jj.ErrBookmarkChanged):
			return savedMsg{refresh: true, observed: observe(ctx, src, r.oldName), err: fmt.Errorf("%s changed since the dialog opened", r.oldName)}
		case err != nil:
			return savedMsg{refresh: true, observed: observe(ctx, src, r.oldName), err: err}
		}
		warnings = append(warnings, w)
		moved = to
	}
	if r.rename {
		w, err := src.RenameBookmark(ctx, r.oldName, r.name)
		if err != nil {
			observed := observe(ctx, src, r.oldName, r.name)
			if moved != "" {
				err = fmt.Errorf("moved %s to %s, but the rename failed: %w", r.oldName, ui.ShortID(moved), err)
			}
			return savedMsg{name: r.oldName, refresh: true, moved: moved, observed: observed, warnings: joinWarnings(warnings), err: err}
		}
		warnings = append(warnings, w)
	}
	return savedMsg{name: r.name, refresh: true, warnings: joinWarnings(warnings)}
}

func joinWarnings(warnings []string) string {
	return strings.Join(slices.DeleteFunc(warnings, func(w string) bool { return w == "" }), "\n")
}

func (b *Model) applySaved(msg savedMsg) Intent {
	var in Intent
	if msg.refresh {
		b.want = msg.name
		in.Refresh = true
	}
	refused := msg.refused != bookmarkMove{}
	if msg.err == nil && !refused {
		in.Status = "✓ saved " + msg.name
	}
	mo := b.modal
	if mo == nil {
		return in
	}
	mo.saving = false
	mo.err = msg.err
	mo.pending = msg.refused
	mo.notice = msg.warnings
	mo.observed = msg.observed
	switch {
	case msg.moved != "":
		mo.oldRef = bookmarkRef{name: mo.oldName, target: msg.moved}
		mo.from = []string{msg.moved}
		mo.oldTarget = jj.CommitRevset(msg.moved)
		mo.inputs[1].SetValue(mo.oldTarget)
	case msg.err != nil || refused:
	case msg.warnings != "":
		mo.done = true
	default:
		b.modal = nil
	}
	return in
}

func (b *Model) Resize(env Env) {
	if b.modal == nil {
		return
	}
	inner := env.modalInner()
	w := inner - labelWidth
	if chromeRows(env.Height, len(b.noteLines(env.Theme, inner))) == fullChrome {
		w = ui.InputBoxInner(inner)
	}
	w = max(w-1, 1)
	for i := range b.modal.inputs {
		in := &b.modal.inputs[i]
		if in.Width() == w {
			continue
		}
		value, pos := in.Value(), in.Position()
		in.SetWidth(w)
		in.SetValue("")
		in.SetValue(value)
		in.SetCursor(pos)
	}
}

func inputStyles(p theme.Theme) textinput.Styles {
	base := lipgloss.NewStyle().Background(p.Input)
	state := textinput.StyleState{
		Text:        base.Foreground(p.Strong),
		Placeholder: base.Foreground(p.Muted),
		Suggestion:  base.Foreground(p.Muted),
		Prompt:      base,
	}
	return textinput.Styles{
		Focused: state,
		Blurred: state,
		Cursor:  textinput.CursorStyle{Color: p.Selection, Shape: tea.CursorBar},
	}
}

func chromeRows(height, notes int) int {
	for _, c := range []int{fullChrome, compactChrome} {
		if height >= c+min(notes, 1) {
			return c
		}
	}
	return minimalChrome
}

func (b Model) modalView(env Env) string {
	p := env.Theme
	inner := env.modalInner()
	notes := b.noteLines(env.Theme, inner)
	chrome := chromeRows(env.Height, len(notes))
	if room := max(env.Height-chrome, 1); len(notes) > room {
		notes = notes[:room]
		notes[room-1].Text = ansi.Truncate(notes[room-1].Text, inner-1, "") + "…"
	}
	var lines []string
	switch chrome {
	case minimalChrome:
		lines = append(lines, b.fields(p, inner, chrome)[b.modal.field])
	default:
		lines = append(lines, ui.Paint(inner, p.Card, b.title(p), nil))
		lines = append(lines, b.fields(p, inner, chrome)...)
	}
	for _, note := range notes {
		lines = append(lines, ui.Paint(inner, p.Card, []ui.Seg{note}, nil))
	}
	lines = append(lines, ui.Paint(inner, p.Card, nil, b.footer(p)))
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(p.ModalBorder).
		BorderBackground(p.Card).
		Background(p.Card).
		Padding(0, 1).
		Render(strings.Join(lines, "\n"))
}

func (b Model) title(p theme.Theme) []ui.Seg {
	mo := b.modal
	if mo.editing {
		return []ui.Seg{ui.Span(ui.Fg(p.Title).Bold(true), "Edit bookmark "), ui.Pill(p.Chip, p.ChipText, mo.oldName)}
	}
	return []ui.Seg{ui.Span(ui.Fg(p.Title).Bold(true), "New bookmark")}
}

func (b Model) fields(p theme.Theme, inner, chrome int) []string {
	mo := b.modal
	var lines []string
	for i, label := range []string{"Name", "Target revision"} {
		in := mo.inputs[i]
		in.SetStyles(inputStyles(p))
		focus := ui.Fg(p.Header)
		border := p.Border
		if i == mo.field {
			focus, border = ui.Fg(p.Selection), p.Selection
		}
		if chrome != fullChrome {
			short, _, _ := strings.Cut(label, " ")
			lines = append(lines, ui.Paint(inner, p.Input, []ui.Seg{ui.Span(focus, fmt.Sprintf("%-*s", labelWidth, short)), ui.Span(lipgloss.NewStyle(), in.View())}, nil))
			continue
		}
		lines = append(lines,
			ui.Paint(inner, p.Card, []ui.Seg{ui.Span(ui.Fg(p.Header), label)}, nil),
			ui.InputBox(p, inner, border, in.View()),
		)
	}
	return lines
}

func (b Model) noteLines(p theme.Theme, width int) []ui.Seg {
	var lines []ui.Seg
	for _, note := range b.notes(p) {
		for text := range strings.SplitSeq(ansi.Wrap(note.Text, width, ""), "\n") {
			lines = append(lines, ui.Span(note.Style, text))
		}
	}
	return lines
}

func (b Model) notes(p theme.Theme) []ui.Seg {
	mo := b.modal
	style := ui.Fg(p.Removed)
	var text string
	switch {
	case mo.saving:
		return []ui.Seg{ui.Span(ui.Fg(p.Status), "saving…")}
	case mo.done:
		text, style = mo.notice, ui.Fg(p.Status)
	case mo.pending != bookmarkMove{}:
		from := "its conflicted targets"
		if !strings.HasPrefix(mo.pending.from, "conflict ") {
			from = ui.ShortID(mo.pending.from)
		}
		text = fmt.Sprintf("%s would move backwards or sideways (%s → %s); press enter again to allow it", mo.pending.name, from, ui.ShortID(mo.pending.to))
	case mo.err != nil:
		text = strings.Join(slices.DeleteFunc([]string{mo.observed, "error: " + mo.err.Error(), mo.notice}, func(s string) bool { return s == "" }), "\n")
	default:
		return nil
	}
	var notes []ui.Seg
	for line := range strings.Lines(text) {
		notes = append(notes, ui.Span(style, strings.TrimRight(line, "\n")))
	}
	return notes
}

func (b Model) footer(p theme.Theme) []ui.Seg {
	key := ui.Fg(p.Key)
	text := ui.Fg(p.Muted)
	mo := b.modal
	switch {
	case mo.done:
		return []ui.Seg{ui.Span(ui.Fg(p.Status), "✓ saved · "), ui.Span(key, "enter"), ui.Span(text, " / "), ui.Span(key, "esc"), ui.Span(text, " close")}
	case mo.pending != bookmarkMove{}:
		return []ui.Seg{ui.Span(key, "enter"), ui.Span(text, " move anyway · "), ui.Span(key, "esc"), ui.Span(text, " cancel")}
	}
	return []ui.Seg{
		ui.Span(key, "tab"), ui.Span(text, " next field · "),
		ui.Span(key, "enter"), ui.Span(text, " save · "),
		ui.Span(key, "esc"), ui.Span(text, " cancel"),
	}
}
