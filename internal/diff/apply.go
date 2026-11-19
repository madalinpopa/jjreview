package diff

import (
	"fmt"
	"strings"
)

type applied struct {
	old          []string
	oldNoNewline bool
	lines        []string
	noNewline    bool
}

func Apply(oldText string, f File, hunks []int) (string, error) {
	if err := checkSelection(hunks, len(f.Hunks)); err != nil {
		return "", err
	}
	a := applied{old: splitLines(oldText), oldNoNewline: !strings.HasSuffix(oldText, "\n")}
	cursor := 0
	for _, i := range hunks {
		h := f.Hunks[i]
		start := firstLine(h.OldStart, h.OldLines) - 1
		if start < cursor || start > len(a.old) {
			return "", fmt.Errorf("apply hunk %d: old start %d outside the text", i, h.OldStart)
		}
		a.copyOld(cursor, start)
		next, err := a.hunk(start, h)
		if err != nil {
			return "", fmt.Errorf("apply hunk %d: %w", i, err)
		}
		cursor = next
	}
	a.copyOld(cursor, len(a.old))
	return a.text(), nil
}

func checkSelection(hunks []int, count int) error {
	for n, i := range hunks {
		if i < 0 || i >= count {
			return fmt.Errorf("apply: hunk index %d out of range [0, %d)", i, count)
		}
		if n > 0 && i <= hunks[n-1] {
			return fmt.Errorf("apply: hunk indexes must be ascending and unique, got %v", hunks)
		}
	}
	return nil
}

func (a *applied) copyOld(from, to int) {
	if from == to {
		return
	}
	a.lines = append(a.lines, a.old[from:to]...)
	a.noNewline = to == len(a.old) && a.oldNoNewline
}

func (a *applied) hunk(pos int, h Hunk) (int, error) {
	for _, l := range h.Lines {
		if l.Kind != LineAdded {
			if err := a.checkOld(pos, l); err != nil {
				return 0, err
			}
			pos++
		}
		if l.Kind != LineRemoved {
			a.lines = append(a.lines, l.Text)
			a.noNewline = l.NoNewline
		}
	}
	return pos, nil
}

func (a *applied) checkOld(pos int, l Line) error {
	if pos >= len(a.old) || a.old[pos] != l.Text {
		return fmt.Errorf("old line %d does not match %q", pos+1, l.Text)
	}
	if want := pos == len(a.old)-1 && a.oldNoNewline; l.NoNewline != want {
		return fmt.Errorf("old line %d newline state does not match the hunk", pos+1)
	}
	return nil
}

func (a *applied) text() string {
	if len(a.lines) == 0 {
		return ""
	}
	text := strings.Join(a.lines, "\n")
	if !a.noNewline {
		text += "\n"
	}
	return text
}
