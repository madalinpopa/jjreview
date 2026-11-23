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
	"github.com/madalinpopa/jjreview/internal/tui/tab"
	"github.com/madalinpopa/jjreview/internal/tui/ui"
)

type rewrittenMsg struct {
	toast     int
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

type noticeRef struct {
	from view
	seq  int
	id   int
}

func (m *Model) notify(from view, n tab.Notice) tea.Cmd {
	if !n.Done {
		id, cmd := m.toast.Start(n.Title, n.CommandLabel)
		m.notice = noticeRef{from: from, seq: n.Seq, id: id}
		return cmd
	}
	if m.notice.from == from && m.notice.seq == n.Seq {
		if cmd, ok := m.toast.Finish(m.notice.id, n.Result); ok {
			return cmd
		}
	}
	m.status = n.Result.Summary()
	return nil
}

func (m *Model) rewrite(rw tab.Rewrite) tea.Cmd {
	keep := m.log.HoldPosition()
	m.reloading = true
	id, spin := m.toast.Start(rw.Title, rw.CommandLabel)
	ctx, src, target := m.ctx, m.src, m.target
	head := m.head()
	return tea.Batch(spin, func() tea.Msg {
		msg := rewrittenMsg{toast: id, focus: rw.Focus, keep: keep}
		r, err := rw.Run(ctx, head)
		msg.done, msg.warnings, msg.keepPicks, msg.err = r.Done, r.Warnings, r.KeepPicks, err
		if target == "" || r.Replaced {
			msg.step, msg.loaded = workingCopyStep, fetch(ctx, src, "@")
			return msg
		}
		msg.step, msg.loaded, msg.lost = reviewedAfterRewrite(ctx, src, target, head.CommitID, r.Restored)
		return msg
	})
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
	result := ui.ToastResult{Text: msg.done, Warning: msg.warnings, Err: msg.err}
	done, shown := m.toast.Finish(msg.toast, result)
	var status string
	if !shown {
		status = result.Summary()
	}
	if msg.lost != "" {
		status = joinStatus(status, msg.lost+" was rewritten, reviewing the working copy")
	}
	if err := msg.loaded.err; err != nil {
		m.reloading = false
		m.outdated = true
		m.change.WorkingCopy = false
		m.status = joinStatus(status, msg.step.failure(err))
		return tea.Batch(done, m.refreshAfterRewrite())
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
	return tea.Batch(done, cmd, m.refreshAfterRewrite())
}

func joinStatus(parts ...string) string {
	return strings.Join(slices.DeleteFunc(parts, func(s string) bool { return s == "" }), "; ")
}

func (m *Model) refreshAfterRewrite() tea.Cmd {
	cmd := m.invalidate()
	if m.view != logView {
		cmd = tea.Batch(cmd, m.log.Load())
	}
	return cmd
}
