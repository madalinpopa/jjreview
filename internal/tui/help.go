package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type helpSection struct {
	title string
	hints []keyHint
}

func helpSections(readOnly bool) []helpSection {
	review := []keyHint{
		{keys: []string{"space"}, label: "select the hunk (the file in the file pane)"},
		{keys: []string{"S"}, label: "select every hunk in the file"},
		{keys: []string{"c"}, label: "commit the selected hunks"},
	}
	if readOnly {
		review = nil
	}
	review = append(review,
		keyHint{keys: []string{"z", "enter"}, label: "fold the hunk"},
		keyHint{keys: []string{"Z"}, label: "fold all hunks"},
		keyHint{keys: []string{"d"}, label: "show removed lines of the hunk"},
	)
	return []helpSection{
		{title: "Navigation", hints: []keyHint{
			{keys: []string{"h", "l"}, label: "focus the file / diff pane"},
			{keys: []string{"j", "k"}, label: "next / previous line or file"},
			{keys: []string{"^d", "^u"}, label: "half page down / up"},
			{keys: []string{"^n", "^p"}, label: "next / previous file"},
		}},
		{title: "Review", hints: review},
		{title: "Other", hints: []keyHint{
			{keys: []string{"e"}, label: "open in $EDITOR"},
			{keys: []string{"f"}, label: "full file / changes only"},
			{keys: []string{"?"}, label: "toggle this help"},
			{keys: []string{"q"}, label: "quit"},
		}},
	}
}

func (m *Model) helpKey(msg tea.KeyPressMsg) {
	switch msg.String() {
	case "?", "esc", "q":
		m.help = false
	}
}

func (m Model) overlay() string {
	switch {
	case m.modal != nil:
		return m.modalView()
	case m.help:
		return m.helpView()
	}
	return ""
}

func (m Model) helpView() string {
	p := m.pal
	sections := helpSections(m.change.Immutable)
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
	lines = append(lines, "", render(p.Card, []seg{span(fg(p.Muted), "? / esc close")}))
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(p.ModalBorder).
		Padding(0, 2).
		Render(strings.Join(lines, "\n"))
}
