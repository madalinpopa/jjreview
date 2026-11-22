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
	intent := m.keys.resolve(msg)
	for _, t := range m.registry() {
		if t.intent != intent {
			continue
		}
		if m.view == t.view {
			return nil, false
		}
		return m.show(t), true
	}
	switch intent {
	case showReviewIntent:
		if m.view == reviewView {
			return nil, false
		}
		m.view = reviewView
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
	if t, ok := m.active(); ok {
		return t.tab.Hints(m.tabsHint(), help)
	}
	return append(m.reviewHints(), help)
}

func (m Model) tabKeys() []key.Binding {
	keys := []key.Binding{m.keys.reviewTab}
	for _, t := range m.registry() {
		keys = append(keys, t.key)
	}
	return keys
}

func (m Model) tabsHint() ui.KeyHint {
	return ui.Hint("tabs", m.tabKeys()...)
}

type helpSection struct {
	title string
	hints []ui.KeyHint
}

func (m Model) helpSections() []helpSection {
	var views helpSection
	views.title = "Views"
	for _, k := range m.tabKeys() {
		views.hints = append(views.hints, ui.Describe(k))
	}
	other := []ui.KeyHint{ui.Describe(m.keys.help), ui.Describe(m.keys.quit)}
	var own []helpSection
	if t, ok := m.active(); ok {
		own = []helpSection{{title: t.label, hints: t.tab.Help()}}
	} else {
		own = m.reviewHelp()
		other = append(m.reviewOtherHelp(), other...)
	}
	return slices.Concat([]helpSection{views}, own, []helpSection{{title: "Other", hints: other}})
}
