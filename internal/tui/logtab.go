package tui

import (
	"slices"

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
		Lock:    m.lock(),
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
	env := m.logEnv()
	if m.log.Ignores(msg, env) {
		return nil
	}
	armed := m.armed
	m.armed = ""
	if !m.log.OverlayOpen() {
		m.status = ""
	}
	if !m.log.Capturing() {
		if cmd, ok := m.globalKey(msg, armed); ok {
			return cmd
		}
	}
	cmd, fx := m.log.Key(msg, env)
	switch {
	case fx.Open != "":
		return m.openCommit(msg, armed, fx.Open)
	case fx.Rewrite != nil:
		return m.rewrite(*fx.Rewrite)
	case fx.Bookmark != nil:
		return m.bookmarks.Pick(*fx.Bookmark)
	case fx.Status != "":
		m.status = fx.Status
	}
	return m.logEffects(cmd, fx)
}

func (m *Model) logEffects(cmd tea.Cmd, fx logview.Effect) tea.Cmd {
	if fx.Retarget && slices.Contains(m.keys.open.Keys(), m.armed) {
		m.armed = ""
		m.status = ""
	}
	if fx.Refresh {
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
