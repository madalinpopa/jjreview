package tui

import (
	"context"
	"errors"
	"os/exec"
	"testing"
)

func TestEditorLaunch_cancelledByQuit(t *testing.T) {
	t.Parallel()
	m := New(t.Context(), nil, nil, "@").WithEditor(func(ctx context.Context, _ string) *exec.Cmd {
		return exec.CommandContext(ctx, "true")
	})
	cmd := m.launch("/repo/a.go")

	m.quit()

	if err := cmd.ctx.Err(); !errors.Is(err, context.Canceled) {
		t.Errorf("launch context error = %v, want %v", err, context.Canceled)
	}
}
