package diff_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/madalinpopa/jjreview/internal/diff"
)

const twoHunks = "@@ -1,3 +1,4 @@\n a\n-b\n+x\n+y\n c\n@@ -10,3 +11,3 @@ j\n j\n-k\n+K\n l\n"

func hunksText(f diff.File) string {
	_, hunks, _ := strings.Cut(diff.Format([]diff.File{f}), "+++ b/f.txt\n")
	return hunks
}

func TestBetween_success(t *testing.T) {
	t.Parallel()
	const (
		pair      = "@@ -1,3 +1,3 @@\n a\n-b\n+B\n c\n"
		newFile   = "@@ -0,0 +1,3 @@\n+a\n+b\n+c\n"
		deleted   = "@@ -1,3 +0,0 @@\n-a\n-b\n-c\n"
		growNoEOL = "@@ -1,2 +1,3 @@\n a\n-b\n\\ No newline at end of file\n+b\n+c\n\\ No newline at end of file\n"
	)
	tests := map[string]struct {
		body string
		from []int
		to   []int
		want string
	}{
		"added line next to the line it replaces":       {body: pair, to: []int{2}, want: "@@ -1,3 +1,4 @@\n a\n b\n+B\n c\n"},
		"removed line without its replacement":          {body: pair, to: []int{1}, want: "@@ -1,3 +1,2 @@\n a\n-b\n c\n"},
		"added line after its removal was committed":    {body: pair, from: []int{1}, to: []int{1, 2}, want: "@@ -1,2 +1,3 @@\n a\n+B\n c\n"},
		"removed line after its addition was committed": {body: pair, from: []int{2}, to: []int{1, 2}, want: "@@ -1,4 +1,3 @@\n a\n-b\n B\n c\n"},
		"committed lines missing from the new mask":     {body: pair, from: []int{1}, to: []int{2}, want: "@@ -1,2 +1,3 @@\n a\n+B\n c\n"},
		"part of a new file":                            {body: newFile, to: []int{1}, want: "@@ -0,0 +1,1 @@\n+b\n"},
		"more of a new file":                            {body: newFile, from: []int{0}, to: []int{0, 1}, want: "@@ -1,1 +1,2 @@\n a\n+b\n"},
		"part of a deleted file":                        {body: deleted, to: []int{0, 2}, want: "@@ -1,3 +1,1 @@\n-a\n b\n-c\n"},
		"rest of a deleted file":                        {body: deleted, from: []int{0, 1}, to: []int{0, 1, 2}, want: "@@ -1,1 +0,0 @@\n-c\n"},
		"line that stops being the last one":            {body: growNoEOL, to: []int{3}, want: "@@ -1,2 +1,3 @@\n a\n-b\n\\ No newline at end of file\n+b\n+c\n\\ No newline at end of file\n"},
		"newline added to the last line":                {body: growNoEOL, to: []int{2}, want: "@@ -1,2 +1,3 @@\n a\n-b\n\\ No newline at end of file\n+b\n+b\n"},
		"last line kept without its newline":            {body: growNoEOL, from: []int{3}, to: []int{2, 3}, want: "@@ -1,3 +1,4 @@\n a\n b\n+b\n c\n\\ No newline at end of file\n"},
		"second hunk after a partly taken first":        {body: twoHunks, to: []int{1, 6, 7}, want: "@@ -1,3 +1,2 @@\n a\n-b\n c\n@@ -10,3 +9,3 @@ j\n j\n-k\n+K\n l\n"},
		"second hunk without the first":                 {body: twoHunks, to: []int{6, 7}, want: "@@ -10,3 +10,3 @@ j\n j\n-k\n+K\n l\n"},
		"second hunk after a committed first":           {body: twoHunks, from: []int{1, 2, 3}, to: []int{1, 2, 3, 6, 7}, want: "@@ -11,3 +11,3 @@ j\n j\n-k\n+K\n l\n"},
		"second hunk after a partly committed first":    {body: twoHunks, from: []int{2}, to: []int{2, 7}, want: "@@ -11,3 +11,4 @@ j\n j\n k\n+K\n l\n"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := parseOne(t, tt.body)

			got := between(t, f, flatMask(f, tt.from), flatMask(f, tt.to))

			if text := hunksText(got); text != tt.want {
				t.Errorf("Between() hunks = %q, want %q", text, tt.want)
			}
		})
	}
}

func between(t *testing.T, f diff.File, from, to [][]bool) diff.File {
	t.Helper()
	step, err := diff.Between(f, from, to)
	if err != nil {
		t.Fatalf("Between() error = %v", err)
	}
	return step
}

func flatMask(f diff.File, lines []int) [][]bool {
	sel := make([][]bool, len(f.Hunks))
	offset := 0
	for i, h := range f.Hunks {
		sel[i] = make([]bool, len(h.Lines))
		for _, n := range lines {
			if n >= offset && n < offset+len(h.Lines) {
				sel[i][n-offset] = true
			}
		}
		offset += len(h.Lines)
	}
	return sel
}

func TestBetween_untouchedHunks(t *testing.T) {
	t.Parallel()
	f := parseOne(t, twoHunks)

	t.Run("returns every fully taken hunk unchanged", func(t *testing.T) {
		t.Parallel()

		got := between(t, f, make([][]bool, len(f.Hunks)), lineMasks(f, true))

		if !reflect.DeepEqual(got, f) {
			t.Errorf("Between() = %+v, want %+v", got, f)
		}
	})

	t.Run("drops hunks without newly taken lines", func(t *testing.T) {
		t.Parallel()
		done := flatMask(f, []int{1, 2, 3})

		got := between(t, f, done, done)

		if len(got.Hunks) != 0 {
			t.Errorf("Between() hunks = %+v, want none", got.Hunks)
		}
	})

	t.Run("keeps the file header", func(t *testing.T) {
		t.Parallel()
		renamed := f
		renamed.OldPath, renamed.Status, renamed.NewMode = "old.txt", diff.StatusRenamed, "100755"

		got := between(t, renamed, flatMask(f, nil), flatMask(f, []int{7}))

		if got.OldPath != "old.txt" || got.NewPath != "f.txt" || got.Status != diff.StatusRenamed || got.NewMode != "100755" {
			t.Errorf("Between() = %+v, want the header of %+v", got, renamed)
		}
	})

	t.Run("leaves the source hunks alone", func(t *testing.T) {
		t.Parallel()
		source := parseOne(t, twoHunks)

		between(t, source, flatMask(source, []int{2}), flatMask(source, []int{2, 7}))

		if !reflect.DeepEqual(source, f) {
			t.Errorf("Between() changed its input to %+v, want %+v", source, f)
		}
	})
}

func TestBetween_roundTrip(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		body    string
		oldText string
	}{
		"replacement":        {body: "@@ -1,3 +1,3 @@\n a\n-b\n+B\n c\n", oldText: "a\nb\nc\n"},
		"separate blocks":    {body: "@@ -1,5 +1,5 @@\n-a\n+A\n b\n-c\n+C\n d\n-e\n+E\n", oldText: "a\nb\nc\nd\ne\n"},
		"uneven block":       {body: "@@ -1,4 +1,5 @@\n a\n-b\n-c\n+B\n+C\n+D\n d\n", oldText: "a\nb\nc\nd\n"},
		"two hunks":          {body: twoHunks, oldText: "a\nb\nc\nd\ne\nf\ng\nh\ni\nj\nk\nl\n"},
		"new file":           {body: "@@ -0,0 +1,3 @@\n+a\n+b\n+c\n", oldText: ""},
		"deleted file":       {body: "@@ -1,3 +0,0 @@\n-a\n-b\n-c\n", oldText: "a\nb\nc\n"},
		"insertion at start": {body: "@@ -1,1 +1,3 @@\n+x\n+y\n a\n", oldText: "a\n"},
		"insertion at end":   {body: "@@ -1,2 +1,4 @@\n a\n b\n+c\n+d\n", oldText: "a\nb\n"},
		"carriage returns":   {body: "@@ -1,3 +1,4 @@\n a\r\n-b\n+B\r\n+x\n c\r\n", oldText: "a\r\nb\nc\r\n"},
		"newline added":      {body: "@@ -1,2 +1,2 @@\n a\n-b\n\\ No newline at end of file\n+b\n", oldText: "a\nb"},
		"newline removed":    {body: "@@ -1,2 +1,2 @@\n a\n-b\n+b\n\\ No newline at end of file\n", oldText: "a\nb\n"},
		"grown without one":  {body: "@@ -1,2 +1,3 @@\n a\n-b\n\\ No newline at end of file\n+b\n+c\n\\ No newline at end of file\n", oldText: "a\nb"},
		"shrunk without one": {body: "@@ -1,3 +1,2 @@\n a\n-b\n-c\n\\ No newline at end of file\n+B\n\\ No newline at end of file\n", oldText: "a\nb\nc"},
		"context at the end": {body: "@@ -1,2 +1,3 @@\n+x\n a\n b\n\\ No newline at end of file\n", oldText: "a\nb"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := parseOne(t, tt.body)

			for from, to := range lineStates(f) {
				base, err := diff.ApplyLines(tt.oldText, f, from)
				if err != nil {
					t.Fatalf("ApplyLines(%v) error = %v", from, err)
				}
				want, err := diff.ApplyLines(tt.oldText, f, to)
				if err != nil {
					t.Fatalf("ApplyLines(%v) error = %v", to, err)
				}

				step := between(t, f, from, to)

				got, err := diff.ApplyLines(base, step, lineMasks(step, true))
				if err != nil {
					t.Fatalf("ApplyLines(Between(%v, %v)) error = %v", from, to, err)
				}
				if got != want {
					t.Fatalf("ApplyLines(Between(%v, %v)) = %q, want %q", from, to, got, want)
				}
				if len(step.Hunks) == 0 {
					continue
				}
				parsed, err := diff.Parse(strings.NewReader(diff.Format([]diff.File{step})))
				if err != nil {
					t.Fatalf("Parse(Format(Between(%v, %v))) error = %v", from, to, err)
				}
				if !reflect.DeepEqual(parsed, []diff.File{step}) {
					t.Fatalf("Parse(Format(Between(%v, %v))) = %+v, want %+v", from, to, parsed, step)
				}
			}
		})
	}
}

func lineStates(f diff.File) func(yield func(from, to [][]bool) bool) {
	type position struct{ hunk, line int }
	var changed []position
	for i, h := range f.Hunks {
		for n, l := range h.Lines {
			if l.Kind != diff.LineContext {
				changed = append(changed, position{i, n})
			}
		}
	}
	total := 1
	for range changed {
		total *= 3
	}
	return func(yield func(from, to [][]bool) bool) {
		for code := range total {
			from, to := lineMasks(f, false), lineMasks(f, false)
			rest := code
			for _, p := range changed {
				from[p.hunk][p.line] = rest%3 == 2
				to[p.hunk][p.line] = rest%3 >= 1
				rest /= 3
			}
			if !yield(from, to) {
				return
			}
		}
	}
}

func TestBetween_header(t *testing.T) {
	t.Parallel()
	const (
		newFile = "@@ -0,0 +1,3 @@\n+a\n+b\n+c\n"
		deleted = "@@ -1,3 +0,0 @@\n-a\n-b\n-c\n"
		edit    = "@@ -1,3 +1,3 @@\n a\n-b\n+B\n c\n"
	)
	added := diff.File{NewPath: "f.txt", NewMode: "100755", Status: diff.StatusAdded}
	removed := diff.File{OldPath: "f.txt", OldMode: "100755", Status: diff.StatusDeleted}
	renamed := diff.File{OldPath: "old.txt", NewPath: "f.txt", OldMode: "100644", NewMode: "100755", Status: diff.StatusRenamed}
	copied := diff.File{OldPath: "old.txt", NewPath: "f.txt", OldMode: "100644", NewMode: "100644", Status: diff.StatusCopied}
	tests := map[string]struct {
		header diff.File
		body   string
		from   []int
		to     []int
		want   diff.File
	}{
		"first lines of a new file create it":      {header: added, body: newFile, to: []int{0}, want: added},
		"later lines of a new file modify it":      {header: added, body: newFile, from: []int{0}, to: []int{0, 1}, want: diff.File{OldPath: "f.txt", NewPath: "f.txt", OldMode: "100755", NewMode: "100755"}},
		"some lines of a deleted file modify it":   {header: removed, body: deleted, to: []int{0}, want: diff.File{OldPath: "f.txt", NewPath: "f.txt", OldMode: "100755", NewMode: "100755"}},
		"the last lines of a deleted file drop it": {header: removed, body: deleted, from: []int{0}, to: []int{0, 1, 2}, want: removed},
		"the first lines of a rename carry it":     {header: renamed, body: edit, to: []int{1}, want: renamed},
		"later lines of a rename do not repeat it": {header: renamed, body: edit, from: []int{1}, to: []int{1, 2}, want: diff.File{OldPath: "f.txt", NewPath: "f.txt", OldMode: "100755", NewMode: "100755"}},
		"later lines of a copy do not repeat it":   {header: copied, body: edit, from: []int{1}, to: []int{1, 2}, want: diff.File{OldPath: "f.txt", NewPath: "f.txt", OldMode: "100644", NewMode: "100644"}},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := tt.header
			f.Hunks = parseOne(t, tt.body).Hunks

			got := between(t, f, flatMask(f, tt.from), flatMask(f, tt.to))

			got.Hunks = nil
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Between() header = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestBetween_failed(t *testing.T) {
	t.Parallel()
	f := parseOne(t, twoHunks)
	short := lineMasks(f, false)
	short[1] = short[1][:2]
	tests := map[string]struct {
		from [][]bool
		to   [][]bool
	}{
		"no committed masks":           {from: nil, to: lineMasks(f, true)},
		"too few committed masks":      {from: lineMasks(f, false)[:1], to: lineMasks(f, true)},
		"too many selected masks":      {from: lineMasks(f, false), to: append(lineMasks(f, true), nil)},
		"a committed mask too short":   {from: short, to: lineMasks(f, true)},
		"a selected mask of no length": {from: lineMasks(f, false), to: [][]bool{{}, nil}},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := diff.Between(f, tt.from, tt.to)

			if err == nil {
				t.Errorf("Between() = %+v, want an error", got)
			}
		})
	}
}
