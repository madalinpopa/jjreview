package tui

import (
	"fmt"
	"iter"
	"slices"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

func (m *Model) reviewKey(msg tea.KeyPressMsg) tea.Cmd {
	m.status = ""
	armed := m.armed
	m.armed = ""
	switch {
	case key.Matches(msg, m.keys.quit):
		if m.confirmDrop(msg, armed) {
			return m.quit()
		}
	case key.Matches(msg, m.keys.logTab):
		return m.showLog()
	case key.Matches(msg, m.keys.bookmarksTab):
		return m.showBookmarks()
	case key.Matches(msg, m.keys.switchPane):
		m.filesFocus = !m.filesFocus
	case key.Matches(msg, m.keys.down):
		return m.move(1)
	case key.Matches(msg, m.keys.up):
		return m.move(-1)
	case key.Matches(msg, m.keys.halfDown):
		return m.page(m.halfPage())
	case key.Matches(msg, m.keys.halfUp):
		return m.page(-m.halfPage())
	case key.Matches(msg, m.keys.top):
		if m.filesFocus {
			return m.moveFile(-len(m.files))
		}
		m.cursor = firstRow(m.rows())
	case key.Matches(msg, m.keys.bottom):
		return m.move(m.extent())
	case key.Matches(msg, m.keys.nextHunk):
		m.jumpHunk(1)
	case key.Matches(msg, m.keys.prevHunk):
		m.jumpHunk(-1)
	case key.Matches(msg, m.keys.nextFile):
		return m.moveFile(1)
	case key.Matches(msg, m.keys.prevFile):
		return m.moveFile(-1)
	case key.Matches(msg, m.keys.workingCopy):
		if m.canLoad() && m.confirmDrop(msg, armed) {
			return m.loadRevision(workingCopyStep, "@")
		}
	case key.Matches(msg, m.keys.reload):
		if m.canReload() && m.confirmDrop(msg, armed) {
			return m.reload()
		}
	case key.Matches(msg, m.keys.full):
		m.full = !m.full
		m.cursor = firstRow(m.rows())
		return m.requestContent()
	case key.Matches(msg, m.keys.commit):
		return m.openModal()
	case key.Matches(msg, m.keys.edit):
		if path, ok := m.editablePath(); ok && m.confirmDrop(msg, armed) {
			return m.startEdit(path)
		}
	case key.Matches(msg, m.keys.help):
		m.help = true
		m.helpTop = 0
	default:
		m.hunkKey(msg)
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
	m.status = fmt.Sprintf("press %s again to drop %d selected %s", m.dropLabel(msg), n, plural(n, "hunk"))
	return false
}

func (m Model) dropLabel(msg tea.KeyPressMsg) string {
	for _, b := range []key.Binding{m.keys.quit, m.keys.workingCopy, m.keys.reload, m.keys.edit, m.keys.open} {
		if key.Matches(msg, b) {
			return b.Help().Key
		}
	}
	return msg.String()
}

func (m *Model) hunkKey(msg tea.KeyPressMsg) {
	if !m.ready() {
		return
	}
	switch {
	case key.Matches(msg, m.keys.fold):
		m.toggleFold()
	case key.Matches(msg, m.keys.removed):
		if m.filesFocus {
			m.status = m.keys.removed.Help().Key + " works in the diff pane; press " + m.keys.switchPane.Help().Key
			return
		}
		m.toggleRemoved()
	case key.Matches(msg, m.keys.removedAll):
		m.toggleRemovedAll()
	case key.Matches(msg, m.keys.foldAll):
		m.currentReview().foldAll()
		m.cursor = firstRow(m.rows())
	case key.Matches(msg, m.keys.selectHunk, m.keys.selectFile):
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
		if key.Matches(msg, m.keys.selectFile) || m.filesFocus {
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
