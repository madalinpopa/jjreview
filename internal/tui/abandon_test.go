package tui_test

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/madalinpopa/jjreview/internal/diff"
	"github.com/madalinpopa/jjreview/internal/jj"
	"github.com/madalinpopa/jjreview/internal/tui"
)

func withoutEntry(lines []jj.LogLine, commitID string) []jj.LogLine {
	return slices.DeleteFunc(lines, func(l jj.LogLine) bool { return l.Entry != nil && l.Entry.CommitID == commitID })
}

func assertAbandoned(t *testing.T, src *fakeSource, want ...string) {
	t.Helper()
	if !slices.Equal(src.abandoned, want) {
		t.Errorf("Abandon() calls = %v, want %v", src.abandoned, want)
	}
}

func repeatKey(k string) tea.KeyPressMsg {
	msg := keyMsg(k)
	msg.IsRepeat = true
	return msg
}

func noRevision() error {
	return fmt.Errorf("jj log: %w", jj.ErrNoRevision)
}

func TestLogAbandon_confirmation(t *testing.T) {
	t.Run("a asks for y before abandoning", func(t *testing.T) {
		src := revisionSource()

		m := press(t, loaded(t, src), "2", "j", "a")

		assertContains(t, view(m), "Abandon wxyzchil childcom", "child feature", "Deletes bookmark feature", "y abandon · any other key cancels")
		assertAbandoned(t, src)
	})

	t.Run("the prompt lists every bookmark that is deleted", func(t *testing.T) {
		src := revisionSource()
		src.log[1].Entry.Bookmarks = []string{"feature", "feat-2"}

		m := press(t, loaded(t, src), "2", "j", "a")

		assertContains(t, view(m), "Deletes bookmarks feature, feat-2")
	})

	t.Run("the prompt leaves out bookmarks when there are none", func(t *testing.T) {
		src := revisionSource()

		m := press(t, loaded(t, src), "2", "j", "j", "a")

		got := view(m)
		assertContains(t, got, "Abandon pqrspare parentco", "y abandon · any other key cancels")
		assertMissing(t, got, "Deletes")
	})

	t.Run("a narrow screen wraps the bookmarks instead of hiding them", func(t *testing.T) {
		src := revisionSource()
		src.log[1].Entry.Bookmarks = []string{"release-candidate-q4", strings.Repeat("ab", 30) + "END"}

		m := press(t, start(t, src, nil, 50, 30), "2", "j", "a")

		got := view(m)
		assertContains(t, got, "release-candidate-q4", "END", "y abandon")
		for line := range strings.Lines(got) {
			if n := utf8.RuneCountInString(strings.TrimSuffix(line, "\n")); n > 50 {
				t.Errorf("line width = %d, want at most 50: %q", n, line)
			}
		}
	})

	t.Run("a short screen keeps the footer", func(t *testing.T) {
		src := revisionSource()
		src.log[1].Entry.Bookmarks = []string{"one", "two", "three", "four", "five", "six", "seven", "eight", "nine", "ten"}

		m := press(t, start(t, src, nil, 30, 8), "2", "j", "a")

		assertContains(t, view(m), "y abandon")
	})

	t.Run("y abandons the revision under the cursor", func(t *testing.T) {
		src := revisionSource()

		m := press(t, loaded(t, src), "2", "j", "a", "y")

		assertAbandoned(t, src, "childcommit")
		if !slices.Equal(src.confirmed[0], []string{"feature"}) {
			t.Errorf("Abandon() bookmarks = %v, want [feature]", src.confirmed[0])
		}
		assertContains(t, view(m), "abandoned wxyzchil")
		assertLogTab(t, m)
	})

	t.Run("the prompt says the selection will be dropped", func(t *testing.T) {
		src := revisionSource()

		m := press(t, loaded(t, src), "tab", "j", "tab", "space", "2", "j", "a")

		assertContains(t, view(m), "Deletes bookmark feature", "Drops 1 selected hunk")
		assertAbandoned(t, src)
	})

	t.Run("help lists the abandon key", func(t *testing.T) {
		got := view(press(t, start(t, revisionSource(), nil, 120, 50), "2", "?"))

		assertContains(t, got, "abandon the revision (y confirms)")
	})
}

func TestLogAbandon_keyRepeat(t *testing.T) {
	t.Run("a held as plain presses does not abandon", func(t *testing.T) {
		src := revisionSource()

		m := press(t, loaded(t, src), "2", "j", "a", "a", "y")

		assertAbandoned(t, src)
		assertMissing(t, view(m), "y abandon ·")
	})

	t.Run("y without a prompt does nothing", func(t *testing.T) {
		src := revisionSource()

		press(t, loaded(t, src), "2", "j", "y", "y")

		assertAbandoned(t, src)
	})

	t.Run("a repeated a does not ask to abandon", func(t *testing.T) {
		src := revisionSource()
		m := press(t, loaded(t, src), "2", "j")

		m, _ = update(t, m, repeatKey("a"))

		assertMissing(t, view(m), "y abandon ·")
	})

	t.Run("a repeated y closes the box without confirming", func(t *testing.T) {
		src := revisionSource()
		m := press(t, loaded(t, src), "2", "j", "a")

		m, cmd := update(t, m, repeatKey("y"))
		m = run(t, m, cmd)

		assertAbandoned(t, src)
		assertMissing(t, view(m), "y abandon ·")
		press(t, m, "y")
		assertAbandoned(t, src)
	})

	t.Run("keys held through the abandon and the refresh abandon once", func(t *testing.T) {
		src := revisionSource()
		m := press(t, loaded(t, src), "2", "j", "a")
		m, abandon := update(t, m, keyMsg("y"))
		src.log = withoutEntry(logLines(), "childcommit")

		for _, k := range []string{"a", "y"} {
			var cmd tea.Cmd
			m, cmd = update(t, m, keyMsg(k))
			if cmd != nil {
				t.Fatalf("%s while the abandon is pending returned a command", k)
			}
		}
		m, refresh := update(t, m, abandon())
		for _, k := range []string{"a", "y"} {
			var cmd tea.Cmd
			m, cmd = update(t, m, keyMsg(k))
			if cmd != nil {
				t.Fatalf("%s while the log refreshes returned a command", k)
			}
		}
		m = run(t, m, refresh)
		m = press(t, m, "y")

		assertAbandoned(t, src, "childcommit")
		assertMissing(t, view(m), "y abandon ·")
	})
}

func TestLogAbandon_cancelled(t *testing.T) {
	for _, k := range []string{"j", "k", "l", "esc", "R", "n"} {
		t.Run(k+" cancels the confirmation", func(t *testing.T) {
			src := revisionSource()

			m := press(t, loaded(t, src), "2", "j", "a", k, "y")

			assertAbandoned(t, src)
			assertMissing(t, view(m), "y abandon ·")
		})
	}

	t.Run("a is ignored while the log refreshes", func(t *testing.T) {
		src := revisionSource()
		m := press(t, loaded(t, src), "2", "j")
		m, refresh := update(t, m, keyMsg("ctrl+r"))

		m = press(t, m, "a")

		assertMissing(t, view(m), "y abandon ·")
		m = press(t, run(t, m, refresh), "j", "a")
		assertAbandoned(t, src)
		assertContains(t, view(m), "Abandon wxyzchil childcom")
	})
}

func TestLogAbandon_refused(t *testing.T) {
	t.Run("an immutable revision is not abandoned", func(t *testing.T) {
		src := revisionSource()

		m := press(t, loaded(t, src), "2", "G", "a")

		assertContains(t, view(m), "zzzzroot is immutable")
		press(t, m, "y")
		assertAbandoned(t, src)
	})

	t.Run("a is blocked after an incomplete split", func(t *testing.T) {
		src := revisionSource()
		src.splitErr = fmt.Errorf("split: %w: reload failed", jj.ErrSplitIncomplete)
		m := press(t, loaded(t, src), "tab", "j", "tab", "space", "c", "ctrl+s", "2", "j")

		m = press(t, m, "a")

		assertContains(t, view(m), "split done; restart jjr to continue")
		press(t, m, "y")
		assertAbandoned(t, src)
	})
}

func TestLogAbandon_bookmarksChanged(t *testing.T) {
	t.Run("asks for a fresh a", func(t *testing.T) {
		src := revisionSource()
		src.commitBookmarks = map[string][]string{"childcommit": {"feature", "other"}}
		m := press(t, loaded(t, src), "2", "j", "a")
		src.log[1].Entry.Bookmarks = []string{"feature", "other"}

		m = press(t, m, "y")

		got := view(m)
		assertContains(t, got, "bookmarks changed; press a to review the abandon again")
		assertMissing(t, got, "y abandon ·")
		m = press(t, m, "a")
		assertContains(t, view(m), "Deletes bookmarks feature, other")
		press(t, m, "y")
		assertAbandoned(t, src, "childcommit", "childcommit")
		if got := src.confirmed[1]; !slices.Equal(got, []string{"feature", "other"}) {
			t.Errorf("Abandon() bookmarks = %v, want [feature other]", got)
		}
	})

	t.Run("a held y does not abandon", func(t *testing.T) {
		src := revisionSource()
		src.commitBookmarks = map[string][]string{"childcommit": {"feature", "other"}}
		m := press(t, loaded(t, src), "2", "j", "a")
		src.log[1].Entry.Bookmarks = []string{"feature", "other"}

		m = press(t, m, "y", "y", "y")
		m, cmd := update(t, m, repeatKey("y"))
		run(t, m, cmd)

		assertAbandoned(t, src, "childcommit")
	})
}

func TestLogAbandon_viewChanges(t *testing.T) {
	for name, keys := range map[string][]string{
		"a tab switch": {"1", "2"},
		"the help":     {"?", "esc"},
		"a search":     {"/", "esc"},
	} {
		t.Run(name+" cancels the confirmation", func(t *testing.T) {
			src := revisionSource()
			m := press(t, loaded(t, src), "2", "j", "a")

			m = press(t, m, append(keys, "y")...)

			assertAbandoned(t, src)
			assertMissing(t, view(m), "y abandon ·")
		})
	}
}

func TestLogAbandon_success(t *testing.T) {
	t.Run("refreshes the log and keeps the cursor position", func(t *testing.T) {
		src := revisionSource()
		m := press(t, loaded(t, src), "2", "j", "a")
		src.log = withoutEntry(logLines(), "childcommit")

		m = press(t, m, "y")

		got := view(m)
		assertMissing(t, got, "child feature")
		assertContains(t, got, "rev 2/3")
		if cursor := m.LogCursor(); cursor != "pqrsparent00" {
			t.Errorf("LogCursor() = %q, want %q", cursor, "pqrsparent00")
		}
	})

	t.Run("clamps the cursor to the last revision", func(t *testing.T) {
		src := revisionSource()
		src.log = logLines()[:4]
		m := press(t, loaded(t, src), "2", "G", "a")
		src.log = withoutEntry(logLines()[:4], "parentcommit")

		m = press(t, m, "y")

		if cursor := m.LogCursor(); cursor != "wxyzchild000" {
			t.Errorf("LogCursor() = %q, want %q", cursor, "wxyzchild000")
		}
	})

	t.Run("a cursor moved during the refresh stays on its revision", func(t *testing.T) {
		src := revisionSource()
		m := press(t, loaded(t, src), "2", "j", "a")
		m, abandon := update(t, m, keyMsg("y"))
		m, refresh := update(t, m, abandon())
		m = press(t, m, "G")
		src.log = withoutEntry(logLines(), "childcommit")

		m = run(t, m, refresh)

		if cursor := m.LogCursor(); cursor != "zzzzroot0000" {
			t.Errorf("LogCursor() = %q, want %q", cursor, "zzzzroot0000")
		}
	})

	t.Run("abandoning the reviewed revision reviews the working copy", func(t *testing.T) {
		src := revisionSource()
		m := press(t, loaded(t, src), "2", "j", "enter", "space", "2", "a")
		src.log = withoutEntry(logLines(), "childcommit")

		m = press(t, m, "y")

		assertAbandoned(t, src, "childcommit")
		if got := src.changeRevs[len(src.changeRevs)-1]; got != "@" {
			t.Errorf("Change() last rev = %q, want %q", got, "@")
		}
		assertLogTab(t, m)
		m = press(t, m, "1")
		assertContains(t, view(m), "@ kxyzabcd", "nothing selected")
	})

	t.Run("abandoning the working copy reviews the new working copy", func(t *testing.T) {
		src := revisionSource()
		m := press(t, loaded(t, src), reviewParent()...)
		m = press(t, m, "2", "g", "a")
		src.change = jj.Change{ChangeID: "nnnnfresh000", CommitID: "freshcommit", Parents: []string{"parentcommit"}, WorkingCopy: true}
		src.revFiles["freshcommit"] = []diff.File{}
		src.log = logLines()
		src.log[0].Entry = &jj.LogEntry{ChangeID: "nnnnfresh000", CommitID: "freshcommit", WorkingCopy: true}

		m = press(t, m, "y")

		assertAbandoned(t, src, "commit1")
		if got := src.changeRevs[len(src.changeRevs)-1]; got != "@" {
			t.Errorf("Change() last rev = %q, want %q", got, "@")
		}
		assertContains(t, lineWith(t, view(m), "nnnnfres (no description"), "▌")
		assertContains(t, view(press(t, m, "1")), "@ nnnnfres")
	})

	t.Run("a stale working-copy marker does not move the review", func(t *testing.T) {
		src := revisionSource()
		src.wcCommit = "elsewhere"
		m := press(t, loaded(t, src), reviewParent()...)
		m = press(t, m, "2", "g", "a")

		m = press(t, m, "y")

		assertAbandoned(t, src, "commit1")
		if got, want := src.changeRevs[len(src.changeRevs)-1], jj.VisibleCommitRevset("parentcommit"); got != want {
			t.Errorf("Change() last rev = %q, want %q", got, want)
		}
		assertContains(t, view(press(t, m, "1")), "@ pqrspare")
	})

	t.Run("a revision that became the working copy moves the review to it", func(t *testing.T) {
		src := revisionSource()
		src.wcCommit = "childcommit"
		m := press(t, loaded(t, src), reviewParent()...)
		m = press(t, m, "2", "g", "j", "a")

		press(t, m, "y")

		assertAbandoned(t, src, "childcommit")
		if got := src.changeRevs[len(src.changeRevs)-1]; got != "@" {
			t.Errorf("Change() last rev = %q, want %q", got, "@")
		}
	})

	t.Run("an unrelated abandon keeps the exact reviewed commit of a divergent change", func(t *testing.T) {
		src := divergentSource()
		src.succErrs = map[string]error{"divergenttwo": fmt.Errorf("jj log: %w", jj.ErrMultipleRevisions)}
		m := press(t, loaded(t, src), "2", "j", "j", "enter", "2", "g", "j", "a")

		m = press(t, m, "y")

		assertAbandoned(t, src, "divergentone")
		if got, want := src.changeRevs[len(src.changeRevs)-1], jj.VisibleCommitRevset("divergenttwo"); got != want {
			t.Errorf("Change() last rev = %q, want %q", got, want)
		}
		assertMissing(t, view(m), "multiple")
		if got := m.CurrentFile(); got != "src/main.go" {
			t.Errorf("CurrentFile() = %q, want %q", got, "src/main.go")
		}
	})

	t.Run("abandoning an ancestor reloads the rebased reviewed revision", func(t *testing.T) {
		src := revisionSource()
		m := press(t, loaded(t, src), "2", "j", "enter", "2", "j", "a")
		rebased := jj.Change{ChangeID: "wxyzchild000", CommitID: "childcommit2", Parents: []string{"rootcommit"}, Description: "child feature"}
		src.revErrs = map[string]error{jj.VisibleCommitRevset("childcommit"): noRevision()}
		src.successors = map[string]jj.Change{"childcommit": rebased}
		src.revisions[jj.VisibleCommitRevset("childcommit2")] = rebased
		src.revFiles["childcommit2"] = []diff.File{mainFile()}
		src.log = withoutEntry(logLines(), "parentcommit")
		src.log[1].Entry.CommitID = "childcommit2"

		m = press(t, m, "y")

		assertAbandoned(t, src, "parentcommit")
		if got := src.diffRevs[len(src.diffRevs)-1]; got != "childcommit2" {
			t.Errorf("Diff() last rev = %q, want %q", got, "childcommit2")
		}
		assertContains(t, lineWith(t, view(m), "○  wxyzchil"), "▌")
		assertLogTab(t, m)
		m = press(t, m, "1")
		assertContains(t, view(m), "@ wxyzchil")
		if got := m.CurrentFile(); got != "src/main.go" {
			t.Errorf("CurrentFile() = %q, want %q", got, "src/main.go")
		}
	})

	t.Run("a revision without a successor falls back to the working copy", func(t *testing.T) {
		src := revisionSource()
		m := press(t, loaded(t, src), "2", "j", "enter", "2", "j", "a")
		src.revErrs = map[string]error{jj.VisibleCommitRevset("childcommit"): noRevision()}

		m = press(t, m, "y")

		if got := src.changeRevs[len(src.changeRevs)-1]; got != "@" {
			t.Errorf("Change() last rev = %q, want %q", got, "@")
		}
		assertContains(t, view(m), "abandoned pqrspare; wxyzchil was rewritten, reviewing the working copy")
		assertContains(t, view(press(t, m, "1")), "@ kxyzabcd")
	})

	t.Run("an ambiguous rewrite falls back to the working copy", func(t *testing.T) {
		src := revisionSource()
		m := press(t, loaded(t, src), "2", "j", "enter", "2", "j", "a")
		src.revErrs = map[string]error{jj.VisibleCommitRevset("childcommit"): noRevision()}
		src.succErrs = map[string]error{"childcommit": fmt.Errorf("jj log: %w", jj.ErrMultipleRevisions)}

		m = press(t, m, "y")

		if got := src.changeRevs[len(src.changeRevs)-1]; got != "@" {
			t.Errorf("Change() last rev = %q, want %q", got, "@")
		}
		assertContains(t, view(m), "abandoned pqrspare; wxyzchil was rewritten, reviewing the working copy")
	})

	t.Run("a commit after abandoning an ancestor splits the rebased revision", func(t *testing.T) {
		src := revisionSource()
		m := press(t, loaded(t, src), "2", "j", "j", "a")
		rebased := jj.Change{ChangeID: "kxyzabcdefgh", CommitID: "commit2", Parents: []string{"parent1"}, WorkingCopy: true}
		src.revErrs = map[string]error{jj.VisibleCommitRevset("commit1"): noRevision()}
		src.successors = map[string]jj.Change{"commit1": rebased}
		src.revisions[jj.VisibleCommitRevset("commit2")] = rebased
		src.revFiles["commit2"] = []diff.File{readmeFile(), mainFile()}
		src.head = "commit2"
		m = press(t, m, "y", "1", "tab", "j", "tab", "space", "c", "ctrl+s")

		if len(src.splits) != 1 {
			t.Fatalf("Split() calls = %d, want 1", len(src.splits))
		}
		assertMissing(t, view(m), "revision changed")
	})
}

func TestLogAbandon_failed(t *testing.T) {
	t.Run("shows the jj error and reloads both views", func(t *testing.T) {
		src := revisionSource()
		src.abandonErr = errors.New("jj abandon: exit status 1: Error: Commit childcom is immutable\nHint: Could not modify commit")
		m := press(t, loaded(t, src), "2", "j")
		calls := src.logCalls

		m = press(t, m, "a", "y")

		got := view(m)
		assertContains(t, got, "abandon wxyzchil: jj abandon: exit status 1: Error: Commit childcom is immutable", "child feature")
		assertMissing(t, got, "Hint")
		assertLogTab(t, m)
		if src.logCalls != calls+1 {
			t.Errorf("Log() calls = %d, want %d", src.logCalls, calls+1)
		}
		if got, want := src.changeRevs[len(src.changeRevs)-1], jj.VisibleCommitRevset("commit1"); got != want {
			t.Errorf("Change() last rev = %q, want %q", got, want)
		}
	})

	t.Run("an ignored a keeps the error", func(t *testing.T) {
		src := revisionSource()
		src.abandonErr = errors.New("jj exploded")
		m := press(t, loaded(t, src), "2", "j", "a")
		m, abandon := update(t, m, keyMsg("y"))
		m, refresh := update(t, m, abandon())

		m, _ = update(t, m, keyMsg("a"))

		assertContains(t, view(m), "abandon wxyzchil: jj exploded")
		m = run(t, m, refresh)
		assertContains(t, view(m), "abandon wxyzchil: jj exploded")
	})

	t.Run("a failed reload marks the review out of date", func(t *testing.T) {
		src := revisionSource()
		m := press(t, loaded(t, src), "tab", "j", "tab", "space", "2", "j", "j", "a")
		src.revErrs = map[string]error{jj.VisibleCommitRevset("commit1"): errors.New("jj exploded")}

		m = press(t, m, "y")

		assertContains(t, view(m), "abandoned pqrspare; ", "jj exploded")
		m = press(t, m, "1", "2", "1")
		assertContains(t, view(m), "review out of date; press @ to review the working copy", "1 hunk selected")
		m = press(t, m, "c")
		if m.ModalOpen() {
			t.Error("ModalOpen() = true, want the commit blocked")
		}
		m = press(t, m, "tab", "j", "tab", "space")
		assertContains(t, view(m), "review out of date; press @ to review the working copy", "1 hunk selected")
		src.revErrs = nil
		m = press(t, m, "@", "@")
		assertMissing(t, view(m), "review out of date")
	})

	t.Run("a failed fallback to the working copy marks the review out of date", func(t *testing.T) {
		src := revisionSource()
		m := press(t, loaded(t, src), "2", "j", "enter", "2", "a")
		src.revErrs = map[string]error{"@": errors.New("jj exploded")}

		m = press(t, m, "y", "1")

		assertContains(t, view(m), "review out of date; press @ to review the working copy")
	})
}

func TestLogAbandon_openWhileOutdated(t *testing.T) {
	outdated := func(t *testing.T, src *fakeSource) tui.Model {
		t.Helper()
		m := press(t, loaded(t, src), "2", "j", "j", "a")
		src.revErrs = map[string]error{jj.VisibleCommitRevset("commit1"): errors.New("jj exploded")}
		m = press(t, m, "y")
		src.revErrs = nil
		return m
	}

	t.Run("enter on the reviewed revision reloads it", func(t *testing.T) {
		src := revisionSource()
		m := outdated(t, src)

		m = press(t, m, "g", "enter")

		if got, want := src.changeRevs[len(src.changeRevs)-1], jj.VisibleCommitRevset("commit1"); got != want {
			t.Errorf("Change() last rev = %q, want %q", got, want)
		}
		assertMissing(t, view(m), "review out of date")
	})

	t.Run("enter on a hidden commit keeps the review out of date", func(t *testing.T) {
		src := revisionSource()
		m := outdated(t, src)
		src.revErrs = map[string]error{jj.VisibleCommitRevset("childcommit"): noRevision()}

		m = press(t, m, "g", "j", "enter")

		if got, want := src.changeRevs[len(src.changeRevs)-1], jj.VisibleCommitRevset("childcommit"); got != want {
			t.Errorf("Change() last rev = %q, want %q", got, want)
		}
		assertContains(t, view(m), "no selected revision")
		assertLogTab(t, m)
		assertContains(t, view(press(t, m, "1")), "review out of date")
	})
}

func TestLogAbandon_refreshesBookmarks(t *testing.T) {
	src := revisionSource()
	m := press(t, loaded(t, src), "3", "2", "j", "a", "y")
	calls := src.bmCalls

	press(t, m, "3")

	if src.bmCalls != calls+1 {
		t.Errorf("Bookmarks() calls = %d, want %d", src.bmCalls, calls+1)
	}
}

func TestOpenReviewed_workingCopyMoved(t *testing.T) {
	for name, keys := range map[string][]string{
		"from the log":       {"2", "R", "enter"},
		"from the bookmarks": {"3", "G", "enter"},
	} {
		t.Run(name, func(t *testing.T) {
			src := bookmarkSource()
			m := press(t, loaded(t, src), "tab", "j", "tab", "space")
			src.revErrs = map[string]error{jj.WorkingCopyRevset("kxyzabcdefgh"): noRevision()}

			m = press(t, m, keys...)

			assertReviewTab(t, m)
			assertContains(t, view(m), "working copy moved; press @ to review it", "1 hunk selected")
			m = press(t, m, "R")
			assertContains(t, view(m), "not the working copy; press @ to reload")
			m = press(t, m, "e")
			assertContains(t, view(m), "not the working copy; press @ to edit files")
		})
	}

	t.Run("a working copy that did not move keeps its flag", func(t *testing.T) {
		src := bookmarkSource()
		src.revisions[jj.WorkingCopyRevset("kxyzabcdefgh")] = src.change
		m := press(t, loaded(t, src), "tab", "j", "tab", "space")

		m = press(t, m, "2", "enter")

		assertMissing(t, view(m), "working copy moved")
		assertContains(t, view(m), "1 hunk selected")
	})
}
