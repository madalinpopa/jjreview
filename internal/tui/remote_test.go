package tui_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/madalinpopa/jjreview/internal/diff"
	"github.com/madalinpopa/jjreview/internal/jj"
)

func assertPushed(t *testing.T, src *fakeSource, commitID string) {
	t.Helper()
	if len(src.pushes) != 1 || src.pushes[0].CommitID != commitID {
		t.Errorf("Push() calls = %+v, want one push of %s", src.pushes, commitID)
	}
}

func TestLogFetch_success(t *testing.T) {
	t.Run("f fetches and refreshes the log", func(t *testing.T) {
		src := revisionSource()
		src.fetchChanged = true
		m := press(t, loaded(t, src), "2")
		calls := src.logCalls

		m = press(t, m, "f")

		if src.fetches != 1 || src.logCalls <= calls {
			t.Errorf("Fetch() calls = %d, Log() calls = %d after %d, want one fetch and a refreshed log", src.fetches, src.logCalls, calls)
		}
		assertContains(t, view(m), "fetched")
	})

	t.Run("an up to date remote is reported", func(t *testing.T) {
		m := press(t, loaded(t, revisionSource()), "2", "f")

		assertContains(t, view(m), "already up to date")
	})

	t.Run("the status says the fetch is running", func(t *testing.T) {
		m := press(t, loaded(t, revisionSource()), "2")

		m, _ = update(t, m, keyMsg("f"))

		assertContains(t, view(m), "Fetch", "jj git fetch")
	})

	t.Run("selected hunks are kept", func(t *testing.T) {
		src := revisionSource()
		src.fetchChanged = true

		m := press(t, loaded(t, src), "space", "2", "f", "1")

		assertContains(t, view(m), "1 hunk selected")
	})

	t.Run("selected hunks are dropped when the reviewed diff changed", func(t *testing.T) {
		src := revisionSource()
		m := press(t, loaded(t, src), "space", "2")
		rebased := jj.Change{ChangeID: "kxyzabcdefgh", CommitID: "commit2", Parents: []string{"parent2"}, WorkingCopy: true}
		src.revErrs = map[string]error{jj.VisibleCommitRevset("commit1"): noRevision()}
		src.successors = map[string]jj.Change{"commit1": rebased}
		src.revisions[jj.VisibleCommitRevset("commit2")] = rebased
		src.revFiles = map[string][]diff.File{"commit2": {readmeFile()}}
		src.head = "commit2"
		src.fetchChanged = true

		m = press(t, m, "f", "1")

		assertContains(t, view(m), "nothing selected")
	})
}

func TestLogFetch_failed(t *testing.T) {
	t.Run("a jj failure is reported", func(t *testing.T) {
		src := revisionSource()
		src.fetchErr = errors.New("jj git fetch: exit status 1: Error: No git remote named 'origin'\nHint: details")

		m := press(t, loaded(t, src), "2", "f")

		assertContains(t, toastText(t, m), "fetch: jj git fetch: exit status 1: Error: No git remote named 'origin'")
		assertMissing(t, view(m), "Hint: details")
	})
}

func TestLogPush_success(t *testing.T) {
	t.Run("P asks before pushing", func(t *testing.T) {
		src := revisionSource()

		m := press(t, loaded(t, src), "2", "P")

		if len(src.pushes) != 0 {
			t.Errorf("Push() calls = %+v, want none before y", src.pushes)
		}
		assertContains(t, view(m), "Push kxyzabcd", "Creates a bookmark for the change and pushes it", "y push · any other key cancels")
	})

	t.Run("y pushes the revision under the cursor", func(t *testing.T) {
		src := revisionSource()
		src.pushResult = []jj.PushResult{{Remote: "origin", Bookmarks: []string{"push-kxyzabcd"}}}

		m := press(t, loaded(t, src), "2", "P", "y")

		assertPushed(t, src, "commit1")
		assertContains(t, view(m), "pushed push-kxyzabcd to origin")
	})

	t.Run("each remote is reported with its bookmarks", func(t *testing.T) {
		src := revisionSource()
		src.pushResult = []jj.PushResult{{Remote: "mirror", Bookmarks: []string{"feature"}}, {Remote: "origin", Bookmarks: []string{"feature", "main"}}}

		m := press(t, loaded(t, src), "2", "P", "y")

		assertContains(t, toastText(t, m), "pushed feature to mirror and feature, main to origin")
	})

	t.Run("the prompt lists the bookmarks that are pushed", func(t *testing.T) {
		src := revisionSource()
		src.log[0].Entry.Bookmarks = []string{"main", "feat"}

		m := press(t, loaded(t, src), "2", "P")

		assertContains(t, view(m), "Pushes bookmarks main, feat")
	})

	t.Run("an up to date remote is reported", func(t *testing.T) {
		m := press(t, loaded(t, revisionSource()), "2", "P", "y")

		assertContains(t, view(m), "nothing to push")
	})

	t.Run("selected hunks are kept", func(t *testing.T) {
		src := revisionSource()

		m := press(t, loaded(t, src), "space", "2", "P", "y")

		assertPushed(t, src, "commit1")
		assertMissing(t, view(m), "press y again")
		assertContains(t, view(press(t, m, "1")), "1 hunk selected")
	})

	t.Run("selected hunks survive a push that re-signs the revision", func(t *testing.T) {
		src := revisionSource()
		m := press(t, loaded(t, src), "space", "2", "P")
		signed := jj.Change{ChangeID: "kxyzabcdefgh", CommitID: "commit2", Parents: []string{"parent1"}, WorkingCopy: true}
		src.revErrs = map[string]error{jj.VisibleCommitRevset("commit1"): noRevision()}
		src.successors = map[string]jj.Change{"commit1": signed}
		src.revisions[jj.VisibleCommitRevset("commit2")] = signed
		src.head = "commit2"

		m = press(t, m, "y", "1")

		if got := src.diffRevs[len(src.diffRevs)-1]; got != "commit2" {
			t.Errorf("Diff() last rev = %q, want %q", got, "commit2")
		}
		assertContains(t, view(m), "1 hunk selected")
	})
}

func TestLogPush_cancelled(t *testing.T) {
	t.Run("any other key closes the prompt", func(t *testing.T) {
		src := revisionSource()

		m := press(t, loaded(t, src), "2", "P", "n", "y")

		if len(src.pushes) != 0 {
			t.Errorf("Push() calls = %+v, want none", src.pushes)
		}
		assertMissing(t, view(m), "any other key cancels")
	})
}

func TestLogPush_failed(t *testing.T) {
	t.Run("a jj failure is reported", func(t *testing.T) {
		src := revisionSource()
		src.pushErr = errors.New("jj git push: exit status 1: Error: Won't push commit 1234 since it has no description\nHint: details")

		m := press(t, loaded(t, src), "2", "P", "y")

		assertContains(t, toastText(t, m), "push kxyzabcd: jj git push: exit status 1: Error: Won't push commit 1234 since it has no description")
		assertMissing(t, view(m), "Hint: details")
	})

	t.Run("bookmarks changed since the log was listed", func(t *testing.T) {
		src := revisionSource()
		src.pushErr = fmt.Errorf("push commit1: %w", jj.ErrBookmarksChanged)

		m := press(t, loaded(t, src), "2", "P", "y")

		assertContains(t, toastText(t, m), "bookmarks changed; press P to review the push again")
	})
}

func TestLogReset_success(t *testing.T) {
	t.Run("R clears the selection without refreshing", func(t *testing.T) {
		src := revisionSource()
		m := press(t, loaded(t, src), "2", "space")
		calls := src.logCalls

		got := view(press(t, m, "R"))

		assertMissing(t, got, "●", "selected")
		if src.logCalls != calls {
			t.Errorf("Log() calls = %d, want %d", src.logCalls, calls)
		}
	})

	t.Run("ctrl+r refreshes the log", func(t *testing.T) {
		src := revisionSource()
		m := press(t, loaded(t, src), "2")
		calls := src.logCalls

		press(t, m, "ctrl+r")

		if src.logCalls <= calls {
			t.Errorf("Log() calls = %d, want more than %d", src.logCalls, calls)
		}
	})
}
