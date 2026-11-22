package tui

import (
	"slices"

	tea "charm.land/bubbletea/v2"
	"github.com/madalinpopa/jjreview/internal/tui/tab"
)

func (m Model) BookmarkCursor() string {
	return m.bookmarks.SelectedName()
}

func (m Model) bookmarkEnv() tab.Env {
	return m.env(bookmarksView)
}

func (m Model) headTitle() string {
	if m.change.CommitID != m.targetHead {
		return ""
	}
	return m.change.Description
}

func (m *Model) showBookmarks() tea.Cmd {
	m.view = bookmarksView
	return m.bookmarks.Show(m.bookmarkEnv())
}

func (m *Model) markStale() {
	m.log.MarkStale()
	m.bookmarks.MarkStale()
	m.operations.MarkStale()
}

func (m *Model) invalidate() tea.Cmd {
	m.markStale()
	switch m.view {
	case logView:
		return m.loadLog()
	case bookmarksView:
		return m.bookmarks.Load()
	case operationsView:
		return m.operations.Load()
	}
	return nil
}

func (m *Model) bookmarksKey(msg tea.KeyPressMsg) tea.Cmd {
	m.status = ""
	armed := m.armed
	m.armed = ""
	if cmd, ok := m.globalKey(msg, armed); ok {
		return cmd
	}
	cmd, eff := m.bookmarks.Key(msg, m.bookmarkEnv())
	if eff.Open == "" {
		return m.bookmarkEffects(cmd, eff)
	}
	if !eff.Reopen {
		armed = ""
	}
	return m.openCommit(msg, armed, eff.Open)
}

func (m *Model) bookmarkEffects(cmd tea.Cmd, eff tab.Effect) tea.Cmd {
	if eff.Status != "" {
		m.status = eff.Status
	}
	if eff.Retarget && slices.Contains(m.keys.open.Keys(), m.armed) {
		m.armed = ""
		m.status = ""
	}
	if eff.Refresh {
		if m.view == logView {
			m.log.RestorePosition(m.log.HoldPosition().Targeting(m.log.SelectedChange()))
		}
		return tea.Batch(cmd, m.invalidate())
	}
	return cmd
}
