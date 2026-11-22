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

const noSearchStatus = "press %s to search"

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
	m.needle, m.hit, m.seek = "", nil, 0
}

func (m *Model) searchKey(msg tea.KeyPressMsg, env Env) (tea.Cmd, Effect) {
	switch m.queryKeys.resolve(msg) {
	case cancelSearchIntent:
		m.closeSearch()
	case submitSearchIntent:
		needle := m.query.Value()
		m.closeSearch()
		m.clearSearch()
		if needle == "" {
			return nil, Effect{}
		}
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
	if w := max(m.diffCardWidth(env.Width)-5, 1); m.query.Width() != w {
		m.query.SetWidth(w)
	}
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
	return m.nextFile(env, step), Effect{}
}

func (m *Model) nextFile(env Env, step int) tea.Cmd {
	n := len(m.files)
	for k := 1; k <= n; k++ {
		i := ((m.fileIdx+step*k)%n + n) % n
		if len(m.matches(m.rowsAt(env, i))) == 0 {
			continue
		}
		cmd := m.moveFile(env, i-m.fileIdx)
		m.seek = step
		m.seekMatch(env)
		return cmd
	}
	return nil
}

func (m Model) rowsAt(env Env, i int) []row {
	m.fileIdx = i
	if m.requestContent(env) != nil {
		m.full = false
	}
	return m.rows(env)
}

func (m *Model) seekMatch(env Env) {
	if m.seek == 0 || m.requestContent(env) != nil {
		return
	}
	rows := m.rows(env)
	edge := -1
	if m.seek < 0 {
		edge = len(rows)
	}
	if x, ok := after(m.matches(rows), edge, edge, m.seek); ok {
		m.land(env, rows, x)
	}
	m.seek = 0
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
	rows := m.rows(env)
	ms := m.matches(rows)
	at := slices.IndexFunc(ms, func(x match) bool { return m.hitAt(rows[x.at]) == x.nth })
	switch {
	case at >= 0:
		return fmt.Sprintf("/%s %d/%d", m.needle, at+1, len(ms))
	case len(ms) == 0:
		return "/" + m.needle + " no matches"
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
