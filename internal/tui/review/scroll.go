package review

import (
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/madalinpopa/jjreview/internal/diff"
	"github.com/madalinpopa/jjreview/internal/tui/ui"
)

const (
	minNumberWidth = 4
	scrollStep     = 8
	hiddenLeft     = "‹"
	hiddenRight    = "›"
)

func numberWidth(rows []row) int {
	largest := 0
	for _, r := range rows {
		if r.kind == rowLine && r.line.Kind != diff.LineRemoved {
			largest = max(largest, r.line.NewNo)
		}
	}
	return max(len(strconv.Itoa(largest)), minNumberWidth)
}

func gutterWidth(number int) int {
	return number + 4
}

func (m Model) maxLeft(env Env) int {
	rows := m.rows(env)
	avail := max(m.diffCardWidth(env.Width)-2-gutterWidth(numberWidth(rows)), 0)
	if avail == 0 {
		return 0
	}
	longest := avail
	for _, r := range rows {
		if r.kind != rowLine {
			continue
		}
		if text := expandTabs(r.line.Text); len(text) > longest {
			longest = max(longest, lipgloss.Width(text))
		}
	}
	return longest - avail
}

func (m Model) diffOnly(b key.Binding) string {
	return b.Help().Key + " works in the diff pane; press " + m.keys.switchPane.Help().Key
}

func (m *Model) scrollCode(env Env, intent intent) string {
	b, step := m.keys.scrollRight, scrollStep
	if intent == scrollLeftIntent {
		b, step = m.keys.scrollLeft, -scrollStep
	}
	if m.filesFocus {
		return m.diffOnly(b)
	}
	limit := m.maxLeft(env)
	m.diffLeft = min(max(min(m.diffLeft, limit)+step, 0), limit)
	if limit == 0 {
		return fitsStatus
	}
	return ""
}

func (m Model) visibleCode(code []ui.Seg, avail int) (shown []ui.Seg, before, after bool) {
	expanded := make([]ui.Seg, len(code))
	for i, c := range code {
		expanded[i] = ui.Span(c.Style, expandTabs(c.Text))
	}
	total, left := ui.Width(expanded), min(m.diffLeft, m.overflow)
	before = left > 0 && total > 0
	if avail <= 0 || total-left <= avail {
		return window(expanded, left, avail), before, false
	}
	shown = window(expanded, left, avail-1)
	if pad := avail - 1 - ui.Width(shown); pad > 0 {
		shown = append(shown, ui.Span(lipgloss.NewStyle(), strings.Repeat(" ", pad)))
	}
	return shown, before, true
}

func window(segs []ui.Seg, left, width int) []ui.Seg {
	var out []ui.Seg
	col := 0
	for _, s := range segs {
		if text := cutText(s.Text, left-col, left+width-col); text != "" {
			out = append(out, ui.Span(s.Style, text))
		}
		col += lipgloss.Width(s.Text)
	}
	return out
}

func cutText(s string, from, to int) string {
	from = max(from, 0)
	if to <= from {
		return ""
	}
	if from == 0 && to >= len(s) {
		return s
	}
	kept := ansi.Truncate(s, to, "")
	width := ansi.StringWidth(kept)
	if from >= width {
		return ""
	}
	rest := ansi.TruncateLeft(kept, from, "")
	if ansi.StringWidth(rest) > width-from {
		return " " + ansi.TruncateLeft(kept, from+1, "")
	}
	return rest
}
