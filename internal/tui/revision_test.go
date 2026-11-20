package tui_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/madalinpopa/jjreview/internal/diff"
	"github.com/madalinpopa/jjreview/internal/jj"
)

func revisionSource() *fakeSource {
	src := newSource()
	src.revisions = map[string]jj.Change{
		"kxyzabcdefgh-": {ChangeID: "pqrsparent00", CommitID: "parentcommit", Description: "parent"},
		"kxyzabcdefgh+": {ChangeID: "wxyzchild000", CommitID: "childcommit", Description: "child"},
		"pqrsparent00+": src.change,
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
		"ctrl+p loads the parent revision":         {keys: []string{"ctrl+p"}, wantRev: "kxyzabcdefgh-", wantID: "@ pqrspare", wantFile: "README.md"},
		"ctrl+n loads the child revision":          {keys: []string{"ctrl+n"}, wantRev: "kxyzabcdefgh+", wantID: "@ wxyzchil", wantFile: "src/main.go"},
		"ctrl+n moves on from the loaded revision": {keys: []string{"ctrl+p", "ctrl+n"}, wantRev: "pqrsparent00+", wantID: "@ kxyzabcd", wantFile: "src/ui/view.go"},
		"@ goes back to the working copy":          {keys: []string{"ctrl+p", "@"}, wantRev: "@", wantID: "@ kxyzabcd", wantFile: "src/ui/view.go"},
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
		m := press(t, loaded(t, revisionSource()), "h", "j", "l", "space", "ctrl+n")

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
		"no parent":        {key: "ctrl+p", rev: "kxyzabcdefgh-", err: jj.ErrNoRevision, want: "no parent revision"},
		"no child":         {key: "ctrl+n", rev: "kxyzabcdefgh+", err: jj.ErrNoRevision, want: "no child revision"},
		"several parents":  {key: "ctrl+p", rev: "kxyzabcdefgh-", err: jj.ErrMultipleRevisions, want: "multiple parent revisions"},
		"several children": {key: "ctrl+n", rev: "kxyzabcdefgh+", err: jj.ErrMultipleRevisions, want: "multiple child revisions"},
		"jj fails":         {key: "ctrl+p", rev: "kxyzabcdefgh-", err: errors.New("boom"), want: "boom"},
		"no working copy":  {key: "@", rev: "@", err: jj.ErrNoRevision, want: "no working copy revision"},
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

func TestRevisionNavigation_afterCommit(t *testing.T) {
	tests := map[string]struct {
		key     string
		wantRev string
	}{
		"ctrl+p starts from the revision left after a commit": {key: "ctrl+p", wantRev: "rest1zzzzzzzzzz-"},
		"ctrl+n starts from the revision left after a commit": {key: "ctrl+n", wantRev: "rest1zzzzzzzzzz+"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			src := revisionSource()
			m := press(t, loaded(t, src), "h", "j", "l", "space", "c", "ctrl+s")

			press(t, m, tt.key)

			if got := src.changeRevs[len(src.changeRevs)-1]; got != tt.wantRev {
				t.Errorf("Change() last rev = %q, want %q", got, tt.wantRev)
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
	for _, key := range []string{"ctrl+p", "ctrl+n", "@", "R"} {
		t.Run(key+" is blocked after an incomplete split", func(t *testing.T) {
			src := revisionSource()
			src.splitErr = fmt.Errorf("split: %w: reload failed", jj.ErrSplitIncomplete)
			m := press(t, loaded(t, src), "h", "j", "l", "space", "c", "ctrl+s")
			calls := len(src.changeRevs)

			m = press(t, m, key)

			if got := len(src.changeRevs); got != calls {
				t.Errorf("Change() calls = %d, want %d", got, calls)
			}
			assertContains(t, view(m), "split done; restart jjr to continue")
		})
	}
}
