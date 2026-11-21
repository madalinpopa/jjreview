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
	Bookmarks(ctx context.Context) ([]jj.Bookmark, error)
	CreateBookmark(ctx context.Context, name, rev string) (string, error)
	RenameBookmark(ctx context.Context, oldName, newName string) (string, error)
	MoveBookmark(ctx context.Context, name string, from []string, to string, allowBackwards bool) (string, error)
	DeleteBookmark(ctx context.Context, name string) (string, error)
	Abandon(ctx context.Context, commitID string, bookmarks []string) (workingCopy bool, err error)
	Squash(ctx context.Context, from []jj.LogEntry, into jj.LogEntry) (string, error)
	Push(ctx context.Context, e jj.LogEntry) ([]jj.PushResult, error)
	Fetch(ctx context.Context) (bool, error)
	Successor(ctx context.Context, changeID, commitID string) (jj.Change, error)
	Operations(ctx context.Context) ([]jj.Operation, error)
	RestoreOperation(ctx context.Context, id string) error
	RevertOperation(ctx context.Context, id string) error
	AbandonOperation(ctx context.Context, id string) error
	Split(ctx context.Context, changeID, commitID, message string, files []jj.SplitFile) (selected, rest jj.Change, err error)
}
