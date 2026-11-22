package review

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/madalinpopa/jjreview/internal/theme"
	"github.com/madalinpopa/jjreview/internal/tui/ui"
)

const (
	noSearchStatus  = "press %s to search"
	noMatchesStatus = "/%s no matches"
)

type hit struct {
	file int
	row  row
	nth  int
}

type match struct {
	at  int
	nth int
}

func (m Model) Searching() bool {
	return m.searching
}

func (m *Model) openSearch(env Env) tea.Cmd {
	if !m.ready(env) {
		return nil
	}
	m.searching = true
	m.query.Reset()
	return m.query.Focus()
}

func (m *Model) closeSearch() {
	m.searching = false
	m.query.Blur()
	m.query.Reset()
}

func (m *Model) clearSearch() {
	m.needle, m.hit = "", nil
}

func (m *Model) searchKey(msg tea.KeyPressMsg, env Env) (tea.Cmd, Effect) {
	switch m.queryKeys.resolve(msg) {
	case cancelSearchIntent:
		m.closeSearch()
	case submitSearchIntent:
		needle := m.query.Value()
		m.closeSearch()
		if needle == "" {
			return nil, Effect{}
		}
		m.clearSearch()
		m.needle = needle
		return m.findNext(env, 1)
	default:
		return m.queryInput(msg), Effect{}
	}
	return nil, Effect{}
}

func (m *Model) queryInput(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	m.query, cmd = m.query.Update(msg)
	return cmd
}

func (m *Model) resizeQuery(env Env) {
	q := &m.query
	w := max(m.diffCardWidth(env.Width)-6, 1)
	if q.Width() == w {
		return
	}
	value, pos := q.Value(), q.Position()
	q.SetWidth(w)
	q.SetValue("")
	q.SetValue(value)
	q.SetCursor(pos)
}

func (m Model) diffHeight(env Env) int {
	if m.searching {
		return max(env.ListHeight-1, 1)
	}
	return env.ListHeight
}

func fold(s string) []rune {
	return []rune(strings.Map(unicode.ToLower, expandTabs(s)))
}

func occurrences(text string, query []rune) [][2]int {
	n := len(query)
	if n == 0 {
		return nil
	}
	hay := fold(text)
	var out [][2]int
	for i := 0; i+n <= len(hay); {
		if slices.Equal(hay[i:i+n], query) {
			out = append(out, [2]int{i, i + n})
			i += n
			continue
		}
		i++
	}
	return out
}

func (m Model) spans(r row) [][2]int {
	if m.needle == "" || r.kind != rowLine {
		return nil
	}
	return occurrences(r.line.Text, fold(m.needle))
}

func (m Model) matches(rows []row) []match {
	var out []match
	for i, r := range rows {
		for n := range len(m.spans(r)) {
			out = append(out, match{at: i, nth: n})
		}
	}
	return out
}

func after(ms []match, at, nth, step int) (match, bool) {
	if step < 0 {
		for _, x := range slices.Backward(ms) {
			if x.at < at || x.at == at && x.nth < nth {
				return x, true
			}
		}
		return match{}, false
	}
	for _, x := range ms {
		if x.at > at || x.at == at && x.nth > nth {
			return x, true
		}
	}
	return match{}, false
}

func (m Model) hitAt(r row) int {
	if m.hit == nil || m.hit.file != m.fileIdx || m.hit.row != r {
		return -1
	}
	return m.hit.nth
}

func (m *Model) findNext(env Env, step int) (tea.Cmd, Effect) {
	if m.needle == "" {
		return nil, Effect{Status: fmt.Sprintf(noSearchStatus, m.keys.search.Help().Key)}
	}
	if !m.ready(env) || m.loading(env) {
		return nil, Effect{}
	}
	rows := m.rows(env)
	nth := math.MaxInt
	if step > 0 {
		nth = -1
	}
	if m.cursor >= 0 && m.cursor < len(rows) {
		if k := m.hitAt(rows[m.cursor]); k >= 0 {
			nth = k
		}
	}
	if x, ok := after(m.matches(rows), m.cursor, nth, step); ok {
		m.land(env, rows, x)
		return nil, Effect{}
	}
	return m.scan(env, step)
}

func (m *Model) scan(env Env, step int) (tea.Cmd, Effect) {
	n := len(m.files)
	for k := 1; k <= n; k++ {
		i := ((m.fileIdx+step*k)%n + n) % n
		rows := m.rowsOf(env, i)
		edge := edgeOf(rows, step)
		x, ok := after(m.matches(rows), edge, edge, step)
		if !ok {
			continue
		}
		cmd := m.moveFile(env, i-m.fileIdx)
		if !m.loading(env) {
			m.land(env, rows, x)
		}
		return cmd, Effect{}
	}
	return nil, Effect{Status: fmt.Sprintf(noMatchesStatus, m.needle)}
}

func (m Model) rowsOf(env Env, i int) []row {
	if i == m.fileIdx {
		return m.rows(env)
	}
	m.fileIdx = i
	if m.failed(env) {
		return nil
	}
	if !m.loading(env) {
		return m.rows(env)
	}
	file, _ := m.currentFile()
	r := m.currentReview()
	r.folded = make([]bool, len(r.folded))
	return changeRows(file, r, -1)
}

func edgeOf(rows []row, step int) int {
	if step < 0 {
		return len(rows)
	}
	return -1
}

func (m Model) failed(env Env) bool {
	key, ok := m.uncached(env)
	_, failed := m.contentErrs[key]
	return ok && failed
}

func (m Model) loading(env Env) bool {
	_, ok := m.uncached(env)
	return ok && !m.failed(env)
}

func (m *Model) land(env Env, rows []row, x match) {
	r := rows[x.at]
	m.cursor = x.at
	m.hit = new(hit{file: m.fileIdx, row: r, nth: x.nth})
	span := m.spans(r)[x.nth]
	text := []rune(expandTabs(r.line.Text))
	from, to := lipgloss.Width(string(text[:span[0]])), lipgloss.Width(string(text[:span[1]]))
	avail := max(m.diffCardWidth(env.Width)-2-gutterWidth(numberWidth(rows)), 1)
	limit := m.maxLeft(env)
	left := max(min(m.diffLeft, limit), to-avail+1)
	m.diffLeft = min(max(min(left, from), 0), limit)
}

func (m Model) searchStatus(env Env) string {
	if m.loading(env) {
		return "/" + m.needle
	}
	rows := m.rows(env)
	ms := m.matches(rows)
	at := slices.IndexFunc(ms, func(x match) bool { return m.hitAt(rows[x.at]) == x.nth })
	switch {
	case at >= 0:
		return fmt.Sprintf("/%s %d/%d", m.needle, at+1, len(ms))
	case len(ms) == 0:
		return fmt.Sprintf(noMatchesStatus, m.needle)
	case len(ms) == 1:
		return "/" + m.needle + " 1 match"
	}
	return fmt.Sprintf("/%s %d matches", m.needle, len(ms))
}

func (m Model) markMatches(p theme.Theme, r row, code []ui.Seg) []ui.Seg {
	spans := m.spans(r)
	if len(spans) == 0 {
		return code
	}
	current := m.hitAt(r)
	var out []ui.Seg
	pos := 0
	for _, s := range code {
		text := []rune(expandTabs(s.Text))
		for i := 0; i < len(text); {
			k, end := spanAt(spans, pos+i)
			next := min(end-pos, len(text))
			style := s.Style
			switch {
			case k < 0:
			case k == current:
				style = style.Foreground(p.OnSelection).Background(p.Selection).Bold(true)
			default:
				style = style.Foreground(p.OnSelection).Background(p.Key)
			}
			out = append(out, ui.Span(style, string(text[i:next])))
			i = next
		}
		pos += len(text)
	}
	return out
}

func spanAt(spans [][2]int, i int) (int, int) {
	for k, s := range spans {
		if i < s[0] {
			return -1, s[0]
		}
		if i < s[1] {
			return k, s[1]
		}
	}
	return -1, math.MaxInt
}
