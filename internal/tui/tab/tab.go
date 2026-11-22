package tab

import (
	"context"
	"fmt"

	"github.com/madalinpopa/jjreview/internal/tui/ui"
)

type Head struct {
	CommitID    string
	WorkingCopy bool
}

type Result struct {
	Done      string
	Warnings  string
	Replaced  bool
	Restored  bool
	KeepPicks bool
}

type Rewrite struct {
	Pending string
	Focus   string
	Run     func(ctx context.Context, head Head) (Result, error)
}

type Lock struct {
	Busy   bool
	Reason string
}

func DropPrompt(key string, n int) string {
	return fmt.Sprintf("press %s again to drop %d selected %s", key, n, ui.Plural(n, "hunk"))
}
