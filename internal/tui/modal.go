package tui

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"strings"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/madalinpopa/jjreview/internal/diff"
	"github.com/madalinpopa/jjreview/internal/jj"
	"github.com/madalinpopa/jjreview/internal/theme"
	"github.com/madalinpopa/jjreview/internal/tui/ui"
)

const (
	modalMaxWidth   = 64
	inputHeight     = 5
	generatorHint   = "set provider in config.toml to generate"
	blockedStatus   = "split done; restart jjr to continue"
	immutableStatus = "immutable revision: read only"
	reloadingStatus = "reloading change…"
)

var errEmptyMessage = errors.New("commit message is empty")

type generatedMsg struct {
	seq  int
	text string
	err  error
}

type commitModal struct {
	ctx        context.Context
	cancel     context.CancelFunc
	genCancel  context.CancelFunc
	input      textarea.Model
	picks      []pick
	seq        int
	generating bool
	generated  bool
	genErr     error
	committing bool
	err        error
}

func (m *Model) openModal() tea.Cmd {
	if m.reloading {
		m.status = reloadingStatus
		return nil
	}
	if !m.ready() {
		return nil
	}
	if m.blocked {
		m.status = blockedStatus
		return nil
	}
	if m.outdated {
		m.status = outdatedStatus
		return nil
	}
	if m.change.Immutable {
		m.status = immutableStatus
		return nil
	}
	picks := m.picks()
	if len(picks) == 0 {
		m.status = "select hunks first (space / S)"
		return nil
	}
	if len(m.change.Parents) != 1 {
		m.status = fmt.Sprintf("cannot commit: %s has %d parents, want 1", ui.ShortID(m.target), len(m.change.Parents))
		return nil
	}
	ctx, cancel := context.WithCancel(m.ctx)
	m.modal = &commitModal{ctx: ctx, cancel: cancel, input: newInput(m.pal), picks: picks}
	return m.generate()
}

func (m *Model) closeModal() {
	m.modal.cancel()
	m.modal = nil
}

func (m *Model) generate() tea.Cmd {
	mo := m.modal
	if m.gen == nil {
		return nil
	}
	if mo.genCancel != nil {
		mo.genCancel()
	}
	genCtx, genCancel := context.WithCancel(mo.ctx)
	mo.genCancel = genCancel
	m.genSeq++
	mo.seq = m.genSeq
	mo.generating = true
	mo.generated = false
	mo.genErr = nil
	mo.input.Reset()
	mo.input.Placeholder = "Generating…"
	seq, gen, ctx := mo.seq, m.gen, genCtx
	patch := diff.Format(m.restricted(mo.picks))
	return func() tea.Msg {
		text, err := gen.Generate(ctx, patch)
		return generatedMsg{seq: seq, text: text, err: err}
	}
}

func (m *Model) applyGenerated(msg generatedMsg) {
	mo := m.modal
	if mo == nil || msg.seq != mo.seq {
		return
	}
	mo.generating = false
	mo.genCancel()
	mo.genCancel = nil
	mo.input.Placeholder = ""
	if msg.err != nil {
		mo.genErr = msg.err
		return
	}
	mo.generated = true
	mo.input.SetValue(strings.TrimSpace(msg.text))
}

func (m *Model) modalKey(msg tea.KeyPressMsg) tea.Cmd {
	mo := m.modal
	if mo.committing {
		return nil
	}
	switch msg.String() {
	case "esc":
		m.closeModal()
		return nil
	case "ctrl+r":
		return m.generate()
	case "ctrl+enter", "ctrl+s":
		if mo.generating {
			return nil
		}
		return m.commit()
	}
	return m.modalInput(msg)
}

func (m *Model) modalInput(msg tea.Msg) tea.Cmd {
	mo := m.modal
	if mo.committing || mo.generating {
		return nil
	}
	if paste, ok := msg.(tea.PasteMsg); ok {
		paste.Content = strings.NewReplacer("\r\n", "\n", "\r", "\n").Replace(paste.Content)
		msg = paste
	}
	var cmd tea.Cmd
	mo.input, cmd = mo.input.Update(msg)
	return cmd
}

func (m *Model) commit() tea.Cmd {
	mo := m.modal
	message := strings.TrimSpace(mo.input.Value())
	if message == "" {
		mo.err = errEmptyMessage
		return nil
	}
	mo.err = nil
	mo.committing = true
	return runCommit(mo.ctx, m.src, m.commitRequest(message, mo.picks))
}

func (m *Model) applyCommitted(msg committedMsg) {
	maps.Copy(m.contents, msg.fetched)
	mo := m.modal
	if mo == nil {
		return
	}
	mo.committing = false
	incomplete := errors.Is(msg.err, jj.ErrSplitIncomplete)
	if msg.err != nil && !incomplete {
		mo.err = msg.err
		return
	}
	m.markStale()
	id := ui.ShortID(msg.selected.ChangeID)
	before := m.rows()
	for _, p := range mo.picks {
		m.reviews[p.file].commit(p.units, id, !wholeOnly(m.files[p.file]))
	}
	m.keepCursor(before)
	if incomplete {
		m.blocked = true
		m.closeModal()
		m.status = blockedStatus
		return
	}
	subject, _, _ := strings.Cut(strings.TrimSpace(mo.input.Value()), "\n")
	m.target = msg.rest.ChangeID
	m.targetHead = msg.rest.CommitID
	m.closeModal()
	m.status = fmt.Sprintf("✓ created %s: %s", id, subject)
}

func (m *Model) resizeModal() {
	if m.modal == nil {
		return
	}
	in := &m.modal.input
	if w := boxInner(m.modalInner()); in.Width() != w {
		in.SetWidth(w)
	}
	if h := m.modalLayout().input; in.Height() != h {
		in.SetHeight(h)
	}
}

func (m Model) modalWidth() int {
	return min(modalMaxWidth, max(m.width-4, 20))
}

func newInput(p theme.Theme) textarea.Model {
	in := textarea.New()
	in.ShowLineNumbers = false
	in.Prompt = ""
	in.KeyMap.Paste.SetEnabled(false)
	in.SetHeight(inputHeight)
	in.SetStyles(inputStyles(p))
	in.Focus()
	return in
}

func inputStyles(p theme.Theme) textarea.Styles {
	base := lipgloss.NewStyle().Background(p.Input)
	state := textarea.StyleState{
		Base:        base,
		Text:        base.Foreground(p.Strong),
		CursorLine:  base.Foreground(p.Strong),
		EndOfBuffer: base.Foreground(p.Input),
		Placeholder: base.Foreground(p.Muted),
		Prompt:      base,
		Selection:   base.Reverse(true),
	}
	return textarea.Styles{
		Focused: state,
		Blurred: state,
		Cursor:  textarea.CursorStyle{Color: p.Selection, Shape: tea.CursorBar},
	}
}
