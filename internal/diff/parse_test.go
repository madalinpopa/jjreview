package diff_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/madalinpopa/jjreview/internal/diff"
)

type wantFile struct {
	oldPath string
	newPath string
	status  diff.Status
	binary  bool
	hunks   []string
}

func hunkHeader(h diff.Hunk) string {
	header := fmt.Sprintf("-%d,%d +%d,%d", h.OldStart, h.OldLines, h.NewStart, h.NewLines)
	if h.Section != "" {
		header += " " + h.Section
	}
	return header
}

func parseFixture(t *testing.T, name string) []diff.File {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	files, err := diff.Parse(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Parse(%s) error = %v", name, err)
	}
	return files
}

func TestParse_fixtures(t *testing.T) {
	tests := map[string][]wantFile{
		"jj.diff": {
			{newPath: "added.txt", status: diff.StatusAdded, hunks: []string{"-0,0 +1,2 new"}},
			{oldPath: "bin.dat", newPath: "bin.dat", status: diff.StatusModified, binary: true},
			{newPath: "copied.txt", status: diff.StatusAdded, hunks: []string{"-0,0 +1,21"}},
			{oldPath: "del.txt", status: diff.StatusDeleted, hunks: []string{"-1,1 +0,0 keep"}},
			{oldPath: "emptied.txt", newPath: "emptied.txt", status: diff.StatusModified, hunks: []string{"-1,2 +0,0 a"}},
			{oldPath: "empty.txt", newPath: "empty.txt", status: diff.StatusModified},
			{oldPath: "mod.txt", newPath: "mod.txt", status: diff.StatusModified, hunks: []string{"-1,5 +1,6 top", "-22,9 +23,8 two"}},
			{newPath: "newempty.txt", status: diff.StatusAdded},
			{oldPath: "nonl.txt", newPath: "nonl.txt", status: diff.StatusModified, hunks: []string{"-1,1 +1,1 y"}},
			{oldPath: "ren.txt", newPath: "renamed.txt", status: diff.StatusRenamed, hunks: []string{"-18,3 +18,4 z"}},
			{oldPath: "sp ace.txt", newPath: "sp ace.txt", status: diff.StatusModified, hunks: []string{"-1,1 +1,1 r"}},
			{oldPath: "ünï.txt", newPath: "ünï.txt", status: diff.StatusModified, hunks: []string{"-1,1 +1,1"}},
		},
		"jj_modes.diff": {
			{oldPath: "chmodedit.sh", newPath: "chmodedit.sh", status: diff.StatusModified, hunks: []string{"-1,1 +1,1"}},
			{oldPath: "chmodonly.sh", newPath: "chmodonly.sh", status: diff.StatusModified},
			{newPath: "exec.sh", status: diff.StatusAdded, hunks: []string{"-0,0 +1,1"}},
			{oldPath: "gonex.sh", status: diff.StatusDeleted, hunks: []string{"-1,1 +0,0"}},
			{newPath: "link", status: diff.StatusAdded, hunks: []string{"-0,0 +1,1 target.txt"}},
			{oldPath: "oldlink", newPath: "oldlink", status: diff.StatusModified, hunks: []string{"-1,1 +1,1 plain.txt"}},
		},
		"jj_newline.diff": {
			{oldPath: "addnl.txt", newPath: "addnl.txt", status: diff.StatusModified, hunks: []string{"-1,3 +1,3 a"}},
			{oldPath: "ctxnl.txt", newPath: "ctxnl.txt", status: diff.StatusModified, hunks: []string{"-1,3 +1,3 A"}},
			{oldPath: "midnl.txt", newPath: "midnl.txt", status: diff.StatusModified, hunks: []string{"-1,4 +1,4 one"}},
			{oldPath: "rmnl.txt", newPath: "rmnl.txt", status: diff.StatusModified, hunks: []string{"-1,3 +1,3 a"}},
		},
		"jj_paths.diff": {
			{newPath: `q"uo te.txt`, status: diff.StatusAdded, hunks: []string{"-0,0 +1,1 a"}},
			{newPath: "t\tab", status: diff.StatusAdded, hunks: []string{"-0,0 +1,1 b"}},
		},
		"git_extended.diff": {
			{oldPath: `spü "q".txt`, newPath: `spü "q".txt`, status: diff.StatusModified, hunks: []string{"-1,1 +1,1"}},
			{oldPath: "src.txt", newPath: "dst.txt", status: diff.StatusCopied, hunks: []string{"-3,2 +3,3 func main() {"}},
			{oldPath: "old name.txt", newPath: "new name.txt", status: diff.StatusRenamed},
			{oldPath: "gone.txt", status: diff.StatusDeleted},
		},
	}

	for name, want := range tests {
		t.Run(name, func(t *testing.T) {
			files := parseFixture(t, name)

			if len(files) != len(want) {
				t.Fatalf("Parse() returned %d files, want %d", len(files), len(want))
			}
			for i, w := range want {
				got := files[i]
				if got.OldPath != w.oldPath || got.NewPath != w.newPath {
					t.Errorf("file %d paths = %q -> %q, want %q -> %q", i, got.OldPath, got.NewPath, w.oldPath, w.newPath)
				}
				if got.Status != w.status {
					t.Errorf("file %d Status = %v, want %v", i, got.Status, w.status)
				}
				if got.Binary != w.binary {
					t.Errorf("file %d Binary = %v, want %v", i, got.Binary, w.binary)
				}
				var headers []string
				for _, h := range got.Hunks {
					headers = append(headers, hunkHeader(h))
				}
				if !slices.Equal(headers, w.hunks) {
					t.Errorf("file %d hunks = %q, want %q", i, headers, w.hunks)
				}
			}
		})
	}
}

func TestParse_lineNumbers(t *testing.T) {
	files := parseFixture(t, "jj.diff")
	byPath := map[string]diff.File{}
	for _, f := range files {
		byPath[f.Path()] = f
	}

	tests := map[string]struct {
		path string
		hunk int
		want []diff.Line
	}{
		"modified first hunk": {path: "mod.txt", hunk: 0, want: []diff.Line{
			{Kind: diff.LineAdded, Text: "top", NewNo: 1},
			{Kind: diff.LineContext, Text: "1", OldNo: 1, NewNo: 2},
			{Kind: diff.LineRemoved, Text: "2", OldNo: 2},
			{Kind: diff.LineAdded, Text: "two", NewNo: 3},
			{Kind: diff.LineContext, Text: "3", OldNo: 3, NewNo: 4},
			{Kind: diff.LineContext, Text: "4", OldNo: 4, NewNo: 5},
			{Kind: diff.LineContext, Text: "5", OldNo: 5, NewNo: 6},
		}},
		"modified second hunk": {path: "mod.txt", hunk: 1, want: []diff.Line{
			{Kind: diff.LineContext, Text: "22", OldNo: 22, NewNo: 23},
			{Kind: diff.LineContext, Text: "23", OldNo: 23, NewNo: 24},
			{Kind: diff.LineContext, Text: "24", OldNo: 24, NewNo: 25},
			{Kind: diff.LineRemoved, Text: "25", OldNo: 25},
			{Kind: diff.LineAdded, Text: "twentyfive", NewNo: 26},
			{Kind: diff.LineContext, Text: "26", OldNo: 26, NewNo: 27},
			{Kind: diff.LineContext, Text: "27", OldNo: 27, NewNo: 28},
			{Kind: diff.LineRemoved, Text: "28", OldNo: 28},
			{Kind: diff.LineContext, Text: "29", OldNo: 29, NewNo: 29},
			{Kind: diff.LineContext, Text: "30", OldNo: 30, NewNo: 30},
		}},
		"no newline at end of file": {path: "nonl.txt", hunk: 0, want: []diff.Line{
			{Kind: diff.LineRemoved, Text: "x", OldNo: 1, NoNewline: true},
			{Kind: diff.LineAdded, Text: "y", NewNo: 1, NoNewline: true},
		}},
		"deleted": {path: "del.txt", hunk: 0, want: []diff.Line{
			{Kind: diff.LineRemoved, Text: "keep", OldNo: 1},
		}},
		"added": {path: "added.txt", hunk: 0, want: []diff.Line{
			{Kind: diff.LineAdded, Text: "new", NewNo: 1},
			{Kind: diff.LineAdded, Text: "file", NewNo: 2},
		}},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got := byPath[tt.path].Hunks[tt.hunk].Lines

			if !slices.Equal(got, tt.want) {
				t.Errorf("Lines = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestParse_noNewline(t *testing.T) {
	files := parseFixture(t, "jj_newline.diff")
	tests := map[string]struct {
		file int
		want []diff.Line
	}{
		"newline added": {file: 0, want: []diff.Line{
			{Kind: diff.LineContext, Text: "a", OldNo: 1, NewNo: 1},
			{Kind: diff.LineContext, Text: "b", OldNo: 2, NewNo: 2},
			{Kind: diff.LineRemoved, Text: "c", OldNo: 3, NoNewline: true},
			{Kind: diff.LineAdded, Text: "c", NewNo: 3},
		}},
		"context without newline": {file: 1, want: []diff.Line{
			{Kind: diff.LineRemoved, Text: "a", OldNo: 1},
			{Kind: diff.LineAdded, Text: "A", NewNo: 1},
			{Kind: diff.LineContext, Text: "b", OldNo: 2, NewNo: 2},
			{Kind: diff.LineContext, Text: "c", OldNo: 3, NewNo: 3, NoNewline: true},
		}},
		"newline removed": {file: 3, want: []diff.Line{
			{Kind: diff.LineContext, Text: "a", OldNo: 1, NewNo: 1},
			{Kind: diff.LineContext, Text: "b", OldNo: 2, NewNo: 2},
			{Kind: diff.LineRemoved, Text: "c", OldNo: 3},
			{Kind: diff.LineAdded, Text: "c", NewNo: 3, NoNewline: true},
		}},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got := files[tt.file].Hunks[0].Lines

			if !slices.Equal(got, tt.want) {
				t.Errorf("Lines = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestParse_modes(t *testing.T) {
	tests := map[string]struct {
		fixture string
		path    string
		oldMode string
		newMode string
	}{
		"mode change with edit":   {fixture: "jj_modes.diff", path: "chmodedit.sh", oldMode: "100644", newMode: "100755"},
		"mode change only":        {fixture: "jj_modes.diff", path: "chmodonly.sh", oldMode: "100644", newMode: "100755"},
		"executable added":        {fixture: "jj_modes.diff", path: "exec.sh", newMode: "100755"},
		"executable deleted":      {fixture: "jj_modes.diff", path: "gonex.sh", oldMode: "100755"},
		"symlink added":           {fixture: "jj_modes.diff", path: "link", newMode: "120000"},
		"symlink target changed":  {fixture: "jj_modes.diff", path: "oldlink", oldMode: "120000", newMode: "120000"},
		"unchanged mode in index": {fixture: "jj.diff", path: "mod.txt", oldMode: "100644", newMode: "100644"},
		"binary":                  {fixture: "jj.diff", path: "bin.dat", oldMode: "100644", newMode: "100644"},
		"renamed without index":   {fixture: "git_extended.diff", path: "new name.txt"},
		"index without mode":      {fixture: "git_extended.diff", path: "dst.txt"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			var got *diff.File
			files := parseFixture(t, tt.fixture)
			for i := range files {
				if files[i].Path() == tt.path {
					got = &files[i]
				}
			}
			if got == nil {
				t.Fatalf("file %q not in %s", tt.path, tt.fixture)
			}

			if got.OldMode != tt.oldMode || got.NewMode != tt.newMode {
				t.Errorf("modes = %q -> %q, want %q -> %q", got.OldMode, got.NewMode, tt.oldMode, tt.newMode)
			}
		})
	}
}

func TestParse_empty(t *testing.T) {
	files, err := diff.Parse(strings.NewReader(""))

	if err != nil || files != nil {
		t.Errorf("Parse(\"\") = %v, %v, want nil, nil", files, err)
	}
}

func TestParse_longLine(t *testing.T) {
	long := strings.Repeat("x", 1<<20)
	input := "diff --git a/f b/f\n--- a/f\n+++ b/f\n@@ -1 +1 @@\n-a\n+" + long + "\n"

	files, err := diff.Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if got := files[0].Hunks[0].Lines[1].Text; got != long {
		t.Errorf("long line length = %d, want %d", len(got), len(long))
	}
}

func TestParse_malformed(t *testing.T) {
	header := "diff --git a/f b/f\n--- a/f\n+++ b/f\n"
	tests := map[string]struct {
		input string
		line  int
	}{
		"text before header":         {input: "hello\n", line: 1},
		"unknown extended header":    {input: "diff --git a/f b/f\nwhat is this\n", line: 2},
		"missing +++":                {input: "diff --git a/f b/f\n--- a/f\n@@ -1 +1 @@\n", line: 3},
		"hunk without file header":   {input: "diff --git a/f b/f\n@@ -1 +1 @@\n", line: 2},
		"bad hunk header":            {input: header + "@@ -x +1 @@\n", line: 4},
		"hunk header missing closer": {input: header + "@@ -1 +1\n", line: 4},
		"too many removed lines":     {input: header + "@@ -1 +1 @@\n-a\n-b\n", line: 6},
		"too many added lines":       {input: header + "@@ -1 +1 @@\n+a\n+b\n", line: 6},
		"truncated hunk":             {input: header + "@@ -1,2 +1,2 @@\n a\n", line: 6},
		"unknown body prefix":        {input: header + "@@ -1 +1 @@\n*a\n", line: 5},
		"empty body line":            {input: header + "@@ -1 +1 @@\n\n", line: 5},
		"stray no newline marker":    {input: header + "\\ No newline at end of file\n", line: 4},
		"content after binary":       {input: "diff --git a/f b/f\nBinary files a/f and b/f differ\n@@ -1 +1 @@\n", line: 3},
		"missing a prefix":           {input: "diff --git f b/f\n", line: 1},
		"bad quoted path":            {input: "diff --git \"a/f b/f\n", line: 1},
		"ambiguous header path":      {input: "diff --git a/x b/y b/z\nnew file mode 100644\n", line: 1},
		"old path without prefix":    {input: "diff --git a/f b/f\n--- f\n", line: 2},
		"range starting at zero":     {input: header + "@@ -0,1 +1 @@\n", line: 4},
		"invalid mode":               {input: "diff --git a/f b/f\nold mode 10x644\n", line: 2},
		"empty mode":                 {input: "diff --git a/f b/f\nnew file mode \n", line: 2},
		"index with extra fields":    {input: "diff --git a/f b/f\nindex 1..2 100644 x\n", line: 2},
		"line after finished hunk":   {input: header + "@@ -1 +1 @@\n-a\n+b\n c\n", line: 7},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			files, err := diff.Parse(strings.NewReader(tt.input))

			if err == nil {
				t.Fatalf("Parse() = %+v, want error", files)
			}
			if want := fmt.Sprintf("line %d:", tt.line); !strings.Contains(err.Error(), want) {
				t.Errorf("Parse() error = %q, want it to contain %q", err, want)
			}
		})
	}
}

func TestFull(t *testing.T) {
	tests := map[string]struct {
		patch   string
		newText string
		want    []diff.Line
	}{
		"change in the middle": {
			patch:   "@@ -2,3 +2,3 @@\n b\n-c\n+C\n d\n",
			newText: "a\nb\nC\nd\ne\n",
			want: []diff.Line{
				{Kind: diff.LineContext, Text: "a", OldNo: 1, NewNo: 1},
				{Kind: diff.LineContext, Text: "b", OldNo: 2, NewNo: 2},
				{Kind: diff.LineRemoved, Text: "c", OldNo: 3},
				{Kind: diff.LineAdded, Text: "C", NewNo: 3},
				{Kind: diff.LineContext, Text: "d", OldNo: 4, NewNo: 4},
				{Kind: diff.LineContext, Text: "e", OldNo: 5, NewNo: 5},
			},
		},
		"change at start shifts numbers": {
			patch:   "@@ -1,1 +1,2 @@\n+z\n a\n",
			newText: "z\na\nb\n",
			want: []diff.Line{
				{Kind: diff.LineAdded, Text: "z", NewNo: 1},
				{Kind: diff.LineContext, Text: "a", OldNo: 1, NewNo: 2},
				{Kind: diff.LineContext, Text: "b", OldNo: 2, NewNo: 3},
			},
		},
		"removal at end": {
			patch:   "@@ -2,2 +2,1 @@\n b\n-c\n",
			newText: "a\nb",
			want: []diff.Line{
				{Kind: diff.LineContext, Text: "a", OldNo: 1, NewNo: 1},
				{Kind: diff.LineContext, Text: "b", OldNo: 2, NewNo: 2},
				{Kind: diff.LineRemoved, Text: "c", OldNo: 3},
			},
		},
		"pure removal without context": {
			patch:   "@@ -2,1 +1,0 @@\n-b\n",
			newText: "a\nc\n",
			want: []diff.Line{
				{Kind: diff.LineContext, Text: "a", OldNo: 1, NewNo: 1},
				{Kind: diff.LineRemoved, Text: "b", OldNo: 2},
				{Kind: diff.LineContext, Text: "c", OldNo: 3, NewNo: 2},
			},
		},
		"deleted file": {
			patch:   "@@ -1,2 +0,0 @@\n-a\n-b\n",
			newText: "",
			want: []diff.Line{
				{Kind: diff.LineRemoved, Text: "a", OldNo: 1},
				{Kind: diff.LineRemoved, Text: "b", OldNo: 2},
			},
		},
		"added file": {
			patch:   "@@ -0,0 +1,2 @@\n+a\n+b\n",
			newText: "a\nb\n",
			want: []diff.Line{
				{Kind: diff.LineAdded, Text: "a", NewNo: 1},
				{Kind: diff.LineAdded, Text: "b", NewNo: 2},
			},
		},
		"keeps no newline marker from hunk": {
			patch:   "@@ -1,1 +1,1 @@\n-a\n+b\n\\ No newline at end of file\n",
			newText: "b",
			want: []diff.Line{
				{Kind: diff.LineRemoved, Text: "a", OldNo: 1},
				{Kind: diff.LineAdded, Text: "b", NewNo: 1, NoNewline: true},
			},
		},
		"marks unchanged last line without newline": {
			patch:   "@@ -1,1 +1,1 @@\n-a\n+A\n",
			newText: "A\nb",
			want: []diff.Line{
				{Kind: diff.LineRemoved, Text: "a", OldNo: 1},
				{Kind: diff.LineAdded, Text: "A", NewNo: 1},
				{Kind: diff.LineContext, Text: "b", OldNo: 2, NewNo: 2, NoNewline: true},
			},
		},
		"no hunks": {
			newText: "a\n",
			want: []diff.Line{
				{Kind: diff.LineContext, Text: "a", OldNo: 1, NewNo: 1},
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			files, err := diff.Parse(strings.NewReader("diff --git a/f b/f\n--- a/f\n+++ b/f\n" + tt.patch))
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}

			got := diff.Full(files[0], tt.newText)

			if !slices.Equal(got, tt.want) {
				t.Errorf("Full() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func FuzzParse(f *testing.F) {
	for _, name := range []string{"jj.diff", "jj_newline.diff", "jj_modes.diff", "jj_paths.diff", "git_extended.diff"} {
		data, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			f.Fatalf("read fixture: %v", err)
		}
		f.Add(string(data))
	}
	f.Add("diff --git a/f b/f\n--- a/f\n+++ b/f\n@@ -1 +1 @@\n-a\n+b\n")

	f.Fuzz(func(t *testing.T, input string) {
		files, err := diff.Parse(strings.NewReader(input))
		if err != nil {
			return
		}
		for _, file := range files {
			for _, h := range file.Hunks {
				checkHunk(t, h)
			}
		}
		again, err := diff.Parse(strings.NewReader(diff.Format(files)))
		if err != nil {
			t.Fatalf("Parse(Format()) error = %v", err)
		}
		if !reflect.DeepEqual(again, files) {
			t.Fatalf("Parse(Format()) = %+v, want %+v", again, files)
		}
	})
}

func checkHunk(t *testing.T, h diff.Hunk) {
	t.Helper()
	oldNo, newNo := h.OldStart, h.NewStart
	var oldCount, newCount int
	for _, l := range h.Lines {
		switch l.Kind {
		case diff.LineContext:
			if l.OldNo != oldNo || l.NewNo != newNo {
				t.Fatalf("context line numbers = %d/%d, want %d/%d", l.OldNo, l.NewNo, oldNo, newNo)
			}
			oldNo, newNo, oldCount, newCount = oldNo+1, newNo+1, oldCount+1, newCount+1
		case diff.LineRemoved:
			if l.OldNo != oldNo || l.NewNo != 0 {
				t.Fatalf("removed line numbers = %d/%d, want %d/0", l.OldNo, l.NewNo, oldNo)
			}
			oldNo, oldCount = oldNo+1, oldCount+1
		case diff.LineAdded:
			if l.NewNo != newNo || l.OldNo != 0 {
				t.Fatalf("added line numbers = %d/%d, want 0/%d", l.OldNo, l.NewNo, newNo)
			}
			newNo, newCount = newNo+1, newCount+1
		}
	}
	if oldCount != h.OldLines || newCount != h.NewLines {
		t.Fatalf("hunk counts = %d/%d, header says %d/%d", oldCount, newCount, h.OldLines, h.NewLines)
	}
}
