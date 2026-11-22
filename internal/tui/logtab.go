package tui

import (
	"slices"

	tea "charm.land/bubbletea/v2"
	"github.com/madalinpopa/jjreview/internal/jj"
	"github.com/madalinpopa/jjreview/internal/tui/tab"
)

func (m Model) LogCursor() string {
	return m.log.SelectedChange()
}

func (m *Model) showLog() tea.Cmd {
	m.view = logView
	return m.log.Show(m.env(logView))
}

func (m *Model) loadLog() tea.Cmd {
	return m.log.Load()
}

func (m *Model) logKey(msg tea.KeyPressMsg) tea.Cmd {
	env := m.env(logView)
	if m.log.Ignores(msg, env) {
		return nil
	}
	armed := m.armed
	m.armed = ""
	if !m.log.OverlayOpen() {
		m.status = ""
	}
	if !m.log.Captures() {
		if cmd, ok := m.globalKey(msg, armed); ok {
			return cmd
		}
	}
	cmd, fx := m.log.Key(msg, env)
	switch {
	case fx.Open != "":
		if !fx.Reopen {
			armed = ""
		}
		return m.openCommit(msg, armed, fx.Open)
	case fx.Rewrite != nil:
		return m.rewrite(*fx.Rewrite)
	case fx.Pick != nil:
		return m.bookmarks.Pick(*fx.Pick)
	case fx.Status != "":
		m.status = fx.Status
	}
	return m.logEffects(cmd, fx)
}

func (m *Model) logEffects(cmd tea.Cmd, fx tab.Effect) tea.Cmd {
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
