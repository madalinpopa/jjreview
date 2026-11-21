package tui_test

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/madalinpopa/jjreview/internal/diff"
	"github.com/madalinpopa/jjreview/internal/jj"
	"github.com/madalinpopa/jjreview/internal/tui"
)

func logLines() []jj.LogLine {
	return []jj.LogLine{
		{Graph: "@  ", Entry: &jj.LogEntry{ChangeID: "kxyzabcdefgh", CommitID: "commit1", Description: "wip\n", Author: "ana", Timestamp: "2 minutes ago", WorkingCopy: true}},
		{Graph: "│ ○  ", Entry: &jj.LogEntry{ChangeID: "wxyzchild000", CommitID: "childcommit", Description: "child feature\n\nbody mentions sprockets\n", Author: "bob", Timestamp: "1 hour ago", Bookmarks: []string{"feature"}}},
		{Graph: "├─╯"},
		{Graph: "○  ", Entry: &jj.LogEntry{ChangeID: "pqrsparent00", CommitID: "parentcommit", Description: "parent\n", Author: "ana", Timestamp: "2 hours ago"}},
		{Graph: "◆  ", Entry: &jj.LogEntry{ChangeID: "zzzzroot0000", CommitID: "rootcommit", Author: "root", Timestamp: "3 days ago", Immutable: true}},
		{Graph: "~"},
	}
}

func assertLogTab(t *testing.T, m tui.Model) {
	t.Helper()
	got := view(m)
	assertContains(t, got, "LOG")
	assertMissing(t, got, "FILES", "UNIFIED DIFF")
}

func assertReviewTab(t *testing.T, m tui.Model) {
	t.Helper()
	got := view(m)
	assertContains(t, got, "FILES", "UNIFIED DIFF")
	assertMissing(t, got, "LOG")
}

func TestLogTab_success(t *testing.T) {
	t.Run("2 shows the log and loads it once", func(t *testing.T) {
		src := revisionSource()

		m := press(t, loaded(t, src), "2", "1", "2")

		assertLogTab(t, m)
		if src.logCalls != 1 {
			t.Errorf("Log() calls = %d, want 1", src.logCalls)
		}
	})

	t.Run("1 goes back to the review", func(t *testing.T) {
		m := press(t, loaded(t, revisionSource()), "2", "1")

		assertReviewTab(t, m)
	})

	t.Run("the log is not loaded before it is shown", func(t *testing.T) {
		src := revisionSource()

		loaded(t, src)

		if src.logCalls != 0 {
			t.Errorf("Log() calls = %d, want 0", src.logCalls)
		}
	})

	t.Run("ctrl+r refreshes the log", func(t *testing.T) {
		src := revisionSource()
		m := press(t, loaded(t, src), "2")
		src.log = append(src.log, jj.LogLine{Graph: "○  ", Entry: &jj.LogEntry{ChangeID: "newnewnew000", Description: "brand new"}})

		m = press(t, m, "ctrl+r")

		assertContains(t, view(m), "brand new")
		if src.logCalls != 2 {
			t.Errorf("Log() calls = %d, want 2", src.logCalls)
		}
	})

	t.Run("R in the review marks the log stale", func(t *testing.T) {
		src := revisionSource()

		press(t, loaded(t, src), "2", "1", "R", "2")

		if src.logCalls != 2 {
			t.Errorf("Log() calls = %d, want 2", src.logCalls)
		}
	})

	t.Run("a commit marks the log stale", func(t *testing.T) {
		src := revisionSource()
		m := press(t, loaded(t, src), "2", "1", "tab", "j", "tab", "space", "c", "ctrl+s")

		press(t, m, "2")

		if src.logCalls != 2 {
			t.Errorf("Log() calls = %d, want 2", src.logCalls)
		}
	})

	t.Run("@ marks the log stale", func(t *testing.T) {
		src := revisionSource()

		press(t, loaded(t, src), "2", "1", "@", "2")

		if src.logCalls != 2 {
			t.Errorf("Log() calls = %d, want 2", src.logCalls)
		}
	})

	t.Run("@ refreshes the log while it is shown", func(t *testing.T) {
		src := revisionSource()
		m := press(t, loaded(t, src), "2", "1")
		m, back := update(t, m, keyMsg("@"))
		m = press(t, m, "2")
		src.log = append(logLines(), jj.LogLine{Graph: "○  ", Entry: &jj.LogEntry{ChangeID: "newnewnew000", CommitID: "newcommit", Description: "brand new"}})

		m = run(t, m, back)

		assertContains(t, view(m), "brand new")
		if src.logCalls != 2 {
			t.Errorf("Log() calls = %d, want 2", src.logCalls)
		}
	})

	t.Run("1 is typed into the commit message", func(t *testing.T) {
		m := press(t, loaded(t, revisionSource()), "tab", "j", "tab", "space", "c", "ctrl+u")

		m = press(t, m, "1", "2")

		if got := m.Message(); got != "12" {
			t.Errorf("Message() = %q, want %q", got, "12")
		}
		if !m.ModalOpen() {
			t.Error("ModalOpen() = false, want true")
		}
	})

	t.Run("q quits from the log", func(t *testing.T) {
		m := press(t, loaded(t, revisionSource()), "2")

		_, cmd := update(t, m, keyMsg("q"))

		if cmd == nil {
			t.Fatal("Update(q) cmd = nil, want tea.Quit")
		}
	})
}

func TestLogTab_outdatedResponse(t *testing.T) {
	src := revisionSource()
	m := loaded(t, src)
	m, first := update(t, m, keyMsg("2"))
	m = press(t, m, "1")
	m, back := update(t, m, keyMsg("@"))
	m = press(t, m, "2")
	src.log = []jj.LogLine{{Graph: "@  ", Entry: &jj.LogEntry{ChangeID: "kxyzabcdefgh", Description: "fresh", WorkingCopy: true}}}
	m = run(t, m, back)
	src.log = logLines()

	m = run(t, m, first)

	assertContains(t, view(m), "fresh")
	assertMissing(t, view(m), "child feature")
}

func TestLogTab_refreshWhileLoading(t *testing.T) {
	src := revisionSource()
	m := press(t, loaded(t, src), "2")
	m, first := update(t, m, keyMsg("ctrl+r"))
	m, second := update(t, m, keyMsg("ctrl+r"))

	m = run(t, m, first)
	run(t, m, second)

	if src.logCalls != 2 {
		t.Errorf("Log() calls = %d, want 2", src.logCalls)
	}
}

func TestLogTab_failed(t *testing.T) {
	src := revisionSource()
	src.logErr = errors.New("jj exploded")

	m := press(t, loaded(t, src), "2")

	assertLogTab(t, m)
	assertContains(t, view(m), "jj exploded")
}

func TestLogCursor_success(t *testing.T) {
	tests := map[string]struct {
		keys []string
		want string
	}{
		"starts on the revision under review": {keys: []string{"2"}, want: "kxyzabcdefgh"},
		"starts on the reviewed parent":       {keys: append(reviewParent(), "2"), want: "pqrsparent00"},
		"j moves to the next revision":        {keys: []string{"2", "j"}, want: "wxyzchild000"},
		"j skips connector lines":             {keys: []string{"2", "j", "j"}, want: "pqrsparent00"},
		"down is j":                           {keys: []string{"2", "down", "down"}, want: "pqrsparent00"},
		"k skips connector lines":             {keys: []string{"2", "j", "j", "k"}, want: "wxyzchild000"},
		"up is k":                             {keys: []string{"2", "j", "up"}, want: "kxyzabcdefgh"},
		"k stops at the first revision":       {keys: []string{"2", "k"}, want: "kxyzabcdefgh"},
		"j stops at the last revision":        {keys: []string{"2", "j", "j", "j", "j", "j"}, want: "zzzzroot0000"},
		"G moves to the last revision":        {keys: []string{"2", "G"}, want: "zzzzroot0000"},
		"g moves to the first revision":       {keys: []string{"2", "G", "g"}, want: "kxyzabcdefgh"},
		"ctrl+d moves half a page down":       {keys: []string{"2", "ctrl+d"}, want: "zzzzroot0000"},
		"ctrl+u moves half a page up":         {keys: []string{"2", "G", "ctrl+u"}, want: "kxyzabcdefgh"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			m := press(t, loaded(t, revisionSource()), tt.keys...)

			if got := m.LogCursor(); got != tt.want {
				t.Errorf("LogCursor() = %q, want %q", got, tt.want)
			}
		})
	}

	t.Run("starts on the working copy when the reviewed revision is not listed", func(t *testing.T) {
		src := revisionSource()
		src.log = src.log[1:]
		src.log[0].Entry.WorkingCopy = true

		m := press(t, loaded(t, src), "2")

		if got := m.LogCursor(); got != "wxyzchild000" {
			t.Errorf("LogCursor() = %q, want %q", got, "wxyzchild000")
		}
	})

	t.Run("starts on the first revision without a working copy", func(t *testing.T) {
		src := revisionSource()
		src.log = src.log[2:]

		m := press(t, loaded(t, src), "2")

		if got := m.LogCursor(); got != "pqrsparent00" {
			t.Errorf("LogCursor() = %q, want %q", got, "pqrsparent00")
		}
	})
}

func TestLogOpen_success(t *testing.T) {
	t.Run("enter reviews the revision under the cursor", func(t *testing.T) {
		src := revisionSource()

		m := press(t, loaded(t, src), "2", "j", "enter")

		if got, want := src.changeRevs[len(src.changeRevs)-1], jj.VisibleCommitRevset("childcommit"); got != want {
			t.Errorf("Change() last rev = %q, want %q", got, want)
		}
		assertReviewTab(t, m)
		assertContains(t, view(m), "@ wxyzchil")
		if got := m.CurrentFile(); got != "src/main.go" {
			t.Errorf("CurrentFile() = %q, want %q", got, "src/main.go")
		}
	})

	t.Run("enter on the reviewed revision goes back without reloading", func(t *testing.T) {
		src := revisionSource()
		m := loaded(t, src)
		calls := src.diffCalls

		m = press(t, m, "2", "enter")

		assertReviewTab(t, m)
		if got := src.diffCalls; got != calls {
			t.Errorf("Diff() calls = %d, want %d", got, calls)
		}
	})

	t.Run("enter asks again before dropping selected hunks", func(t *testing.T) {
		src := revisionSource()
		m := press(t, loaded(t, src), "tab", "j", "tab", "space", "2", "j")
		calls := len(src.changeRevs)

		m = press(t, m, "enter")

		assertLogTab(t, m)
		assertContains(t, view(m), "press enter again to drop 1 selected hunk")
		if got := len(src.changeRevs); got != calls {
			t.Errorf("Change() calls = %d, want %d", got, calls)
		}

		m = press(t, m, "enter")

		assertReviewTab(t, m)
		assertContains(t, view(m), "@ wxyzchil", "nothing selected")
	})
}

func TestLogOpen_confirmationFollowsSelection(t *testing.T) {
	src := revisionSource()
	m := press(t, loaded(t, src), "tab", "j", "tab", "space", "2")
	m, refresh := update(t, m, keyMsg("ctrl+r"))
	m = press(t, m, "j", "enter")
	assertContains(t, view(m), "press enter again")
	src.log = logLines()
	src.log[0].Entry.CommitID = "commit2"
	m = run(t, m, refresh)
	assertMissing(t, view(m), "press enter again")
	calls := len(src.changeRevs)

	m = press(t, m, "enter")

	if got := len(src.changeRevs); got != calls {
		t.Errorf("Change() calls = %d, want %d", got, calls)
	}
	assertLogTab(t, m)
	assertContains(t, view(m), "press enter again to drop 1 selected hunk")
}

func TestLogRefresh_keepsReviewConfirmation(t *testing.T) {
	src := revisionSource()
	m := press(t, loaded(t, src), "tab", "j", "tab", "space", "2")
	m, refresh := update(t, m, keyMsg("ctrl+r"))
	m = press(t, m, "1", "q")
	src.log = logLines()
	src.log[0].Entry.CommitID = "commit2"

	m = run(t, m, refresh)

	assertContains(t, view(m), "press q again to drop 1 selected hunk")
	_, cmd := update(t, m, keyMsg("q"))
	if cmd == nil {
		t.Fatal("Update(q) cmd = nil, want tea.Quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Error("Update(q) cmd does not quit")
	}
}

func divergentSource() *fakeSource {
	src := revisionSource()
	src.log = []jj.LogLine{
		{Graph: "@  ", Entry: &jj.LogEntry{ChangeID: "kxyzabcdefgh", CommitID: "commit1", Description: "wip\n", WorkingCopy: true}},
		{Graph: "○  ", Entry: &jj.LogEntry{ChangeID: "dupdupdup000", CommitID: "divergentone", Description: "first copy\n"}},
		{Graph: "○  ", Entry: &jj.LogEntry{ChangeID: "dupdupdup000", CommitID: "divergenttwo", Description: "second copy\n"}},
	}
	src.revisions[jj.VisibleCommitRevset("divergentone")] = jj.Change{ChangeID: "dupdupdup000", CommitID: "divergentone", Description: "first copy\n"}
	src.revisions[jj.VisibleCommitRevset("divergenttwo")] = jj.Change{ChangeID: "dupdupdup000", CommitID: "divergenttwo", Description: "second copy\n"}
	src.revFiles["divergentone"] = []diff.File{readmeFile()}
	src.revFiles["divergenttwo"] = []diff.File{mainFile()}
	return src
}

func TestLogOpen_divergent(t *testing.T) {
	t.Run("enter reviews the picked commit", func(t *testing.T) {
		src := divergentSource()

		m := press(t, loaded(t, src), "2", "j", "j", "enter")

		if got, want := src.changeRevs[len(src.changeRevs)-1], jj.VisibleCommitRevset("divergenttwo"); got != want {
			t.Errorf("Change() last rev = %q, want %q", got, want)
		}
		if got := m.CurrentFile(); got != "src/main.go" {
			t.Errorf("CurrentFile() = %q, want %q", got, "src/main.go")
		}
	})

	t.Run("only the reviewed commit is marked", func(t *testing.T) {
		got := view(press(t, loaded(t, divergentSource()), "2", "j", "j", "enter", "2"))

		assertContains(t, lineWith(t, got, "dupdupdu second copy"), "▌")
		assertMissing(t, lineWith(t, got, "dupdupdu first copy"), "▌")
	})

	t.Run("enter on the sibling of the reviewed commit loads it", func(t *testing.T) {
		src := divergentSource()
		m := press(t, loaded(t, src), "2", "j", "j", "enter", "2", "k")

		m = press(t, m, "enter")

		if got, want := src.changeRevs[len(src.changeRevs)-1], jj.VisibleCommitRevset("divergentone"); got != want {
			t.Errorf("Change() last rev = %q, want %q", got, want)
		}
		if got := m.CurrentFile(); got != "README.md" {
			t.Errorf("CurrentFile() = %q, want %q", got, "README.md")
		}
	})
}

func TestLogOpen_failed(t *testing.T) {
	t.Run("a failed load stays in the log", func(t *testing.T) {
		src := revisionSource()
		m := press(t, loaded(t, src), "2", "j")
		src.revErrs = map[string]error{jj.VisibleCommitRevset("childcommit"): errors.New("boom")}

		m = press(t, m, "enter")

		assertLogTab(t, m)
		assertContains(t, view(m), "boom", "@ kxyzabcd")
	})

	t.Run("enter is blocked after an incomplete split", func(t *testing.T) {
		src := revisionSource()
		src.splitErr = errors.Join(jj.ErrSplitIncomplete, errors.New("reload failed"))
		m := press(t, loaded(t, src), "tab", "j", "tab", "space", "c", "ctrl+s", "2", "j")
		calls := len(src.changeRevs)

		m = press(t, m, "enter")

		if got := len(src.changeRevs); got != calls {
			t.Errorf("Change() calls = %d, want %d", got, calls)
		}
		assertContains(t, view(m), "split done; restart jjr to continue")
	})
}

func TestLogView_success(t *testing.T) {
	t.Run("the top bar shows both tabs", func(t *testing.T) {
		got := view(loaded(t, revisionSource()))

		assertContains(t, lineWith(t, got, "@ kxyzabcd"), "1 Review", "2 Log")
	})

	t.Run("the log card lists every revision", func(t *testing.T) {
		got := view(press(t, loaded(t, revisionSource()), "2"))

		assertContains(t, got, "rev 1/4", "child feature", "feature", "1 hour ago", "bob", "├─╯", "(no description set)")
		assertContains(t, lineWith(t, got, "child feature"), "│ ○  wxyzchil")
		assertMissing(t, got, "sprockets")
	})

	t.Run("a narrow log keeps the ID and drops the metadata", func(t *testing.T) {
		got := view(press(t, start(t, revisionSource(), nil, 34, 20), "2"))

		line := lineWith(t, got, "│ ○  wxyzchil")
		assertContains(t, line, "child")
		assertMissing(t, line, "1 hour ago")
	})

	t.Run("a very narrow log keeps the ID", func(t *testing.T) {
		got := view(press(t, start(t, revisionSource(), nil, 24, 20), "2"))

		assertContains(t, got, "│ ○  wxyzchil")
	})

	t.Run("a wide log keeps the metadata beside a long description", func(t *testing.T) {
		src := revisionSource()
		src.log[1].Entry.Description = strings.Repeat("long words ", 20) + "\n"

		got := view(press(t, start(t, src, nil, 120, 20), "2"))

		assertContains(t, lineWith(t, got, "│ ○  wxyzchil"), "bob · 1 hour ago")
	})

	t.Run("a revision checked out in other workspaces shows their names", func(t *testing.T) {
		src := revisionSource()
		src.log[1].Entry.Workspaces = []string{"p1-ui", "p2-api"}

		got := view(press(t, start(t, src, nil, 120, 20), "2"))

		assertContains(t, lineWith(t, got, "│ ○  wxyzchil"), "p1-ui@", "p2-api@", "feature", "bob · 1 hour ago")
	})

	t.Run("workspace names count toward the width that drops the metadata", func(t *testing.T) {
		src := revisionSource()
		src.log[1].Entry.Workspaces = []string{"p1-ui"}

		got := view(press(t, start(t, src, nil, 60, 20), "2"))

		line := lineWith(t, got, "│ ○  wxyzchil")
		assertContains(t, line, "p1-ui@", "child")
		assertMissing(t, line, "1 hour ago")
	})

	t.Run("the default workspace name is not shown", func(t *testing.T) {
		src := revisionSource()
		src.log[0].Entry.Workspaces = []string{"default"}

		got := view(press(t, start(t, src, nil, 120, 20), "2"))

		assertMissing(t, lineWith(t, got, "@  kxyzabcd"), "default")
	})

	t.Run("the key bar lists the log keys", func(t *testing.T) {
		got := view(press(t, loaded(t, revisionSource()), "2"))

		assertContains(t, got, "1 2 3 tabs", "j k revision", "enter review", "space select", "f fetch", "P push", "? help")
		assertMissing(t, got, "s squash", "R reset", "R refresh")
		assertMissing(t, got, "c commit", "file 1/4")
	})

	t.Run("help lists the log keys", func(t *testing.T) {
		got := view(press(t, start(t, revisionSource(), nil, 120, 50), "2", "?"))

		assertContains(t, got, "VIEWS", "review tab", "log tab", "review the revision", "refresh the log")
		assertMissing(t, got, "fold all hunks")
	})

	t.Run("help in the review lists the tabs", func(t *testing.T) {
		got := view(press(t, start(t, revisionSource(), nil, 120, 50), "?"))

		assertContains(t, got, "VIEWS", "review tab", "log tab", "fold all hunks")
	})

	t.Run("loading shows a placeholder", func(t *testing.T) {
		m, _ := update(t, loaded(t, revisionSource()), keyMsg("2"))

		assertContains(t, view(m), "loading…")
	})

	t.Run("keeps the cursor visible in a long log", func(t *testing.T) {
		src := revisionSource()
		src.log = nil
		for i := range 60 {
			src.log = append(src.log, jj.LogLine{Graph: "○  ", Entry: &jj.LogEntry{ChangeID: string(rune('a'+i%26)) + "change" + string(rune('a'+i/26)), Description: "entry " + string(rune('A'+i%26)) + string(rune('a'+i/26))}})
		}
		m := start(t, src, nil, 100, 20)

		m = press(t, m, "2", "G")

		assertContains(t, view(m), "entry Hc")
		assertMissing(t, view(m), "entry Aa")
	})
}

func typeKeys(text string) []string {
	var keys []string
	for _, r := range text {
		keys = append(keys, string(r))
	}
	return keys
}

func search(query string) []string {
	return append([]string{"2", "/"}, typeKeys(query)...)
}

func TestLogSearch_success(t *testing.T) {
	tests := map[string]struct {
		query string
		want  string
	}{
		"matches the description":           {query: "feature", want: "wxyzchild000"},
		"ignores case in the description":   {query: "FEATURE", want: "wxyzchild000"},
		"matches a change ID prefix":        {query: "pqrs", want: "pqrsparent00"},
		"matches a commit ID prefix":        {query: "childc", want: "wxyzchild000"},
		"ignores case in the ID":            {query: "PQRS", want: "pqrsparent00"},
		"matches a substring of a sentence": {query: "arent", want: "pqrsparent00"},
		"matches the description body":      {query: "sprockets", want: "wxyzchild000"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			m := press(t, loaded(t, revisionSource()), search(tt.query)...)

			if got := m.LogCursor(); got != tt.want {
				t.Errorf("LogCursor() = %q, want %q", got, tt.want)
			}
			assertContains(t, view(m), "rev 1/1", "/ "+tt.query)
		})
	}

	t.Run("pasted text goes to the query", func(t *testing.T) {
		m := press(t, loaded(t, revisionSource()), search("")...)

		m = run(t, m, func() tea.Msg { return tea.PasteMsg{Content: "PQRS"} })

		if got := m.LogCursor(); got != "pqrsparent00" {
			t.Errorf("LogCursor() = %q, want %q", got, "pqrsparent00")
		}
		assertContains(t, view(m), "rev 1/1", "/ PQRS")
	})

	t.Run("hides connector lines while filtering", func(t *testing.T) {
		m := press(t, loaded(t, revisionSource()), search("e")...)

		assertMissing(t, view(m), "├─╯")
	})

	t.Run("typed keys go to the query instead of running commands", func(t *testing.T) {
		m := press(t, loaded(t, revisionSource()), search("q1j")...)

		assertLogTab(t, m)
		assertContains(t, view(m), "/ q1j", "no matching revisions")
	})

	t.Run("enter keeps the filter and moves through the matches", func(t *testing.T) {
		m := press(t, loaded(t, revisionSource()), search("a")...)
		m = press(t, m, "enter", "j")

		if got := m.LogCursor(); got != "pqrsparent00" {
			t.Errorf("LogCursor() = %q, want %q", got, "pqrsparent00")
		}
		assertContains(t, view(m), "rev 2/2", "/ a")
	})

	t.Run("enter after applying the filter reviews the match", func(t *testing.T) {
		src := revisionSource()

		m := press(t, loaded(t, src), search("parent")...)
		m = press(t, m, "enter", "enter")

		if got, want := src.changeRevs[len(src.changeRevs)-1], jj.VisibleCommitRevset("parentcommit"); got != want {
			t.Errorf("Change() last rev = %q, want %q", got, want)
		}
		assertReviewTab(t, m)
	})

	t.Run("esc while typing clears the filter", func(t *testing.T) {
		m := press(t, loaded(t, revisionSource()), search("pqrs")...)
		m = press(t, m, "esc")

		got := view(m)
		assertContains(t, got, "rev 3/4", "├─╯")
		assertMissing(t, got, "/ pqrs")
		if cursor := m.LogCursor(); cursor != "pqrsparent00" {
			t.Errorf("LogCursor() = %q, want the match kept", cursor)
		}
	})

	t.Run("esc after applying clears the filter", func(t *testing.T) {
		m := press(t, loaded(t, revisionSource()), search("pqrs")...)
		m = press(t, m, "enter", "esc")

		assertContains(t, view(m), "rev 3/4")
	})

	t.Run("backspace widens the filter", func(t *testing.T) {
		m := press(t, loaded(t, revisionSource()), search("pqrsx")...)
		m = press(t, m, "backspace")

		assertContains(t, view(m), "rev 1/1", "/ pqrs")
	})

	t.Run("refresh keeps the filter", func(t *testing.T) {
		m := press(t, loaded(t, revisionSource()), search("pqrs")...)
		m = press(t, m, "enter", "R")

		assertContains(t, view(m), "rev 1/1", "/ pqrs")
	})

	t.Run("shows the search keys", func(t *testing.T) {
		src := revisionSource()

		typing := view(press(t, loaded(t, src), search("p")...))
		listing := view(press(t, loaded(t, src), "2"))

		assertContains(t, typing, "enter apply", "esc clear")
		assertContains(t, listing, "/ search")
	})
}

func diffCallsFor(src *fakeSource, rev string) int {
	n := 0
	for _, r := range src.diffRevs {
		if r == rev {
			n++
		}
	}
	return n
}

func manyFiles(n int) []diff.File {
	var files []diff.File
	for i := range n {
		files = append(files, diff.File{NewPath: fmt.Sprintf("file%02d.go", i), OldPath: fmt.Sprintf("file%02d.go", i)})
	}
	return files
}

func TestLogExpand_success(t *testing.T) {
	t.Run("l lists the files changed by the revision", func(t *testing.T) {
		src := revisionSource()
		src.revFiles["childcommit"] = []diff.File{
			mainFile(),
			{NewPath: "docs/new.md", Status: diff.StatusAdded},
			{OldPath: "old.txt", Status: diff.StatusDeleted},
			{OldPath: "a.go", NewPath: "b.go", Status: diff.StatusRenamed},
		}

		m := press(t, loaded(t, src), "2", "j", "l")

		got := view(m)
		assertContains(t, got, "M src/main.go", "A docs/new.md", "D old.txt", "R a.go → b.go")
		assertContains(t, lineWith(t, got, "src/main.go"), "│ │")
	})

	t.Run("l escapes control characters in file names", func(t *testing.T) {
		src := revisionSource()
		src.revFiles["childcommit"] = []diff.File{
			{NewPath: "a\nb.go", Status: diff.StatusAdded},
			{OldPath: "c\rd.go", NewPath: "e\nf.go", Status: diff.StatusRenamed},
		}

		got := view(press(t, loaded(t, src), "2", "j", "l"))

		assertRows(t, got, "child feature", `A a\nb.go`, `R c\rd.go → e\nf.go`, "├─╯")
	})

	t.Run("l again keeps the files shown", func(t *testing.T) {
		m := press(t, loaded(t, revisionSource()), "2", "j", "l", "l")

		assertContains(t, view(m), "src/main.go")
	})

	t.Run("h hides the files", func(t *testing.T) {
		m := press(t, loaded(t, revisionSource()), "2", "j", "l", "h")

		assertMissing(t, view(m), "src/main.go")
	})

	t.Run("h on a collapsed revision does nothing", func(t *testing.T) {
		m := press(t, loaded(t, revisionSource()), "2", "j", "h")

		assertMissing(t, view(m), "src/main.go")
		assertLogTab(t, m)
	})

	t.Run("loads the files of a revision once", func(t *testing.T) {
		src := revisionSource()

		press(t, loaded(t, src), "2", "j", "l", "h", "l")

		if n := diffCallsFor(src, "childcommit"); n != 1 {
			t.Errorf("Diff(childcommit) calls = %d, want 1", n)
		}
	})

	t.Run("the cursor skips the file rows", func(t *testing.T) {
		m := press(t, loaded(t, revisionSource()), "2", "j", "l", "j")

		if got := m.LogCursor(); got != "pqrsparent00" {
			t.Errorf("LogCursor() = %q, want %q", got, "pqrsparent00")
		}
	})

	t.Run("a revision without changes says so", func(t *testing.T) {
		src := revisionSource()
		src.revFiles["rootcommit"] = []diff.File{}

		m := press(t, loaded(t, src), "2", "G", "l")

		assertContains(t, view(m), "no changes")
	})

	t.Run("keeps the cursor visible below expanded files", func(t *testing.T) {
		src := revisionSource()
		src.revFiles["childcommit"] = manyFiles(20)

		m := press(t, start(t, src, nil, 80, 14), "2", "j", "l", "j")

		assertContains(t, view(m), "pqrspare")
	})

	t.Run("ctrl+d scrolls through a tall file list", func(t *testing.T) {
		src := revisionSource()
		src.revFiles["rootcommit"] = manyFiles(20)
		m := press(t, start(t, src, nil, 80, 14), "2", "G", "l")

		m = press(t, m, slices.Repeat([]string{"ctrl+d"}, 8)...)

		got := view(m)
		assertContains(t, got, "file19.go")
		assertMissing(t, got, "zzzzroot")
		if cursor := m.LogCursor(); cursor != "zzzzroot0000" {
			t.Errorf("LogCursor() = %q, want %q", cursor, "zzzzroot0000")
		}
	})

	t.Run("ctrl+u scrolls back to the top of a tall file list", func(t *testing.T) {
		src := revisionSource()
		src.revFiles["rootcommit"] = manyFiles(20)
		m := press(t, start(t, src, nil, 80, 14), "2", "G", "l")
		m = press(t, m, slices.Repeat([]string{"ctrl+d"}, 8)...)

		m = press(t, m, slices.Repeat([]string{"ctrl+u"}, 5)...)

		got := view(m)
		assertContains(t, got, "zzzzroot", "file00.go")
		assertMissing(t, got, "file19.go")
		if cursor := m.LogCursor(); cursor != "zzzzroot0000" {
			t.Errorf("LogCursor() = %q, want %q", cursor, "zzzzroot0000")
		}
	})

	t.Run("shows the expand key", func(t *testing.T) {
		m := press(t, loaded(t, revisionSource()), "2")

		assertContains(t, view(m), "l h files")
	})
}

func TestLogExpand_failed(t *testing.T) {
	src := revisionSource()
	src.diffErrs = map[string]error{"childcommit": errors.New("diff exploded")}

	m := press(t, loaded(t, src), "2", "j", "l")

	assertContains(t, view(m), "diff exploded")
	assertLogTab(t, m)
}

func TestLogExpand_retryAfterFailure(t *testing.T) {
	src := revisionSource()
	src.diffErrs = map[string]error{"childcommit": errors.New("diff exploded")}
	m := press(t, loaded(t, src), "2", "j", "l")
	src.diffErrs = nil

	m = press(t, m, "h", "l")

	got := view(m)
	assertContains(t, got, "M src/main.go")
	assertMissing(t, got, "diff exploded")
}

func TestLogExpand_loadingOnce(t *testing.T) {
	src := revisionSource()
	m := press(t, loaded(t, src), "2", "j")
	m, first := update(t, m, keyMsg("l"))
	m, second := update(t, m, keyMsg("l"))

	m = run(t, m, first)
	run(t, m, second)

	if n := diffCallsFor(src, "childcommit"); n != 1 {
		t.Errorf("Diff(childcommit) calls = %d, want 1", n)
	}
}

func TestLogOpen_selectionWhilePending(t *testing.T) {
	for _, k := range []string{"space", "S"} {
		t.Run(k+" is blocked while the picked revision loads", func(t *testing.T) {
			src := revisionSource()
			m := press(t, loaded(t, src), "2", "j")
			m, pending := update(t, m, keyMsg("enter"))

			m = press(t, m, "1", "tab", "j", "tab", k)

			assertContains(t, view(m), "reloading change…", "nothing selected")
			m = run(t, m, pending)
			assertContains(t, view(m), "@ wxyzchil", "nothing selected")
		})
	}
}

func TestLogTab_staleDuringRefresh(t *testing.T) {
	t.Run("reloads again when the log is shown", func(t *testing.T) {
		src := revisionSource()
		m := press(t, loaded(t, src), "2")
		m, refresh := update(t, m, keyMsg("ctrl+r"))
		m = press(t, m, "1", "tab", "j", "tab", "space", "c", "ctrl+s", "2")

		run(t, m, refresh)

		if src.logCalls != 3 {
			t.Errorf("Log() calls = %d, want 3", src.logCalls)
		}
	})

	t.Run("stays stale while the review is shown", func(t *testing.T) {
		src := revisionSource()
		m := press(t, loaded(t, src), "2")
		m, refresh := update(t, m, keyMsg("ctrl+r"))
		m = press(t, m, "1", "tab", "j", "tab", "space", "c", "ctrl+s")

		m = run(t, m, refresh)

		if src.logCalls != 2 {
			t.Errorf("Log() calls after the response = %d, want 2", src.logCalls)
		}
		press(t, m, "2")
		if src.logCalls != 3 {
			t.Errorf("Log() calls after showing the log = %d, want 3", src.logCalls)
		}
	})
}

func TestLogCursor_reviewLoadedAfterLog(t *testing.T) {
	launch := func(t *testing.T) (tui.Model, tea.Cmd) {
		t.Helper()
		src := revisionSource()
		src.change = src.revisions[jj.VisibleCommitRevset("parentcommit")]
		src.files = []diff.File{readmeFile()}
		m := tui.New(t.Context(), src, nil, "pqrsparent00")
		m, _ = update(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
		initial := m.Init()
		m, showLog := update(t, m, keyMsg("2"))
		return run(t, m, showLog), initial
	}

	t.Run("focuses the reviewed revision", func(t *testing.T) {
		m, initial := launch(t)

		m = run(t, m, initial)

		if got := m.LogCursor(); got != "pqrsparent00" {
			t.Errorf("LogCursor() = %q, want %q", got, "pqrsparent00")
		}
	})

	t.Run("keeps a cursor the user moved", func(t *testing.T) {
		m, initial := launch(t)
		m = press(t, m, "G")

		m = run(t, m, initial)

		if got := m.LogCursor(); got != "zzzzroot0000" {
			t.Errorf("LogCursor() = %q, want %q", got, "zzzzroot0000")
		}
	})
}

func TestLogSearch_longQuery(t *testing.T) {
	query := "abcdefghijklmnopqrstuvwxyz0123456789"

	m := press(t, start(t, revisionSource(), nil, 30, 20), search(query)...)

	assertContains(t, lineWith(t, view(m), "/ "), "456789")
}

func TestLogSearch_resizeKeepsTheCaretVisible(t *testing.T) {
	query := "abcdefghijklmnopqrstuvwxyz0123456789"

	t.Run("caret at the end", func(t *testing.T) {
		m := press(t, start(t, revisionSource(), nil, 80, 20), search(query)...)

		m, _ = update(t, m, tea.WindowSizeMsg{Width: 30, Height: 20})

		assertContains(t, lineWith(t, view(m), "/ "), "456789")
	})

	t.Run("caret inside the query", func(t *testing.T) {
		m := press(t, start(t, revisionSource(), nil, 80, 20), search(query)...)
		m = press(t, m, slices.Repeat([]string{"left"}, 10)...)

		m, _ = update(t, m, tea.WindowSizeMsg{Width: 30, Height: 20})

		assertContains(t, lineWith(t, view(m), "/ "), "xyz0")
	})
}

func TestLogSearch_notLoaded(t *testing.T) {
	t.Run("/ while the log loads does not capture keys", func(t *testing.T) {
		src := revisionSource()
		m, showLog := update(t, loaded(t, src), keyMsg("2"))

		m = press(t, m, "/", "x")
		m = run(t, m, showLog)

		got := view(m)
		assertMissing(t, got, "/ x")
		assertContains(t, got, "child feature", "parent")
	})

	t.Run("/ after a failed load does not capture keys", func(t *testing.T) {
		src := revisionSource()
		src.logErr = errors.New("jj exploded")
		m := press(t, loaded(t, src), "2")
		src.logErr = nil

		m = press(t, m, "/", "ctrl+r")

		if src.logCalls != 2 {
			t.Errorf("Log() calls = %d, want 2", src.logCalls)
		}
		got := view(m)
		assertMissing(t, got, "jj exploded")
		assertContains(t, got, "child feature")
	})
}

func TestLogSearch_ctrlVReadsTheClipboard(t *testing.T) {
	m := press(t, loaded(t, revisionSource()), search("")...)

	_, cmd := update(t, m, tea.KeyPressMsg{Code: 'v', Mod: tea.ModCtrl})

	if cmd == nil {
		t.Error("Update(ctrl+v) cmd = nil, want a clipboard read")
	}
}

func TestLogExpand_jumpShowsTheHeader(t *testing.T) {
	t.Run("g on the first revision", func(t *testing.T) {
		src := revisionSource()
		src.revFiles["commit1"] = manyFiles(20)
		m := press(t, start(t, src, nil, 80, 14), "2", "l", "ctrl+d", "ctrl+d")
		assertMissing(t, view(m), "kxyzabcd wip")

		m = press(t, m, "g")

		assertContains(t, view(m), "kxyzabcd wip")
	})

	t.Run("G on the last revision", func(t *testing.T) {
		src := revisionSource()
		src.revFiles["rootcommit"] = manyFiles(20)
		m := press(t, start(t, src, nil, 80, 14), "2", "G", "l")
		m = press(t, m, slices.Repeat([]string{"ctrl+d"}, 8)...)
		assertMissing(t, view(m), "zzzzroot")

		m = press(t, m, "G")

		assertContains(t, view(m), "zzzzroot")
	})
}

func TestLogSearch_failedRefresh(t *testing.T) {
	searchDuringFailedRefresh := func(t *testing.T, src *fakeSource) tui.Model {
		t.Helper()
		m := press(t, loaded(t, src), "2")
		m, refresh := update(t, m, keyMsg("ctrl+r"))
		m = press(t, m, "/", "c", "h", "i", "l", "d")
		src.logErr = errors.New("jj exploded")
		m = run(t, m, refresh)
		src.logErr = nil
		return m
	}

	t.Run("ctrl+r retries and keeps the query", func(t *testing.T) {
		src := revisionSource()
		m := searchDuringFailedRefresh(t, src)

		m = press(t, m, "ctrl+r")

		if src.logCalls != 3 {
			t.Errorf("Log() calls = %d, want 3", src.logCalls)
		}
		got := view(m)
		assertMissing(t, got, "jj exploded", "parent")
		assertContains(t, got, "child feature")
	})

	t.Run("1 switches to the review tab", func(t *testing.T) {
		m := searchDuringFailedRefresh(t, revisionSource())

		m = press(t, m, "1")

		assertReviewTab(t, m)
	})
}
