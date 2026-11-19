package diff_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/madalinpopa/jjreview/internal/diff"
)

func readApplyFixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "apply", name))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return string(data)
}

func fixtureFile(t *testing.T, fixture, path string) diff.File {
	t.Helper()
	for _, f := range parseFixture(t, fixture) {
		if f.Path() == path {
			return f
		}
	}
	t.Fatalf("file %q not in %s", path, fixture)
	return diff.File{}
}

func allHunks(f diff.File) []int {
	indexes := make([]int, len(f.Hunks))
	for i := range indexes {
		indexes[i] = i
	}
	return indexes
}

func TestApply_allHunksReproduceChild(t *testing.T) {
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
		"modified":                  {path: "mod.txt", oldName: "mod.txt.old", newName: "mod.txt.new"},
		"added":                     {path: "added.txt", oldName: "added.txt.old", newName: "added.txt.new"},
		"copied":                    {path: "copied.txt", oldName: "copied.txt.old", newName: "copied.txt.new"},
		"deleted":                   {path: "del.txt", oldName: "del.txt.old", newName: "del.txt.new"},
		"emptied":                   {path: "emptied.txt", oldName: "emptied.txt.old", newName: "emptied.txt.new"},
		"no newline":                {path: "nonl.txt", oldName: "nonl.txt.old", newName: "nonl.txt.new"},
		"renamed":                   {path: "renamed.txt", oldName: "ren.txt.old", newName: "renamed.txt.new"},
		"space in path":             {path: "sp ace.txt", oldName: "sp ace.txt.old", newName: "sp ace.txt.new"},
		"unicode":                   {path: "ünï.txt", oldName: "ünï.txt.old", newName: "ünï.txt.new"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			fixture := tt.fixture
			if fixture == "" {
				fixture = "jj.diff"
			}
			f := fixtureFile(t, fixture, tt.path)

			got, err := diff.Apply(readApplyFixture(t, tt.oldName), f, allHunks(f))
			if err != nil {
				t.Fatalf("Apply() error = %v", err)
			}

			if want := readApplyFixture(t, tt.newName); got != want {
				t.Errorf("Apply() = %q, want %q", got, want)
			}
		})
	}
}

func TestApply_selectedHunks(t *testing.T) {
	f := fixtureFile(t, "jj.diff", "mod.txt")
	oldText := readApplyFixture(t, "mod.txt.old")
	tests := map[string]struct {
		hunks []int
		want  string
	}{
		"none":        {hunks: nil, want: oldText},
		"first only":  {hunks: []int{0}, want: readApplyFixture(t, "mod.txt.hunk0")},
		"second only": {hunks: []int{1}, want: readApplyFixture(t, "mod.txt.hunk1")},
		"empty slice": {hunks: []int{}, want: oldText},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := diff.Apply(oldText, f, tt.hunks)
			if err != nil {
				t.Fatalf("Apply() error = %v", err)
			}

			if got != tt.want {
				t.Errorf("Apply() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestApply_newlineMismatch(t *testing.T) {
	tests := map[string]struct {
		path    string
		oldText string
	}{
		"hunk expects missing newline":    {path: "addnl.txt", oldText: "a\nb\nc\n"},
		"hunk expects trailing newline":   {path: "rmnl.txt", oldText: "a\nb\nc"},
		"context expects missing newline": {path: "ctxnl.txt", oldText: "a\nb\nc\n"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			f := fixtureFile(t, "jj_newline.diff", tt.path)

			got, err := diff.Apply(tt.oldText, f, allHunks(f))

			if err == nil {
				t.Errorf("Apply() = %q, want error", got)
			}
		})
	}
}

func TestApply_errors(t *testing.T) {
	f := fixtureFile(t, "jj.diff", "mod.txt")
	oldText := readApplyFixture(t, "mod.txt.old")
	tests := map[string]struct {
		oldText string
		hunks   []int
	}{
		"descending indexes":    {oldText: oldText, hunks: []int{1, 0}},
		"duplicate indexes":     {oldText: oldText, hunks: []int{0, 0}},
		"index out of range":    {oldText: oldText, hunks: []int{2}},
		"negative index":        {oldText: oldText, hunks: []int{-1}},
		"context mismatch":      {oldText: strings.Replace(oldText, "1\n", "one\n", 1), hunks: []int{0}},
		"removed line mismatch": {oldText: strings.Replace(oldText, "25\n", "x\n", 1), hunks: []int{1}},
		"old text too short":    {oldText: "1\n2\n", hunks: []int{1}},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := diff.Apply(tt.oldText, f, tt.hunks)

			if err == nil {
				t.Errorf("Apply() = %q, want error", got)
			}
		})
	}
}
