package splittool_test

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/madalinpopa/jjreview/internal/jj"
	"github.com/madalinpopa/jjreview/internal/splittool"
)

func writeFile(tb testing.TB, path, content string, perm os.FileMode) {
	tb.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		tb.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), perm); err != nil {
		tb.Fatalf("WriteFile(%q) error = %v", path, err)
	}
	if err := os.Chmod(path, perm); err != nil {
		tb.Fatalf("Chmod(%q) error = %v", path, err)
	}
}

func writeSpec(tb testing.TB, files []jj.SplitFile) string {
	tb.Helper()
	data, err := json.Marshal(files)
	if err != nil {
		tb.Fatalf("Marshal() error = %v", err)
	}
	path := filepath.Join(tb.TempDir(), "spec.json")
	writeFile(tb, path, string(data), 0o644)
	return path
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	return string(data)
}

func assertMissing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Lstat(%q) error = %v, want %v", path, err, fs.ErrNotExist)
	}
}

func assertPerm(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("Lstat(%q) error = %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Errorf("Lstat(%q) perm = %v, want %v", path, got, want)
	}
}

func TestRun_success(t *testing.T) {
	t.Run("syncs right to left with an empty spec", func(t *testing.T) {
		left, right := t.TempDir(), t.TempDir()
		writeFile(t, filepath.Join(left, "a.txt"), "old a\n", 0o444)
		writeFile(t, filepath.Join(left, "d", "e", "b.txt"), "old b\n", 0o444)
		writeFile(t, filepath.Join(right, "a.txt"), "new a\n", 0o644)
		writeFile(t, filepath.Join(right, "extra.txt"), "extra\n", 0o644)
		writeFile(t, filepath.Join(right, "x", "y", "z.txt"), "z\n", 0o644)
		spec := writeSpec(t, nil)

		err := splittool.Run(spec, left, right)
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}

		if got := readFile(t, filepath.Join(right, "a.txt")); got != "old a\n" {
			t.Errorf("right/a.txt = %q, want %q", got, "old a\n")
		}
		if got := readFile(t, filepath.Join(right, "d", "e", "b.txt")); got != "old b\n" {
			t.Errorf("right/d/e/b.txt = %q, want %q", got, "old b\n")
		}
		assertMissing(t, filepath.Join(right, "extra.txt"))
		assertMissing(t, filepath.Join(right, "x"))
	})

	t.Run("preserves executable bits and keeps files writable", func(t *testing.T) {
		left, right := t.TempDir(), t.TempDir()
		writeFile(t, filepath.Join(left, "run.sh"), "#!/bin/sh\n", 0o555)
		writeFile(t, filepath.Join(right, "run.sh"), "#!/bin/sh\necho\n", 0o644)
		spec := writeSpec(t, nil)

		err := splittool.Run(spec, left, right)
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}

		assertPerm(t, filepath.Join(right, "run.sh"), 0o755)
	})

	t.Run("copies symlinks as symlinks", func(t *testing.T) {
		left, right := t.TempDir(), t.TempDir()
		writeFile(t, filepath.Join(left, "a.txt"), "a\n", 0o644)
		if err := os.Symlink("a.txt", filepath.Join(left, "link")); err != nil {
			t.Fatalf("Symlink() error = %v", err)
		}
		writeFile(t, filepath.Join(right, "link"), "not a link\n", 0o644)
		spec := writeSpec(t, nil)

		err := splittool.Run(spec, left, right)
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}

		got, err := os.Readlink(filepath.Join(right, "link"))
		if err != nil {
			t.Fatalf("Readlink() error = %v", err)
		}
		if got != "a.txt" {
			t.Errorf("Readlink(right/link) = %q, want %q", got, "a.txt")
		}
	})

	t.Run("leaves JJ-INSTRUCTIONS in right untouched", func(t *testing.T) {
		left, right := t.TempDir(), t.TempDir()
		writeFile(t, filepath.Join(right, "JJ-INSTRUCTIONS"), "help\n", 0o644)
		spec := writeSpec(t, nil)

		err := splittool.Run(spec, left, right)
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}

		if got := readFile(t, filepath.Join(right, "JJ-INSTRUCTIONS")); got != "help\n" {
			t.Errorf("right/JJ-INSTRUCTIONS = %q, want %q", got, "help\n")
		}
	})

	t.Run("writes content over an existing file keeping its left mode", func(t *testing.T) {
		left, right := t.TempDir(), t.TempDir()
		writeFile(t, filepath.Join(left, "run.sh"), "old\n", 0o555)
		writeFile(t, filepath.Join(right, "run.sh"), "new\n", 0o755)
		spec := writeSpec(t, []jj.SplitFile{{Path: "run.sh", Content: "picked\n"}})

		err := splittool.Run(spec, left, right)
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}

		if got := readFile(t, filepath.Join(right, "run.sh")); got != "picked\n" {
			t.Errorf("right/run.sh = %q, want %q", got, "picked\n")
		}
		assertPerm(t, filepath.Join(right, "run.sh"), 0o755)
	})

	t.Run("writes invalid UTF-8 content unchanged", func(t *testing.T) {
		left, right := t.TempDir(), t.TempDir()
		content := "\x89PNG\r\n\x1a\n\xff\xfe\x00\xff"
		spec := writeSpec(t, []jj.SplitFile{{Path: "image.png", Content: content}})

		err := splittool.Run(spec, left, right)
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}

		if got := readFile(t, filepath.Join(right, "image.png")); got != content {
			t.Errorf("right/image.png = %q, want %q", got, content)
		}
	})

	t.Run("writes a new file in a new directory with mode 0644", func(t *testing.T) {
		left, right := t.TempDir(), t.TempDir()
		writeFile(t, filepath.Join(right, "new", "dir", "f.txt"), "f\n", 0o755)
		spec := writeSpec(t, []jj.SplitFile{{Path: "new/dir/f.txt", Content: "picked\n"}})

		err := splittool.Run(spec, left, right)
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}

		if got := readFile(t, filepath.Join(right, "new", "dir", "f.txt")); got != "picked\n" {
			t.Errorf("right/new/dir/f.txt = %q, want %q", got, "picked\n")
		}
		assertPerm(t, filepath.Join(right, "new", "dir", "f.txt"), 0o644)
	})

	t.Run("deletes a file present in left", func(t *testing.T) {
		left, right := t.TempDir(), t.TempDir()
		writeFile(t, filepath.Join(left, "gone.txt"), "gone\n", 0o444)
		spec := writeSpec(t, []jj.SplitFile{{Path: "gone.txt", Delete: true}})

		err := splittool.Run(spec, left, right)
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}

		assertMissing(t, filepath.Join(right, "gone.txt"))
	})

	t.Run("deleting a file absent after sync succeeds", func(t *testing.T) {
		left, right := t.TempDir(), t.TempDir()
		writeFile(t, filepath.Join(right, "added.txt"), "added\n", 0o644)
		spec := writeSpec(t, []jj.SplitFile{{Path: "added.txt", Delete: true}})

		err := splittool.Run(spec, left, right)
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}

		assertMissing(t, filepath.Join(right, "added.txt"))
	})

	t.Run("replaces a file with a directory when the addition is listed first", func(t *testing.T) {
		left, right := t.TempDir(), t.TempDir()
		writeFile(t, filepath.Join(left, "entry"), "file\n", 0o644)
		spec := writeSpec(t, []jj.SplitFile{
			{Path: "entry/nested", Content: "nested\n"},
			{Path: "entry", Delete: true},
		})

		err := splittool.Run(spec, left, right)
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}

		if got := readFile(t, filepath.Join(right, "entry", "nested")); got != "nested\n" {
			t.Errorf("right/entry/nested = %q, want %q", got, "nested\n")
		}
	})

	t.Run("replaces a directory with a file when the addition is listed first", func(t *testing.T) {
		left, right := t.TempDir(), t.TempDir()
		writeFile(t, filepath.Join(left, "entry", "nested"), "nested\n", 0o644)
		spec := writeSpec(t, []jj.SplitFile{
			{Path: "entry", Content: "file\n"},
			{Path: "entry/nested", Delete: true},
		})

		err := splittool.Run(spec, left, right)
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}

		if got := readFile(t, filepath.Join(right, "entry")); got != "file\n" {
			t.Errorf("right/entry = %q, want %q", got, "file\n")
		}
	})

	t.Run("replaces a nested directory tree with a file", func(t *testing.T) {
		left, right := t.TempDir(), t.TempDir()
		writeFile(t, filepath.Join(left, "entry", "a", "b", "nested"), "nested\n", 0o644)
		writeFile(t, filepath.Join(left, "keep", "kept"), "kept\n", 0o644)
		spec := writeSpec(t, []jj.SplitFile{
			{Path: "entry/a/b/nested", Delete: true},
			{Path: "entry", Content: "file\n"},
			{Path: "keep/kept", Delete: true},
		})

		err := splittool.Run(spec, left, right)
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}

		if got := readFile(t, filepath.Join(right, "entry")); got != "file\n" {
			t.Errorf("right/entry = %q, want %q", got, "file\n")
		}
		assertMissing(t, filepath.Join(right, "keep"))
	})
}

func TestRunMode_success(t *testing.T) {
	t.Run("writes an executable file for mode 100755", func(t *testing.T) {
		left, right := t.TempDir(), t.TempDir()
		spec := writeSpec(t, []jj.SplitFile{{Path: "bin/run.sh", Content: "#!/bin/sh\n", Mode: "100755"}})

		err := splittool.Run(spec, left, right)
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}

		assertPerm(t, filepath.Join(right, "bin", "run.sh"), 0o755)
	})

	t.Run("applies a mode-only change with content equal to left", func(t *testing.T) {
		left, right := t.TempDir(), t.TempDir()
		writeFile(t, filepath.Join(left, "run.sh"), "same\n", 0o444)
		spec := writeSpec(t, []jj.SplitFile{{Path: "run.sh", Content: "same\n", Mode: "100755"}})

		err := splittool.Run(spec, left, right)
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}

		if got := readFile(t, filepath.Join(right, "run.sh")); got != "same\n" {
			t.Errorf("right/run.sh = %q, want %q", got, "same\n")
		}
		assertPerm(t, filepath.Join(right, "run.sh"), 0o755)
	})

	t.Run("clears the executable bit for mode 100644", func(t *testing.T) {
		left, right := t.TempDir(), t.TempDir()
		writeFile(t, filepath.Join(left, "run.sh"), "x\n", 0o555)
		spec := writeSpec(t, []jj.SplitFile{{Path: "run.sh", Content: "x\n", Mode: "100644"}})

		err := splittool.Run(spec, left, right)
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}

		assertPerm(t, filepath.Join(right, "run.sh"), 0o644)
	})

	t.Run("creates a symlink whose target is the content for mode 120000", func(t *testing.T) {
		left, right := t.TempDir(), t.TempDir()
		writeFile(t, filepath.Join(left, "link"), "regular\n", 0o444)
		spec := writeSpec(t, []jj.SplitFile{{Path: "d/link", Content: "../../outside/target", Mode: "120000"}})

		err := splittool.Run(spec, left, right)
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}

		got, err := os.Readlink(filepath.Join(right, "d", "link"))
		if err != nil {
			t.Fatalf("Readlink() error = %v", err)
		}
		if got != "../../outside/target" {
			t.Errorf("Readlink(right/d/link) = %q, want %q", got, "../../outside/target")
		}
	})

	t.Run("replaces a regular file with a symlink", func(t *testing.T) {
		left, right := t.TempDir(), t.TempDir()
		writeFile(t, filepath.Join(left, "link"), "regular\n", 0o444)
		spec := writeSpec(t, []jj.SplitFile{{Path: "link", Content: "target", Mode: "120000"}})

		err := splittool.Run(spec, left, right)
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}

		got, err := os.Readlink(filepath.Join(right, "link"))
		if err != nil {
			t.Fatalf("Readlink() error = %v", err)
		}
		if got != "target" {
			t.Errorf("Readlink(right/link) = %q, want %q", got, "target")
		}
	})
}

func TestRunMode_failed(t *testing.T) {
	t.Run("rejects an unknown mode before changing right", func(t *testing.T) {
		left, right := t.TempDir(), t.TempDir()
		writeFile(t, filepath.Join(right, "keep.txt"), "keep\n", 0o644)
		spec := writeSpec(t, []jj.SplitFile{{Path: "a.txt", Content: "a\n", Mode: "040000"}})

		err := splittool.Run(spec, left, right)

		if !errors.Is(err, splittool.ErrInvalidMode) {
			t.Errorf("Run() error = %v, want %v", err, splittool.ErrInvalidMode)
		}
		if got := readFile(t, filepath.Join(right, "keep.txt")); got != "keep\n" {
			t.Errorf("right/keep.txt = %q, want %q", got, "keep\n")
		}
	})

	t.Run("rejects a symlink path that escapes right through a symlink", func(t *testing.T) {
		left, right, outside := t.TempDir(), t.TempDir(), t.TempDir()
		if err := os.Symlink(outside, filepath.Join(left, "out")); err != nil {
			t.Fatalf("Symlink() error = %v", err)
		}
		spec := writeSpec(t, []jj.SplitFile{{Path: "out/link", Content: "target", Mode: "120000"}})

		err := splittool.Run(spec, left, right)

		if err == nil {
			t.Error("Run() error = nil, want an error")
		}
		assertMissing(t, filepath.Join(outside, "link"))
	})
}

func TestRun_failed(t *testing.T) {
	t.Run("rejects unsafe paths", func(t *testing.T) {
		tests := map[string]string{
			"absolute":          "/etc/passwd",
			"parent segment":    "../escape.txt",
			"nested parent":     "a/../../escape.txt",
			"empty":             "",
			"current directory": ".",
		}
		for name, path := range tests {
			t.Run(name, func(t *testing.T) {
				root := t.TempDir()
				left, right := filepath.Join(root, "left"), filepath.Join(root, "right")
				for _, dir := range []string{left, right} {
					if err := os.Mkdir(dir, 0o755); err != nil {
						t.Fatalf("Mkdir(%q) error = %v", dir, err)
					}
				}
				spec := writeSpec(t, []jj.SplitFile{{Path: path, Content: "x\n"}})

				err := splittool.Run(spec, left, right)

				if !errors.Is(err, splittool.ErrInvalidPath) {
					t.Errorf("Run() error = %v, want %v", err, splittool.ErrInvalidPath)
				}
				assertMissing(t, filepath.Join(root, "escape.txt"))
			})
		}
	})

	t.Run("rejects writing through a symlink that escapes right", func(t *testing.T) {
		left, right, outside := t.TempDir(), t.TempDir(), t.TempDir()
		if err := os.Symlink(outside, filepath.Join(left, "out")); err != nil {
			t.Fatalf("Symlink() error = %v", err)
		}
		spec := writeSpec(t, []jj.SplitFile{{Path: "out/escape.txt", Content: "x\n"}})

		err := splittool.Run(spec, left, right)

		if err == nil {
			t.Error("Run() error = nil, want an error")
		}
		assertMissing(t, filepath.Join(outside, "escape.txt"))
	})

	t.Run("rejects malformed JSON", func(t *testing.T) {
		left, right := t.TempDir(), t.TempDir()
		spec := filepath.Join(t.TempDir(), "spec.json")
		writeFile(t, spec, "[not json", 0o644)

		err := splittool.Run(spec, left, right)

		if _, ok := errors.AsType[*jsontext.SyntacticError](err); !ok {
			t.Errorf("Run() error = %v, want *jsontext.SyntacticError", err)
		}
	})

	t.Run("reports a missing spec file", func(t *testing.T) {
		left, right := t.TempDir(), t.TempDir()

		err := splittool.Run(filepath.Join(t.TempDir(), "missing.json"), left, right)

		if !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("Run() error = %v, want %v", err, fs.ErrNotExist)
		}
	})
}

func BenchmarkRun_deleteManySiblings(b *testing.B) {
	const count = 10_000
	left, right := b.TempDir(), b.TempDir()
	files := make([]jj.SplitFile, 0, count)
	for i := range count {
		name := "f" + strconv.Itoa(i)
		writeFile(b, filepath.Join(left, "dir", name), "x\n", 0o644)
		files = append(files, jj.SplitFile{Path: "dir/" + name, Delete: true})
	}
	spec := writeSpec(b, files)

	for b.Loop() {
		if err := splittool.Run(spec, left, right); err != nil {
			b.Fatalf("Run() error = %v", err)
		}
	}
}
