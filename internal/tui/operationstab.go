package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/madalinpopa/jjreview/internal/tui/operations"
)

func (m Model) OperationCursor() string {
	return m.operations.SelectedID()
}

func (m Model) operationEnv() operations.Env {
	e := m.env(operationsView)
	return operations.Env{Layout: e.Layout, Dropped: e.Dropped, Active: e.Active, Lock: e.Lock}
}

func (m *Model) showOperations() tea.Cmd {
	m.view = operationsView
	return m.operations.Show()
}

func (m *Model) operationsKey(msg tea.KeyPressMsg) tea.Cmd {
	env := m.operationEnv()
	if m.operations.Ignores(msg, env) {
		return nil
	}
	armed := m.armed
	m.armed = ""
	if !m.operations.Captures() {
		m.status = ""
		if cmd, ok := m.globalKey(msg, armed); ok {
			return cmd
		}
	}
	return m.operationEffects(m.operations.Key(msg, env))
}

func (m *Model) operationEffects(cmd tea.Cmd, eff operations.Effect) tea.Cmd {
	switch {
	case eff.Rewrite != nil:
		return tea.Batch(cmd, m.rewrite(*eff.Rewrite))
	case eff.Status != "":
		m.status = eff.Status
	case eff.Refresh:
		return tea.Batch(cmd, m.invalidate())
	}
	return cmd
}
