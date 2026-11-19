package diff_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/madalinpopa/jjreview/internal/diff"
)

func TestFormat_roundTripFixtures(t *testing.T) {
	for _, name := range []string{"jj.diff", "jj_newline.diff", "jj_modes.diff", "jj_paths.diff", "git_extended.diff"} {
		t.Run(name, func(t *testing.T) {
			files := parseFixture(t, name)

			got, err := diff.Parse(strings.NewReader(diff.Format(files)))
			if err != nil {
				t.Fatalf("Parse(Format()) error = %v", err)
			}

			if !reflect.DeepEqual(got, files) {
				t.Errorf("Parse(Format()) = %+v, want %+v", got, files)
			}
		})
	}
}

func TestFormat_output(t *testing.T) {
	hunk := diff.Hunk{OldStart: 1, OldLines: 2, NewStart: 1, NewLines: 2, Section: "func f() {", Lines: []diff.Line{
		{Kind: diff.LineContext, Text: "a", OldNo: 1, NewNo: 1},
		{Kind: diff.LineRemoved, Text: "b", OldNo: 2},
		{Kind: diff.LineAdded, Text: "B", NewNo: 2},
	}}
	tests := map[string]struct {
		file diff.File
		want string
	}{
		"modified": {
			file: diff.File{OldPath: "f.go", NewPath: "f.go", Hunks: []diff.Hunk{hunk}},
			want: "diff --git a/f.go b/f.go\n--- a/f.go\n+++ b/f.go\n@@ -1,2 +1,2 @@ func f() {\n a\n-b\n+B\n",
		},
		"added": {
			file: diff.File{NewPath: "n", Status: diff.StatusAdded, Hunks: []diff.Hunk{{NewStart: 1, NewLines: 1, Lines: []diff.Line{{Kind: diff.LineAdded, Text: "x", NewNo: 1}}}}},
			want: "diff --git a/n b/n\nnew file mode 100644\n--- /dev/null\n+++ b/n\n@@ -0,0 +1,1 @@\n+x\n",
		},
		"deleted without hunks": {
			file: diff.File{OldPath: "d", Status: diff.StatusDeleted},
			want: "diff --git a/d b/d\ndeleted file mode 100644\n",
		},
		"renamed": {
			file: diff.File{OldPath: "a b", NewPath: "c", Status: diff.StatusRenamed},
			want: "diff --git a/a b b/c\nrename from a b\nrename to c\n",
		},
		"copied": {
			file: diff.File{OldPath: "a", NewPath: "c", Status: diff.StatusCopied},
			want: "diff --git a/a b/c\ncopy from a\ncopy to c\n",
		},
		"binary": {
			file: diff.File{OldPath: "x.bin", NewPath: "x.bin", Binary: true},
			want: "diff --git a/x.bin b/x.bin\nBinary files a/x.bin and b/x.bin differ\n",
		},
		"no newline marker": {
			file: diff.File{OldPath: "f", NewPath: "f", Hunks: []diff.Hunk{{OldStart: 1, OldLines: 1, NewStart: 1, NewLines: 1, Lines: []diff.Line{
				{Kind: diff.LineRemoved, Text: "a", OldNo: 1, NoNewline: true},
				{Kind: diff.LineAdded, Text: "b", NewNo: 1, NoNewline: true},
			}}}},
			want: "diff --git a/f b/f\n--- a/f\n+++ b/f\n@@ -1,1 +1,1 @@\n-a\n\\ No newline at end of file\n+b\n\\ No newline at end of file\n",
		},
		"mode change only": {
			file: diff.File{OldPath: "s", NewPath: "s", OldMode: "100644", NewMode: "100755"},
			want: "diff --git a/s b/s\nold mode 100644\nnew mode 100755\n",
		},
		"unchanged mode": {
			file: diff.File{OldPath: "s", NewPath: "s", OldMode: "100644", NewMode: "100644"},
			want: "diff --git a/s b/s\nindex 0000000..0000000 100644\n",
		},
		"symlink added": {
			file: diff.File{NewPath: "l", NewMode: "120000", Status: diff.StatusAdded},
			want: "diff --git a/l b/l\nnew file mode 120000\n",
		},
		"executable deleted": {
			file: diff.File{OldPath: "x", OldMode: "100755", Status: diff.StatusDeleted},
			want: "diff --git a/x b/x\ndeleted file mode 100755\n",
		},
		"quoted path": {
			file: diff.File{OldPath: "q\"\n", NewPath: "q\"\n"},
			want: "diff --git \"a/q\\\"\\n\" \"b/q\\\"\\n\"\n",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got := diff.Format([]diff.File{tt.file})

			if got != tt.want {
				t.Errorf("Format() = %q, want %q", got, tt.want)
			}
		})
	}
}
