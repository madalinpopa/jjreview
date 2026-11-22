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

func TestSplitLines_success(t *testing.T) {
	tests := map[string]struct {
		base    string
		work    string
		keys    []string
		rows    []int
		then    []string
		partial string
		wantOne string
	}{
		"only the added line of a replaced pair": {
			base: "a\nold\nz\n", work: "a\nnew\nz\n",
			keys: []string{"D"}, rows: []int{3},
			wantOne: "a\nold\nnew\nz\n",
		},
		"only the removed line of a replaced pair": {
			base: "a\nold\nz\n", work: "a\nnew\nz\n",
			keys: []string{"D"}, rows: []int{2},
			wantOne: "a\nz\n",
		},
		"only an added last line without a trailing newline": {
			base: "a\nb", work: "a\nb\nc",
			keys: []string{"D"}, rows: []int{4},
			wantOne: "a\nb\nc",
		},
		"only the removal of the last line without a trailing newline": {
			base: "a\nb", work: "a\nb\nc",
			keys: []string{"D"}, rows: []int{2},
			wantOne: "a\n",
		},
		"an added first line before a last line without a trailing newline": {
			base: "a\nb", work: "x\ny\na\nb",
			rows:    []int{1},
			wantOne: "x\na\nb",
		},
		"a line of a file with CRLF and LF endings": {
			base: "a\r\nold\r\nlf\nz\r\n", work: "a\r\nnew\r\nextra\r\nlf\nz\r\n",
			keys: []string{"D"}, rows: []int{4},
			wantOne: "a\r\nold\r\nextra\r\nlf\nz\r\n",
		},
		"one line of a new file": {
			work:    "one\ntwo\nthree\n",
			rows:    []int{2},
			wantOne: "two\n",
		},
		"a line then the rest of the hunk with space": {
			base: "a\nold\nz\n", work: "a\nnew\nz\n",
			keys: []string{"D"}, rows: []int{3}, then: []string{"space"},
			partial: "1 hunk selected (1 line)",
			wantOne: "a\nnew\nz\n",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			dir, client := newSplitRepo(t)
			path := filepath.Join(dir, "file.txt")
			if tt.base != "" {
				writeSplitFile(t, path, tt.base, 0o644)
			}
			runSplitJJ(t, dir, "new", "-m", "work")
			writeSplitFile(t, path, tt.work, 0o644)

			m := press(t, loaded(t, client), tt.keys...)
			for _, row := range tt.rows {
				m = pressAtRow(t, m, row, "s")
			}
			if tt.partial != "" {
				assertContains(t, view(m), tt.partial)
			}
			m = press(t, m, tt.then...)
			if tt.partial != "" {
				assertMissing(t, view(m), tt.partial)
			}
			commitSplit(t, m)

			assertSplitContent(t, dir, "@-", "file.txt", tt.wantOne)
			assertSplitContent(t, dir, "@", "file.txt", tt.work)
		})
	}
}

func TestSplitLines_successRepeated(t *testing.T) {
	dir, client := newSplitRepo(t)
	path := filepath.Join(dir, "file.txt")
	work := "a\none\ntwo\nthree\nz\n"
	writeSplitFile(t, path, "a\nz\n", 0o644)
	runSplitJJ(t, dir, "new", "-m", "work")
	writeSplitFile(t, path, work, 0o644)
	m := loaded(t, client)

	steps := []struct {
		row  int
		want string
	}{
		{row: 3, want: "a\ntwo\nz\n"},
		{row: 2, want: "a\none\ntwo\nz\n"},
		{row: 4, want: work},
	}
	for _, step := range steps {
		m = commitSplit(t, pressAtRow(t, m, step.row, "s"))

		assertSplitContent(t, dir, "@-", "file.txt", step.want)
		assertSplitContent(t, dir, "@", "file.txt", work)
	}

	assertSplitContent(t, dir, "@---", "file.txt", "a\ntwo\nz\n")
	assertSplitContent(t, dir, "@--", "file.txt", "a\none\ntwo\nz\n")
}

func TestSplitLines_successDeletedFile(t *testing.T) {
	dir, client := newSplitRepo(t)
	path := filepath.Join(dir, "gone.txt")
	writeSplitFile(t, path, "one\ntwo\nthree\n", 0o644)
	runSplitJJ(t, dir, "new", "-m", "work")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}

	commitSplit(t, pressAtRow(t, loaded(t, client), 2, "s"))

	assertSplitContent(t, dir, "@-", "gone.txt", "one\nthree\n")
	if got := runSplitJJ(t, dir, "file", "list", "-r", "@"); got != "" {
		t.Errorf("remainder files = %q, want none", got)
	}
}

func TestSplitLines_successRenamedFile(t *testing.T) {
	dir, client := newSplitRepo(t)
	oldPath := filepath.Join(dir, "old.txt")
	newPath := filepath.Join(dir, "new.txt")
	work := "l1\nL2\nl3\nadded\nl4\nl5\nl6\nl7\nl8\n"
	writeSplitFile(t, oldPath, "l1\nl2\nl3\nl4\nl5\nl6\nl7\nl8\n", 0o644)
	runSplitJJ(t, dir, "new", "-m", "work")
	if err := os.Remove(oldPath); err != nil {
		t.Fatal(err)
	}
	writeSplitFile(t, newPath, work, 0o644)

	m := press(t, loaded(t, client), "D")
	commitSplit(t, pressAtRow(t, m, 5, "s"))

	assertSplitContent(t, dir, "@-", "new.txt", "l1\nl2\nl3\nadded\nl4\nl5\nl6\nl7\nl8\n")
	if got := runSplitJJ(t, dir, "file", "list", "-r", "@-"); got != "new.txt\n" {
		t.Errorf("selected commit files = %q, want only new.txt", got)
	}
	assertSplitContent(t, dir, "@", "new.txt", work)
	if got := runSplitJJ(t, dir, "file", "list", "-r", "@"); got != "new.txt\n" {
		t.Errorf("remainder files = %q, want only new.txt", got)
	}
}

func TestSplitLines_successExecutableFile(t *testing.T) {
	dir, client := newSplitRepo(t)
	path := filepath.Join(dir, "script")
	work := "a\nnew\nextra\nz\n"
	writeSplitFile(t, path, "a\nold\nz\n", 0o755)
	runSplitJJ(t, dir, "new", "-m", "work")
	writeSplitFile(t, path, work, 0o755)

	m := press(t, loaded(t, client), "D")
	commitSplit(t, pressAtRow(t, m, 4, "s"))

	assertSplitContent(t, dir, "@-", "script", "a\nold\nextra\nz\n")
	assertSplitContent(t, dir, "@", "script", work)
	runSplitJJ(t, dir, "edit", "@-")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat(selected commit) error = %v", err)
	}
	if got := info.Mode().Perm(); got&0o100 == 0 {
		t.Errorf("selected commit script mode = %#o, want owner execute bit", got)
	}
}

func TestSplitLines_successRange(t *testing.T) {
	dir, client := newSplitRepo(t)
	path := filepath.Join(dir, "file.txt")
	work := "a\nB\nc\nd\ne\nf\ng\nh\ni\nj\nK\nl\n"
	writeSplitFile(t, path, "a\nb\nc\nd\ne\nf\ng\nh\ni\nj\nk\nl\n", 0o644)
	runSplitJJ(t, dir, "new", "-m", "work")
	writeSplitFile(t, path, work, 0o644)
	m := loaded(t, client)

	m = commitSplit(t, press(t, m, "g", "v", "G", "s"))

	assertSplitContent(t, dir, "@-", "file.txt", "a\nb\nB\nc\nd\ne\nf\ng\nh\ni\nj\nk\nK\nl\n")
	assertSplitContent(t, dir, "@", "file.txt", work)

	commitSplit(t, press(t, m, "g", "v", "G", "space"))

	assertSplitContent(t, dir, "@-", "file.txt", work)
	assertSplitContent(t, dir, "@", "file.txt", work)
	assertSplitContent(t, dir, "@--", "file.txt", "a\nb\nB\nc\nd\ne\nf\ng\nh\ni\nj\nk\nK\nl\n")
}

func pressAtRow(t *testing.T, m tui.Model, row int, key string) tui.Model {
	t.Helper()
	m = press(t, m, "g")
	for range row - m.Cursor() {
		m = press(t, m, "j")
	}
	if got := m.Cursor(); got != row {
		t.Fatalf("Cursor() = %d, want %d:\n%s", got, row, view(m))
	}
	return press(t, m, key)
}

func commitSplit(t *testing.T, m tui.Model) tui.Model {
	t.Helper()
	m = press(t, m, "c", "ctrl+s")
	if m.ModalOpen() {
		t.Fatalf("split did not complete:\n%s", view(m))
	}
	assertContains(t, view(m), "✓ created")
	return m
}

func assertSplitContent(t *testing.T, dir, rev, path, want string) {
	t.Helper()
	if got := runSplitJJ(t, dir, "file", "show", "-r", rev, path); got != want {
		t.Errorf("jj file show -r %s %s = %q, want %q", rev, path, got, want)
	}
}
