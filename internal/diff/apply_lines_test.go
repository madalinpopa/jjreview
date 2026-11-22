package diff_test

import (
	"strings"
	"testing"

	"github.com/madalinpopa/jjreview/internal/diff"
)

func parseOne(t *testing.T, body string) diff.File {
	t.Helper()
	files, err := diff.Parse(strings.NewReader("diff --git a/f.txt b/f.txt\n--- a/f.txt\n+++ b/f.txt\n" + body))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("Parse() = %d files, want 1", len(files))
	}
	return files[0]
}

func lineMasks(f diff.File, value bool) [][]bool {
	sel := make([][]bool, len(f.Hunks))
	for i, h := range f.Hunks {
		sel[i] = make([]bool, len(h.Lines))
		for n := range sel[i] {
			sel[i][n] = value
		}
	}
	return sel
}

func takeLines(f diff.File, hunk int, lines ...int) [][]bool {
	sel := make([][]bool, len(f.Hunks))
	sel[hunk] = make([]bool, len(f.Hunks[hunk].Lines))
	for _, n := range lines {
		sel[hunk][n] = true
	}
	return sel
}

func TestApplyLines_fixtures(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		fixture string
		path    string
		oldName string
		newName string
	}{
		"newline added":             {fixture: "jj_newline.diff", path: "addnl.txt", oldName: "addnl.txt.old", newName: "addnl.txt.new"},
		"newline removed":           {fixture: "jj_newline.diff", path: "rmnl.txt", oldName: "rmnl.txt.old", newName: "rmnl.txt.new"},
		"context without newline":   {fixture: "jj_newline.diff", path: "ctxnl.txt", oldName: "ctxnl.txt.old", newName: "ctxnl.txt.new"},
		"untouched missing newline": {fixture: "jj_newline.diff", path: "midnl.txt", oldName: "midnl.txt.old", newName: "midnl.txt.new"},
		"modified":                  {fixture: "jj.diff", path: "mod.txt", oldName: "mod.txt.old", newName: "mod.txt.new"},
		"added":                     {fixture: "jj.diff", path: "added.txt", oldName: "added.txt.old", newName: "added.txt.new"},
		"copied":                    {fixture: "jj.diff", path: "copied.txt", oldName: "copied.txt.old", newName: "copied.txt.new"},
		"deleted":                   {fixture: "jj.diff", path: "del.txt", oldName: "del.txt.old", newName: "del.txt.new"},
		"emptied":                   {fixture: "jj.diff", path: "emptied.txt", oldName: "emptied.txt.old", newName: "emptied.txt.new"},
		"no newline":                {fixture: "jj.diff", path: "nonl.txt", oldName: "nonl.txt.old", newName: "nonl.txt.new"},
		"renamed":                   {fixture: "jj.diff", path: "renamed.txt", oldName: "ren.txt.old", newName: "renamed.txt.new"},
		"space in path":             {fixture: "jj.diff", path: "sp ace.txt", oldName: "sp ace.txt.old", newName: "sp ace.txt.new"},
		"unicode":                   {fixture: "jj.diff", path: "ünï.txt", oldName: "ünï.txt.old", newName: "ünï.txt.new"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := fixtureFile(t, tt.fixture, tt.path)
			oldText, newText := readApplyFixture(t, tt.oldName), readApplyFixture(t, tt.newName)

			skipped, err := diff.ApplyLines(oldText, f, make([][]bool, len(f.Hunks)))
			if err != nil {
				t.Fatalf("ApplyLines(skipped) error = %v", err)
			}
			none, err := diff.ApplyLines(oldText, f, lineMasks(f, false))
			if err != nil {
				t.Fatalf("ApplyLines(none) error = %v", err)
			}
			all, err := diff.ApplyLines(oldText, f, lineMasks(f, true))
			if err != nil {
				t.Fatalf("ApplyLines(all) error = %v", err)
			}

			if skipped != oldText {
				t.Errorf("ApplyLines(skipped) = %q, want %q", skipped, oldText)
			}
			if none != oldText {
				t.Errorf("ApplyLines(none) = %q, want %q", none, oldText)
			}
			if all != newText {
				t.Errorf("ApplyLines(all) = %q, want %q", all, newText)
			}
		})
	}
}

func TestApplyLines_partial(t *testing.T) {
	t.Parallel()
	const (
		pair      = "@@ -1,3 +1,3 @@\n a\n-b\n+B\n c\n"
		growNoEOL = "@@ -1,2 +1,3 @@\n a\n-b\n\\ No newline at end of file\n+b\n+c\n\\ No newline at end of file\n"
		dropEOL   = "@@ -1,2 +1,2 @@\n a\n-b\n+b\n\\ No newline at end of file\n"
		addEOL    = "@@ -1,1 +1,2 @@\n-a\n\\ No newline at end of file\n+a\n+b\n"
		crlf      = "@@ -1,3 +1,4 @@\n a\r\n-b\n+B\r\n+x\n c\r\n"
		newFile   = "@@ -0,0 +1,3 @@\n+a\n+b\n+c\n"
		prepend   = "@@ -1,1 +1,3 @@\n+x\n+y\n a\n"
		tail      = "@@ -1,2 +1,4 @@\n a\n b\n+c\n+d\n"
		deleted   = "@@ -1,3 +0,0 @@\n-a\n-b\n-c\n"
		blocks    = "@@ -1,5 +1,5 @@\n-a\n+A\n b\n-c\n+C\n d\n-e\n+E\n"
	)
	tests := map[string]struct {
		body    string
		oldText string
		take    []int
		want    string
	}{
		"added line keeps the line it replaces":       {body: pair, oldText: "a\nb\nc\n", take: []int{2}, want: "a\nb\nB\nc\n"},
		"removed line without its replacement":        {body: pair, oldText: "a\nb\nc\n", take: []int{1}, want: "a\nc\n"},
		"context selection is ignored":                {body: pair, oldText: "a\nb\nc\n", take: []int{0, 3}, want: "a\nb\nc\n"},
		"last added line keeps the missing newline":   {body: growNoEOL, oldText: "a\nb", take: []int{3}, want: "a\nb\nc"},
		"removing the last line restores the newline": {body: growNoEOL, oldText: "a\nb", take: []int{1}, want: "a\n"},
		"added line after a kept unterminated line":   {body: growNoEOL, oldText: "a\nb", take: []int{2}, want: "a\nb\nb\n"},
		"unterminated added line after a kept line":   {body: dropEOL, oldText: "a\nb\n", take: []int{2}, want: "a\nb\nb"},
		"removed newline only":                        {body: dropEOL, oldText: "a\nb\n", take: []int{1}, want: "a\n"},
		"terminated line after a kept one":            {body: addEOL, oldText: "a", take: []int{2}, want: "a\nb\n"},
		"carriage returns are preserved":              {body: crlf, oldText: "a\r\nb\nc\r\n", take: []int{2}, want: "a\r\nb\nB\r\nc\r\n"},
		"mixed endings in the taken lines":            {body: crlf, oldText: "a\r\nb\nc\r\n", take: []int{1, 3}, want: "a\r\nx\nc\r\n"},
		"part of a new file":                          {body: newFile, oldText: "", take: []int{1}, want: "b\n"},
		"nothing of a new file":                       {body: newFile, oldText: "", take: nil, want: ""},
		"part of an insertion at the start":           {body: prepend, oldText: "a\n", take: []int{1}, want: "y\na\n"},
		"part of an insertion at the end":             {body: tail, oldText: "a\nb\n", take: []int{3}, want: "a\nb\nd\n"},
		"part of a deleted file":                      {body: deleted, oldText: "a\nb\nc\n", take: []int{0, 2}, want: "b\n"},
		"all of a deleted file":                       {body: deleted, oldText: "a\nb\nc\n", take: []int{0, 1, 2}, want: ""},
		"lines from separate blocks":                  {body: blocks, oldText: "a\nb\nc\nd\ne\n", take: []int{1, 3, 6, 7}, want: "a\nA\nb\nd\nE\n"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := parseOne(t, tt.body)

			got, err := diff.ApplyLines(tt.oldText, f, takeLines(f, 0, tt.take...))
			if err != nil {
				t.Fatalf("ApplyLines() error = %v", err)
			}

			if got != tt.want {
				t.Errorf("ApplyLines() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestApplyLines_everyMask(t *testing.T) {
	t.Parallel()
	f := parseOne(t, "@@ -1,4 +1,4 @@\n a\n-b\n-c\n+B\n+C\n d\n")
	for bits := range 16 {
		mask := []bool{false, bits&1 != 0, bits&2 != 0, bits&4 != 0, bits&8 != 0, false}
		want := "a\n"
		if !mask[1] {
			want += "b\n"
		}
		if !mask[2] {
			want += "c\n"
		}
		if mask[3] {
			want += "B\n"
		}
		if mask[4] {
			want += "C\n"
		}
		want += "d\n"

		got, err := diff.ApplyLines("a\nb\nc\nd\n", f, [][]bool{mask})
		if err != nil {
			t.Fatalf("ApplyLines(%v) error = %v", mask, err)
		}

		if got != want {
			t.Errorf("ApplyLines(%v) = %q, want %q", mask, got, want)
		}
	}
}

func TestApplyLines_severalHunks(t *testing.T) {
	t.Parallel()
	f := fixtureFile(t, "jj.diff", "mod.txt")
	oldText := readApplyFixture(t, "mod.txt.old")
	sel := make([][]bool, len(f.Hunks))
	sel[1] = make([]bool, len(f.Hunks[1].Lines))
	for n := range sel[1] {
		sel[1][n] = true
	}

	got, err := diff.ApplyLines(oldText, f, sel)
	if err != nil {
		t.Fatalf("ApplyLines() error = %v", err)
	}

	if want := readApplyFixture(t, "mod.txt.hunk1"); got != want {
		t.Errorf("ApplyLines() = %q, want %q", got, want)
	}
}

func TestApplyLines_failed(t *testing.T) {
	t.Parallel()
	f := parseOne(t, "@@ -1,3 +1,3 @@\n a\n-b\n+B\n c\n")
	tests := map[string]struct {
		oldText string
		sel     [][]bool
	}{
		"no mask for a hunk":             {oldText: "a\nb\nc\n", sel: [][]bool{}},
		"a mask too many":                {oldText: "a\nb\nc\n", sel: [][]bool{nil, nil}},
		"mask shorter than the hunk":     {oldText: "a\nb\nc\n", sel: [][]bool{{false, true, true}}},
		"mask longer than the hunk":      {oldText: "a\nb\nc\n", sel: [][]bool{{false, true, true, false, false}}},
		"empty mask":                     {oldText: "a\nb\nc\n", sel: [][]bool{{}}},
		"kept removed line differs":      {oldText: "a\nx\nc\n", sel: [][]bool{{false, false, true, false}}},
		"context differs":                {oldText: "a\nb\nx\n", sel: [][]bool{{false, false, true, false}}},
		"old text shorter than the hunk": {oldText: "a\n", sel: [][]bool{{false, false, true, false}}},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := diff.ApplyLines(tt.oldText, f, tt.sel)

			if err == nil {
				t.Errorf("ApplyLines() = %q, want error", got)
			}
		})
	}
}
