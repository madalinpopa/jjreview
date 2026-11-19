package tui

import (
	"iter"

	tea "charm.land/bubbletea/v2"
)

func (m *Model) reviewKey(msg tea.KeyPressMsg) tea.Cmd {
	m.status = ""
	switch msg.String() {
	case "q":
		return tea.Quit
	case "h":
		m.filesFocus = true
	case "l":
		m.filesFocus = false
	case "j", "down":
		return m.move(1)
	case "k", "up":
		return m.move(-1)
	case "ctrl+d":
		return m.page(m.halfPage())
	case "ctrl+u":
		return m.page(-m.halfPage())
	case "ctrl+n":
		return m.moveFile(1)
	case "ctrl+p":
		return m.moveFile(-1)
	case "f":
		m.full = !m.full
		m.cursor = firstRow(m.rows())
		return m.requestContent()
	case "c":
		return m.openModal()
	case "e":
		return m.openEditor()
	case "?":
		m.help = true
	default:
		m.hunkKey(msg.String())
	}
	return nil
}

func (m *Model) hunkKey(key string) {
	if !m.ready() {
		return
	}
	switch key {
	case "z", "enter":
		m.toggleFold()
	case "d":
		if !m.filesFocus {
			m.toggleRemoved()
		}
	case "Z":
		m.currentReview().foldAll()
		m.cursor = firstRow(m.rows())
	case "space", "S":
		if m.change.Immutable {
			m.status = immutableStatus
			return
		}
		if key == "S" || m.filesFocus {
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

func (m *Model) toggleRemoved() {
	k := m.cursorHunk()
	file, _ := m.currentFile()
	review := m.currentReview()
	if k < 0 || !m.full && review.folded[k] || deletesOnly(file.Hunks[k]) {
		return
	}
	before := m.rows()
	review.removed[k] = !review.removed[k]
	shown := make(map[row]int)
	for j, r := range m.rows() {
		if r.hunk != k {
			continue
		}
		if _, ok := shown[r]; !ok {
			shown[r] = j
		}
	}
	for i := range nearest(m.cursor, len(before)) {
		if before[i].hunk != k {
			continue
		}
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
