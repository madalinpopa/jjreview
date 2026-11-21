package tui

import (
	"strings"

	"charm.land/bubbles/v2/key"
)

type keyMap struct {
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
	workingCopy key.Binding
	selectHunk  key.Binding
	selectFile  key.Binding
	commit      key.Binding
	fold        key.Binding
	foldAll     key.Binding
	removed     key.Binding
	removedAll  key.Binding
	reload      key.Binding
	edit        key.Binding
	full        key.Binding
	help        key.Binding
	closeHelp   key.Binding
	quit        key.Binding
	reviewTab   key.Binding
	logTab      key.Binding
	open        key.Binding
	search      key.Binding
	clearSearch key.Binding
	expand      key.Binding
	collapse    key.Binding
}

func newKeyMap() keyMap {
	return keyMap{
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
		workingCopy: key.NewBinding(key.WithKeys("@"), key.WithHelp("@", "working-copy revision")),
		selectHunk:  key.NewBinding(key.WithKeys("space"), key.WithHelp("space", "select the hunk (the file in the file pane)")),
		selectFile:  key.NewBinding(key.WithKeys("S"), key.WithHelp("S", "select every hunk in the file")),
		commit:      key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "commit the selected hunks")),
		fold:        key.NewBinding(key.WithKeys("z", "enter"), key.WithHelp("z enter", "fold the hunk")),
		foldAll:     key.NewBinding(key.WithKeys("Z"), key.WithHelp("Z", "fold all hunks")),
		removed:     key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "show removed lines of the hunk")),
		removedAll:  key.NewBinding(key.WithKeys("D"), key.WithHelp("D", "show removed lines of all hunks")),
		reload:      key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "reload the change")),
		edit:        key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "open in $EDITOR")),
		full:        key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "full file / changes only")),
		help:        key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "toggle this help")),
		closeHelp:   key.NewBinding(key.WithKeys("?", "esc", "q"), key.WithHelp("? / esc", "close")),
		quit:        key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit")),
		reviewTab:   key.NewBinding(key.WithKeys("1"), key.WithHelp("1", "review tab")),
		logTab:      key.NewBinding(key.WithKeys("2"), key.WithHelp("2", "log tab")),
		open:        key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "review the revision")),
		search:      key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search by ID or description")),
		expand:      key.NewBinding(key.WithKeys("l"), key.WithHelp("l", "show the files of the revision")),
		collapse:    key.NewBinding(key.WithKeys("h"), key.WithHelp("h", "hide the files of the revision")),
		clearSearch: key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "clear the search")),
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
	if m.view == logView {
		return m.logHints()
	}
	move := hint("line", m.keys.down, m.keys.up)
	if m.filesFocus {
		move.label = "file"
	}
	hints := []keyHint{hint("pane", m.keys.switchPane), move}
	if !m.change.Immutable {
		hints = append(hints, hint("select", m.keys.selectHunk), hint("commit", m.keys.commit))
	}
	if m.filesFocus {
		hints = append(hints, hint("diff", m.keys.removedAll))
	} else {
		hints = append(hints, hint("diff", m.keys.removed))
	}
	if m.change.WorkingCopy {
		hints = append(hints, hint("edit", m.keys.edit))
	}
	return append(hints, hint("help", m.keys.help))
}

func (m Model) logHints() []keyHint {
	if m.log.searching {
		return []keyHint{hint("apply", m.keys.open), hint("clear", m.keys.clearSearch)}
	}
	hints := []keyHint{
		hint("tabs", m.keys.reviewTab, m.keys.logTab),
		hint("revision", m.keys.down, m.keys.up),
		hint("review", m.keys.open),
		hint("files", m.keys.expand, m.keys.collapse),
		hint("search", m.keys.search),
	}
	if m.log.filtered() {
		hints = append(hints, hint("clear", m.keys.clearSearch))
	}
	return append(hints, hint("refresh", m.keys.reload), hint("help", m.keys.help))
}

type helpSection struct {
	title string
	hints []keyHint
}

func (m Model) helpSections() []helpSection {
	views := helpSection{title: "Views", hints: []keyHint{describe(m.keys.reviewTab), describe(m.keys.logTab)}}
	if m.view == logView {
		return []helpSection{
			views,
			{title: "Log", hints: []keyHint{
				hint("next / previous revision", m.keys.down, m.keys.up),
				hint("half page down / up", m.keys.halfDown, m.keys.halfUp),
				hint("first / last revision", m.keys.top, m.keys.bottom),
				describe(m.keys.open),
				describe(m.keys.expand),
				describe(m.keys.collapse),
				describe(m.keys.search),
				describe(m.keys.clearSearch),
				hint("refresh the log", m.keys.reload),
			}},
			{title: "Other", hints: []keyHint{describe(m.keys.help), describe(m.keys.quit)}},
		}
	}
	var review []keyHint
	if !m.change.Immutable {
		review = []keyHint{describe(m.keys.selectHunk), describe(m.keys.selectFile), describe(m.keys.commit)}
	}
	review = append(review, describe(m.keys.fold), describe(m.keys.foldAll), describe(m.keys.removed), describe(m.keys.removedAll))
	return []helpSection{
		views,
		{title: "Navigation", hints: []keyHint{
			describe(m.keys.switchPane),
			hint("next / previous line or file", m.keys.down, m.keys.up),
			hint("half page down / up", m.keys.halfDown, m.keys.halfUp),
			hint("first / last line or file", m.keys.top, m.keys.bottom),
			hint("next / previous hunk", m.keys.nextHunk, m.keys.prevHunk),
			hint("next / previous file", m.keys.nextFile, m.keys.prevFile),
			describe(m.keys.workingCopy),
		}},
		{title: "Review", hints: review},
		{title: "Other", hints: m.otherHints()},
	}
}

func (m Model) otherHints() []keyHint {
	var hints []keyHint
	if m.change.WorkingCopy {
		hints = append(hints, describe(m.keys.reload), describe(m.keys.edit))
	}
	return append(hints, describe(m.keys.full), describe(m.keys.help), describe(m.keys.quit))
}
