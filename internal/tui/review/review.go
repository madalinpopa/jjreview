package review

import (
	"fmt"
	"slices"

	"github.com/madalinpopa/jjreview/internal/diff"
)

type fileState int

const (
	stateNone fileState = iota
	stateSome
	stateAll
	stateDone
)

func (s fileState) glyph() string {
	switch s {
	case stateDone:
		return "✓"
	case stateAll:
		return "●"
	case stateSome:
		return "◐"
	}
	return "○"
}

type lineState int

const (
	lineContext lineState = iota
	lineOpen
	linePicked
	lineDone
)

type fileReview struct {
	folded    []bool
	removed   []bool
	lines     [][]lineState
	committed []string
}

const symlinkMode = "120000"

func wholeOnly(f diff.File) bool {
	return len(f.Hunks) == 0 || f.Binary || f.NewMode == symlinkMode || f.OldMode == symlinkMode
}

func deletesOnly(h diff.Hunk) bool {
	return h.NewLines == 0
}

func (r fileReview) keepView(f diff.File, prev diff.File, prevReview fileReview) {
	if wholeOnly(f) || wholeOnly(prev) {
		return
	}
	used := make([]bool, len(prev.Hunks))
	kept := make([]bool, len(f.Hunks))
	keep := func(k int, samePlace bool) {
		match := -1
		for j, p := range prev.Hunks {
			if used[j] || !sameLines(p, f.Hunks[k]) || samePlace && p.OldStart != f.Hunks[k].OldStart {
				continue
			}
			if match >= 0 {
				return
			}
			match = j
		}
		if match < 0 {
			return
		}
		used[match], kept[k] = true, true
		r.folded[k] = prevReview.folded[match]
		r.removed[k] = prevReview.removed[match]
	}
	for k := range f.Hunks {
		keep(k, true)
	}
	for k := range f.Hunks {
		if !kept[k] {
			keep(k, false)
		}
	}
}

func sameLines(a, b diff.Hunk) bool {
	return slices.EqualFunc(a.Lines, b.Lines, func(x, y diff.Line) bool {
		return x.Kind == y.Kind && x.Text == y.Text
	})
}

func sameFiles(a, b []diff.File) bool {
	return slices.EqualFunc(a, b, func(x, y diff.File) bool {
		return x.OldPath == y.OldPath && x.NewPath == y.NewPath && x.OldMode == y.OldMode && x.NewMode == y.NewMode &&
			x.Status == y.Status && x.Binary == y.Binary && slices.EqualFunc(x.Hunks, y.Hunks, func(p, q diff.Hunk) bool {
			return p.OldStart == q.OldStart && p.OldLines == q.OldLines && p.NewStart == q.NewStart &&
				p.NewLines == q.NewLines && p.Section == q.Section && slices.Equal(p.Lines, q.Lines)
		})
	})
}

func modeChanged(f diff.File) bool {
	return f.OldMode != "" && f.NewMode != "" && f.OldMode != f.NewMode
}

func newReview(f diff.File) fileReview {
	n := len(f.Hunks)
	if wholeOnly(f) {
		n = 1
	}
	removed := make([]bool, n)
	for k := range removed {
		removed[k] = f.Status == diff.StatusDeleted || (!wholeOnly(f) && deletesOnly(f.Hunks[k]))
	}
	return fileReview{
		folded:    make([]bool, n),
		removed:   removed,
		lines:     lineStates(f),
		committed: make([]string, n),
	}
}

func lineStates(f diff.File) [][]lineState {
	if wholeOnly(f) {
		return [][]lineState{{lineOpen}}
	}
	lines := make([][]lineState, len(f.Hunks))
	for k, h := range f.Hunks {
		lines[k] = make([]lineState, len(h.Lines))
		for n, l := range h.Lines {
			if l.Kind != diff.LineContext {
				lines[k][n] = lineOpen
			}
		}
	}
	return lines
}

func tally(lines []lineState) (pending, picked int) {
	for _, st := range lines {
		switch st {
		case lineOpen:
			pending++
		case linePicked:
			pending++
			picked++
		}
	}
	return pending, picked
}

func stateOf(pending, picked int) fileState {
	switch {
	case pending == 0:
		return stateDone
	case picked == 0:
		return stateNone
	case picked == pending:
		return stateAll
	}
	return stateSome
}

func (r fileReview) state() fileState {
	pending, picked := 0, 0
	for _, lines := range r.lines {
		p, s := tally(lines)
		pending += p
		picked += s
	}
	return stateOf(pending, picked)
}

func (r fileReview) unitState(k int) fileState {
	return stateOf(tally(r.lines[k]))
}

func (r fileReview) setUnit(k int, to lineState) {
	for n, st := range r.lines[k] {
		if st == lineOpen || st == linePicked {
			r.lines[k][n] = to
		}
	}
}

func (r fileReview) toggleLine(k, n int) bool {
	switch r.lines[k][n] {
	case lineOpen:
		r.lines[k][n] = linePicked
	case linePicked:
		r.lines[k][n] = lineOpen
	default:
		return false
	}
	return true
}

func (r fileReview) partial(k int) bool {
	seen := lineContext
	for _, st := range r.lines[k] {
		if st == lineContext {
			continue
		}
		if seen != lineContext && st != seen {
			return true
		}
		seen = st
	}
	return false
}

func (r fileReview) showsRemoved(k int) bool {
	return r.removed[k] || r.partial(k)
}

func pickOrRelease(st fileState) lineState {
	if st == stateAll {
		return lineOpen
	}
	return linePicked
}

func (r fileReview) toggleUnit(k int) {
	if k < 0 || k >= len(r.lines) {
		return
	}
	r.setUnit(k, pickOrRelease(r.unitState(k)))
}

func (r fileReview) toggleFile() {
	to := pickOrRelease(r.state())
	for k := range r.lines {
		r.setUnit(k, to)
	}
}

func (r fileReview) picked() []int {
	var out []int
	for k, lines := range r.lines {
		if _, picked := tally(lines); picked > 0 {
			out = append(out, k)
		}
	}
	return out
}

func (r fileReview) included() [][]bool {
	out := make([][]bool, len(r.lines))
	for k, lines := range r.lines {
		if !slices.ContainsFunc(lines, taken) {
			continue
		}
		out[k] = make([]bool, len(lines))
		for n, st := range lines {
			out[k][n] = taken(st)
		}
	}
	return out
}

func (r fileReview) masks() (done, taken [][]bool) {
	done, taken = make([][]bool, len(r.lines)), make([][]bool, len(r.lines))
	for k, lines := range r.lines {
		done[k], taken[k] = make([]bool, len(lines)), make([]bool, len(lines))
		for n, st := range lines {
			done[k][n] = st == lineDone
			taken[k][n] = st == linePicked
		}
	}
	return done, taken
}

func (r fileReview) pickedLines() (lines int, partial bool) {
	for _, states := range r.lines {
		pending, picked := tally(states)
		lines += picked
		partial = partial || picked > 0 && (picked < pending || slices.Contains(states, lineDone))
	}
	return lines, partial
}

func taken(st lineState) bool {
	return st == linePicked || st == lineDone
}

func (r fileReview) complete() bool {
	for _, lines := range r.lines {
		if slices.Contains(lines, lineOpen) {
			return false
		}
	}
	return true
}

func (r fileReview) commit(units []int, id string, fold bool) {
	for _, k := range units {
		for n, st := range r.lines[k] {
			if st == linePicked {
				r.lines[k][n] = lineDone
			}
		}
		r.committed[k] = id
		r.folded[k] = fold && (r.folded[k] || r.unitState(k) == stateDone)
	}
}

func (r fileReview) foldedCount() int {
	n := 0
	for _, f := range r.folded {
		if f {
			n++
		}
	}
	return n
}

func (r fileReview) foldAll() {
	anyOpen := false
	for _, f := range r.folded {
		anyOpen = anyOpen || !f
	}
	for k := range r.folded {
		r.folded[k] = anyOpen
	}
}

func (r fileReview) toggleRemovedAll(f diff.File, full bool) {
	if wholeOnly(f) {
		return
	}
	hiddenAny, hiddenShown, anyShown := false, false, false
	for k, h := range f.Hunks {
		if !hidesRemoved(h) {
			continue
		}
		hiddenAny = hiddenAny || !r.showsRemoved(k)
		if full || !r.folded[k] {
			anyShown = true
			hiddenShown = hiddenShown || !r.showsRemoved(k)
		}
	}
	show := hiddenAny
	if anyShown {
		show = hiddenShown
	}
	for k, h := range f.Hunks {
		if hidesRemoved(h) {
			r.removed[k] = show
		}
	}
}

func hidesRemoved(h diff.Hunk) bool {
	_, removed := hunkStats(h)
	return removed > 0 && !deletesOnly(h)
}

func hunkStats(h diff.Hunk) (added, removed int) {
	for _, l := range h.Lines {
		switch l.Kind {
		case diff.LineAdded:
			added++
		case diff.LineRemoved:
			removed++
		}
	}
	return added, removed
}

func pickedStats(h diff.Hunk, states []lineState) (added, removed int) {
	for n, l := range h.Lines {
		switch {
		case states[n] != linePicked:
		case l.Kind == diff.LineAdded:
			added++
		case l.Kind == diff.LineRemoved:
			removed++
		}
	}
	return added, removed
}

func statsText(h diff.Hunk, states []lineState) string {
	a, d := hunkStats(h)
	total := fmt.Sprintf("+%d −%d", a, d)
	pa, pd := pickedStats(h, states)
	if n := pa + pd; n == 0 || n == a+d {
		return total
	}
	return fmt.Sprintf("+%d −%d of %s", pa, pd, total)
}

func fileStats(f diff.File) (added, removed int) {
	for _, h := range f.Hunks {
		a, d := hunkStats(h)
		added += a
		removed += d
	}
	return added, removed
}
