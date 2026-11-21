package splittool

import (
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"

	"github.com/madalinpopa/jjreview/internal/jj"
)

const (
	instructionsFile = "JJ-INSTRUCTIONS"
	modeRegular      = "100644"
	modeExecutable   = "100755"
	modeSymlink      = "120000"
)

var (
	ErrInvalidPath = errors.New("invalid path")
	ErrInvalidMode = errors.New("invalid mode")
)

type entry struct {
	jj.SplitFile
	name string
}

func Run(specPath, left, right string) (err error) {
	entries, err := readSpec(specPath)
	if err != nil {
		return err
	}

	leftRoot, err := os.OpenRoot(left)
	if err != nil {
		return fmt.Errorf("open left %s: %w", left, err)
	}
	defer func() { err = errors.Join(err, closeRoot(leftRoot)) }()

	rightRoot, err := os.OpenRoot(right)
	if err != nil {
		return fmt.Errorf("open right %s: %w", right, err)
	}
	defer func() { err = errors.Join(err, closeRoot(rightRoot)) }()

	if err := syncTree(leftRoot, rightRoot); err != nil {
		return err
	}
	for _, e := range entries {
		if e.Delete {
			if err := deleteEntry(rightRoot, e.name); err != nil {
				return err
			}
		}
	}
	for _, e := range entries {
		if !e.Delete {
			if err := apply(leftRoot, rightRoot, e); err != nil {
				return err
			}
		}
	}
	return nil
}

func closeRoot(root *os.Root) error {
	if err := root.Close(); err != nil {
		return fmt.Errorf("close %s: %w", root.Name(), err)
	}
	return nil
}

func readSpec(specPath string) ([]entry, error) {
	data, err := os.ReadFile(specPath) //nolint:gosec
	if err != nil {
		return nil, fmt.Errorf("read spec %s: %w", specPath, err)
	}
	var files []jj.SplitFile
	if err := json.Unmarshal(data, &files); err != nil {
		return nil, fmt.Errorf("parse spec %s: %w", specPath, err)
	}
	entries := make([]entry, 0, len(files))
	for _, f := range files {
		name, err := localName(f.Path)
		if err != nil {
			return nil, err
		}
		if err := checkMode(f); err != nil {
			return nil, err
		}
		entries = append(entries, entry{SplitFile: f, name: name})
	}
	return entries, nil
}

func checkMode(f jj.SplitFile) error {
	switch f.Mode {
	case "", modeRegular, modeExecutable, modeSymlink:
		return nil
	}
	return fmt.Errorf("%w %q for %s", ErrInvalidMode, f.Mode, f.Path)
}

func localName(path string) (string, error) {
	name := filepath.FromSlash(path)
	if !filepath.IsLocal(name) || filepath.Clean(name) == "." {
		return "", fmt.Errorf("%w: %q", ErrInvalidPath, path)
	}
	return filepath.Clean(name), nil
}

func syncTree(left, right *os.Root) error {
	want, err := scan(left)
	if err != nil {
		return err
	}
	if err := prune(right, want); err != nil {
		return err
	}
	return copyTree(left, right)
}

func scan(left *os.Root) (map[string]fs.FileMode, error) {
	want := make(map[string]fs.FileMode)
	err := walkTracked(left, func(name string, d fs.DirEntry) error {
		want[name] = d.Type()
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan left %s: %w", left.Name(), err)
	}
	return want, nil
}

func walkTracked(root *os.Root, fn func(name string, d fs.DirEntry) error) error {
	return fs.WalkDir(root.FS(), ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := filepath.FromSlash(path)
		if name == "." || name == instructionsFile {
			return nil
		}
		return fn(name, d)
	})
}

func prune(right *os.Root, want map[string]fs.FileMode) error {
	return fs.WalkDir(right.FS(), ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("scan right %s: %w", path, err)
		}
		name := filepath.FromSlash(path)
		if name == "." || name == instructionsFile {
			return nil
		}
		if typ, ok := want[name]; ok && typ == d.Type() {
			return nil
		}
		if err := right.RemoveAll(name); err != nil {
			return fmt.Errorf("remove %s: %w", path, err)
		}
		if d.IsDir() {
			return fs.SkipDir
		}
		return nil
	})
}

func copyTree(left, right *os.Root) error {
	return walkTracked(left, func(name string, d fs.DirEntry) error {
		switch {
		case d.IsDir():
			if err := right.Mkdir(name, 0o755); err != nil && !errors.Is(err, fs.ErrExist) {
				return fmt.Errorf("create directory %s: %w", name, err)
			}
		case d.Type()&fs.ModeSymlink != 0:
			return copySymlink(left, right, name)
		case d.Type().IsRegular():
			return copyFile(left, right, name)
		}
		return nil
	})
}

func copySymlink(left, right *os.Root, name string) error {
	target, err := left.Readlink(name)
	if err != nil {
		return fmt.Errorf("read symlink %s: %w", name, err)
	}
	return replaceSymlink(right, name, target)
}

func replaceSymlink(root *os.Root, name, target string) error {
	if err := removeIfExists(root, name); err != nil {
		return err
	}
	if err := root.Symlink(target, name); err != nil {
		return fmt.Errorf("create symlink %s: %w", name, err)
	}
	return nil
}

func copyFile(left, right *os.Root, name string) error {
	info, err := left.Lstat(name)
	if err != nil {
		return fmt.Errorf("stat %s: %w", name, err)
	}
	data, err := left.ReadFile(name)
	if err != nil {
		return fmt.Errorf("read %s: %w", name, err)
	}
	return replaceFile(right, name, data, writable(info.Mode()))
}

func deleteEntry(right *os.Root, name string) error {
	if err := removeIfExists(right, name); err != nil {
		return err
	}
	for dir := filepath.Dir(name); dir != "."; dir = filepath.Dir(dir) {
		empty, err := isEmptyDir(right, dir)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if !empty {
			return nil
		}
		if err := right.Remove(dir); err != nil {
			return fmt.Errorf("remove %s: %w", dir, err)
		}
	}
	return nil
}

func isEmptyDir(root *os.Root, dir string) (empty bool, err error) {
	f, err := root.Open(dir)
	if err != nil {
		return false, fmt.Errorf("read directory %s: %w", dir, err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil {
			err = errors.Join(err, fmt.Errorf("close directory %s: %w", dir, cerr))
		}
	}()
	if _, err := f.Readdirnames(1); err != nil {
		if errors.Is(err, io.EOF) {
			return true, nil
		}
		return false, fmt.Errorf("read directory %s: %w", dir, err)
	}
	return false, nil
}

func apply(left, right *os.Root, e entry) error {
	if err := right.MkdirAll(filepath.Dir(e.name), 0o755); err != nil {
		return fmt.Errorf("create directory for %s: %w", e.Path, err)
	}
	switch e.Mode {
	case modeSymlink:
		return replaceSymlink(right, e.name, e.Content)
	case modeExecutable:
		return replaceFile(right, e.name, []byte(e.Content), 0o755)
	case modeRegular:
		return replaceFile(right, e.name, []byte(e.Content), 0o644)
	}
	perm, err := leftPerm(left, e.name)
	if err != nil {
		return err
	}
	return replaceFile(right, e.name, []byte(e.Content), perm)
}

func leftPerm(left *os.Root, name string) (fs.FileMode, error) {
	info, err := left.Lstat(name)
	if errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR) {
		return 0o644, nil
	}
	if err != nil {
		return 0, fmt.Errorf("stat left %s: %w", name, err)
	}
	if !info.Mode().IsRegular() {
		return 0o644, nil
	}
	return writable(info.Mode()), nil
}

func writable(mode fs.FileMode) fs.FileMode {
	return mode.Perm() | 0o200
}

func replaceFile(root *os.Root, name string, data []byte, perm fs.FileMode) error {
	if err := removeIfExists(root, name); err != nil {
		return err
	}
	if err := root.WriteFile(name, data, perm); err != nil {
		return fmt.Errorf("write %s: %w", name, err)
	}
	if err := root.Chmod(name, perm); err != nil {
		return fmt.Errorf("chmod %s: %w", name, err)
	}
	return nil
}

func removeIfExists(root *os.Root, name string) error {
	if err := root.Remove(name); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove %s: %w", name, err)
	}
	return nil
}
