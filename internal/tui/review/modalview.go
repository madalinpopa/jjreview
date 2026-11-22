package review

import (
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/madalinpopa/jjreview/internal/diff"
	"github.com/madalinpopa/jjreview/internal/jj"
	"github.com/madalinpopa/jjreview/internal/theme"
	"github.com/madalinpopa/jjreview/internal/tui/ui"
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

func (m Model) modalLayout(env Env) modalRows {
	notes := len(m.noteSegs(env.Theme, modalInner(env)))
	rows := modalRows{preview: 1}
	chrome := modalChrome
	if env.Height < modalChrome+2+min(notes, modalNoteRows) {
		rows.preview, chrome = 0, modalCompactChrome
	}
	room := max(env.Height-chrome-1-rows.preview, 0)
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

func modalInner(env Env) int {
	return max(env.ModalWidth-4, 1)
}

func (m Model) modalView(env Env) string {
	p := env.Theme
	mo := m.modal
	inner := modalInner(env)
	rows := m.modalLayout(env)
	lines := []string{
		ui.Paint(inner, p.Card, []ui.Seg{
			ui.Span(ui.Fg(p.Title).Bold(true), "New commit "),
			ui.Pill(p.Chip, p.ChipText, "from "+ui.ShortID(env.Head.ChangeID)),
		}, nil),
	}
	if rows.preview > 0 {
		lines = append(lines,
			ui.Blank(inner, p.Card),
			ui.InputBox(p, inner, p.Border, strings.Join(m.pickLines(p, ui.InputBoxInner(inner), rows.preview), "\n")),
		)
	}
	heading := ui.Paint(inner, p.Card, []ui.Seg{
		ui.Span(ui.Fg(p.Header), "Commit message "),
		m.generatorNote(p),
	}, nil)
	notes := m.noteLines(p, inner, rows.notes)
	if rows.noteInHeading {
		heading, notes = notes[0], nil
	}
	lines = append(lines, heading, ui.InputBox(p, inner, p.Selection, mo.input.View()))
	lines = append(lines, notes...)
	lines = append(lines, ui.Paint(inner, p.Card, nil, m.modalFooter(p, inner)))
	return ui.ModalBox(p, lines)
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

func (m Model) pickItems(p theme.Theme, limit int) [][]ui.Seg {
	var items [][]ui.Seg
	for _, pk := range m.modal.picks {
		f := m.files[pk.file]
		name := ui.Span(ui.Fg(p.Strong), ui.EscapePath(f.Path()))
		if wholeOnly(f) {
			if len(items) == limit {
				return items
			}
			items = append(items, []ui.Seg{name, ui.Span(ui.Fg(p.Header), "  "+wholeLabel(f))})
			continue
		}
		for _, k := range pk.units {
			if len(items) == limit {
				return items
			}
			a, d := hunkStats(f.Hunks[k])
			detail := fmt.Sprintf("  %s  +%d −%d", hunkHeader(f.Hunks[k]), a, d)
			items = append(items, []ui.Seg{name, ui.Span(ui.Fg(p.Header), detail)})
		}
	}
	return items
}

func (m Model) pickLines(p theme.Theme, width, rows int) []string {
	count := m.pickCount()
	var items [][]ui.Seg
	if count > rows {
		items = append(m.pickItems(p, rows-1), []ui.Seg{ui.Span(ui.Fg(p.Muted), fmt.Sprintf("… %d more", count-rows+1))})
	} else {
		items = m.pickItems(p, count)
	}
	lines := make([]string, len(items))
	for i, item := range items {
		lines[i] = ui.Paint(width, p.Input, item, nil)
	}
	return lines
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

func (m Model) generatorNote(p theme.Theme) ui.Seg {
	mo := m.modal
	switch {
	case m.gen == nil:
		return ui.Span(ui.Fg(p.Muted), generatorHint)
	case mo.generating:
		return ui.Span(ui.Fg(p.Status), "✨ generating…")
	case mo.genErr != nil:
		return ui.Span(ui.Fg(p.Removed), "✨ generation failed")
	case mo.generated:
		n := pickedUnits(mo.picks)
		return ui.Span(ui.Fg(p.Status), fmt.Sprintf("✨ generated from %d %s, edit freely", n, ui.Plural(n, "hunk")))
	}
	return ui.Span(ui.Fg(p.Status), "")
}

func (m Model) noteSegs(p theme.Theme, width int) []ui.Seg {
	var segs []ui.Seg
	for _, note := range m.modalNotes(p) {
		for text := range strings.SplitSeq(ansi.Wrap(note.Text, width, ""), "\n") {
			segs = append(segs, ui.Span(note.Style, text))
		}
	}
	return segs
}

func (m Model) noteLines(p theme.Theme, width, room int) []string {
	segs := m.noteSegs(p, width)
	if len(segs) > room {
		segs = segs[:room]
		if room > 0 {
			segs[room-1].Text = ansi.Truncate(segs[room-1].Text, width-1, "") + "…"
		}
	}
	lines := make([]string, len(segs))
	for i, s := range segs {
		lines[i] = ui.Paint(width, p.Card, []ui.Seg{s}, nil)
	}
	return lines
}

func (m Model) modalNotes(p theme.Theme) []ui.Seg {
	mo := m.modal
	var notes []ui.Seg
	switch {
	case mo.committing:
		notes = append(notes, ui.Span(ui.Fg(p.Status), "committing…"))
	case errors.Is(mo.err, jj.ErrStaleRevision):
		notes = append(notes, ui.Span(ui.Fg(p.Removed), "error: "+jj.ErrStaleRevision.Error()))
	case mo.err != nil:
		notes = append(notes, ui.Span(ui.Fg(p.Removed), "error: "+mo.err.Error()))
	}
	if mo.genErr != nil {
		notes = append(notes, ui.Span(ui.Fg(p.Removed), "generation failed: "+mo.genErr.Error()))
	}
	return notes
}

func (m Model) modalFooter(p theme.Theme, width int) []ui.Seg {
	key := ui.Fg(p.Key)
	text := ui.Fg(p.Muted)
	full := []ui.Seg{
		ui.Span(key, "esc"), ui.Span(text, " cancel · "),
		ui.Span(key, "ctrl+enter"), ui.Span(text, " / "), ui.Span(key, "ctrl+s"), ui.Span(text, " commit"),
	}
	if m.gen != nil {
		full = append([]ui.Seg{ui.Span(key, "ctrl+r"), ui.Span(text, " regenerate · ")}, full...)
	}
	if ui.Width(full) <= width {
		return full
	}
	compact := []ui.Seg{
		ui.Span(key, "ctrl+s"), ui.Span(text, " commit · "),
		ui.Span(key, "esc"), ui.Span(text, " cancel"),
	}
	if ui.Width(compact) <= width {
		return compact
	}
	return []ui.Seg{ui.Span(key, "^s"), ui.Span(text, " commit · "), ui.Span(key, "esc")}
}
