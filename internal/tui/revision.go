package tui

import (
	"cmp"
	"errors"

	tea "charm.land/bubbletea/v2"
	"github.com/madalinpopa/jjreview/internal/jj"
)

type revisionStep int

const (
	parentStep revisionStep = iota
	childStep
	workingCopyStep
)

type revisionMsg struct {
	step   revisionStep
	loaded loadedMsg
}

func (s revisionStep) String() string {
	switch s {
	case parentStep:
		return "parent"
	case childStep:
		return "child"
	case workingCopyStep:
		return "working copy"
	}
	return "unknown"
}

func (s revisionStep) revset(changeID string) (string, bool) {
	switch s {
	case parentStep:
		return changeID + "-", true
	case childStep:
		return changeID + "+", true
	case workingCopyStep:
		return "@", true
	}
	return "", false
}

func (s revisionStep) relative() bool {
	return s == parentStep || s == childStep
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

func (m *Model) canLoad() bool {
	if !m.loaded || m.reloading {
		return false
	}
	if m.blocked {
		m.status = blockedStatus
		return false
	}
	return true
}

func (m *Model) moveRevision(step revisionStep) tea.Cmd {
	if !m.canLoad() || step.relative() && m.err != nil {
		return nil
	}
	rev, ok := step.revset(cmp.Or(m.target, m.change.ChangeID))
	if !ok {
		return nil
	}
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
	m.files, m.reviews = nil, nil
	return m.applyLoad(msg.loaded)
}
