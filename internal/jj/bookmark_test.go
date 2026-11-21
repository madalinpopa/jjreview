package jj_test

import (
	"errors"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/madalinpopa/jjreview/internal/jj"
)

func bookmarkNames(t *testing.T, client jj.Client) []string {
	t.Helper()
	bookmarks, err := client.Bookmarks(t.Context())
	if err != nil {
		t.Fatalf("Bookmarks() error = %v", err)
	}
	var names []string
	for _, b := range bookmarks {
		names = append(names, b.Name)
	}
	return names
}

func bookmarkTarget(t *testing.T, client jj.Client, name string) string {
	t.Helper()
	bookmarks, err := client.Bookmarks(t.Context())
	if err != nil {
		t.Fatalf("Bookmarks() error = %v", err)
	}
	for _, b := range bookmarks {
		if b.Name == name {
			return b.CommitID
		}
	}
	t.Fatalf("Bookmarks() has no bookmark %q", name)
	return ""
}

func clonedRepo(t *testing.T) string {
	t.Helper()
	origin := newRepo(t)
	runJJ(t, origin, "describe", "-m", "base")
	runJJ(t, origin, "bookmark", "create", "main", "-r", "@")
	runJJ(t, origin, "new", "-m", "other")
	runJJ(t, origin, "bookmark", "create", "other", "-r", "@")
	runJJ(t, origin, "new")
	dir := filepath.Join(t.TempDir(), "clone")
	runJJ(t, origin, "git", "clone", origin, dir)
	runJJ(t, dir, "bookmark", "track", "main", "other", "--remote", "origin")
	return dir
}

func conflictedRepo(t *testing.T) string {
	t.Helper()
	dir := branchyRepo(t)
	op := strings.TrimSpace(runJJ(t, dir, "operation", "log", "--no-graph", "--limit=1", "-T", "id"))
	runJJ(t, dir, "bookmark", "set", "main", "-r", "@")
	runJJ(t, dir, "--at-op", op, "bookmark", "set", "main", "-r", "feat")
	runJJ(t, dir, "status")
	return dir
}

func TestBookmarks_success(t *testing.T) {
	t.Run("lists local bookmarks with their targets", func(t *testing.T) {
		dir := branchyRepo(t)
		want := []jj.Bookmark{
			{Name: "feat", ChangeID: changeID(t, dir, "feat"), CommitID: commitID(t, dir, "feat"), Description: "child a\n\nbody line\n"},
			{Name: "feat-2", ChangeID: changeID(t, dir, "feat"), CommitID: commitID(t, dir, "feat"), Description: "child a\n\nbody line\n"},
			{Name: "main", ChangeID: changeID(t, dir, "main"), CommitID: commitID(t, dir, "main"), Description: "base \"quoted\" ü ✓\tend\n"},
		}

		got, err := newClient(t, dir).Bookmarks(t.Context())
		if err != nil {
			t.Fatalf("Bookmarks() error = %v", err)
		}

		if !reflect.DeepEqual(got, want) {
			t.Errorf("Bookmarks() = %+v, want %+v", got, want)
		}
	})

	t.Run("returns no bookmarks in a repository without any", func(t *testing.T) {
		dir := newRepo(t)

		got, err := newClient(t, dir).Bookmarks(t.Context())
		if err != nil {
			t.Fatalf("Bookmarks() error = %v", err)
		}

		if len(got) != 0 {
			t.Errorf("Bookmarks() = %+v, want none", got)
		}
	})

	t.Run("reports how tracked remotes relate to the local bookmark", func(t *testing.T) {
		dir := clonedRepo(t)
		runJJ(t, dir, "new", "main", "-m", "ahead")
		runJJ(t, dir, "bookmark", "set", "main", "-r", "@")

		got, err := newClient(t, dir).Bookmarks(t.Context())
		if err != nil {
			t.Fatalf("Bookmarks() error = %v", err)
		}

		want := map[string][]jj.RemoteBookmark{
			"main":  {{Remote: "origin", Ahead: 1}},
			"other": {{Remote: "origin", Synced: true}},
		}
		if len(got) != len(want) {
			t.Fatalf("Bookmarks() = %+v, want bookmarks %v", got, want)
		}
		for _, b := range got {
			if !reflect.DeepEqual(b.Remotes, want[b.Name]) {
				t.Errorf("Bookmarks() %q Remotes = %+v, want %+v", b.Name, b.Remotes, want[b.Name])
			}
		}
	})

	t.Run("reports a bookmark behind its remote", func(t *testing.T) {
		dir := clonedRepo(t)
		runJJ(t, dir, "bookmark", "set", "other", "-r", "main", "--allow-backwards")

		got, err := newClient(t, dir).Bookmarks(t.Context())
		if err != nil {
			t.Fatalf("Bookmarks() error = %v", err)
		}

		want := []jj.RemoteBookmark{{Remote: "origin", Behind: 1}}
		if len(got) != 2 || got[1].Name != "other" || !reflect.DeepEqual(got[1].Remotes, want) {
			t.Errorf("Bookmarks() = %+v, want other with Remotes %+v", got, want)
		}
	})

	t.Run("skips a deleted local bookmark that a remote still tracks", func(t *testing.T) {
		dir := clonedRepo(t)
		runJJ(t, dir, "bookmark", "delete", "main")

		got := bookmarkNames(t, newClient(t, dir))

		if want := []string{"other"}; !reflect.DeepEqual(got, want) {
			t.Errorf("Bookmarks() names = %v, want %v", got, want)
		}
	})

	t.Run("marks a conflicted bookmark with its targets", func(t *testing.T) {
		dir := conflictedRepo(t)

		got, err := newClient(t, dir).Bookmarks(t.Context())
		if err != nil {
			t.Fatalf("Bookmarks() error = %v", err)
		}

		want := jj.Bookmark{
			Name:     "main",
			Conflict: true,
			Added:    slices.Sorted(slices.Values([]string{commitID(t, dir, "@"), commitID(t, dir, "feat")})),
			Removed:  []string{commitID(t, dir, "@-")},
		}
		if len(got) != 3 || !reflect.DeepEqual(got[2], want) {
			t.Errorf("Bookmarks() = %+v, want last %+v", got, want)
		}
	})
}

func TestBookmarks_absentConflictSide(t *testing.T) {
	dir := newRepo(t)
	runJJ(t, dir, "describe", "-m", "a")
	a := commitID(t, dir, "@")
	runJJ(t, dir, "new", "root()", "-m", "b")
	op := strings.TrimSpace(runJJ(t, dir, "operation", "log", "--no-graph", "--limit=1", "-T", "id"))
	runJJ(t, dir, "bookmark", "create", "x", "-r", "@")
	runJJ(t, dir, "--at-op", op, "bookmark", "create", "x", "-r", jj.CommitRevset(a))
	runJJ(t, dir, "status")

	got, err := newClient(t, dir).Bookmarks(t.Context())
	if err != nil {
		t.Fatalf("Bookmarks() error = %v", err)
	}

	want := []jj.Bookmark{{
		Name:     "x",
		Conflict: true,
		Added:    slices.Sorted(slices.Values([]string{a, commitID(t, dir, "@")})),
		Removed:  []string{""},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Bookmarks() = %+v, want %+v", got, want)
	}
}

func TestBookmarks_failed(t *testing.T) {
	newRepo(t)

	_, err := newClient(t, t.TempDir()).Bookmarks(t.Context())

	if err == nil || !strings.Contains(err.Error(), "jj bookmark list") {
		t.Errorf("Bookmarks() error = %v, want it to name the command", err)
	}
}

func TestCreateBookmark_success(t *testing.T) {
	tests := map[string]string{
		"plain name":                 "topic",
		"name with revset operators": `-we"ird a*b|c\d`,
	}
	for name, bookmark := range tests {
		t.Run(name, func(t *testing.T) {
			dir := branchyRepo(t)
			client := newClient(t, dir)

			if _, err := client.CreateBookmark(t.Context(), bookmark, "feat"); err != nil {
				t.Fatalf("CreateBookmark(%q) error = %v", bookmark, err)
			}

			if got, want := bookmarkTarget(t, client, bookmark), commitID(t, dir, "feat"); got != want {
				t.Errorf("CreateBookmark(%q) target = %q, want %q", bookmark, got, want)
			}
		})
	}
}

func TestCreateBookmark_failed(t *testing.T) {
	tests := map[string]struct {
		name    string
		rev     string
		wantErr string
	}{
		"existing bookmark": {name: "main", rev: "@", wantErr: "already exists"},
		"unknown revision":  {name: "topic", rev: "nope", wantErr: "doesn't exist"},
		"empty name":        {name: "", rev: "@", wantErr: "jj bookmark create"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			dir := branchyRepo(t)
			client := newClient(t, dir)

			_, err := client.CreateBookmark(t.Context(), tt.name, tt.rev)

			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("CreateBookmark(%q, %q) error = %v, want it to contain %q", tt.name, tt.rev, err, tt.wantErr)
			}
			if got, want := bookmarkNames(t, client), []string{"feat", "feat-2", "main"}; !reflect.DeepEqual(got, want) {
				t.Errorf("Bookmarks() names = %v, want %v", got, want)
			}
		})
	}
}

func TestRenameBookmark_success(t *testing.T) {
	dir := branchyRepo(t)
	client := newClient(t, dir)
	if _, err := client.CreateBookmark(t.Context(), `a"b*`, "main"); err != nil {
		t.Fatalf("CreateBookmark() error = %v", err)
	}

	if _, err := client.RenameBookmark(t.Context(), `a"b*`, `c d\e`); err != nil {
		t.Fatalf("RenameBookmark() error = %v", err)
	}

	if got, want := bookmarkNames(t, client), []string{`c d\e`, "feat", "feat-2", "main"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Bookmarks() names = %v, want %v", got, want)
	}
	if got, want := bookmarkTarget(t, client, `c d\e`), commitID(t, dir, "main"); got != want {
		t.Errorf("RenameBookmark() target = %q, want %q", got, want)
	}
}

func TestRenameBookmark_failed(t *testing.T) {
	dir := branchyRepo(t)
	client := newClient(t, dir)

	_, err := client.RenameBookmark(t.Context(), "main", "feat")

	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("RenameBookmark() error = %v, want it to report the existing bookmark", err)
	}
}

func TestMoveBookmark_success(t *testing.T) {
	t.Run("moves a bookmark forwards", func(t *testing.T) {
		dir := branchyRepo(t)
		client := newClient(t, dir)

		if _, err := client.MoveBookmark(t.Context(), "main", []string{commitID(t, dir, "main")}, commitID(t, dir, "@"), false); err != nil {
			t.Fatalf("MoveBookmark() error = %v", err)
		}

		if got, want := bookmarkTarget(t, client, "main"), commitID(t, dir, "@"); got != want {
			t.Errorf("MoveBookmark() target = %q, want %q", got, want)
		}
	})

	t.Run("moves a bookmark sideways when allowed", func(t *testing.T) {
		dir := branchyRepo(t)
		client := newClient(t, dir)

		if _, err := client.MoveBookmark(t.Context(), "feat", []string{commitID(t, dir, "feat")}, commitID(t, dir, "@"), true); err != nil {
			t.Fatalf("MoveBookmark() error = %v", err)
		}

		if got, want := bookmarkTarget(t, client, "feat"), commitID(t, dir, "@"); got != want {
			t.Errorf("MoveBookmark() target = %q, want %q", got, want)
		}
	})

	t.Run("moves a bookmark with special characters", func(t *testing.T) {
		dir := branchyRepo(t)
		client := newClient(t, dir)
		if _, err := client.CreateBookmark(t.Context(), `x|y"*`, "main"); err != nil {
			t.Fatalf("CreateBookmark() error = %v", err)
		}

		if _, err := client.MoveBookmark(t.Context(), `x|y"*`, []string{commitID(t, dir, "main")}, commitID(t, dir, "@"), false); err != nil {
			t.Fatalf("MoveBookmark() error = %v", err)
		}

		if got, want := bookmarkTarget(t, client, `x|y"*`), commitID(t, dir, "@"); got != want {
			t.Errorf("MoveBookmark() target = %q, want %q", got, want)
		}
	})

	t.Run("a description quoting jj does not hide the move", func(t *testing.T) {
		dir := branchyRepo(t)
		runJJ(t, dir, "describe", "-m", "No bookmarks to update.")
		client := newClient(t, dir)

		if _, err := client.MoveBookmark(t.Context(), "main", []string{commitID(t, dir, "main")}, commitID(t, dir, "@"), false); err != nil {
			t.Fatalf("MoveBookmark() error = %v", err)
		}

		if got, want := bookmarkTarget(t, client, "main"), commitID(t, dir, "@"); got != want {
			t.Errorf("MoveBookmark() target = %q, want %q", got, want)
		}
	})

	t.Run("resolves a conflicted bookmark", func(t *testing.T) {
		dir := conflictedRepo(t)
		client := newClient(t, dir)
		from := []string{commitID(t, dir, "@"), commitID(t, dir, "feat")}

		if _, err := client.MoveBookmark(t.Context(), "main", from, commitID(t, dir, "@"), true); err != nil {
			t.Fatalf("MoveBookmark() error = %v", err)
		}

		if got, want := bookmarkTarget(t, client, "main"), commitID(t, dir, "@"); got != want {
			t.Errorf("MoveBookmark() target = %q, want %q", got, want)
		}
	})
}

func TestMoveBookmark_failed(t *testing.T) {
	t.Run("refuses to move a bookmark sideways", func(t *testing.T) {
		dir := branchyRepo(t)
		client := newClient(t, dir)
		want := commitID(t, dir, "feat")

		_, err := client.MoveBookmark(t.Context(), "feat", []string{want}, commitID(t, dir, "@"), false)

		if !errors.Is(err, jj.ErrBookmarkBackwards) {
			t.Errorf("MoveBookmark() error = %v, want %v", err, jj.ErrBookmarkBackwards)
		}
		if got := bookmarkTarget(t, client, "feat"); got != want {
			t.Errorf("MoveBookmark() target = %q, want unchanged %q", got, want)
		}
	})

	t.Run("refuses a bookmark that is no longer at the old target", func(t *testing.T) {
		dir := branchyRepo(t)
		client := newClient(t, dir)
		want := commitID(t, dir, "main")

		_, err := client.MoveBookmark(t.Context(), "main", []string{commitID(t, dir, "feat")}, commitID(t, dir, "@"), true)

		if !errors.Is(err, jj.ErrBookmarkChanged) {
			t.Errorf("MoveBookmark() error = %v, want %v", err, jj.ErrBookmarkChanged)
		}
		if got := bookmarkTarget(t, client, "main"); got != want {
			t.Errorf("MoveBookmark() target = %q, want unchanged %q", got, want)
		}
	})

	t.Run("does not create a missing bookmark", func(t *testing.T) {
		dir := branchyRepo(t)
		client := newClient(t, dir)

		_, err := client.MoveBookmark(t.Context(), "gone", []string{commitID(t, dir, "main")}, commitID(t, dir, "@"), true)

		if !errors.Is(err, jj.ErrBookmarkChanged) {
			t.Errorf("MoveBookmark() error = %v, want %v", err, jj.ErrBookmarkChanged)
		}
		if got, want := bookmarkNames(t, client), []string{"feat", "feat-2", "main"}; !reflect.DeepEqual(got, want) {
			t.Errorf("Bookmarks() names = %v, want %v", got, want)
		}
	})

	t.Run("a change right before the move is not overwritten", func(t *testing.T) {
		dir := branchyRepo(t)
		client := newClient(t, dir)
		from, racer := commitID(t, dir, "main"), commitID(t, dir, "feat")
		beforeCommand(t, "move", `"$`+realJJEnv+`" bookmark set main -r 'commit_id("`+racer+`")' >/dev/null 2>&1`)

		_, err := client.MoveBookmark(t.Context(), "main", []string{from}, commitID(t, dir, "@"), true)

		if !errors.Is(err, jj.ErrBookmarkChanged) {
			t.Errorf("MoveBookmark() error = %v, want %v", err, jj.ErrBookmarkChanged)
		}
		if got := bookmarkTarget(t, client, "main"); got != racer {
			t.Errorf("MoveBookmark() target = %q, want the concurrent target %q", got, racer)
		}
	})

	t.Run("a quiet jj still reports a skipped move", func(t *testing.T) {
		dir := branchyRepo(t)
		runJJ(t, dir, "config", "set", "--repo", "ui.quiet", "true")
		client := newClient(t, dir)
		want := commitID(t, dir, "main")

		_, err := client.MoveBookmark(t.Context(), "main", []string{commitID(t, dir, "feat")}, commitID(t, dir, "@"), true)

		if !errors.Is(err, jj.ErrBookmarkChanged) {
			t.Errorf("MoveBookmark() error = %v, want %v", err, jj.ErrBookmarkChanged)
		}
		if got := bookmarkTarget(t, client, "main"); got != want {
			t.Errorf("MoveBookmark() target = %q, want unchanged %q", got, want)
		}
	})

	t.Run("reports an unknown target", func(t *testing.T) {
		dir := branchyRepo(t)

		_, err := newClient(t, dir).MoveBookmark(t.Context(), "main", []string{commitID(t, dir, "main")}, strings.Repeat("f", 40), true)

		if err == nil || errors.Is(err, jj.ErrBookmarkBackwards) || errors.Is(err, jj.ErrBookmarkChanged) {
			t.Errorf("MoveBookmark() error = %v, want an unknown revision error", err)
		}
	})
}

func TestDeleteBookmark_success(t *testing.T) {
	dir := branchyRepo(t)
	client := newClient(t, dir)
	if _, err := client.CreateBookmark(t.Context(), "f*", "main"); err != nil {
		t.Fatalf("CreateBookmark() error = %v", err)
	}

	if _, err := client.DeleteBookmark(t.Context(), "f*"); err != nil {
		t.Fatalf("DeleteBookmark() error = %v", err)
	}

	if got, want := bookmarkNames(t, client), []string{"feat", "feat-2", "main"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Bookmarks() names = %v, want %v", got, want)
	}
}

func TestDeleteBookmark_failed(t *testing.T) {
	newRepo(t)

	_, err := newClient(t, t.TempDir()).DeleteBookmark(t.Context(), "main")

	if err == nil || !strings.Contains(err.Error(), "jj bookmark delete") {
		t.Errorf("DeleteBookmark() error = %v, want it to name the command", err)
	}
}

func TestBookmarkWarnings_success(t *testing.T) {
	t.Run("a rename reports the remote bookmark it left behind", func(t *testing.T) {
		dir := clonedRepo(t)

		warnings, err := newClient(t, dir).RenameBookmark(t.Context(), "main", "trunk")
		if err != nil {
			t.Fatalf("RenameBookmark() error = %v", err)
		}

		if !strings.Contains(warnings, "were not renamed") || !strings.Contains(warnings, "Hint:") {
			t.Errorf("RenameBookmark() warnings = %q, want the untracked remote warning and its hint", warnings)
		}
	})

	t.Run("only warnings are returned, not progress messages", func(t *testing.T) {
		dir := branchyRepo(t)
		runJJ(t, dir, "new")

		warnings, err := newClient(t, dir).CreateBookmark(t.Context(), "topic", "@")
		if err != nil {
			t.Fatalf("CreateBookmark() error = %v", err)
		}

		if !strings.Contains(warnings, "Target revision is empty") || strings.Contains(warnings, "Created") {
			t.Errorf("CreateBookmark() warnings = %q, want only the empty target warning", warnings)
		}
	})

	t.Run("a warning keeps its unindented continuation lines", func(t *testing.T) {
		dir := branchyRepo(t)
		runJJ(t, dir, "config", "set", "--repo", `revset-aliases."trunk()"`, "main@origin")

		warnings, err := newClient(t, dir).CreateBookmark(t.Context(), "topic", "main")
		if err != nil {
			t.Fatalf("CreateBookmark() error = %v", err)
		}

		want := "Warning: Failed to resolve `revset-aliases.trunk()`: Revision `main@origin` doesn't exist\n" +
			"The `trunk()` alias is temporarily set to `root()`.\n" +
			"Hint: Use `jj config edit --repo` to adjust the `trunk()` alias."
		if warnings != want {
			t.Errorf("CreateBookmark() warnings = %q, want %q", warnings, want)
		}
	})

	t.Run("a quiet command has no warnings", func(t *testing.T) {
		dir := branchyRepo(t)

		warnings, err := newClient(t, dir).DeleteBookmark(t.Context(), "feat")
		if err != nil {
			t.Fatalf("DeleteBookmark() error = %v", err)
		}

		if warnings != "" {
			t.Errorf("DeleteBookmark() warnings = %q, want none", warnings)
		}
	})
}

func TestVisibleRevset_success(t *testing.T) {
	dir := branchyRepo(t)
	hidden := commitID(t, dir, "@")
	runJJ(t, dir, "abandon", "@")
	client := newClient(t, dir)

	_, err := client.Change(t.Context(), jj.VisibleRevset(jj.CommitRevset(hidden)))

	if !errors.Is(err, jj.ErrNoRevision) {
		t.Errorf("Change(VisibleRevset(hidden)) error = %v, want %v", err, jj.ErrNoRevision)
	}
	if _, err := client.Change(t.Context(), jj.VisibleRevset(jj.CommitRevset(commitID(t, dir, "main")))); err != nil {
		t.Errorf("Change(VisibleRevset(main)) error = %v", err)
	}
}
