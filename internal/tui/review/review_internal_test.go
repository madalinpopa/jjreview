package review

import (
	"reflect"
	"testing"

	"github.com/madalinpopa/jjreview/internal/diff"
)

func reviewFile() diff.File {
	return diff.File{OldPath: "f.txt", NewPath: "f.txt", Hunks: []diff.Hunk{
		{OldStart: 1, OldLines: 3, NewStart: 1, NewLines: 3, Lines: []diff.Line{
			{Kind: diff.LineContext, Text: "a", OldNo: 1, NewNo: 1},
			{Kind: diff.LineRemoved, Text: "b", OldNo: 2},
			{Kind: diff.LineAdded, Text: "B", NewNo: 2},
			{Kind: diff.LineContext, Text: "c", OldNo: 3, NewNo: 3},
		}},
		{OldStart: 10, OldLines: 1, NewStart: 10, NewLines: 2, Lines: []diff.Line{
			{Kind: diff.LineContext, Text: "j", OldNo: 10, NewNo: 10},
			{Kind: diff.LineAdded, Text: "k", NewNo: 11},
		}},
	}}
}

func TestNewReview_lineStates(t *testing.T) {
	t.Parallel()

	t.Run("marks changed lines open and the rest context", func(t *testing.T) {
		t.Parallel()

		r := newReview(reviewFile())

		want := [][]lineState{{lineContext, lineOpen, lineOpen, lineContext}, {lineContext, lineOpen}}
		if !reflect.DeepEqual(r.lines, want) {
			t.Errorf("newReview() lines = %v, want %v", r.lines, want)
		}
	})

	t.Run("gives a whole-only file one open line", func(t *testing.T) {
		t.Parallel()

		r := newReview(diff.File{OldPath: "x.bin", NewPath: "x.bin", Binary: true})

		if want := [][]lineState{{lineOpen}}; !reflect.DeepEqual(r.lines, want) {
			t.Errorf("newReview() lines = %v, want %v", r.lines, want)
		}
	})
}

func TestFileReview_state(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		lines    [][]lineState
		want     fileState
		units    []fileState
		picked   []int
		included [][]bool
		complete bool
	}{
		"nothing picked": {
			lines:    [][]lineState{{lineContext, lineOpen, lineOpen, lineContext}, {lineContext, lineOpen}},
			want:     stateNone,
			units:    []fileState{stateNone, stateNone},
			included: [][]bool{nil, nil},
		},
		"one hunk picked": {
			lines:    [][]lineState{{lineContext, linePicked, linePicked, lineContext}, {lineContext, lineOpen}},
			want:     stateSome,
			units:    []fileState{stateAll, stateNone},
			picked:   []int{0},
			included: [][]bool{{false, true, true, false}, nil},
		},
		"one line short of whole": {
			lines:    [][]lineState{{lineContext, linePicked, lineOpen, lineContext}, {lineContext, linePicked}},
			want:     stateSome,
			units:    []fileState{stateSome, stateAll},
			picked:   []int{0, 1},
			included: [][]bool{{false, true, false, false}, {false, true}},
		},
		"every line picked": {
			lines:    [][]lineState{{lineContext, linePicked, linePicked, lineContext}, {lineContext, linePicked}},
			want:     stateAll,
			units:    []fileState{stateAll, stateAll},
			picked:   []int{0, 1},
			included: [][]bool{{false, true, true, false}, {false, true}},
			complete: true,
		},
		"committed and picked lines make it whole": {
			lines:    [][]lineState{{lineContext, lineDone, linePicked, lineContext}, {lineContext, lineDone}},
			want:     stateAll,
			units:    []fileState{stateAll, stateDone},
			picked:   []int{0},
			included: [][]bool{{false, true, true, false}, {false, true}},
			complete: true,
		},
		"committed lines next to an open one": {
			lines:    [][]lineState{{lineContext, lineDone, lineOpen, lineContext}, {lineContext, lineDone}},
			want:     stateNone,
			units:    []fileState{stateNone, stateDone},
			included: [][]bool{{false, true, false, false}, {false, true}},
		},
		"everything committed": {
			lines:    [][]lineState{{lineContext, lineDone, lineDone, lineContext}, {lineContext, lineDone}},
			want:     stateDone,
			units:    []fileState{stateDone, stateDone},
			included: [][]bool{{false, true, true, false}, {false, true}},
			complete: true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := newReview(reviewFile())
			r.lines = tt.lines

			if got := r.state(); got != tt.want {
				t.Errorf("state() = %v, want %v", got, tt.want)
			}
			for k, want := range tt.units {
				if got := r.unitState(k); got != want {
					t.Errorf("unitState(%d) = %v, want %v", k, got, want)
				}
			}
			if got := r.picked(); !reflect.DeepEqual(got, tt.picked) {
				t.Errorf("picked() = %v, want %v", got, tt.picked)
			}
			if got := r.included(); !reflect.DeepEqual(got, tt.included) {
				t.Errorf("included() = %v, want %v", got, tt.included)
			}
			if got := r.complete(); got != tt.complete {
				t.Errorf("complete() = %v, want %v", got, tt.complete)
			}
		})
	}
}

func TestFileReview_toggle(t *testing.T) {
	t.Parallel()

	t.Run("a hunk picks its open lines and then releases them", func(t *testing.T) {
		t.Parallel()
		r := newReview(reviewFile())

		r.toggleUnit(0)
		picked := r.unitState(0)
		r.toggleUnit(0)

		if picked != stateAll || r.unitState(0) != stateNone {
			t.Errorf("unitState(0) = %v then %v, want %v then %v", picked, r.unitState(0), stateAll, stateNone)
		}
	})

	t.Run("a partly picked hunk is completed first", func(t *testing.T) {
		t.Parallel()
		r := newReview(reviewFile())
		r.lines[0][1] = linePicked

		r.toggleUnit(0)

		if want := []lineState{lineContext, linePicked, linePicked, lineContext}; !reflect.DeepEqual(r.lines[0], want) {
			t.Errorf("lines[0] = %v, want %v", r.lines[0], want)
		}
	})

	t.Run("committed lines are left alone", func(t *testing.T) {
		t.Parallel()
		r := newReview(reviewFile())
		r.lines[0][1] = lineDone
		r.lines[1][1] = lineDone

		r.toggleUnit(0)
		r.toggleUnit(1)
		r.toggleFile()

		want := [][]lineState{{lineContext, lineDone, lineOpen, lineContext}, {lineContext, lineDone}}
		if !reflect.DeepEqual(r.lines, want) {
			t.Errorf("lines = %v, want %v", r.lines, want)
		}
	})

	t.Run("an unknown hunk is ignored", func(t *testing.T) {
		t.Parallel()
		r := newReview(reviewFile())

		r.toggleUnit(-1)
		r.toggleUnit(2)

		if got := r.state(); got != stateNone {
			t.Errorf("state() = %v, want %v", got, stateNone)
		}
	})

	t.Run("the file completes a partial selection and then clears it", func(t *testing.T) {
		t.Parallel()
		r := newReview(reviewFile())
		r.lines[0][2] = linePicked

		r.toggleFile()
		all := r.state()
		r.toggleFile()

		if all != stateAll || r.state() != stateNone {
			t.Errorf("state() = %v then %v, want %v then %v", all, r.state(), stateAll, stateNone)
		}
	})
}

func TestFileReview_commit(t *testing.T) {
	t.Parallel()

	t.Run("turns picked lines into committed ones and folds the hunk", func(t *testing.T) {
		t.Parallel()
		r := newReview(reviewFile())
		r.toggleUnit(1)

		r.commit(r.picked(), "abc", true)

		if r.unitState(1) != stateDone || r.committed[1] != "abc" || !r.folded[1] {
			t.Errorf("unit 1 = %v %q folded %v, want %v \"abc\" folded true", r.unitState(1), r.committed[1], r.folded[1], stateDone)
		}
		if r.unitState(0) != stateNone || r.committed[0] != "" || r.folded[0] {
			t.Errorf("unit 0 = %v %q folded %v, want it untouched", r.unitState(0), r.committed[0], r.folded[0])
		}
	})

	t.Run("keeps a hunk with open lines unfolded", func(t *testing.T) {
		t.Parallel()
		r := newReview(reviewFile())
		r.lines[0][1] = linePicked

		r.commit(r.picked(), "abc", true)

		want := []lineState{lineContext, lineDone, lineOpen, lineContext}
		if !reflect.DeepEqual(r.lines[0], want) || r.folded[0] || r.committed[0] != "abc" {
			t.Errorf("unit 0 = %v %q folded %v, want %v \"abc\" folded false", r.lines[0], r.committed[0], r.folded[0], want)
		}
	})
}

func TestFileReview_toggleLine(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		before lineState
		want   lineState
		ok     bool
	}{
		"picks an open line":           {before: lineOpen, want: linePicked, ok: true},
		"releases a picked line":       {before: linePicked, want: lineOpen, ok: true},
		"leaves a committed line":      {before: lineDone, want: lineDone},
		"leaves a context line as is":  {before: lineContext, want: lineContext},
		"leaves other lines untouched": {before: lineOpen, want: linePicked, ok: true},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := newReview(reviewFile())
			r.lines[0][1] = tt.before

			ok := r.toggleLine(0, 1)

			want := [][]lineState{{lineContext, tt.want, lineOpen, lineContext}, {lineContext, lineOpen}}
			if ok != tt.ok || !reflect.DeepEqual(r.lines, want) {
				t.Errorf("toggleLine(0, 1) = %v with lines %v, want %v with %v", ok, r.lines, tt.ok, want)
			}
		})
	}
}

func TestFileReview_partial(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		lines   []lineState
		removed bool
		want    bool
		shown   bool
	}{
		"nothing picked":                 {lines: []lineState{lineContext, lineOpen, lineOpen, lineContext}},
		"every line picked":              {lines: []lineState{lineContext, linePicked, linePicked, lineContext}},
		"every line committed":           {lines: []lineState{lineContext, lineDone, lineDone, lineContext}},
		"one line picked":                {lines: []lineState{lineContext, lineOpen, linePicked, lineContext}, want: true, shown: true},
		"one line committed":             {lines: []lineState{lineContext, lineDone, lineOpen, lineContext}, want: true, shown: true},
		"committed and picked lines":     {lines: []lineState{lineContext, lineDone, linePicked, lineContext}, want: true, shown: true},
		"removed lines shown on request": {lines: []lineState{lineContext, lineOpen, lineOpen, lineContext}, removed: true, shown: true},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := newReview(reviewFile())
			r.lines[0] = tt.lines
			r.removed[0] = tt.removed

			if got := r.partial(0); got != tt.want {
				t.Errorf("partial(0) = %v, want %v", got, tt.want)
			}
			if got := r.showsRemoved(0); got != tt.shown {
				t.Errorf("showsRemoved(0) = %v, want %v", got, tt.shown)
			}
		})
	}
}

func TestFileReview_toggleRemovedAll(t *testing.T) {
	t.Parallel()

	t.Run("hides every removed line when a partly selected hunk already shows its own", func(t *testing.T) {
		t.Parallel()
		f := diff.File{OldPath: "f.txt", NewPath: "f.txt", Hunks: []diff.Hunk{
			{OldStart: 1, OldLines: 1, NewStart: 1, NewLines: 1, Lines: []diff.Line{
				{Kind: diff.LineRemoved, Text: "a", OldNo: 1},
				{Kind: diff.LineAdded, Text: "A", NewNo: 1},
			}},
			{OldStart: 10, OldLines: 1, NewStart: 10, NewLines: 1, Lines: []diff.Line{
				{Kind: diff.LineRemoved, Text: "j", OldNo: 10},
				{Kind: diff.LineAdded, Text: "J", NewNo: 10},
			}},
		}}
		r := newReview(f)
		r.lines[0][1] = linePicked
		r.removed[1] = true

		r.toggleRemovedAll(f, true)

		if r.showsRemoved(1) {
			t.Errorf("showsRemoved(1) = true after toggleRemovedAll, want false")
		}
		r.lines[0][1] = lineOpen
		if r.showsRemoved(0) {
			t.Errorf("showsRemoved(0) = true once hunk 0 is no longer partly selected, want false")
		}
	})
}

func TestFileReview_pickedLines(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		lines   [][]lineState
		want    int
		partial bool
	}{
		"nothing picked":            {lines: [][]lineState{{lineContext, lineOpen, lineOpen}, {lineOpen}}},
		"a whole hunk":              {lines: [][]lineState{{lineContext, linePicked, linePicked}, {lineOpen}}, want: 2},
		"one line of a hunk":        {lines: [][]lineState{{lineContext, linePicked, lineOpen}, {linePicked}}, want: 2, partial: true},
		"the rest of a partly done": {lines: [][]lineState{{lineContext, lineDone, linePicked}, {lineOpen}}, want: 1, partial: true},
		"a committed hunk":          {lines: [][]lineState{{lineContext, lineDone, lineDone}, {linePicked}}, want: 1},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := fileReview{lines: tt.lines}

			got, partial := r.pickedLines()

			if got != tt.want || partial != tt.partial {
				t.Errorf("pickedLines() = %d, %v, want %d, %v", got, partial, tt.want, tt.partial)
			}
		})
	}
}

func TestFileReview_masks(t *testing.T) {
	t.Parallel()
	r := fileReview{lines: [][]lineState{{lineContext, lineDone, linePicked, lineOpen}, {lineContext, lineOpen}}}

	done, taken := r.masks()

	wantDone := [][]bool{{false, true, false, false}, {false, false}}
	wantTaken := [][]bool{{false, false, true, false}, {false, false}}
	if !reflect.DeepEqual(done, wantDone) || !reflect.DeepEqual(taken, wantTaken) {
		t.Errorf("masks() = %v, %v, want %v, %v", done, taken, wantDone, wantTaken)
	}
}

func TestStatsText_success(t *testing.T) {
	t.Parallel()
	h := reviewFile().Hunks[0]
	tests := map[string]struct {
		lines []lineState
		want  string
	}{
		"nothing picked":       {lines: []lineState{lineContext, lineOpen, lineOpen, lineContext}, want: "+1 −1"},
		"every line picked":    {lines: []lineState{lineContext, linePicked, linePicked, lineContext}, want: "+1 −1"},
		"the added line":       {lines: []lineState{lineContext, lineOpen, linePicked, lineContext}, want: "+1 −0 of +1 −1"},
		"the removed line":     {lines: []lineState{lineContext, linePicked, lineOpen, lineContext}, want: "+0 −1 of +1 −1"},
		"the rest of a commit": {lines: []lineState{lineContext, lineDone, linePicked, lineContext}, want: "+1 −0 of +1 −1"},
		"a committed hunk":     {lines: []lineState{lineContext, lineDone, lineDone, lineContext}, want: "+1 −1"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := statsText(h, tt.lines); got != tt.want {
				t.Errorf("statsText() = %q, want %q", got, tt.want)
			}
		})
	}
}
