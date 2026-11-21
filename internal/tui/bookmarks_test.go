package tui_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/madalinpopa/jjreview/internal/jj"
	"github.com/madalinpopa/jjreview/internal/tui"
)

func bookmarkSource() *fakeSource {
	src := revisionSource()
	src.bookmarks = []jj.Bookmark{
		{Name: "feature", ChangeID: "wxyzchild000", CommitID: "childcommit", Description: "child feature\n\nbody\n", Remotes: []jj.RemoteBookmark{{Remote: "origin", Ahead: 2, Behind: 1}}},
		{Name: "main", ChangeID: "pqrsparent00", CommitID: "parentcommit", Description: "parent\n", Remotes: []jj.RemoteBookmark{{Remote: "origin", Synced: true}}},
		{Name: "split", Conflict: true, Added: []string{"childcommit", "parentcommit"}, Removed: []string{"rootcommit"}},
		{Name: "todo", ChangeID: "kxyzabcdefgh", CommitID: "commit1"},
	}
	src.revisions["root()"] = jj.Change{ChangeID: "zzzzroot0000", CommitID: "rootcommit"}
	src.revisions["main"] = jj.Change{ChangeID: "pqrsparent00", CommitID: "parentcommit"}
	return src
}

func assertBookmarksTab(t *testing.T, m tui.Model) {
	t.Helper()
	got := view(m)
	assertContains(t, got, "BOOKMARKS")
	assertMissing(t, got, "FILES", "UNIFIED DIFF", "│ LOG")
}

func typeText(t *testing.T, m tui.Model, text string) tui.Model {
	t.Helper()
	for _, r := range text {
		m = press(t, m, string(r))
	}
	return m
}

func names(src *fakeSource) []string {
	var out []string
	for _, b := range src.bookmarks {
		out = append(out, b.Name+"="+b.CommitID)
	}
	return out
}

func assertBookmarks(t *testing.T, src *fakeSource, want ...string) {
	t.Helper()
	got := names(src)
	if len(got) != len(want) {
		t.Fatalf("bookmarks = %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("bookmarks = %v, want %v", got, want)
			return
		}
	}
}

func TestBookmarksTab_success(t *testing.T) {
	t.Run("3 shows the bookmarks and loads them once", func(t *testing.T) {
		src := bookmarkSource()

		m := press(t, loaded(t, src), "3", "1", "3")

		assertBookmarksTab(t, m)
		if src.bmCalls != 1 {
			t.Errorf("Bookmarks() calls = %d, want 1", src.bmCalls)
		}
	})

	t.Run("the bookmarks are not loaded before they are shown", func(t *testing.T) {
		src := bookmarkSource()

		loaded(t, src)

		if src.bmCalls != 0 {
			t.Errorf("Bookmarks() calls = %d, want 0", src.bmCalls)
		}
	})

	t.Run("1 and 2 leave the bookmarks", func(t *testing.T) {
		m := press(t, loaded(t, bookmarkSource()), "3", "1")
		assertReviewTab(t, m)

		m = press(t, m, "3", "2")
		assertLogTab(t, m)

		m = press(t, m, "3")
		assertBookmarksTab(t, m)
	})

	t.Run("the top bar shows the bookmarks tab", func(t *testing.T) {
		got := view(loaded(t, bookmarkSource()))

		assertContains(t, lineWith(t, got, "@ kxyzabcd"), "1 Review", "2 Log", "3 Bookmarks")
	})

	t.Run("R refreshes the bookmarks", func(t *testing.T) {
		src := bookmarkSource()
		m := press(t, loaded(t, src), "3")
		src.bookmarks = append(src.bookmarks, jj.Bookmark{Name: "zeta", ChangeID: "zetazetazeta", CommitID: "zetacommit", Description: "brand new"})

		m = press(t, m, "R")

		assertContains(t, view(m), "zeta", "brand new")
		if src.bmCalls != 2 {
			t.Errorf("Bookmarks() calls = %d, want 2", src.bmCalls)
		}
	})

	t.Run("a commit marks the bookmarks stale", func(t *testing.T) {
		src := bookmarkSource()
		m := press(t, loaded(t, src), "3", "1", "tab", "j", "tab", "space", "c", "ctrl+s")

		press(t, m, "3")

		if src.bmCalls != 2 {
			t.Errorf("Bookmarks() calls = %d, want 2", src.bmCalls)
		}
	})

	t.Run("R in the review marks the bookmarks stale", func(t *testing.T) {
		src := bookmarkSource()

		press(t, loaded(t, src), "3", "1", "R", "3")

		if src.bmCalls != 2 {
			t.Errorf("Bookmarks() calls = %d, want 2", src.bmCalls)
		}
	})

	t.Run("@ marks the bookmarks stale", func(t *testing.T) {
		src := bookmarkSource()

		press(t, loaded(t, src), "3", "1", "@", "3")

		if src.bmCalls != 2 {
			t.Errorf("Bookmarks() calls = %d, want 2", src.bmCalls)
		}
	})

	t.Run("ctrl+r in the log marks the bookmarks stale", func(t *testing.T) {
		src := bookmarkSource()

		press(t, loaded(t, src), "3", "2", "ctrl+r", "3")

		if src.bmCalls != 2 {
			t.Errorf("Bookmarks() calls = %d, want 2", src.bmCalls)
		}
	})

	t.Run("R in the bookmarks marks the log stale", func(t *testing.T) {
		src := bookmarkSource()

		press(t, loaded(t, src), "2", "3", "R", "2")

		if src.logCalls != 2 {
			t.Errorf("Log() calls = %d, want 2", src.logCalls)
		}
	})

	t.Run("R in the bookmarks keeps the review selection", func(t *testing.T) {
		m := press(t, loaded(t, bookmarkSource()), "tab", "j", "tab", "space", "3", "R", "1")

		assertContains(t, view(m), "1 hunk selected")
	})

	t.Run("an empty repository says so", func(t *testing.T) {
		src := revisionSource()

		m := press(t, loaded(t, src), "3")

		assertContains(t, view(m), "no bookmarks")
	})
}

func TestBookmarksTab_failed(t *testing.T) {
	src := bookmarkSource()
	src.bmLoadErr = errors.New("jj exploded")

	m := press(t, loaded(t, src), "3")

	assertBookmarksTab(t, m)
	assertContains(t, view(m), "jj exploded")
}

func TestBookmarksView_success(t *testing.T) {
	got := view(press(t, loaded(t, bookmarkSource()), "3"))

	tests := map[string]struct {
		row  string
		want []string
	}{
		"ahead and behind its remote": {row: "feature", want: []string{"wxyzchil", "childcom", "child feature", "origin ↑2 ↓1"}},
		"synced with its remote":      {row: "main", want: []string{"pqrspare", "parentco", "parent", "origin ✓"}},
		"conflicted":                  {row: "split", want: []string{"conflicted"}},
		"without a description":       {row: "todo", want: []string{"kxyzabcd", "commit1", "(no description set)"}},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			assertContains(t, lineWith(t, got, " "+tt.row+" "), tt.want...)
		})
	}

	t.Run("the body of the description is not shown", func(t *testing.T) {
		assertMissing(t, got, "body")
	})

	t.Run("an untracked bookmark shows no remote", func(t *testing.T) {
		assertMissing(t, lineWith(t, got, " todo "), "origin")
	})

	t.Run("the change IDs line up", func(t *testing.T) {
		feature := strings.Index(lineWith(t, got, " feature "), "wxyzchil")
		todo := strings.Index(lineWith(t, got, " todo "), "kxyzabcd")

		if feature != todo {
			t.Errorf("change ID columns = %d and %d, want them equal", feature, todo)
		}
	})

	t.Run("a refresh shows the new target", func(t *testing.T) {
		src := bookmarkSource()
		m := press(t, loaded(t, src), "3")
		src.bookmarks[0] = jj.Bookmark{Name: "feature", ChangeID: "pqrsparent00", CommitID: "parentcommit", Description: "parent\n"}

		line := lineWith(t, view(press(t, m, "R")), " feature ")

		assertContains(t, line, "pqrspare", "parentco", "parent")
		assertMissing(t, line, "childcom", "origin")
	})

	t.Run("the header counts the bookmarks", func(t *testing.T) {
		assertContains(t, got, "bookmark 1/4")
	})
}

func TestBookmarksView_remoteMarkers(t *testing.T) {
	t.Run("an inexact count is a lower bound", func(t *testing.T) {
		src := bookmarkSource()
		src.bookmarks[0].Remotes = []jj.RemoteBookmark{{Remote: "origin", Ahead: 10, AheadAtLeast: true, Behind: 3, BehindAtLeast: true}}

		got := view(press(t, loaded(t, src), "3"))

		assertContains(t, lineWith(t, got, " feature "), "origin ↑10+ ↓3+")
	})

	t.Run("long markers leave the name visible", func(t *testing.T) {
		src := bookmarkSource()
		for i := range 6 {
			src.bookmarks[0].Remotes = append(src.bookmarks[0].Remotes, jj.RemoteBookmark{Remote: fmt.Sprintf("mirror-number-%d", i), Ahead: 1})
		}

		got := view(press(t, start(t, src, nil, 60, 20), "3"))

		assertContains(t, got, " feature ", "origin")
	})
}

func TestBookmarksCursor_success(t *testing.T) {
	tests := map[string]struct {
		keys []string
		want string
	}{
		"starts on the first bookmark":   {keys: []string{"3"}, want: "feature"},
		"j moves down":                   {keys: []string{"3", "j"}, want: "main"},
		"k moves up":                     {keys: []string{"3", "j", "j", "k"}, want: "main"},
		"k stops at the first bookmark":  {keys: []string{"3", "k"}, want: "feature"},
		"j stops at the last bookmark":   {keys: []string{"3", "j", "j", "j", "j", "j"}, want: "todo"},
		"G moves to the last bookmark":   {keys: []string{"3", "G"}, want: "todo"},
		"g moves to the first bookmark":  {keys: []string{"3", "G", "g"}, want: "feature"},
		"ctrl+d moves half a page down":  {keys: []string{"3", "ctrl+d"}, want: "todo"},
		"ctrl+u moves half a page up":    {keys: []string{"3", "G", "ctrl+u"}, want: "feature"},
		"refreshing keeps the bookmark":  {keys: []string{"3", "j", "R"}, want: "main"},
		"the cursor survives a tab trip": {keys: []string{"3", "j", "1", "3"}, want: "main"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			m := press(t, loaded(t, bookmarkSource()), tt.keys...)

			if got := m.BookmarkCursor(); got != tt.want {
				t.Errorf("BookmarkCursor() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBookmarksHelp_success(t *testing.T) {
	m := press(t, loaded(t, bookmarkSource()), "3", "?")

	assertContains(t, view(m), "BOOKMARKS", "new bookmark", "edit the bookmark", "delete the bookmark", "bookmarks tab")
}

func TestBookmarksOpen_success(t *testing.T) {
	t.Run("enter reviews the target of the bookmark", func(t *testing.T) {
		src := bookmarkSource()

		m := press(t, loaded(t, src), "3", "enter")

		if got, want := src.changeRevs[len(src.changeRevs)-1], jj.VisibleCommitRevset("childcommit"); got != want {
			t.Errorf("Change() last rev = %q, want %q", got, want)
		}
		assertReviewTab(t, m)
		assertContains(t, view(m), "@ wxyzchil")
	})

	t.Run("enter on the reviewed revision goes back without reloading", func(t *testing.T) {
		src := bookmarkSource()
		m := loaded(t, src)
		calls := src.diffCalls

		m = press(t, m, "3", "G", "enter")

		assertReviewTab(t, m)
		if got := src.diffCalls; got != calls {
			t.Errorf("Diff() calls = %d, want %d", got, calls)
		}
	})

	t.Run("enter asks again before dropping selected hunks", func(t *testing.T) {
		src := bookmarkSource()
		m := press(t, loaded(t, src), "tab", "j", "tab", "space", "3")
		calls := len(src.changeRevs)

		m = press(t, m, "enter")

		assertBookmarksTab(t, m)
		assertContains(t, view(m), "press enter again to drop 1 selected hunk")
		if got := len(src.changeRevs); got != calls {
			t.Errorf("Change() calls = %d, want %d", got, calls)
		}

		m = press(t, m, "enter")

		assertReviewTab(t, m)
		assertContains(t, view(m), "@ wxyzchil", "nothing selected")
	})
}

func TestBookmarksOpen_confirmationFollowsTheTarget(t *testing.T) {
	src := bookmarkSource()
	m := press(t, loaded(t, src), "tab", "j", "tab", "space", "3")
	m, refresh := update(t, m, keyMsg("R"))
	m = press(t, m, "enter")
	assertContains(t, view(m), "press enter again")
	src.bookmarks[0].CommitID = "parentcommit"
	m = run(t, m, refresh)
	assertMissing(t, view(m), "press enter again")
	calls := len(src.changeRevs)

	m = press(t, m, "enter")

	if got := len(src.changeRevs); got != calls {
		t.Errorf("Change() calls = %d, want %d", got, calls)
	}
	assertBookmarksTab(t, m)
	assertContains(t, view(m), "press enter again to drop 1 selected hunk")
}

func TestBookmarksOpen_failed(t *testing.T) {
	src := bookmarkSource()
	m := press(t, loaded(t, src), "3", "j", "j")
	calls := len(src.changeRevs)

	m = press(t, m, "enter")

	assertBookmarksTab(t, m)
	assertContains(t, view(m), "split is conflicted")
	if got := len(src.changeRevs); got != calls {
		t.Errorf("Change() calls = %d, want %d", got, calls)
	}
}

func TestBookmarkCreate_success(t *testing.T) {
	t.Run("n creates a bookmark on the commit under review", func(t *testing.T) {
		src := bookmarkSource()
		m := press(t, loaded(t, src), "3", "n")
		assertContains(t, view(m), "New bookmark", "Target revision · wip", "kxyzabcd")
		assertMissing(t, view(m), "commit_id(")

		m = press(t, typeText(t, m, "topic"), "enter")

		assertBookmarks(t, src, "feature=childcommit", "main=parentcommit", "split=", "todo=commit1", "topic=commit1")
		assertMissing(t, view(m), "New bookmark")
		assertContains(t, lineWith(t, view(m), " topic "), "commit1")
		if got := m.BookmarkCursor(); got != "topic" {
			t.Errorf("BookmarkCursor() = %q, want %q", got, "topic")
		}
	})

	t.Run("the target is resolved to a commit", func(t *testing.T) {
		src := bookmarkSource()
		m := press(t, loaded(t, src), "3", "n")
		m = press(t, typeText(t, m, "topic"), "tab", "ctrl+u")

		m = press(t, typeText(t, m, "main"), "ctrl+s")

		assertBookmarks(t, src, "feature=childcommit", "main=parentcommit", "split=", "todo=commit1", "topic=parentcommit")
		if got := m.BookmarkCursor(); got != "topic" {
			t.Errorf("BookmarkCursor() = %q, want %q", got, "topic")
		}
	})

	t.Run("the name keeps its spaces", func(t *testing.T) {
		src := bookmarkSource()
		m := press(t, loaded(t, src), "3", "n")

		press(t, typeText(t, m, " topic "), "enter")

		assertBookmarks(t, src, " topic =commit1", "feature=childcommit", "main=parentcommit", "split=", "todo=commit1")
	})

	t.Run("a new bookmark refreshes the log", func(t *testing.T) {
		src := bookmarkSource()
		m := press(t, loaded(t, src), "2", "3", "n")

		press(t, typeText(t, m, "topic"), "enter", "2")

		if src.logCalls != 2 {
			t.Errorf("Log() calls = %d, want 2", src.logCalls)
		}
	})

	t.Run("jj warnings are shown before the dialog closes", func(t *testing.T) {
		src := bookmarkSource()
		src.bmWarnings = "Warning: Target revision is empty."
		m := press(t, loaded(t, src), "3", "n")

		m = press(t, typeText(t, m, "topic"), "enter")

		assertContains(t, view(m), "New bookmark", "Target revision is empty.")
		assertContains(t, lineWith(t, view(m), " topic "), "commit1")

		m = press(t, m, "enter")

		assertMissing(t, view(m), "New bookmark")
	})

	t.Run("esc cancels", func(t *testing.T) {
		src := bookmarkSource()
		m := press(t, loaded(t, src), "3", "n")

		m = press(t, typeText(t, m, "topic"), "esc")

		assertMissing(t, view(m), "New bookmark")
		assertBookmarks(t, src, "feature=childcommit", "main=parentcommit", "split=", "todo=commit1")
	})

	t.Run("ctrl+v does not read the clipboard", func(t *testing.T) {
		m := press(t, loaded(t, bookmarkSource()), "3", "n")

		_, cmd := update(t, m, tea.KeyPressMsg{Code: 'v', Mod: tea.ModCtrl})

		if cmd != nil {
			t.Error("Update(ctrl+v) cmd != nil, want no clipboard read")
		}
	})

	t.Run("a bracketed paste fills the input", func(t *testing.T) {
		src := bookmarkSource()
		m := press(t, loaded(t, src), "3", "n")
		m, _ = update(t, m, tea.PasteMsg{Content: "pasted"})

		press(t, m, "enter")

		assertBookmarks(t, src, "feature=childcommit", "main=parentcommit", "pasted=commit1", "split=", "todo=commit1")
	})
}

func TestBookmarkCreate_failed(t *testing.T) {
	t.Run("jj errors stay in the dialog with the input", func(t *testing.T) {
		src := bookmarkSource()
		m := press(t, loaded(t, src), "3", "n")

		m = press(t, typeText(t, m, "main"), "enter")

		assertContains(t, view(m), "New bookmark", "already exists", "main")
		press(t, typeText(t, m, "2"), "enter")
		assertBookmarks(t, src, "feature=childcommit", "main=parentcommit", "main2=commit1", "split=", "todo=commit1")
	})

	t.Run("an unknown target stays in the dialog", func(t *testing.T) {
		src := bookmarkSource()
		src.revErrs = map[string]error{"nope": errors.New("Revision `nope` doesn't exist")}
		m := press(t, loaded(t, src), "3", "n")
		m = press(t, typeText(t, m, "topic"), "tab", "ctrl+u")

		m = press(t, typeText(t, m, "nope"), "enter")

		assertContains(t, view(m), "New bookmark", "doesn't exist")
		assertBookmarks(t, src, "feature=childcommit", "main=parentcommit", "split=", "todo=commit1")
	})

	tests := map[string]string{
		"an empty name":          "",
		"a whitespace-only name": "   ",
	}
	for name, input := range tests {
		t.Run(name+" is rejected", func(t *testing.T) {
			src := bookmarkSource()
			m := press(t, loaded(t, src), "3", "n")

			m = press(t, typeText(t, m, input), "enter")

			assertContains(t, view(m), "New bookmark", "bookmark name is empty")
			assertBookmarks(t, src, "feature=childcommit", "main=parentcommit", "split=", "todo=commit1")
		})
	}
}

func TestBookmarkTarget_success(t *testing.T) {
	t.Run("after a split the target is the remaining revision", func(t *testing.T) {
		src := bookmarkSource()
		m := press(t, loaded(t, src), "tab", "j", "tab", "space", "c", "ctrl+s")

		m = press(t, m, "3", "n")

		got := boxText(t, view(m), "New bookmark")
		assertContains(t, got, "rest1zzz")
		assertMissing(t, got, "kxyzabcd", "· wip")
	})

	t.Run("an edited target hides the description of the prefilled revision", func(t *testing.T) {
		m := press(t, loaded(t, bookmarkSource()), "3", "n", "tab", "backspace")

		assertMissing(t, view(m), "· wip")
	})
}

func TestBookmarkEdit_success(t *testing.T) {
	t.Run("e is prefilled with the bookmark and its commit", func(t *testing.T) {
		m := press(t, loaded(t, bookmarkSource()), "3", "e")

		got := view(m)
		assertContains(t, got, "Edit bookmark", "Target revision · child feature", "wxyzchil")
		assertMissing(t, got, "commit_id(")
	})

	t.Run("a new name renames the bookmark", func(t *testing.T) {
		src := bookmarkSource()
		m := press(t, loaded(t, src), "3", "e", "ctrl+u")

		m = press(t, typeText(t, m, "renamed"), "enter")

		assertBookmarks(t, src, "main=parentcommit", "renamed=childcommit", "split=", "todo=commit1")
		assertContains(t, lineWith(t, view(m), " renamed "), "childcom")
		if got := m.BookmarkCursor(); got != "renamed" {
			t.Errorf("BookmarkCursor() = %q, want %q", got, "renamed")
		}
	})

	t.Run("a trailing space is a new name", func(t *testing.T) {
		src := bookmarkSource()
		m := press(t, loaded(t, src), "3", "e")

		press(t, typeText(t, m, " "), "enter")

		assertBookmarks(t, src, "feature =childcommit", "main=parentcommit", "split=", "todo=commit1")
	})

	t.Run("a new target moves the bookmark to the resolved commit", func(t *testing.T) {
		src := bookmarkSource()
		m := press(t, loaded(t, src), "3", "e", "tab", "ctrl+u")

		m = press(t, typeText(t, m, "@"), "enter")

		assertBookmarks(t, src, "feature=commit1", "main=parentcommit", "split=", "todo=commit1")
		assertMissing(t, view(m), "Edit bookmark")
		assertContains(t, lineWith(t, view(m), " feature "), "commit1")
	})

	t.Run("a new name and target do both", func(t *testing.T) {
		src := bookmarkSource()
		m := press(t, loaded(t, src), "3", "e", "ctrl+u")
		m = press(t, typeText(t, m, "renamed"), "tab", "ctrl+u")

		press(t, typeText(t, m, "@"), "enter")

		assertBookmarks(t, src, "main=parentcommit", "renamed=commit1", "split=", "todo=commit1")
	})

	t.Run("moving backwards asks before allowing it", func(t *testing.T) {
		src := bookmarkSource()
		src.backwards = true
		m := press(t, loaded(t, src), "3", "e", "tab", "ctrl+u")

		m = press(t, typeText(t, m, "root()"), "enter")

		assertContains(t, view(m), "Edit bookmark", "backwards or sideways", "enter again")
		assertBookmarks(t, src, "feature=childcommit", "main=parentcommit", "split=", "todo=commit1")

		m = press(t, m, "enter")

		assertBookmarks(t, src, "feature=rootcommit", "main=parentcommit", "split=", "todo=commit1")
		assertMissing(t, view(m), "Edit bookmark")
	})

	t.Run("a backwards move is approved before renaming", func(t *testing.T) {
		src := bookmarkSource()
		src.backwards = true
		m := press(t, loaded(t, src), "3", "e", "ctrl+u")
		m = press(t, typeText(t, m, "renamed"), "tab", "ctrl+u")

		m = press(t, typeText(t, m, "root()"), "enter")

		assertContains(t, view(m), "Edit bookmark", "backwards or sideways")
		assertBookmarks(t, src, "feature=childcommit", "main=parentcommit", "split=", "todo=commit1")

		m = press(t, m, "enter")

		assertBookmarks(t, src, "main=parentcommit", "renamed=rootcommit", "split=", "todo=commit1")
		if got := m.BookmarkCursor(); got != "renamed" {
			t.Errorf("BookmarkCursor() = %q, want %q", got, "renamed")
		}
	})

	t.Run("jj warnings are shown before the dialog closes", func(t *testing.T) {
		src := bookmarkSource()
		src.bmWarnings = "Warning: Tracked remote bookmarks for bookmark feature were not renamed."
		m := press(t, loaded(t, src), "3", "e", "ctrl+u")

		m = press(t, typeText(t, m, "renamed"), "enter")

		assertContains(t, view(m), "Edit bookmark", "Tracked remote bookmarks")
		assertBookmarks(t, src, "main=parentcommit", "renamed=childcommit", "split=", "todo=commit1")

		m = press(t, m, "esc")

		assertMissing(t, view(m), "Edit bookmark")
	})
}

func TestBookmarkEdit_failed(t *testing.T) {
	t.Run("editing the input withdraws the backwards approval", func(t *testing.T) {
		src := bookmarkSource()
		src.backwards = true
		m := press(t, loaded(t, src), "3", "e", "tab", "ctrl+u")
		m = press(t, typeText(t, m, "root()"), "enter", "backspace")

		m = press(t, typeText(t, m, ")"), "enter")

		assertContains(t, view(m), "backwards or sideways")
		assertBookmarks(t, src, "feature=childcommit", "main=parentcommit", "split=", "todo=commit1")
	})

	t.Run("the approval does not carry over to another destination", func(t *testing.T) {
		src := bookmarkSource()
		src.backwards = true
		m := press(t, loaded(t, src), "3", "e", "tab", "ctrl+u")
		m = press(t, typeText(t, m, "root()"), "enter")
		src.revisions["root()"] = jj.Change{CommitID: "othercommit"}

		m = press(t, m, "enter")

		assertContains(t, view(m), "Edit bookmark", "backwards or sideways")
		assertBookmarks(t, src, "feature=childcommit", "main=parentcommit", "split=", "todo=commit1")
	})

	t.Run("a bookmark moved meanwhile is not edited", func(t *testing.T) {
		src := bookmarkSource()
		m := press(t, loaded(t, src), "3", "e", "ctrl+u")
		m = typeText(t, m, "renamed")
		src.bookmarks[0].CommitID = "parentcommit"

		m = press(t, m, "enter")

		assertContains(t, view(m), "Edit bookmark", "feature changed")
		assertBookmarks(t, src, "feature=parentcommit", "main=parentcommit", "split=", "todo=commit1")
	})

	t.Run("a bookmark deleted meanwhile is not recreated", func(t *testing.T) {
		src := bookmarkSource()
		m := press(t, loaded(t, src), "3", "e", "tab", "ctrl+u")
		m = typeText(t, m, "@")
		src.bookmarks = src.bookmarks[1:]

		m = press(t, m, "enter")

		assertContains(t, view(m), "Edit bookmark", "feature no longer exists")
		assertBookmarks(t, src, "main=parentcommit", "split=", "todo=commit1")
	})

	t.Run("renaming onto an existing bookmark changes nothing", func(t *testing.T) {
		src := bookmarkSource()
		m := press(t, loaded(t, src), "3", "e", "ctrl+u")
		m = press(t, typeText(t, m, "main"), "tab", "ctrl+u")

		m = press(t, typeText(t, m, "root()"), "enter")

		assertContains(t, view(m), "Edit bookmark", "main already exists")
		assertBookmarks(t, src, "feature=childcommit", "main=parentcommit", "split=", "todo=commit1")
	})

	t.Run("a rename that fails after the move says the move was saved", func(t *testing.T) {
		src := bookmarkSource()
		src.renameErr = errors.New("jj bookmark rename: Failed to parse bookmark name")
		m := press(t, loaded(t, src), "3", "e", "ctrl+u")
		m = press(t, typeText(t, m, "re named"), "tab", "ctrl+u")

		m = press(t, typeText(t, m, "root()"), "enter")

		assertContains(t, view(m), "Edit bookmark", "moved feature to rootcomm", "rename failed")
		assertBookmarks(t, src, "feature=rootcommit", "main=parentcommit", "split=", "todo=commit1")
		assertContains(t, lineWith(t, view(m), " feature "), "rootcomm")
	})

	t.Run("a rename followed by an invalid move changes nothing", func(t *testing.T) {
		src := bookmarkSource()
		src.revErrs = map[string]error{"nope": errors.New("Revision `nope` doesn't exist")}
		m := press(t, loaded(t, src), "3", "e", "ctrl+u")
		m = press(t, typeText(t, m, "renamed"), "tab", "ctrl+u")

		m = press(t, typeText(t, m, "nope"), "enter")

		assertContains(t, view(m), "Edit bookmark", "doesn't exist")
		assertBookmarks(t, src, "feature=childcommit", "main=parentcommit", "split=", "todo=commit1")
	})

	t.Run("e without bookmarks does nothing", func(t *testing.T) {
		m := press(t, loaded(t, revisionSource()), "3", "e")

		assertMissing(t, view(m), "Edit bookmark")
	})
}

func TestBookmarkDelete_success(t *testing.T) {
	t.Run("d shows the bookmark and y deletes it", func(t *testing.T) {
		src := bookmarkSource()
		m := press(t, loaded(t, src), "3", "j", "d")

		assertContains(t, boxText(t, view(m), "Delete bookmark"), "main", "pqrspare", "parentco", "parent", "y delete · any other key cancels")
		assertBookmarks(t, src, "feature=childcommit", "main=parentcommit", "split=", "todo=commit1")

		m = press(t, m, "y")

		assertBookmarks(t, src, "feature=childcommit", "split=", "todo=commit1")
		assertMissing(t, view(m), "Delete bookmark", " main ")
		if got := m.BookmarkCursor(); got != "split" {
			t.Errorf("BookmarkCursor() = %q, want %q", got, "split")
		}
	})

	t.Run("holding y deletes exactly one bookmark", func(t *testing.T) {
		src := bookmarkSource()

		press(t, loaded(t, src), "3", "d", "y", "y", "y")

		assertBookmarks(t, src, "main=parentcommit", "split=", "todo=commit1")
	})

	t.Run("holding d deletes nothing", func(t *testing.T) {
		src := bookmarkSource()
		m := press(t, loaded(t, src), "3", "d", "d", "d", "d", "d")
		for range 4 {
			m, _ = update(t, m, tea.KeyPressMsg{Code: 'd', Text: "d", IsRepeat: true})
		}

		assertBookmarks(t, src, "feature=childcommit", "main=parentcommit", "split=", "todo=commit1")
	})

	t.Run("another key cancels without acting", func(t *testing.T) {
		src := bookmarkSource()

		m := press(t, loaded(t, src), "3", "d", "j")

		assertMissing(t, view(m), "Delete bookmark")
		assertBookmarks(t, src, "feature=childcommit", "main=parentcommit", "split=", "todo=commit1")
		if got := m.BookmarkCursor(); got != "feature" {
			t.Errorf("BookmarkCursor() = %q, want %q", got, "feature")
		}
	})

	t.Run("a tab key closes the confirmation", func(t *testing.T) {
		src := bookmarkSource()

		m := press(t, loaded(t, src), "3", "d", "1", "3")

		assertMissing(t, view(m), "Delete bookmark")
		assertBookmarks(t, src, "feature=childcommit", "main=parentcommit", "split=", "todo=commit1")
	})

	t.Run("a long name is wrapped, not clipped", func(t *testing.T) {
		src := bookmarkSource()
		long := strings.Repeat("abcdefghij", 12) + "TAILEND"
		src.bookmarks[0].Name = long

		m := press(t, start(t, src, nil, 80, 30), "3", "d")

		assertContains(t, view(m), "Delete bookmark", "TAILEND")
	})

	t.Run("a conflicted bookmark can be deleted", func(t *testing.T) {
		src := bookmarkSource()

		m := press(t, loaded(t, src), "3", "j", "j", "d")
		assertContains(t, view(m), "Delete bookmark", "conflicted")
		press(t, m, "y")

		assertBookmarks(t, src, "feature=childcommit", "main=parentcommit", "todo=commit1")
	})

	t.Run("deleting the last bookmark keeps the cursor on the list", func(t *testing.T) {
		src := bookmarkSource()

		m := press(t, loaded(t, src), "3", "G", "d", "y")

		if got := m.BookmarkCursor(); got != "split" {
			t.Errorf("BookmarkCursor() = %q, want %q", got, "split")
		}
	})

	t.Run("jj warnings are shown with the outcome", func(t *testing.T) {
		src := bookmarkSource()
		src.bmWarnings = "Warning: something to know"

		m := press(t, loaded(t, src), "3", "d", "y")

		assertContains(t, view(m), "deleted feature", "something to know")
	})
}

func TestBookmarkDelete_confirmationFollowsTheTarget(t *testing.T) {
	tests := map[string]struct {
		keys   []string
		change func(src *fakeSource)
		want   []string
	}{
		"a moved bookmark": {
			keys:   []string{"3"},
			change: func(src *fakeSource) { src.bookmarks[0].CommitID = "parentcommit" },
			want:   []string{"feature=parentcommit", "main=parentcommit", "split=", "todo=commit1"},
		},
		"a conflicted bookmark with other targets": {
			keys:   []string{"3", "j", "j"},
			change: func(src *fakeSource) { src.bookmarks[2].Removed = []string{"commit1"} },
			want:   []string{"feature=childcommit", "main=parentcommit", "split=", "todo=commit1"},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			src := bookmarkSource()
			m := press(t, loaded(t, src), tt.keys...)
			m, refresh := update(t, m, keyMsg("R"))
			m = press(t, m, "d")
			tt.change(src)
			m = run(t, m, refresh)

			m = press(t, m, "y")

			assertContains(t, view(m), "changed since you pressed d")
			assertBookmarks(t, src, tt.want...)
		})
	}
}

func TestBookmarkDelete_failed(t *testing.T) {
	t.Run("jj errors are shown", func(t *testing.T) {
		src := bookmarkSource()
		m := press(t, loaded(t, src), "3", "d")
		src.bmErr = errors.New("jj refused")

		m = press(t, m, "y")

		assertBookmarksTab(t, m)
		assertContains(t, view(m), "jj refused")
	})

	t.Run("a bookmark moved meanwhile is not deleted", func(t *testing.T) {
		src := bookmarkSource()
		m := press(t, loaded(t, src), "3", "d")
		src.bookmarks[0].CommitID = "parentcommit"

		m = press(t, m, "y")

		assertContains(t, view(m), "feature changed")
		assertBookmarks(t, src, "feature=parentcommit", "main=parentcommit", "split=", "todo=commit1")
	})
}

func TestBookmarkModal_shortTerminal(t *testing.T) {
	t.Run("a long error keeps the footer", func(t *testing.T) {
		src := bookmarkSource()
		src.bmErr = errors.New(strings.Repeat("Warning: failed to export a bookmark\n", 10) + "Error: boom")
		m := press(t, start(t, src, nil, 80, 20), "3", "n")

		m = press(t, typeText(t, m, "topic"), "enter")

		assertContains(t, view(m), "esc cancel", "…")
	})

	for _, height := range []int{8, 12, 16} {
		t.Run(fmt.Sprintf("the backwards confirmation stays visible at height %d", height), func(t *testing.T) {
			src := bookmarkSource()
			src.backwards = true
			m := press(t, start(t, src, nil, 80, height), "3", "e", "tab", "ctrl+u")

			m = press(t, typeText(t, m, "root()"), "enter")

			assertContains(t, view(m), "backwards or sideways", "enter move anyway")
		})
	}
}

func TestBookmarkRound2_success(t *testing.T) {
	t.Run("a failed rename after a move is retried without moving again", func(t *testing.T) {
		src := bookmarkSource()
		src.renameErr = errors.New("jj bookmark rename: Failed to parse bookmark name")
		src.bmWarnings = "Warning: something about the move"
		m := press(t, loaded(t, src), "3", "e", "ctrl+u")
		m = press(t, typeText(t, m, "renamed"), "tab", "ctrl+u")
		m = press(t, typeText(t, m, "root()"), "enter")
		assertContains(t, view(m), "rename failed", "something about the move", "rootcomm")
		src.renameErr = nil

		press(t, m, "enter")

		assertBookmarks(t, src, "main=parentcommit", "renamed=rootcommit", "split=", "todo=commit1")
		if len(src.moves) != 1 {
			t.Errorf("moves = %v, want one", src.moves)
		}
	})

	t.Run("a name with a tab is kept when only the target changes", func(t *testing.T) {
		src := bookmarkSource()
		src.bookmarks = []jj.Bookmark{{Name: "a\tb", ChangeID: "wxyzchild000", CommitID: "childcommit"}}
		m := press(t, loaded(t, src), "3", "e", "tab", "ctrl+u")

		press(t, typeText(t, m, "@"), "enter")

		assertBookmarks(t, src, "a\tb=commit1")
	})

	t.Run("a bookmark change while the log is shown reloads the log", func(t *testing.T) {
		src := bookmarkSource()
		m := press(t, loaded(t, src), "2", "3", "d")
		m, cmd := update(t, m, keyMsg("y"))
		m = press(t, m, "2")

		run(t, m, cmd)

		if src.logCalls != 2 {
			t.Errorf("Log() calls = %d, want 2", src.logCalls)
		}
	})

	t.Run("a background bookmark load keeps the log confirmation", func(t *testing.T) {
		src := bookmarkSource()
		m := press(t, loaded(t, src), "tab", "j", "tab", "space", "3")
		m, refresh := update(t, m, keyMsg("R"))
		m = press(t, m, "2", "j", "enter")
		assertContains(t, view(m), "press enter again")
		src.bookmarks[0].CommitID = "elsewhere"

		m = run(t, m, refresh)

		assertContains(t, view(m), "press enter again")
	})

	t.Run("a background log load keeps the bookmarks confirmation", func(t *testing.T) {
		src := bookmarkSource()
		m := press(t, loaded(t, src), "tab", "j", "tab", "space", "2")
		m, refresh := update(t, m, keyMsg("R"))
		m = press(t, m, "3", "enter")
		assertContains(t, view(m), "press enter again")
		src.log = logLines()[1:]

		m = run(t, m, refresh)

		assertContains(t, view(m), "press enter again")
	})

	t.Run("a warning after saving shows its text in a short terminal", func(t *testing.T) {
		src := bookmarkSource()
		src.bmWarnings = "Warning: Tracked remote bookmarks for bookmark feature were not renamed.\nHint: push the old name first."
		m := press(t, start(t, src, nil, 80, 7), "3", "e", "ctrl+u")

		m = press(t, typeText(t, m, "renamed"), "enter")

		assertContains(t, view(m), "Tracked remote", "close")
	})

	t.Run("a tiny terminal keeps the focused field and the confirm key", func(t *testing.T) {
		src := bookmarkSource()
		src.backwards = true
		m := press(t, start(t, src, nil, 80, 6), "3", "e", "tab", "ctrl+u")

		m = press(t, typeText(t, m, "root()"), "enter")

		assertContains(t, view(m), "root()", "enter move anyway")
	})
}

func TestBookmarkRound2_failed(t *testing.T) {
	t.Run("a change right before the move is not overwritten", func(t *testing.T) {
		src := bookmarkSource()
		m := press(t, loaded(t, src), "3", "e", "tab", "ctrl+u")
		m = typeText(t, m, "@")
		src.beforeMove = func() { src.bookmarks[0].CommitID = "parentcommit" }

		m = press(t, m, "enter")

		assertContains(t, view(m), "Edit bookmark", "feature changed")
		assertBookmarks(t, src, "feature=parentcommit", "main=parentcommit", "split=", "todo=commit1")
	})

	t.Run("a conflicted bookmark is not edited", func(t *testing.T) {
		src := bookmarkSource()

		m := press(t, loaded(t, src), "3", "j", "j", "e")

		assertMissing(t, view(m), "Edit bookmark")
		assertContains(t, view(m), "split is conflicted; resolve it with jj bookmark set")
	})

	t.Run("a short dialog shows the error and the outcome before the warnings", func(t *testing.T) {
		src := bookmarkSource()
		src.renameErr = errors.New("jj bookmark rename: exit status 1:\nError: Failed to parse bookmark name\nCaused by: something\nHint: quote it")
		src.bmWarnings = "Warning: one\nWarning: two\nWarning: three"
		m := press(t, start(t, src, nil, 80, 10), "3", "e", "ctrl+u")
		m = press(t, typeText(t, m, "renamed"), "tab", "ctrl+u")

		m = press(t, typeText(t, m, "root()"), "enter")

		assertContains(t, view(m), "rename failed", "now: feature → rootcomm")
	})

	t.Run("an edited name with a tab uses exactly what was typed", func(t *testing.T) {
		src := bookmarkSource()
		src.bookmarks = []jj.Bookmark{{Name: "a\tb", ChangeID: "wxyzchild000", CommitID: "childcommit"}}
		m := press(t, loaded(t, src), "3", "e", "ctrl+u")

		press(t, typeText(t, m, "a b"), "enter")

		assertBookmarks(t, src, "a b=childcommit")
	})

	t.Run("a create that jj published before failing reports the outcome", func(t *testing.T) {
		src := bookmarkSource()
		src.publishErr = errors.New("jj crashed")
		m := press(t, loaded(t, src), "3", "n")

		m = press(t, typeText(t, m, "topic"), "enter")

		assertContains(t, view(m), "jj crashed", "now: topic → commit1")
		assertContains(t, lineWith(t, view(m), " topic "), "commit1")
	})

	t.Run("a delete that jj published before failing reports the outcome", func(t *testing.T) {
		src := bookmarkSource()
		src.publishErr = errors.New("jj crashed")

		m := press(t, loaded(t, src), "3", "d", "y")

		assertContains(t, view(m), "jj crashed", "now: feature does not exist", "bookmark 1/3")
	})
}

func boxText(t *testing.T, screen, title string) string {
	t.Helper()
	lines := strings.Split(screen, "\n")
	for i, line := range lines {
		before, _, found := strings.Cut(line, title)
		if !found {
			continue
		}
		runes := []rune(line)
		start := len([]rune(before))
		left := start - 1
		for left >= 0 && runes[left] != '│' {
			left--
		}
		right := start
		for right < len(runes) && runes[right] != '│' {
			right++
		}
		var box []string
		for _, l := range lines[i:] {
			r := []rune(l)
			if left < 0 || right > len(r) || r[left] != '│' {
				break
			}
			box = append(box, string(r[left+1:right]))
		}
		return strings.Join(box, "\n")
	}
	t.Fatalf("View() has no box titled %q:\n%s", title, screen)
	return ""
}

func TestBookmarkModal_targetFits(t *testing.T) {
	for _, size := range [][2]int{{64, 11}, {80, 11}, {64, 30}, {80, 30}} {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			src := bookmarkSource()
			full := "0123456789abcdef0123456789abcdef01234567"
			src.head = full
			src.change.CommitID = full

			m := press(t, start(t, src, nil, size[0], size[1]), "3", "n", "tab")

			box := boxText(t, view(m), "New bookmark")
			assertContains(t, box, "kxyzabcd")
			assertMissing(t, box, "0123456789abcdef")
		})
	}
}

func TestTopBar_narrow(t *testing.T) {
	for _, width := range []int{90, 80} {
		t.Run(fmt.Sprintf("%d columns", width), func(t *testing.T) {
			src := bookmarkSource()
			m := press(t, start(t, src, nil, width, 20), "tab", "j", "tab", "space")

			top, _, _ := strings.Cut(view(m), "\n")

			assertContains(t, top, "1 Review", "2 Log", "3 Bookmarks", "4 Operations", "@ kxyzabcd")
		})
	}

	t.Run("very narrow keeps the tab keys and the revision", func(t *testing.T) {
		m := start(t, bookmarkSource(), nil, 40, 20)

		top, _, _ := strings.Cut(view(m), "\n")

		assertContains(t, top, " 1 ", " 2 ", " 3 ", " 4 ", "@ kxyzabcd")
	})
}

func TestTopBar_subject(t *testing.T) {
	src := newSource()
	src.change.Description = "fix: answer 404 for unknown codes\n"
	m := press(t, start(t, src, nil, 102, 20), "f")

	top, _, _ := strings.Cut(view(m), "\n")

	assertContains(t, top, "1 Review", "2 Log", "3 Bookmarks", "@ kxyzabcd", "fix: answer 404 for unknown codes")
}

func TestTopBar_keepsBadges(t *testing.T) {
	t.Run("a selection keeps its badge and truncates the subject", func(t *testing.T) {
		src := newSource()
		src.change.Description = "fix: answer 404 for unknown codes\n"
		m := press(t, start(t, src, nil, 94, 20), "f", "tab", "j", "tab", "space")

		top, _, _ := strings.Cut(view(m), "\n")

		assertContains(t, top, "1 Review", "2 Log", "3 Bookmarks", "@ kxyzabcd", "1 hunk selected", "fix: answer")
		assertMissing(t, top, "fix: answer 404 for unknown codes", "full file")
	})

	t.Run("an immutable revision keeps the read only badge", func(t *testing.T) {
		src := newSource()
		src.change.Description = "fix: answer 404 for unknown codes\n"
		src.change.Immutable = true
		m := start(t, src, nil, 94, 20)

		top, _, _ := strings.Cut(view(m), "\n")

		assertContains(t, top, "1 Review", "2 Log", "3 Bookmarks", "@ kxyzabcd", "read only", "fix: answer")
	})
}

func TestBookmarkCreate_hiddenTarget(t *testing.T) {
	src := bookmarkSource()
	src.revErrs = map[string]error{
		jj.VisibleRevset(jj.CommitRevset("commit1")): fmt.Errorf("jj log: %w", jj.ErrNoRevision),
	}
	m := press(t, loaded(t, src), "3", "n")

	m = press(t, typeText(t, m, "topic"), "enter")

	assertContains(t, boxText(t, view(m), "New bookmark"), "is no longer visible; press R in", "the review or choose another revision")
	assertBookmarks(t, src, "feature=childcommit", "main=parentcommit", "split=", "todo=commit1")
}

func TestBookmarkEdit_hiddenTarget(t *testing.T) {
	src := bookmarkSource()
	src.revErrs = map[string]error{
		jj.VisibleRevset("old"): fmt.Errorf("jj log: %w", jj.ErrNoRevision),
	}
	m := press(t, loaded(t, src), "3", "e", "tab", "ctrl+u")

	m = press(t, typeText(t, m, "old"), "enter")

	assertContains(t, boxText(t, view(m), "Edit bookmark"), "no longer visible")
	assertBookmarks(t, src, "feature=childcommit", "main=parentcommit", "split=", "todo=commit1")
}

func TestBookmarksQuit_success(t *testing.T) {
	t.Parallel()

	t.Run("q from the bookmarks tab drops a held editor", func(t *testing.T) {
		t.Parallel()
		var opened []string
		m, hold := update(t, pendingEditor(t, &opened), keyMsg("e"))
		m = press(t, m, "3")
		m, cmd := update(t, m, keyMsg("q"))
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Fatalf("Update(q) cmd() = %T, want tea.QuitMsg", cmd())
		}

		var got []tea.Msg
		for _, msg := range []tea.Msg{darkBg(), hold()} {
			m, cmd = update(t, m, msg)
			got = append(got, messages(t, cmd)...)
		}

		if len(got) != 0 || len(opened) != 0 {
			t.Errorf("messages after quitting = %v, opened = %v, want nothing", got, opened)
		}
	})

	t.Run("keys after q from the bookmarks tab are ignored", func(t *testing.T) {
		t.Parallel()
		m := press(t, loaded(t, bookmarkSource()), "3", "q")

		_, cmd := update(t, m, keyMsg("2"))

		if cmd != nil {
			t.Errorf("Update(2) after quitting messages = %v, want nil cmd", messages(t, cmd))
		}
	})
}
