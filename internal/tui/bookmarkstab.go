package tui

import (
	"slices"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

func (m Model) BookmarkCursor() string {
	return m.bookmarks.selectedName()
}

func (m Model) bookmarkEnv() bookmarkEnv {
	return bookmarkEnv{
		pal:    m.pal,
		width:  m.width,
		height: m.height,
		body:   m.bodyHeight(),
		list:   m.diffHeight(),
		modal:  m.modalInner(),
		head:   m.targetHead,
		active: m.view == bookmarksView,
	}
}

func (m *Model) showBookmarks() tea.Cmd {
	m.view = bookmarksView
	return m.bookmarks.show()
}

func (m *Model) markStale() {
	m.log.stale = true
	m.bookmarks.markStale()
}

func (m *Model) invalidate() tea.Cmd {
	m.markStale()
	switch m.view {
	case logView:
		return m.loadLog()
	case bookmarksView:
		return m.bookmarks.load()
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
	case key.Matches(msg, m.keys.quit):
		if m.confirmDrop(msg, armed) {
			return m.quit()
		}
	case key.Matches(msg, m.keys.help):
		m.help = true
		m.helpTop = 0
	default:
		cmd, in := m.bookmarks.key(msg, m.bookmarkEnv())
		if in.open == "" {
			return m.bookmarkEffects(cmd, in)
		}
		if !in.sameTarget {
			armed = ""
		}
		return m.openCommit(msg, armed, in.open)
	}
	return nil
}

func (m *Model) bookmarkEffects(cmd tea.Cmd, in bookmarkIntent) tea.Cmd {
	if in.status != "" {
		m.status = in.status
	}
	if in.retarget && slices.Contains(m.keys.open.Keys(), m.armed) {
		m.armed = ""
		m.status = ""
	}
	if in.refresh {
		return tea.Batch(cmd, m.invalidate())
	}
	return cmd
}
