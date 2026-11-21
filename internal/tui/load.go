package tui

import (
	"context"
	"errors"
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/madalinpopa/jjreview/internal/diff"
	"github.com/madalinpopa/jjreview/internal/jj"
)

const movedStatus = "working copy moved; press @ to review it"

type loadedMsg struct {
	change jj.Change
	files  []diff.File
	root   string
	moved  bool
	err    error
}

type contentMsg struct {
	key     contentKey
	content string
	err     error
}

func load(ctx context.Context, src Source, rev string) tea.Cmd {
	return func() tea.Msg {
		return fetch(ctx, src, rev)
	}
}

func (m *Model) canReload() bool {
	if !m.canLoad() {
		return false
	}
	if m.err == nil && !m.change.WorkingCopy {
		m.status = "not the working copy; press @ to reload"
		return false
	}
	return true
}

func (m *Model) reload() tea.Cmd {
	if file, ok := m.currentFile(); ok {
		m.reloadPath = file.Path()
	}
	m.reloading = true
	m.log.stale = true
	rev := m.rev
	if m.target != "" {
		rev = jj.WorkingCopyRevset(m.target)
	}
	return load(m.ctx, m.src, rev)
}

func fetch(ctx context.Context, src Source, rev string) loadedMsg {
	change, err := src.Change(ctx, rev)
	if err != nil {
		return loadedMsg{err: fmt.Errorf("load change %s: %w", rev, err), moved: errors.Is(err, jj.ErrNoRevision)}
	}
	files, err := src.Diff(ctx, change.CommitID)
	if err != nil {
		return loadedMsg{err: fmt.Errorf("load diff %s: %w", rev, err)}
	}
	root, err := src.WorkspaceRoot(ctx)
	if err != nil {
		return loadedMsg{err: fmt.Errorf("find workspace root: %w", err)}
	}
	return loadedMsg{change: change, files: files, root: root}
}

func loadContent(ctx context.Context, src Source, key contentKey) tea.Cmd {
	return func() tea.Msg {
		content, err := src.FileContent(ctx, key.rev, key.path)
		if err != nil {
			err = fmt.Errorf("read %s: %w", key.path, err)
		}
		return contentMsg{key: key, content: content, err: err}
	}
}
