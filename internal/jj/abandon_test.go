package jj_test

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/madalinpopa/jjreview/internal/jj"
)

func abandonRepo(t *testing.T) string {
	t.Helper()
	dir := newRepo(t)
	writeFile(t, dir, "a.txt", "a\n")
	commit(t, dir, "parent")
	writeFile(t, dir, "b.txt", "b\n")
	commit(t, dir, "child")
	writeFile(t, dir, "c.txt", "c\n")
	runJJ(t, dir, "describe", "-m", "work")
	return dir
}

func loadChange(t *testing.T, client jj.Client, rev string) jj.Change {
	t.Helper()
	change, err := client.Change(t.Context(), rev)
	if err != nil {
		t.Fatalf("Change(%q) error = %v", rev, err)
	}
	return change
}

func TestAbandon_success(t *testing.T) {
	t.Run("removes the revision", func(t *testing.T) {
		dir := abandonRepo(t)
		client := newClient(t, dir)
		child := loadChange(t, client, "@-")

		_, err := client.Abandon(t.Context(), child.CommitID, nil)

		if err != nil {
			t.Fatalf("Abandon(%q) error = %v", child.CommitID, err)
		}
		_, err = client.Change(t.Context(), jj.ChangeRevset(child.ChangeID))
		if !errors.Is(err, jj.ErrNoRevision) {
			t.Errorf("Change(ChangeRevset(%q)) error = %v, want %v", child.ChangeID, err, jj.ErrNoRevision)
		}
	})

	t.Run("rebases the descendants onto the parent", func(t *testing.T) {
		dir := abandonRepo(t)
		client := newClient(t, dir)
		parent := loadChange(t, client, "@--")
		child := loadChange(t, client, "@-")
		work := loadChange(t, client, "@")

		_, err := client.Abandon(t.Context(), child.CommitID, nil)

		if err != nil {
			t.Fatalf("Abandon(%q) error = %v", child.CommitID, err)
		}
		got := loadChange(t, client, jj.ChangeRevset(work.ChangeID))
		if !slices.Equal(got.Parents, []string{parent.CommitID}) {
			t.Errorf("Parents = %v, want [%s]", got.Parents, parent.CommitID)
		}
		if got.CommitID == work.CommitID {
			t.Errorf("CommitID = %q, want a rebased commit", got.CommitID)
		}
		if paths := diffPaths(t, client, got.CommitID); !slices.Equal(paths, []string{"c.txt"}) {
			t.Errorf("diff paths = %v, want [c.txt]", paths)
		}
	})

	t.Run("gives the working copy a new empty revision", func(t *testing.T) {
		dir := abandonRepo(t)
		client := newClient(t, dir)
		work := loadChange(t, client, "@")

		_, err := client.Abandon(t.Context(), work.CommitID, nil)

		if err != nil {
			t.Fatalf("Abandon(%q) error = %v", work.CommitID, err)
		}
		got := loadChange(t, client, "@")
		if got.ChangeID == work.ChangeID {
			t.Errorf("Change(@).ChangeID = %q, want a new revision", got.ChangeID)
		}
		if paths := diffPaths(t, client, got.CommitID); len(paths) != 0 {
			t.Errorf("diff paths = %v, want none", paths)
		}
	})
}

func TestAbandon_failed(t *testing.T) {
	t.Run("an immutable revision", func(t *testing.T) {
		dir := abandonRepo(t)
		client := newClient(t, dir)
		root := loadChange(t, client, "root()")

		_, err := client.Abandon(t.Context(), root.CommitID, nil)

		if err == nil || !strings.Contains(err.Error(), "immutable") {
			t.Errorf("Abandon(root) error = %v, want it to mention immutable", err)
		}
	})

	t.Run("a commit rewritten since it was listed", func(t *testing.T) {
		dir := abandonRepo(t)
		client := newClient(t, dir)
		child := loadChange(t, client, "@-")
		runJJ(t, dir, "describe", "-r", "@-", "-m", "child again")

		_, err := client.Abandon(t.Context(), child.CommitID, nil)

		if !errors.Is(err, jj.ErrHiddenRevision) {
			t.Errorf("Abandon(%q) error = %v, want %v", child.CommitID, err, jj.ErrHiddenRevision)
		}
		if got := loadChange(t, client, jj.ChangeRevset(child.ChangeID)); got.Description != "child again\n" {
			t.Errorf("Description = %q, want the rewritten revision kept", got.Description)
		}
	})

	t.Run("an unknown commit", func(t *testing.T) {
		dir := abandonRepo(t)
		client := newClient(t, dir)

		_, err := client.Abandon(t.Context(), "0123456789abcdef0123456789abcdef01234567", nil)

		if err == nil {
			t.Fatal("Abandon(unknown) error = nil, want error")
		}
		if got := loadChange(t, client, "@"); got.Description != "work\n" {
			t.Errorf("Change(@).Description = %q, want the working copy kept", got.Description)
		}
	})
}

func TestAbandon_successMerge(t *testing.T) {
	dir := newRepo(t)
	writeFile(t, dir, "a.txt", "a\n")
	runJJ(t, dir, "describe", "-m", "left")
	left := changeID(t, dir, "@")
	runJJ(t, dir, "new", "root()", "-m", "right")
	writeFile(t, dir, "b.txt", "b\n")
	runJJ(t, dir, "new", left, "@", "-m", "merge")
	writeFile(t, dir, "m.txt", "m\n")
	runJJ(t, dir, "new", "-m", "child")
	writeFile(t, dir, "c.txt", "c\n")
	client := newClient(t, dir)
	merge := loadChange(t, client, "@-")
	child := loadChange(t, client, "@")

	_, err := client.Abandon(t.Context(), merge.CommitID, nil)

	if err != nil {
		t.Fatalf("Abandon(%q) error = %v", merge.CommitID, err)
	}
	got := loadChange(t, client, jj.ChangeRevset(child.ChangeID))
	if !slices.Equal(got.Parents, merge.Parents) {
		t.Errorf("Parents = %v, want %v", got.Parents, merge.Parents)
	}
	if paths := diffPaths(t, client, got.CommitID); !slices.Equal(paths, []string{"c.txt"}) {
		t.Errorf("diff paths = %v, want [c.txt]", paths)
	}
}

func TestAbandon_successBookmark(t *testing.T) {
	dir := abandonRepo(t)
	runJJ(t, dir, "bookmark", "create", "feature", "-r", "@-")
	client := newClient(t, dir)
	child := loadChange(t, client, "@-")

	_, err := client.Abandon(t.Context(), child.CommitID, []string{"feature"})

	if err != nil {
		t.Fatalf("Abandon(%q) error = %v", child.CommitID, err)
	}
	if got := runJJ(t, dir, "bookmark", "list"); strings.Contains(got, "feature") {
		t.Errorf("bookmark list = %q, want feature deleted", got)
	}
	if got := runJJ(t, dir, "log", "--no-graph", "-r", "@-", "-T", "description"); got != "parent\n" {
		t.Errorf("@- description = %q, want %q", got, "parent\n")
	}
}

func TestAbandon_failedRewrittenBeforeAbandon(t *testing.T) {
	dir := abandonRepo(t)
	client := newClient(t, dir)
	child := loadChange(t, client, "@-")
	beforeCommand(t, "abandon", `"$`+realJJEnv+`" describe -r 'change_id("`+child.ChangeID+`")' -m rewritten >/dev/null 2>&1`)

	_, err := client.Abandon(t.Context(), child.CommitID, nil)

	if err == nil {
		t.Fatalf("Abandon(%q) error = nil, want error", child.CommitID)
	}
	got := loadChange(t, client, jj.ChangeRevset(child.ChangeID))
	if got.Description != "rewritten\n" {
		t.Errorf("Description = %q, want the rewritten revision kept", got.Description)
	}
	work := loadChange(t, client, "@")
	if !slices.Equal(work.Parents, []string{got.CommitID}) {
		t.Errorf("Change(@).Parents = %v, want [%s]", work.Parents, got.CommitID)
	}
}

func TestVisibleCommitRevset_success(t *testing.T) {
	dir := abandonRepo(t)
	client := newClient(t, dir)
	child := loadChange(t, client, "@-")

	got := loadChange(t, client, jj.VisibleCommitRevset(child.CommitID))

	if got.CommitID != child.CommitID {
		t.Errorf("CommitID = %q, want %q", got.CommitID, child.CommitID)
	}
}

func TestVisibleCommitRevset_failed(t *testing.T) {
	dir := abandonRepo(t)
	client := newClient(t, dir)
	child := loadChange(t, client, "@-")
	runJJ(t, dir, "describe", "-r", "@-", "-m", "child again")

	_, err := client.Change(t.Context(), jj.VisibleCommitRevset(child.CommitID))

	if !errors.Is(err, jj.ErrNoRevision) {
		t.Errorf("Change(VisibleCommitRevset(%q)) error = %v, want %v", child.CommitID, err, jj.ErrNoRevision)
	}
}

func TestAbandon_successReportsWorkingCopy(t *testing.T) {
	tests := map[string]struct {
		rev  string
		want bool
	}{
		"the working copy": {rev: "@", want: true},
		"another revision": {rev: "@-", want: false},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			dir := abandonRepo(t)
			client := newClient(t, dir)
			target := loadChange(t, client, tt.rev)

			got, err := client.Abandon(t.Context(), target.CommitID, nil)

			if err != nil {
				t.Fatalf("Abandon(%q) error = %v", target.CommitID, err)
			}
			if got != tt.want {
				t.Errorf("Abandon(%q) working copy = %t, want %t", target.CommitID, got, tt.want)
			}
		})
	}
}

func TestAbandon_failedBookmarksChanged(t *testing.T) {
	tests := map[string]struct {
		existing  []string
		confirmed []string
	}{
		"a bookmark was added":   {existing: []string{"feature"}, confirmed: nil},
		"a bookmark was removed": {existing: nil, confirmed: []string{"feature"}},
		"a bookmark was renamed": {existing: []string{"other"}, confirmed: []string{"feature"}},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			dir := abandonRepo(t)
			for _, b := range tt.existing {
				runJJ(t, dir, "bookmark", "create", b, "-r", "@-")
			}
			client := newClient(t, dir)
			child := loadChange(t, client, "@-")

			_, err := client.Abandon(t.Context(), child.CommitID, tt.confirmed)

			if !errors.Is(err, jj.ErrBookmarksChanged) {
				t.Errorf("Abandon(%q) error = %v, want %v", child.CommitID, err, jj.ErrBookmarksChanged)
			}
			if got := loadChange(t, client, jj.VisibleCommitRevset(child.CommitID)); got.CommitID != child.CommitID {
				t.Errorf("CommitID = %q, want the revision kept", got.CommitID)
			}
		})
	}
}

func TestAbandon_successBookmarksInAnyOrder(t *testing.T) {
	dir := abandonRepo(t)
	runJJ(t, dir, "bookmark", "create", "a-first", "z-last", "-r", "@-")
	client := newClient(t, dir)
	child := loadChange(t, client, "@-")

	_, err := client.Abandon(t.Context(), child.CommitID, []string{"z-last", "a-first"})

	if err != nil {
		t.Fatalf("Abandon(%q) error = %v", child.CommitID, err)
	}
}

func divergentRepo(t *testing.T) (dir, change, original, first string) {
	t.Helper()
	dir = abandonRepo(t)
	change = changeID(t, dir, "@-")
	original = commitID(t, dir, "@-")
	op := runJJ(t, dir, "operation", "log", "--ignore-working-copy", "--no-graph", "-n1", "-T", "id")
	runJJ(t, dir, "describe", "-r", "@-", "-m", "first")
	first = commitID(t, dir, "@-")
	runJJ(t, dir, "--at-op", op, "describe", "-r", `change_id("`+change+`")`, "-m", "second")
	return dir, change, original, first
}

func TestSuccessor_success(t *testing.T) {
	dir := abandonRepo(t)
	client := newClient(t, dir)
	child := loadChange(t, client, "@-")
	work := loadChange(t, client, "@")
	if _, err := client.Abandon(t.Context(), child.CommitID, nil); err != nil {
		t.Fatalf("Abandon(%q) error = %v", child.CommitID, err)
	}

	got, err := client.Successor(t.Context(), work.ChangeID, work.CommitID)

	if err != nil {
		t.Fatalf("Successor(%q) error = %v", work.CommitID, err)
	}
	if want := loadChange(t, client, "@"); got.CommitID != want.CommitID {
		t.Errorf("Successor(%q) = %q, want %q", work.CommitID, got.CommitID, want.CommitID)
	}
}

func TestSuccessor_failed(t *testing.T) {
	t.Run("a divergent sibling is not a successor", func(t *testing.T) {
		dir, change, _, first := divergentRepo(t)
		runJJ(t, dir, "abandon", "-r", `commit_id("`+first+`")`)
		client := newClient(t, dir)

		_, err := client.Successor(t.Context(), change, first)

		if !errors.Is(err, jj.ErrNoRevision) {
			t.Errorf("Successor(%q) error = %v, want %v", first, err, jj.ErrNoRevision)
		}
	})

	t.Run("several rewrites are ambiguous", func(t *testing.T) {
		dir, change, original, _ := divergentRepo(t)
		client := newClient(t, dir)

		_, err := client.Successor(t.Context(), change, original)

		if !errors.Is(err, jj.ErrMultipleRevisions) {
			t.Errorf("Successor(%q) error = %v, want %v", original, err, jj.ErrMultipleRevisions)
		}
	})
}

func aliasTemplates(t *testing.T, dir string, names ...string) {
	t.Helper()
	for _, name := range names {
		runJJ(t, dir, "config", "set", "--repo", "template-aliases."+name, `"description"`)
	}
}

func TestSuccessor_successWithTemplateAliases(t *testing.T) {
	dir := abandonRepo(t)
	client := newClient(t, dir)
	child := loadChange(t, client, "@-")
	work := loadChange(t, client, "@")
	if _, err := client.Abandon(t.Context(), child.CommitID, nil); err != nil {
		t.Fatalf("Abandon(%q) error = %v", child.CommitID, err)
	}
	aliasTemplates(t, dir, "commit")

	got, err := client.Successor(t.Context(), work.ChangeID, work.CommitID)

	if err != nil {
		t.Fatalf("Successor(%q) error = %v", work.CommitID, err)
	}
	if got.ChangeID != work.ChangeID {
		t.Errorf("Successor(%q).ChangeID = %q, want %q", work.CommitID, got.ChangeID, work.ChangeID)
	}
}

func TestAbandon_successWithTemplateAliases(t *testing.T) {
	dir := abandonRepo(t)
	runJJ(t, dir, "bookmark", "create", "feature", "-r", "@")
	client := newClient(t, dir)
	work := loadChange(t, client, "@")
	aliasTemplates(t, dir, "local_bookmarks", "current_working_copy")

	got, err := client.Abandon(t.Context(), work.CommitID, []string{"feature"})

	if err != nil {
		t.Fatalf("Abandon(%q) error = %v", work.CommitID, err)
	}
	if !got {
		t.Errorf("Abandon(%q) working copy = false, want true", work.CommitID)
	}
}

func TestSuccessor_successStopsAtTheFirstMatch(t *testing.T) {
	dir := abandonRepo(t)
	client := newClient(t, dir)
	work := loadChange(t, client, "@")
	beforeCommand(t, "evolog", `printf '%s\n' `+work.CommitID+`; exec sleep 30`)
	start := time.Now()

	got, err := client.Successor(t.Context(), work.ChangeID, work.CommitID)

	if err != nil {
		t.Fatalf("Successor(%q) error = %v", work.CommitID, err)
	}
	if got.CommitID != work.CommitID {
		t.Errorf("Successor(%q) = %q, want %q", work.CommitID, got.CommitID, work.CommitID)
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Errorf("Successor() took %v, want it to stop after the match", elapsed)
	}
}

func TestSuccessor_failedEvolog(t *testing.T) {
	dir := abandonRepo(t)
	client := newClient(t, dir)
	work := loadChange(t, client, "@")
	beforeCommand(t, "evolog", `echo 'evolog broke' >&2; exit 1`)

	_, err := client.Successor(t.Context(), work.ChangeID, work.CommitID)

	if err == nil || !strings.Contains(err.Error(), "evolog broke") {
		t.Errorf("Successor() error = %v, want the evolog failure", err)
	}
}
