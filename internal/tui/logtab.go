package tui

import (
	"slices"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/madalinpopa/jjreview/internal/jj"
)

func (m Model) LogCursor() string {
	if e := m.log.selected(); e != nil {
		return e.ChangeID
	}
	return ""
}

func (m Model) logEnv() logEnv {
	return logEnv{
		pal:     m.pal,
		width:   m.width,
		height:  m.height,
		body:    m.bodyHeight(),
		list:    m.diffHeight(),
		modal:   m.modalInner(),
		head:    m.targetHead,
		active:  m.view == logView,
		dropped: pickedUnits(m.picks()),
	}
}

func (m *Model) showLog() tea.Cmd {
	m.view = logView
	return m.log.show(m.targetHead)
}

func (m *Model) loadLog() tea.Cmd {
	return m.log.load()
}

func (m *Model) logKey(msg tea.KeyPressMsg) tea.Cmd {
	if m.log.ignores(msg, m.reloading) {
		return nil
	}
	armed := m.armed
	m.armed = ""
	if !m.log.confirming() {
		m.status = ""
	}
	if !m.log.capturing() {
		switch {
		case key.Matches(msg, m.keys.reviewTab):
			m.view = reviewView
			return nil
		case key.Matches(msg, m.keys.bookmarksTab):
			return m.showBookmarks()
		case key.Matches(msg, m.keys.quit):
			if m.confirmDrop(msg, armed) {
				return m.quit()
			}
			return nil
		case key.Matches(msg, m.keys.help):
			m.help = true
			m.helpTop = 0
			return nil
		}
	}
	cmd, in := m.log.key(msg, m.logEnv())
	switch {
	case in.open != "":
		return m.openCommit(msg, armed, in.open)
	case in.arm:
		if m.canLoad() {
			m.status = m.log.arm()
		}
	case in.abandon != nil:
		if m.canLoad() {
			return m.abandon(*in.abandon)
		}
	}
	return m.logEffects(cmd, in)
}

func (m *Model) logEffects(cmd tea.Cmd, in logIntent) tea.Cmd {
	if in.retarget && slices.Contains(m.keys.open.Keys(), m.armed) {
		m.armed = ""
		m.status = ""
	}
	if in.refresh {
		return tea.Batch(cmd, m.invalidate())
	}
	return cmd
}

func (m *Model) openCommit(msg tea.KeyPressMsg, armed, commitID string) tea.Cmd {
	if commitID == m.targetHead && !m.outdated {
		m.view = reviewView
		if m.change.WorkingCopy {
			return m.checkWorkingCopy()
		}
		return nil
	}
	if !m.canLoad() || !m.confirmDrop(msg, armed) {
		return nil
	}
	return m.loadRevision(pickedStep, jj.VisibleCommitRevset(commitID))
}
