package tui

import (
	"errors"

	tea "charm.land/bubbletea/v2"
	"github.com/madalinpopa/jjreview/internal/jj"
	"github.com/madalinpopa/jjreview/internal/tui/review"
	"github.com/madalinpopa/jjreview/internal/tui/tab"
)

type revisionStep int

const (
	workingCopyStep revisionStep = iota
	pickedStep
	reviewedStep
)

type revisionMsg struct {
	step   revisionStep
	loaded loadedMsg
}

func (s revisionStep) String() string {
	switch s {
	case workingCopyStep:
		return "working copy"
	case pickedStep:
		return "selected"
	case reviewedStep:
		return "reviewed"
	}
	return "unknown"
}

func (s revisionStep) failure(err error) string {
	switch {
	case errors.Is(err, jj.ErrNoRevision):
		return "no " + s.String() + " revision"
	case errors.Is(err, jj.ErrMultipleRevisions):
		return "multiple " + s.String() + " revisions"
	}
	return err.Error()
}

func (m Model) lock() tab.Lock {
	l := tab.Lock{Busy: !m.loaded || m.reloading}
	if m.blocked {
		l.Reason = review.BlockedStatus
	}
	return l
}

func (m *Model) canLoad() bool {
	l := m.lock()
	if l.Busy {
		return false
	}
	if l.Reason != "" {
		m.status = l.Reason
		return false
	}
	return true
}

func (m *Model) loadRevision(step revisionStep, rev string) tea.Cmd {
	m.reloading = true
	ctx, src := m.ctx, m.src
	return func() tea.Msg {
		return revisionMsg{step: step, loaded: fetch(ctx, src, rev)}
	}
}

func (m *Model) applyRevision(msg revisionMsg) tea.Cmd {
	if err := msg.loaded.err; err != nil {
		m.reloading = false
		m.status = msg.step.failure(err)
		return nil
	}
	if msg.step == pickedStep {
		m.view = reviewView
	}
	if change := msg.loaded.change; change.ChangeID != m.target || msg.step == pickedStep && change.CommitID != m.targetHead {
		m.review.ChangesOnly()
	}
	m.review.Reset()
	cmd := m.applyLoad(msg.loaded)
	if msg.step != workingCopyStep {
		return cmd
	}
	return tea.Batch(cmd, m.invalidate())
}
