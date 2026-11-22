package bookmarks

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/madalinpopa/jjreview/internal/tui/ui"
)

type keyMap struct {
	ui.NavKeys
	open      key.Binding
	create    key.Binding
	edit      key.Binding
	remove    key.Binding
	reload    key.Binding
	cancel    key.Binding
	confirm   key.Binding
	nextField key.Binding
	save      key.Binding
}

func newKeyMap() keyMap {
	return keyMap{
		NavKeys:   ui.NewNavKeys("next bookmark", "previous bookmark", "first bookmark", "last bookmark"),
		open:      key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "review the target revision")),
		create:    key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "new bookmark on a revision")),
		edit:      key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "edit the bookmark: rename or move it")),
		remove:    key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "delete the bookmark (y confirms)")),
		reload:    key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "refresh the bookmarks")),
		cancel:    key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
		confirm:   key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "confirm")),
		nextField: key.NewBinding(key.WithKeys("tab", "shift+tab", "up", "down"), key.WithHelp("tab", "next field")),
		save:      key.NewBinding(key.WithKeys("enter", "ctrl+enter", "ctrl+s"), key.WithHelp("enter", "save")),
	}
}

type intent int

const (
	noIntent intent = iota
	moveDownIntent
	moveUpIntent
	halfDownIntent
	halfUpIntent
	firstIntent
	lastIntent
	openIntent
	createIntent
	editIntent
	deleteIntent
	reloadIntent
	cancelIntent
	confirmIntent
	nextFieldIntent
	saveIntent
)

func (k keyMap) resolve(msg tea.KeyPressMsg) intent {
	switch {
	case key.Matches(msg, k.Down):
		return moveDownIntent
	case key.Matches(msg, k.Up):
		return moveUpIntent
	case key.Matches(msg, k.HalfDown):
		return halfDownIntent
	case key.Matches(msg, k.HalfUp):
		return halfUpIntent
	case key.Matches(msg, k.Top):
		return firstIntent
	case key.Matches(msg, k.Bottom):
		return lastIntent
	case key.Matches(msg, k.open):
		return openIntent
	case key.Matches(msg, k.create):
		return createIntent
	case key.Matches(msg, k.edit):
		return editIntent
	case key.Matches(msg, k.remove):
		return deleteIntent
	case key.Matches(msg, k.reload):
		return reloadIntent
	}
	return noIntent
}

func (k keyMap) resolveDialog(msg tea.KeyPressMsg) intent {
	switch {
	case key.Matches(msg, k.cancel):
		return cancelIntent
	case key.Matches(msg, k.nextField):
		return nextFieldIntent
	case key.Matches(msg, k.save):
		return saveIntent
	}
	return noIntent
}

func (k keyMap) resolveDelete(msg tea.KeyPressMsg) intent {
	if key.Matches(msg, k.confirm) {
		return confirmIntent
	}
	return cancelIntent
}

func (k keyMap) resolvePicker(msg tea.KeyPressMsg) intent {
	switch {
	case key.Matches(msg, k.cancel):
		return cancelIntent
	case key.Matches(msg, k.confirm) && !msg.IsRepeat:
		return confirmIntent
	case key.Matches(msg, k.Down):
		return moveDownIntent
	case key.Matches(msg, k.Up):
		return moveUpIntent
	case key.Matches(msg, k.Top):
		return firstIntent
	case key.Matches(msg, k.Bottom):
		return lastIntent
	case key.Matches(msg, k.open) && !msg.IsRepeat:
		return openIntent
	}
	return noIntent
}
