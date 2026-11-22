package tui

import (
	"context"
	"os/exec"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/madalinpopa/jjreview/internal/jj"
	"github.com/madalinpopa/jjreview/internal/llm"
	"github.com/madalinpopa/jjreview/internal/theme"
	"github.com/madalinpopa/jjreview/internal/tui/bookmarks"
	"github.com/madalinpopa/jjreview/internal/tui/logview"
	"github.com/madalinpopa/jjreview/internal/tui/operations"
	"github.com/madalinpopa/jjreview/internal/tui/review"
)

const (
	defaultWidth  = 100
	defaultHeight = 30
)

type view int

const (
	reviewView view = iota
	logView
	bookmarksView
	operationsView
)

type Model struct {
	ctx        context.Context
	src        Source
	rev        string
	themes     theme.Set
	pal        theme.Theme
	dark       bool
	bgPending  bool
	bgDeferred bool
	edit       editState
	held       tea.Cmd
	holdSeq    int
	cancelEdit context.CancelFunc
	quitting   bool
	keys       globalKeyMap
	loaded     bool
	reloading  bool
	outdated   bool
	err        error
	change     jj.Change
	target     string
	targetHead string
	status     string
	armed      string
	width      int
	height     int
	help       bool
	helpTop    int
	blocked    bool
	editor     func(ctx context.Context, path string) *exec.Cmd
	root       string
	reloadPath string
	view       view
	log        logview.Model
	bookmarks  bookmarks.Model
	operations operations.Model
	review     review.Model
}

func New(ctx context.Context, src Source, gen llm.Provider, rev string) Model {
	m := Model{
		ctx:        ctx,
		src:        src,
		rev:        rev,
		themes:     theme.DefaultSet(),
		pal:        theme.Default(),
		dark:       true,
		keys:       newGlobalKeyMap(),
		review:     review.New(ctx, src, gen),
		log:        logview.New(ctx, src),
		bookmarks:  bookmarks.New(ctx, src),
		operations: operations.New(ctx, src),
		width:      defaultWidth,
		height:     defaultHeight,
	}
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
	case editCheckedMsg:
		cmd = m.editChecked(msg)
	case EditorFinishedMsg:
		cmd = m.editorFinished(msg)
	case tea.KeyPressMsg:
		cmd = m.keyPress(msg)
	case tea.PasteMsg:
		switch {
		case m.review.ModalOpen(), m.view == reviewView && m.review.Searching():
			cmd = m.review.Paste(msg)
		case m.bookmarks.OverlayOpen():
			cmd = m.bookmarks.Paste(msg)
		default:
			if t, ok := m.active(); ok {
				cmd = t.tab.Paste(msg)
			}
		}
	default:
		if c, eff, ok := m.review.Update(msg, m.reviewEnv()); ok {
			cmd = m.applyReview(c, eff)
		} else {
			cmd = m.updateTabs(msg)
		}
	}
	env := m.reviewEnv()
	m.review.Sync(env)
	m.bookmarks.Resize(m.env(bookmarksView))
	m.scroll(env)
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
	case m.review.ModalOpen():
		cmd, eff := m.review.Key(msg, m.reviewEnv())
		return m.applyReview(cmd, eff)
	case m.bookmarks.OverlayOpen():
		cmd, eff := m.bookmarks.Key(msg, m.env(bookmarksView))
		return m.applyEffect(bookmarksView, cmd, eff)
	case m.help:
		m.helpKey(msg)
		return nil
	}
	if t, ok := m.active(); ok {
		return m.tabKey(t, msg)
	}
	return m.reviewKey(msg)
}

func (m Model) CurrentFile() string {
	file, ok := m.review.CurrentFile()
	if !ok {
		return ""
	}
	return file.Path()
}

func (m Model) Cursor() int {
	return m.review.Cursor()
}

func (m Model) FilesFocused() bool {
	return m.review.FilesFocused()
}

func (m Model) FullFile() bool {
	return m.review.FullFile()
}

func (m Model) ModalOpen() bool {
	return m.review.ModalOpen()
}

func (m Model) Message() string {
	return m.review.Message()
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
	m.review.Repaint(old, m.pal)
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
	cmd := m.review.Load(msg.files, m.reloadPath, m.reviewEnv())
	m.reloadPath = ""
	return cmd
}

func (m Model) reviewEnv() review.Env {
	return review.Env{
		Env:       m.env(reviewView),
		Change:    m.change,
		Loaded:    m.loaded,
		Err:       m.err,
		Reloading: m.reloading,
		Outdated:  m.outdated,
		Blocked:   m.blocked,
	}
}

func (m *Model) applyReview(cmd tea.Cmd, eff review.Effect) tea.Cmd {
	if c := eff.Committed; c != nil {
		m.markStale()
		if c.Incomplete {
			m.blocked = true
		} else {
			m.target = c.Rest.ChangeID
			m.targetHead = c.Rest.CommitID
		}
	}
	if eff.Status != "" {
		m.status = eff.Status
	}
	return cmd
}

func (m *Model) scroll(env review.Env) {
	if t, ok := m.active(); ok {
		t.tab.KeepVisible(m.env(t.view))
		return
	}
	m.review.KeepVisible(env)
}
