package tui

import (
	"slices"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/madalinpopa/jjreview/internal/tui/ui"
)

type globalKeyMap struct {
	reviewTab     key.Binding
	logTab        key.Binding
	bookmarksTab  key.Binding
	operationsTab key.Binding
	help          key.Binding
	quit          key.Binding
	closeHelp     key.Binding
	scrollDown    key.Binding
	scrollUp      key.Binding
	open          key.Binding
}

func newGlobalKeyMap() globalKeyMap {
	return globalKeyMap{
		reviewTab:     key.NewBinding(key.WithKeys("1"), key.WithHelp("1", "review tab")),
		logTab:        key.NewBinding(key.WithKeys("2"), key.WithHelp("2", "log tab")),
		bookmarksTab:  key.NewBinding(key.WithKeys("3"), key.WithHelp("3", "bookmarks tab")),
		operationsTab: key.NewBinding(key.WithKeys("4"), key.WithHelp("4", "operations tab")),
		help:          key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "toggle this help")),
		quit:          key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit")),
		closeHelp:     key.NewBinding(key.WithKeys("?", "esc", "q"), key.WithHelp("? / esc", "close")),
		scrollDown:    key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("j", "scroll down")),
		scrollUp:      key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("k", "scroll up")),
		open:          key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "review the revision")),
	}
}

type globalIntent int

const (
	noGlobalIntent globalIntent = iota
	showReviewIntent
	showLogIntent
	showBookmarksIntent
	showOperationsIntent
	helpIntent
	quitIntent
)

func (k globalKeyMap) resolve(msg tea.KeyPressMsg) globalIntent {
	switch {
	case key.Matches(msg, k.reviewTab):
		return showReviewIntent
	case key.Matches(msg, k.logTab):
		return showLogIntent
	case key.Matches(msg, k.bookmarksTab):
		return showBookmarksIntent
	case key.Matches(msg, k.operationsTab):
		return showOperationsIntent
	case key.Matches(msg, k.help):
		return helpIntent
	case key.Matches(msg, k.quit):
		return quitIntent
	}
	return noGlobalIntent
}

func (m *Model) globalKey(msg tea.KeyPressMsg, armed string) (tea.Cmd, bool) {
	switch m.keys.resolve(msg) {
	case showReviewIntent:
		if m.view == reviewView {
			return nil, false
		}
		m.view = reviewView
	case showLogIntent:
		if m.view == logView {
			return nil, false
		}
		return m.showLog(), true
	case showBookmarksIntent:
		if m.view == bookmarksView {
			return nil, false
		}
		return m.showBookmarks(), true
	case showOperationsIntent:
		if m.view == operationsView {
			return nil, false
		}
		return m.showOperations(), true
	case helpIntent:
		m.help = true
		m.helpTop = 0
	case quitIntent:
		if m.confirmDrop(msg, armed) {
			return m.quit(), true
		}
	default:
		return nil, false
	}
	return nil, true
}

func (m Model) keyHints() []ui.KeyHint {
	help := ui.Hint("help", m.keys.help)
	switch m.view {
	case logView:
		return m.log.Hints(m.tabsHint(), help)
	case bookmarksView:
		return slices.Concat([]ui.KeyHint{m.tabsHint()}, m.bookmarks.Hints(), []ui.KeyHint{help})
	case operationsView:
		return slices.Concat([]ui.KeyHint{m.tabsHint()}, m.operations.Hints(), []ui.KeyHint{help})
	}
	return append(m.reviewHints(), help)
}

func (m Model) tabsHint() ui.KeyHint {
	return ui.Hint("tabs", m.keys.reviewTab, m.keys.logTab, m.keys.bookmarksTab, m.keys.operationsTab)
}

type helpSection struct {
	title string
	hints []ui.KeyHint
}

func (m Model) helpSections() []helpSection {
	views := helpSection{title: "Views", hints: []ui.KeyHint{ui.Describe(m.keys.reviewTab), ui.Describe(m.keys.logTab), ui.Describe(m.keys.bookmarksTab), ui.Describe(m.keys.operationsTab)}}
	other := []ui.KeyHint{ui.Describe(m.keys.help), ui.Describe(m.keys.quit)}
	var own []helpSection
	switch m.view {
	case bookmarksView:
		own = []helpSection{{title: "Bookmarks", hints: m.bookmarks.Help()}}
	case operationsView:
		own = []helpSection{{title: "Operations", hints: m.operations.Help()}}
	case logView:
		own = []helpSection{{title: "Log", hints: m.log.Help()}}
	default:
		own = m.reviewHelp()
		other = append(m.reviewOtherHelp(), other...)
	}
	return slices.Concat([]helpSection{views}, own, []helpSection{{title: "Other", hints: other}})
}
