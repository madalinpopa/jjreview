package tui

import (
	"errors"
	"fmt"
	"image/color"
	"strconv"
	"strings"
	"unicode"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/madalinpopa/jjreview/internal/diff"
	"github.com/madalinpopa/jjreview/internal/jj"
)

const (
	modalListRows      = 4
	modalChrome        = 10
	modalCompactChrome = 7
	modalNoteRows      = 3
)

type modalRows struct {
	preview       int
	input         int
	notes         int
	noteInHeading bool
}

func (m Model) modalLayout() modalRows {
	notes := len(m.noteSegs(m.modalInner()))
	rows := modalRows{preview: 1}
	chrome := modalChrome
	if m.height < modalChrome+2+min(notes, modalNoteRows) {
		rows.preview, chrome = 0, modalCompactChrome
	}
	room := max(m.height-chrome-1-rows.preview, 0)
	if notes > 0 && room == 0 {
		rows.notes, rows.noteInHeading = 1, true
		rows.input = 1
		return rows
	}
	rows.notes = min(notes, modalNoteRows, room)
	room -= rows.notes
	rows.input = 1 + min(inputHeight-1, room)
	room -= rows.input - 1
	if rows.preview > 0 {
		rows.preview += min(max(min(m.pickCount(), modalListRows)-1, 0), room)
		room -= rows.preview - 1
	}
	rows.notes += min(notes-rows.notes, room)
	return rows
}

func (m Model) modalInner() int {
	return max(m.modalWidth()-4, 1)
}

func (m Model) modalView() string {
	p := m.pal
	mo := m.modal
	inner := m.modalInner()
	rows := m.modalLayout()
	lines := []string{
		paint(inner, p.Card, []seg{
			span(fg(p.Title).Bold(true), "New commit "),
			m.pill(p.Chip, p.ChipText, "from "+shortID(m.target)),
		}, nil),
	}
	if rows.preview > 0 {
		lines = append(lines,
			blank(inner, p.Card),
			m.box(inner, p.Border, strings.Join(m.pickLines(boxInner(inner), rows.preview), "\n")),
		)
	}
	heading := paint(inner, p.Card, []seg{
		span(fg(p.Header), "Commit message "),
		m.generatorNote(),
	}, nil)
	notes := m.noteLines(inner, rows.notes)
	if rows.noteInHeading {
		heading, notes = notes[0], nil
	}
	lines = append(lines, heading, m.box(inner, p.Selection, mo.input.View()))
	lines = append(lines, notes...)
	lines = append(lines, paint(inner, p.Card, nil, m.modalFooter(inner)))
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(p.ModalBorder).
		BorderBackground(p.Card).
		Background(p.Card).
		Padding(0, 1).
		Render(strings.Join(lines, "\n"))
}

func boxInner(width int) int {
	return max(width-4, 1)
}

func (m Model) box(width int, border color.Color, content string) string {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(border).
		BorderBackground(m.pal.Card).
		Background(m.pal.Input).
		Padding(0, 1).
		Width(width).
		Render(content)
}

func (m Model) pickCount() int {
	n := 0
	for _, pk := range m.modal.picks {
		if wholeOnly(m.files[pk.file]) {
			n++
			continue
		}
		n += len(pk.units)
	}
	return n
}

func (m Model) pickItems(limit int) [][]seg {
	p := m.pal
	var items [][]seg
	for _, pk := range m.modal.picks {
		f := m.files[pk.file]
		name := span(fg(p.Strong), escapePath(f.Path()))
		if wholeOnly(f) {
			if len(items) == limit {
				return items
			}
			items = append(items, []seg{name, span(fg(p.Header), "  "+wholeLabel(f))})
			continue
		}
		for _, k := range pk.units {
			if len(items) == limit {
				return items
			}
			a, d := hunkStats(f.Hunks[k])
			detail := fmt.Sprintf("  %s  +%d −%d", hunkHeader(f.Hunks[k]), a, d)
			items = append(items, []seg{name, span(fg(p.Header), detail)})
		}
	}
	return items
}

func (m Model) pickLines(width, rows int) []string {
	p := m.pal
	count := m.pickCount()
	var items [][]seg
	if count > rows {
		items = append(m.pickItems(rows-1), []seg{span(fg(p.Muted), fmt.Sprintf("… %d more", count-rows+1))})
	} else {
		items = m.pickItems(count)
	}
	lines := make([]string, len(items))
	for i, item := range items {
		lines[i] = paint(width, p.Input, item, nil)
	}
	return lines
}

func escapePath(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '\\':
			b.WriteString(`\\`)
		case unicode.IsControl(r):
			q := strconv.QuoteRune(r)
			b.WriteString(q[1 : len(q)-1])
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func wholeLabel(f diff.File) string {
	if text := describeWhole(f); text != "" {
		return text
	}
	if modeChanged(f) {
		return modeText(f)
	}
	return "whole file"
}

func (m Model) generatorNote() seg {
	p := m.pal
	mo := m.modal
	switch {
	case m.gen == nil:
		return span(fg(p.Muted), generatorHint)
	case mo.generating:
		return span(fg(p.Status), "✨ generating…")
	case mo.genErr != nil:
		return span(fg(p.Removed), "✨ generation failed")
	case mo.generated:
		n := pickedUnits(mo.picks)
		return span(fg(p.Status), fmt.Sprintf("✨ generated from %d %s, edit freely", n, plural(n, "hunk")))
	}
	return span(fg(p.Status), "")
}

func (m Model) noteSegs(width int) []seg {
	var segs []seg
	for _, note := range m.modalNotes() {
		for text := range strings.SplitSeq(ansi.Wrap(note.text, width, ""), "\n") {
			segs = append(segs, span(note.style, text))
		}
	}
	return segs
}

func (m Model) noteLines(width, room int) []string {
	segs := m.noteSegs(width)
	if len(segs) > room {
		segs = segs[:room]
		if room > 0 {
			segs[room-1].text = ansi.Truncate(segs[room-1].text, width-1, "") + "…"
		}
	}
	lines := make([]string, len(segs))
	for i, s := range segs {
		lines[i] = paint(width, m.pal.Card, []seg{s}, nil)
	}
	return lines
}

func (m Model) modalNotes() []seg {
	mo := m.modal
	var notes []seg
	switch {
	case mo.committing:
		notes = append(notes, span(fg(m.pal.Status), "committing…"))
	case errors.Is(mo.err, jj.ErrStaleRevision):
		notes = append(notes, span(fg(m.pal.Removed), "error: "+jj.ErrStaleRevision.Error()))
	case mo.err != nil:
		notes = append(notes, span(fg(m.pal.Removed), "error: "+mo.err.Error()))
	}
	if mo.genErr != nil {
		notes = append(notes, span(fg(m.pal.Removed), "generation failed: "+mo.genErr.Error()))
	}
	return notes
}

func (m Model) modalFooter(width int) []seg {
	p := m.pal
	key := fg(p.Key)
	text := fg(p.Muted)
	full := []seg{
		span(key, "esc"), span(text, " cancel · "),
		span(key, "ctrl+enter"), span(text, " / "), span(key, "ctrl+s"), span(text, " commit"),
	}
	if m.gen != nil {
		full = append([]seg{span(key, "ctrl+r"), span(text, " regenerate · ")}, full...)
	}
	if segsWidth(full) <= width {
		return full
	}
	compact := []seg{
		span(key, "ctrl+s"), span(text, " commit · "),
		span(key, "esc"), span(text, " cancel"),
	}
	if segsWidth(compact) <= width {
		return compact
	}
	return []seg{span(key, "^s"), span(text, " commit · "), span(key, "esc")}
}
