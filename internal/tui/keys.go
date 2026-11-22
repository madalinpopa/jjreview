package tui

import (
	"iter"
	"slices"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/madalinpopa/jjreview/internal/tui/tab"
)

func (m *Model) reviewKey(msg tea.KeyPressMsg) tea.Cmd {
	m.status = ""
	armed := m.armed
	m.armed = ""
	if cmd, ok := m.globalKey(msg, armed); ok {
		return cmd
	}
	intent := m.reviewKeys.resolve(msg)
	switch intent {
	case reviewSwitchPaneIntent:
		m.filesFocus = !m.filesFocus
	case reviewDownIntent:
		return m.move(1)
	case reviewUpIntent:
		return m.move(-1)
	case reviewHalfDownIntent:
		return m.page(m.halfPage())
	case reviewHalfUpIntent:
		return m.page(-m.halfPage())
	case reviewTopIntent:
		if m.filesFocus {
			return m.moveFile(-len(m.files))
		}
		m.cursor = firstRow(m.rows())
	case reviewBottomIntent:
		return m.move(m.extent())
	case reviewNextHunkIntent:
		m.jumpHunk(1)
	case reviewPrevHunkIntent:
		m.jumpHunk(-1)
	case reviewNextFileIntent:
		return m.moveFile(1)
	case reviewPrevFileIntent:
		return m.moveFile(-1)
	case reviewWorkingCopyIntent:
		if m.canLoad() && m.confirmDrop(msg, armed) {
			return m.loadRevision(workingCopyStep, "@")
		}
	case reviewReloadIntent:
		if m.canReload() && m.confirmDrop(msg, armed) {
			return m.reload()
		}
	case reviewFullIntent:
		m.full = !m.full
		m.cursor = firstRow(m.rows())
		return m.requestContent()
	case reviewCommitIntent:
		return m.openModal()
	case reviewEditIntent:
		if path, ok := m.editablePath(); ok && m.confirmDrop(msg, armed) {
			return m.startEdit(path)
		}
	default:
		m.hunkKey(intent)
	}
	return nil
}

func (m *Model) confirmDrop(msg tea.KeyPressMsg, armed string) bool {
	n := pickedUnits(m.picks())
	k := msg.String()
	if n == 0 || k == armed {
		return true
	}
	m.armed = k
	m.status = tab.DropPrompt(m.dropLabel(msg), n)
	return false
}

func (m Model) dropLabel(msg tea.KeyPressMsg) string {
	for _, b := range []key.Binding{m.keys.quit, m.reviewKeys.workingCopy, m.reviewKeys.reload, m.reviewKeys.edit, m.keys.open} {
		if key.Matches(msg, b) {
			return b.Help().Key
		}
	}
	return msg.String()
}

func (m *Model) hunkKey(intent reviewIntent) {
	if !m.ready() {
		return
	}
	switch intent {
	case reviewFoldIntent:
		m.toggleFold()
	case reviewRemovedIntent:
		if m.filesFocus {
			m.status = m.reviewKeys.removed.Help().Key + " works in the diff pane; press " + m.reviewKeys.switchPane.Help().Key
			return
		}
		m.toggleRemoved()
	case reviewRemovedAllIntent:
		m.toggleRemovedAll()
	case reviewFoldAllIntent:
		m.currentReview().foldAll()
		m.cursor = firstRow(m.rows())
	case reviewSelectHunkIntent, reviewSelectFileIntent:
		if m.reloading {
			m.status = reloadingStatus
			return
		}
		if m.outdated {
			m.status = outdatedStatus
			return
		}
		if m.change.Immutable {
			m.status = immutableStatus
			return
		}
		if intent == reviewSelectFileIntent || m.filesFocus {
			m.currentReview().toggleFile()
			return
		}
		m.currentReview().toggleUnit(m.cursorHunk())
	}
}

func (m Model) ready() bool {
	_, ok := m.currentFile()
	return m.loaded && m.err == nil && ok
}

func (m *Model) move(delta int) tea.Cmd {
	if m.filesFocus {
		return m.moveFile(delta)
	}
	m.moveCursor(delta)
	return nil
}

func (m *Model) page(delta int) tea.Cmd {
	if m.filesFocus {
		before := m.currentTreeRow()
		cmd := m.moveFile(delta)
		m.listTop = max(m.listTop+m.currentTreeRow()-before, 0)
		return cmd
	}
	before := m.cursor
	m.moveCursor(delta)
	m.diffTop = max(m.diffTop+m.cursor-before, 0)
	return nil
}

func (m Model) extent() int {
	return len(m.rows()) + len(m.files)
}

func (m *Model) jumpHunk(step int) {
	rows := m.rows()
	for i := m.cursor + step; i >= 0 && i < len(rows); i += step {
		if rows[i].hunk >= 0 && (i == 0 || rows[i-1].hunk != rows[i].hunk) {
			m.cursor = i
			return
		}
	}
}

func (m Model) halfPage() int {
	return max(m.diffHeight()/2, 1)
}

func (m *Model) moveCursor(delta int) {
	rows := m.rows()
	if len(rows) == 0 || delta == 0 {
		return
	}
	step := 1
	if delta < 0 {
		step = -1
	}
	target := min(max(m.cursor+delta, 0), len(rows)-1)
	if target == m.cursor {
		return
	}
	for i := target; i >= 0 && i < len(rows); i += step {
		if rows[i].kind != rowGap {
			m.cursor = i
			return
		}
	}
	for i := target - step; i != m.cursor && i >= 0 && i < len(rows); i -= step {
		if rows[i].kind != rowGap {
			m.cursor = i
			return
		}
	}
}

func (m *Model) moveFile(delta int) tea.Cmd {
	next := min(max(m.fileIdx+delta, 0), max(len(m.files)-1, 0))
	if next == m.fileIdx {
		return nil
	}
	m.fileIdx = next
	m.cursor = firstRow(m.rows())
	return m.requestContent()
}

func (m Model) cursorHunk() int {
	rows := m.rows()
	if m.cursor < 0 || m.cursor >= len(rows) {
		return -1
	}
	return rows[m.cursor].hunk
}

func (m *Model) toggleFold() {
	k := m.cursorHunk()
	if k < 0 {
		return
	}
	folded := m.currentReview().folded
	folded[k] = !folded[k]
	for i, r := range m.rows() {
		if r.kind == rowHunk && r.hunk == k {
			m.cursor = i
			return
		}
	}
}

func (m *Model) keepCursor(before []row) {
	after := m.rows()
	if len(after) == len(before) || m.cursor < 0 || m.cursor >= len(before) {
		return
	}
	folded := m.currentReview().folded
	hidden := func(r row) bool { return r.kind == rowLine && r.hunk >= 0 && folded[r.hunk] }
	if cur := before[m.cursor]; hidden(cur) {
		m.cursor = slices.IndexFunc(after, func(r row) bool { return r.kind == rowHunk && r.hunk == cur.hunk })
		return
	}
	shift := 0
	for _, r := range before[:m.cursor] {
		if hidden(r) {
			shift++
		}
	}
	m.cursor -= shift
}

func (m *Model) toggleRemoved() {
	k := m.cursorHunk()
	file, _ := m.currentFile()
	review := m.currentReview()
	if k < 0 || !m.full && review.folded[k] || deletesOnly(file.Hunks[k]) {
		return
	}
	before := m.rows()
	review.removed[k] = !review.removed[k]
	m.followRows(before)
}

func (m *Model) toggleRemovedAll() {
	file, _ := m.currentFile()
	before := m.rows()
	m.currentReview().toggleRemovedAll(file, m.full)
	m.followRows(before)
}

func (m *Model) followRows(before []row) {
	if m.cursor < 0 || m.cursor >= len(before) {
		return
	}
	k := before[m.cursor].hunk
	shown := make(map[row]int)
	for j, r := range m.rows() {
		if _, ok := shown[r]; !ok && r.kind != rowGap && r.hunk == k {
			shown[r] = j
		}
	}
	for i := range nearest(m.cursor, len(before)) {
		if j, ok := shown[before[i]]; ok {
			m.cursor = j
			return
		}
	}
}

func nearest(from, n int) iter.Seq[int] {
	return func(yield func(int) bool) {
		for i := from; i < n; i++ {
			if !yield(i) {
				return
			}
		}
		for i := from - 1; i >= 0; i-- {
			if !yield(i) {
				return
			}
		}
	}
}
