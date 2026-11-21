package jj

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

var ErrNoSquashSources = errors.New("no revisions selected for squash")

func (c Client) Squash(ctx context.Context, from []LogEntry, into LogEntry) (string, error) {
	if len(from) == 0 {
		return "", fmt.Errorf("squash into %s: %w", into.CommitID, ErrNoSquashSources)
	}
	revs := make([]string, len(from))
	descriptions := []string{into.Description}
	for i, e := range from {
		if e.CommitID == into.CommitID {
			return "", fmt.Errorf("squash %s into itself", into.CommitID)
		}
		revs[i] = CommitRevset(e.CommitID)
		descriptions = append(descriptions, e.Description)
	}
	sources := VisibleRevset(strings.Join(revs, " | "))
	target := VisibleCommitRevset(into.CommitID)
	out, err := c.run(ctx, "log", "--revision="+sources+" | "+target, "--no-graph", "-T", `self.commit_id() ++ "\n"`)
	if err != nil {
		return "", err
	}
	if strings.Count(string(out), "\n") != len(from)+1 {
		return "", fmt.Errorf("squash into %s: %w", into.CommitID, ErrHiddenRevision)
	}
	_, stderr, err := c.output(ctx, "squash",
		"--from=exactly("+sources+", "+strconv.Itoa(len(from))+")",
		"--into=exactly("+target+", 1)",
		"--message="+squashMessage(descriptions),
	)
	if err != nil {
		return "", err
	}
	return squashWarnings(stderr), nil
}

func squashWarnings(stderr string) string {
	var kept []string
	for line := range strings.Lines(stderr) {
		if strings.HasPrefix(line, "Warning:") || strings.HasPrefix(line, "New conflicts appeared") {
			kept = append(kept, strings.TrimRight(line, "\n"))
		}
	}
	return strings.Join(kept, "\n")
}

func squashMessage(descriptions []string) string {
	var parts []string
	for _, d := range descriptions {
		if d = strings.TrimRight(d, "\n"); strings.TrimSpace(d) != "" {
			parts = append(parts, d)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "\n\n") + "\n"
}
