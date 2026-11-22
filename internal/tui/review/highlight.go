package review

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

func (m *Model) ensureHighlight(env Env) {
	file, ok := m.currentFile()
	if !ok || !env.Loaded || env.Err != nil || wholeOnly(file) {
		return
	}
	key := highlightKey{file: m.fileIdx, full: m.full}
	if _, done := m.highlights[key]; done {
		return
	}
	content, cached := m.contents[contentKeyOf(env, file)]
	if m.full && !cached && file.Status != diff.StatusDeleted {
		return
	}
	lexer := pickLexer(file, content)
	hl, reuse := m.highlights[highlightKey{file: m.fileIdx, full: !m.full}]
	if !reuse {
		hl.hunks = highlightHunks(env.Theme, lexer, file)
	}
	hl.full = nil
	if m.full {
		hl.full = highlightText(env.Theme, lexer, content)
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

func highlightHunks(p theme.Theme, lexer chroma.Lexer, f diff.File) [][][]ui.Seg {
	out := make([][][]ui.Seg, len(f.Hunks))
	for k, h := range f.Hunks {
		newSide := highlightText(p, lexer, sideText(diff.File{Hunks: []diff.Hunk{h}}, diff.LineRemoved))
		oldSide := highlightText(p, lexer, sideText(diff.File{Hunks: []diff.Hunk{h}}, diff.LineAdded))
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

func highlightText(p theme.Theme, lexer chroma.Lexer, text string) [][]ui.Seg {
	tokens, err := chroma.Tokenise(lexer, nil, text)
	if err != nil {
		return nil
	}
	lines := [][]ui.Seg{nil}
	for _, tok := range tokens {
		style := tokenStyle(p, tok.Type)
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

func tokenStyle(p theme.Theme, t chroma.TokenType) lipgloss.Style {
	style := ui.Fg(tokenColor(p, t))
	if t.InCategory(chroma.Comment) {
		style = style.Italic(true)
	}
	return style
}

func tokenColor(p theme.Theme, t chroma.TokenType) color.Color {
	switch {
	case t == chroma.KeywordType, t.InSubCategory(chroma.NameBuiltin):
		return p.Syntax.Type
	case t.InCategory(chroma.Keyword):
		return p.Syntax.Keyword
	case t.InSubCategory(chroma.NameFunction), t == chroma.NameClass:
		return p.Syntax.Function
	case t.InSubCategory(chroma.LiteralString):
		return p.Syntax.String
	case t.InCategory(chroma.Literal):
		return p.Syntax.Literal
	case t.InCategory(chroma.Comment):
		return p.Syntax.Comment
	}
	return p.Text
}

func (m Model) codeSegs(r row) []ui.Seg {
	_, ok := m.currentFile()
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
	return lineAt(hl.hunks[r.hunk], r.index)
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
