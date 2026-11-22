package tui

import (
	"slices"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/madalinpopa/jjreview/internal/tui/bookmarks"
)

func (m Model) BookmarkCursor() string {
	return m.bookmarks.SelectedName()
}

func (m Model) bookmarkEnv() bookmarks.Env {
	return bookmarks.Env{
		Layout:     m.layout(),
		Head:       m.targetHead,
		HeadChange: m.target,
		HeadTitle:  m.headTitle(),
		Active:     m.view == bookmarksView,
	}
}

func (m Model) headTitle() string {
	if m.change.CommitID != m.targetHead {
		return ""
	}
	return m.change.Description
}

func (m *Model) showBookmarks() tea.Cmd {
	m.view = bookmarksView
	return m.bookmarks.Show()
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
	switch {
	case key.Matches(msg, m.keys.reviewTab):
		m.view = reviewView
	case key.Matches(msg, m.keys.logTab):
		return m.showLog()
	case key.Matches(msg, m.keys.operationsTab):
		return m.showOperations()
	case key.Matches(msg, m.keys.quit):
		if m.confirmDrop(msg, armed) {
			return m.quit()
		}
	case key.Matches(msg, m.keys.help):
		m.help = true
		m.helpTop = 0
	default:
		cmd, in := m.bookmarks.Key(msg, m.bookmarkEnv())
		if in.Open == "" {
			return m.bookmarkEffects(cmd, in)
		}
		if !in.Reopen {
			armed = ""
		}
		return m.openCommit(msg, armed, in.Open)
	}
	return nil
}

func (m *Model) bookmarkEffects(cmd tea.Cmd, in bookmarks.Intent) tea.Cmd {
	if in.Status != "" {
		m.status = in.Status
	}
	if in.Retarget && slices.Contains(m.keys.open.Keys(), m.armed) {
		m.armed = ""
		m.status = ""
	}
	if in.Refresh {
		if m.view == logView {
			m.log.RestorePosition(m.log.HoldPosition().Targeting(m.log.SelectedChange()))
		}
		return tea.Batch(cmd, m.invalidate())
	}
	return cmd
}
