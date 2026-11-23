package review

import (
	"context"
	"fmt"
	"maps"
	"slices"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/madalinpopa/jjreview/internal/diff"
	"github.com/madalinpopa/jjreview/internal/jj"
	"github.com/madalinpopa/jjreview/internal/llm"
	"github.com/madalinpopa/jjreview/internal/theme"
	"github.com/madalinpopa/jjreview/internal/tui/tab"
	"github.com/madalinpopa/jjreview/internal/tui/ui"
)

type Source interface {
	FileContent(ctx context.Context, rev, path string) (string, error)
	Split(ctx context.Context, changeID, commitID, message string, files []jj.SplitFile) (selected, rest jj.Change, err error)
}

type Env struct {
	tab.Env
	Change    jj.Change
	Loaded    bool
	Err       error
	Reloading bool
	Outdated  bool
	Blocked   bool
}

type Effect struct {
	Status      string
	WorkingCopy bool
	Reload      bool
	Edit        bool
	Committed   *Committed
}

type Committed struct {
	Selected   jj.Change
	Rest       jj.Change
	Incomplete bool
}

type Held struct {
	files   []diff.File
	reviews []fileReview
}

type contentKey struct {
	rev  string
	path string
}

type contentMsg struct {
	key     contentKey
	content string
	err     error
}

type Model struct {
	ctx         context.Context
	src         Source
	gen         llm.Provider
	keys        keyMap
	files       []diff.File
	reviews     []fileReview
	contents    map[contentKey]string
	contentErrs map[contentKey]error
	highlights  map[highlightKey]fileHighlight
	fileIdx     int
	cursor      int
	ranging     bool
	anchor      int
	diffTop     int
	diffLeft    int
	overflow    int
	listTop     int
	treeWidth   int
	full        bool
	zoomed      bool
	filesFocus  bool
	modal       *commitModal
	genSeq      int
	queryKeys   queryKeyMap
	query       textinput.Model
	searching   bool
	needle      string
	hit         *hit
}

func New(ctx context.Context, src Source, gen llm.Provider) Model {
	m := Model{
		ctx:         ctx,
		src:         src,
		gen:         gen,
		keys:        newKeyMap(),
		queryKeys:   newQueryKeyMap(),
		query:       ui.NewSearchInput(),
		contents:    map[contentKey]string{},
		contentErrs: map[contentKey]error{},
		highlights:  map[highlightKey]fileHighlight{},
	}
	m.treeWidth = m.measureTree(theme.Default())
	return m
}

func (m *Model) Load(files []diff.File, focus string, env Env) tea.Cmd {
	prevFiles, prevReviews := m.files, m.reviews
	m.ranging = false
	m.diffLeft = 0
	m.hit = nil
	m.files = slices.Clone(files)
	slices.SortStableFunc(m.files, compareTree)
	m.reviews = make([]fileReview, len(m.files))
	for i, f := range m.files {
		m.reviews[i] = newReview(f)
		if j := slices.IndexFunc(prevFiles, func(p diff.File) bool { return p.Path() == f.Path() }); j >= 0 {
			m.reviews[i].keepView(f, prevFiles[j], prevReviews[j])
		}
	}
	m.treeWidth = m.measureTree(env.Theme)
	m.fileIdx = max(slices.IndexFunc(m.files, func(f diff.File) bool { return f.Path() == focus }), 0)
	clear(m.contents)
	clear(m.contentErrs)
	clear(m.highlights)
	m.cursor = firstRow(m.rows(env))
	return m.requestContent(env)
}

func (m *Model) Reset() {
	m.files, m.reviews = nil, nil
}

func (m *Model) ChangesOnly() {
	m.full = false
	m.zoomed = false
}

func (m Model) Hold() Held {
	return Held{files: m.files, reviews: m.reviews}
}

func (m *Model) KeepPicks(h Held) {
	if sameFiles(h.files, m.files) {
		m.reviews = h.reviews
	}
}

func (m *Model) Repaint(old, pal theme.Theme) {
	if !sameColors(highlightColors(old), highlightColors(pal)) {
		clear(m.highlights)
	}
	if m.modal != nil {
		m.modal.input.SetStyles(inputStyles(pal))
	}
}

func (m *Model) Update(msg tea.Msg, env Env) (tea.Cmd, Effect, bool) {
	switch msg := msg.(type) {
	case contentMsg:
		m.applyContent(env, msg)
	case generatedMsg:
		m.applyGenerated(msg)
	case committedMsg:
		return nil, m.applyCommitted(env, msg), true
	default:
		return nil, Effect{}, false
	}
	return nil, Effect{}, true
}

func (m *Model) Sync(env Env) {
	m.resizeModal(env)
	m.resizeQuery(env)
	m.ensureHighlight(env)
}

func (m *Model) KeepVisible(env Env) {
	m.diffTop = ui.ScrollTop(m.diffTop, m.cursor, m.diffHeight(env), len(m.rows(env)))
	m.listTop = ui.ScrollTop(m.listTop, m.currentTreeRow(), env.ListHeight, len(m.treeRows()))
	m.overflow = m.maxLeft(env)
}

func (m Model) CurrentFile() (diff.File, bool) {
	return m.currentFile()
}

func (m Model) Cursor() int {
	return m.cursor
}

func (m Model) FilesFocused() bool {
	return m.filesFocus
}

func (m Model) FullFile() bool {
	return m.full
}

func (m Model) ModalOpen() bool {
	return m.modal != nil
}

func (m Model) Message() string {
	if m.modal == nil {
		return ""
	}
	return m.modal.input.Value()
}

func (m Model) Picked() int {
	return pickedUnits(m.picks())
}

func (m Model) LineNote() string {
	return m.lineNote()
}

func (m *Model) CancelRange() {
	m.ranging = false
}

func (m *Model) applyContent(env Env, msg contentMsg) {
	if msg.err != nil {
		m.contentErrs[msg.key] = msg.err
		if file, ok := m.currentFile(); ok && contentKeyOf(env, file) == msg.key {
			m.cursor = 0
		}
		return
	}
	delete(m.contentErrs, msg.key)
	m.contents[msg.key] = msg.content
	maps.DeleteFunc(m.highlights, func(k highlightKey, _ fileHighlight) bool { return k.full })
	if file, ok := m.currentFile(); ok && m.full && contentKeyOf(env, file) == msg.key {
		m.cursor = firstRow(m.rows(env))
	}
}

func (m Model) currentFile() (diff.File, bool) {
	if m.fileIdx < 0 || m.fileIdx >= len(m.files) {
		return diff.File{}, false
	}
	return m.files[m.fileIdx], true
}

func (m Model) currentReview() fileReview {
	return m.reviews[m.fileIdx]
}

func contentKeyOf(env Env, f diff.File) contentKey {
	return contentKey{rev: env.Change.CommitID, path: f.Path()}
}

func (m Model) requestContent(env Env) tea.Cmd {
	key, ok := m.uncached(env)
	if !ok {
		return nil
	}
	return loadContent(m.ctx, m.src, key)
}

func (m Model) uncached(env Env) (contentKey, bool) {
	file, ok := m.currentFile()
	if !m.full || !ok || file.Status == diff.StatusDeleted || wholeOnly(file) {
		return contentKey{}, false
	}
	key := contentKeyOf(env, file)
	_, cached := m.contents[key]
	return key, !cached
}

func loadContent(ctx context.Context, src Source, key contentKey) tea.Cmd {
	return func() tea.Msg {
		content, err := src.FileContent(ctx, key.rev, key.path)
		if err != nil {
			err = fmt.Errorf("read %s: %w", key.path, err)
		}
		return contentMsg{key: key, content: content, err: err}
	}
}
