package tui

import (
	"context"
	"maps"
	"os/exec"
	"slices"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/madalinpopa/jjreview/internal/diff"
	"github.com/madalinpopa/jjreview/internal/jj"
	"github.com/madalinpopa/jjreview/internal/llm"
	"github.com/madalinpopa/jjreview/internal/theme"
	"github.com/madalinpopa/jjreview/internal/tui/bookmarks"
	"github.com/madalinpopa/jjreview/internal/tui/logview"
	"github.com/madalinpopa/jjreview/internal/tui/operations"
	"github.com/madalinpopa/jjreview/internal/tui/ui"
)

const (
	defaultWidth  = 100
	defaultHeight = 30
)

type contentKey struct {
	rev  string
	path string
}

type view int

const (
	reviewView view = iota
	logView
	bookmarksView
	operationsView
)

type Model struct {
	ctx         context.Context
	src         Source
	gen         llm.Provider
	rev         string
	themes      theme.Set
	pal         theme.Theme
	dark        bool
	bgPending   bool
	bgDeferred  bool
	edit        editState
	held        tea.Cmd
	holdSeq     int
	cancelEdit  context.CancelFunc
	quitting    bool
	keys        globalKeyMap
	reviewKeys  reviewKeyMap
	loaded      bool
	reloading   bool
	outdated    bool
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
	treeWidth   int
	full        bool
	filesFocus  bool
	status      string
	armed       string
	width       int
	height      int
	modal       *commitModal
	genSeq      int
	help        bool
	helpTop     int
	blocked     bool
	editor      func(ctx context.Context, path string) *exec.Cmd
	root        string
	reloadPath  string
	view        view
	log         logview.Model
	bookmarks   bookmarks.Model
	operations  operations.Model
}

func New(ctx context.Context, src Source, gen llm.Provider, rev string) Model {
	m := Model{
		ctx:         ctx,
		src:         src,
		gen:         gen,
		rev:         rev,
		themes:      theme.DefaultSet(),
		pal:         theme.Default(),
		dark:        true,
		keys:        newGlobalKeyMap(),
		reviewKeys:  newReviewKeyMap(),
		contents:    map[contentKey]string{},
		contentErrs: map[contentKey]error{},
		highlights:  map[highlightKey]fileHighlight{},
		log:         logview.New(ctx, src),
		bookmarks:   bookmarks.New(ctx, src),
		operations:  operations.New(ctx, src),
		width:       defaultWidth,
		height:      defaultHeight,
	}
	m.treeWidth = m.measureTree()
	m.log.Resize(m.width)
	return m
}

func (m Model) WithTheme(s theme.Set) Model {
	m.themes = s
	m.dark = true
	m.pal = s.Resolve(true)
	return m
}

func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{load(m.ctx, m.src, m.rev), func() tea.Msg { return queryBackgroundMsg{} }}
	if m.autoTheme() {
		cmds = append(cmds, tea.Raw(ansi.SetModeLightDark))
	}
	return tea.Batch(cmds...)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.log.Resize(m.width)
	case queryBackgroundMsg:
		cmd = m.requestBackground()
	case tea.BackgroundColorMsg:
		cmd = m.backgroundAnswered()
		if msg.Color == nil {
			break
		}
		m.setProfile(msg.IsDark())
		m.pal = m.themes.Resolve(m.dark).WithBackground(msg.Color, msg.IsDark())
	case holdTimeoutMsg:
		cmd = m.holdTimedOut(msg)
	case uv.DarkColorSchemeEvent:
		cmd = m.schemeChanged(true)
	case uv.LightColorSchemeEvent:
		cmd = m.schemeChanged(false)
	case tea.FocusMsg:
		if m.autoTheme() {
			cmd = m.requestBackground()
		}
	case loadedMsg:
		if m.edit == editReloading {
			m.edit, m.status = editIdle, ""
		}
		cmd = m.applyLoad(msg)
	case revisionMsg:
		cmd = m.applyRevision(msg)
	case workingCopyMsg:
		m.applyWorkingCopy(msg)
	case rewrittenMsg:
		cmd = m.applyRewritten(msg)
	case contentMsg:
		m.applyContent(msg)
	case generatedMsg:
		m.applyGenerated(msg)
	case committedMsg:
		m.applyCommitted(msg)
	case editCheckedMsg:
		cmd = m.editChecked(msg)
	case EditorFinishedMsg:
		cmd = m.editorFinished(msg)
	case tea.KeyPressMsg:
		cmd = m.keyPress(msg)
	case tea.PasteMsg:
		switch {
		case m.modal != nil:
			cmd = m.modalInput(msg)
		case m.bookmarks.OverlayOpen():
			cmd = m.bookmarks.Paste(msg)
		case m.view == logView:
			cmd = m.log.Input(msg)
		}
	default:
		if c, in, ok := m.bookmarks.Update(msg, m.bookmarkEnv()); ok {
			cmd = m.bookmarkEffects(c, in)
		} else if c, in, ok := m.operations.Update(msg, m.operationEnv()); ok {
			cmd = m.operationEffects(c, in)
		} else if c, in, ok := m.log.Update(msg, m.logEnv()); ok {
			cmd = m.logEffects(c, in)
		} else if m.view == logView {
			cmd = m.log.Input(msg)
		}
	}
	m.resizeModal()
	m.bookmarks.Resize(m.bookmarkEnv())
	m.ensureHighlight()
	m.scroll()
	return m, cmd
}

func (m *Model) keyPress(msg tea.KeyPressMsg) tea.Cmd {
	switch {
	case m.quitting:
		return nil
	case msg.String() == "ctrl+c":
		return m.quit()
	case m.edit == editRunning, m.edit == editReloading && !key.Matches(msg, m.keys.quit):
		return nil
	case m.modal != nil:
		return m.modalKey(msg)
	case m.bookmarks.OverlayOpen():
		return m.bookmarkEffects(m.bookmarks.Key(msg, m.bookmarkEnv()))
	case m.help:
		m.helpKey(msg)
		return nil
	case m.view == logView:
		return m.logKey(msg)
	case m.view == bookmarksView:
		return m.bookmarksKey(msg)
	case m.view == operationsView:
		return m.operationsKey(msg)
	}
	return m.reviewKey(msg)
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

func (m Model) autoTheme() bool {
	return m.themes.Mode == theme.ModeAuto
}

func (m *Model) schemeChanged(dark bool) tea.Cmd {
	if !m.autoTheme() {
		return nil
	}
	m.setProfile(dark)
	return m.requestBackground()
}

func (m *Model) setProfile(dark bool) {
	if !m.autoTheme() || dark == m.dark {
		return
	}
	old := m.pal
	m.dark = dark
	m.pal = m.themes.Resolve(dark)
	if !sameColors(highlightColors(old), highlightColors(m.pal)) {
		clear(m.highlights)
	}
	if m.modal != nil {
		m.modal.input.SetStyles(inputStyles(m.pal))
	}
}

func (m *Model) applyLoad(msg loadedMsg) tea.Cmd {
	m.loaded = true
	m.reloading = false
	if m.target != "" && msg.moved {
		m.reloadPath = ""
		m.err = nil
		m.change.WorkingCopy = false
		m.status = movedStatus
		return nil
	}
	m.err = msg.err
	if msg.err != nil {
		return nil
	}
	m.outdated = false
	m.change = msg.change
	m.target = msg.change.ChangeID
	m.targetHead = msg.change.CommitID
	m.log.Follow(m.targetHead)
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
	m.treeWidth = m.measureTree()
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
		if file, ok := m.currentFile(); ok && m.contentKey(file) == msg.key {
			m.cursor = 0
		}
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
	switch m.view {
	case logView:
		m.log.KeepVisible(m.logEnv())
		return
	case bookmarksView:
		m.bookmarks.KeepVisible(m.bookmarkEnv())
		return
	case operationsView:
		m.operations.KeepVisible(m.operationEnv())
		return
	}
	m.diffTop = ui.ScrollTop(m.diffTop, m.cursor, m.diffHeight(), len(m.rows()))
	m.listTop = ui.ScrollTop(m.listTop, m.currentTreeRow(), m.diffHeight(), len(m.treeRows()))
}
