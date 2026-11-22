package jj_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/madalinpopa/jjreview/internal/jj"
)

func fakeJJ(t *testing.T, script string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "jj"), []byte("#!/bin/sh\n"+script+"\n"), 0o755); err != nil {
		t.Fatalf("write fake jj: %v", err)
	}
	t.Setenv("PATH", dir)
}

func TestParseVersion_success(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		output string
		want   jj.Version
	}{
		"release":                  {output: "jj 0.47.1\n", want: jj.Version{Major: 0, Minor: 47, Patch: 1}},
		"development suffix":       {output: "jj 0.48.0-dev", want: jj.Version{Major: 0, Minor: 48}},
		"commit suffix":            {output: "jj 0.47.0-abc123def", want: jj.Version{Major: 0, Minor: 47}},
		"build metadata":           {output: "jj 0.47.2+build.5", want: jj.Version{Major: 0, Minor: 47, Patch: 2}},
		"suffix and metadata":      {output: "jj 1.2.3-dev+abc.4", want: jj.Version{Major: 1, Minor: 2, Patch: 3}},
		"multi-digit components":   {output: "jj 12.340.56", want: jj.Version{Major: 12, Minor: 340, Patch: 56}},
		"without the program name": {output: "0.47.1", want: jj.Version{Major: 0, Minor: 47, Patch: 1}},
		"without a patch":          {output: "0.47", want: jj.Version{Major: 0, Minor: 47}},
		"surrounding whitespace":   {output: "  jj 0.47.1 \r\n", want: jj.Version{Major: 0, Minor: 47, Patch: 1}},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := jj.ParseVersion(tt.output)

			if err != nil {
				t.Fatalf("ParseVersion(%q) error = %v", tt.output, err)
			}
			if got != tt.want {
				t.Errorf("ParseVersion(%q) = %v, want %v", tt.output, got, tt.want)
			}
		})
	}
}

func TestParseVersion_failed(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"empty":                  "",
		"program name only":      "jj",
		"no numbers":             "jj unknown",
		"major only":             "jj 1",
		"empty component":        "jj 0..1",
		"trailing dot":           "jj 0.47.",
		"too many components":    "jj 0.47.1.2",
		"suffix before a minor":  "jj 0-dev.47.1",
		"another program":        "git version 2.47.1",
		"component out of range": "jj 0.99999999999999999999.0",
	}

	for name, output := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := jj.ParseVersion(output)

			if !errors.Is(err, jj.ErrUnknownVersion) {
				t.Fatalf("ParseVersion(%q) error = %v, want %v", output, err, jj.ErrUnknownVersion)
			}
			if !strings.Contains(err.Error(), strings.TrimSpace(output)) {
				t.Errorf("ParseVersion(%q) error = %q, want it to include the output", output, err)
			}
		})
	}
}

func TestVersionCompare_success(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		a, b jj.Version
		want int
	}{
		"equal":                      {a: jj.Version{Minor: 47, Patch: 1}, b: jj.Version{Minor: 47, Patch: 1}, want: 0},
		"older patch":                {a: jj.Version{Minor: 47}, b: jj.Version{Minor: 47, Patch: 1}, want: -1},
		"newer patch":                {a: jj.Version{Minor: 47, Patch: 2}, b: jj.Version{Minor: 47, Patch: 1}, want: 1},
		"older minor, newer patch":   {a: jj.Version{Minor: 46, Patch: 9}, b: jj.Version{Minor: 47}, want: -1},
		"newer minor, older patch":   {a: jj.Version{Minor: 100}, b: jj.Version{Minor: 47, Patch: 9}, want: 1},
		"newer major, older minor":   {a: jj.Version{Major: 1}, b: jj.Version{Minor: 47}, want: 1},
		"older major, newer minor":   {a: jj.Version{Minor: 99, Patch: 9}, b: jj.Version{Major: 1}, want: -1},
		"minor compared numerically": {a: jj.Version{Minor: 9}, b: jj.Version{Minor: 47}, want: -1},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := tt.a.Compare(tt.b)

			if got != tt.want {
				t.Errorf("%v.Compare(%v) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestVersionString_success(t *testing.T) {
	t.Parallel()

	got := jj.Version{Minor: 44, Patch: 2}.String()

	if got != "0.44.2" {
		t.Errorf("Version.String() = %q, want %q", got, "0.44.2")
	}
}

func TestMinVersion_success(t *testing.T) {
	t.Parallel()

	got, err := jj.ParseVersion(jj.MinVersion)

	if err != nil {
		t.Fatalf("ParseVersion(%q) error = %v", jj.MinVersion, err)
	}
	if want := (jj.Version{Minor: 47}); got != want {
		t.Errorf("ParseVersion(%q) = %v, want %v", jj.MinVersion, got, want)
	}
}

func TestCheckVersion_success(t *testing.T) {
	tests := map[string]string{
		"the minimum version":   "jj 0.47.0",
		"a newer patch":         "jj 0.47.1",
		"a newer development":   "jj 0.48.0-dev+abc123",
		"a newer major version": "jj 1.0.0",
	}

	for name, output := range tests {
		t.Run(name, func(t *testing.T) {
			fakeJJ(t, "echo '"+output+"'")

			err := jj.CheckVersion(t.Context(), t.TempDir())

			if err != nil {
				t.Errorf("CheckVersion() with %q error = %v, want nil", output, err)
			}
		})
	}
}

func TestCheckVersion_directory(t *testing.T) {
	t.Run("runs jj in the given directory", func(t *testing.T) {
		dir, err := filepath.EvalSymlinks(t.TempDir())
		if err != nil {
			t.Fatalf("resolve directory: %v", err)
		}
		ranIn := filepath.Join(t.TempDir(), "cwd")
		fakeJJ(t, "pwd -P > '"+ranIn+"'; echo 'jj 0.47.0'")

		err = jj.CheckVersion(t.Context(), dir)

		if err != nil {
			t.Fatalf("CheckVersion(%q) error = %v", dir, err)
		}
		got, err := os.ReadFile(ranIn)
		if err != nil {
			t.Fatalf("read the directory jj ran in: %v", err)
		}
		if strings.TrimSpace(string(got)) != dir {
			t.Errorf("CheckVersion(%q) ran jj in %q, want %q", dir, strings.TrimSpace(string(got)), dir)
		}
	})

	t.Run("reports a directory that does not exist", func(t *testing.T) {
		fakeJJ(t, "echo 'jj 0.47.0'")
		missing := filepath.Join(t.TempDir(), "missing")

		err := jj.CheckVersion(t.Context(), missing)

		if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("CheckVersion(%q) error = %v, want %v", missing, err, os.ErrNotExist)
		}
		if errors.Is(err, jj.ErrNotFound) {
			t.Errorf("CheckVersion(%q) error = %v, want it not to report a missing jj", missing, err)
		}
		if !strings.Contains(err.Error(), missing) {
			t.Errorf("CheckVersion(%q) error = %q, want it to include the directory", missing, err)
		}
	})
}

func TestCheckVersion_canceled(t *testing.T) {
	t.Run("reports a context canceled before jj starts", func(t *testing.T) {
		fakeJJ(t, "echo 'jj 0.47.0'")
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		err := jj.CheckVersion(ctx, t.TempDir())

		if !errors.Is(err, context.Canceled) {
			t.Errorf("CheckVersion() error = %v, want %v", err, context.Canceled)
		}
	})

	t.Run("reports a context canceled while jj runs", func(t *testing.T) {
		fifo := filepath.Join(t.TempDir(), "started")
		if err := syscall.Mkfifo(fifo, 0o600); err != nil {
			t.Fatalf("create fifo: %v", err)
		}
		fakeJJ(t, "read _ < '"+fifo+"'")
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		dir := t.TempDir()
		result := make(chan error, 1)
		go func() { result <- jj.CheckVersion(ctx, dir) }()
		started, err := os.OpenFile(fifo, os.O_WRONLY, 0)
		if err != nil {
			t.Fatalf("wait for jj to start: %v", err)
		}

		cancel()
		err = <-result

		if closeErr := started.Close(); closeErr != nil {
			t.Errorf("close fifo: %v", closeErr)
		}
		if !errors.Is(err, context.Canceled) {
			t.Errorf("CheckVersion() error = %v, want %v", err, context.Canceled)
		}
	})
}

func TestCheckVersion_failed(t *testing.T) {
	t.Run("reports an older jj with the version found", func(t *testing.T) {
		fakeJJ(t, "echo 'jj 0.44.0'")

		err := jj.CheckVersion(t.Context(), t.TempDir())

		if !errors.Is(err, jj.ErrUnsupportedVersion) {
			t.Fatalf("CheckVersion() error = %v, want %v", err, jj.ErrUnsupportedVersion)
		}
		if want := "requires jj 0.47 or newer; found jj 0.44.0"; err.Error() != want {
			t.Errorf("CheckVersion() error = %q, want %q", err, want)
		}
	})

	t.Run("reports an older patch-level prerelease of the previous minor", func(t *testing.T) {
		fakeJJ(t, "echo 'jj 0.46.9-dev'")

		err := jj.CheckVersion(t.Context(), t.TempDir())

		if !errors.Is(err, jj.ErrUnsupportedVersion) {
			t.Errorf("CheckVersion() error = %v, want %v", err, jj.ErrUnsupportedVersion)
		}
	})

	t.Run("reports a missing jj", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())

		err := jj.CheckVersion(t.Context(), t.TempDir())

		if !errors.Is(err, jj.ErrNotFound) {
			t.Fatalf("CheckVersion() error = %v, want %v", err, jj.ErrNotFound)
		}
		if want := "jj not found in PATH; jj 0.47 or newer is required"; err.Error() != want {
			t.Errorf("CheckVersion() error = %q, want %q", err, want)
		}
	})

	t.Run("reports output it cannot parse", func(t *testing.T) {
		fakeJJ(t, "echo 'jujutsu nightly'")

		err := jj.CheckVersion(t.Context(), t.TempDir())

		if !errors.Is(err, jj.ErrUnknownVersion) {
			t.Fatalf("CheckVersion() error = %v, want %v", err, jj.ErrUnknownVersion)
		}
		if !strings.Contains(err.Error(), "jujutsu nightly") {
			t.Errorf("CheckVersion() error = %q, want it to include the output", err)
		}
	})

	t.Run("reports a failing jj with its stderr", func(t *testing.T) {
		fakeJJ(t, "echo 'config is broken' >&2; exit 3")

		err := jj.CheckVersion(t.Context(), t.TempDir())

		if err == nil || !strings.HasPrefix(err.Error(), "jj --version: ") || !strings.HasSuffix(err.Error(), ": config is broken") {
			t.Errorf("CheckVersion() error = %v, want the command and its stderr", err)
		}
		if errors.Is(err, jj.ErrNotFound) || errors.Is(err, jj.ErrUnsupportedVersion) {
			t.Errorf("CheckVersion() error = %v, want neither missing nor unsupported", err)
		}
	})
}
