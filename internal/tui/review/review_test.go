package review_test

import (
	"context"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/madalinpopa/jjreview/internal/diff"
	"github.com/madalinpopa/jjreview/internal/jj"
	"github.com/madalinpopa/jjreview/internal/theme"
	"github.com/madalinpopa/jjreview/internal/tui/review"
	"github.com/madalinpopa/jjreview/internal/tui/tab"
	"github.com/madalinpopa/jjreview/internal/tui/ui"
)

type fakeSource struct {
	requested []string
	splits    []string
	splitErr  error
}

func (s *fakeSource) FileContent(_ context.Context, rev, path string) (string, error) {
	s.requested = append(s.requested, rev+":"+path)
	return "ctx\n", nil
}

func (s *fakeSource) Split(_ context.Context, changeID, commitID, message string, _ []jj.SplitFile) (jj.Change, jj.Change, error) {
	s.splits = append(s.splits, changeID+"@"+commitID+": "+message)
	return jj.Change{ChangeID: "selected123"}, jj.Change{ChangeID: "rest456", CommitID: "resthead"}, s.splitErr
}

func hunk(start int, text string) diff.Hunk {
	return diff.Hunk{OldStart: start, OldLines: 1, NewStart: start, NewLines: 2, Lines: []diff.Line{
		{Kind: diff.LineContext, Text: "ctx", OldNo: start, NewNo: start},
		{Kind: diff.LineAdded, Text: text, NewNo: start + 1},
	}}
}

func file(path string, hunks ...diff.Hunk) diff.File {
	return diff.File{OldPath: path, NewPath: path, Hunks: hunks}
}

func files() []diff.File {
	return []diff.File{file("a.go", hunk(1, "x")), file("b/x.go", hunk(1, "a"), hunk(10, "b"))}
}

func change() jj.Change {
	return jj.Change{ChangeID: "target", CommitID: "head", Parents: []string{"parent"}}
}

func env(c jj.Change) review.Env {
	return review.Env{
		Theme:      theme.Default(),
		Width:      100,
		Height:     30,
		BodyHeight: 28,
		ListHeight: 28 - ui.CardChrome,
		ModalWidth: 64,
		Head:       tab.Head{ChangeID: c.ChangeID, CommitID: c.CommitID, WorkingCopy: c.WorkingCopy},
		Active:     true,
		Change:     c,
		Loaded:     true,
	}
}

func press(k string) tea.KeyPressMsg {
	switch k {
	case "space":
		return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	case "ctrl+s":
		return tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl}
	}
	return tea.KeyPressMsg{Code: rune(k[0]), Text: k}
}

func loaded(t *testing.T, src *fakeSource, e review.Env, focus string) review.Model {
	t.Helper()
	m := review.New(t.Context(), src, nil)
	m.Load(files(), focus, e)
	return m
}

func TestLoad_success(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		focus string
		want  string
	}{
		"focuses the named file":                 {focus: "a.go", want: "a.go"},
		"starts at the first file in tree order": {focus: "", want: "b/x.go"},
		"ignores a file no longer in the change": {focus: "gone.go", want: "b/x.go"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			m := loaded(t, &fakeSource{}, env(change()), tt.focus)

			got, ok := m.CurrentFile()
			if !ok || got.Path() != tt.want {
				t.Errorf("CurrentFile() = %q, %v, want %q", got.Path(), ok, tt.want)
			}
		})
	}
}

func TestLoad_keepsFoldsUnlessReset(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		reset bool
		want  string
	}{
		"keeps the folded hunk":       {reset: false, want: "file 1/2 · 2 hunks (1 folded)"},
		"forgets the view when reset": {reset: true, want: "file 1/2 · 2 hunks"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := env(change())
			m := loaded(t, &fakeSource{}, e, "")
			m.Key(press("z"), e)

			if tt.reset {
				m.Reset()
			}
			m.Load(files(), "", e)

			if got := m.Status(true); got != tt.want {
				t.Errorf("Status() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestKeepPicks_success(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		next []diff.File
		want int
	}{
		"keeps the picks of the same files": {next: files(), want: 1},
		"drops the picks of other files":    {next: []diff.File{file("a.go", hunk(3, "a"))}, want: 0},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := env(change())
			m := loaded(t, &fakeSource{}, e, "")
			m.Key(press("space"), e)
			held := m.Hold()

			m.Load(tt.next, "", e)
			m.KeepPicks(held)

			if got := m.Picked(); got != tt.want {
				t.Errorf("Picked() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestKey_requests(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		key  string
		want review.Effect
	}{
		"@ asks for the working copy": {key: "@", want: review.Effect{WorkingCopy: true}},
		"R asks for a reload":         {key: "R", want: review.Effect{Reload: true}},
		"e asks for the editor":       {key: "e", want: review.Effect{Edit: true}},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := env(change())
			m := loaded(t, &fakeSource{}, e, "")

			_, got := m.Key(press(tt.key), e)

			if got != tt.want {
				t.Errorf("Key(%q) effect = %+v, want %+v", tt.key, got, tt.want)
			}
		})
	}
}

func TestKey_select(t *testing.T) {
	t.Parallel()
	immutable := change()
	immutable.Immutable = true
	reloading := env(change())
	reloading.Reloading = true
	outdated := env(change())
	outdated.Outdated = true
	tests := map[string]struct {
		env        review.Env
		wantStatus string
		wantPicked int
	}{
		"selects the hunk":            {env: env(change()), wantPicked: 1},
		"refuses an immutable change": {env: env(immutable), wantStatus: "immutable revision: read only"},
		"refuses while reloading":     {env: reloading, wantStatus: review.ReloadingStatus},
		"refuses an outdated review":  {env: outdated, wantStatus: review.OutdatedStatus},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			m := loaded(t, &fakeSource{}, tt.env, "")

			_, eff := m.Key(press("space"), tt.env)

			if eff.Status != tt.wantStatus {
				t.Errorf("Key(space) status = %q, want %q", eff.Status, tt.wantStatus)
			}
			if got := m.Picked(); got != tt.wantPicked {
				t.Errorf("Picked() = %d, want %d", got, tt.wantPicked)
			}
		})
	}
}

func TestKey_commitRefused(t *testing.T) {
	t.Parallel()
	blocked := env(change())
	blocked.Blocked = true
	tests := map[string]struct {
		env    review.Env
		pick   bool
		status string
	}{
		"without picks":      {env: env(change()), status: "select hunks first (space / S), or lines (s / v)"},
		"after a split stop": {env: blocked, pick: true, status: review.BlockedStatus},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			m := loaded(t, &fakeSource{}, tt.env, "")
			if tt.pick {
				m.Key(press("space"), tt.env)
			}

			_, eff := m.Key(press("c"), tt.env)

			if eff.Status != tt.status || m.ModalOpen() {
				t.Errorf("Key(c) = status %q, modal %v, want status %q and no modal", eff.Status, m.ModalOpen(), tt.status)
			}
		})
	}
}

func commitPicked(t *testing.T, src *fakeSource, e review.Env) (review.Model, review.Effect) {
	t.Helper()
	m := loaded(t, src, e, "")
	m.Key(press("space"), e)
	m.Key(press("c"), e)
	if !m.ModalOpen() {
		t.Fatal("ModalOpen() = false after c, want the commit dialog")
	}
	m.Sync(e)
	m.Paste(tea.PasteMsg{Content: "add a\n\nbody"})
	cmd, _ := m.Key(press("ctrl+s"), e)
	if cmd == nil {
		t.Fatal("Key(ctrl+s) cmd = nil, want the split command")
	}
	_, eff, ok := m.Update(cmd(), e)
	if !ok {
		t.Fatal("Update(committed) handled = false, want true")
	}
	return m, eff
}

func TestCommit_success(t *testing.T) {
	t.Parallel()
	src := &fakeSource{}

	m, eff := commitPicked(t, src, env(change()))

	if want := "✓ created selected: add a"; eff.Status != want {
		t.Errorf("Update(committed) status = %q, want %q", eff.Status, want)
	}
	if c := eff.Committed; c == nil || c.Incomplete || c.Selected.ChangeID != "selected123" || c.Rest.ChangeID != "rest456" || c.Rest.CommitID != "resthead" {
		t.Errorf("Update(committed) Committed = %+v, want selected123 split from rest456 at resthead", c)
	}
	if !slices.Equal(src.splits, []string{"target@head: add a\n\nbody"}) {
		t.Errorf("splits = %q, want one split of target@head", src.splits)
	}
	if m.ModalOpen() || m.Picked() != 0 {
		t.Errorf("after commit ModalOpen() = %v, Picked() = %d, want closed and nothing picked", m.ModalOpen(), m.Picked())
	}
}

func TestCommit_incomplete(t *testing.T) {
	t.Parallel()
	src := &fakeSource{splitErr: jj.ErrSplitIncomplete}

	_, eff := commitPicked(t, src, env(change()))

	if eff.Status != review.BlockedStatus || eff.Committed == nil || !eff.Committed.Incomplete {
		t.Errorf("Update(committed) effect = %+v, want an incomplete commit with %q", eff, review.BlockedStatus)
	}
}

func TestFullFile_requestsContent(t *testing.T) {
	t.Parallel()
	src := &fakeSource{}
	e := env(change())
	m := loaded(t, src, e, "")

	cmd, _ := m.Key(press("f"), e)
	if cmd == nil {
		t.Fatal("Key(f) cmd = nil, want a content request")
	}
	_, _, ok := m.Update(cmd(), e)

	if !ok || !m.FullFile() || !slices.Equal(src.requested, []string{"head:b/x.go"}) {
		t.Errorf("full file: handled %v, FullFile() %v, requested %q, want true, true, [head:b/x.go]", ok, m.FullFile(), src.requested)
	}
	m.ChangesOnly()
	if m.FullFile() {
		t.Error("FullFile() = true after ChangesOnly, want false")
	}
}

func TestView_success(t *testing.T) {
	t.Parallel()
	e := env(change())
	m := loaded(t, &fakeSource{}, e, "")

	got := ansi.Strip(m.View(e))

	for _, want := range []string{"FILES", "a.go", "UNIFIED DIFF", "@@ -1,1 +1,2 @@"} {
		if !strings.Contains(got, want) {
			t.Errorf("View() missing %q", want)
		}
	}
}
