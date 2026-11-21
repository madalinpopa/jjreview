package tui

import (
	"image/color"
	"reflect"
	"slices"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/madalinpopa/jjreview/internal/diff"
	"github.com/madalinpopa/jjreview/internal/theme"
	"github.com/madalinpopa/jjreview/internal/tui/ui"
)

type fileHighlight struct {
	hunks [][][]ui.Seg
	full  [][]ui.Seg
}

type highlightKey struct {
	file int
	full bool
}

func (m *Model) ensureHighlight() {
	file, ok := m.currentFile()
	if !ok || !m.loaded || m.err != nil || wholeOnly(file) {
		return
	}
	key := highlightKey{file: m.fileIdx, full: m.full}
	if _, done := m.highlights[key]; done {
		return
	}
	content, cached := m.contents[m.contentKey(file)]
	if m.full && !cached && file.Status != diff.StatusDeleted {
		return
	}
	lexer := pickLexer(file, content)
	hl, reuse := m.highlights[highlightKey{file: m.fileIdx, full: !m.full}]
	if !reuse {
		hl.hunks = m.highlightHunks(lexer, file)
	}
	hl.full = nil
	if m.full {
		hl.full = m.highlightText(lexer, content)
	}
	m.highlights[key] = hl
}

func pickLexer(f diff.File, content string) chroma.Lexer {
	lexer := lexers.Match(f.Path())
	if lexer == nil {
		if content == "" {
			content = sideText(f, diff.LineRemoved)
		}
		lexer = lexers.Analyse(content)
	}
	if lexer == nil {
		lexer = lexers.Fallback
	}
	return chroma.Coalesce(lexer)
}

func (m Model) highlightHunks(lexer chroma.Lexer, f diff.File) [][][]ui.Seg {
	out := make([][][]ui.Seg, len(f.Hunks))
	for k, h := range f.Hunks {
		newSide := m.highlightText(lexer, sideText(diff.File{Hunks: []diff.Hunk{h}}, diff.LineRemoved))
		oldSide := m.highlightText(lexer, sideText(diff.File{Hunks: []diff.Hunk{h}}, diff.LineAdded))
		lines := make([][]ui.Seg, len(h.Lines))
		n, o := 0, 0
		for i, l := range h.Lines {
			if l.Kind == diff.LineRemoved {
				lines[i] = lineAt(oldSide, o)
				o++
				continue
			}
			lines[i] = lineAt(newSide, n)
			n++
			if l.Kind == diff.LineContext {
				o++
			}
		}
		out[k] = lines
	}
	return out
}

func sideText(f diff.File, skip diff.LineKind) string {
	var b strings.Builder
	for _, h := range f.Hunks {
		for _, l := range h.Lines {
			if l.Kind != skip {
				b.WriteString(l.Text)
				b.WriteByte('\n')
			}
		}
	}
	return b.String()
}

func lineAt(lines [][]ui.Seg, i int) []ui.Seg {
	if i < 0 || i >= len(lines) {
		return nil
	}
	return lines[i]
}

func (m Model) highlightText(lexer chroma.Lexer, text string) [][]ui.Seg {
	tokens, err := chroma.Tokenise(lexer, nil, text)
	if err != nil {
		return nil
	}
	lines := [][]ui.Seg{nil}
	for _, tok := range tokens {
		style := m.tokenStyle(tok.Type)
		for i, part := range strings.Split(tok.Value, "\n") {
			if i > 0 {
				lines = append(lines, nil)
			}
			if part != "" {
				last := len(lines) - 1
				lines[last] = append(lines[last], ui.Span(style, part))
			}
		}
	}
	return lines
}

func (m Model) tokenStyle(t chroma.TokenType) lipgloss.Style {
	style := ui.Fg(m.tokenColor(t))
	if t.InCategory(chroma.Comment) {
		style = style.Italic(true)
	}
	return style
}

func (m Model) tokenColor(t chroma.TokenType) color.Color {
	switch {
	case t == chroma.KeywordType, t.InSubCategory(chroma.NameBuiltin):
		return m.pal.Syntax.Type
	case t.InCategory(chroma.Keyword):
		return m.pal.Syntax.Keyword
	case t.InSubCategory(chroma.NameFunction), t == chroma.NameClass:
		return m.pal.Syntax.Function
	case t.InSubCategory(chroma.LiteralString):
		return m.pal.Syntax.String
	case t.InCategory(chroma.Literal):
		return m.pal.Syntax.Literal
	case t.InCategory(chroma.Comment):
		return m.pal.Syntax.Comment
	}
	return m.pal.Text
}

func (m Model) codeSegs(r row) []ui.Seg {
	file, ok := m.currentFile()
	hl, cached := m.highlights[highlightKey{file: m.fileIdx, full: m.full}]
	if !ok || !cached || r.hunk < 0 && r.line.Kind == diff.LineRemoved {
		return nil
	}
	if m.full && r.line.Kind != diff.LineRemoved {
		return lineAt(hl.full, r.line.NewNo-1)
	}
	if r.hunk < 0 || r.hunk >= len(hl.hunks) {
		return nil
	}
	for i, l := range file.Hunks[r.hunk].Lines {
		if l.Kind == r.line.Kind && l.OldNo == r.line.OldNo && l.NewNo == r.line.NewNo {
			return lineAt(hl.hunks[r.hunk], i)
		}
	}
	return nil
}

func highlightColors(p theme.Theme) []color.Color {
	s := p.Syntax
	return []color.Color{p.Text, s.Type, s.Keyword, s.Function, s.String, s.Literal, s.Comment}
}

func sameColors(a, b []color.Color) bool {
	return slices.EqualFunc(a, b, func(x, y color.Color) bool {
		t := reflect.TypeOf(x)
		return t == reflect.TypeOf(y) && (t == nil || t.Comparable() && x == y)
	})
}
