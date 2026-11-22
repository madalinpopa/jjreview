package tui_test

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/madalinpopa/jjreview/internal/diff"
	"github.com/madalinpopa/jjreview/internal/jj"
	"github.com/madalinpopa/jjreview/internal/tui"
)

const (
	partMain   = "l1\nl2\nkeep\nold line\nnew line\nl5\nl6\nl7\nl8\nl9\nx\nl11\nl12\n"
	firstMain  = "l1\nl2\nkeep\nnew line\nl5\nl6\nl7\nl8\nl9\nx\nl11\nl12\n"
	fullMain   = "l1\nl2\nkeep\nnew line\nl5\nl6\nl7\nl8\nl9\nx\ny\nl11\nl12\n"
	goneParent = "one\ntwo\nthree\n"
)

func fullSource() *fakeSource {
	src := newSource()
	src.contents["commit1:src/main.go"] = fullMain
	return src
}

func onMain(t *testing.T, src *fakeSource) tui.Model {
	t.Helper()
	return press(t, loaded(t, src), "tab", "j", "tab")
}

func goneFile() diff.File {
	return diff.File{OldPath: "gone.txt", OldMode: "100755", Status: diff.StatusDeleted, Hunks: []diff.Hunk{
		{OldStart: 1, OldLines: 3, Lines: []diff.Line{
			{Kind: diff.LineRemoved, Text: "one", OldNo: 1},
			{Kind: diff.LineRemoved, Text: "two", OldNo: 2},
			{Kind: diff.LineRemoved, Text: "three", OldNo: 3},
		}},
	}}
}

func movedFile() diff.File {
	f := mainFile()
	f.OldPath, f.Status = "src/old.go", diff.StatusRenamed
	return f
}

func splitFiles(t *testing.T, src *fakeSource, n int) []jj.SplitFile {
	t.Helper()
	if len(src.splits) <= n {
		t.Fatalf("splits = %d, want more than %d", len(src.splits), n)
	}
	return src.splits[n].files
}

func TestLineSelect_success(t *testing.T) {
	t.Run("s selects the changed line under the cursor", func(t *testing.T) {
		m := press(t, onMain(t, newSource()), "s")

		got := view(m)
		assertContains(t, got, "▾ ◐ @@ -3,2 +3,2 @@", "+1 −0 of +1 −1", "1 hunk selected (1 line)", "◐ main.go")
		assertContains(t, lineWith(t, got, "new line"), "4●+ new line")
	})

	t.Run("a partly selected hunk header keeps its plain background", func(t *testing.T) {
		m := press(t, onMain(t, newSource()), "s")

		if regexp.MustCompile(`\x1b\[([0-9]+;)*44(;[0-9]+)*m[^\x1b]*@@ -3,2 \+3,2 @@`).MatchString(m.View().Content) {
			t.Error("View() partly selected hunk header has the ANSI blue (SGR 44) background")
		}
	})

	t.Run("s twice clears the line", func(t *testing.T) {
		m := press(t, onMain(t, newSource()), "s", "s")

		got := view(m)
		assertContains(t, got, "▾ ○ @@ -3,2 +3,2 @@", "nothing selected", "○ main.go", "4 + new line")
		assertMissing(t, got, " of +1 −1", "- old line")
	})

	t.Run("a partly selected hunk shows its removed lines", func(t *testing.T) {
		m := press(t, onMain(t, newSource()), "s")

		got := view(m)
		assertContains(t, got, "- old line", "1 removed line not selected")
		assertContains(t, lineWith(t, got, "- old line"), "     - old line")
		if c := m.Cursor(); c != 4 {
			t.Errorf("Cursor() = %d, want 4, still on the added line", c)
		}
	})

	t.Run("a selected hunk turned partial reports no removed lines to select", func(t *testing.T) {
		m := press(t, onMain(t, newSource()), "space", "s")

		got := view(m)
		assertContains(t, got, "●- old line", "1 hunk selected (1 line)")
		assertMissing(t, got, "removed lines not selected")
	})

	t.Run("d reports nothing on a partly selected hunk without removed lines", func(t *testing.T) {
		src := newSource()
		src.files = []diff.File{viewFile()}

		m := press(t, loaded(t, src), "j", "s", "d")

		got := view(m)
		assertContains(t, got, "1 hunk selected (1 line)")
		assertMissing(t, got, "removed lines stay visible")
	})

	t.Run("S selects a whole file as the line key says", func(t *testing.T) {
		m := press(t, onMain(t, newSource()), "J", "J", "s")
		assertContains(t, view(m), "this file can only be selected whole; press S", "○ img.png")

		assertContains(t, view(press(t, m, "S")), "● img.png", "1 hunk selected")
	})

	t.Run("d keeps the removed lines of a partly selected hunk", func(t *testing.T) {
		m := press(t, onMain(t, newSource()), "s", "d")

		assertContains(t, view(m), "- old line", "removed lines stay visible in a partly selected hunk")
	})

	t.Run("s selects a removed line", func(t *testing.T) {
		m := press(t, onMain(t, newSource()), "d", "k", "s")

		got := view(m)
		assertContains(t, got, "▾ ◐ @@ -3,2 +3,2 @@", "+0 −1 of +1 −1", "1 hunk selected (1 line)")
		assertContains(t, lineWith(t, got, "- old line"), "●- old line")
	})

	t.Run("space completes a partly selected hunk then clears it", func(t *testing.T) {
		m := press(t, onMain(t, newSource()), "s", "space")

		got := view(m)
		assertContains(t, got, "▾ ● @@ -3,2 +3,2 @@", "1 hunk selected", "●+ new line")
		assertMissing(t, got, "(1 line)", " of +1 −1", "- old line")

		got = view(press(t, m, "space"))
		assertContains(t, got, "▾ ○ @@ -3,2 +3,2 @@", "nothing selected")
	})

	t.Run("S completes a partly selected file then clears it", func(t *testing.T) {
		m := press(t, onMain(t, newSource()), "s", "S")

		got := view(m)
		assertContains(t, got, "● main.go", "2 hunks selected")
		assertMissing(t, got, "lines)")

		assertContains(t, view(press(t, m, "S")), "○ main.go", "nothing selected")
	})

	t.Run("counts the lines of every selected hunk when one is partly selected", func(t *testing.T) {
		m := press(t, onMain(t, newSource()), "s", "]", "space")

		assertContains(t, view(m), "2 hunks selected (2 lines)")
	})

	t.Run("a selected hunk marks its lines in the full file", func(t *testing.T) {
		m := press(t, onMain(t, fullSource()), "f", "space")

		got := view(m)
		assertContains(t, got, "1 hunk selected")
		assertContains(t, lineWith(t, got, "new line"), "4●+ new line")
	})

	t.Run("s selects a line in the full file", func(t *testing.T) {
		m := press(t, onMain(t, fullSource()), "f", "s")

		got := view(m)
		assertContains(t, got, "1 hunk selected (1 line)", "- old line")
		assertContains(t, lineWith(t, got, "new line"), "4●+ new line")
	})

	t.Run("quitting names the selected lines", func(t *testing.T) {
		m := press(t, onMain(t, newSource()), "s", "q")

		assertContains(t, view(m), "press q again to drop 1 selected hunk (1 line)")
	})

	t.Run("the help lists the line key", func(t *testing.T) {
		assertContains(t, view(press(t, loaded(t, newSource()), "?")), "select the line")
	})

	t.Run("the bar names the line key in the diff pane", func(t *testing.T) {
		assertContains(t, view(loaded(t, newSource())), "s space select")
		assertMissing(t, view(press(t, loaded(t, newSource()), "tab")), "s space select")
	})
}

func TestLineSelect_failed(t *testing.T) {
	tests := map[string]struct {
		keys []string
		want string
	}{
		"a context line":  {keys: []string{"k", "s"}, want: "s selects a changed line; space selects the hunk"},
		"a hunk header":   {keys: []string{"k", "k", "s"}, want: "s selects a changed line; space selects the hunk"},
		"the file pane":   {keys: []string{"tab", "s"}, want: "s works in the diff pane; press tab"},
		"a whole file":    {keys: []string{"J", "J", "s"}, want: "this file can only be selected whole; press S"},
		"a full file row": {keys: []string{"f", "k", "k", "s"}, want: "s selects a changed line; space selects the hunk"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			m := press(t, onMain(t, fullSource()), tt.keys...)

			assertContains(t, view(m), tt.want, "nothing selected")
		})
	}

	t.Run("an immutable revision", func(t *testing.T) {
		src := newSource()
		src.change.Immutable = true

		m := press(t, onMain(t, src), "s")

		assertContains(t, view(m), "immutable revision: read only")
		assertMissing(t, view(m), "selected (")
	})

	t.Run("a committed line", func(t *testing.T) {
		m := press(t, onMain(t, newSource()), "s", "c", "ctrl+s", "s")

		assertContains(t, view(m), "line already committed", "nothing selected")
	})
}

func TestLineCommit_success(t *testing.T) {
	t.Run("commits the selected added line and keeps the old one", func(t *testing.T) {
		src := newSource()

		press(t, onMain(t, src), "s", "c", "ctrl+s")

		want := []jj.SplitFile{{Path: "src/main.go", Content: partMain, Mode: "100644"}}
		if got := splitFiles(t, src, 0); !slices.Equal(got, want) {
			t.Errorf("split files = %+v, want %+v", got, want)
		}
	})

	t.Run("commits the selected removed line without its replacement", func(t *testing.T) {
		src := newSource()

		press(t, onMain(t, src), "d", "k", "s", "c", "ctrl+s")

		want := []jj.SplitFile{{Path: "src/main.go", Content: "l1\nl2\nkeep\nl5\nl6\nl7\nl8\nl9\nx\nl11\nl12\n", Mode: "100644"}}
		if got := splitFiles(t, src, 0); !slices.Equal(got, want) {
			t.Errorf("split files = %+v, want %+v", got, want)
		}
	})

	t.Run("keeps the rest of a partly deleted file", func(t *testing.T) {
		src := newSource()
		src.files = []diff.File{goneFile()}
		src.contents["parent1:gone.txt"] = goneParent

		press(t, loaded(t, src), "j", "s", "c", "ctrl+s")

		want := []jj.SplitFile{{Path: "gone.txt", Content: "one\nthree\n"}}
		if got := splitFiles(t, src, 0); !slices.Equal(got, want) {
			t.Errorf("split files = %+v, want %+v", got, want)
		}
	})

	t.Run("deletes a file once its last lines are selected", func(t *testing.T) {
		src := newSource()
		src.files = []diff.File{goneFile()}
		src.contents["parent1:gone.txt"] = goneParent
		m := press(t, loaded(t, src), "j", "s", "c", "ctrl+s")

		press(t, m, "space", "c", "ctrl+s")

		want := []jj.SplitFile{{Path: "gone.txt", Delete: true}}
		if got := splitFiles(t, src, 1); !slices.Equal(got, want) {
			t.Errorf("split files = %+v, want %+v", got, want)
		}
	})

	t.Run("moves a partly selected renamed file with the selected lines", func(t *testing.T) {
		src := newSource()
		src.files = []diff.File{movedFile()}
		src.contents["parent1:src/old.go"] = mainParent

		press(t, loaded(t, src), "s", "c", "ctrl+s")

		want := []jj.SplitFile{{Path: "src/main.go", Content: partMain, Mode: "100644"}, {Path: "src/old.go", Delete: true}}
		if got := splitFiles(t, src, 0); !slices.Equal(got, want) {
			t.Errorf("split files = %+v, want %+v", got, want)
		}
	})

	t.Run("creates a new file with the selected lines", func(t *testing.T) {
		src := newSource()
		src.files = []diff.File{viewFile()}

		press(t, loaded(t, src), "j", "s", "c", "ctrl+s")

		want := []jj.SplitFile{{Path: "src/ui/view.go", Content: "func View() {}\n", Mode: "100644"}}
		if got := splitFiles(t, src, 0); !slices.Equal(got, want) {
			t.Errorf("split files = %+v, want %+v", got, want)
		}
	})

	t.Run("a second commit in the same hunk adds to the first", func(t *testing.T) {
		src := newSource()
		m := press(t, onMain(t, src), "s", "c", "ctrl+s")

		press(t, m, "k", "s", "c", "ctrl+s")

		want := []jj.SplitFile{{Path: "src/main.go", Content: firstMain, Mode: "100644"}}
		if got := splitFiles(t, src, 1); !slices.Equal(got, want) {
			t.Errorf("second split files = %+v, want %+v", got, want)
		}
	})
}

func TestLineCommit_display(t *testing.T) {
	t.Run("a partly committed hunk stays open with its committed lines dimmed", func(t *testing.T) {
		src := newSource()

		m := press(t, onMain(t, src), "s", "c", "ctrl+s")

		got := view(m)
		assertContains(t, got, "▾ ○ @@ -3,2 +3,2 @@", "+1 −1 ✓ 1/2 kxyzabcd", "- old line", "4 + new line", "nothing selected", "○ main.go")
		assertMissing(t, got, "▸ ✓ @@ -3,2 +3,2 @@", "●+ new line")
		if c := m.Cursor(); c != 4 {
			t.Errorf("Cursor() = %d, want 4, still on the committed line", c)
		}
	})

	t.Run("the rest of a partly committed hunk is selected in part", func(t *testing.T) {
		m := press(t, onMain(t, newSource()), "s", "c", "ctrl+s", "k", "s")

		assertContains(t, view(m), "▾ ● @@ -3,2 +3,2 @@", "+0 −1 of +1 −1 ✓ 1/2 kxyzabcd", "1 hunk selected (1 line)")
	})

	t.Run("a hunk folds once its last line is committed", func(t *testing.T) {
		src := newSource()
		m := press(t, onMain(t, src), "s", "c", "ctrl+s")

		m = press(t, m, "k", "s", "c", "ctrl+s")

		got := view(m)
		assertContains(t, got, "▸ ✓ @@ -3,2 +3,2 @@", "+1 −1 · folded rest1zzz")
		assertMissing(t, got, "1/2", "- old line")
	})
}

func TestLineCommit_cursor(t *testing.T) {
	t.Run("stays on the line when the last line of a hunk is committed in the full file", func(t *testing.T) {
		m := press(t, onMain(t, fullSource()), "f", "s", "c", "ctrl+s", "space")

		m = press(t, m, "c", "ctrl+s")

		assertMissing(t, view(m), "- old line")
		if c := m.Cursor(); c != 3 {
			t.Errorf("Cursor() = %d, want 3, still on the added line", c)
		}
	})

	t.Run("moves up by the rows that left the full file", func(t *testing.T) {
		m := press(t, onMain(t, fullSource()), "f", "s", "c", "ctrl+s", "space", "j")
		before := m.Cursor()

		m = press(t, m, "c", "ctrl+s")

		if c := m.Cursor(); c != before-1 {
			t.Errorf("Cursor() = %d, want %d, still on the line below the committed hunk", c, before-1)
		}
	})

	t.Run("moves to the nearest line when its removed line leaves the full file", func(t *testing.T) {
		m := press(t, onMain(t, fullSource()), "f", "s", "c", "ctrl+s", "space", "k")

		m = press(t, m, "c", "ctrl+s")

		if c := m.Cursor(); c != 3 {
			t.Errorf("Cursor() = %d, want 3, on the added line next to the removed one", c)
		}
	})

	t.Run("a partial commit keeps a folded hunk folded", func(t *testing.T) {
		m := press(t, onMain(t, newSource()), "s", "z", "]", "j")
		before := m.Cursor()

		m = press(t, m, "c", "ctrl+s")

		got := view(m)
		assertContains(t, got, "▸ ○ @@ -3,2 +3,2 @@")
		assertMissing(t, got, "- old line")
		if c := m.Cursor(); c != before {
			t.Errorf("Cursor() = %d, want %d, still on the line of the next hunk", c, before)
		}
	})
}

func TestLinePatch_success(t *testing.T) {
	header := "diff --git a/src/main.go b/src/main.go\nindex 0000000..0000000 100644\n--- a/src/main.go\n+++ b/src/main.go\n"

	t.Run("generates from the selected lines only", func(t *testing.T) {
		gen := &fakeGenerator{text: "feat: line"}
		m := start(t, newSource(), gen, 120, 30)

		m = press(t, m, "tab", "j", "tab", "s", "c")

		want := header + "@@ -3,2 +3,3 @@\n keep\n old line\n+new line\n"
		if !slices.Equal(gen.patches, []string{want}) {
			t.Errorf("patches = %q, want %q", gen.patches, want)
		}
		assertContains(t, view(m), "src/main.go  @@ -3,2 +3,2 @@  +1 −0 of +1 −1", "✨ generated from 1 line, edit freely")
	})

	t.Run("leaves committed lines out of the next patch", func(t *testing.T) {
		gen := &fakeGenerator{text: "feat: line"}
		m := start(t, newSource(), gen, 120, 30)
		m = press(t, m, "tab", "j", "tab", "s", "c", "ctrl+s")

		press(t, m, "k", "s", "c")

		want := header + "@@ -3,3 +3,2 @@\n keep\n-old line\n new line\n"
		if len(gen.patches) != 2 || gen.patches[1] != want {
			t.Errorf("patches = %q, want the second to be %q", gen.patches, want)
		}
	})

	t.Run("numbers a later hunk without the unselected one before it", func(t *testing.T) {
		gen := &fakeGenerator{text: "feat: line"}
		src := newSource()
		f := mainFile()
		f.Hunks[0].Lines = append(f.Hunks[0].Lines, diff.Line{Kind: diff.LineAdded, Text: "more", NewNo: 5})
		f.Hunks[0].NewLines, f.Hunks[1].NewStart = 3, 11
		f.Hunks[1].Lines[0].NewNo, f.Hunks[1].Lines[1].NewNo = 11, 12
		src.files = []diff.File{f}
		m := start(t, src, gen, 120, 30)

		press(t, m, "]", "space", "c")

		want := header + "@@ -10,1 +10,2 @@\n x\n+y\n"
		if !slices.Equal(gen.patches, []string{want}) {
			t.Errorf("patches = %q, want %q", gen.patches, want)
		}
	})
}

func rawLineWith(t *testing.T, m tui.Model, needle string) string {
	t.Helper()
	for l := range strings.SplitSeq(m.View().Content, "\n") {
		if strings.Contains(ansiPattern.ReplaceAllString(l, ""), needle) {
			return l
		}
	}
	t.Fatalf("View() has no line with %q:\n%s", needle, view(m))
	return ""
}

func TestLineRange_success(t *testing.T) {
	t.Run("v then s selects the changed lines between the anchor and the cursor", func(t *testing.T) {
		m := press(t, onMain(t, newSource()), "v", "G")
		before := m.Cursor()

		m = press(t, m, "s")

		got := view(m)
		assertContains(t, got, "2 hunks selected (2 lines)", "- old line", "1 removed line not selected", "s space select")
		assertContains(t, lineWith(t, got, "new line"), "4●+ new line")
		assertContains(t, lineWith(t, got, "+ y"), "11●+ y")
		if m.Cursor() != before+1 {
			t.Errorf("Cursor() = %d, want %d: the line under the cursor moved down by the removed line", m.Cursor(), before+1)
		}
	})

	t.Run("reports the removed lines of a hunk that turns partial while another hides its own", func(t *testing.T) {
		src := newSource()
		src.files = []diff.File{{OldPath: "pairs.txt", NewPath: "pairs.txt", OldMode: "100644", NewMode: "100644", Hunks: []diff.Hunk{
			{OldStart: 1, OldLines: 1, NewStart: 1, NewLines: 1, Lines: []diff.Line{
				{Kind: diff.LineRemoved, Text: "x", OldNo: 1},
				{Kind: diff.LineAdded, Text: "y", NewNo: 1},
			}},
			{OldStart: 10, OldLines: 1, NewStart: 10, NewLines: 1, Lines: []diff.Line{
				{Kind: diff.LineRemoved, Text: "p", OldNo: 10},
				{Kind: diff.LineAdded, Text: "q", NewNo: 10},
			}},
		}}}
		m := press(t, loaded(t, src), "g", "s")
		assertContains(t, view(m), "- x", "1 hunk selected (1 line)")

		m = press(t, m, "k", "v", "G", "s")

		got := view(m)
		assertContains(t, got, "2 hunks selected (3 lines)", "- p", "1 removed line not selected")
		assertMissing(t, got, "- x")
		assertContains(t, lineWith(t, got, "+ q"), "10●+ q")
	})

	t.Run("space applies the range", func(t *testing.T) {
		m := press(t, onMain(t, newSource()), "v", "G", "space")

		assertContains(t, view(m), "2 hunks selected (2 lines)")
	})

	t.Run("a range extended upwards selects the same lines", func(t *testing.T) {
		m := press(t, onMain(t, newSource()), "G", "v", "g", "s")

		assertContains(t, view(m), "2 hunks selected (2 lines)")
	})

	t.Run("a range selects the removed lines it shows", func(t *testing.T) {
		m := press(t, onMain(t, newSource()), "d", "g", "v", "G", "s")

		assertContains(t, view(m), "2 hunks selected")
		assertMissing(t, view(m), "selected (")
	})

	t.Run("a range with an unselected line selects the rest", func(t *testing.T) {
		m := press(t, onMain(t, newSource()), "s", "v", "G", "s")

		got := view(m)
		assertContains(t, lineWith(t, got, "new line"), "4●+ new line")
		assertContains(t, lineWith(t, got, "+ y"), "11●+ y")
	})

	t.Run("a fully selected range is cleared", func(t *testing.T) {
		m := press(t, onMain(t, newSource()), "d", "g", "v", "G", "s", "v", "g", "s")

		assertContains(t, view(m), "nothing selected")
	})

	t.Run("a folded hunk in the range is left alone", func(t *testing.T) {
		m := press(t, onMain(t, newSource()), "]", "z", "g", "v", "G", "s")

		assertContains(t, view(m), "1 hunk selected (1 line)", "▸ ○ @@ -10,1 +10,2 @@")
	})

	t.Run("a committed line in the range is left alone", func(t *testing.T) {
		src := newSource()

		m := press(t, onMain(t, src), "s", "c", "ctrl+s", "g", "v", "G", "s", "c", "ctrl+s")

		want := []jj.SplitFile{{Path: "src/main.go", Content: fullMain, Mode: "100644"}}
		if got := splitFiles(t, src, 1); !slices.Equal(got, want) {
			t.Errorf("second split files = %+v, want %+v", got, want)
		}
		assertContains(t, view(m), "nothing selected")
	})

	t.Run("the bar and the status name the range while it is open", func(t *testing.T) {
		m := press(t, onMain(t, newSource()), "v")

		assertContains(t, view(m), "selecting a range", "s space select range", "esc v cancel")
	})

	t.Run("the rows of the range are marked", func(t *testing.T) {
		plain := press(t, onMain(t, newSource()), "G")
		ranged := press(t, onMain(t, newSource()), "v", "G")
		closed := press(t, ranged, "esc")

		if rawLineWith(t, ranged, "new line") == rawLineWith(t, plain, "new line") {
			t.Error("View() draws a row inside the range like a row outside it")
		}
		if rawLineWith(t, closed, "new line") != rawLineWith(t, plain, "new line") {
			t.Error("View() still marks the row after the range was cancelled")
		}
	})

	t.Run("the help lists the range key", func(t *testing.T) {
		assertContains(t, view(press(t, loaded(t, newSource()), "?")), "s v", "select the line / a range of lines")
	})
}

func TestLineRange_cancel(t *testing.T) {
	for _, k := range []string{"esc", "v"} {
		t.Run(k+" leaves the range without selecting", func(t *testing.T) {
			m := press(t, onMain(t, newSource()), "v", "G", k)

			assertContains(t, view(m), "nothing selected", "s space select")
			assertMissing(t, view(m), "select range", "selecting a range")

			got := view(press(t, m, "s"))
			assertContains(t, got, "1 hunk selected")
			assertContains(t, lineWith(t, got, "+ y"), "11●+ y")
		})
	}

	tests := map[string]struct {
		key  string
		want func(tui.Model) bool
	}{
		"tab switches the pane":        {key: "tab", want: func(m tui.Model) bool { return m.FilesFocused() }},
		"J moves to the next file":     {key: "J", want: func(m tui.Model) bool { return m.CurrentFile() == "README.md" }},
		"K moves to the previous file": {key: "K", want: func(m tui.Model) bool { return m.CurrentFile() == "src/ui/view.go" }},
		"f shows the full file":        {key: "f", want: func(m tui.Model) bool { return m.FullFile() }},
		"c asks for a selection":       {key: "c", want: func(m tui.Model) bool { return strings.Contains(view(m), "select hunks first") }},
		"d shows the removed lines":    {key: "d", want: func(m tui.Model) bool { return strings.Contains(view(m), "- old line") }},
		"z folds the hunk":             {key: "z", want: func(m tui.Model) bool { return strings.Contains(view(m), "▸ ○ @@ -3,2 +3,2 @@") }},
		"? opens the help":             {key: "?", want: func(m tui.Model) bool { return strings.Contains(view(m), "a range of lines") }},
	}
	for name, tt := range tests {
		t.Run(name+" and closes the range", func(t *testing.T) {
			m := press(t, onMain(t, fullSource()), "v", tt.key)

			if !tt.want(m) {
				t.Errorf("%s did not act after closing the range:\n%s", tt.key, view(m))
			}
			assertMissing(t, view(m), "select range", "selecting a range")
			assertContains(t, view(press(t, m, "esc")), "nothing selected")
		})
	}
}

func TestLineRange_failed(t *testing.T) {
	tests := map[string]struct {
		keys []string
		want string
	}{
		"the file pane": {keys: []string{"tab", "v"}, want: "v works in the diff pane; press tab"},
		"a whole file":  {keys: []string{"J", "J", "v"}, want: "this file can only be selected whole; press S"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			m := press(t, onMain(t, newSource()), tt.keys...)

			assertContains(t, view(m), tt.want, "nothing selected")
			assertMissing(t, view(m), "select range")
		})
	}

	t.Run("an immutable revision", func(t *testing.T) {
		src := newSource()
		src.change.Immutable = true

		m := press(t, onMain(t, src), "v")

		assertContains(t, view(m), "immutable revision: read only")
		assertMissing(t, view(m), "select range")
	})

	t.Run("a range without open changed lines", func(t *testing.T) {
		m := press(t, onMain(t, newSource()), "k", "v", "s")

		assertContains(t, view(m), "no changed lines to select in the range", "nothing selected")
		assertMissing(t, view(m), "select range")
	})
}
