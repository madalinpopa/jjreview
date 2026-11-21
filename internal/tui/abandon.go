package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/madalinpopa/jjreview/internal/jj"
	"github.com/madalinpopa/jjreview/internal/tui/ui"
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

func (m *Model) abandon(e jj.LogEntry) tea.Cmd {
	keep := m.log.holdPosition()
	m.reloading = true
	ctx, src, target, head := m.ctx, m.src, m.target, m.targetHead
	return func() tea.Msg {
		msg := abandonedMsg{changeID: e.ChangeID, keep: keep}
		workingCopy, err := src.Abandon(ctx, e.CommitID, e.Bookmarks)
		if err != nil {
			msg.err = fmt.Errorf("abandon %s: %w", ui.ShortID(e.ChangeID), err)
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
		return workingCopyStep, fetch(ctx, src, "@"), ui.ShortID(target)
	}
	return reviewedStep, loadedMsg{err: fmt.Errorf("find the rewritten %s: %w", ui.ShortID(target), err)}, ""
}

func (m *Model) applyAbandoned(msg abandonedMsg) tea.Cmd {
	m.log.restorePosition(msg.keep)
	status := "abandoned " + ui.ShortID(msg.changeID)
	switch {
	case errors.Is(msg.err, jj.ErrBookmarksChanged):
		status = "bookmarks changed; press " + m.log.keys.abandon.Help().Key + " to review the abandon again"
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
