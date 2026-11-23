package tab

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/madalinpopa/jjreview/internal/jj"
	"github.com/madalinpopa/jjreview/internal/tui/ui"
)

type Head struct {
	ChangeID    string
	CommitID    string
	Title       string
	WorkingCopy bool
}

type Env struct {
	ui.Layout
	Head    Head
	Active  bool
	Dropped int
	Lock    Lock
}

type Effect struct {
	Status    string
	Refresh   bool
	Retarget  bool
	KeepArmed bool
	Open      string
	Pick      *jj.LogEntry
	Rewrite   *Rewrite
	Notice    *Notice
}

type View interface {
	Show(env Env) tea.Cmd
	Load() tea.Cmd
	MarkStale()
	Update(msg tea.Msg, env Env) (tea.Cmd, Effect, bool)
	Key(msg tea.KeyPressMsg, env Env) (tea.Cmd, Effect)
	Paste(msg tea.PasteMsg) tea.Cmd
	Ignores(msg tea.KeyPressMsg, env Env) bool
	Captures() bool
	OverlayOpen() bool
	View(env Env) string
	Overlay(env Env) string
	KeepVisible(env Env)
	Hints(tabs, help ui.KeyHint) []ui.KeyHint
	Help() []ui.KeyHint
}

type Result struct {
	Done      string
	Warnings  string
	Replaced  bool
	Restored  bool
	KeepPicks bool
}

type Rewrite struct {
	Title        string
	CommandLabel string
	Focus        string
	Run          func(ctx context.Context, head Head) (Result, error)
}

type Notice struct {
	Seq          int
	Title        string
	CommandLabel string
	Done         bool
	Result       ui.ToastResult
}

type Lock struct {
	Busy   bool
	Reason string
}

func DropPrompt(key string, n int) string {
	return fmt.Sprintf("press %s again to drop %d selected %s", key, n, ui.Plural(n, "hunk"))
}
