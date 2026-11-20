package tui

import (
	"context"
	"maps"
	"os/exec"
	"slices"

	tea "charm.land/bubbletea/v2"
	"github.com/madalinpopa/jjreview/internal/diff"
	"github.com/madalinpopa/jjreview/internal/jj"
	"github.com/madalinpopa/jjreview/internal/llm"
	"github.com/madalinpopa/jjreview/internal/theme"
)

const (
	defaultWidth  = 100
	defaultHeight = 30
	shortIDLen    = 8
)

type contentKey struct {
	rev  string
	path string
}

type Model struct {
	ctx         context.Context
	src         Source
	gen         llm.Provider
	rev         string
	pal         theme.Theme
	loaded      bool
	reloading   bool
	err         error
	change      jj.Change
	target      string
	targetHead  string
	files       []diff.File
	reviews     []fileReview
	contents    map[contentKey]string
	contentErrs map[contentKey]error
	highlights  map[highlightKey]fileHighlight
	fileIdx     int
	cursor      int
	diffTop     int
	listTop     int
	full        bool
	filesFocus  bool
	status      string
	width       int
	height      int
	modal       *commitModal
	genSeq      int
	help        bool
	blocked     bool
	editor      func(ctx context.Context, path string) *exec.Cmd
	root        string
	reloadPath  string
}

func New(ctx context.Context, src Source, gen llm.Provider, rev string) Model {
	return Model{
		ctx:         ctx,
		src:         src,
		gen:         gen,
		rev:         rev,
		pal:         theme.Default(),
		contents:    map[contentKey]string{},
		contentErrs: map[contentKey]error{},
		highlights:  map[highlightKey]fileHighlight{},
		width:       defaultWidth,
		height:      defaultHeight,
	}
}

func (m Model) WithTheme(t theme.Theme) Model {
	m.pal = t
	return m
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(load(m.ctx, m.src, m.rev), tea.RequestBackgroundColor)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.resizeModal()
	case tea.BackgroundColorMsg:
		m.pal = m.pal.WithBackground(msg.Color, msg.IsDark())
	case loadedMsg:
		cmd = m.applyLoad(msg)
	case contentMsg:
		m.applyContent(msg)
	case generatedMsg:
		m.applyGenerated(msg)
	case committedMsg:
		m.applyCommitted(msg)
	case EditorFinishedMsg:
		cmd = m.editorFinished(msg)
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		switch {
		case m.modal != nil:
			cmd = m.modalKey(msg)
		case m.help:
			m.helpKey(msg)
		default:
			cmd = m.reviewKey(msg)
		}
	}
	m.ensureHighlight()
	m.scroll()
	return m, cmd
}

func (m Model) CurrentFile() string {
	file, ok := m.currentFile()
	if !ok {
		return ""
	}
	return file.Path()
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

func (m *Model) applyLoad(msg loadedMsg) tea.Cmd {
	m.loaded = true
	m.reloading = false
	m.err = msg.err
	if msg.err != nil {
		return nil
	}
	m.change = msg.change
	m.target = msg.change.ChangeID
	m.targetHead = msg.change.CommitID
	m.root = msg.root
	prevFiles, prevReviews := m.files, m.reviews
	m.files = slices.Clone(msg.files)
	slices.SortStableFunc(m.files, compareTree)
	m.reviews = make([]fileReview, len(m.files))
	for i, f := range m.files {
		m.reviews[i] = newReview(f)
		if j := slices.IndexFunc(prevFiles, func(p diff.File) bool { return p.Path() == f.Path() }); j >= 0 {
			m.reviews[i].keepView(f, prevFiles[j], prevReviews[j])
		}
	}
	m.fileIdx = max(slices.IndexFunc(m.files, func(f diff.File) bool { return f.Path() == m.reloadPath }), 0)
	m.reloadPath = ""
	clear(m.contents)
	clear(m.contentErrs)
	clear(m.highlights)
	m.cursor = firstRow(m.rows())
	return m.requestContent()
}

func (m *Model) applyContent(msg contentMsg) {
	if msg.err != nil {
		m.contentErrs[msg.key] = msg.err
		m.cursor = 0
		return
	}
	delete(m.contentErrs, msg.key)
	m.contents[msg.key] = msg.content
	maps.DeleteFunc(m.highlights, func(k highlightKey, _ fileHighlight) bool { return k.full })
	if file, ok := m.currentFile(); ok && m.full && m.contentKey(file) == msg.key {
		m.cursor = firstRow(m.rows())
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

func (m Model) contentKey(f diff.File) contentKey {
	return contentKey{rev: m.change.CommitID, path: f.Path()}
}

func (m Model) requestContent() tea.Cmd {
	file, ok := m.currentFile()
	if !m.full || !ok || file.Status == diff.StatusDeleted || wholeOnly(file) {
		return nil
	}
	key := m.contentKey(file)
	if _, cached := m.contents[key]; cached {
		return nil
	}
	return loadContent(m.ctx, m.src, key)
}

func (m *Model) scroll() {
	m.diffTop = scrollTop(m.diffTop, m.cursor, m.diffHeight(), len(m.rows()))
	m.listTop = scrollTop(m.listTop, m.currentTreeRow(), m.diffHeight(), len(m.treeRows()))
}

func shortID(id string) string {
	if len(id) > shortIDLen {
		return id[:shortIDLen]
	}
	return id
}
