package tui

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/madalinpopa/jjreview/internal/jj"
	"github.com/madalinpopa/jjreview/internal/tui/logview"
	"github.com/madalinpopa/jjreview/internal/tui/ui"
)

const outdatedStatus = "review out of date; press @ to review the working copy"

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

type rewriteResult struct {
	done      string
	replaced  bool
	warnings  string
	keepPicks bool
}

func (m *Model) abandon(e jj.LogEntry) tea.Cmd {
	return m.rewrite("", func(ctx context.Context, src Source, head string) (rewriteResult, error) {
		workingCopy, err := src.Abandon(ctx, e.CommitID, e.Bookmarks)
		if err != nil {
			return rewriteResult{}, fmt.Errorf("abandon %s: %w", ui.ShortID(e.ChangeID), err)
		}
		return rewriteResult{done: "abandoned " + ui.ShortID(e.ChangeID), replaced: workingCopy || e.CommitID == head}, nil
	})
}

func (m *Model) squash(s logview.Squash) tea.Cmd {
	done := "squashed " + ui.ShortID(s.From[0].ChangeID)
	if n := len(s.From); n > 1 {
		done = fmt.Sprintf("squashed %d revisions", n)
	}
	into := ui.ShortID(s.Into.ChangeID)
	return m.rewrite(s.Into.ChangeID, func(ctx context.Context, src Source, head string) (rewriteResult, error) {
		warnings, err := src.Squash(ctx, s.From, s.Into)
		if err != nil {
			return rewriteResult{}, fmt.Errorf("squash into %s: %w", into, err)
		}
		replaced := slices.ContainsFunc(s.From, func(e jj.LogEntry) bool { return e.CommitID == head })
		return rewriteResult{done: done + " into " + into, replaced: replaced, warnings: warnings}, nil
	})
}

func (m *Model) rewrite(focus string, apply func(ctx context.Context, src Source, head string) (rewriteResult, error)) tea.Cmd {
	keep := m.log.HoldPosition()
	m.reloading = true
	ctx, src, target, head := m.ctx, m.src, m.target, m.targetHead
	return func() tea.Msg {
		msg := rewrittenMsg{focus: focus, keep: keep}
		r, err := apply(ctx, src, head)
		msg.done, msg.warnings, msg.keepPicks, msg.err = r.done, r.warnings, r.keepPicks, err
		if target == "" || r.replaced {
			msg.step, msg.loaded = workingCopyStep, fetch(ctx, src, "@")
			return msg
		}
		msg.step, msg.loaded, msg.lost = reviewedAfterRewrite(ctx, src, target, head)
		return msg
	}
}

func reviewedAfterRewrite(ctx context.Context, src Source, target, head string) (revisionStep, loadedMsg, string) {
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

func (m *Model) applyRewritten(msg rewrittenMsg) tea.Cmd {
	keep := msg.keep
	if msg.err == nil && msg.focus != "" {
		keep = keep.Targeting(msg.focus)
	}
	m.log.RestorePosition(keep)
	status := msg.done
	switch {
	case errors.Is(msg.err, jj.ErrBookmarksChanged):
		status = "bookmarks changed; press " + m.log.AbandonKey() + " to review the abandon again"
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
	prevFiles, prevReviews := m.files, m.reviews
	if msg.step == workingCopyStep {
		m.files, m.reviews = nil, nil
	} else if file, ok := m.currentFile(); ok {
		m.reloadPath = file.Path()
	}
	cmd := m.applyLoad(msg.loaded)
	if msg.keepPicks && msg.step == reviewedStep && !msg.loaded.moved && sameFiles(prevFiles, m.files) {
		m.reviews = prevReviews
	}
	m.status = status
	return tea.Batch(cmd, m.refreshAfterRewrite())
}

func (m *Model) refreshAfterRewrite() tea.Cmd {
	cmd := m.invalidate()
	if m.view != logView {
		cmd = tea.Batch(cmd, m.loadLog())
	}
	return cmd
}
