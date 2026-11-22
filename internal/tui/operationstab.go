package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/madalinpopa/jjreview/internal/tui/tab"
)

func (m Model) OperationCursor() string {
	return m.operations.SelectedID()
}

func (m Model) operationEnv() tab.Env {
	return m.env(operationsView)
}

func (m *Model) showOperations() tea.Cmd {
	m.view = operationsView
	return m.operations.Show(m.operationEnv())
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

func (m *Model) operationEffects(cmd tea.Cmd, eff tab.Effect) tea.Cmd {
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
