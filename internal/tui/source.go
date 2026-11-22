package tui

import (
	"context"

	"github.com/madalinpopa/jjreview/internal/jj"
	"github.com/madalinpopa/jjreview/internal/tui/bookmarks"
	"github.com/madalinpopa/jjreview/internal/tui/logview"
	"github.com/madalinpopa/jjreview/internal/tui/operations"
)

type Source interface {
	logview.Source
	bookmarks.Source
	operations.Source
	FileContent(ctx context.Context, rev, path string) (string, error)
	WorkspaceRoot(ctx context.Context) (string, error)
	Successor(ctx context.Context, changeID, commitID string) (jj.Change, error)
	Split(ctx context.Context, changeID, commitID, message string, files []jj.SplitFile) (selected, rest jj.Change, err error)
}
