package tui

import (
	"errors"

	tea "charm.land/bubbletea/v2"
	"github.com/madalinpopa/jjreview/internal/jj"
)

type revisionStep int

const (
	workingCopyStep revisionStep = iota
	pickedStep
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
	m.files, m.reviews = nil, nil
	cmd := m.applyLoad(msg.loaded)
	if msg.step != workingCopyStep {
		return cmd
	}
	return tea.Batch(cmd, m.invalidate())
}
