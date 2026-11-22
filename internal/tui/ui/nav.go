package ui

import "charm.land/bubbles/v2/key"

type NavKeys struct {
	Down     key.Binding
	Up       key.Binding
	HalfDown key.Binding
	HalfUp   key.Binding
	Top      key.Binding
	Bottom   key.Binding
}

func NewNavKeys(next, previous, first, last string) NavKeys {
	return NavKeys{
		Down:     key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("j", next)),
		Up:       key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("k", previous)),
		HalfDown: key.NewBinding(key.WithKeys("ctrl+d", "pgdown"), key.WithHelp("^d", "half page down")),
		HalfUp:   key.NewBinding(key.WithKeys("ctrl+u", "pgup"), key.WithHelp("^u", "half page up")),
		Top:      key.NewBinding(key.WithKeys("g", "home"), key.WithHelp("g", first)),
		Bottom:   key.NewBinding(key.WithKeys("G", "end"), key.WithHelp("G", last)),
	}
}
