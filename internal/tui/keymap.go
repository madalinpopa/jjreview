package tui

import (
	"strings"

	"charm.land/bubbles/v2/key"
)

type keyMap struct {
	focusFiles  key.Binding
	focusDiff   key.Binding
	switchPane  key.Binding
	down        key.Binding
	up          key.Binding
	halfDown    key.Binding
	halfUp      key.Binding
	top         key.Binding
	bottom      key.Binding
	nextHunk    key.Binding
	prevHunk    key.Binding
	nextFile    key.Binding
	prevFile    key.Binding
	child       key.Binding
	parent      key.Binding
	workingCopy key.Binding
	selectHunk  key.Binding
	selectFile  key.Binding
	commit      key.Binding
	fold        key.Binding
	foldAll     key.Binding
	removed     key.Binding
	reload      key.Binding
	edit        key.Binding
	full        key.Binding
	help        key.Binding
	closeHelp   key.Binding
	quit        key.Binding
}

func newKeyMap() keyMap {
	return keyMap{
		focusFiles:  key.NewBinding(key.WithKeys("h", "ctrl+h", "left"), key.WithHelp("h", "focus the file pane")),
		focusDiff:   key.NewBinding(key.WithKeys("l", "ctrl+l", "right"), key.WithHelp("l", "focus the diff pane")),
		switchPane:  key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "switch pane")),
		down:        key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("j", "next line or file")),
		up:          key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("k", "previous line or file")),
		halfDown:    key.NewBinding(key.WithKeys("ctrl+d", "pgdown"), key.WithHelp("^d", "half page down")),
		halfUp:      key.NewBinding(key.WithKeys("ctrl+u", "pgup"), key.WithHelp("^u", "half page up")),
		top:         key.NewBinding(key.WithKeys("g", "home"), key.WithHelp("g", "first line or file")),
		bottom:      key.NewBinding(key.WithKeys("G", "end"), key.WithHelp("G", "last line or file")),
		nextHunk:    key.NewBinding(key.WithKeys("]"), key.WithHelp("]", "next hunk")),
		prevHunk:    key.NewBinding(key.WithKeys("["), key.WithHelp("[", "previous hunk")),
		nextFile:    key.NewBinding(key.WithKeys("J"), key.WithHelp("J", "next file")),
		prevFile:    key.NewBinding(key.WithKeys("K"), key.WithHelp("K", "previous file")),
		child:       key.NewBinding(key.WithKeys("ctrl+n"), key.WithHelp("^n", "child revision")),
		parent:      key.NewBinding(key.WithKeys("ctrl+p"), key.WithHelp("^p", "parent revision")),
		workingCopy: key.NewBinding(key.WithKeys("@"), key.WithHelp("@", "working-copy revision")),
		selectHunk:  key.NewBinding(key.WithKeys("space"), key.WithHelp("space", "select the hunk (the file in the file pane)")),
		selectFile:  key.NewBinding(key.WithKeys("S"), key.WithHelp("S", "select every hunk in the file")),
		commit:      key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "commit the selected hunks")),
		fold:        key.NewBinding(key.WithKeys("z", "enter"), key.WithHelp("z enter", "fold the hunk")),
		foldAll:     key.NewBinding(key.WithKeys("Z"), key.WithHelp("Z", "fold all hunks")),
		removed:     key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "show removed lines of the hunk")),
		reload:      key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "reload the change")),
		edit:        key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "open in $EDITOR")),
		full:        key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "full file / changes only")),
		help:        key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "toggle this help")),
		closeHelp:   key.NewBinding(key.WithKeys("?", "esc", "q"), key.WithHelp("? / esc", "close")),
		quit:        key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit")),
	}
}

type keyHint struct {
	keys  []string
	label string
}

func hint(label string, bindings ...key.Binding) keyHint {
	h := keyHint{label: label}
	for _, b := range bindings {
		h.keys = append(h.keys, strings.Fields(b.Help().Key)...)
	}
	return h
}

func describe(b key.Binding) keyHint {
	return hint(b.Help().Desc, b)
}

func (m Model) keyHints() []keyHint {
	move := hint("line", m.keys.down, m.keys.up)
	if m.filesFocus {
		move.label = "file"
	}
	hints := []keyHint{hint("pane", m.keys.focusFiles, m.keys.focusDiff), move}
	if !m.change.Immutable {
		hints = append(hints, hint("select", m.keys.selectHunk), hint("commit", m.keys.commit))
	}
	if !m.filesFocus {
		hints = append(hints, hint("diff", m.keys.removed))
	}
	if m.change.WorkingCopy {
		hints = append(hints, hint("edit", m.keys.edit))
	}
	return append(hints, hint("help", m.keys.help))
}

type helpSection struct {
	title string
	hints []keyHint
}

func (m Model) helpSections() []helpSection {
	var review []keyHint
	if !m.change.Immutable {
		review = []keyHint{describe(m.keys.selectHunk), describe(m.keys.selectFile), describe(m.keys.commit)}
	}
	review = append(review, describe(m.keys.fold), describe(m.keys.foldAll), describe(m.keys.removed))
	return []helpSection{
		{title: "Navigation", hints: []keyHint{
			hint("focus the file / diff pane", m.keys.focusFiles, m.keys.focusDiff),
			describe(m.keys.switchPane),
			hint("next / previous line or file", m.keys.down, m.keys.up),
			hint("half page down / up", m.keys.halfDown, m.keys.halfUp),
			hint("first / last line or file", m.keys.top, m.keys.bottom),
			hint("next / previous hunk", m.keys.nextHunk, m.keys.prevHunk),
			hint("next / previous file", m.keys.nextFile, m.keys.prevFile),
			hint("child / parent revision", m.keys.child, m.keys.parent),
			describe(m.keys.workingCopy),
		}},
		{title: "Review", hints: review},
		{title: "Other", hints: m.otherHints()},
	}
}

func (m Model) otherHints() []keyHint {
	hints := []keyHint{describe(m.keys.reload)}
	if m.change.WorkingCopy {
		hints = append(hints, describe(m.keys.edit))
	}
	return append(hints, describe(m.keys.full), describe(m.keys.help), describe(m.keys.quit))
}
