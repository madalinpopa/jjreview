package review

import (
	"fmt"
	"strings"

	"github.com/madalinpopa/jjreview/internal/diff"
	"github.com/madalinpopa/jjreview/internal/tui/ui"
)

type rowKind int

const (
	rowMessage rowKind = iota
	rowError
	rowGap
	rowHunk
	rowLine
)

type row struct {
	kind  rowKind
	hunk  int
	index int
	line  diff.Line
	text  string
}

func messageRow(text string) row {
	return row{kind: rowMessage, hunk: -1, index: -1, text: text}
}

func errorRow(err error) row {
	return row{kind: rowError, hunk: -1, index: -1, text: "error: " + ui.EscapePath(err.Error())}
}

func (m Model) rows(env Env) []row {
	file, ok := m.currentFile()
	switch {
	case !env.Loaded:
		return []row{messageRow("loading…")}
	case env.Err != nil:
		return []row{errorRow(env.Err)}
	case !ok:
		return []row{messageRow("no changes")}
	case wholeOnly(file):
		return wholeRows(file)
	case m.full:
		return withMode(file, m.fullRows(env, file))
	}
	total := -1
	if content, cached := m.contents[contentKeyOf(env, file)]; cached {
		total = lineCount(content)
	}
	return withMode(file, changeRows(file, m.currentReview(), total))
}

func withMode(f diff.File, rows []row) []row {
	if !modeChanged(f) {
		return rows
	}
	return append([]row{messageRow(modeText(f))}, rows...)
}

func modeText(f diff.File) string {
	return fmt.Sprintf("mode %s → %s", f.OldMode, f.NewMode)
}

func wholeRows(f diff.File) []row {
	var rows []row
	if text := describeWhole(f); text != "" {
		rows = append(rows, messageRow(text))
	}
	for _, h := range f.Hunks {
		for _, l := range h.Lines {
			rows = append(rows, row{kind: rowLine, hunk: -1, index: -1, line: l})
		}
	}
	rows = withMode(f, rows)
	if len(rows) == 0 {
		rows = append(rows, messageRow("no content changes"))
	}
	return rows
}

func describeWhole(f diff.File) string {
	switch {
	case f.Binary:
		return "binary file"
	case f.NewMode == symlinkMode || f.OldMode == symlinkMode:
		return "symlink"
	case f.Status == diff.StatusRenamed:
		return "renamed from " + ui.EscapePath(f.OldPath)
	case f.Status == diff.StatusCopied:
		return "copied from " + ui.EscapePath(f.OldPath)
	}
	return ""
}

func (m Model) fullRows(env Env, file diff.File) []row {
	if file.Status == diff.StatusDeleted {
		return []row{messageRow("file deleted")}
	}
	key := contentKeyOf(env, file)
	content, ok := m.contents[key]
	if !ok {
		if err, failed := m.contentErrs[key]; failed {
			return []row{errorRow(err)}
		}
		return []row{messageRow("loading…")}
	}
	return interleave(file, diff.Full(file, content), m.currentReview())
}

func interleave(file diff.File, lines []diff.Line, r fileReview) []row {
	rows := make([]row, 0, len(lines))
	seen := make([]int, len(file.Hunks))
	shows := make([]bool, len(file.Hunks))
	for k := range shows {
		shows[k] = r.showsRemoved(k)
	}
	for _, l := range lines {
		k, index := hunkOf(file, l), -1
		if k >= 0 {
			index = seen[k]
			seen[k]++
		}
		if l.Kind == diff.LineRemoved && (k < 0 || !shows[k]) {
			continue
		}
		rows = append(rows, row{kind: rowLine, hunk: k, index: index, line: l})
	}
	return rows
}

func changeRows(file diff.File, r fileReview, total int) []row {
	var rows []row
	next := 1
	for k, h := range file.Hunks {
		rows = appendGap(rows, newBegin(h)-next)
		rows = append(rows, hunkRow(file, k))
		if !r.folded[k] {
			shows := r.showsRemoved(k)
			for n, l := range h.Lines {
				if l.Kind == diff.LineRemoved && !shows {
					continue
				}
				rows = append(rows, row{kind: rowLine, hunk: k, index: n, line: l})
			}
		}
		next = newBegin(h) + h.NewLines
	}
	if total >= 0 {
		rows = appendGap(rows, total-next+1)
	}
	return rows
}

func hunkRow(file diff.File, k int) row {
	return row{kind: rowHunk, hunk: k, index: -1, text: hunkHeader(file.Hunks[k])}
}

func appendGap(rows []row, n int) []row {
	if n <= 0 {
		return rows
	}
	return append(rows, row{kind: rowGap, hunk: -1, index: -1, text: fmt.Sprintf("⋯ %d unchanged %s", n, ui.Plural(n, "line"))})
}

func hunkOf(file diff.File, l diff.Line) int {
	for k, h := range file.Hunks {
		if l.Kind == diff.LineRemoved {
			if within(l.OldNo, oldBegin(h), h.OldLines) {
				return k
			}
			continue
		}
		if within(l.NewNo, newBegin(h), h.NewLines) {
			return k
		}
	}
	return -1
}

func within(n, begin, count int) bool {
	return n >= begin && n < begin+count
}

func oldBegin(h diff.Hunk) int {
	if h.OldLines == 0 {
		return h.OldStart + 1
	}
	return h.OldStart
}

func newBegin(h diff.Hunk) int {
	if h.NewLines == 0 {
		return h.NewStart + 1
	}
	return h.NewStart
}

func hunkHeader(h diff.Hunk) string {
	header := fmt.Sprintf("@@ -%d,%d +%d,%d @@", h.OldStart, h.OldLines, h.NewStart, h.NewLines)
	if h.Section != "" {
		header += " " + h.Section
	}
	return header
}

func lineCount(text string) int {
	if text == "" {
		return 0
	}
	n := strings.Count(text, "\n")
	if !strings.HasSuffix(text, "\n") {
		n++
	}
	return n
}

func firstRow(rows []row) int {
	for i, r := range rows {
		if r.kind == rowLine && r.line.Kind != diff.LineContext {
			return i
		}
	}
	for i, r := range rows {
		if r.kind != rowGap {
			return i
		}
	}
	return 0
}
