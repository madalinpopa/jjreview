package jj

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"
	"syscall"
)

type PushResult struct {
	Remote    string
	Bookmarks []string
}

func (c Client) Push(ctx context.Context, e LogEntry) ([]PushResult, error) {
	if _, err := c.listed(ctx, e.CommitID, e.Bookmarks); err != nil {
		return nil, fmt.Errorf("push %s: %w", e.CommitID, err)
	}
	args := []string{"git", "push"}
	for _, b := range e.Bookmarks {
		args = append(args, "--bookmark=exact:"+quote(b))
	}
	if len(e.Bookmarks) == 0 {
		args = append(args, "--change=exactly("+VisibleCommitRevset(e.CommitID)+", 1)")
	}
	stderr, err := c.remote(ctx, args...)
	if err != nil {
		return nil, err
	}
	return pushed(stderr), nil
}

func pushed(stderr string) []PushResult {
	var out []PushResult
	for line := range strings.Lines(stderr) {
		line = strings.TrimRight(line, "\n")
		if remote, ok := strings.CutPrefix(line, "Changes to push to "); ok {
			out = append(out, PushResult{Remote: strings.TrimSuffix(remote, ":")})
			continue
		}
		if rest, ok := strings.CutPrefix(line, "  bookmark: "); ok && len(out) > 0 {
			name, _, _ := strings.Cut(rest, " [")
			out[len(out)-1].Bookmarks = append(out[len(out)-1].Bookmarks, name)
		}
	}
	return slices.DeleteFunc(out, func(r PushResult) bool { return len(r.Bookmarks) == 0 })
}

func (c Client) Fetch(ctx context.Context) (bool, error) {
	stderr, err := c.remote(ctx, "git", "fetch")
	if err != nil {
		return false, err
	}
	return !strings.Contains(stderr, "Nothing changed."), nil
}

func (c Client) remote(ctx context.Context, args ...string) (string, error) {
	cmd := c.command(ctx, args)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if _, err := cmd.Output(); err != nil {
		return "", jjError(args, err, errorFirst(stderr.String()))
	}
	return stderr.String(), nil
}

func errorFirst(stderr string) string {
	var cause string
	var rest []string
	for line := range strings.Lines(stderr) {
		if cause == "" && strings.HasPrefix(line, "Error:") {
			cause = strings.TrimSuffix(line, "\n")
			continue
		}
		rest = append(rest, line)
	}
	if cause == "" {
		return stderr
	}
	return cause + "\n" + strings.Join(rest, "")
}
