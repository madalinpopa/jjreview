package tui_test

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/madalinpopa/jjreview/internal/jj"
	"github.com/madalinpopa/jjreview/internal/splittool"
	"github.com/madalinpopa/jjreview/internal/tui"
)

func TestMain(m *testing.M) {
	if len(os.Args) == 5 && os.Args[1] == "apply-split" {
		if err := splittool.Run(os.Args[2], os.Args[3], os.Args[4]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestSplit_preservesUnselectedInstructions(t *testing.T) {
	dir, client := newSplitRepo(t)
	instructions := filepath.Join(dir, "JJ-INSTRUCTIONS")
	writeSplitFile(t, instructions, "original instructions\n", 0o644)
	runSplitJJ(t, dir, "new", "-m", "work")
	writeSplitFile(t, instructions, "modified instructions\n", 0o644)
	writeSplitFile(t, filepath.Join(dir, "keep.txt"), "selected content\n", 0o644)

	m := press(t, loaded(t, client), "J")
	splitSelectedFile(t, m, "keep.txt")

	if got := runSplitJJ(t, dir, "file", "show", "-r", "@-", "JJ-INSTRUCTIONS"); got != "original instructions\n" {
		t.Errorf("selected commit JJ-INSTRUCTIONS = %q, want original instructions", got)
	}
	if got := runSplitJJ(t, dir, "file", "show", "-r", "@", "JJ-INSTRUCTIONS"); got != "modified instructions\n" {
		t.Errorf("remainder JJ-INSTRUCTIONS = %q, want modified instructions", got)
	}
}

func TestSplit_preservesSelectedSymlinkTarget(t *testing.T) {
	dir, client := newSplitRepo(t)
	link := filepath.Join(dir, "link")
	if err := os.Symlink("old-target", link); err != nil {
		t.Fatal(err)
	}
	runSplitJJ(t, dir, "new", "-m", "work")
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("new-target", link); err != nil {
		t.Fatal(err)
	}
	writeSplitFile(t, filepath.Join(dir, "remaining.txt"), "unselected\n", 0o644)

	splitSelectedFile(t, loaded(t, client), "link")
	runSplitJJ(t, dir, "edit", "@-")

	got, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("Readlink(selected commit) error = %v", err)
	}
	if got != "new-target" {
		t.Errorf("Readlink(selected commit) = %q, want %q", got, "new-target")
	}
}

func TestSplit_preservesExecutableOnPureRename(t *testing.T) {
	dir, client := newSplitRepo(t)
	oldPath := filepath.Join(dir, "script-old")
	newPath := filepath.Join(dir, "script-new")
	writeSplitFile(t, oldPath, "printf test\n", 0o755)
	runSplitJJ(t, dir, "new", "-m", "work")
	if err := os.Rename(oldPath, newPath); err != nil {
		t.Fatal(err)
	}
	writeSplitFile(t, filepath.Join(dir, "z-remaining.txt"), "unselected\n", 0o644)

	splitSelectedFile(t, loaded(t, client), "script-new")
	runSplitJJ(t, dir, "edit", "@-")

	info, err := os.Stat(newPath)
	if err != nil {
		t.Fatalf("Stat(selected commit) error = %v", err)
	}
	if got := info.Mode().Perm(); got&0o100 == 0 {
		t.Errorf("selected commit script-new mode = %#o, want owner execute bit", got)
	}
}

func newSplitRepo(t *testing.T) (string, jj.Client) {
	t.Helper()
	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj not found on PATH")
	}
	config := filepath.Join(t.TempDir(), "config.toml")
	writeSplitFile(t, config, "", 0o600)
	t.Setenv("JJ_CONFIG", config)
	t.Setenv("JJ_USER", "Test User")
	t.Setenv("JJ_EMAIL", "test@example.com")
	dir := t.TempDir()
	runSplitJJ(t, dir, "git", "init", ".")
	tool, err := os.Executable()
	if err != nil {
		t.Fatalf("Executable() error = %v", err)
	}
	return dir, jj.New(dir, tool)
}

func runSplitJJ(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "jj", append([]string{"--no-pager", "--color", "never"}, args...)...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
		t.Fatalf("jj %v: %v: %s", args, err, exitErr.Stderr)
	}
	if err != nil {
		t.Fatalf("jj %v: %v", args, err)
	}
	return string(out)
}

func writeSplitFile(t *testing.T, path, text string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(text), mode); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}

func splitSelectedFile(t *testing.T, m tui.Model, path string) {
	t.Helper()
	if got := m.CurrentFile(); got != path {
		t.Fatalf("CurrentFile() = %q, want %q", got, path)
	}
	m = press(t, m, "S", "c", "ctrl+s")
	if m.ModalOpen() {
		t.Fatalf("split did not complete:\n%s", view(m))
	}
	assertContains(t, view(m), "✓ created")
}
