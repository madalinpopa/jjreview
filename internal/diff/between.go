package diff

import "fmt"

type stepLine struct {
	line Line
	from bool
	to   bool
}

func Between(f File, from, to [][]bool) (File, error) {
	if err := checkMasks(f, from); err != nil {
		return File{}, err
	}
	if err := checkMasks(f, to); err != nil {
		return File{}, err
	}
	out := stepHeader(f, from, to)
	out.Hunks = nil
	fromShift, toShift := 0, 0
	for i, h := range f.Hunks {
		lines := stepLines(h, from[i], to[i])
		first := firstLine(h.OldStart, h.OldLines)
		if hunk, ok := stepHunk(h.Section, lines, first+fromShift, first+toShift); ok {
			out.Hunks = append(out.Hunks, hunk)
		}
		fromLines, toLines := sideLines(lines)
		fromShift += fromLines - h.OldLines
		toShift += toLines - h.OldLines
	}
	return out, nil
}

func checkMasks(f File, masks [][]bool) error {
	if len(masks) != len(f.Hunks) {
		return fmt.Errorf("between: %d line masks for %d hunks", len(masks), len(f.Hunks))
	}
	for i, mask := range masks {
		if mask != nil && len(mask) != len(f.Hunks[i].Lines) {
			return fmt.Errorf("between hunk %d: mask of %d lines for %d lines", i, len(mask), len(f.Hunks[i].Lines))
		}
	}
	return nil
}

func stepHeader(f File, from, to [][]bool) File {
	started := anyTaken(f, from, from)
	finished := !anyOpen(f, from, to)
	switch {
	case f.Status == StatusDeleted && !finished:
		f.NewPath, f.NewMode, f.Status = f.OldPath, f.OldMode, StatusModified
	case f.Status != StatusDeleted && started:
		f.OldPath, f.OldMode, f.Status = f.NewPath, f.NewMode, StatusModified
	}
	return f
}

func anyTaken(f File, from, to [][]bool) bool {
	for i, h := range f.Hunks {
		for n, l := range h.Lines {
			if l.Kind != LineContext && taken(from[i], to[i], n) {
				return true
			}
		}
	}
	return false
}

func anyOpen(f File, from, to [][]bool) bool {
	for i, h := range f.Hunks {
		for n, l := range h.Lines {
			if l.Kind != LineContext && !taken(from[i], to[i], n) {
				return true
			}
		}
	}
	return false
}

func taken(from, to []bool, n int) bool {
	return from != nil && from[n] || to != nil && to[n]
}

func stepLines(h Hunk, from, to []bool) []stepLine {
	var lines []stepLine
	for n, l := range h.Lines {
		s := stepLine{line: l, from: true, to: true}
		switch l.Kind {
		case LineAdded:
			s.from, s.to = taken(from, nil, n), taken(from, to, n)
		case LineRemoved:
			s.from, s.to = !taken(from, nil, n), !taken(from, to, n)
		}
		if s.from || s.to {
			lines = append(lines, s)
		}
	}
	return lines
}

func sideLines(lines []stepLine) (fromLines, toLines int) {
	for _, s := range lines {
		if s.from {
			fromLines++
		}
		if s.to {
			toLines++
		}
	}
	return fromLines, toLines
}

func stepHunk(section string, lines []stepLine, oldNo, newNo int) (Hunk, bool) {
	h := Hunk{Section: section}
	lastFrom, lastTo := -1, -1
	changed := false
	for n, s := range lines {
		if s.from {
			lastFrom = n
		}
		if s.to {
			lastTo = n
		}
		changed = changed || s.from != s.to
	}
	if !changed {
		return Hunk{}, false
	}
	oldFirst, newFirst := oldNo, newNo
	for n, s := range lines {
		fromEnd := n == lastFrom && s.line.NoNewline
		toEnd := n == lastTo && s.line.NoNewline
		if s.from && s.to && fromEnd == toEnd {
			h.Lines = append(h.Lines, Line{Kind: LineContext, Text: s.line.Text, OldNo: oldNo, NewNo: newNo, NoNewline: fromEnd})
			oldNo++
			newNo++
			continue
		}
		if s.from {
			h.Lines = append(h.Lines, Line{Kind: LineRemoved, Text: s.line.Text, OldNo: oldNo, NoNewline: fromEnd})
			oldNo++
		}
		if s.to {
			h.Lines = append(h.Lines, Line{Kind: LineAdded, Text: s.line.Text, NewNo: newNo, NoNewline: toEnd})
			newNo++
		}
	}
	h.OldLines, h.NewLines = oldNo-oldFirst, newNo-newFirst
	h.OldStart, h.NewStart = hunkStart(oldFirst, h.OldLines), hunkStart(newFirst, h.NewLines)
	return h, true
}

func hunkStart(first, count int) int {
	if count == 0 {
		return first - 1
	}
	return first
}
