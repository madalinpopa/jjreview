package ui

import (
	"image/color"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/madalinpopa/jjreview/internal/theme"
)

const (
	toastMaxWidth    = 48
	toastMinWidth    = 24
	toastResultLines = 3
	toastExpiry      = 3 * time.Second
)

type ToastResult struct {
	Text    string
	Warning string
	Err     error
}

type toastState int

const (
	toastHidden toastState = iota
	toastRunning
	toastDone
)

type toastTickMsg struct{ id int }

type toastExpireMsg struct{ id int }

type Toast struct {
	seq    int
	state  toastState
	title  string
	label  string
	result ToastResult
	frame  int
	after  func(time.Duration, tea.Msg) tea.Cmd
}

func NewToast(after func(time.Duration, tea.Msg) tea.Cmd) Toast {
	return Toast{after: after}
}

func (t *Toast) Start(title, label string) (int, tea.Cmd) {
	t.seq++
	t.state = toastRunning
	t.title, t.label = title, label
	t.result = ToastResult{}
	t.frame = 0
	return t.seq, t.after(spinner.MiniDot.FPS, toastTickMsg{id: t.seq})
}

func (t *Toast) Finish(id int, r ToastResult) (tea.Cmd, bool) {
	if id != t.seq || t.state != toastRunning {
		return nil, false
	}
	t.state = toastDone
	t.result = r
	if t.persists() {
		return nil, true
	}
	return t.after(toastExpiry, toastExpireMsg{id: id}), true
}

func (t *Toast) Update(msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case toastTickMsg:
		if msg.id != t.seq || t.state != toastRunning {
			return nil, true
		}
		t.frame = (t.frame + 1) % len(spinner.MiniDot.Frames)
		return t.after(spinner.MiniDot.FPS, toastTickMsg{id: msg.id}), true
	case toastExpireMsg:
		if msg.id == t.seq && t.state == toastDone {
			t.state = toastHidden
		}
		return nil, true
	}
	return nil, false
}

func (t *Toast) Dismiss() bool {
	if t.state != toastDone {
		return false
	}
	t.state = toastHidden
	return true
}

func (t Toast) Visible() bool {
	return t.state != toastHidden
}

func (t Toast) persists() bool {
	return t.result.Err != nil || t.result.Warning != ""
}

func (t Toast) Summary() string {
	switch t.state {
	case toastRunning:
		return Printable(t.title) + "…"
	case toastHidden:
		return ""
	}
	return t.result.Summary()
}

func (r ToastResult) Summary() string {
	if r.Err != nil {
		return firstLine(r.Err.Error())
	}
	parts := []string{Printable(r.Text), firstLine(r.Warning)}
	return strings.Join(slices.DeleteFunc(parts, func(s string) bool { return s == "" }), " · ")
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return Printable(line)
}

func (t Toast) View(p theme.Theme, width int, dismiss KeyHint) string {
	if t.state == toastHidden || width < toastMinWidth {
		return ""
	}
	inner := min(toastMaxWidth, width-2) - 4
	glyph, edge := spinner.MiniDot.Frames[t.frame], p.Selection
	switch {
	case t.state == toastRunning:
	case t.result.Err != nil:
		glyph, edge = "✗", p.Removed
	case t.result.Warning != "":
		glyph, edge = "!", p.Key
	default:
		glyph, edge = "✓", p.Added
	}
	lines := []string{
		Paint(inner, p.Card, []Seg{Span(Fg(edge).Bold(true), glyph+" "), Span(Fg(p.Title).Bold(true), ansi.Truncate(Printable(t.title), inner-2, "…"))}, nil),
		Paint(inner, p.Card, []Seg{Span(Fg(p.Muted), ansi.Truncate(Printable(t.label), inner, "…"))}, nil),
	}
	if t.state == toastDone {
		lines = append(lines, TruncateLines(t.resultLines(p, inner), toastResultLines, Ellipsis(p, inner))...)
		if t.persists() {
			lines = append(lines, Paint(inner, p.Card, nil, HintSegs(p, dismiss)))
		}
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(edge).
		BorderBackground(p.Card).
		Background(p.Card).
		Padding(0, 1).
		Render(strings.Join(lines, "\n"))
}

func (t Toast) resultLines(p theme.Theme, width int) []string {
	var out []string
	add := func(c color.Color, text string) {
		for line := range strings.SplitSeq(text, "\n") {
			if strings.TrimSpace(line) == "" {
				continue
			}
			wrapped := ansi.Hardwrap(ansi.Wrap(Printable(line), width, ""), width, true)
			for part := range strings.SplitSeq(wrapped, "\n") {
				out = append(out, Paint(width, p.Card, []Seg{Span(Fg(c), part)}, nil))
			}
		}
	}
	if t.result.Err != nil {
		line, _, _ := strings.Cut(t.result.Err.Error(), "\n")
		add(p.Removed, line)
		return out
	}
	add(p.Text, t.result.Text)
	add(p.Key, t.result.Warning)
	return out
}

func Printable(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsControl(r) {
			q := strconv.QuoteRune(r)
			b.WriteString(q[1 : len(q)-1])
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
