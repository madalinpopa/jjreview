package tui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/madalinpopa/jjreview/internal/diff"
	"github.com/madalinpopa/jjreview/internal/jj"
	"github.com/madalinpopa/jjreview/internal/tui/review"
)

const editorHoldTimeout = 300 * time.Millisecond

type editState int

const (
	editIdle editState = iota
	editChecking
	editRunning
	editReloading
)

type EditorFinishedMsg struct {
	Err error
}

type editCheckedMsg struct {
	path string
	err  error
}

type queryBackgroundMsg struct{}

type holdTimeoutMsg struct {
	seq int
}

type editorCommand struct {
	*exec.Cmd
	ctx context.Context
}

func (c editorCommand) SetStdin(r io.Reader) {
	if c.Stdin == nil {
		c.Stdin = r
	}
}

func (c editorCommand) SetStdout(w io.Writer) {
	if c.Stdout == nil {
		c.Stdout = w
	}
}

func (c editorCommand) SetStderr(w io.Writer) {
	if c.Stderr == nil {
		c.Stderr = w
	}
}

func (c editorCommand) Run() error {
	if err := c.ctx.Err(); err != nil {
		return err
	}
	if f, ok := c.Stdin.(interface{ Fd() uintptr }); ok {
		if err := discardInput(f.Fd()); err != nil {
			return fmt.Errorf("discard pending input: %w", err)
		}
	}
	return c.Cmd.Run()
}

func (m Model) WithEditor(editor func(ctx context.Context, path string) *exec.Cmd) Model {
	m.editor = editor
	return m
}

func (m *Model) editablePath() (string, bool) {
	file, ok := m.review.CurrentFile()
	if !m.loaded || m.reloading || !ok {
		return "", false
	}
	switch {
	case !m.change.WorkingCopy:
		m.status = "not the working copy; press @ to edit files"
	case file.Status == diff.StatusDeleted:
		m.status = "file deleted; nothing to edit"
	case m.editor == nil:
		m.status = "no editor configured"
	default:
		return filepath.Join(m.root, file.Path()), true
	}
	return "", false
}

func (m *Model) startEdit(path string) tea.Cmd {
	m.reloading = true
	m.edit = editChecking
	if !m.bgPending {
		return m.checkEdit(path)
	}
	m.held = m.checkEdit(path)
	m.holdSeq++
	seq := m.holdSeq
	return tea.Tick(editorHoldTimeout, func(time.Time) tea.Msg {
		return holdTimeoutMsg{seq: seq}
	})
}

func (m *Model) checkEdit(path string) tea.Cmd {
	ctx, src, rev := m.ctx, m.src, jj.WorkingCopyRevset(m.target)
	return func() tea.Msg {
		_, err := src.Change(ctx, rev)
		return editCheckedMsg{path: path, err: err}
	}
}

func (m *Model) editChecked(msg editCheckedMsg) tea.Cmd {
	switch {
	case m.quitting:
		return nil
	case errors.Is(msg.err, jj.ErrNoRevision):
		m.change.WorkingCopy = false
		m.status = movedStatus
	case msg.err != nil:
		m.status = "edit: " + msg.err.Error()
	default:
		return m.openEditor(msg.path)
	}
	m.reloading = false
	return m.endEdit()
}

func (m *Model) launch(path string) editorCommand {
	ctx, cancel := context.WithCancel(m.ctx)
	m.cancelEdit = cancel
	return editorCommand{Cmd: m.editor(ctx, path), ctx: ctx}
}

func (m *Model) openEditor(path string) tea.Cmd {
	m.edit = editRunning
	run := tea.Exec(m.launch(path), func(err error) tea.Msg {
		return EditorFinishedMsg{Err: err}
	})
	if m.autoTheme() {
		return tea.Sequence(tea.Raw(ansi.ResetModeLightDark), run)
	}
	return run
}

func (m *Model) requestBackground() tea.Cmd {
	if m.bgPending || m.edit == editChecking || m.edit == editRunning {
		m.bgDeferred = true
		return nil
	}
	m.bgPending, m.bgDeferred = true, false
	return tea.RequestBackgroundColor
}

func (m *Model) backgroundAnswered() tea.Cmd {
	m.bgPending = false
	if m.held != nil {
		return m.releaseEditor()
	}
	if m.bgDeferred {
		return m.requestBackground()
	}
	return nil
}

func (m *Model) holdTimedOut(msg holdTimeoutMsg) tea.Cmd {
	if msg.seq != m.holdSeq || m.held == nil {
		return nil
	}
	return m.releaseEditor()
}

func (m *Model) releaseEditor() tea.Cmd {
	run := m.held
	m.held = nil
	return run
}

func (m *Model) endEdit() tea.Cmd {
	m.edit = editIdle
	return m.handBack()
}

func (m *Model) handBack() tea.Cmd {
	if m.bgPending {
		m.bgPending, m.bgDeferred = false, true
	}
	if !m.bgDeferred {
		return nil
	}
	return m.requestBackground()
}

func (m *Model) editorFinished(msg EditorFinishedMsg) tea.Cmd {
	if m.edit != editRunning || m.quitting {
		return nil
	}
	m.edit = editReloading
	m.cancelEdit()
	var cmds []tea.Cmd
	if m.autoTheme() {
		m.bgDeferred = true
		cmds = append(cmds, tea.Raw(ansi.SetModeLightDark))
	}
	cmds = append(cmds, m.handBack())
	if msg.Err == nil {
		m.status = review.ReloadingStatus
		return tea.Batch(append(cmds, m.reload())...)
	}
	m.edit = editIdle
	m.reloading = false
	m.status = "editor: " + msg.Err.Error()
	return tea.Batch(cmds...)
}

func (m *Model) quit() tea.Cmd {
	m.quitting = true
	m.held = nil
	if m.cancelEdit != nil {
		m.cancelEdit()
	}
	return tea.Quit
}
