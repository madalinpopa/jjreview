package tui

import (
	"context"
	"fmt"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/madalinpopa/jjreview/internal/tui/operations"
	"github.com/madalinpopa/jjreview/internal/tui/tab"
	"github.com/madalinpopa/jjreview/internal/tui/ui"
)

func (m Model) OperationCursor() string {
	return m.operations.SelectedID()
}

func (m Model) operationEnv() operations.Env {
	return operations.Env{
		Layout:  m.layout(),
		Dropped: pickedUnits(m.picks()),
	}
}

func (m *Model) showOperations() tea.Cmd {
	m.view = operationsView
	return m.operations.Show()
}

func (m *Model) operationsKey(msg tea.KeyPressMsg) tea.Cmd {
	if m.operations.Ignores(msg, m.reloading) {
		return nil
	}
	armed := m.armed
	m.armed = ""
	if !m.operations.OverlayOpen() {
		m.status = ""
		switch {
		case key.Matches(msg, m.keys.reviewTab):
			m.view = reviewView
			return nil
		case key.Matches(msg, m.keys.logTab):
			return m.showLog()
		case key.Matches(msg, m.keys.bookmarksTab):
			return m.showBookmarks()
		case key.Matches(msg, m.keys.quit):
			if m.confirmDrop(msg, armed) {
				return m.quit()
			}
			return nil
		case key.Matches(msg, m.keys.help):
			m.help = true
			m.helpTop = 0
			return nil
		}
	}
	in := m.operations.Key(msg, m.operationEnv())
	switch {
	case in.Run != nil:
		if m.canLoad() {
			return m.runOperation(*in.Run)
		}
	case in.Status != "":
		m.status = in.Status
	case in.Refresh:
		return m.invalidate()
	}
	return nil
}

func (m *Model) runOperation(a operations.Action) tea.Cmd {
	op := a.Op
	id := ui.ShortID(op.ID)
	src := m.src
	return m.rewrite(tab.Rewrite{Run: func(ctx context.Context, head tab.Head) (tab.Result, error) {
		switch a.Kind {
		case operations.Restore:
			if err := src.RestoreOperation(ctx, op.ID); err != nil {
				return tab.Result{KeepPicks: true}, fmt.Errorf("restore operation %s: %w", id, err)
			}
			return tab.Result{Done: "restored operation " + id, Replaced: head.WorkingCopy, Restored: true}, nil
		case operations.Undo:
			if err := src.RevertOperation(ctx, op.ID); err != nil {
				return tab.Result{KeepPicks: true}, fmt.Errorf("undo operation %s: %w", id, err)
			}
			return tab.Result{Done: "undid operation " + id, Replaced: head.WorkingCopy, Restored: true}, nil
		}
		if err := src.AbandonOperation(ctx, op.ID); err != nil {
			return tab.Result{KeepPicks: true}, fmt.Errorf("abandon operation %s: %w", id, err)
		}
		return tab.Result{Done: "abandoned operation " + id, KeepPicks: true}, nil
	}})
}
