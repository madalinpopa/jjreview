package tui

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/madalinpopa/jjreview/internal/diff"
)

const modalListRows = 4

func (m Model) modalView() string {
	p := m.pal
	mo := m.modal
	width := m.modalWidth()
	inner := max(width-4, 1)
	lines := []string{
		paint(inner, p.Card, []seg{
			span(fg(p.Title).Bold(true), "New commit "),
			m.pill(p.Chip, p.ChipText, "from "+shortID(m.target)),
		}, nil),
		blank(inner, p.Card),
		m.box(inner, p.Border, strings.Join(m.pickLines(inner-4), "\n")),
		paint(inner, p.Card, []seg{
			span(fg(p.Header), "Commit message "),
			m.generatorNote(),
		}, nil),
		m.box(inner, p.Selection, mo.input.View()),
	}
	for _, note := range m.modalNotes() {
		lines = append(lines, note.style.Background(p.Card).Width(inner).Render(note.text))
	}
	lines = append(lines, paint(inner, p.Card, nil, m.modalFooter()))
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(p.ModalBorder).
		BorderBackground(p.Card).
		Background(p.Card).
		Padding(0, 1).
		Render(strings.Join(lines, "\n"))
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

func (m Model) pickLines(width int) []string {
	p := m.pal
	var items [][]seg
	for _, pk := range m.modal.picks {
		f := m.files[pk.file]
		name := span(fg(p.Strong), f.Path())
		if wholeOnly(f) {
			items = append(items, []seg{name, span(fg(p.Header), "  "+wholeLabel(f))})
			continue
		}
		for _, k := range pk.units {
			a, d := hunkStats(f.Hunks[k])
			detail := fmt.Sprintf("  %s  +%d −%d", hunkHeader(f.Hunks[k]), a, d)
			items = append(items, []seg{name, span(fg(p.Header), detail)})
		}
	}
	if len(items) > modalListRows {
		more := len(items) - modalListRows + 1
		items = append(items[:modalListRows-1], []seg{span(fg(p.Muted), fmt.Sprintf("… %d more", more))})
	}
	lines := make([]string, len(items))
	for i, item := range items {
		lines[i] = paint(width, p.Input, item, nil)
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

func (m Model) modalNotes() []seg {
	mo := m.modal
	var notes []seg
	if mo.genErr != nil {
		notes = append(notes, span(fg(m.pal.Removed), "generation failed: "+mo.genErr.Error()))
	}
	switch {
	case mo.committing:
		notes = append(notes, span(fg(m.pal.Status), "committing…"))
	case mo.err != nil:
		notes = append(notes, span(fg(m.pal.Removed), "error: "+mo.err.Error()))
	}
	return notes
}

func (m Model) modalFooter() []seg {
	p := m.pal
	key := fg(p.Key)
	text := fg(p.Muted)
	return []seg{
		span(key, "ctrl+r"), span(text, " regenerate · "),
		span(key, "esc"), span(text, " cancel · "),
		span(key, "ctrl+enter"), span(text, " / "), span(key, "ctrl+s"), span(text, " commit"),
	}
}
