package review

import (
	"errors"
	"reflect"
	"testing"

	"github.com/madalinpopa/jjreview/internal/diff"
)

type rowPlace struct {
	kind  rowKind
	hunk  int
	index int
}

func places(rows []row) []rowPlace {
	out := make([]rowPlace, len(rows))
	for i, r := range rows {
		out[i] = rowPlace{kind: r.kind, hunk: r.hunk, index: r.index}
	}
	return out
}

func repeatedFile() diff.File {
	return diff.File{OldPath: "f.txt", NewPath: "f.txt", Hunks: []diff.Hunk{
		{OldStart: 2, OldLines: 4, NewStart: 2, NewLines: 4, Lines: []diff.Line{
			{Kind: diff.LineContext, Text: "x", OldNo: 2, NewNo: 2},
			{Kind: diff.LineRemoved, Text: "x", OldNo: 3},
			{Kind: diff.LineRemoved, Text: "x", OldNo: 4},
			{Kind: diff.LineAdded, Text: "x", NewNo: 3},
			{Kind: diff.LineAdded, Text: "x", NewNo: 4},
			{Kind: diff.LineContext, Text: "x", OldNo: 5, NewNo: 5},
		}},
		{OldStart: 7, OldLines: 1, NewStart: 7, NewLines: 2, Lines: []diff.Line{
			{Kind: diff.LineRemoved, Text: "x", OldNo: 7},
			{Kind: diff.LineAdded, Text: "x", NewNo: 7},
			{Kind: diff.LineAdded, Text: "x", NewNo: 8},
		}},
	}}
}

const repeatedText = "p\nx\nx\nx\nx\nq\nx\nx\n"

func TestChangeRows_lineIndex(t *testing.T) {
	t.Parallel()
	file := repeatedFile()

	t.Run("with removed lines hidden", func(t *testing.T) {
		t.Parallel()

		got := places(changeRows(file, newReview(file), -1))

		want := []rowPlace{
			{rowGap, -1, -1},
			{rowHunk, 0, -1}, {rowLine, 0, 0}, {rowLine, 0, 3}, {rowLine, 0, 4}, {rowLine, 0, 5},
			{rowGap, -1, -1},
			{rowHunk, 1, -1}, {rowLine, 1, 1}, {rowLine, 1, 2},
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("changeRows() = %v, want %v", got, want)
		}
	})

	t.Run("with removed lines shown", func(t *testing.T) {
		t.Parallel()
		review := newReview(file)
		review.removed[0], review.removed[1] = true, true

		got := places(changeRows(file, review, -1))

		want := []rowPlace{
			{rowGap, -1, -1},
			{rowHunk, 0, -1}, {rowLine, 0, 0}, {rowLine, 0, 1}, {rowLine, 0, 2}, {rowLine, 0, 3}, {rowLine, 0, 4}, {rowLine, 0, 5},
			{rowGap, -1, -1},
			{rowHunk, 1, -1}, {rowLine, 1, 0}, {rowLine, 1, 1}, {rowLine, 1, 2},
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("changeRows() = %v, want %v", got, want)
		}
	})
}

func TestInterleave_lineIndex(t *testing.T) {
	t.Parallel()
	file := repeatedFile()
	lines := diff.Full(file, repeatedText)

	t.Run("with removed lines hidden", func(t *testing.T) {
		t.Parallel()

		got := places(interleave(file, lines, newReview(file)))

		want := []rowPlace{
			{rowLine, -1, -1},
			{rowLine, 0, 0}, {rowLine, 0, 3}, {rowLine, 0, 4}, {rowLine, 0, 5},
			{rowLine, -1, -1},
			{rowLine, 1, 1}, {rowLine, 1, 2},
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("interleave() = %v, want %v", got, want)
		}
	})

	t.Run("with removed lines shown", func(t *testing.T) {
		t.Parallel()
		review := newReview(file)
		review.removed[0], review.removed[1] = true, true

		got := places(interleave(file, lines, review))

		want := []rowPlace{
			{rowLine, -1, -1},
			{rowLine, 0, 0}, {rowLine, 0, 1}, {rowLine, 0, 2}, {rowLine, 0, 3}, {rowLine, 0, 4}, {rowLine, 0, 5},
			{rowLine, -1, -1},
			{rowLine, 1, 0}, {rowLine, 1, 1}, {rowLine, 1, 2},
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("interleave() = %v, want %v", got, want)
		}
	})

	t.Run("each row carries the line at its index", func(t *testing.T) {
		t.Parallel()
		review := newReview(file)
		review.removed[0], review.removed[1] = true, true

		for _, r := range interleave(file, lines, review) {
			if r.hunk < 0 {
				continue
			}
			if want := file.Hunks[r.hunk].Lines[r.index]; r.line != want {
				t.Errorf("row %d:%d line = %+v, want %+v", r.hunk, r.index, r.line, want)
			}
		}
	})
}

func TestRows_noLineIndex(t *testing.T) {
	t.Parallel()
	link := diff.File{NewPath: "link", NewMode: symlinkMode, Status: diff.StatusAdded, Hunks: []diff.Hunk{
		{NewStart: 1, NewLines: 1, Lines: []diff.Line{{Kind: diff.LineAdded, Text: "target", NewNo: 1, NoNewline: true}}},
	}}

	got := places(append(wholeRows(link), messageRow("m"), errorRow(errors.New("boom"))))

	want := []rowPlace{{rowMessage, -1, -1}, {rowLine, -1, -1}, {rowMessage, -1, -1}, {rowError, -1, -1}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("rows = %v, want %v", got, want)
	}
}
