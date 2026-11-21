package tui

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/madalinpopa/jjreview/internal/diff"
	"github.com/madalinpopa/jjreview/internal/jj"
)

const editorHoldTimeout = 300 * time.Millisecond

type EditorFinishedMsg struct {
	Err error
}

type editCheckedMsg struct {
	err error
}

type queryBackgroundMsg struct{}

type holdTimeoutMsg struct {
	seq int
}

func (m Model) WithEditor(editor func(ctx context.Context, path string) *exec.Cmd) Model {
	m.editor = editor
	return m
}

func (m *Model) canEdit() bool {
	file, ok := m.currentFile()
	if !m.loaded || m.reloading || !ok {
		return false
	}
	switch {
	case !m.change.WorkingCopy:
		m.status = "not the working copy; press @ to edit files"
	case file.Status == diff.StatusDeleted:
		m.status = "file deleted; nothing to edit"
	case m.editor == nil:
		m.status = "no editor configured"
	default:
		return true
	}
	return false
}

func (m *Model) checkEdit() tea.Cmd {
	m.reloading = true
	ctx, src, rev := m.ctx, m.src, jj.WorkingCopyRevset(m.target)
	return func() tea.Msg {
		_, err := src.Change(ctx, rev)
		return editCheckedMsg{err: err}
	}
}

func (m *Model) editChecked(msg editCheckedMsg) tea.Cmd {
	m.reloading = false
	switch {
	case errors.Is(msg.err, jj.ErrNoRevision):
		m.change.WorkingCopy = false
		m.status = movedStatus
	case msg.err != nil:
		m.status = "edit: " + msg.err.Error()
	default:
		return m.openEditor()
	}
	return nil
}

func (m *Model) openEditor() tea.Cmd {
	file, _ := m.currentFile()
	cmd := m.editor(m.ctx, filepath.Join(m.root, file.Path()))
	m.reloading = true
	run := tea.ExecProcess(cmd, func(err error) tea.Msg {
		return EditorFinishedMsg{Err: err}
	})
	if m.autoTheme() {
		run = tea.Sequence(tea.Raw(ansi.ResetModeLightDark), run)
	}
	if m.bgPending == 0 {
		return run
	}
	m.held = run
	m.holdSeq++
	seq := m.holdSeq
	return tea.Tick(editorHoldTimeout, func(time.Time) tea.Msg {
		return holdTimeoutMsg{seq: seq}
	})
}

func (m *Model) requestBackground() tea.Cmd {
	m.bgPending++
	return tea.RequestBackgroundColor
}

func (m *Model) backgroundAnswered() tea.Cmd {
	m.bgPending = max(m.bgPending-1, 0)
	if m.bgPending > 0 {
		return nil
	}
	return m.releaseEditor()
}

func (m *Model) releaseEditor() tea.Cmd {
	run := m.held
	m.held = nil
	return run
}

func (m *Model) editorFinished(msg EditorFinishedMsg) tea.Cmd {
	var cmd tea.Cmd
	if msg.Err != nil {
		m.reloading = false
		m.status = "editor: " + msg.Err.Error()
	} else {
		cmd = m.reload()
	}
	if m.autoTheme() {
		return tea.Batch(cmd, tea.Raw(ansi.SetModeLightDark), m.requestBackground())
	}
	return cmd
}
