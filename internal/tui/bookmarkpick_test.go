package tui_test

import (
	"slices"
	"testing"

	"github.com/madalinpopa/jjreview/internal/jj"
	"github.com/madalinpopa/jjreview/internal/tui"
)

func bookmarkTarget(t *testing.T, src *fakeSource, name string) string {
	t.Helper()
	i := slices.IndexFunc(src.bookmarks, func(b jj.Bookmark) bool { return b.Name == name })
	if i < 0 {
		t.Fatalf("bookmarks = %+v, want %s", src.bookmarks, name)
	}
	return src.bookmarks[i].CommitID
}

func TestLogBookmark_success(t *testing.T) {
	t.Run("b lists a new bookmark and the existing ones", func(t *testing.T) {
		m := press(t, loaded(t, bookmarkSource()), "2", "b")

		got := view(m)
		assertContains(t, got, "Bookmark kxyzabcd", "+ new bookmark", "feature", "main", "enter choose · esc cancel")
		assertLogTab(t, m)
	})

	t.Run("a new bookmark is created on the revision under the cursor", func(t *testing.T) {
		src := bookmarkSource()
		m := press(t, loaded(t, src), "2", "j", "b", "enter")
		assertContains(t, view(m), "New bookmark", "Target revision · child feature", "wxyzchil")

		m = press(t, typeText(t, m, "fresh"), "enter")

		if got := bookmarkTarget(t, src, "fresh"); got != "childcommit" {
			t.Errorf("fresh target = %q, want %q", got, "childcommit")
		}
		assertLogTab(t, m)
	})

	t.Run("enter on an existing bookmark moves it to the revision under the cursor", func(t *testing.T) {
		src := bookmarkSource()

		m := press(t, loaded(t, src), "2", "j", "b", "j", "j", "enter")

		if got := bookmarkTarget(t, src, "main"); got != "childcommit" {
			t.Errorf("main target = %q, want %q", got, "childcommit")
		}
		got := view(m)
		assertContains(t, got, "moved main to wxyzchil")
		assertMissing(t, got, "Edit bookmark", "+ new bookmark")
		assertLogTab(t, m)
	})

	t.Run("a backwards move waits for y", func(t *testing.T) {
		src := bookmarkSource()
		src.backwards = true
		m := press(t, loaded(t, src), "2", "j", "b", "j", "j", "enter")
		assertContains(t, view(m), "main would move backwards or sideways; press y to move", "anyway", "y move anyway · esc cancel")
		if got := bookmarkTarget(t, src, "main"); got != "parentcommit" {
			t.Errorf("main target before y = %q, want %q", got, "parentcommit")
		}

		m = press(t, m, "y")

		if got := bookmarkTarget(t, src, "main"); got != "childcommit" {
			t.Errorf("main target = %q, want %q", got, "childcommit")
		}
		assertContains(t, view(m), "moved main to wxyzchil")
	})

	t.Run("the key bar lists the bookmark key", func(t *testing.T) {
		assertContains(t, view(press(t, loaded(t, bookmarkSource()), "2")), "b bookmark")
	})
}

func TestLogBookmark_cancelled(t *testing.T) {
	t.Run("esc closes the list without changes", func(t *testing.T) {
		src := bookmarkSource()
		before := slices.Clone(src.bookmarks)

		m := press(t, loaded(t, src), "2", "b", "esc")

		assertMissing(t, view(m), "+ new bookmark")
		if len(src.bookmarks) != len(before) {
			t.Errorf("bookmarks = %+v, want %+v", src.bookmarks, before)
		}
		assertLogTab(t, m)
	})
}

func TestLogBookmark_refused(t *testing.T) {
	t.Run("a bookmark already on the revision", func(t *testing.T) {
		m := press(t, loaded(t, bookmarkSource()), "2", "b", "G", "enter")

		got := view(m)
		assertContains(t, got, "todo is already on kxyzabcd")
		assertMissing(t, got, "Edit bookmark")
	})

	t.Run("a conflicted bookmark", func(t *testing.T) {
		m := press(t, loaded(t, bookmarkSource()), "2", "b", "G", "k", "enter")

		got := view(m)
		assertContains(t, got, "split is conflicted; resolve it with jj bookmark set")
		assertMissing(t, got, "Edit bookmark")
	})
}

func pressRepeat(t *testing.T, m tui.Model, k string) tui.Model {
	t.Helper()
	m, cmd := update(t, m, repeatKey(k))
	return run(t, m, cmd)
}

func TestLogBookmarkRepeat_ignored(t *testing.T) {
	t.Run("a repeated enter does not move the bookmark", func(t *testing.T) {
		src := bookmarkSource()
		m := press(t, loaded(t, src), "2", "j", "b", "j", "j")

		m = pressRepeat(t, m, "enter")

		assertContains(t, view(m), "+ new bookmark")
		if got := bookmarkTarget(t, src, "main"); got != "parentcommit" {
			t.Errorf("main target = %q, want %q", got, "parentcommit")
		}
	})

	t.Run("a repeated y does not allow a backwards move", func(t *testing.T) {
		src := bookmarkSource()
		src.backwards = true
		m := press(t, loaded(t, src), "2", "j", "b", "j", "j", "enter")

		pressRepeat(t, m, "y")

		if got := bookmarkTarget(t, src, "main"); got != "parentcommit" {
			t.Errorf("main target = %q, want %q", got, "parentcommit")
		}
	})
}

func TestLogBookmarkBackwards_cancelled(t *testing.T) {
	t.Run("another key clears the approval and still navigates", func(t *testing.T) {
		src := bookmarkSource()
		src.backwards = true
		m := press(t, loaded(t, src), "2", "j", "b", "j", "j", "enter")

		m = press(t, m, "k")

		got := view(m)
		assertContains(t, got, "+ new bookmark", "enter choose")
		assertMissing(t, got, "press y to move anyway")

		m = press(t, m, "y")

		if got := bookmarkTarget(t, src, "main"); got != "parentcommit" {
			t.Errorf("main target = %q, want %q", got, "parentcommit")
		}
		assertContains(t, view(m), "+ new bookmark")
	})
}

func TestLogBookmarkStale_success(t *testing.T) {
	t.Run("the list cannot be chosen from until the bookmarks reload", func(t *testing.T) {
		src := bookmarkSource()
		m := press(t, loaded(t, src), "3", "2", "ctrl+r")
		src.bookmarks = slices.Insert(src.bookmarks, 0, jj.Bookmark{Name: "alpha", ChangeID: "zzzzroot0000", CommitID: "rootcommit"})

		m, reload := update(t, m, keyMsg("b"))
		m = press(t, m, "j", "j", "enter")

		got := view(m)
		assertContains(t, got, "loading…")
		assertMissing(t, got, "Edit bookmark")

		press(t, run(t, m, reload), "j", "j", "enter")

		if got := bookmarkTarget(t, src, "feature"); got != "commit1" {
			t.Errorf("feature target = %q, want %q", got, "commit1")
		}
		if got := bookmarkTarget(t, src, "main"); got != "parentcommit" {
			t.Errorf("main target = %q, want %q", got, "parentcommit")
		}
	})
}

func TestLogBookmarkCursor_success(t *testing.T) {
	t.Run("the log cursor stays on the bookmarked revision after a save", func(t *testing.T) {
		src := bookmarkSource()
		m := press(t, loaded(t, src), "2", "j", "b", "j", "j", "enter")

		if got := m.LogCursor(); got != "wxyzchild000" {
			t.Errorf("LogCursor() = %q, want %q", got, "wxyzchild000")
		}
	})

	t.Run("a move from the log keeps the bookmarks tab cursor", func(t *testing.T) {
		src := bookmarkSource()
		m := press(t, loaded(t, src), "3", "G", "2", "j", "b", "j", "j", "enter")

		m = press(t, m, "3")

		if got := m.BookmarkCursor(); got != "todo" {
			t.Errorf("BookmarkCursor() = %q, want %q", got, "todo")
		}
	})

	t.Run("a new bookmark from the log keeps the bookmarks tab cursor", func(t *testing.T) {
		src := bookmarkSource()
		m := press(t, loaded(t, src), "3", "G", "2", "j", "b", "enter")
		m = press(t, typeText(t, m, "fresh"), "enter")

		m = press(t, m, "3")

		if got := m.BookmarkCursor(); got != "todo" {
			t.Errorf("BookmarkCursor() = %q, want %q", got, "todo")
		}
	})
}

func TestLogBookmarkMoving_success(t *testing.T) {
	t.Run("keys wait while the move runs", func(t *testing.T) {
		src := bookmarkSource()
		m := press(t, loaded(t, src), "2", "j", "b", "j", "j")
		m, move := update(t, m, keyMsg("enter"))

		m = press(t, m, "k", "enter")

		assertContains(t, view(m), "moving…")
		run(t, m, move)
		if got := bookmarkTarget(t, src, "main"); got != "childcommit" {
			t.Errorf("main target = %q, want %q", got, "childcommit")
		}
		if got := bookmarkTarget(t, src, "feature"); got != "childcommit" {
			t.Errorf("feature target = %q, want %q", got, "childcommit")
		}
	})
}
