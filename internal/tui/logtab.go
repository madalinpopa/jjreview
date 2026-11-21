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
		Theme:      m.pal,
		Width:      m.width,
		Height:     m.height,
		BodyHeight: m.bodyHeight(),
		ListHeight: m.diffHeight(),
		ModalWidth: m.modalWidth(),
		Head:       m.targetHead,
		Active:     m.view == logView,
		Dropped:    pickedUnits(m.picks()),
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
	switch {
	case in.Open != "":
		return m.openCommit(msg, armed, in.Open)
	case in.Arm:
		if m.canLoad() {
			m.status = m.log.Arm()
		}
	case in.Abandon != nil:
		if m.canLoad() {
			return m.abandon(*in.Abandon)
		}
	case in.Squash:
		if m.canLoad() {
			m.status = m.log.StartSquash()
		}
	case in.Squashed != nil:
		if m.canLoad() && m.confirmDrop(msg, armed) {
			m.log.FinishSquash()
			return m.squash(*in.Squashed)
		}
	case in.Status != "":
		m.status = in.Status
	}
	return m.logEffects(cmd, in)
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
