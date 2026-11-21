package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/madalinpopa/jjreview/internal/jj"
	"github.com/madalinpopa/jjreview/internal/tui/ui"
)

func (m *Model) push(e jj.LogEntry) tea.Cmd {
	id := ui.ShortID(e.ChangeID)
	retry := "bookmarks changed; press " + m.log.PushKey() + " to review the push again"
	m.status = "pushing " + id + "…"
	return m.rewrite(e.ChangeID, func(ctx context.Context, src Source, _ string) (rewriteResult, error) {
		r, err := src.Push(ctx, e)
		res := rewriteResult{keepPicks: true}
		switch {
		case errors.Is(err, jj.ErrBookmarksChanged):
			return res, errors.New(retry)
		case err != nil:
			return res, fmt.Errorf("push %s: %w", id, err)
		case len(r) == 0:
			res.done = "nothing to push"
		default:
			res.done = "pushed " + pushSummary(r)
		}
		return res, nil
	})
}

func pushSummary(results []jj.PushResult) string {
	parts := make([]string, len(results))
	for i, r := range results {
		parts[i] = strings.Join(r.Bookmarks, ", ") + " to " + r.Remote
	}
	return strings.Join(parts, " and ")
}

func (m *Model) fetchRemote() tea.Cmd {
	m.status = "fetching…"
	return m.rewrite("", func(ctx context.Context, src Source, _ string) (rewriteResult, error) {
		changed, err := src.Fetch(ctx)
		res := rewriteResult{keepPicks: true, done: "fetched"}
		switch {
		case err != nil:
			return res, fmt.Errorf("fetch: %w", err)
		case !changed:
			res.done = "already up to date"
		}
		return res, nil
	})
}
