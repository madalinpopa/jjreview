package tui

import (
	"slices"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/madalinpopa/jjreview/internal/jj"
	"github.com/madalinpopa/jjreview/internal/tui/logview"
)

func (m Model) LogCursor() string {
	return m.log.SelectedChange()
}

func (m Model) logEnv() logview.Env {
	return logview.Env{
		Layout:  m.layout(),
		Head:    m.targetHead,
		Active:  m.view == logView,
		Dropped: pickedUnits(m.picks()),
	}
}

func (m *Model) showLog() tea.Cmd {
	m.view = logView
	return m.log.Show(m.targetHead)
}

func (m *Model) loadLog() tea.Cmd {
	return m.log.Load()
}

func (m *Model) logKey(msg tea.KeyPressMsg) tea.Cmd {
	if m.log.Ignores(msg, m.reloading) {
		return nil
	}
	armed := m.armed
	m.armed = ""
	if !m.log.OverlayOpen() {
		m.status = ""
	}
	if !m.log.Capturing() {
		switch {
		case key.Matches(msg, m.keys.reviewTab):
			m.view = reviewView
			return nil
		case key.Matches(msg, m.keys.bookmarksTab):
			return m.showBookmarks()
		case key.Matches(msg, m.keys.operationsTab):
			return m.showOperations()
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
	cmd, in := m.log.Key(msg, m.logEnv())
	if c, handled := m.logIntent(msg, armed, in); handled {
		return c
	}
	return m.logEffects(cmd, in)
}

func (m *Model) logIntent(msg tea.KeyPressMsg, armed string, in logview.Intent) (tea.Cmd, bool) {
	switch {
	case in.Open != "":
		return m.openCommit(msg, armed, in.Open), true
	case in.Arm:
		if m.canLoad() {
			m.status = m.log.Arm()
		}
	case in.Abandon != nil:
		if m.canLoad() {
			return m.abandon(*in.Abandon), true
		}
	case in.ArmPush:
		if m.canLoad() {
			m.log.ArmPush()
		}
	case in.Push != nil:
		if m.canLoad() {
			return m.push(*in.Push), true
		}
	case in.Bookmark != nil:
		return m.bookmarks.Pick(*in.Bookmark), true
	case in.Fetch:
		if m.canLoad() {
			return m.fetchRemote(), true
		}
	case in.Squash:
		if m.canLoad() {
			m.status = m.log.StartSquash()
		}
	case in.Squashed != nil:
		if m.canLoad() && m.confirmDrop(msg, armed) {
			m.log.FinishSquash()
			return m.squash(*in.Squashed), true
		}
	case in.Status != "":
		m.status = in.Status
	}
	return nil, false
}

func (m *Model) logEffects(cmd tea.Cmd, in logview.Intent) tea.Cmd {
	if in.Retarget && slices.Contains(m.keys.open.Keys(), m.armed) {
		m.armed = ""
		m.status = ""
	}
	if in.Refresh {
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
