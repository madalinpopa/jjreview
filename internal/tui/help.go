package tui

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const helpChrome = 4

func (m *Model) helpKey(msg tea.KeyPressMsg) {
	switch {
	case key.Matches(msg, m.keys.closeHelp):
		m.help = false
	case key.Matches(msg, m.keys.down):
		m.helpTop = min(m.helpTop+1, m.helpOverflow())
	case key.Matches(msg, m.keys.up):
		m.helpTop = max(min(m.helpTop, m.helpOverflow())-1, 0)
	}
}

func (m Model) overlay() string {
	switch {
	case m.modal != nil:
		return m.modalView()
	case m.bmModal != nil:
		return m.bookmarkModalView()
	case m.bmDelete != nil:
		return m.bookmarkDeleteView()
	case m.help:
		return m.helpView()
	}
	return ""
}

func (m Model) helpRows() int {
	if m.height <= helpChrome {
		return max(m.height-2, 1)
	}
	return m.height - helpChrome
}

func (m Model) helpOverflow() int {
	return max(len(m.helpLines())-m.helpRows(), 0)
}

func (m Model) helpLines() []string {
	p := m.pal
	sections := m.helpSections()
	keyWidth := 0
	for _, s := range sections {
		for _, h := range s.hints {
			keyWidth = max(keyWidth, lipgloss.Width(strings.Join(h.keys, " ")))
		}
	}
	lines := []string{render(p.Card, []seg{span(fg(p.Title).Bold(true), "Keys")})}
	for _, s := range sections {
		lines = append(lines, "", render(p.Card, []seg{span(fg(p.Header), strings.ToUpper(s.title))}))
		for _, h := range s.hints {
			pad := keyWidth - lipgloss.Width(strings.Join(h.keys, " "))
			label := strings.Repeat(" ", pad+1) + h.label
			lines = append(lines, render(p.Card, m.hintSegs(keyHint{keys: h.keys, label: label})))
		}
	}
	return lines
}

func (m Model) helpView() string {
	p := m.pal
	lines := m.helpLines()
	footer := m.keys.closeHelp.Help().Key + " " + m.keys.closeHelp.Help().Desc
	if overflow := m.helpOverflow(); overflow > 0 {
		top := min(m.helpTop, overflow)
		lines = lines[top : top+m.helpRows()]
		footer = strings.Join(hint("scroll", m.keys.down, m.keys.up).keys, " ") + " scroll · " + footer
	}
	if m.height > helpChrome {
		lines = append(lines, "", render(p.Card, []seg{span(fg(p.Muted), footer)}))
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(p.ModalBorder).
		Padding(0, 2).
		Render(strings.Join(lines, "\n"))
}
