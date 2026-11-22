package tui

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/madalinpopa/jjreview/internal/tui/ui"
)

type reviewKeyMap struct {
	ui.NavKeys
	switchPane  key.Binding
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
}

func newReviewKeyMap() reviewKeyMap {
	return reviewKeyMap{
		NavKeys:     ui.NewNavKeys("next line or file", "previous line or file", "first line or file", "last line or file"),
		switchPane:  key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "switch pane")),
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
	}
}

func (m Model) reviewHints() []ui.KeyHint {
	k := m.reviewKeys
	move := ui.Hint("line", k.Down, k.Up)
	if m.filesFocus {
		move.Label = "file"
	}
	hints := []ui.KeyHint{ui.Hint("pane", k.switchPane), move}
	if !m.change.Immutable {
		hints = append(hints, ui.Hint("select", k.selectHunk), ui.Hint("commit", k.commit))
	}
	if m.filesFocus {
		hints = append(hints, ui.Hint("diff", k.removedAll))
	} else {
		hints = append(hints, ui.Hint("diff", k.removed))
	}
	if m.change.WorkingCopy {
		hints = append(hints, ui.Hint("edit", k.edit))
	}
	return hints
}

func (m Model) reviewHelp() []helpSection {
	k := m.reviewKeys
	var review []ui.KeyHint
	if !m.change.Immutable {
		review = []ui.KeyHint{ui.Describe(k.selectHunk), ui.Describe(k.selectFile), ui.Describe(k.commit)}
	}
	review = append(review, ui.Describe(k.fold), ui.Describe(k.foldAll), ui.Describe(k.removed), ui.Describe(k.removedAll))
	return []helpSection{
		{title: "Navigation", hints: []ui.KeyHint{
			ui.Describe(k.switchPane),
			ui.Hint("next / previous line or file", k.Down, k.Up),
			ui.Hint("half page down / up", k.HalfDown, k.HalfUp),
			ui.Hint("first / last line or file", k.Top, k.Bottom),
			ui.Hint("next / previous hunk", k.nextHunk, k.prevHunk),
			ui.Hint("next / previous file", k.nextFile, k.prevFile),
			ui.Describe(k.workingCopy),
		}},
		{title: "Review", hints: review},
	}
}

func (m Model) reviewOtherHelp() []ui.KeyHint {
	var hints []ui.KeyHint
	if m.change.WorkingCopy {
		hints = append(hints, ui.Describe(m.reviewKeys.reload), ui.Describe(m.reviewKeys.edit))
	}
	return append(hints, ui.Describe(m.reviewKeys.full))
}

type reviewIntent int

const (
	noReviewIntent reviewIntent = iota
	reviewSwitchPaneIntent
	reviewDownIntent
	reviewUpIntent
	reviewHalfDownIntent
	reviewHalfUpIntent
	reviewTopIntent
	reviewBottomIntent
	reviewNextHunkIntent
	reviewPrevHunkIntent
	reviewNextFileIntent
	reviewPrevFileIntent
	reviewWorkingCopyIntent
	reviewReloadIntent
	reviewFullIntent
	reviewCommitIntent
	reviewEditIntent
	reviewFoldIntent
	reviewRemovedIntent
	reviewRemovedAllIntent
	reviewFoldAllIntent
	reviewSelectHunkIntent
	reviewSelectFileIntent
)

func (k reviewKeyMap) resolve(msg tea.KeyPressMsg) reviewIntent {
	switch {
	case key.Matches(msg, k.switchPane):
		return reviewSwitchPaneIntent
	case key.Matches(msg, k.Down):
		return reviewDownIntent
	case key.Matches(msg, k.Up):
		return reviewUpIntent
	case key.Matches(msg, k.HalfDown):
		return reviewHalfDownIntent
	case key.Matches(msg, k.HalfUp):
		return reviewHalfUpIntent
	case key.Matches(msg, k.Top):
		return reviewTopIntent
	case key.Matches(msg, k.Bottom):
		return reviewBottomIntent
	case key.Matches(msg, k.nextHunk):
		return reviewNextHunkIntent
	case key.Matches(msg, k.prevHunk):
		return reviewPrevHunkIntent
	case key.Matches(msg, k.nextFile):
		return reviewNextFileIntent
	case key.Matches(msg, k.prevFile):
		return reviewPrevFileIntent
	case key.Matches(msg, k.workingCopy):
		return reviewWorkingCopyIntent
	case key.Matches(msg, k.reload):
		return reviewReloadIntent
	case key.Matches(msg, k.full):
		return reviewFullIntent
	case key.Matches(msg, k.commit):
		return reviewCommitIntent
	case key.Matches(msg, k.edit):
		return reviewEditIntent
	case key.Matches(msg, k.fold):
		return reviewFoldIntent
	case key.Matches(msg, k.removed):
		return reviewRemovedIntent
	case key.Matches(msg, k.removedAll):
		return reviewRemovedAllIntent
	case key.Matches(msg, k.foldAll):
		return reviewFoldAllIntent
	case key.Matches(msg, k.selectHunk):
		return reviewSelectHunkIntent
	case key.Matches(msg, k.selectFile):
		return reviewSelectFileIntent
	}
	return noReviewIntent
}
