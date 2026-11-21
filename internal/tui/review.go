package tui

import (
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

type fileReview struct {
	folded    []bool
	removed   []bool
	selected  []bool
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
		selected:  make([]bool, n),
		committed: make([]string, n),
	}
}

func (r fileReview) state() fileState {
	open, sel := 0, 0
	for k, id := range r.committed {
		if id != "" {
			continue
		}
		open++
		if r.selected[k] {
			sel++
		}
	}
	switch {
	case open == 0:
		return stateDone
	case sel == 0:
		return stateNone
	case sel == open:
		return stateAll
	}
	return stateSome
}

func (r fileReview) toggleUnit(k int) {
	if k < 0 || k >= len(r.selected) || r.committed[k] != "" {
		return
	}
	r.selected[k] = !r.selected[k]
}

func (r fileReview) toggleFile() {
	st := r.state()
	if st == stateDone {
		return
	}
	for k := range r.selected {
		if r.committed[k] == "" {
			r.selected[k] = st != stateAll
		}
	}
}

func (r fileReview) picked() []int {
	var out []int
	for k, sel := range r.selected {
		if sel && r.committed[k] == "" {
			out = append(out, k)
		}
	}
	return out
}

func (r fileReview) included() []int {
	var out []int
	for k, sel := range r.selected {
		if sel || r.committed[k] != "" {
			out = append(out, k)
		}
	}
	return out
}

func (r fileReview) commit(units []int, id string, fold bool) {
	for _, k := range units {
		r.committed[k] = id
		r.selected[k] = false
		r.folded[k] = fold
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
		hiddenAny = hiddenAny || !r.removed[k]
		if full || !r.folded[k] {
			anyShown = true
			hiddenShown = hiddenShown || !r.removed[k]
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

func fileStats(f diff.File) (added, removed int) {
	for _, h := range f.Hunks {
		a, d := hunkStats(h)
		added += a
		removed += d
	}
	return added, removed
}
