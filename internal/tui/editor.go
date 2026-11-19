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

func (m *Model) openEditor() tea.Cmd {
	file, ok := m.currentFile()
	if !m.loaded || !ok {
		return nil
	}
	switch {
	case file.Status == diff.StatusDeleted:
		m.status = "file deleted; nothing to edit"
		return nil
	case m.editor == nil:
		m.status = "no editor configured"
		return nil
	}
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
	if file, ok := m.currentFile(); ok {
		m.reloadPath = file.Path()
	}
	rev := m.target
	if rev == "" {
		rev = m.rev
	}
	return load(m.ctx, m.src, rev)
}
