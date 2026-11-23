package tui_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/madalinpopa/jjreview/internal/diff"
	"github.com/madalinpopa/jjreview/internal/jj"
	"github.com/madalinpopa/jjreview/internal/tui"
)

type recordedTimer struct {
	calls []tea.Msg
	waits []time.Duration
}

func (r *recordedTimer) after(d time.Duration, msg tea.Msg) tea.Cmd {
	r.calls = append(r.calls, msg)
	r.waits = append(r.waits, d)
	return nil
}

func (r *recordedTimer) expiry(t *testing.T) tea.Msg {
	t.Helper()
	for i, d := range r.waits {
		if d == 3*time.Second {
			return r.calls[i]
		}
	}
	t.Fatalf("timer waits = %v, want a 3s expiry", r.waits)
	return nil
}

func toastText(t *testing.T, m tui.Model) string {
	t.Helper()
	var parts []string
	inside := false
	for line := range strings.SplitSeq(view(m), "\n") {
		row := []rune(line)
		if len(row) < 120 {
			continue
		}
		switch edge := row[71]; {
		case edge == '╭':
			inside = true
		case edge == '╰':
			inside = false
		case inside && edge == '│':
			parts = append(parts, strings.TrimSpace(string(row[73:117])))
		}
	}
	if len(parts) == 0 {
		t.Fatalf("View() shows no toast:\n%s", view(m))
	}
	return strings.Join(parts, " ")
}

func failedFetch(t *testing.T, src *fakeSource) tui.Model {
	t.Helper()
	src.fetchErr = errors.New("fetch failed: offline")
	return press(t, loaded(t, src), "2", "f")
}

func assertToast(t *testing.T, m tui.Model) {
	t.Helper()
	assertContains(t, view(m), "✗ Fetch", "jj git fetch")
}

func TestCommandToast_success(t *testing.T) {
	t.Run("shows a spinner, the title and the command while running", func(t *testing.T) {
		m := press(t, loaded(t, revisionSource()), "2")

		m, _ = update(t, m, keyMsg("f"))

		assertContains(t, view(m), "⠋ Fetch", "jj git fetch")
	})

	t.Run("reports a success once, in the toast", func(t *testing.T) {
		src := revisionSource()
		src.fetchChanged = true

		got := view(press(t, loaded(t, src), "2", "f"))

		assertContains(t, got, "✓ Fetch", "jj git fetch")
		if n := strings.Count(got, "fetched"); n != 1 {
			t.Errorf("View() shows fetched %d times, want once:\n%s", n, got)
		}
	})

	t.Run("expires a success through the toast timer", func(t *testing.T) {
		timer := &recordedTimer{}
		m := tui.New(t.Context(), revisionSource(), nil, "@").WithToastTimer(timer.after)
		m, _ = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 30})
		m = run(t, m, m.Init())
		m = press(t, m, "2", "f")

		m = run(t, m, func() tea.Msg { return timer.expiry(t) })

		assertMissing(t, view(m), "Fetch", "already up to date")
	})

	t.Run("keeps a failure until x dismisses it", func(t *testing.T) {
		m := failedFetch(t, revisionSource())

		m = press(t, m, "j")
		assertToast(t, m)
		m = press(t, m, "x")

		assertMissing(t, view(m), "✗ Fetch", "fetch failed")
	})

	t.Run("sits at the bottom right of the body above the bottom bar", func(t *testing.T) {
		lines := strings.Split(view(failedFetch(t, revisionSource())), "\n")

		card := -1
		for i, l := range lines {
			if strings.HasPrefix(l, "╰") {
				card = i
			}
		}
		if card < 1 {
			t.Fatalf("View() has no card bottom border:\n%s", strings.Join(lines, "\n"))
		}
		bottom := lines[card-1]
		before, _, found := strings.CutLast(bottom, "╯")
		if !found || lipgloss.Width(before) != 118 || !strings.Contains(lines[card-2], "dismiss") {
			t.Errorf("toast bottom row = %q, want it to end at column 118 right above the card border", bottom)
		}
	})

	t.Run("reports a bookmark delete", func(t *testing.T) {
		got := view(press(t, loaded(t, bookmarkSource()), "3", "d", "y"))

		assertContains(t, got, "✓ Delete bookmark", "jj bookmark delete")
		if n := strings.Count(got, "deleted "); n != 1 {
			t.Errorf("View() shows the delete result %d times, want once:\n%s", n, got)
		}
	})

	t.Run("reports an operation restore", func(t *testing.T) {
		got := view(press(t, loaded(t, operationSource()), "4", "j", "j", "r", "y"))

		assertContains(t, got, "✓ Restore operation cccc3333", "jj operation restore cccc3333", "restored operation cccc3333")
	})

	t.Run("keeps restoring the log cursor after an abandon", func(t *testing.T) {
		src := revisionSource()
		m := press(t, loaded(t, src), "2", "j", "a")
		src.log = withoutEntry(logLines(), "childcommit")

		m = press(t, m, "y")

		assertContains(t, view(m), "✓ Abandon wxyzchil", "jj abandon -r wxyzchil")
		if got := m.LogCursor(); got != "pqrsparent00" {
			t.Errorf("LogCursor() = %q, want the next revision", got)
		}
	})
}

func TestCommandToast_fallback(t *testing.T) {
	t.Run("a short terminal reports the result in the status line", func(t *testing.T) {
		src := revisionSource()
		src.fetchChanged = true
		m := press(t, start(t, src, nil, 120, 7), "2", "f")

		got := view(m)

		assertContains(t, got, "fetched")
		assertMissing(t, got, "jj git fetch")
	})

	t.Run("an expired status-line result gives its row back to the diff", func(t *testing.T) {
		src := bookmarkSource()
		src.bookmarks = append(src.bookmarks, jj.Bookmark{Name: "zz" + strings.Repeat("y", 110), ChangeID: "kxyzabcdefgh", CommitID: "commit1"})
		var lines []diff.Line
		for i := range 40 {
			lines = append(lines, diff.Line{Kind: diff.LineAdded, Text: fmt.Sprintf("line %d", i+1), NewNo: i + 1})
		}
		src.files = []diff.File{{NewPath: "big.go", NewMode: "100644", Status: diff.StatusAdded, Hunks: []diff.Hunk{{NewStart: 1, NewLines: 40, Lines: lines}}}}
		timer := &recordedTimer{}
		m := tui.New(t.Context(), src, nil, "@").WithToastTimer(timer.after)
		m, _ = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 8})
		m = run(t, m, m.Init())
		m = press(t, m, "3", "G", "d", "y", "1", "G")
		assertContains(t, statusLine(m), "deleted zzyyy")

		m = run(t, m, func() tea.Msg { return timer.expiry(t) })

		assertContains(t, view(m), "39 + line 39", "40 + line 40")
	})

	t.Run("an open modal reports the result in the status line", func(t *testing.T) {
		m := failedFetch(t, revisionSource())

		got := view(press(t, m, "a"))

		assertContains(t, got, "fetch: fetch failed: offline", "y abandon")
		assertMissing(t, got, "jj git fetch")
	})

	t.Run("a failed reload stays in the status line next to the toast", func(t *testing.T) {
		src := revisionSource()
		m := press(t, loaded(t, src), "2", "j", "j", "a")
		src.revErrs = map[string]error{jj.VisibleCommitRevset("commit1"): errors.New("jj exploded")}

		m = press(t, m, "y")

		got := view(m)
		assertContains(t, got, "✓ Abandon pqrspare", "jj exploded")
		if n := strings.Count(got, "abandoned pqrspare"); n != 1 {
			t.Errorf("View() shows the abandon result %d times, want once:\n%s", n, got)
		}
		assertContains(t, view(press(t, m, "1")), "review out of date")
	})

	t.Run("a lost target stays in the status line next to the toast", func(t *testing.T) {
		src := revisionSource()
		m := press(t, loaded(t, src), "2", "j", "enter", "2", "j", "a")
		src.revErrs = map[string]error{jj.VisibleCommitRevset("childcommit"): noRevision()}

		m = press(t, m, "y")

		got := view(m)
		assertContains(t, got, "✓ Abandon pqrspare", "wxyzchil was rewritten, reviewing the working copy")
		if n := strings.Count(got, "abandoned pqrspare"); n != 1 {
			t.Errorf("View() shows the abandon result %d times, want once:\n%s", n, got)
		}
	})
}

func TestCommandToast_keys(t *testing.T) {
	t.Run("x is ignored without a finished toast", func(t *testing.T) {
		m := press(t, loaded(t, revisionSource()), "2")
		m, _ = update(t, m, keyMsg("f"))

		m, _ = update(t, m, keyMsg("x"))

		assertContains(t, view(m), "⠋ Fetch")
	})

	t.Run("x dismissing the toast keeps an open line range", func(t *testing.T) {
		m := press(t, failedFetch(t, revisionSource()), "1", "tab", "j", "tab", "v")

		m = press(t, m, "x")

		got := view(m)
		assertContains(t, got, "selecting a range")
		assertMissing(t, got, "✗ Fetch")
	})

	t.Run("x typed into the review search stays in the query", func(t *testing.T) {
		m := press(t, failedFetch(t, revisionSource()), "1", "/", "x")

		assertToast(t, m)
		assertContains(t, view(m), "/ x")
	})

	t.Run("x typed into the log filter stays in the query", func(t *testing.T) {
		m := press(t, failedFetch(t, revisionSource()), "/", "x")

		assertToast(t, m)
		assertContains(t, view(m), "/ x")
	})

	t.Run("help lists the dismiss key", func(t *testing.T) {
		m := press(t, start(t, revisionSource(), nil, 120, 60), "2", "?")

		assertContains(t, lineWith(t, view(m), "dismiss the notification"), "x ")
	})

	escapes := map[string]struct {
		keys    []string
		missing string
	}{
		"review search while typing": {keys: []string{"1", "/", "T", "esc"}, missing: "/ T"},
		"active review search":       {keys: []string{"1", "/", "T", "U", "I", "enter", "esc"}, missing: "/TUI"},
		"review line range":          {keys: []string{"1", "tab", "j", "tab", "v", "esc"}, missing: "selecting a range"},
		"log filter":                 {keys: []string{"/", "p", "q", "esc"}, missing: "/ pq"},
		"log marks":                  {keys: []string{"space", "esc"}, missing: "1 selected"},
		"log squash":                 {keys: []string{"s", "esc"}, missing: " into "},
		"log abandon confirmation":   {keys: []string{"a", "esc"}, missing: "y abandon"},
		"operations confirmation":    {keys: []string{"4", "j", "j", "r", "esc"}, missing: "y restore"},
	}
	for name, tt := range escapes {
		t.Run("esc in the "+name+" keeps the toast", func(t *testing.T) {
			m := press(t, failedFetch(t, operationSource()), tt.keys...)

			assertToast(t, m)
			assertMissing(t, view(m), tt.missing)
		})
	}
}

func pending(t *testing.T, cmd tea.Cmd) []tea.Msg {
	t.Helper()
	var out []tea.Msg
	queue := []tea.Cmd{cmd}
	for len(queue) > 0 {
		next := queue[0]
		queue = queue[1:]
		if next == nil {
			continue
		}
		switch msg := next().(type) {
		case tea.BatchMsg:
			queue = append(queue, msg...)
		default:
			out = append(out, msg)
		}
	}
	return out
}

func deliver(t *testing.T, m tui.Model, msgs []tea.Msg) tui.Model {
	t.Helper()
	for _, msg := range msgs {
		m = run(t, m, func() tea.Msg { return msg })
	}
	return m
}

func statusLine(m tui.Model) string {
	lines := strings.Split(view(m), "\n")
	return lines[len(lines)-1]
}

func TestCommandToast_overlap(t *testing.T) {
	t.Run("a fetch overtaken by a bookmark delete reports its failure in the status line", func(t *testing.T) {
		src := bookmarkSource()
		src.fetchErr = errors.New("fetch failed: offline")
		m := press(t, loaded(t, src), "2")
		m, fetch := update(t, m, keyMsg("f"))
		fetched := pending(t, fetch)
		m = press(t, m, "3", "d", "y")

		m = deliver(t, m, fetched)

		assertContains(t, view(m), "✓ Delete bookmark")
		assertContains(t, statusLine(m), "fetch failed: offline")
	})

	t.Run("a bookmark delete overtaken by another reports its failure in the status line", func(t *testing.T) {
		src := bookmarkSource()
		m := press(t, loaded(t, src), "3", "d")
		m, del := update(t, m, keyMsg("y"))
		src.bmErr = errors.New("delete refused")
		deleted := pending(t, del)
		src.bmErr = nil
		m = press(t, m, "j", "d", "y")

		m = deliver(t, m, deleted)

		assertContains(t, view(m), "✓ Delete bookmark")
		assertContains(t, statusLine(m), "delete refused")
	})
}
