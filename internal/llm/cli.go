package llm

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode"
)

const (
	cliWaitDelay   = 2 * time.Second
	cliStderrLimit = 500
)

func cliPath(env, name string, notFound error) (string, error) {
	value := os.Getenv(env)
	if value == "" {
		path, err := exec.LookPath(name)
		if err != nil {
			return "", notFound
		}
		return path, nil
	}
	path, err := exec.LookPath(value)
	if err != nil {
		return "", fmt.Errorf("resolve %s %q: %w", env, value, err)
	}
	return filepath.Abs(path)
}

func runCLI(ctx context.Context, path string, args []string, stdin string, stripEnv ...string) (stdout, stderr []byte, err error) {
	dir, err := os.MkdirTemp("", "jjr-")
	if err != nil {
		return nil, nil, fmt.Errorf("create CLI working directory: %w", err)
	}
	defer func() { err = errors.Join(err, os.RemoveAll(dir)) }()

	var out, errOut bytes.Buffer
	cmd := exec.CommandContext(ctx, path, args...) //nolint:gosec
	cmd.Dir = dir
	cmd.Env = slices.DeleteFunc(os.Environ(), func(kv string) bool {
		name, _, _ := strings.Cut(kv, "=")
		return slices.Contains(stripEnv, name)
	})
	cmd.Stdin = strings.NewReader(stdin)
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	cmd.WaitDelay = cliWaitDelay

	if err := cmd.Run(); err != nil {
		return out.Bytes(), errOut.Bytes(), fmt.Errorf("run %s: %w", filepath.Base(path), err)
	}
	return out.Bytes(), errOut.Bytes(), nil
}

func sanitize(b []byte) string {
	if len(b) > cliStderrLimit {
		b = b[len(b)-cliStderrLimit:]
	}
	text := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, strings.ToValidUTF8(string(b), ""))
	return strings.Join(strings.Fields(text), " ")
}

func withStderr(err error, stderr string) error {
	if stderr == "" {
		return err
	}
	return fmt.Errorf("%w: %s", err, stderr)
}
