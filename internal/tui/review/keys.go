package review

import (
	"cmp"
	"fmt"
	"iter"
	"slices"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/madalinpopa/jjreview/internal/diff"
	"github.com/madalinpopa/jjreview/internal/tui/ui"
)

func (m *Model) Key(msg tea.KeyPressMsg, env Env) (tea.Cmd, Effect) {
	if m.modal != nil {
		return m.modalKey(msg, env), Effect{}
	}
	intent := m.keys.resolve(msg)
	if m.ranging {
		if status, done := m.rangeKey(env, intent); done {
			return nil, Effect{Status: status}
		}
	}
	switch intent {
	case switchPaneIntent:
		m.filesFocus = !m.filesFocus
	case downIntent:
		return m.move(env, 1), Effect{}
	case upIntent:
		return m.move(env, -1), Effect{}
	case halfDownIntent:
		return m.page(env, halfPage(env)), Effect{}
	case halfUpIntent:
		return m.page(env, -halfPage(env)), Effect{}
	case topIntent:
		if m.filesFocus {
			return m.moveFile(env, -len(m.files)), Effect{}
		}
		m.cursor = firstRow(m.rows(env))
	case bottomIntent:
		return m.move(env, m.extent(env)), Effect{}
	case nextHunkIntent:
		m.jumpHunk(env, 1)
	case prevHunkIntent:
		m.jumpHunk(env, -1)
	case nextFileIntent:
		return m.moveFile(env, 1), Effect{}
	case prevFileIntent:
		return m.moveFile(env, -1), Effect{}
	case workingCopyIntent:
		return nil, Effect{WorkingCopy: true}
	case reloadIntent:
		return nil, Effect{Reload: true}
	case editIntent:
		return nil, Effect{Edit: true}
	case fullIntent:
		m.full = !m.full
		m.cursor = firstRow(m.rows(env))
		return m.requestContent(env), Effect{}
	case commitIntent:
		cmd, status := m.openModal(env)
		return cmd, Effect{Status: status}
	default:
		return nil, Effect{Status: m.hunkKey(env, intent)}
	}
	return nil, Effect{}
}

func (m *Model) hunkKey(env Env, intent intent) string {
	if !m.ready(env) {
		return ""
	}
	switch intent {
	case foldIntent:
		m.toggleFold(env)
	case removedIntent:
		if m.filesFocus {
			return m.keys.removed.Help().Key + " works in the diff pane; press " + m.keys.switchPane.Help().Key
		}
		return m.toggleRemoved(env)
	case removedAllIntent:
		m.toggleRemovedAll(env)
	case foldAllIntent:
		m.currentReview().foldAll()
		m.cursor = firstRow(m.rows(env))
	case selectRangeIntent:
		return m.startRange(env)
	case selectLineIntent, selectHunkIntent, selectFileIntent:
		return m.toggleSelect(env, intent)
	}
	return ""
}

func selectBlocked(env Env) string {
	switch {
	case env.Reloading:
		return ReloadingStatus
	case env.Outdated:
		return OutdatedStatus
	case env.Change.Immutable:
		return immutableStatus
	}
	return ""
}

func (m Model) linesBlocked(b key.Binding) string {
	file, _ := m.currentFile()
	switch {
	case m.filesFocus:
		return b.Help().Key + " works in the diff pane; press " + m.keys.switchPane.Help().Key
	case wholeOnly(file):
		return wholeFileStatus + m.keys.selectFile.Help().Key
	}
	return ""
}

func (m *Model) startRange(env Env) string {
	if status := cmp.Or(selectBlocked(env), m.linesBlocked(m.keys.selectRange)); status != "" {
		return status
	}
	if m.cursor < 0 || m.cursor >= len(m.rows(env)) {
		return ""
	}
	m.ranging, m.anchor = true, m.cursor
	return ""
}

func (m *Model) rangeKey(env Env, intent intent) (string, bool) {
	switch intent {
	case downIntent, upIntent, halfDownIntent, halfUpIntent, topIntent, bottomIntent, nextHunkIntent, prevHunkIntent:
		return "", false
	case selectLineIntent, selectHunkIntent:
		m.ranging = false
		return m.selectRange(env), true
	}
	m.ranging = false
	return "", intent == selectRangeIntent || intent == cancelRangeIntent
}

func (m Model) rangeBounds(n int) (lo, hi int) {
	lo, hi = min(m.anchor, m.cursor), max(m.anchor, m.cursor)
	return max(lo, 0), min(hi, n-1)
}

func (m Model) inRange(i int) bool {
	if !m.ranging {
		return false
	}
	lo, hi := m.rangeBounds(i + 1)
	return i >= lo && i <= hi
}

func (m *Model) selectRange(env Env) string {
	if !m.ready(env) {
		return ""
	}
	if status := selectBlocked(env); status != "" {
		return status
	}
	before := m.rows(env)
	lo, hi := m.rangeBounds(len(before))
	file, _ := m.currentFile()
	review := m.currentReview()
	to := lineOpen
	var hunks []int
	for _, r := range before[lo : hi+1] {
		if r.kind != rowLine || r.hunk < 0 || r.index < 0 {
			continue
		}
		switch review.lines[r.hunk][r.index] {
		case lineOpen:
			to = linePicked
		case linePicked:
		default:
			continue
		}
		if !slices.Contains(hunks, r.hunk) {
			hunks = append(hunks, r.hunk)
		}
	}
	if len(hunks) == 0 {
		return rangeEmptyStatus
	}
	hidden := slices.DeleteFunc(hunks, review.showsRemoved)
	for _, r := range before[lo : hi+1] {
		if r.kind == rowLine && r.hunk >= 0 && r.index >= 0 && review.lines[r.hunk][r.index] != to {
			review.toggleLine(r.hunk, r.index)
		}
	}
	if slices.Equal(m.rows(env), before) {
		return ""
	}
	m.followRows(env, before)
	n := 0
	for _, k := range hidden {
		if review.showsRemoved(k) {
			n += openRemoved(file.Hunks[k], review.lines[k])
		}
	}
	if n > 0 {
		return removedNote(n)
	}
	return ""
}

func removedNote(n int) string {
	return fmt.Sprintf("%d removed %s not selected", n, ui.Plural(n, "line"))
}

func (m *Model) toggleSelect(env Env, intent intent) string {
	if status := selectBlocked(env); status != "" {
		return status
	}
	before := m.rows(env)
	status := ""
	switch {
	case intent == selectLineIntent:
		status = m.toggleLine(env, before)
	case intent == selectFileIntent || m.filesFocus:
		m.currentReview().toggleFile()
	default:
		m.currentReview().toggleUnit(m.cursorHunk(env))
	}
	if !slices.Equal(m.rows(env), before) {
		m.followRows(env, before)
	}
	return status
}

func (m *Model) toggleLine(env Env, rows []row) string {
	file, _ := m.currentFile()
	review := m.currentReview()
	if status := m.linesBlocked(m.keys.selectLine); status != "" {
		return status
	}
	if m.cursor < 0 || m.cursor >= len(rows) {
		return ""
	}
	r := rows[m.cursor]
	if r.kind != rowLine || r.hunk < 0 || r.index < 0 || review.lines[r.hunk][r.index] == lineContext {
		return fmt.Sprintf(lineKeyStatus, m.keys.selectLine.Help().Key, m.keys.selectHunk.Help().Key)
	}
	if !review.toggleLine(r.hunk, r.index) {
		return lineDoneStatus
	}
	if n := openRemoved(file.Hunks[r.hunk], review.lines[r.hunk]); n > 0 && len(m.rows(env)) > len(rows) {
		return removedNote(n)
	}
	return ""
}

func openRemoved(h diff.Hunk, states []lineState) int {
	n := 0
	for i, l := range h.Lines {
		if l.Kind == diff.LineRemoved && states[i] == lineOpen {
			n++
		}
	}
	return n
}

func (m Model) ready(env Env) bool {
	_, ok := m.currentFile()
	return env.Loaded && env.Err == nil && ok
}

func (m *Model) move(env Env, delta int) tea.Cmd {
	if m.filesFocus {
		return m.moveFile(env, delta)
	}
	m.moveCursor(env, delta)
	return nil
}

func (m *Model) page(env Env, delta int) tea.Cmd {
	if m.filesFocus {
		before := m.currentTreeRow()
		cmd := m.moveFile(env, delta)
		m.listTop = max(m.listTop+m.currentTreeRow()-before, 0)
		return cmd
	}
	before := m.cursor
	m.moveCursor(env, delta)
	m.diffTop = max(m.diffTop+m.cursor-before, 0)
	return nil
}

func (m Model) extent(env Env) int {
	return len(m.rows(env)) + len(m.files)
}

func (m *Model) jumpHunk(env Env, step int) {
	rows := m.rows(env)
	for i := m.cursor + step; i >= 0 && i < len(rows); i += step {
		if rows[i].hunk >= 0 && (i == 0 || rows[i-1].hunk != rows[i].hunk) {
			m.cursor = i
			return
		}
	}
}

func halfPage(env Env) int {
	return max(env.ListHeight/2, 1)
}

func (m *Model) moveCursor(env Env, delta int) {
	rows := m.rows(env)
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

func (m *Model) moveFile(env Env, delta int) tea.Cmd {
	next := min(max(m.fileIdx+delta, 0), max(len(m.files)-1, 0))
	if next == m.fileIdx {
		return nil
	}
	m.fileIdx = next
	m.cursor = firstRow(m.rows(env))
	return m.requestContent(env)
}

func (m Model) cursorHunk(env Env) int {
	rows := m.rows(env)
	if m.cursor < 0 || m.cursor >= len(rows) {
		return -1
	}
	return rows[m.cursor].hunk
}

func (m *Model) toggleFold(env Env) {
	k := m.cursorHunk(env)
	if k < 0 {
		return
	}
	folded := m.currentReview().folded
	folded[k] = !folded[k]
	for i, r := range m.rows(env) {
		if r.kind == rowHunk && r.hunk == k {
			m.cursor = i
			return
		}
	}
}

func (m *Model) keepCursor(env Env, before []row) {
	if m.cursor < 0 || m.cursor >= len(before) {
		return
	}
	after := m.rows(env)
	cur := before[m.cursor]
	if i := slices.Index(after, cur); i >= 0 {
		m.cursor = i
		return
	}
	if i := slices.IndexFunc(after, func(r row) bool { return r.kind == rowHunk && r.hunk == cur.hunk }); i >= 0 {
		m.cursor = i
		return
	}
	m.followRows(env, before)
}

func (m *Model) toggleRemoved(env Env) string {
	k := m.cursorHunk(env)
	file, _ := m.currentFile()
	review := m.currentReview()
	if k < 0 || !m.full && review.folded[k] || deletesOnly(file.Hunks[k]) {
		return ""
	}
	if hidesRemoved(file.Hunks[k]) && review.partial(k) {
		return partialRemovedStatus
	}
	before := m.rows(env)
	review.removed[k] = !review.removed[k]
	m.followRows(env, before)
	return ""
}

func (m *Model) toggleRemovedAll(env Env) {
	file, _ := m.currentFile()
	before := m.rows(env)
	m.currentReview().toggleRemovedAll(file, m.full)
	m.followRows(env, before)
}

func (m *Model) followRows(env Env, before []row) {
	if m.cursor < 0 || m.cursor >= len(before) {
		return
	}
	k := before[m.cursor].hunk
	shown := make(map[row]int)
	for j, r := range m.rows(env) {
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
