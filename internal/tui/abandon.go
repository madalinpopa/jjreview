package tui

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/madalinpopa/jjreview/internal/jj"
)

const outdatedStatus = "review out of date; press @ to review the working copy"

type abandonedMsg struct {
	changeID string
	keep     logPosition
	step     revisionStep
	loaded   loadedMsg
	lost     string
	err      error
}

type logPosition struct {
	at     int
	cursor int
	query  string
}

func (m Model) abandonBusy() bool {
	return m.reloading || m.log.loading
}

func (m *Model) abandonPicked() tea.Cmd {
	e := m.log.selected()
	if e == nil || !m.canLoad() {
		return nil
	}
	if e.Immutable {
		m.status = shortID(e.ChangeID) + " is immutable"
		return nil
	}
	m.armAbandon(*e)
	return nil
}

func (m *Model) armAbandon(e jj.LogEntry) {
	e.Bookmarks = slices.Clone(e.Bookmarks)
	m.log.confirm = &e
	m.status = ""
}

func (m *Model) confirmAbandon(target jj.LogEntry) tea.Cmd {
	e := m.log.selected()
	if e == nil || e.CommitID != target.CommitID || !m.canLoad() {
		return nil
	}
	return m.abandon(target)
}

func (m Model) abandonView() string {
	p := m.pal
	e := m.log.confirm
	inner := m.modalInner()
	wrap := func(style lipgloss.Style, text string) []string {
		var out []string
		for line := range strings.SplitSeq(ansi.Wrap(text, inner, ""), "\n") {
			out = append(out, paint(inner, p.Card, []seg{span(style, line)}, nil))
		}
		return out
	}
	lines := wrap(fg(p.Title).Bold(true), "Abandon "+shortID(e.ChangeID)+" "+shortID(e.CommitID))
	if title, _, _ := strings.Cut(e.Description, "\n"); title != "" {
		lines = append(lines, wrap(fg(p.Strong), title)...)
	}
	if n := len(e.Bookmarks); n > 0 {
		lines = append(lines, wrap(fg(p.Removed), "Deletes "+plural(n, "bookmark")+" "+strings.Join(e.Bookmarks, ", "))...)
	}
	if n := pickedUnits(m.picks()); n > 0 {
		lines = append(lines, wrap(fg(p.Removed), fmt.Sprintf("Drops %d selected %s", n, plural(n, "hunk")))...)
	}
	footer := wrap(fg(p.Muted), m.keys.confirm.Help().Key+" abandon · any other key cancels")
	if room := max(m.height-2-len(footer), 0); len(lines) > room {
		lines = lines[:room]
		if room > 0 {
			lines[room-1] = paint(inner, p.Card, []seg{span(fg(p.Muted), "…")}, nil)
		}
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(p.ModalBorder).
		BorderBackground(p.Card).
		Background(p.Card).
		Padding(0, 1).
		Render(strings.Join(append(lines, footer...), "\n"))
}

func (m *Model) abandon(e jj.LogEntry) tea.Cmd {
	at, _ := m.log.position()
	keep := logPosition{at: at, cursor: m.log.cursor, query: m.log.query.Value()}
	m.reloading = true
	m.log.followed = ""
	ctx, src, target, head := m.ctx, m.src, m.target, m.targetHead
	return func() tea.Msg {
		msg := abandonedMsg{changeID: e.ChangeID, keep: keep}
		workingCopy, err := src.Abandon(ctx, e.CommitID, e.Bookmarks)
		if err != nil {
			msg.err = fmt.Errorf("abandon %s: %w", shortID(e.ChangeID), err)
		}
		if target == "" || err == nil && (workingCopy || e.CommitID == head) {
			msg.step, msg.loaded = workingCopyStep, fetch(ctx, src, "@")
			return msg
		}
		msg.step, msg.loaded, msg.lost = reviewedAfterAbandon(ctx, src, target, head)
		return msg
	}
}

func reviewedAfterAbandon(ctx context.Context, src Source, target, head string) (revisionStep, loadedMsg, string) {
	loaded := fetch(ctx, src, jj.VisibleCommitRevset(head))
	if !loaded.moved {
		return reviewedStep, loaded, ""
	}
	next, err := src.Successor(ctx, target, head)
	switch {
	case err == nil:
		return reviewedStep, fetch(ctx, src, jj.VisibleCommitRevset(next.CommitID)), ""
	case errors.Is(err, jj.ErrNoRevision) || errors.Is(err, jj.ErrMultipleRevisions):
		return workingCopyStep, fetch(ctx, src, "@"), shortID(target)
	}
	return reviewedStep, loadedMsg{err: fmt.Errorf("find the rewritten %s: %w", shortID(target), err)}, ""
}

func (m *Model) applyAbandoned(msg abandonedMsg) tea.Cmd {
	m.log.keep = msg.keep
	status := "abandoned " + shortID(msg.changeID)
	switch {
	case errors.Is(msg.err, jj.ErrBookmarksChanged):
		status = "bookmarks changed; press " + m.keys.abandon.Help().Key + " to review the abandon again"
	case msg.err != nil:
		status, _, _ = strings.Cut(msg.err.Error(), "\n")
	}
	if msg.lost != "" {
		status += "; " + msg.lost + " was rewritten, reviewing the working copy"
	}
	if err := msg.loaded.err; err != nil {
		m.reloading = false
		m.outdated = true
		m.change.WorkingCopy = false
		m.status = status + "; " + msg.step.failure(err)
		return m.refreshAfterAbandon()
	}
	if msg.step == workingCopyStep {
		m.files, m.reviews = nil, nil
	} else if file, ok := m.currentFile(); ok {
		m.reloadPath = file.Path()
	}
	cmd := m.applyLoad(msg.loaded)
	m.status = status
	return tea.Batch(cmd, m.refreshAfterAbandon())
}

func (m *Model) refreshAfterAbandon() tea.Cmd {
	cmd := m.invalidate()
	if m.view != logView {
		cmd = tea.Batch(cmd, m.loadLog())
	}
	return cmd
}
