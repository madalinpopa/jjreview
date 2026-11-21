package tui

import (
	"context"

	"github.com/madalinpopa/jjreview/internal/diff"
	"github.com/madalinpopa/jjreview/internal/jj"
)

type Source interface {
	Change(ctx context.Context, rev string) (jj.Change, error)
	Diff(ctx context.Context, rev string) ([]diff.File, error)
	FileContent(ctx context.Context, rev, path string) (string, error)
	WorkspaceRoot(ctx context.Context) (string, error)
	Log(ctx context.Context) ([]jj.LogLine, error)
	Split(ctx context.Context, changeID, commitID, message string, files []jj.SplitFile) (selected, rest jj.Change, err error)
}
