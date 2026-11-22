package tui

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/madalinpopa/jjreview/internal/tui/ui"
)

const helpChrome = 4

func (m *Model) helpKey(msg tea.KeyPressMsg) {
	switch m.keys.resolveHelpOverlay(msg) {
	case closeHelpIntent:
		m.help = false
	case scrollHelpDownIntent:
		m.helpTop = min(m.helpTop+1, m.helpOverflow())
	case scrollHelpUpIntent:
		m.helpTop = max(min(m.helpTop, m.helpOverflow())-1, 0)
	}
}

func (m Model) overlay() string {
	switch {
	case m.modal != nil:
		return m.modalView()
	case m.bookmarks.OverlayOpen():
		return m.bookmarks.Overlay(m.env(bookmarksView))
	case m.help:
		return m.helpView()
	}
	if t, ok := m.active(); ok && t.tab.OverlayOpen() {
		return t.tab.Overlay(m.env(t.view))
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
			keyWidth = max(keyWidth, lipgloss.Width(strings.Join(h.Keys, " ")))
		}
	}
	lines := []string{ui.Render(p.Card, []ui.Seg{ui.Span(ui.Fg(p.Title).Bold(true), "Keys")})}
	for _, s := range sections {
		lines = append(lines, "", ui.Render(p.Card, []ui.Seg{ui.Span(ui.Fg(p.Header), strings.ToUpper(s.title))}))
		for _, h := range s.hints {
			pad := keyWidth - lipgloss.Width(strings.Join(h.Keys, " "))
			label := strings.Repeat(" ", pad+1) + h.Label
			lines = append(lines, ui.Render(p.Card, ui.HintSegs(m.pal, ui.KeyHint{Keys: h.Keys, Label: label})))
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
		footer = strings.Join(ui.Hint("scroll", m.keys.scrollDown, m.keys.scrollUp).Keys, " ") + " scroll · " + footer
	}
	if m.height > helpChrome {
		lines = append(lines, "", ui.Render(p.Card, []ui.Seg{ui.Span(ui.Fg(p.Muted), footer)}))
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(p.ModalBorder).
		Padding(0, 2).
		Render(strings.Join(lines, "\n"))
}

type helpOverlayIntent int

const (
	noHelpOverlayIntent helpOverlayIntent = iota
	closeHelpIntent
	scrollHelpDownIntent
	scrollHelpUpIntent
)

func (k globalKeyMap) resolveHelpOverlay(msg tea.KeyPressMsg) helpOverlayIntent {
	switch {
	case key.Matches(msg, k.closeHelp):
		return closeHelpIntent
	case key.Matches(msg, k.scrollDown):
		return scrollHelpDownIntent
	case key.Matches(msg, k.scrollUp):
		return scrollHelpUpIntent
	}
	return noHelpOverlayIntent
}
