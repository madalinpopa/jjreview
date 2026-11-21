package tui_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/madalinpopa/jjreview/internal/diff"
	"github.com/madalinpopa/jjreview/internal/jj"
)

func reviewParent() []string {
	return []string{"2", "j", "j", "enter"}
}

func revisionSource() *fakeSource {
	src := newSource()
	src.log = logLines()
	src.revisions = map[string]jj.Change{
		jj.CommitRevset("childcommit"):  {ChangeID: "wxyzchild000", CommitID: "childcommit", Parents: []string{"parentcommit"}, Description: "child feature"},
		jj.CommitRevset("parentcommit"): {ChangeID: "pqrsparent00", CommitID: "parentcommit", Parents: []string{"rootcommit"}, Description: "parent"},
	}
	src.revFiles = map[string][]diff.File{
		"parentcommit": {readmeFile()},
		"childcommit":  {mainFile()},
	}
	return src
}

func TestRevisionNavigation_success(t *testing.T) {
	tests := map[string]struct {
		keys     []string
		wantRev  string
		wantID   string
		wantFile string
	}{
		"enter in the log loads the picked revision": {keys: reviewParent(), wantRev: jj.CommitRevset("parentcommit"), wantID: "@ pqrspare", wantFile: "README.md"},
		"@ goes back to the working copy":            {keys: append(reviewParent(), "@"), wantRev: "@", wantID: "@ kxyzabcd", wantFile: "src/ui/view.go"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			src := revisionSource()

			m := press(t, loaded(t, src), tt.keys...)

			if got := src.changeRevs[len(src.changeRevs)-1]; got != tt.wantRev {
				t.Errorf("Change() last rev = %q, want %q", got, tt.wantRev)
			}
			if got := m.CurrentFile(); got != tt.wantFile {
				t.Errorf("CurrentFile() = %q, want %q", got, tt.wantFile)
			}
			assertContains(t, view(m), tt.wantID)
		})
	}

	t.Run("navigation clears the selection of the previous revision", func(t *testing.T) {
		m := press(t, loaded(t, revisionSource()), reviewParent()...)
		m = press(t, m, "space", "@", "@")

		assertContains(t, view(m), "nothing selected")
	})
}

func TestRevisionNavigation_failure(t *testing.T) {
	tests := map[string]struct {
		key  string
		rev  string
		err  error
		want string
	}{
		"jj fails":               {key: "@", rev: "@", err: errors.New("boom"), want: "boom"},
		"several working copies": {key: "@", rev: "@", err: jj.ErrMultipleRevisions, want: "multiple working copy revisions"},
		"no working copy":        {key: "@", rev: "@", err: jj.ErrNoRevision, want: "no working copy revision"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			src := revisionSource()
			m := loaded(t, src)
			src.revErrs = map[string]error{tt.rev: fmt.Errorf("jj log: revision %q: %w", tt.rev, tt.err)}

			m = press(t, m, tt.key)

			got := view(m)
			assertContains(t, got, tt.want, "@ kxyzabcd")
			if file := m.CurrentFile(); file != "src/ui/view.go" {
				t.Errorf("CurrentFile() = %q, want the current revision kept", file)
			}
		})
	}
}

func TestRevisionNavigation_afterFailedLoad(t *testing.T) {
	src := revisionSource()
	src.err = errors.New("revision not found")
	m := loaded(t, src)
	src.err = nil

	m = press(t, m, "@")

	if got := src.changeRevs[len(src.changeRevs)-1]; got != "@" {
		t.Errorf("Change() last rev = %q, want %q", got, "@")
	}
	assertMissing(t, view(m), "revision not found")
}

func TestRevisionNavigation_blocked(t *testing.T) {
	for _, key := range []string{"@", "R"} {
		t.Run(key+" is blocked after an incomplete split", func(t *testing.T) {
			src := revisionSource()
			src.splitErr = fmt.Errorf("split: %w: reload failed", jj.ErrSplitIncomplete)
			m := press(t, loaded(t, src), "tab", "j", "tab", "space", "c", "ctrl+s")
			calls := len(src.changeRevs)

			m = press(t, m, key)

			if got := len(src.changeRevs); got != calls {
				t.Errorf("Change() calls = %d, want %d", got, calls)
			}
			assertContains(t, view(m), "split done; restart jjr to continue")
		})
	}
}
