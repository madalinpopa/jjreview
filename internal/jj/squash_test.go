package jj_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/madalinpopa/jjreview/internal/jj"
)

func logEntry(t *testing.T, client jj.Client, rev string) jj.LogEntry {
	t.Helper()
	c := loadChange(t, client, rev)
	return jj.LogEntry{ChangeID: c.ChangeID, CommitID: c.CommitID, Description: c.Description}
}

func TestSquash_success(t *testing.T) {
	t.Run("moves the changes into the destination and drops the source", func(t *testing.T) {
		dir := abandonRepo(t)
		client := newClient(t, dir)
		parent := logEntry(t, client, "@--")
		child := logEntry(t, client, "@-")

		_, err := client.Squash(t.Context(), []jj.LogEntry{child}, parent)

		if err != nil {
			t.Fatalf("Squash() error = %v", err)
		}
		if _, err := client.Change(t.Context(), jj.ChangeRevset(child.ChangeID)); !errors.Is(err, jj.ErrNoRevision) {
			t.Errorf("Change(%q) error = %v, want %v", child.ChangeID, err, jj.ErrNoRevision)
		}
		got := loadChange(t, client, jj.ChangeRevset(parent.ChangeID))
		if paths := diffPaths(t, client, got.CommitID); !slices.Equal(paths, []string{"a.txt", "b.txt"}) {
			t.Errorf("diff paths = %v, want [a.txt b.txt]", paths)
		}
		if got.Description != "parent\n\nchild\n" {
			t.Errorf("Description = %q, want %q", got.Description, "parent\n\nchild\n")
		}
	})

	t.Run("squashes several revisions at once", func(t *testing.T) {
		dir := abandonRepo(t)
		client := newClient(t, dir)
		parent := logEntry(t, client, "@--")
		child := logEntry(t, client, "@-")
		work := logEntry(t, client, "@")

		_, err := client.Squash(t.Context(), []jj.LogEntry{work, child}, parent)

		if err != nil {
			t.Fatalf("Squash() error = %v", err)
		}
		got := loadChange(t, client, jj.ChangeRevset(parent.ChangeID))
		if paths := diffPaths(t, client, got.CommitID); !slices.Equal(paths, []string{"a.txt", "b.txt", "c.txt"}) {
			t.Errorf("diff paths = %v, want [a.txt b.txt c.txt]", paths)
		}
		if got.Description != "parent\n\nwork\n\nchild\n" {
			t.Errorf("Description = %q, want %q", got.Description, "parent\n\nwork\n\nchild\n")
		}
	})

	t.Run("keeps the only non-empty description", func(t *testing.T) {
		dir := abandonRepo(t)
		runJJ(t, dir, "describe", "-r", "@--", "-m", "")
		client := newClient(t, dir)
		parent := logEntry(t, client, "@--")
		child := logEntry(t, client, "@-")

		if _, err := client.Squash(t.Context(), []jj.LogEntry{child}, parent); err != nil {
			t.Fatalf("Squash() error = %v", err)
		}

		if got := loadChange(t, client, jj.ChangeRevset(parent.ChangeID)); got.Description != "child\n" {
			t.Errorf("Description = %q, want %q", got.Description, "child\n")
		}
	})
}

func TestSquashWarnings_success(t *testing.T) {
	t.Run("reports the conflicts the squash creates", func(t *testing.T) {
		dir := newRepo(t)
		writeFile(t, dir, "f.txt", "one\n")
		commit(t, dir, "one")
		writeFile(t, dir, "f.txt", "two\n")
		commit(t, dir, "two")
		writeFile(t, dir, "f.txt", "three\n")
		runJJ(t, dir, "describe", "-m", "three")
		client := newClient(t, dir)

		warnings, err := client.Squash(t.Context(), []jj.LogEntry{logEntry(t, client, "@")}, logEntry(t, client, "@--"))

		if err != nil {
			t.Fatalf("Squash() error = %v", err)
		}
		if !strings.HasPrefix(warnings, "New conflicts appeared") {
			t.Errorf("Squash() warnings = %q, want the new conflicts reported", warnings)
		}
	})

	t.Run("a clean squash has no warnings", func(t *testing.T) {
		dir := abandonRepo(t)
		client := newClient(t, dir)

		warnings, err := client.Squash(t.Context(), []jj.LogEntry{logEntry(t, client, "@-")}, logEntry(t, client, "@--"))

		if err != nil || warnings != "" {
			t.Errorf("Squash() = %q, %v, want no warnings and no error", warnings, err)
		}
	})
}

func TestSquash_failed(t *testing.T) {
	t.Run("a source rewritten since it was listed", func(t *testing.T) {
		dir := abandonRepo(t)
		client := newClient(t, dir)
		parent := logEntry(t, client, "@--")
		child := logEntry(t, client, "@-")
		runJJ(t, dir, "describe", "-r", "@-", "-m", "reworded")

		_, err := client.Squash(t.Context(), []jj.LogEntry{child}, parent)

		if !errors.Is(err, jj.ErrHiddenRevision) {
			t.Errorf("Squash() error = %v, want %v", err, jj.ErrHiddenRevision)
		}
	})

	t.Run("an immutable destination", func(t *testing.T) {
		dir := abandonRepo(t)
		client := newClient(t, dir)
		root := logEntry(t, client, "root()")
		parent := logEntry(t, client, "@--")

		if _, err := client.Squash(t.Context(), []jj.LogEntry{parent}, root); err == nil {
			t.Error("Squash() error = nil, want an error")
		}
	})

	t.Run("no sources", func(t *testing.T) {
		dir := abandonRepo(t)
		client := newClient(t, dir)

		_, err := client.Squash(t.Context(), nil, logEntry(t, client, "@-"))

		if !errors.Is(err, jj.ErrNoSquashSources) {
			t.Errorf("Squash() error = %v, want %v", err, jj.ErrNoSquashSources)
		}
	})

	t.Run("the destination is a source", func(t *testing.T) {
		dir := abandonRepo(t)
		client := newClient(t, dir)
		child := logEntry(t, client, "@-")

		if _, err := client.Squash(t.Context(), []jj.LogEntry{child}, child); err == nil {
			t.Error("Squash() error = nil, want an error")
		}
	})
}
