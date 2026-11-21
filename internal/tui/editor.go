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
	path  string
	bgGen int
	err   error
}

type queryBackgroundMsg struct{}

type holdTimeoutMsg struct {
	seq int
}

func (m Model) WithEditor(editor func(ctx context.Context, path string) *exec.Cmd) Model {
	m.editor = editor
	return m
}

func (m *Model) editablePath() (string, bool) {
	file, ok := m.currentFile()
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
	if m.bgPending == 0 {
		return m.checkEdit(path)
	}
	return m.hold(m.checkEdit(path))
}

func (m *Model) checkEdit(path string) tea.Cmd {
	ctx, src, rev, gen := m.ctx, m.src, jj.WorkingCopyRevset(m.target), m.bgGen
	return func() tea.Msg {
		_, err := src.Change(ctx, rev)
		return editCheckedMsg{path: path, bgGen: gen, err: err}
	}
}

func (m *Model) editChecked(msg editCheckedMsg) tea.Cmd {
	switch {
	case errors.Is(msg.err, jj.ErrNoRevision):
		m.reloading = false
		m.change.WorkingCopy = false
		m.status = movedStatus
	case msg.err != nil:
		m.reloading = false
		m.status = "edit: " + msg.err.Error()
	case m.bgPending > 0:
		return m.hold(m.checkEdit(msg.path))
	case msg.bgGen != m.bgGen:
		return m.checkEdit(msg.path)
	default:
		return m.openEditor(msg.path)
	}
	return nil
}

func (m *Model) openEditor(path string) tea.Cmd {
	m.editing = true
	run := tea.ExecProcess(m.editor(m.ctx, path), func(err error) tea.Msg {
		return EditorFinishedMsg{Err: err}
	})
	if m.autoTheme() {
		return tea.Sequence(tea.Raw(ansi.ResetModeLightDark), run)
	}
	return run
}

func (m *Model) hold(cmd tea.Cmd) tea.Cmd {
	m.held = cmd
	m.holdSeq++
	seq := m.holdSeq
	return tea.Tick(editorHoldTimeout, func(time.Time) tea.Msg {
		return holdTimeoutMsg{seq: seq}
	})
}

func (m *Model) requestBackground() tea.Cmd {
	if m.editing {
		m.bgDeferred = true
		return nil
	}
	m.bgGen++
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
	deferred := m.bgDeferred
	m.editing, m.bgDeferred = false, false
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
	if deferred {
		return tea.Batch(cmd, m.requestBackground())
	}
	return cmd
}
