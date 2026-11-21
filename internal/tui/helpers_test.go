package tui_test

import (
	"context"
	"fmt"
	"image/color"
	"regexp"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/madalinpopa/jjreview/internal/diff"
	"github.com/madalinpopa/jjreview/internal/jj"
	"github.com/madalinpopa/jjreview/internal/llm"
	"github.com/madalinpopa/jjreview/internal/tui"
)

type splitCall struct {
	changeID string
	message  string
	files    []jj.SplitFile
}

type fakeSource struct {
	change          jj.Change
	files           []diff.File
	err             error
	contents        map[string]string
	contentErr      map[string]error
	requested       []string
	splitErr        error
	splits          []splitCall
	head            string
	diffCalls       int
	diffRevs        []string
	revisions       map[string]jj.Change
	revFiles        map[string][]diff.File
	revErrs         map[string]error
	diffErrs        map[string]error
	changeRevs      []string
	log             []jj.LogLine
	logErr          error
	logCalls        int
	bookmarks       []jj.Bookmark
	bmLoadErr       error
	bmCalls         int
	bmErr           error
	backwards       bool
	bmWarnings      string
	renameErr       error
	moves           []string
	beforeMove      func()
	publishErr      error
	abandoned       []string
	abandonErr      error
	confirmed       [][]string
	commitBookmarks map[string][]string
	wcCommit        string
	successors      map[string]jj.Change
	succErrs        map[string]error
	squashes        []squashCall
	squashErr       error
	squashWarnings  string
	pushes          []jj.LogEntry
	pushResult      []jj.PushResult
	pushErr         error
	fetches         int
	fetchChanged    bool
	fetchErr        error
	ops             []jj.Operation
	opsErr          error
	opsCalls        int
	opRuns          []string
	opErr           error
	afterOp         func()
}

type squashCall struct {
	from []string
	into string
}

func (s *fakeSource) Bookmarks(context.Context) ([]jj.Bookmark, error) {
	s.bmCalls++
	return slices.Clone(s.bookmarks), s.bmLoadErr
}

func (s *fakeSource) bookmarkIndex(name string) int {
	return slices.IndexFunc(s.bookmarks, func(b jj.Bookmark) bool { return b.Name == name })
}

func (s *fakeSource) sortBookmarks() {
	slices.SortFunc(s.bookmarks, func(a, b jj.Bookmark) int { return strings.Compare(a.Name, b.Name) })
}

func fakeCommit(rev string) string {
	return strings.TrimSuffix(strings.TrimPrefix(rev, `commit_id("`), `")`)
}

func (s *fakeSource) CreateBookmark(_ context.Context, name, rev string) (string, error) {
	if s.bmErr != nil {
		return "", s.bmErr
	}
	if s.bookmarkIndex(name) >= 0 {
		return "", fmt.Errorf("jj bookmark create: Bookmark already exists: %s", name)
	}
	s.bookmarks = append(s.bookmarks, jj.Bookmark{Name: name, CommitID: fakeCommit(rev)})
	s.sortBookmarks()
	return s.published()
}

func (s *fakeSource) RenameBookmark(_ context.Context, oldName, newName string) (string, error) {
	if s.renameErr != nil {
		return "", s.renameErr
	}
	i := s.bookmarkIndex(oldName)
	if i < 0 || s.bookmarkIndex(newName) >= 0 {
		return "", fmt.Errorf("jj bookmark rename: cannot rename %s to %s", oldName, newName)
	}
	s.bookmarks[i].Name = newName
	s.sortBookmarks()
	return s.bmWarnings, nil
}

func (s *fakeSource) MoveBookmark(_ context.Context, name string, from []string, to string, allowBackwards bool) (string, error) {
	if s.beforeMove != nil {
		s.beforeMove()
		s.beforeMove = nil
	}
	if s.bmErr != nil {
		return "", s.bmErr
	}
	i := s.bookmarkIndex(name)
	if i < 0 {
		return "", fmt.Errorf("move %s: %w", name, jj.ErrBookmarkChanged)
	}
	b := s.bookmarks[i]
	at := slices.Contains(from, b.CommitID)
	if b.Conflict {
		at = slices.ContainsFunc(b.Added, func(id string) bool { return slices.Contains(from, id) })
	}
	switch {
	case !at:
		return "", fmt.Errorf("move %s: %w", name, jj.ErrBookmarkChanged)
	case s.backwards && !allowBackwards:
		return "", fmt.Errorf("move %s: %w", name, jj.ErrBookmarkBackwards)
	}
	s.moves = append(s.moves, to)
	s.bookmarks[i] = jj.Bookmark{Name: name, CommitID: to}
	return s.published()
}

func (s *fakeSource) published() (string, error) {
	if s.publishErr != nil {
		return "", s.publishErr
	}
	return s.bmWarnings, nil
}

func (s *fakeSource) DeleteBookmark(_ context.Context, name string) (string, error) {
	if s.bmErr != nil {
		return "", s.bmErr
	}
	s.bookmarks = slices.DeleteFunc(s.bookmarks, func(b jj.Bookmark) bool { return b.Name == name })
	return s.published()
}

func (s *fakeSource) Abandon(_ context.Context, commitID string, bookmarks []string) (bool, error) {
	s.abandoned = append(s.abandoned, commitID)
	s.confirmed = append(s.confirmed, bookmarks)
	if current, ok := s.commitBookmarks[commitID]; ok && !slices.Equal(current, bookmarks) {
		return false, fmt.Errorf("abandon %s: %w", commitID, jj.ErrBookmarksChanged)
	}
	return commitID == s.wcCommit, s.abandonErr
}

func (s *fakeSource) Squash(_ context.Context, from []jj.LogEntry, into jj.LogEntry) (string, error) {
	call := squashCall{into: into.CommitID}
	for _, e := range from {
		call.from = append(call.from, e.CommitID)
	}
	s.squashes = append(s.squashes, call)
	if s.squashErr != nil {
		return "", s.squashErr
	}
	return s.squashWarnings, nil
}

func (s *fakeSource) Push(_ context.Context, e jj.LogEntry) ([]jj.PushResult, error) {
	s.pushes = append(s.pushes, e)
	if s.pushErr != nil {
		return nil, s.pushErr
	}
	return s.pushResult, nil
}

func (s *fakeSource) Fetch(context.Context) (bool, error) {
	s.fetches++
	return s.fetchChanged, s.fetchErr
}

func (s *fakeSource) Successor(_ context.Context, changeID, commitID string) (jj.Change, error) {
	if err, ok := s.succErrs[commitID]; ok {
		return jj.Change{}, err
	}
	if change, ok := s.successors[commitID]; ok {
		return change, nil
	}
	return jj.Change{}, fmt.Errorf("successor of %s in %s: %w", commitID, changeID, jj.ErrNoRevision)
}

func (s *fakeSource) Operations(context.Context) ([]jj.Operation, error) {
	s.opsCalls++
	return slices.Clone(s.ops), s.opsErr
}

func (s *fakeSource) runOp(kind, id string) error {
	s.opRuns = append(s.opRuns, kind+" "+id)
	if s.opErr != nil {
		return s.opErr
	}
	if s.afterOp != nil {
		s.afterOp()
	}
	return nil
}

func (s *fakeSource) RestoreOperation(_ context.Context, id string) error {
	return s.runOp("restore", id)
}

func (s *fakeSource) RevertOperation(_ context.Context, id string) error {
	return s.runOp("revert", id)
}

func (s *fakeSource) AbandonOperation(_ context.Context, id string) error {
	return s.runOp("abandon", id)
}

func (s *fakeSource) Log(context.Context) ([]jj.LogLine, error) {
	s.logCalls++
	return s.log, s.logErr
}

func (s *fakeSource) WorkspaceRoot(context.Context) (string, error) {
	return "/repo", nil
}

func (s *fakeSource) Change(_ context.Context, rev string) (jj.Change, error) {
	s.changeRevs = append(s.changeRevs, rev)
	if err, ok := s.revErrs[rev]; ok {
		return jj.Change{}, err
	}
	if change, ok := s.revisions[rev]; ok {
		return change, nil
	}
	if inner, ok := strings.CutSuffix(rev, ") & visible()"); ok {
		rev = strings.TrimPrefix(inner, "(")
		if err, ok := s.revErrs[rev]; ok {
			return jj.Change{}, err
		}
	}
	if change, ok := s.revisions[rev]; ok {
		return change, nil
	}
	return s.change, s.err
}

func (s *fakeSource) Diff(_ context.Context, rev string) ([]diff.File, error) {
	s.diffCalls++
	s.diffRevs = append(s.diffRevs, rev)
	if err, ok := s.diffErrs[rev]; ok {
		return nil, err
	}
	if files, ok := s.revFiles[rev]; ok {
		return files, nil
	}
	return s.files, s.err
}

func (s *fakeSource) FileContent(_ context.Context, rev, path string) (string, error) {
	key := rev + ":" + path
	s.requested = append(s.requested, key)
	if err := s.contentErr[key]; err != nil {
		return "", err
	}
	return s.contents[key], nil
}

func (s *fakeSource) Split(_ context.Context, changeID, commitID, message string, files []jj.SplitFile) (jj.Change, jj.Change, error) {
	if s.splitErr != nil {
		return jj.Change{ChangeID: changeID}, jj.Change{}, s.splitErr
	}
	if commitID != s.head {
		return jj.Change{}, jj.Change{}, fmt.Errorf("split %q: %w", changeID, jj.ErrStaleRevision)
	}
	s.splits = append(s.splits, splitCall{changeID: changeID, message: message, files: files})
	n := string(rune('0' + len(s.splits)))
	rest := jj.Change{ChangeID: "rest" + n + "zzzzzzzzzz", CommitID: "restcommit" + n}
	s.head = rest.CommitID
	return jj.Change{ChangeID: changeID}, rest, nil
}

type fakeGenerator struct {
	text    string
	err     error
	patches []string
	ctxs    []context.Context
}

func (g *fakeGenerator) Generate(ctx context.Context, patch string) (string, error) {
	g.patches = append(g.patches, patch)
	g.ctxs = append(g.ctxs, ctx)
	return g.text, g.err
}

const (
	mainParent   = "l1\nl2\nkeep\nold line\nl5\nl6\nl7\nl8\nl9\nx\nl11\nl12\n"
	readmeParent = "# jjreview\n\nTUI for git\n"
)

func mainFile() diff.File {
	return diff.File{OldPath: "src/main.go", NewPath: "src/main.go", OldMode: "100644", NewMode: "100644", Hunks: []diff.Hunk{
		{OldStart: 3, OldLines: 2, NewStart: 3, NewLines: 2, Lines: []diff.Line{
			{Kind: diff.LineContext, Text: "keep", OldNo: 3, NewNo: 3},
			{Kind: diff.LineRemoved, Text: "old line", OldNo: 4},
			{Kind: diff.LineAdded, Text: "new line", NewNo: 4},
		}},
		{OldStart: 10, OldLines: 1, NewStart: 10, NewLines: 2, Lines: []diff.Line{
			{Kind: diff.LineContext, Text: "x", OldNo: 10, NewNo: 10},
			{Kind: diff.LineAdded, Text: "y", NewNo: 11},
		}},
	}}
}

func readmeFile() diff.File {
	return diff.File{OldPath: "README.md", NewPath: "README.md", OldMode: "100644", NewMode: "100644", Hunks: []diff.Hunk{
		{OldStart: 3, OldLines: 1, NewStart: 3, NewLines: 1, Lines: []diff.Line{
			{Kind: diff.LineRemoved, Text: "TUI for git", OldNo: 3},
			{Kind: diff.LineAdded, Text: "TUI for jujutsu", NewNo: 3},
		}},
	}}
}

func viewFile() diff.File {
	return diff.File{NewPath: "src/ui/view.go", NewMode: "100644", Status: diff.StatusAdded, Hunks: []diff.Hunk{
		{OldStart: 0, OldLines: 0, NewStart: 1, NewLines: 2, Lines: []diff.Line{
			{Kind: diff.LineAdded, Text: "package ui", NewNo: 1},
			{Kind: diff.LineAdded, Text: "func View() {}", NewNo: 2},
		}},
	}}
}

func imageFile() diff.File {
	return diff.File{OldPath: "img.png", NewPath: "img.png", OldMode: "100644", NewMode: "100644", Binary: true}
}

func newSource() *fakeSource {
	return &fakeSource{
		change: jj.Change{ChangeID: "kxyzabcdefgh", CommitID: "commit1", Parents: []string{"parent1"}, Description: "wip", WorkingCopy: true},
		files:  []diff.File{readmeFile(), mainFile(), viewFile(), imageFile()},
		contents: map[string]string{
			"parent1:src/main.go": mainParent,
			"parent1:README.md":   readmeParent,
			"commit1:img.png":     "PNG",
		},
		contentErr: map[string]error{},
		head:       "commit1",
		wcCommit:   "commit1",
	}
}

func update(tb testing.TB, m tui.Model, msg tea.Msg) (tui.Model, tea.Cmd) {
	tb.Helper()
	next, cmd := m.Update(msg)
	got, ok := next.(tui.Model)
	if !ok {
		tb.Fatalf("Update() model type = %T, want tui.Model", next)
	}
	return got, cmd
}

func run(tb testing.TB, m tui.Model, cmd tea.Cmd) tui.Model {
	tb.Helper()
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
		case tea.QuitMsg:
		default:
			var follow tea.Cmd
			m, follow = update(tb, m, msg)
			queue = append(queue, follow)
		}
	}
	return m
}

func start(tb testing.TB, src tui.Source, gen llm.Provider, width, height int) tui.Model {
	tb.Helper()
	m := tui.New(tb.Context(), src, gen, "@")
	m, _ = update(tb, m, tea.WindowSizeMsg{Width: width, Height: height})
	return run(tb, m, m.Init())
}

func loaded(t *testing.T, src tui.Source) tui.Model {
	t.Helper()
	return start(t, src, &fakeGenerator{text: "feat: generated"}, 120, 30)
}

func answered(tb testing.TB, m tui.Model) tui.Model {
	tb.Helper()
	m, _ = update(tb, m, tea.BackgroundColorMsg{Color: color.Black})
	return m
}

func keyMsg(k string) tea.KeyPressMsg {
	switch k {
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "left":
		return tea.KeyPressMsg{Code: tea.KeyLeft}
	case "right":
		return tea.KeyPressMsg{Code: tea.KeyRight}
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "pgdown":
		return tea.KeyPressMsg{Code: tea.KeyPgDown}
	case "pgup":
		return tea.KeyPressMsg{Code: tea.KeyPgUp}
	case "home":
		return tea.KeyPressMsg{Code: tea.KeyHome}
	case "end":
		return tea.KeyPressMsg{Code: tea.KeyEnd}
	case "space":
		return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "backspace":
		return tea.KeyPressMsg{Code: tea.KeyBackspace}
	case "ctrl+c", "ctrl+r", "ctrl+s", "ctrl+d", "ctrl+u", "ctrl+h", "ctrl+l":
		return tea.KeyPressMsg{Code: rune(k[len(k)-1]), Mod: tea.ModCtrl}
	case "ctrl+enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModCtrl}
	}
	return tea.KeyPressMsg{Code: rune(k[0]), Text: k}
}

func press(t *testing.T, m tui.Model, keys ...string) tui.Model {
	t.Helper()
	for _, k := range keys {
		var cmd tea.Cmd
		m, cmd = update(t, m, keyMsg(k))
		m = run(t, m, cmd)
	}
	return m
}

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;:?]*[A-Za-z]`)

func view(m tui.Model) string {
	return ansiPattern.ReplaceAllString(m.View().Content, "")
}
