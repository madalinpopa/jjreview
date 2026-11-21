package jj_test

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/madalinpopa/jjreview/internal/jj"
)

func remoteRepo(t *testing.T) (dir, origin string) {
	t.Helper()
	origin = newRepo(t)
	runJJ(t, origin, "describe", "-m", "base")
	runJJ(t, origin, "bookmark", "create", "main", "-r", "@")
	runJJ(t, origin, "new")
	dir = filepath.Join(t.TempDir(), "clone")
	runJJ(t, origin, "git", "clone", origin, dir)
	runJJ(t, dir, "bookmark", "track", "main", "--remote", "origin")
	writeFile(t, dir, "a.txt", "a\n")
	runJJ(t, dir, "describe", "-m", "work")
	return dir, origin
}

func remoteTarget(t *testing.T, origin, bookmark string) string {
	t.Helper()
	runJJ(t, origin, "git", "import")
	return strings.TrimSpace(runJJ(t, origin, "log", "--no-graph", "-r", `bookmarks(exact:"`+bookmark+`")`, "-T", "commit_id"))
}

func samePushes(a, b []jj.PushResult) bool {
	return slices.EqualFunc(a, b, func(x, y jj.PushResult) bool {
		return x.Remote == y.Remote && slices.Equal(x.Bookmarks, y.Bookmarks)
	})
}

func TestPush_success(t *testing.T) {
	t.Run("pushes the bookmarks of the revision", func(t *testing.T) {
		dir, origin := remoteRepo(t)
		runJJ(t, dir, "bookmark", "create", "feature", "-r", "@")
		client := newClient(t, dir)
		e := logEntry(t, client, "@")
		e.Bookmarks = []string{"feature"}

		got, err := client.Push(t.Context(), e)

		if err != nil {
			t.Fatalf("Push() error = %v", err)
		}
		if want := []jj.PushResult{{Remote: "origin", Bookmarks: []string{"feature"}}}; !samePushes(got, want) {
			t.Errorf("Push() = %+v, want %+v", got, want)
		}
		if remote := remoteTarget(t, origin, "feature"); remote != logEntry(t, client, "@").CommitID {
			t.Errorf("origin feature = %q, want the pushed revision", remote)
		}
	})

	t.Run("groups the pushed bookmarks by remote", func(t *testing.T) {
		dir, origin := remoteRepo(t)
		mirror := newRepo(t)
		runJJ(t, dir, "git", "remote", "add", "mirror", mirror)
		runJJ(t, dir, "config", "set", "--repo", "git.push", `["origin", "mirror"]`)
		runJJ(t, dir, "bookmark", "create", "feature", "-r", "@")
		runJJ(t, dir, "bookmark", "track", "feature", "--remote", "origin")
		runJJ(t, dir, "bookmark", "track", "feature", "--remote", "mirror")
		client := newClient(t, dir)
		e := logEntry(t, client, "@")
		e.Bookmarks = []string{"feature"}

		got, err := client.Push(t.Context(), e)

		if err != nil {
			t.Fatalf("Push() error = %v", err)
		}
		want := []jj.PushResult{{Remote: "mirror", Bookmarks: []string{"feature"}}, {Remote: "origin", Bookmarks: []string{"feature"}}}
		if !samePushes(got, want) {
			t.Errorf("Push() = %+v, want %+v", got, want)
		}
		if remote := remoteTarget(t, origin, "feature"); remote != logEntry(t, client, "@").CommitID {
			t.Errorf("origin feature = %q, want the pushed revision", remote)
		}
	})

	t.Run("pushes a revision without bookmarks as a new change bookmark", func(t *testing.T) {
		dir, origin := remoteRepo(t)
		client := newClient(t, dir)

		got, err := client.Push(t.Context(), logEntry(t, client, "@"))

		if err != nil {
			t.Fatalf("Push() error = %v", err)
		}
		if len(got) != 1 || got[0].Remote != "origin" || len(got[0].Bookmarks) != 1 || !strings.HasPrefix(got[0].Bookmarks[0], "push-") {
			t.Fatalf("Push() = %+v, want one push- bookmark on origin", got)
		}
		if remote := remoteTarget(t, origin, got[0].Bookmarks[0]); remote != logEntry(t, client, "@").CommitID {
			t.Errorf("origin %s = %q, want the pushed revision", got[0].Bookmarks[0], remote)
		}
	})

	t.Run("reports nothing when the remote is up to date", func(t *testing.T) {
		dir, _ := remoteRepo(t)
		client := newClient(t, dir)

		e := logEntry(t, client, "main")
		e.Bookmarks = []string{"main"}

		got, err := client.Push(t.Context(), e)

		if err != nil || len(got) != 0 {
			t.Errorf("Push() = %+v, %v, want nothing pushed and no error", got, err)
		}
	})
}

func TestPush_failed(t *testing.T) {
	t.Run("a bookmark added since the revision was listed", func(t *testing.T) {
		dir, _ := remoteRepo(t)
		client := newClient(t, dir)
		e := logEntry(t, client, "@")
		runJJ(t, dir, "bookmark", "create", "feature", "-r", "@")

		_, err := client.Push(t.Context(), e)

		if !errors.Is(err, jj.ErrBookmarksChanged) {
			t.Errorf("Push() error = %v, want %v", err, jj.ErrBookmarksChanged)
		}
	})

	t.Run("a revision rewritten since it was listed", func(t *testing.T) {
		dir, _ := remoteRepo(t)
		client := newClient(t, dir)
		e := logEntry(t, client, "@")
		runJJ(t, dir, "describe", "-m", "reworded")

		_, err := client.Push(t.Context(), e)

		if !errors.Is(err, jj.ErrHiddenRevision) {
			t.Errorf("Push() error = %v, want %v", err, jj.ErrHiddenRevision)
		}
	})

	t.Run("a bookmark moved on the remote reports the jj error first", func(t *testing.T) {
		dir, origin := remoteRepo(t)
		runJJ(t, dir, "bookmark", "set", "main", "-r", "@")
		runJJ(t, origin, "describe", "-m", "upstream")
		runJJ(t, origin, "new")
		runJJ(t, origin, "bookmark", "set", "main", "-r", "@-")
		client := newClient(t, dir)
		e := logEntry(t, client, "@")
		e.Bookmarks = []string{"main"}

		_, err := client.Push(t.Context(), e)

		if err == nil {
			t.Fatal("Push() error = nil, want an error")
		}
		if first, _, _ := strings.Cut(err.Error(), "\n"); !strings.HasSuffix(first, "Error: Failed to push some bookmarks") {
			t.Errorf("Push() error first line = %q, want the jj error", first)
		}
		if !strings.Contains(err.Error(), "stale info") {
			t.Errorf("Push() error = %q, want the stale reference warning kept", err)
		}
	})

	t.Run("a repository without a remote", func(t *testing.T) {
		dir := abandonRepo(t)
		client := newClient(t, dir)

		if _, err := client.Push(t.Context(), logEntry(t, client, "@")); err == nil {
			t.Error("Push() error = nil, want an error")
		}
	})
}

func TestFetch_success(t *testing.T) {
	t.Run("imports the new remote bookmarks", func(t *testing.T) {
		dir, origin := remoteRepo(t)
		runJJ(t, origin, "describe", "-m", "upstream")
		runJJ(t, origin, "bookmark", "create", "upstream", "-r", "@")
		client := newClient(t, dir)

		changed, err := client.Fetch(t.Context())

		if err != nil || !changed {
			t.Fatalf("Fetch() = %v, %v, want a change and no error", changed, err)
		}
		if got := runJJ(t, dir, "log", "--no-graph", "-r", `remote_bookmarks(exact:"upstream")`, "-T", "description"); got != "upstream\n" {
			t.Errorf("upstream@origin description = %q, want %q", got, "upstream\n")
		}
	})

	t.Run("reports an up to date remote", func(t *testing.T) {
		dir, _ := remoteRepo(t)

		changed, err := newClient(t, dir).Fetch(t.Context())

		if err != nil || changed {
			t.Errorf("Fetch() = %v, %v, want no change and no error", changed, err)
		}
	})
}

func TestFetch_failed(t *testing.T) {
	t.Run("a repository without a remote", func(t *testing.T) {
		dir := abandonRepo(t)

		if _, err := newClient(t, dir).Fetch(t.Context()); err == nil {
			t.Error("Fetch() error = nil, want an error")
		}
	})
}
