package tui_test

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/madalinpopa/jjreview/internal/diff"
	"github.com/madalinpopa/jjreview/internal/jj"
	"github.com/madalinpopa/jjreview/internal/tui"
)

func operationSource() *fakeSource {
	src := revisionSource()
	src.ops = []jj.Operation{
		{ID: "aaaa1111cccc", Description: "describe commit 123", Args: "jj describe -m wip", User: "me@host", Time: "1 minute ago", Current: true},
		{ID: "bbbb2222dddd", Description: "snapshot working copy", Args: "jj status", User: "me@host", Time: "2 minutes ago", Snapshot: true},
		{ID: "cccc3333eeee", Description: "new empty commit", Args: "jj new", User: "me@host", Time: "3 minutes ago"},
		{ID: "000000000000", Root: true},
	}
	return src
}

func assertOperationsTab(t *testing.T, m tui.Model) {
	t.Helper()
	got := view(m)
	assertContains(t, got, "OPERATIONS")
	assertMissing(t, got, "FILES", "UNIFIED DIFF", "│ LOG", "BOOKMARKS")
}

func assertOpRuns(t *testing.T, src *fakeSource, want ...string) {
	t.Helper()
	if !slices.Equal(src.opRuns, want) {
		t.Errorf("operation runs = %v, want %v", src.opRuns, want)
	}
}

func TestOperationsTab_success(t *testing.T) {
	t.Run("4 shows the operations and loads them once", func(t *testing.T) {
		src := operationSource()

		m := press(t, loaded(t, src), "4", "1", "4")

		assertOperationsTab(t, m)
		assertContains(t, view(m), "aaaa1111", "describe commit 123", "1 minute ago", "operation 1/4", "l h command")
		assertMissing(t, view(m), "jj describe -m wip")
		if src.opsCalls != 1 {
			t.Errorf("Operations() calls = %d, want 1", src.opsCalls)
		}
	})

	t.Run("the operations are not loaded before they are shown", func(t *testing.T) {
		src := operationSource()

		loaded(t, src)

		if src.opsCalls != 0 {
			t.Errorf("Operations() calls = %d, want 0", src.opsCalls)
		}
	})

	t.Run("every tab reaches the operations and back", func(t *testing.T) {
		for _, from := range []string{"1", "2", "3"} {
			m := press(t, loaded(t, operationSource()), from, "4")
			assertOperationsTab(t, m)
		}
		assertReviewTab(t, press(t, loaded(t, operationSource()), "4", "1"))
		assertLogTab(t, press(t, loaded(t, operationSource()), "4", "2"))
		assertBookmarksTab(t, press(t, loaded(t, operationSource()), "4", "3"))
	})

	t.Run("j and G move the cursor", func(t *testing.T) {
		m := press(t, loaded(t, operationSource()), "4", "j")
		if got := m.OperationCursor(); got != "bbbb2222dddd" {
			t.Errorf("OperationCursor() = %q, want bbbb2222dddd", got)
		}

		m = press(t, m, "G")

		if got := m.OperationCursor(); got != "000000000000" {
			t.Errorf("OperationCursor() = %q, want the root operation", got)
		}
	})

	t.Run("y restores the repo to the selected operation", func(t *testing.T) {
		src := operationSource()
		m := press(t, loaded(t, src), "4", "j", "j", "r")
		assertContains(t, view(m), "Restore operation cccc3333", "new empty commit", "jj new", "y restore")

		m = press(t, m, "y")

		assertOpRuns(t, src, "restore cccc3333eeee")
		assertContains(t, view(m), "restored operation cccc3333")
		assertOperationsTab(t, m)
		if src.opsCalls != 2 {
			t.Errorf("Operations() calls = %d, want 2", src.opsCalls)
		}
	})

	t.Run("y undoes the selected operation", func(t *testing.T) {
		src := operationSource()

		m := press(t, loaded(t, src), "4", "u", "y")

		assertOpRuns(t, src, "revert aaaa1111cccc")
		assertContains(t, view(m), "undid operation aaaa1111")
	})

	t.Run("y abandons the selected operation", func(t *testing.T) {
		src := operationSource()

		m := press(t, loaded(t, src), "4", "j", "j", "a", "y")

		assertOpRuns(t, src, "abandon cccc3333eeee")
		assertContains(t, view(m), "abandoned operation cccc3333")
	})

	t.Run("any other key cancels the prompt", func(t *testing.T) {
		src := operationSource()

		m := press(t, loaded(t, src), "4", "j", "r", "n")

		assertOpRuns(t, src)
		assertMissing(t, view(m), "Restore operation")
	})

	t.Run("the cursor stays on the operation after a restore adds a new one", func(t *testing.T) {
		src := operationSource()
		src.afterOp = func() {
			src.ops = append([]jj.Operation{{ID: "ffff9999ffff", Description: "restore to operation cccc3333eeee", Current: true}}, src.ops...)
			src.ops[1].Current = false
		}

		m := press(t, loaded(t, src), "4", "j", "j", "r", "y")

		if got := m.OperationCursor(); got != "cccc3333eeee" {
			t.Errorf("OperationCursor() = %q, want cccc3333eeee", got)
		}
	})

	t.Run("the prompt says a restore drops the selected hunks", func(t *testing.T) {
		m := press(t, loaded(t, operationSource()), "tab", "j", "tab", "space", "4", "j", "j", "r")

		assertContains(t, view(m), "Drops 1 selected hunk")
	})

	t.Run("the abandon prompt does not mention the selected hunks", func(t *testing.T) {
		m := press(t, loaded(t, operationSource()), "tab", "j", "tab", "space", "4", "j", "j", "a")

		assertContains(t, view(m), "Abandon operation cccc3333")
		assertMissing(t, view(m), "Drops 1 selected hunk")
	})

	t.Run("abandoning an operation keeps the selected hunks", func(t *testing.T) {
		m := press(t, loaded(t, operationSource()), "tab", "j", "tab", "space", "4", "j", "j", "a", "y")

		assertContains(t, view(m), "● 1")
	})

	t.Run("R refreshes the operations", func(t *testing.T) {
		src := operationSource()

		press(t, loaded(t, src), "4", "R")

		if src.opsCalls != 2 {
			t.Errorf("Operations() calls = %d, want 2", src.opsCalls)
		}
	})

	t.Run("a revision rewrite in the log refreshes the operations on return", func(t *testing.T) {
		src := operationSource()

		press(t, loaded(t, src), "4", "2", "j", "a", "y", "4")

		if src.opsCalls != 2 {
			t.Errorf("Operations() calls = %d, want 2", src.opsCalls)
		}
	})

	t.Run("l shows the command under the operation and h hides it", func(t *testing.T) {
		src := operationSource()
		src.ops[0].Args = "jj describe -m \"first\nsecond\nthird\""

		m := press(t, loaded(t, src), "4", "l")

		got := view(m)
		assertContains(t, lineWith(t, got, "aaaa1111"), "describe commit 123", "1 minute ago")
		assertMissing(t, lineWith(t, got, "aaaa1111"), "jj describe")
		assertContains(t, lineWith(t, got, "jj describe -m \"first"), "jj describe -m \"first")
		assertContains(t, got, "second", "third\"", "bbbb2222", "00000000")
		if got := press(t, m, "j").OperationCursor(); got != "bbbb2222dddd" {
			t.Errorf("OperationCursor() after j = %q, want bbbb2222dddd", got)
		}

		m = press(t, m, "h")

		assertMissing(t, view(m), "jj describe", "second")
	})

	t.Run("l on an operation without a command says so", func(t *testing.T) {
		m := press(t, loaded(t, operationSource()), "4", "G", "l")

		assertContains(t, view(m), "no command recorded")
	})

	t.Run("an expanded command stays open after a refresh", func(t *testing.T) {
		m := press(t, loaded(t, operationSource()), "4", "j", "j", "l", "R")

		assertContains(t, view(m), "jj new")
	})

	t.Run("expanding the last operation scrolls its command into view", func(t *testing.T) {
		src := operationSource()
		src.ops[3].Args = "jj a\nb\nc\nd\ne\nlast line"

		m := press(t, start(t, src, nil, 120, 14), "4", "G", "l")

		got := view(m)
		assertContains(t, got, "00000000", "last line")
		assertMissing(t, got, "aaaa1111")
	})

	t.Run("half page down moves through expanded commands", func(t *testing.T) {
		src := operationSource()
		src.ops[0].Args = strings.Repeat("line\n", 30) + "end"

		m := press(t, start(t, src, nil, 120, 20), "4", "l", "ctrl+d")
		if got := m.OperationCursor(); got != "aaaa1111cccc" {
			t.Errorf("OperationCursor() after one page = %q, want aaaa1111cccc", got)
		}
		assertContains(t, view(m), "line")

		m = press(t, m, "ctrl+d", "ctrl+d", "ctrl+d", "ctrl+d")

		if got := m.OperationCursor(); got == "aaaa1111cccc" {
			t.Errorf("OperationCursor() = %q, want the cursor past the expanded operation", got)
		}
	})

	t.Run("l wraps a long command so its last arguments stay visible", func(t *testing.T) {
		src := operationSource()
		src.ops[0].Args = "jj squash --into abcdefgh " + strings.Repeat("internal/some/package/file.go ", 5) + "final/operand.go"

		m := press(t, start(t, src, nil, 80, 24), "4", "l")

		assertContains(t, view(m), "jj squash --into abcdefgh", "final/operand.go")
	})

	t.Run("wrapped command lines scroll one row at a time", func(t *testing.T) {
		src := operationSource()
		src.ops[0].Args = "jj squash " + strings.Repeat("internal/some/package/file.go ", 60) + "final/operand.go"

		m := press(t, start(t, src, nil, 80, 20), "4", "l", "ctrl+d", "ctrl+d", "ctrl+d", "ctrl+d")

		if got := m.OperationCursor(); got != "aaaa1111cccc" {
			t.Errorf("OperationCursor() = %q, want aaaa1111cccc", got)
		}
		assertContains(t, view(m), "final/operand.go")
	})

	for _, tt := range []struct{ name, key string }{{"g", "g"}, {"home", "home"}} {
		t.Run(tt.name+" reveals the newest operation from inside its command", func(t *testing.T) {
			src := operationSource()
			src.ops[0].Args = strings.Repeat("line\n", 30) + "end"

			m := press(t, start(t, src, nil, 120, 20), "4", "l", "ctrl+d")
			assertMissing(t, view(m), "aaaa1111")

			m = press(t, m, tt.key)

			assertContains(t, view(m), "aaaa1111")
		})
	}

	for _, tt := range []struct{ name, key string }{{"G", "G"}, {"end", "end"}} {
		t.Run(tt.name+" reveals the oldest operation from inside its command", func(t *testing.T) {
			src := operationSource()
			src.ops[3].Args = strings.Repeat("line\n", 30) + "end"

			m := press(t, start(t, src, nil, 120, 20), "4", "G", "l", "ctrl+d")
			assertMissing(t, view(m), "00000000")

			m = press(t, m, tt.key)

			assertContains(t, view(m), "00000000")
		})
	}

	t.Run("l keeps every character of a command that wraps at a separator", func(t *testing.T) {
		src := operationSource()
		src.ops[0].Args = "jj squash --from feature/operations --into feature/review-operations -- internal/tui/model.go"

		m := press(t, start(t, src, nil, 80, 24), "4", "l")

		got := view(m)
		assertContains(t, got, "feature/review-operations", "internal/tui/model.go")
		if n := strings.Count(got, "--"); n != 3 {
			t.Errorf("View() shows %d of the 3 \"--\" in the command:\n%s", n, got)
		}
	})

	t.Run("k back onto an expanded operation reveals its header", func(t *testing.T) {
		src := operationSource()
		src.ops[0].Args = strings.Repeat("line\n", 30) + "end"

		m := press(t, start(t, src, nil, 120, 20), "4", "l")
		for range 10 {
			if m.OperationCursor() != "aaaa1111cccc" {
				break
			}
			m = press(t, m, "ctrl+d")
		}
		if got := m.OperationCursor(); got != "bbbb2222dddd" {
			t.Fatalf("OperationCursor() after paging = %q, want bbbb2222dddd", got)
		}
		assertMissing(t, view(m), "aaaa1111")

		m = press(t, m, "k")

		if got := m.OperationCursor(); got != "aaaa1111cccc" {
			t.Errorf("OperationCursor() = %q, want aaaa1111cccc", got)
		}
		assertContains(t, view(m), "aaaa1111")
	})

	t.Run("l shows backslashes in the command unchanged", func(t *testing.T) {
		src := operationSource()
		src.ops[0].Args = `jj describe -m 'don'\''t'`

		m := press(t, loaded(t, src), "4", "l")

		assertContains(t, view(m), `jj describe -m 'don'\''t'`)
		assertMissing(t, view(m), `\\`)
	})

	t.Run("l escapes control characters in the command", func(t *testing.T) {
		src := operationSource()
		src.ops[0].Args = "jj describe -m \x1b[31mred"

		m := press(t, loaded(t, src), "4", "l")

		assertContains(t, view(m), `jj describe -m \x1b[31mred`)
	})

	t.Run("the prompt keeps the hunk warning below a long multiline command", func(t *testing.T) {
		src := operationSource()
		src.ops[2].Args = "jj describe -m \"" + strings.Repeat("a long line of the commit message body\n", 40) + "\""

		m := press(t, start(t, src, nil, 80, 24), "tab", "j", "tab", "space", "4", "j", "j", "r")

		assertContains(t, view(m), "Restore operation cccc3333", "Returns the repo to its state", "Drops 1 selected hunk", "y restore", "…")
	})

	for _, action := range []string{"r", "u"} {
		t.Run(action+" on the working copy reviews the new working copy", func(t *testing.T) {
			src := operationSource()
			former := src.change
			former.WorkingCopy = false
			src.revisions[jj.VisibleCommitRevset("commit1")] = former
			src.afterOp = func() {
				src.change = jj.Change{ChangeID: "mnopqrstuvwx", CommitID: "editedcommit", Parents: []string{"parent1"}, Description: "edited", WorkingCopy: true}
			}

			m := press(t, loaded(t, src), "4", "j", "j", action, "y", "1")

			if got := src.changeRevs[len(src.changeRevs)-1]; got != "@" {
				t.Errorf("Change() last rev = %q, want %q", got, "@")
			}
			assertContains(t, view(m), "@ mnopqrst")
		})
	}

	t.Run("abandoning an operation keeps reviewing the same commit", func(t *testing.T) {
		src := operationSource()
		former := src.change
		former.WorkingCopy = false
		src.revisions[jj.VisibleCommitRevset("commit1")] = former
		src.afterOp = func() {
			src.change = jj.Change{ChangeID: "mnopqrstuvwx", CommitID: "editedcommit", Parents: []string{"parent1"}, WorkingCopy: true}
		}

		m := press(t, loaded(t, src), "4", "j", "j", "a", "y", "1")

		if got, want := src.changeRevs[len(src.changeRevs)-1], jj.VisibleCommitRevset("commit1"); got != want {
			t.Errorf("Change() last rev = %q, want %q", got, want)
		}
		assertContains(t, view(m), "@ kxyzabcd")
	})

	t.Run("a restore that hides the reviewed commit reviews the visible commit of its change", func(t *testing.T) {
		src := operationSource()
		restored := jj.Change{ChangeID: "wxyzchild000", CommitID: "childold", Parents: []string{"parentcommit"}, Description: "child feature"}
		src.afterOp = func() {
			src.revErrs = map[string]error{jj.VisibleCommitRevset("childcommit"): noRevision()}
			src.revisions[jj.VisibleRevset(jj.ChangeRevset("wxyzchild000"))] = restored
			src.revFiles["childold"] = []diff.File{mainFile()}
		}

		m := press(t, loaded(t, src), "2", "j", "enter", "4", "j", "j", "r", "y")

		if got := src.diffRevs[len(src.diffRevs)-1]; got != "childold" {
			t.Errorf("Diff() last rev = %q, want %q", got, "childold")
		}
		assertMissing(t, view(m), "reviewing the working copy")
		assertContains(t, view(press(t, m, "1")), "@ wxyzchil")
	})

	t.Run("help lists the operation keys", func(t *testing.T) {
		got := view(press(t, start(t, operationSource(), nil, 120, 50), "4", "?"))

		assertContains(t, got, "OPERATIONS", "restore the repo to the operation", "undo the operation", "abandon the operation from the history", "show the command of the operation", "hide the command of the operation", "operations tab")
	})
}

func TestOperationsTab_failed(t *testing.T) {
	tests := map[string]struct {
		keys   []string
		status string
	}{
		"restoring the current operation":  {keys: []string{"r"}, status: "already at operation aaaa1111"},
		"abandoning the current operation": {keys: []string{"a"}, status: "the current operation cannot be abandoned; press u to undo it first"},
		"undoing the root operation":       {keys: []string{"G", "u"}, status: "the root operation cannot be undone"},
		"abandoning the root operation":    {keys: []string{"G", "a"}, status: "the root operation cannot be abandoned"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			src := operationSource()

			m := press(t, loaded(t, src), append([]string{"4"}, tt.keys...)...)

			assertContains(t, view(m), tt.status)
			assertMissing(t, view(m), "y restore", "y undo", "y abandon")
			press(t, m, "y")
			assertOpRuns(t, src)
		})
	}

	t.Run("a failed restore shows the error", func(t *testing.T) {
		src := operationSource()
		src.opErr = errors.New("jj operation restore: exit status 1")

		m := press(t, loaded(t, src), "4", "j", "j", "r", "y")

		assertContains(t, toastText(t, m), "restore operation cccc3333: jj operation restore: exit status 1")
	})

	t.Run("a restore that leaves the reviewed change divergent reviews the working copy", func(t *testing.T) {
		src := operationSource()
		src.afterOp = func() {
			src.revErrs = map[string]error{
				jj.VisibleCommitRevset("childcommit"):             noRevision(),
				jj.VisibleRevset(jj.ChangeRevset("wxyzchild000")): fmt.Errorf("jj log: %w", jj.ErrMultipleRevisions),
			}
		}

		m := press(t, loaded(t, src), "2", "j", "enter", "4", "j", "j", "r", "y")

		if got := src.changeRevs[len(src.changeRevs)-1]; got != "@" {
			t.Errorf("Change() last rev = %q, want %q", got, "@")
		}
		assertContains(t, view(m), "wxyzchil was rewritten, reviewing the working copy")
	})

	t.Run("a failed load shows the error", func(t *testing.T) {
		src := operationSource()
		src.opsErr = errors.New("boom")

		m := press(t, loaded(t, src), "4")

		assertContains(t, view(m), "error: load operations: boom")
	})
}
