package tui_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/madalinpopa/jjreview/internal/jj"
)

func squashSource() *fakeSource {
	src := revisionSource()
	src.log[0].Entry.Parents = []string{"parentcommit"}
	src.log[1].Entry.Parents = []string{"parentcommit"}
	src.log[3].Entry.Parents = []string{"rootcommit"}
	return src
}

func assertSquashed(t *testing.T, src *fakeSource, from []string, into string) {
	t.Helper()
	if len(src.squashes) != 1 || !slices.Equal(src.squashes[0].from, from) || src.squashes[0].into != into {
		t.Errorf("Squash() calls = %+v, want one call squashing %v into %s", src.squashes, from, into)
	}
}

func TestLogSquash_success(t *testing.T) {
	t.Run("s marks the sources and the default destination", func(t *testing.T) {
		m := press(t, loaded(t, squashSource()), "2", "s")

		got := view(m)
		assertContains(t, got, "@   squash  kxyzabcd wip", "○   into  pqrspare parent")
		assertContains(t, got, "j k into · y squash · esc cancel")
	})

	t.Run("y squashes the cursor revision into its parent", func(t *testing.T) {
		src := squashSource()

		m := press(t, loaded(t, src), "2", "s", "y")

		assertSquashed(t, src, []string{"commit1"}, "parentcommit")
		assertContains(t, view(m), "squashed kxyzabcd into pqrspare")
		if got := src.changeRevs[len(src.changeRevs)-1]; got != "@" {
			t.Errorf("Change() last rev = %q, want %q", got, "@")
		}
	})

	t.Run("space selects several revisions to squash together", func(t *testing.T) {
		src := squashSource()

		m := press(t, loaded(t, src), "2", "space", "j", "space")
		assertContains(t, view(m), "● kxyzabcd", "● wxyzchil", "2 selected")
		m = press(t, m, "s", "y")

		assertSquashed(t, src, []string{"commit1", "childcommit"}, "parentcommit")
		got := view(m)
		assertContains(t, got, "squashed 2 revisions into pqrspare")
		assertMissing(t, got, "●", "selected")
	})

	t.Run("the destination follows the cursor", func(t *testing.T) {
		src := squashSource()

		press(t, loaded(t, src), "2", "j", "s", "g", "y")

		assertSquashed(t, src, []string{"childcommit"}, "commit1")
	})

	t.Run("a conflict warning follows the squash status", func(t *testing.T) {
		src := squashSource()
		src.squashWarnings = "New conflicts appeared in 1 commits:\n  pqrspare 1fd99db8 (conflict) parent"

		m := press(t, loaded(t, src), "2", "s", "y")

		got := view(m)
		assertContains(t, got, "squashed kxyzabcd into pqrspare · New conflicts appeared in 1 commits:")
		assertMissing(t, got, "1fd99db8")
	})

	t.Run("squashing the working copy into the reviewed revision keeps the review", func(t *testing.T) {
		src := squashSource()
		m := press(t, loaded(t, src), reviewParent()...)

		m = press(t, m, "2", "g", "s", "y")

		assertSquashed(t, src, []string{"commit1"}, "parentcommit")
		if got, want := src.changeRevs[len(src.changeRevs)-1], jj.VisibleCommitRevset("parentcommit"); got != want {
			t.Errorf("Change() last rev = %q, want %q", got, want)
		}
		assertContains(t, view(press(t, m, "1")), "@ pqrspare")
	})

	t.Run("squashing the reviewed revision reviews the working copy", func(t *testing.T) {
		src := squashSource()
		m := press(t, loaded(t, src), "2", "j", "enter")

		press(t, m, "2", "s", "y")

		assertSquashed(t, src, []string{"childcommit"}, "parentcommit")
		if got := src.changeRevs[len(src.changeRevs)-1]; got != "@" {
			t.Errorf("Change() last rev = %q, want %q", got, "@")
		}
	})

	t.Run("the cursor lands on the destination after the log reloads", func(t *testing.T) {
		src := squashSource()
		m := press(t, loaded(t, src), "2", "s")
		src.log = withoutEntry(logLines(), "commit1")
		src.log[2].Entry.CommitID = "parentcommit2"

		m = press(t, m, "y")

		if cursor := m.LogCursor(); cursor != "pqrsparent00" {
			t.Errorf("LogCursor() = %q, want %q", cursor, "pqrsparent00")
		}
	})

	t.Run("a failed squash keeps the cursor row", func(t *testing.T) {
		src := squashSource()
		src.squashErr = errors.New("jj squash: exit status 1")
		m := press(t, loaded(t, src), "2", "s")
		src.log = withoutEntry(logLines(), "commit1")

		m = press(t, m, "y")

		if cursor := m.LogCursor(); cursor != "zzzzroot0000" {
			t.Errorf("LogCursor() = %q, want %q", cursor, "zzzzroot0000")
		}
	})

	t.Run("y twice drops the selected hunks", func(t *testing.T) {
		src := squashSource()
		m := press(t, loaded(t, src), "space", "2", "s", "y")
		assertContains(t, view(m), "press y again to drop 1 selected hunk")

		press(t, m, "y")

		assertSquashed(t, src, []string{"commit1"}, "parentcommit")
	})
}

func TestLogSquash_cancelled(t *testing.T) {
	t.Run("esc leaves the squash without squashing", func(t *testing.T) {
		src := squashSource()

		m := press(t, loaded(t, src), "2", "s", "esc", "y")

		if len(src.squashes) != 0 {
			t.Errorf("Squash() calls = %+v, want none", src.squashes)
		}
		assertMissing(t, view(m), " into ")
	})

	t.Run("tab keys do not leave the squash", func(t *testing.T) {
		m := press(t, loaded(t, squashSource()), "2", "s", "1")

		assertLogTab(t, m)
	})
}

func TestLogSquash_failed(t *testing.T) {
	t.Run("an immutable revision is not squashed", func(t *testing.T) {
		src := squashSource()

		m := press(t, loaded(t, src), "2", "G", "s")

		if len(src.squashes) != 0 {
			t.Errorf("Squash() calls = %+v, want none", src.squashes)
		}
		assertContains(t, view(m), "zzzzroot is immutable")
	})

	t.Run("a source cannot be the destination", func(t *testing.T) {
		src := squashSource()

		m := press(t, loaded(t, src), "2", "s", "g", "y")

		if len(src.squashes) != 0 {
			t.Errorf("Squash() calls = %+v, want none", src.squashes)
		}
		assertContains(t, view(m), "pick a revision that is not being squashed")
	})

	t.Run("a jj failure is reported", func(t *testing.T) {
		src := squashSource()
		src.squashErr = errors.New("jj squash: exit status 1: Error: Commit pqrspare is immutable\nHint: details")

		m := press(t, loaded(t, src), "2", "s", "y")

		got := view(m)
		assertContains(t, got, "squash into pqrspare: jj squash: exit status 1: Error: Commit pqrspare is immutable")
		assertMissing(t, got, "Hint: details")
	})
}
