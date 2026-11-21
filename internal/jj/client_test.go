package jj_test

import (
	"encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/madalinpopa/jjreview/internal/jj"
)

const (
	applySplitEnv  = "JJREVIEW_TEST_APPLY_SPLIT"
	breakConfigEnv = "JJREVIEW_TEST_BREAK_CONFIG"
	realJJEnv      = "JJREVIEW_TEST_REAL_JJ"
)

func TestMain(m *testing.M) {
	if os.Getenv(applySplitEnv) == "1" {
		if err := applySplit(os.Args[1:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func applySplit(args []string) error {
	if len(args) != 4 || args[0] != "apply-split" {
		return fmt.Errorf("usage: apply-split <spec> <left> <right>, got %q", args)
	}
	specPath, left, right := args[1], args[2], args[3]
	data, err := os.ReadFile(specPath)
	if err != nil {
		return err
	}
	var files []jj.SplitFile
	if err := json.Unmarshal(data, &files); err != nil {
		return err
	}
	if err := os.RemoveAll(right); err != nil {
		return err
	}
	if err := os.CopyFS(right, os.DirFS(left)); err != nil {
		return err
	}
	for _, f := range files {
		target := filepath.Join(right, filepath.FromSlash(f.Path))
		if f.Delete {
			if err := os.Remove(target); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(target, []byte(f.Content), 0o644); err != nil {
			return err
		}
	}
	if os.Getenv(breakConfigEnv) == "1" {
		return os.WriteFile(os.Getenv("JJ_CONFIG"), []byte("not = [valid toml"), 0o600)
	}
	return nil
}

func newClient(t *testing.T, dir string) jj.Client {
	t.Helper()
	tool, err := os.Executable()
	if err != nil {
		t.Fatalf("test executable: %v", err)
	}
	return jj.New(dir, tool)
}

func newRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj not found on PATH")
	}
	configPath := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(configPath, nil, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("JJ_CONFIG", configPath)
	t.Setenv("JJ_USER", "Test User")
	t.Setenv("JJ_EMAIL", "test@example.com")
	t.Setenv(applySplitEnv, "1")
	dir := t.TempDir()
	runJJ(t, dir, "git", "init", ".")
	return dir
}

func runJJ(t *testing.T, dir string, args ...string) string {
	t.Helper()
	return runJJInput(t, dir, "", args...)
}

func runJJInput(t *testing.T, dir, stdin string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "jj", append([]string{"--no-pager", "--color", "never"}, args...)...)
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(stdin)
	out, err := cmd.Output()
	if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
		t.Fatalf("jj %v: %v: %s", args, err, exitErr.Stderr)
	}
	if err != nil {
		t.Fatalf("jj %v: %v", args, err)
	}
	return string(out)
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func commit(t *testing.T, dir, message string) {
	t.Helper()
	runJJ(t, dir, "describe", "-m", message)
	runJJ(t, dir, "new")
}

func TestChange_success(t *testing.T) {
	t.Run("marks immutable revisions", func(t *testing.T) {
		dir := newRepo(t)
		client := newClient(t, dir)

		root, err := client.Change(t.Context(), "root()")
		if err != nil {
			t.Fatalf("Change(root()) error = %v", err)
		}
		work, err := client.Change(t.Context(), "@")
		if err != nil {
			t.Fatalf("Change(@) error = %v", err)
		}

		if !root.Immutable {
			t.Error("Change(root()).Immutable = false, want true")
		}
		if work.Immutable {
			t.Error("Change(@).Immutable = true, want false")
		}
	})

	t.Run("marks the working-copy revision", func(t *testing.T) {
		dir := newRepo(t)
		client := newClient(t, dir)

		work, err := client.Change(t.Context(), "@")
		if err != nil {
			t.Fatalf("Change(@) error = %v", err)
		}
		root, err := client.Change(t.Context(), "root()")
		if err != nil {
			t.Fatalf("Change(root()) error = %v", err)
		}

		if !work.WorkingCopy {
			t.Error("Change(@).WorkingCopy = false, want true")
		}
		if root.WorkingCopy {
			t.Error("Change(root()).WorkingCopy = true, want false")
		}
	})

	t.Run("fills every field", func(t *testing.T) {
		dir := newRepo(t)
		writeFile(t, dir, "a.txt", "hello\n")
		commit(t, dir, "subject\n\nbody line\n")
		wantChangeID := runJJ(t, dir, "log", "-r", "@-", "--no-graph", "-T", "change_id")
		wantCommitID := runJJ(t, dir, "log", "-r", "@-", "--no-graph", "-T", "commit_id")
		wantParent := runJJ(t, dir, "log", "-r", "@--", "--no-graph", "-T", "commit_id")

		got, err := newClient(t, dir).Change(t.Context(), "@-")
		if err != nil {
			t.Fatalf("Change() error = %v", err)
		}

		want := jj.Change{
			ChangeID:    wantChangeID,
			CommitID:    wantCommitID,
			Parents:     []string{wantParent},
			Author:      "Test User",
			Description: "subject\n\nbody line\n",
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("Change() = %+v, want %+v", got, want)
		}
	})

	t.Run("author falls back to email", func(t *testing.T) {
		dir := newRepo(t)
		t.Setenv("JJ_USER", "")
		runJJ(t, dir, "new")

		got, err := newClient(t, dir).Change(t.Context(), "@")
		if err != nil {
			t.Fatalf("Change() error = %v", err)
		}

		if got.Author != "test@example.com" {
			t.Errorf("Change() Author = %q, want %q", got.Author, "test@example.com")
		}
	})

	t.Run("description with NUL byte", func(t *testing.T) {
		dir := newRepo(t)
		runJJInput(t, dir, "before\x00after\n", "describe", "--stdin")

		got, err := newClient(t, dir).Change(t.Context(), "@")
		if err != nil {
			t.Fatalf("Change() error = %v", err)
		}

		if want := "before\x00after\n"; got.Description != want {
			t.Errorf("Change() Description = %q, want %q", got.Description, want)
		}
	})
}

func TestChange_failed(t *testing.T) {
	dir := newRepo(t)
	commit(t, dir, "first")

	tests := []struct {
		name     string
		rev      string
		wantIs   error
		wantText string
	}{
		{name: "unknown revision", rev: "doesnotexist", wantText: "doesnotexist"},
		{name: "multiple commits", rev: "@ | @-", wantIs: jj.ErrMultipleRevisions},
		{name: "no commits", rev: "none()", wantIs: jj.ErrNoRevision},
		{name: "flag-like revision", rev: "--help", wantText: "jj log"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := newClient(t, dir).Change(t.Context(), tt.rev)
			if err == nil {
				t.Fatalf("Change(%q) error = nil, want error", tt.rev)
			}
			if tt.wantIs != nil && !errors.Is(err, tt.wantIs) {
				t.Errorf("Change(%q) error = %v, want %v", tt.rev, err, tt.wantIs)
			}
			if !strings.Contains(err.Error(), tt.wantText) {
				t.Errorf("Change(%q) error = %q, want it to contain %q", tt.rev, err, tt.wantText)
			}
		})
	}
}

func TestFileContent_success(t *testing.T) {
	dir := newRepo(t)
	files := map[string]string{
		"plain.txt":              "plain\n",
		"with space.txt":         "spaced\n",
		`quote " and \ back.txt`: "quoted\n",
	}
	for name, content := range files {
		writeFile(t, dir, name, content)
	}
	commit(t, dir, "files")
	writeFile(t, dir, "plain.txt", "changed\n")

	client := newClient(t, dir)
	for name, want := range files {
		t.Run(name, func(t *testing.T) {
			got, err := client.FileContent(t.Context(), commitID(t, dir, "@-"), name)
			if err != nil {
				t.Fatalf("FileContent(%q) error = %v", name, err)
			}
			if got != want {
				t.Errorf("FileContent(%q) = %q, want %q", name, got, want)
			}
		})
	}

	t.Run("resolves paths from the workspace root", func(t *testing.T) {
		dir := newRepo(t)
		sub := filepath.Join(dir, "sub")
		if err := os.Mkdir(sub, 0o700); err != nil {
			t.Fatalf("mkdir sub: %v", err)
		}
		writeFile(t, dir, "x.txt", "root\n")
		writeFile(t, sub, "x.txt", "nested\n")
		commit(t, dir, "files")

		got, err := newClient(t, sub).FileContent(t.Context(), commitID(t, dir, "@-"), "x.txt")
		if err != nil {
			t.Fatalf("FileContent(%q) error = %v", "x.txt", err)
		}
		if got != "root\n" {
			t.Errorf("FileContent(%q) = %q, want %q", "x.txt", got, "root\n")
		}
	})
}

func TestFileContent_failed(t *testing.T) {
	dir := newRepo(t)
	commit(t, dir, "empty")

	_, err := newClient(t, dir).FileContent(t.Context(), commitID(t, dir, "@-"), "missing.txt")
	if err == nil {
		t.Fatal("FileContent() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "file show") || !strings.Contains(err.Error(), "missing.txt") {
		t.Errorf("FileContent() error = %q, want it to name the command and path", err)
	}
}

func TestDiff_success(t *testing.T) {
	dir := newRepo(t)
	writeFile(t, dir, "a.txt", "a\n")
	writeFile(t, dir, "b.txt", "b\n")
	commit(t, dir, "files")

	files, err := newClient(t, dir).Diff(t.Context(), commitID(t, dir, "@-"))
	if err != nil {
		t.Fatalf("Diff() error = %v", err)
	}

	var got []string
	for _, f := range files {
		got = append(got, f.Path())
	}
	slices.Sort(got)
	if want := []string{"a.txt", "b.txt"}; !slices.Equal(got, want) {
		t.Errorf("Diff() paths = %v, want %v", got, want)
	}
}

func TestDiff_failed(t *testing.T) {
	dir := newRepo(t)
	commit(t, dir, "first")
	unknown := "0123456789abcdef0123456789abcdef01234567"

	_, err := newClient(t, dir).Diff(t.Context(), unknown)

	if !errors.Is(err, jj.ErrNoRevision) {
		t.Errorf("Diff(%q) error = %v, want %v", unknown, err, jj.ErrNoRevision)
	}
}

func TestShadowedIDs_success(t *testing.T) {
	dir := newRepo(t)
	writeFile(t, dir, "a.txt", "a\n")
	commit(t, dir, "target")
	target := commitID(t, dir, "@-")
	runJJ(t, dir, "config", "set", "--repo", `revset-aliases."`+target+`"`, "root()")
	client := newClient(t, dir)

	t.Run("Change of a commit revset", func(t *testing.T) {
		got, err := client.Change(t.Context(), jj.CommitRevset(target))
		if err != nil {
			t.Fatalf("Change(CommitRevset(%q)) error = %v", target, err)
		}
		if got.CommitID != target {
			t.Errorf("Change(CommitRevset(%q)) CommitID = %q, want %q", target, got.CommitID, target)
		}
	})

	t.Run("Diff", func(t *testing.T) {
		if got := diffPaths(t, client, target); !slices.Equal(got, []string{"a.txt"}) {
			t.Errorf("Diff(%q) paths = %v, want [a.txt]", target, got)
		}
	})

	t.Run("FileContent", func(t *testing.T) {
		if got := fileContent(t, client, target, "a.txt"); got != "a\n" {
			t.Errorf("FileContent(%q, a.txt) = %q, want %q", target, got, "a\n")
		}
	})
}

func TestSplit_successWhenShadowed(t *testing.T) {
	dir := splitRepo(t)
	client := newClient(t, dir)
	original, err := client.Change(t.Context(), "@")
	if err != nil {
		t.Fatalf("Change(@) error = %v", err)
	}
	runJJ(t, dir, "config", "set", "--repo", `revset-aliases."`+original.ChangeID+`"`, "root()")

	selected, rest, err := client.Split(t.Context(), original.ChangeID, original.CommitID, "selected", []jj.SplitFile{{Path: "b.txt", Content: "b2\n"}})
	if err != nil {
		t.Fatalf("Split() error = %v", err)
	}

	if selected.ChangeID != original.ChangeID || selected.Description != "selected\n" {
		t.Errorf("Split() selected = %s %q, want %s %q", selected.ChangeID, selected.Description, original.ChangeID, "selected\n")
	}
	if !slices.Equal(rest.Parents, []string{selected.CommitID}) {
		t.Errorf("Split() rest Parents = %v, want [%s]", rest.Parents, selected.CommitID)
	}
}

func diffPaths(t *testing.T, client jj.Client, rev string) []string {
	t.Helper()
	files, err := client.Diff(t.Context(), rev)
	if err != nil {
		t.Fatalf("Diff(%q) error = %v", rev, err)
	}
	var paths []string
	for _, f := range files {
		paths = append(paths, f.Path())
	}
	slices.Sort(paths)
	return paths
}

func fileContent(t *testing.T, client jj.Client, rev, path string) string {
	t.Helper()
	got, err := client.FileContent(t.Context(), rev, path)
	if err != nil {
		t.Fatalf("FileContent(%q, %q) error = %v", rev, path, err)
	}
	return got
}

func splitRepo(t *testing.T) string {
	t.Helper()
	dir := newRepo(t)
	writeFile(t, dir, "a.txt", "1\n2\n3\n")
	writeFile(t, dir, "b.txt", "b\n")
	writeFile(t, dir, "c.txt", "c\n")
	commit(t, dir, "base")
	writeFile(t, dir, "a.txt", "1x\n2\n3x\n")
	writeFile(t, dir, "b.txt", "b2\n")
	if err := os.Remove(filepath.Join(dir, "c.txt")); err != nil {
		t.Fatalf("remove c.txt: %v", err)
	}
	runJJ(t, dir, "describe", "-m", "work")
	return dir
}

func TestSplit_success(t *testing.T) {
	tests := []struct {
		name          string
		files         []jj.SplitFile
		wantSelected  []string
		wantRemaining []string
		wantContent   map[string]string
	}{
		{
			name:          "one of two modified files",
			files:         []jj.SplitFile{{Path: "b.txt", Content: "b2\n"}},
			wantSelected:  []string{"b.txt"},
			wantRemaining: []string{"a.txt", "c.txt"},
			wantContent:   map[string]string{"b.txt": "b2\n", "a.txt": "1\n2\n3\n"},
		},
		{
			name:          "partial file content",
			files:         []jj.SplitFile{{Path: "a.txt", Content: "1x\n2\n3\n"}},
			wantSelected:  []string{"a.txt"},
			wantRemaining: []string{"a.txt", "b.txt", "c.txt"},
			wantContent:   map[string]string{"a.txt": "1x\n2\n3\n", "b.txt": "b\n"},
		},
		{
			name:          "invalid UTF-8 content",
			files:         []jj.SplitFile{{Path: "b.txt", Content: "\x89PNG\r\n\x1a\n\xff\xfe\x00\xff"}},
			wantSelected:  []string{"b.txt"},
			wantRemaining: []string{"a.txt", "b.txt", "c.txt"},
			wantContent:   map[string]string{"b.txt": "\x89PNG\r\n\x1a\n\xff\xfe\x00\xff"},
		},
		{
			name:          "delete",
			files:         []jj.SplitFile{{Path: "c.txt", Delete: true}},
			wantSelected:  []string{"c.txt"},
			wantRemaining: []string{"a.txt", "b.txt"},
			wantContent:   map[string]string{"a.txt": "1\n2\n3\n"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, workingCopy := range []bool{false, true} {
				t.Run(fmt.Sprintf("working copy %v", workingCopy), func(t *testing.T) {
					dir := splitRepo(t)
					if !workingCopy {
						runJJ(t, dir, "new")
					}
					client := newClient(t, dir)
					rev := "@-"
					if workingCopy {
						rev = "@"
					}
					original, err := client.Change(t.Context(), rev)
					if err != nil {
						t.Fatalf("Change(%q) error = %v", rev, err)
					}

					selected, remaining, err := client.Split(t.Context(), original.ChangeID, original.CommitID, "selected part", tt.files)
					if err != nil {
						t.Fatalf("Split() error = %v", err)
					}

					if selected.Description != "selected part\n" {
						t.Errorf("Split() Description = %q, want %q", selected.Description, "selected part\n")
					}
					if selected.ChangeID != original.ChangeID {
						t.Errorf("selected ChangeID = %s, want original %s", selected.ChangeID, original.ChangeID)
					}
					if remaining.ChangeID == original.ChangeID {
						t.Errorf("remaining ChangeID = %s, want a new change ID", remaining.ChangeID)
					}
					if remaining.Description != "work\n" {
						t.Errorf("remaining Description = %q, want %q", remaining.Description, "work\n")
					}
					if !slices.Equal(remaining.Parents, []string{selected.CommitID}) {
						t.Errorf("remaining Parents = %v, want [%s]", remaining.Parents, selected.CommitID)
					}
					if got := diffPaths(t, client, selected.CommitID); !slices.Equal(got, tt.wantSelected) {
						t.Errorf("selected diff paths = %v, want %v", got, tt.wantSelected)
					}
					if got := diffPaths(t, client, remaining.CommitID); !slices.Equal(got, tt.wantRemaining) {
						t.Errorf("remaining diff paths = %v, want %v", got, tt.wantRemaining)
					}
					for path, want := range tt.wantContent {
						if got := fileContent(t, client, selected.CommitID, path); got != want {
							t.Errorf("selected %s = %q, want %q", path, got, want)
						}
					}
					if got := fileContent(t, client, remaining.CommitID, "a.txt"); got != "1x\n2\n3x\n" {
						t.Errorf("remaining a.txt = %q, want %q", got, "1x\n2\n3x\n")
					}
					if workingCopy {
						current, err := client.Change(t.Context(), "@")
						if err != nil {
							t.Fatalf("Change(@) error = %v", err)
						}
						if current.CommitID != remaining.CommitID {
							t.Errorf("working copy = %s, want remaining %s", current.CommitID, remaining.CommitID)
						}
					}
				})
			}
		})
	}
}

func TestSplit_failed(t *testing.T) {
	dir := splitRepo(t)
	client := newClient(t, dir)
	before, err := client.Change(t.Context(), "@")
	if err != nil {
		t.Fatalf("Change(@) error = %v", err)
	}

	_, _, err = client.Split(t.Context(), before.ChangeID, before.CommitID, "selected", nil)
	if !errors.Is(err, jj.ErrNoSplitFiles) {
		t.Errorf("Split() error = %v, want %v", err, jj.ErrNoSplitFiles)
	}

	after, err := client.Change(t.Context(), "@")
	if err != nil {
		t.Fatalf("Change(@) error = %v", err)
	}
	if after.CommitID != before.CommitID {
		t.Errorf("Split() changed @ from %s to %s", before.CommitID, after.CommitID)
	}
}

func TestSplit_failedStale(t *testing.T) {
	dir := splitRepo(t)
	client := newClient(t, dir)
	loaded, err := client.Change(t.Context(), "@")
	if err != nil {
		t.Fatalf("Change(@) error = %v", err)
	}
	writeFile(t, dir, "b.txt", "b3\n")

	_, _, err = client.Split(t.Context(), loaded.ChangeID, loaded.CommitID, "selected", []jj.SplitFile{{Path: "b.txt", Content: "b2\n"}})

	if !errors.Is(err, jj.ErrStaleRevision) {
		t.Fatalf("Split() error = %v, want %v", err, jj.ErrStaleRevision)
	}
	current, err := client.Change(t.Context(), "@")
	if err != nil {
		t.Fatalf("Change(@) error = %v", err)
	}
	if current.ChangeID != loaded.ChangeID || current.Description != "work\n" {
		t.Errorf("Change(@) = %s %q, want unsplit %s %q", current.ChangeID, current.Description, loaded.ChangeID, "work\n")
	}
	if got := fileContent(t, client, current.CommitID, "b.txt"); got != "b3\n" {
		t.Errorf("b.txt = %q, want %q", got, "b3\n")
	}
}

func TestSplit_successTwiceOnRest(t *testing.T) {
	dir := splitRepo(t)
	runJJ(t, dir, "new")
	client := newClient(t, dir)
	original, err := client.Change(t.Context(), "@-")
	if err != nil {
		t.Fatalf("Change(@-) error = %v", err)
	}

	first, rest, err := client.Split(t.Context(), original.ChangeID, original.CommitID, "first", []jj.SplitFile{{Path: "b.txt", Content: "b2\n"}})
	if err != nil {
		t.Fatalf("first Split() error = %v", err)
	}
	second, last, err := client.Split(t.Context(), rest.ChangeID, rest.CommitID, "second", []jj.SplitFile{{Path: "c.txt", Delete: true}})
	if err != nil {
		t.Fatalf("second Split() error = %v", err)
	}

	if second.ChangeID != rest.ChangeID {
		t.Errorf("second selected ChangeID = %s, want rest %s", second.ChangeID, rest.ChangeID)
	}
	if !slices.Equal(second.Parents, []string{first.CommitID}) {
		t.Errorf("second Parents = %v, want [%s]", second.Parents, first.CommitID)
	}
	if !slices.Equal(last.Parents, []string{second.CommitID}) {
		t.Errorf("last Parents = %v, want [%s]", last.Parents, second.CommitID)
	}
	if last.Description != "work\n" {
		t.Errorf("last Description = %q, want %q", last.Description, "work\n")
	}
	for rev, want := range map[string][]string{
		first.CommitID:  {"b.txt"},
		second.CommitID: {"c.txt"},
		last.CommitID:   {"a.txt"},
	} {
		if got := diffPaths(t, client, rev); !slices.Equal(got, want) {
			t.Errorf("diff paths of %s = %v, want %v", rev, got, want)
		}
	}
}

func TestSplit_failedAfterSplit(t *testing.T) {
	dir := splitRepo(t)
	client := newClient(t, dir)
	original, err := client.Change(t.Context(), "@")
	if err != nil {
		t.Fatalf("Change(@) error = %v", err)
	}
	t.Setenv(breakConfigEnv, "1")

	selected, rest, err := client.Split(t.Context(), original.ChangeID, original.CommitID, "selected", []jj.SplitFile{{Path: "b.txt", Content: "b2\n"}})

	if !errors.Is(err, jj.ErrSplitIncomplete) {
		t.Fatalf("Split() error = %v, want %v", err, jj.ErrSplitIncomplete)
	}
	if selected.ChangeID != original.ChangeID {
		t.Errorf("Split() selected ChangeID = %q, want %q", selected.ChangeID, original.ChangeID)
	}
	if rest.ChangeID != "" {
		t.Errorf("Split() rest ChangeID = %q, want empty", rest.ChangeID)
	}
	if err := os.WriteFile(os.Getenv("JJ_CONFIG"), nil, 0o600); err != nil {
		t.Fatalf("restore config: %v", err)
	}
	t.Setenv(breakConfigEnv, "")
	if got := diffPaths(t, client, commitID(t, dir, original.ChangeID)); !slices.Equal(got, []string{"b.txt"}) {
		t.Errorf("selected diff paths = %v, want [b.txt]", got)
	}
}

func TestWorkspaceRoot_success(t *testing.T) {
	dir := newRepo(t)
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	want, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatalf("EvalSymlinks() error = %v", err)
	}

	got, err := newClient(t, sub).WorkspaceRoot(t.Context())
	if err != nil {
		t.Fatalf("WorkspaceRoot() error = %v", err)
	}

	resolved, err := filepath.EvalSymlinks(got)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q) error = %v", got, err)
	}
	if resolved != want {
		t.Errorf("WorkspaceRoot() = %q, want %q", got, want)
	}
}

func TestWorkingCopyRevset_success(t *testing.T) {
	dir := newRepo(t)
	writeFile(t, dir, "a.txt", "a\n")
	runJJ(t, dir, "describe", "-m", "work")
	client := newClient(t, dir)
	wc, err := client.Change(t.Context(), "@")
	if err != nil {
		t.Fatalf("Change(@) error = %v", err)
	}
	runJJ(t, dir, "config", "set", "--repo", `revset-aliases."`+wc.ChangeID+`"`, "root()")

	got, err := client.Change(t.Context(), jj.WorkingCopyRevset(wc.ChangeID))
	if err != nil {
		t.Fatalf("Change(WorkingCopyRevset(%q)) error = %v", wc.ChangeID, err)
	}

	if got.CommitID != wc.CommitID {
		t.Errorf("Change(WorkingCopyRevset(%q)) CommitID = %q, want %q", wc.ChangeID, got.CommitID, wc.CommitID)
	}
}

func TestWorkingCopyRevset_failed(t *testing.T) {
	dir := newRepo(t)
	writeFile(t, dir, "a.txt", "a\n")
	runJJ(t, dir, "describe", "-m", "work")
	client := newClient(t, dir)
	wc, err := client.Change(t.Context(), "@")
	if err != nil {
		t.Fatalf("Change(@) error = %v", err)
	}
	runJJ(t, dir, "new")

	_, err = client.Change(t.Context(), jj.WorkingCopyRevset(wc.ChangeID))

	if !errors.Is(err, jj.ErrNoRevision) {
		t.Errorf("Change(WorkingCopyRevset(%q)) error = %v, want %v", wc.ChangeID, err, jj.ErrNoRevision)
	}
}

func beforeSplit(t *testing.T, script string) {
	t.Helper()
	beforeCommand(t, "split", script)
}

func beforeCommand(t *testing.T, subcommand, script string) {
	t.Helper()
	jjPath, err := exec.LookPath("jj")
	if err != nil {
		t.Fatalf("LookPath(jj) error = %v", err)
	}
	bin := t.TempDir()
	wrapper := "#!/bin/sh\nfor arg; do\n\tif [ \"$arg\" = " + subcommand + " ]; then\n\t\t" + script + "\n\t\tbreak\n\tfi\ndone\nexec \"$" + realJJEnv + "\" \"$@\"\n"
	if err := os.WriteFile(filepath.Join(bin, "jj"), []byte(wrapper), 0o700); err != nil {
		t.Fatalf("write jj wrapper: %v", err)
	}
	t.Setenv(realJJEnv, jjPath)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestSplit_failedChangedBeforeSplit(t *testing.T) {
	tests := map[string]struct {
		rev    string
		script func(changeID string) string
	}{
		"working copy snapshot": {
			rev:    "@",
			script: func(string) string { return `printf 'b3\n' > b.txt` },
		},
		"rewritten parent": {
			rev: "@-",
			script: func(changeID string) string {
				return `"$` + realJJEnv + `" describe -r 'change_id("` + changeID + `")' -m rewritten >/dev/null 2>&1`
			},
		},
		"abandoned parent": {
			rev: "@-",
			script: func(changeID string) string {
				return `"$` + realJJEnv + `" abandon -r 'change_id("` + changeID + `")' >/dev/null 2>&1`
			},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			dir := splitRepo(t)
			if tt.rev == "@-" {
				runJJ(t, dir, "new")
			}
			client := newClient(t, dir)
			loaded, err := client.Change(t.Context(), tt.rev)
			if err != nil {
				t.Fatalf("Change(%q) error = %v", tt.rev, err)
			}
			beforeSplit(t, tt.script(loaded.ChangeID))

			_, _, err = client.Split(t.Context(), loaded.ChangeID, loaded.CommitID, "selected", []jj.SplitFile{{Path: "b.txt", Content: "b2\n"}})

			if !errors.Is(err, jj.ErrStaleRevision) {
				t.Fatalf("Split() error = %v, want %v", err, jj.ErrStaleRevision)
			}
			if _, ok := errors.AsType[*exec.ExitError](err); !ok || !strings.Contains(err.Error(), "jj split:") {
				t.Errorf("Split() error = %v, want the jj split failure", err)
			}
			if _, err := client.Change(t.Context(), `description(substring:"selected")`); !errors.Is(err, jj.ErrNoRevision) {
				t.Errorf("Change(selected) error = %v, want %v", err, jj.ErrNoRevision)
			}
		})
	}
}

func TestSplit_failedDivergedBeforeSplit(t *testing.T) {
	dir := splitRepo(t)
	runJJ(t, dir, "new")
	op := runJJ(t, dir, "operation", "log", "--ignore-working-copy", "--no-graph", "-n1", "-T", "id")
	runJJ(t, dir, "describe", "-r", "@-", "-m", "loaded")
	client := newClient(t, dir)
	loaded, err := client.Change(t.Context(), "@-")
	if err != nil {
		t.Fatalf("Change(@-) error = %v", err)
	}
	beforeSplit(t, `"$`+realJJEnv+`" --at-op `+op+` describe -r 'change_id("`+loaded.ChangeID+`")' -m sibling >/dev/null 2>&1`)

	_, _, err = client.Split(t.Context(), loaded.ChangeID, loaded.CommitID, "selected", []jj.SplitFile{{Path: "b.txt", Content: "b2\n"}})

	if !errors.Is(err, jj.ErrStaleRevision) {
		t.Fatalf("Split() error = %v, want %v", err, jj.ErrStaleRevision)
	}
	if _, err := client.Change(t.Context(), `description(substring:"selected")`); !errors.Is(err, jj.ErrNoRevision) {
		t.Errorf("Change(selected) error = %v, want %v", err, jj.ErrNoRevision)
	}
	if _, err := client.Change(t.Context(), `change_id("`+loaded.ChangeID+`")`); !errors.Is(err, jj.ErrMultipleRevisions) {
		t.Errorf("Change(%s) error = %v, want the change divergent", loaded.ChangeID, err)
	}
}

func TestSplit_failedAfterPublishing(t *testing.T) {
	dir := splitRepo(t)
	client := newClient(t, dir)
	loaded, err := client.Change(t.Context(), "@")
	if err != nil {
		t.Fatalf("Change(@) error = %v", err)
	}
	beforeSplit(t, `"$`+realJJEnv+`" "$@" >/dev/null 2>&1; echo 'checkout failed' >&2; exit 1`)

	selected, _, err := client.Split(t.Context(), loaded.ChangeID, loaded.CommitID, "selected", []jj.SplitFile{{Path: "b.txt", Content: "b2\n"}})

	if !errors.Is(err, jj.ErrSplitIncomplete) {
		t.Fatalf("Split() error = %v, want %v", err, jj.ErrSplitIncomplete)
	}
	if errors.Is(err, jj.ErrStaleRevision) {
		t.Errorf("Split() error = %v, want it not stale", err)
	}
	if !strings.Contains(err.Error(), "checkout failed") {
		t.Errorf("Split() error = %v, want the jj split failure", err)
	}
	if selected.ChangeID != loaded.ChangeID {
		t.Errorf("Split() selected ChangeID = %q, want %q", selected.ChangeID, loaded.ChangeID)
	}
}

func TestSplit_failedGoneBeforeLookup(t *testing.T) {
	tests := map[string]struct {
		change  func(t *testing.T, dir, op string, loaded jj.Change)
		wantErr error
	}{
		"abandoned": {
			change: func(t *testing.T, dir, _ string, loaded jj.Change) {
				t.Helper()
				runJJ(t, dir, "abandon", "-r", `change_id("`+loaded.ChangeID+`")`)
			},
			wantErr: jj.ErrNoRevision,
		},
		"divergent": {
			change: func(t *testing.T, dir, op string, loaded jj.Change) {
				t.Helper()
				runJJ(t, dir, "--at-op", op, "describe", "-r", `change_id("`+loaded.ChangeID+`")`, "-m", "sibling")
			},
			wantErr: jj.ErrMultipleRevisions,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			dir := splitRepo(t)
			runJJ(t, dir, "new")
			op := runJJ(t, dir, "operation", "log", "--ignore-working-copy", "--no-graph", "-n1", "-T", "id")
			runJJ(t, dir, "describe", "-r", "@-", "-m", "loaded")
			client := newClient(t, dir)
			loaded, err := client.Change(t.Context(), "@-")
			if err != nil {
				t.Fatalf("Change(@-) error = %v", err)
			}
			tt.change(t, dir, op, loaded)

			_, _, err = client.Split(t.Context(), loaded.ChangeID, loaded.CommitID, "selected", []jj.SplitFile{{Path: "b.txt", Content: "b2\n"}})

			if !errors.Is(err, jj.ErrStaleRevision) || !errors.Is(err, tt.wantErr) {
				t.Errorf("Split() error = %v, want %v and %v", err, jj.ErrStaleRevision, tt.wantErr)
			}
		})
	}
}

func TestSplit_failedAfterPublishingWrapped(t *testing.T) {
	dir := splitRepo(t)
	if err := os.WriteFile(os.Getenv("JJ_CONFIG"), []byte("ui.log-word-wrap = true\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("COLUMNS", "20")
	client := newClient(t, dir)
	loaded, err := client.Change(t.Context(), "@")
	if err != nil {
		t.Fatalf("Change(@) error = %v", err)
	}
	beforeSplit(t, `"$`+realJJEnv+`" "$@" >/dev/null 2>&1; echo 'checkout failed' >&2; exit 1`)

	_, _, err = client.Split(t.Context(), loaded.ChangeID, loaded.CommitID, "selected", []jj.SplitFile{{Path: "b.txt", Content: "b2\n"}})

	if !errors.Is(err, jj.ErrSplitIncomplete) {
		t.Errorf("Split() error = %v, want %v", err, jj.ErrSplitIncomplete)
	}
}

func TestSplit_failedAfterAnotherSplit(t *testing.T) {
	dir := splitRepo(t)
	client := newClient(t, dir)
	loaded, err := client.Change(t.Context(), "@")
	if err != nil {
		t.Fatalf("Change(@) error = %v", err)
	}
	beforeSplit(t, `"$`+realJJEnv+`" split -r 'commit_id("`+loaded.CommitID+`")' -m other a.txt >/dev/null 2>&1`)

	_, _, err = client.Split(t.Context(), loaded.ChangeID, loaded.CommitID, "selected", []jj.SplitFile{{Path: "b.txt", Content: "b2\n"}})

	if errors.Is(err, jj.ErrSplitIncomplete) {
		t.Errorf("Split() error = %v, want the other split not taken for ours", err)
	}
	if !errors.Is(err, jj.ErrStaleRevision) {
		t.Errorf("Split() error = %v, want %v", err, jj.ErrStaleRevision)
	}
	if _, err := client.Change(t.Context(), `description(substring:"other")`); err != nil {
		t.Errorf("Change(other) error = %v, want the other split kept", err)
	}
}

func TestSplit_failedAfterSplitNamingOurSpec(t *testing.T) {
	dir := splitRepo(t)
	client := newClient(t, dir)
	loaded, err := client.Change(t.Context(), "@")
	if err != nil {
		t.Fatalf("Change(@) error = %v", err)
	}
	beforeSplit(t, `for a; do case $a in *edit-args=*) cfg=$a;; esac; done
		"$`+realJJEnv+`" split -r 'commit_id("`+loaded.CommitID+`")' -m "other $cfg" a.txt >/dev/null 2>&1; exit 1`)

	_, _, err = client.Split(t.Context(), loaded.ChangeID, loaded.CommitID, "selected", []jj.SplitFile{{Path: "b.txt", Content: "b2\n"}})

	if errors.Is(err, jj.ErrSplitIncomplete) {
		t.Errorf("Split() error = %v, want the other split not taken for ours", err)
	}
	if !errors.Is(err, jj.ErrStaleRevision) {
		t.Errorf("Split() error = %v, want %v", err, jj.ErrStaleRevision)
	}
}

func TestSplit_failedAfterPublishingQuotedSpec(t *testing.T) {
	dir := splitRepo(t)
	tmp := filepath.Join(t.TempDir(), "it's a dir")
	if err := os.Mkdir(tmp, 0o700); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	t.Setenv("TMPDIR", tmp)
	client := newClient(t, dir)
	loaded, err := client.Change(t.Context(), "@")
	if err != nil {
		t.Fatalf("Change(@) error = %v", err)
	}
	beforeSplit(t, `"$`+realJJEnv+`" "$@" >/dev/null 2>&1; echo 'checkout failed' >&2; exit 1`)

	_, _, err = client.Split(t.Context(), loaded.ChangeID, loaded.CommitID, "selected", []jj.SplitFile{{Path: "b.txt", Content: "b2\n"}})

	if !errors.Is(err, jj.ErrSplitIncomplete) {
		t.Errorf("Split() error = %v, want %v", err, jj.ErrSplitIncomplete)
	}
}
