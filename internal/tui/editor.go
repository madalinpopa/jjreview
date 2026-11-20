package tui

import (
	"context"
	"os/exec"
	"path/filepath"

	tea "charm.land/bubbletea/v2"
	"github.com/madalinpopa/jjreview/internal/diff"
)

type EditorFinishedMsg struct {
	Err error
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

func (m *Model) openEditor() tea.Cmd {
	file, _ := m.currentFile()
	cmd := m.editor(m.ctx, filepath.Join(m.root, file.Path()))
	m.reloading = true
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return EditorFinishedMsg{Err: err}
	})
}

func (m *Model) editorFinished(msg EditorFinishedMsg) tea.Cmd {
	if msg.Err != nil {
		m.reloading = false
		m.status = "editor: " + msg.Err.Error()
		return nil
	}
	return m.reload()
}
