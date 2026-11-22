package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/madalinpopa/jjreview/internal/jj"
	"github.com/madalinpopa/jjreview/internal/tui/tab"
	"github.com/madalinpopa/jjreview/internal/tui/ui"
)

func (m *Model) push(e jj.LogEntry) tea.Cmd {
	id := ui.ShortID(e.ChangeID)
	retry := "bookmarks changed; press " + m.log.PushKey() + " to review the push again"
	src := m.src
	return m.rewrite(tab.Rewrite{Pending: "pushing " + id + "…", Focus: e.ChangeID, Run: func(ctx context.Context, _ tab.Head) (tab.Result, error) {
		r, err := src.Push(ctx, e)
		res := tab.Result{KeepPicks: true}
		switch {
		case errors.Is(err, jj.ErrBookmarksChanged):
			return res, errors.New(retry)
		case err != nil:
			return res, fmt.Errorf("push %s: %w", id, err)
		case len(r) == 0:
			res.Done = "nothing to push"
		default:
			res.Done = "pushed " + pushSummary(r)
		}
		return res, nil
	}})
}

func pushSummary(results []jj.PushResult) string {
	parts := make([]string, len(results))
	for i, r := range results {
		parts[i] = strings.Join(r.Bookmarks, ", ") + " to " + r.Remote
	}
	return strings.Join(parts, " and ")
}

func (m *Model) fetchRemote() tea.Cmd {
	src := m.src
	return m.rewrite(tab.Rewrite{Pending: "fetching…", Run: func(ctx context.Context, _ tab.Head) (tab.Result, error) {
		changed, err := src.Fetch(ctx)
		res := tab.Result{KeepPicks: true, Done: "fetched"}
		switch {
		case err != nil:
			return res, fmt.Errorf("fetch: %w", err)
		case !changed:
			res.Done = "already up to date"
		}
		return res, nil
	}})
}
