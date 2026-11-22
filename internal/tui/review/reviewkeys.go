package review

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/madalinpopa/jjreview/internal/jj"
	"github.com/madalinpopa/jjreview/internal/tui/ui"
)

type keyMap struct {
	ui.NavKeys
	switchPane  key.Binding
	nextHunk    key.Binding
	prevHunk    key.Binding
	nextFile    key.Binding
	prevFile    key.Binding
	workingCopy key.Binding
	selectLine  key.Binding
	selectRange key.Binding
	cancelRange key.Binding
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
}

func newKeyMap() keyMap {
	return keyMap{
		NavKeys:     ui.NewNavKeys("next line or file", "previous line or file", "first line or file", "last line or file"),
		switchPane:  key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "switch pane")),
		nextHunk:    key.NewBinding(key.WithKeys("]"), key.WithHelp("]", "next hunk")),
		prevHunk:    key.NewBinding(key.WithKeys("["), key.WithHelp("[", "previous hunk")),
		nextFile:    key.NewBinding(key.WithKeys("J"), key.WithHelp("J", "next file")),
		prevFile:    key.NewBinding(key.WithKeys("K"), key.WithHelp("K", "previous file")),
		workingCopy: key.NewBinding(key.WithKeys("@"), key.WithHelp("@", "working-copy revision")),
		selectLine:  key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "select the line")),
		selectRange: key.NewBinding(key.WithKeys("v"), key.WithHelp("v", "a range of lines")),
		cancelRange: key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel the range")),
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
	}
}

func (m Model) Hints(c jj.Change) []ui.KeyHint {
	k := m.keys
	move := ui.Hint("line", k.Down, k.Up)
	if m.ranging {
		return []ui.KeyHint{move, ui.Hint("select range", k.selectLine, k.selectHunk), ui.Hint("cancel", k.cancelRange, k.selectRange)}
	}
	if m.filesFocus {
		move.Label = "file"
	}
	hints := []ui.KeyHint{ui.Hint("pane", k.switchPane), move}
	if !c.Immutable {
		pick := ui.Hint("select", k.selectLine, k.selectHunk)
		if m.filesFocus {
			pick = ui.Hint("select", k.selectHunk)
		}
		hints = append(hints, pick, ui.Hint("commit", k.commit))
	}
	if m.filesFocus {
		hints = append(hints, ui.Hint("diff", k.removedAll))
	} else {
		hints = append(hints, ui.Hint("diff", k.removed))
	}
	if c.WorkingCopy {
		hints = append(hints, ui.Hint("edit", k.edit))
	}
	return hints
}

type HelpSection struct {
	Title string
	Hints []ui.KeyHint
}

func (m Model) Help(c jj.Change) ([]HelpSection, []ui.KeyHint) {
	k := m.keys
	var review []ui.KeyHint
	if !c.Immutable {
		review = []ui.KeyHint{ui.Hint(k.selectLine.Help().Desc+" / "+k.selectRange.Help().Desc, k.selectLine, k.selectRange), ui.Describe(k.selectHunk), ui.Describe(k.selectFile), ui.Describe(k.commit)}
	}
	review = append(review, ui.Describe(k.fold), ui.Describe(k.foldAll), ui.Describe(k.removed), ui.Describe(k.removedAll))
	sections := []HelpSection{
		{Title: "Navigation", Hints: []ui.KeyHint{
			ui.Describe(k.switchPane),
			ui.Hint("next / previous line or file", k.Down, k.Up),
			ui.Hint("half page down / up", k.HalfDown, k.HalfUp),
			ui.Hint("first / last line or file", k.Top, k.Bottom),
			ui.Hint("next / previous hunk", k.nextHunk, k.prevHunk),
			ui.Hint("next / previous file", k.nextFile, k.prevFile),
			ui.Describe(k.workingCopy),
		}},
		{Title: "Review", Hints: review},
	}
	var other []ui.KeyHint
	if c.WorkingCopy {
		other = append(other, ui.Describe(k.reload), ui.Describe(k.edit))
	}
	return sections, append(other, ui.Describe(k.full))
}

func (m Model) DropKeys() []key.Binding {
	return []key.Binding{m.keys.workingCopy, m.keys.reload, m.keys.edit}
}

func Bindings() map[string]key.Binding {
	k := newKeyMap()
	return map[string]key.Binding{
		"switchPane":  k.switchPane,
		"down":        k.Down,
		"up":          k.Up,
		"halfDown":    k.HalfDown,
		"halfUp":      k.HalfUp,
		"top":         k.Top,
		"bottom":      k.Bottom,
		"nextHunk":    k.nextHunk,
		"prevHunk":    k.prevHunk,
		"nextFile":    k.nextFile,
		"prevFile":    k.prevFile,
		"workingCopy": k.workingCopy,
		"selectLine":  k.selectLine,
		"selectRange": k.selectRange,
		"cancelRange": k.cancelRange,
		"selectHunk":  k.selectHunk,
		"selectFile":  k.selectFile,
		"commit":      k.commit,
		"fold":        k.fold,
		"foldAll":     k.foldAll,
		"removed":     k.removed,
		"removedAll":  k.removedAll,
		"reload":      k.reload,
		"edit":        k.edit,
		"full":        k.full,
	}
}

type intent int

const (
	noIntent intent = iota
	switchPaneIntent
	downIntent
	upIntent
	halfDownIntent
	halfUpIntent
	topIntent
	bottomIntent
	nextHunkIntent
	prevHunkIntent
	nextFileIntent
	prevFileIntent
	workingCopyIntent
	reloadIntent
	fullIntent
	commitIntent
	editIntent
	foldIntent
	removedIntent
	removedAllIntent
	foldAllIntent
	selectLineIntent
	selectRangeIntent
	cancelRangeIntent
	selectHunkIntent
	selectFileIntent
)

func (k keyMap) resolve(msg tea.KeyPressMsg) intent {
	switch {
	case key.Matches(msg, k.switchPane):
		return switchPaneIntent
	case key.Matches(msg, k.Down):
		return downIntent
	case key.Matches(msg, k.Up):
		return upIntent
	case key.Matches(msg, k.HalfDown):
		return halfDownIntent
	case key.Matches(msg, k.HalfUp):
		return halfUpIntent
	case key.Matches(msg, k.Top):
		return topIntent
	case key.Matches(msg, k.Bottom):
		return bottomIntent
	case key.Matches(msg, k.nextHunk):
		return nextHunkIntent
	case key.Matches(msg, k.prevHunk):
		return prevHunkIntent
	case key.Matches(msg, k.nextFile):
		return nextFileIntent
	case key.Matches(msg, k.prevFile):
		return prevFileIntent
	case key.Matches(msg, k.workingCopy):
		return workingCopyIntent
	case key.Matches(msg, k.reload):
		return reloadIntent
	case key.Matches(msg, k.full):
		return fullIntent
	case key.Matches(msg, k.commit):
		return commitIntent
	case key.Matches(msg, k.edit):
		return editIntent
	case key.Matches(msg, k.fold):
		return foldIntent
	case key.Matches(msg, k.removed):
		return removedIntent
	case key.Matches(msg, k.removedAll):
		return removedAllIntent
	case key.Matches(msg, k.foldAll):
		return foldAllIntent
	case key.Matches(msg, k.selectLine):
		return selectLineIntent
	case key.Matches(msg, k.selectRange):
		return selectRangeIntent
	case key.Matches(msg, k.cancelRange):
		return cancelRangeIntent
	case key.Matches(msg, k.selectHunk):
		return selectHunkIntent
	case key.Matches(msg, k.selectFile):
		return selectFileIntent
	}
	return noIntent
}
