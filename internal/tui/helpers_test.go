package tui_test

import (
	"context"
	"fmt"
	"image/color"
	"regexp"
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
	change     jj.Change
	files      []diff.File
	err        error
	contents   map[string]string
	contentErr map[string]error
	requested  []string
	splitErr   error
	splits     []splitCall
	head       string
	diffCalls  int
	diffRevs   []string
	revisions  map[string]jj.Change
	revFiles   map[string][]diff.File
	revErrs    map[string]error
	changeRevs []string
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
	return s.change, s.err
}

func (s *fakeSource) Diff(_ context.Context, rev string) ([]diff.File, error) {
	s.diffCalls++
	s.diffRevs = append(s.diffRevs, rev)
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
	case "ctrl+c", "ctrl+r", "ctrl+s", "ctrl+d", "ctrl+u", "ctrl+n", "ctrl+p", "ctrl+h", "ctrl+l":
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
