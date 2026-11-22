package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/madalinpopa/jjreview/internal/jj"
	"github.com/madalinpopa/jjreview/internal/tui/logview"
	"github.com/madalinpopa/jjreview/internal/tui/tab"
	"github.com/madalinpopa/jjreview/internal/tui/ui"
)

type rewrittenMsg struct {
	done      string
	focus     string
	keep      logview.Position
	step      revisionStep
	loaded    loadedMsg
	lost      string
	warnings  string
	keepPicks bool
	err       error
}

func (m *Model) rewrite(rw tab.Rewrite) tea.Cmd {
	keep := m.log.HoldPosition()
	m.reloading = true
	if rw.Pending != "" {
		m.status = rw.Pending
	}
	ctx, src, target := m.ctx, m.src, m.target
	head := m.head()
	return func() tea.Msg {
		msg := rewrittenMsg{focus: rw.Focus, keep: keep}
		r, err := rw.Run(ctx, head)
		msg.done, msg.warnings, msg.keepPicks, msg.err = r.Done, r.Warnings, r.KeepPicks, err
		if target == "" || r.Replaced {
			msg.step, msg.loaded = workingCopyStep, fetch(ctx, src, "@")
			return msg
		}
		msg.step, msg.loaded, msg.lost = reviewedAfterRewrite(ctx, src, target, head.CommitID, r.Restored)
		return msg
	}
}

func reviewedAfterRewrite(ctx context.Context, src Source, target, head string, restored bool) (revisionStep, loadedMsg, string) {
	loaded := fetch(ctx, src, jj.VisibleCommitRevset(head))
	if !loaded.moved {
		return reviewedStep, loaded, ""
	}
	next, err := src.Successor(ctx, target, head)
	switch {
	case err == nil:
		return reviewedStep, fetch(ctx, src, jj.VisibleCommitRevset(next.CommitID)), ""
	case errors.Is(err, jj.ErrNoRevision) || errors.Is(err, jj.ErrMultipleRevisions):
		if restored {
			if loaded := fetch(ctx, src, jj.VisibleRevset(jj.ChangeRevset(target))); !errors.Is(loaded.err, jj.ErrNoRevision) && !errors.Is(loaded.err, jj.ErrMultipleRevisions) {
				return reviewedStep, loaded, ""
			}
		}
		return workingCopyStep, fetch(ctx, src, "@"), ui.ShortID(target)
	}
	return reviewedStep, loadedMsg{err: fmt.Errorf("find the rewritten %s: %w", ui.ShortID(target), err)}, ""
}

func (m *Model) applyRewritten(msg rewrittenMsg) tea.Cmd {
	keep := msg.keep
	if msg.err == nil && msg.focus != "" {
		keep = keep.Targeting(msg.focus)
	}
	m.log.RestorePosition(keep)
	status := msg.done
	switch {
	case msg.err != nil:
		status, _, _ = strings.Cut(msg.err.Error(), "\n")
	case msg.warnings != "":
		warning, _, _ := strings.Cut(msg.warnings, "\n")
		status += " · " + warning
	}
	if msg.lost != "" {
		status += "; " + msg.lost + " was rewritten, reviewing the working copy"
	}
	if err := msg.loaded.err; err != nil {
		m.reloading = false
		m.outdated = true
		m.change.WorkingCopy = false
		m.status = status + "; " + msg.step.failure(err)
		return m.refreshAfterRewrite()
	}
	held := m.review.Hold()
	if msg.step == workingCopyStep {
		m.review.Reset()
	} else if file, ok := m.review.CurrentFile(); ok {
		m.reloadPath = file.Path()
	}
	cmd := m.applyLoad(msg.loaded)
	if msg.keepPicks && msg.step == reviewedStep && !msg.loaded.moved {
		m.review.KeepPicks(held)
	}
	m.status = status
	return tea.Batch(cmd, m.refreshAfterRewrite())
}

func (m *Model) refreshAfterRewrite() tea.Cmd {
	cmd := m.invalidate()
	if m.view != logView {
		cmd = tea.Batch(cmd, m.log.Load())
	}
	return cmd
}
