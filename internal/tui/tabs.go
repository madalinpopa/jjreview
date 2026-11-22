package tui

import (
	"slices"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/madalinpopa/jjreview/internal/jj"
	"github.com/madalinpopa/jjreview/internal/tui/tab"
)

type tabEntry struct {
	view   view
	label  string
	key    key.Binding
	intent globalIntent
	tab    tab.View
}

func (m *Model) registry() []tabEntry {
	return []tabEntry{
		{view: logView, label: "Log", key: m.keys.logTab, intent: showLogIntent, tab: &m.log},
		{view: bookmarksView, label: "Bookmarks", key: m.keys.bookmarksTab, intent: showBookmarksIntent, tab: &m.bookmarks},
		{view: operationsView, label: "Operations", key: m.keys.operationsTab, intent: showOperationsIntent, tab: &m.operations},
	}
}

func (m *Model) active() (tabEntry, bool) {
	for _, t := range m.registry() {
		if t.view == m.view {
			return t, true
		}
	}
	return tabEntry{}, false
}

func (m Model) LogCursor() string {
	return m.log.SelectedChange()
}

func (m Model) BookmarkCursor() string {
	return m.bookmarks.SelectedName()
}

func (m Model) OperationCursor() string {
	return m.operations.SelectedID()
}

func (m Model) headTitle() string {
	if m.change.CommitID != m.targetHead {
		return ""
	}
	return m.change.Description
}

func (m *Model) show(t tabEntry) tea.Cmd {
	m.view = t.view
	return t.tab.Show(m.env(t.view))
}

func (m *Model) markStale() {
	for _, t := range m.registry() {
		t.tab.MarkStale()
	}
}

func (m *Model) invalidate() tea.Cmd {
	m.markStale()
	if t, ok := m.active(); ok {
		return t.tab.Load()
	}
	return nil
}

func (m *Model) updateTabs(msg tea.Msg) tea.Cmd {
	var cmds []tea.Cmd
	for _, t := range m.registry() {
		if c, eff, ok := t.tab.Update(msg, m.env(t.view)); ok {
			cmds = append(cmds, m.applyEffect(t.view, c, eff))
		}
	}
	return tea.Batch(cmds...)
}

func (m *Model) tabKey(t tabEntry, msg tea.KeyPressMsg) tea.Cmd {
	env := m.env(t.view)
	if t.tab.Ignores(msg, env) {
		return nil
	}
	armed := m.armed
	m.armed = ""
	if !t.tab.OverlayOpen() {
		m.status = ""
	}
	if !t.tab.Captures() {
		if cmd, ok := m.globalKey(msg, armed); ok {
			return cmd
		}
	}
	cmd, eff := t.tab.Key(msg, env)
	if eff.Open != "" {
		if !eff.KeepArmed {
			armed = ""
		}
		return m.openCommit(msg, armed, eff.Open)
	}
	return m.applyEffect(t.view, cmd, eff)
}

func (m *Model) applyEffect(from view, cmd tea.Cmd, eff tab.Effect) tea.Cmd {
	switch {
	case eff.Rewrite != nil:
		return tea.Batch(cmd, m.rewrite(*eff.Rewrite))
	case eff.Pick != nil:
		return m.bookmarks.Pick(*eff.Pick)
	}
	if eff.Status != "" {
		m.status = eff.Status
	}
	if eff.Retarget && slices.Contains(m.keys.open.Keys(), m.armed) {
		m.armed = ""
		m.status = ""
	}
	if eff.Refresh {
		if from == bookmarksView && m.view == logView {
			m.log.RestorePosition(m.log.HoldPosition().Targeting(m.log.SelectedChange()))
		}
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
