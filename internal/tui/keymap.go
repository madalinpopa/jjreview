package tui

import (
	"charm.land/bubbles/v2/key"
	"github.com/madalinpopa/jjreview/internal/tui/ui"
)

type keyMap struct {
	switchPane   key.Binding
	down         key.Binding
	up           key.Binding
	halfDown     key.Binding
	halfUp       key.Binding
	top          key.Binding
	bottom       key.Binding
	nextHunk     key.Binding
	prevHunk     key.Binding
	nextFile     key.Binding
	prevFile     key.Binding
	workingCopy  key.Binding
	selectHunk   key.Binding
	selectFile   key.Binding
	commit       key.Binding
	fold         key.Binding
	foldAll      key.Binding
	removed      key.Binding
	removedAll   key.Binding
	reload       key.Binding
	edit         key.Binding
	full         key.Binding
	help         key.Binding
	closeHelp    key.Binding
	quit         key.Binding
	reviewTab    key.Binding
	logTab       key.Binding
	bookmarksTab key.Binding
	open         key.Binding
	search       key.Binding
	clearSearch  key.Binding
	expand       key.Binding
	collapse     key.Binding
	abandon      key.Binding
	confirm      key.Binding
}

func newKeyMap() keyMap {
	return keyMap{
		switchPane:   key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "switch pane")),
		down:         key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("j", "next line or file")),
		up:           key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("k", "previous line or file")),
		halfDown:     key.NewBinding(key.WithKeys("ctrl+d", "pgdown"), key.WithHelp("^d", "half page down")),
		halfUp:       key.NewBinding(key.WithKeys("ctrl+u", "pgup"), key.WithHelp("^u", "half page up")),
		top:          key.NewBinding(key.WithKeys("g", "home"), key.WithHelp("g", "first line or file")),
		bottom:       key.NewBinding(key.WithKeys("G", "end"), key.WithHelp("G", "last line or file")),
		nextHunk:     key.NewBinding(key.WithKeys("]"), key.WithHelp("]", "next hunk")),
		prevHunk:     key.NewBinding(key.WithKeys("["), key.WithHelp("[", "previous hunk")),
		nextFile:     key.NewBinding(key.WithKeys("J"), key.WithHelp("J", "next file")),
		prevFile:     key.NewBinding(key.WithKeys("K"), key.WithHelp("K", "previous file")),
		workingCopy:  key.NewBinding(key.WithKeys("@"), key.WithHelp("@", "working-copy revision")),
		selectHunk:   key.NewBinding(key.WithKeys("space"), key.WithHelp("space", "select the hunk (the file in the file pane)")),
		selectFile:   key.NewBinding(key.WithKeys("S"), key.WithHelp("S", "select every hunk in the file")),
		commit:       key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "commit the selected hunks")),
		fold:         key.NewBinding(key.WithKeys("z", "enter"), key.WithHelp("z enter", "fold the hunk")),
		foldAll:      key.NewBinding(key.WithKeys("Z"), key.WithHelp("Z", "fold all hunks")),
		removed:      key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "show removed lines of the hunk")),
		removedAll:   key.NewBinding(key.WithKeys("D"), key.WithHelp("D", "show removed lines of all hunks")),
		reload:       key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "reload the change")),
		edit:         key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "open in $EDITOR")),
		full:         key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "full file / changes only")),
		help:         key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "toggle this help")),
		closeHelp:    key.NewBinding(key.WithKeys("?", "esc", "q"), key.WithHelp("? / esc", "close")),
		quit:         key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit")),
		reviewTab:    key.NewBinding(key.WithKeys("1"), key.WithHelp("1", "review tab")),
		logTab:       key.NewBinding(key.WithKeys("2"), key.WithHelp("2", "log tab")),
		bookmarksTab: key.NewBinding(key.WithKeys("3"), key.WithHelp("3", "bookmarks tab")),
		open:         key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "review the revision")),
		search:       key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search by ID or description")),
		expand:       key.NewBinding(key.WithKeys("l"), key.WithHelp("l", "show the files of the revision")),
		collapse:     key.NewBinding(key.WithKeys("h"), key.WithHelp("h", "hide the files of the revision")),
		clearSearch:  key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "clear the search")),
		abandon:      key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "abandon the revision (y confirms)")),
		confirm:      key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "confirm")),
	}
}

func (m Model) keyHints() []ui.KeyHint {
	switch m.view {
	case logView:
		return m.logHints()
	case bookmarksView:
		return m.bookmarkHints()
	}
	move := ui.Hint("line", m.keys.down, m.keys.up)
	if m.filesFocus {
		move.Label = "file"
	}
	hints := []ui.KeyHint{ui.Hint("pane", m.keys.switchPane), move}
	if !m.change.Immutable {
		hints = append(hints, ui.Hint("select", m.keys.selectHunk), ui.Hint("commit", m.keys.commit))
	}
	if m.filesFocus {
		hints = append(hints, ui.Hint("diff", m.keys.removedAll))
	} else {
		hints = append(hints, ui.Hint("diff", m.keys.removed))
	}
	if m.change.WorkingCopy {
		hints = append(hints, ui.Hint("edit", m.keys.edit))
	}
	return append(hints, ui.Hint("help", m.keys.help))
}

func (m Model) logHints() []ui.KeyHint {
	if m.log.searching {
		return []ui.KeyHint{ui.Hint("apply", m.keys.open), ui.Hint("clear", m.keys.clearSearch)}
	}
	hints := []ui.KeyHint{
		ui.Hint("tabs", m.keys.reviewTab, m.keys.logTab, m.keys.bookmarksTab),
		ui.Hint("revision", m.keys.down, m.keys.up),
		ui.Hint("review", m.keys.open),
		ui.Hint("files", m.keys.expand, m.keys.collapse),
		ui.Hint("search", m.keys.search),
	}
	if m.log.filtered() {
		hints = append(hints, ui.Hint("clear", m.keys.clearSearch))
	}
	return append(hints, ui.Hint("refresh", m.keys.reload), ui.Hint("help", m.keys.help))
}

func (m Model) bookmarkHints() []ui.KeyHint {
	hints := append([]ui.KeyHint{ui.Hint("tabs", m.keys.reviewTab, m.keys.logTab, m.keys.bookmarksTab)}, m.bookmarks.hints()...)
	return append(hints, ui.Hint("help", m.keys.help))
}

type helpSection struct {
	title string
	hints []ui.KeyHint
}

func (m Model) helpSections() []helpSection {
	views := helpSection{title: "Views", hints: []ui.KeyHint{ui.Describe(m.keys.reviewTab), ui.Describe(m.keys.logTab), ui.Describe(m.keys.bookmarksTab)}}
	if m.view == bookmarksView {
		return []helpSection{
			views,
			{title: "Bookmarks", hints: m.bookmarks.help()},
			{title: "Other", hints: []ui.KeyHint{ui.Describe(m.keys.help), ui.Describe(m.keys.quit)}},
		}
	}
	if m.view == logView {
		return []helpSection{
			views,
			{title: "Log", hints: []ui.KeyHint{
				ui.Hint("next / previous revision", m.keys.down, m.keys.up),
				ui.Hint("half page down / up", m.keys.halfDown, m.keys.halfUp),
				ui.Hint("first / last revision", m.keys.top, m.keys.bottom),
				ui.Describe(m.keys.open),
				ui.Describe(m.keys.expand),
				ui.Describe(m.keys.collapse),
				ui.Describe(m.keys.abandon),
				ui.Describe(m.keys.search),
				ui.Describe(m.keys.clearSearch),
				ui.Hint("refresh the log", m.keys.reload),
			}},
			{title: "Other", hints: []ui.KeyHint{ui.Describe(m.keys.help), ui.Describe(m.keys.quit)}},
		}
	}
	var review []ui.KeyHint
	if !m.change.Immutable {
		review = []ui.KeyHint{ui.Describe(m.keys.selectHunk), ui.Describe(m.keys.selectFile), ui.Describe(m.keys.commit)}
	}
	review = append(review, ui.Describe(m.keys.fold), ui.Describe(m.keys.foldAll), ui.Describe(m.keys.removed), ui.Describe(m.keys.removedAll))
	return []helpSection{
		views,
		{title: "Navigation", hints: []ui.KeyHint{
			ui.Describe(m.keys.switchPane),
			ui.Hint("next / previous line or file", m.keys.down, m.keys.up),
			ui.Hint("half page down / up", m.keys.halfDown, m.keys.halfUp),
			ui.Hint("first / last line or file", m.keys.top, m.keys.bottom),
			ui.Hint("next / previous hunk", m.keys.nextHunk, m.keys.prevHunk),
			ui.Hint("next / previous file", m.keys.nextFile, m.keys.prevFile),
			ui.Describe(m.keys.workingCopy),
		}},
		{title: "Review", hints: review},
		{title: "Other", hints: m.otherHints()},
	}
}

func (m Model) otherHints() []ui.KeyHint {
	var hints []ui.KeyHint
	if m.change.WorkingCopy {
		hints = append(hints, ui.Describe(m.keys.reload), ui.Describe(m.keys.edit))
	}
	return append(hints, ui.Describe(m.keys.full), ui.Describe(m.keys.help), ui.Describe(m.keys.quit))
}
