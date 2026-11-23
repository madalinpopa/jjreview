package tui

import (
	"slices"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/madalinpopa/jjreview/internal/tui/tab"
)

func (m *Model) reviewKey(msg tea.KeyPressMsg) tea.Cmd {
	m.status = ""
	armed := m.armed
	m.armed = ""
	if !m.review.Searching() {
		if m.dismissKey(msg) {
			return nil
		}
		if cmd, ok := m.globalKey(msg, armed); ok {
			m.review.CancelRange()
			return cmd
		}
	}
	cmd, eff := m.review.Key(msg, m.reviewEnv())
	switch {
	case eff.WorkingCopy:
		if m.canLoad() && m.confirmDrop(msg, armed) {
			return m.loadRevision(workingCopyStep, "@")
		}
	case eff.Reload:
		if m.canReload() && m.confirmDrop(msg, armed) {
			return m.reload()
		}
	case eff.Edit:
		if path, ok := m.editablePath(); ok && m.confirmDrop(msg, armed) {
			return m.startEdit(path)
		}
	default:
		return m.applyReview(cmd, eff)
	}
	return nil
}

func (m *Model) confirmDrop(msg tea.KeyPressMsg, armed string) bool {
	n := m.review.Picked()
	k := msg.String()
	if n == 0 || k == armed {
		return true
	}
	m.armed = k
	m.status = tab.DropPrompt(m.dropLabel(msg), n) + m.review.LineNote()
	return false
}

func (m Model) dropLabel(msg tea.KeyPressMsg) string {
	for _, b := range slices.Concat([]key.Binding{m.keys.quit}, m.review.DropKeys(), []key.Binding{m.keys.open}) {
		if key.Matches(msg, b) {
			return b.Help().Key
		}
	}
	return msg.String()
}
