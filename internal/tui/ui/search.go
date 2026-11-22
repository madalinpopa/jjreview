package ui

import (
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/madalinpopa/jjreview/internal/theme"
)

func NewSearchInput() textinput.Model {
	in := textinput.New()
	in.Prompt = "/ "
	styles := in.Styles()
	styles.Cursor.Blink = false
	in.SetStyles(styles)
	return in
}

func SearchStyles(p theme.Theme) textinput.Styles {
	base := lipgloss.NewStyle().Background(p.Card)
	state := textinput.StyleState{
		Text:        base.Foreground(p.Strong),
		Placeholder: base.Foreground(p.Muted),
		Suggestion:  base.Foreground(p.Muted),
		Prompt:      base.Foreground(p.Key),
	}
	return textinput.Styles{
		Focused: state,
		Blurred: state,
		Cursor:  textinput.CursorStyle{Color: p.Selection, Shape: tea.CursorBar},
	}
}
