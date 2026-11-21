package jj_test

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/madalinpopa/jjreview/internal/jj"
)

func changeID(t *testing.T, dir, rev string) string {
	t.Helper()
	return runJJ(t, dir, "log", "-r", rev, "--no-graph", "-T", "change_id")
}

func commitID(t *testing.T, dir, rev string) string {
	t.Helper()
	return runJJ(t, dir, "log", "-r", rev, "--no-graph", "-T", "commit_id")
}

func branchyRepo(t *testing.T) string {
	t.Helper()
	dir := newRepo(t)
	runJJ(t, dir, "describe", "-m", "base \"quoted\" ü ✓\tend")
	runJJ(t, dir, "bookmark", "create", "main", "-r", "@")
	runJJ(t, dir, "new", "-m", "child a\n\nbody line\n")
	runJJ(t, dir, "bookmark", "create", "feat", "feat-2", "-r", "@")
	runJJ(t, dir, "new", "main", "-m", "child b")
	return dir
}

func TestLog_success(t *testing.T) {
	t.Run("returns graph lines and entries in jj log order", func(t *testing.T) {
		dir := branchyRepo(t)
		childB := changeID(t, dir, "@")
		childA := changeID(t, dir, "feat")
		base := changeID(t, dir, "main")
		want := []jj.LogLine{
			{Graph: "@  ", Entry: &jj.LogEntry{ChangeID: childB, CommitID: commitID(t, dir, "@"), Description: "child b\n", Author: "Test User", Parents: []string{commitID(t, dir, "main")}, WorkingCopy: true}},
			{Graph: "│ ○  ", Entry: &jj.LogEntry{ChangeID: childA, CommitID: commitID(t, dir, "feat"), Description: "child a\n\nbody line\n", Author: "Test User", Bookmarks: []string{"feat", "feat-2"}, Parents: []string{commitID(t, dir, "main")}}},
			{Graph: "├─╯"},
			{Graph: "○  ", Entry: &jj.LogEntry{ChangeID: base, CommitID: commitID(t, dir, "main"), Description: "base \"quoted\" ü ✓\tend\n", Author: "Test User", Bookmarks: []string{"main"}, Parents: []string{commitID(t, dir, "root()")}}},
			{Graph: "◆  ", Entry: &jj.LogEntry{ChangeID: changeID(t, dir, "root()"), CommitID: commitID(t, dir, "root()"), Immutable: true}},
		}

		got, err := newClient(t, dir).Log(t.Context())
		if err != nil {
			t.Fatalf("Log() error = %v", err)
		}

		if len(got) != len(want) {
			t.Fatalf("Log() returned %d lines, want %d: %+v", len(got), len(want), got)
		}
		for i, line := range got {
			if line.Graph != want[i].Graph {
				t.Errorf("Log()[%d].Graph = %q, want %q", i, line.Graph, want[i].Graph)
			}
			if (line.Entry == nil) != (want[i].Entry == nil) {
				t.Errorf("Log()[%d].Entry = %+v, want %+v", i, line.Entry, want[i].Entry)
				continue
			}
			if line.Entry == nil {
				continue
			}
			if line.Entry.Timestamp == "" {
				t.Errorf("Log()[%d].Entry.Timestamp is empty", i)
			}
			gotEntry := *line.Entry
			gotEntry.Timestamp = ""
			if !reflect.DeepEqual(gotEntry, *want[i].Entry) {
				t.Errorf("Log()[%d].Entry = %+v, want %+v", i, gotEntry, *want[i].Entry)
			}
		}
	})

	t.Run("lists the workspaces whose working copy each revision is", func(t *testing.T) {
		dir := newRepo(t)
		root := t.TempDir()
		ui := filepath.Join(root, "ui")
		api := filepath.Join(root, "api")
		runJJ(t, dir, "workspace", "add", "--name", "p1-ui", ui)
		runJJ(t, dir, "workspace", "add", "--name", "p2-api", api)
		shared := changeID(t, ui, "@")
		runJJ(t, api, "edit", shared)
		runJJ(t, dir, "new", "-m", "parent")
		parent := changeID(t, dir, "@")
		runJJ(t, dir, "new")

		got, err := newClient(t, dir).Log(t.Context())
		if err != nil {
			t.Fatalf("Log() error = %v", err)
		}

		workspaces := map[string][]string{}
		for _, line := range got {
			if line.Entry != nil {
				workspaces[line.Entry.ChangeID] = line.Entry.Workspaces
			}
		}
		want := map[string][]string{
			changeID(t, dir, "@"):      {"default"},
			parent:                     nil,
			shared:                     {"p1-ui", "p2-api"},
			changeID(t, dir, "root()"): nil,
		}
		for id, names := range want {
			if !reflect.DeepEqual(workspaces[id], names) {
				t.Errorf("Log() entry %s Workspaces = %q, want %q", id, workspaces[id], names)
			}
		}
	})

	t.Run("lists every revision regardless of revsets.log", func(t *testing.T) {
		dir := branchyRepo(t)
		runJJ(t, dir, "config", "set", "--repo", "revsets.log", "@")

		got, err := newClient(t, dir).Log(t.Context())
		if err != nil {
			t.Fatalf("Log() error = %v", err)
		}

		var ids []string
		for _, line := range got {
			if line.Entry != nil {
				ids = append(ids, line.Entry.ChangeID)
			}
		}
		want := []string{changeID(t, dir, "@"), changeID(t, dir, "feat"), changeID(t, dir, "main"), changeID(t, dir, "root()")}
		if !reflect.DeepEqual(ids, want) {
			t.Errorf("Log() change IDs = %v, want %v", ids, want)
		}
	})

	t.Run("timestamps are relative", func(t *testing.T) {
		dir := newRepo(t)

		got, err := newClient(t, dir).Log(t.Context())
		if err != nil {
			t.Fatalf("Log() error = %v", err)
		}

		if len(got) == 0 {
			t.Fatal("Log() returned no lines")
		}
		last := got[len(got)-1]
		if last.Entry == nil || !strings.HasSuffix(last.Entry.Timestamp, " ago") {
			t.Errorf("Log() root entry = %+v, want a relative timestamp ending in \" ago\"", last.Entry)
		}
	})

	t.Run("author falls back to email", func(t *testing.T) {
		dir := newRepo(t)
		t.Setenv("JJ_USER", "")
		runJJ(t, dir, "new")

		got, err := newClient(t, dir).Log(t.Context())
		if err != nil {
			t.Fatalf("Log() error = %v", err)
		}

		if len(got) == 0 {
			t.Fatal("Log() returned no lines")
		}
		if got[0].Entry == nil || got[0].Entry.Author != "test@example.com" {
			t.Errorf("Log()[0].Entry = %+v, want Author %q", got[0].Entry, "test@example.com")
		}
	})
}

func TestLog_failed(t *testing.T) {
	newRepo(t)

	_, err := newClient(t, t.TempDir()).Log(t.Context())

	if err == nil {
		t.Fatal("Log() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "jj log") || !strings.Contains(err.Error(), "no jj repo") {
		t.Errorf("Log() error = %q, want it to name the command and the jj failure", err)
	}
}

func TestLog_successWithWordWrap(t *testing.T) {
	dir := newRepo(t)
	description := strings.Repeat("a long description that wraps ", 8) + "\n\nwith a body that wraps as well " + strings.Repeat("word ", 20) + "\n"
	runJJ(t, dir, "describe", "-m", description)
	runJJ(t, dir, "config", "set", "--repo", "ui.log-word-wrap", "true")
	t.Setenv("COLUMNS", "20")
	client := newClient(t, dir)

	lines, err := client.Log(t.Context())
	if err != nil {
		t.Fatalf("Log() error = %v", err)
	}
	change, err := client.Change(t.Context(), "@")
	if err != nil {
		t.Fatalf("Change() error = %v", err)
	}

	if len(lines) == 0 || lines[0].Entry == nil {
		t.Fatalf("Log() = %+v, want the working copy entry first", lines)
	}
	if got := lines[0].Entry.Description; got != description {
		t.Errorf("Log()[0].Entry.Description = %q, want %q", got, description)
	}
	if change.Description != description {
		t.Errorf("Change() Description = %q, want %q", change.Description, description)
	}
}
