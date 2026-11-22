package tui

import (
	"fmt"
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/madalinpopa/jjreview/internal/tui/review"
	"github.com/madalinpopa/jjreview/internal/tui/tab"
	"github.com/madalinpopa/jjreview/internal/tui/ui"
)

const modalMaxWidth = 64

func (m Model) View() tea.View {
	content := m.screen()
	if box := m.overlay(); box != "" {
		x := max((m.width-lipgloss.Width(box))/2, 0)
		y := max((m.height-lipgloss.Height(box))/2, 0)
		content = lipgloss.NewCompositor(
			lipgloss.NewLayer(content),
			lipgloss.NewLayer(box).X(x).Y(y).Z(1),
		).Render()
	}
	v := tea.NewView(fit(content, m.width, m.height))
	v.AltScreen = true
	v.ReportFocus = m.autoTheme()
	return v
}

func fit(content string, width, height int) string {
	lines := strings.Split(content, "\n")
	lines = lines[:min(len(lines), max(height, 0))]
	for i, l := range lines {
		lines[i] = ui.Clip(l, width)
	}
	return strings.Join(lines, "\n")
}

func (m Model) screen() string {
	var body string
	if t, ok := m.active(); ok {
		body = t.tab.View(m.env(t.view))
	} else {
		body = m.review.View(m.reviewEnv())
	}
	return strings.Join(append([]string{m.topBar(), body}, m.bottomBar()...), "\n")
}

func (m Model) bodyHeight() int {
	return max(m.height-1-len(m.bottomBar()), 0)
}

func (m Model) diffHeight() int {
	return max(m.bodyHeight()-ui.CardChrome, 1)
}

func (m Model) topBar() string {
	p := m.pal
	id := m.target
	if id == "" {
		id = m.rev
	}
	gap := ui.Span(lipgloss.NewStyle(), " ")
	revision := ui.Pill(p.Chip, p.ChipText, "@ "+ui.ShortID(id))
	n := m.review.Picked()
	var indicators, optional []ui.Seg
	if m.change.Immutable {
		indicators = append(indicators, ui.Pill(p.Chip, p.Removed, "read only"))
	}
	switch {
	case m.view == reviewView && n > 0:
		indicators = append(indicators, bold(ui.Pill(p.BadgeSel, p.Picked, fmt.Sprintf("%d %s selected%s", n, ui.Plural(n, "hunk"), m.review.LineNote()))))
	case m.view == reviewView:
		optional = append(optional, bold(ui.Pill(p.BadgeIdle, p.BadgeIdleFg, "nothing selected")))
	case n > 0:
		indicators = append(indicators, bold(ui.Pill(p.BadgeSel, p.Picked, fmt.Sprintf("● %d", n))))
	}
	var title []ui.Seg
	if subject, _, _ := strings.Cut(strings.TrimSpace(m.change.Description), "\n"); subject != "" {
		title = []ui.Seg{gap, ui.Span(ui.Fg(p.Title).Bold(true), subject)}
	}
	fits := func(left, right, title []ui.Seg) bool {
		return ui.RowWidth(slices.Concat(left, title), spaced(right))+1 <= m.width
	}
	bar := func(left, right []ui.Seg) string {
		return ui.Paint(m.width, p.Page, slices.Concat(left, title), spaced(right))
	}
	left := spaced(append(m.tabs(false), revision))
	chips := slices.Concat(optional, indicators)
	for drop := range len(optional) + 1 {
		if fits(left, chips[drop:], title) {
			return bar(left, chips[drop:])
		}
	}
	if fits(left, indicators, nil) {
		return bar(left, indicators)
	}
	left = spaced(append(m.tabs(true), revision))
	for drop := range len(indicators) {
		if fits(left, indicators[drop:], nil) {
			return bar(left, indicators[drop:])
		}
	}
	return bar(left, nil)
}

func bold(s ui.Seg) ui.Seg {
	s.Style = s.Style.Bold(true)
	return s
}

func spaced(segs []ui.Seg) []ui.Seg {
	var out []ui.Seg
	for i, s := range segs {
		if i > 0 {
			out = append(out, ui.Span(lipgloss.NewStyle(), " "))
		}
		out = append(out, s)
	}
	return out
}

func (m Model) tabs(short bool) []ui.Seg {
	tabs := append([]tabEntry{{view: reviewView, label: "Review", key: m.keys.reviewTab}}, m.registry()...)
	segs := make([]ui.Seg, len(tabs))
	for i, t := range tabs {
		label := t.label
		if short {
			label = ""
		}
		segs[i] = m.tab(t.view, t.key, label)
	}
	return segs
}

func (m Model) tab(v view, b key.Binding, label string) ui.Seg {
	text := strings.TrimSpace(b.Help().Key + " " + label)
	if v == m.view {
		s := ui.Pill(m.pal.Chip, m.pal.Key, text)
		s.Style = s.Style.Bold(true)
		return s
	}
	return ui.Pill(m.pal.Chip, m.pal.ChipText, text)
}

func (m Model) bottomBar() []string {
	p := m.pal
	sep := ui.Span(ui.Fg(p.Faint), " · ")
	var lines [][]ui.Seg
	var line []ui.Seg
	used := 0
	for _, h := range m.keyHints() {
		segs := ui.HintSegs(m.pal, h)
		w := lipgloss.Width(ui.Render(p.Page, segs))
		switch {
		case len(line) == 0:
		case used+lipgloss.Width(sep.Text)+w <= m.width:
			line = append(line, sep)
			used += lipgloss.Width(sep.Text)
		default:
			lines = append(lines, line)
			line, used = nil, 0
		}
		line = append(line, segs...)
		used += w
	}
	status := []ui.Seg{ui.Span(ui.Fg(p.Status), m.statusText())}
	out := make([]string, 0, len(lines)+2)
	for _, l := range lines {
		out = append(out, ui.Paint(m.width, p.Page, l, nil))
	}
	if used+1+lipgloss.Width(m.statusText()) <= m.width {
		return append(out, ui.Paint(m.width, p.Page, line, status))
	}
	return append(out, ui.Paint(m.width, p.Page, line, nil), ui.Paint(m.width, p.Page, nil, status))
}

func (m Model) statusText() string {
	if m.status == "" && m.outdated {
		return review.OutdatedStatus
	}
	if m.status != "" || m.view != reviewView {
		return m.status
	}
	return m.review.Status(review.Env{Change: m.change, Loaded: m.loaded, Err: m.err})
}

func (m Model) modalWidth() int {
	return min(modalMaxWidth, max(m.width-4, 20))
}

func (m Model) layout() ui.Layout {
	return ui.Layout{
		Theme:      m.pal,
		Width:      m.width,
		Height:     m.height,
		BodyHeight: m.bodyHeight(),
		ListHeight: m.diffHeight(),
		ModalWidth: m.modalWidth(),
	}
}

func (m Model) head() tab.Head {
	return tab.Head{ChangeID: m.target, CommitID: m.targetHead, Title: m.headTitle(), WorkingCopy: m.change.WorkingCopy}
}

func (m Model) env(v view) tab.Env {
	return tab.Env{
		Layout:  m.layout(),
		Head:    m.head(),
		Active:  m.view == v,
		Dropped: m.review.Picked(),
		Lock:    m.lock(),
	}
}
