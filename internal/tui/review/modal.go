package review

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
	inputHeight     = 5
	generatorHint   = "set provider in config.toml to generate"
	immutableStatus = "immutable revision: read only"
	BlockedStatus   = "split done; restart jjr to continue"
	ReloadingStatus = "reloading change…"
	OutdatedStatus  = "review out of date; press @ to review the working copy"

	lineKeyStatus        = "%s selects a changed line; %s selects the hunk"
	rangeStatus          = "selecting a range"
	fitsStatus           = "every line fits the pane"
	rangeEmptyStatus     = "no changed lines to select in the range"
	lineDoneStatus       = "line already committed"
	wholeFileStatus      = "this file can only be selected whole; press "
	partialRemovedStatus = "removed lines stay visible in a partly selected hunk"
	noSelectionStatus    = "select hunks first (%s / %s), or lines (%s / %s)"
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

func (m *Model) openModal(env Env) (tea.Cmd, string) {
	switch {
	case env.Reloading:
		return nil, ReloadingStatus
	case !m.ready(env):
		return nil, ""
	case env.Blocked:
		return nil, BlockedStatus
	case env.Outdated:
		return nil, OutdatedStatus
	case env.Change.Immutable:
		return nil, immutableStatus
	}
	picks := m.picks()
	if len(picks) == 0 {
		k := m.keys
		return nil, fmt.Sprintf(noSelectionStatus, k.selectHunk.Help().Key, k.selectFile.Help().Key, k.selectLine.Help().Key, k.selectRange.Help().Key)
	}
	if len(env.Change.Parents) != 1 {
		return nil, fmt.Sprintf("cannot commit: %s has %d parents, want 1", ui.ShortID(env.Head.ChangeID), len(env.Change.Parents))
	}
	ctx, cancel := context.WithCancel(m.ctx)
	m.modal = &commitModal{ctx: ctx, cancel: cancel, input: newInput(env.Theme), picks: picks}
	return m.generate(), ""
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
	files, err := m.restricted(mo.picks)
	if err != nil {
		mo.genErr = err
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
	patch := diff.Format(files)
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

func (m *Model) modalKey(msg tea.KeyPressMsg, env Env) tea.Cmd {
	mo := m.modal
	if mo.committing {
		return nil
	}
	switch resolveCommitDialog(msg) {
	case cancelCommitIntent:
		m.closeModal()
		return nil
	case regenerateMessageIntent:
		return m.generate()
	case submitCommitIntent:
		if mo.generating {
			return nil
		}
		return m.commit(env)
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

func (m *Model) commit(env Env) tea.Cmd {
	mo := m.modal
	message := strings.TrimSpace(mo.input.Value())
	if message == "" {
		mo.err = errEmptyMessage
		return nil
	}
	mo.err = nil
	mo.committing = true
	return runCommit(mo.ctx, m.src, m.commitRequest(env, message, mo.picks))
}

func (m *Model) applyCommitted(env Env, msg committedMsg) Effect {
	maps.Copy(m.contents, msg.fetched)
	mo := m.modal
	if mo == nil {
		return Effect{}
	}
	mo.committing = false
	incomplete := errors.Is(msg.err, jj.ErrSplitIncomplete)
	if msg.err != nil && !incomplete {
		mo.err = msg.err
		return Effect{}
	}
	id := ui.ShortID(msg.selected.ChangeID)
	before := m.rows(env)
	for _, p := range mo.picks {
		m.reviews[p.file].commit(p.units, id, !wholeOnly(m.files[p.file]))
	}
	m.keepCursor(env, before)
	m.closeModal()
	eff := Effect{Committed: &Committed{Selected: msg.selected, Rest: msg.rest, Incomplete: incomplete}}
	if incomplete {
		eff.Status = BlockedStatus
		return eff
	}
	subject, _, _ := strings.Cut(strings.TrimSpace(mo.input.Value()), "\n")
	eff.Status = fmt.Sprintf("✓ created %s: %s", id, subject)
	return eff
}

func (m *Model) resizeModal(env Env) {
	if m.modal == nil {
		return
	}
	in := &m.modal.input
	if w := ui.InputBoxInner(modalInner(env)); in.Width() != w {
		in.SetWidth(w)
	}
	if h := m.modalLayout(env).input; in.Height() != h {
		in.SetHeight(h)
	}
}

func (m *Model) Paste(msg tea.PasteMsg) tea.Cmd {
	if m.modal == nil {
		return nil
	}
	return m.modalInput(msg)
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

type commitDialogIntent int

const (
	typeMessageIntent commitDialogIntent = iota
	cancelCommitIntent
	regenerateMessageIntent
	submitCommitIntent
)

func resolveCommitDialog(msg tea.KeyPressMsg) commitDialogIntent {
	switch msg.String() {
	case "esc":
		return cancelCommitIntent
	case "ctrl+r":
		return regenerateMessageIntent
	case "ctrl+enter", "ctrl+s":
		return submitCommitIntent
	}
	return typeMessageIntent
}
