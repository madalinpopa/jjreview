package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"syscall"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/madalinpopa/jjreview/internal/config"
	"github.com/madalinpopa/jjreview/internal/theme"
)

func TestRun_success(t *testing.T) {
	t.Run("prints the version", func(t *testing.T) {
		var out bytes.Buffer

		err := run(t.Context(), []string{"-version"}, &out)

		if err != nil {
			t.Fatalf("run(-version) error = %v", err)
		}
		if got := out.String(); got != "dev\n" {
			t.Errorf("run(-version) output = %q, want %q", got, "dev\n")
		}
	})
}

func TestRun_help(t *testing.T) {
	err := run(t.Context(), []string{"-h"}, &bytes.Buffer{})

	if err != nil {
		t.Errorf("run(-h) error = %v, want nil", err)
	}
}

func TestRun_failed(t *testing.T) {
	t.Run("rejects apply-split with missing arguments", func(t *testing.T) {
		err := run(t.Context(), []string{"apply-split", "spec.json"}, &bytes.Buffer{})

		if !errors.Is(err, errUsage) {
			t.Errorf("run(apply-split spec.json) error = %v, want %v", err, errUsage)
		}
	})

}

func TestParseArgs_success(t *testing.T) {
	tests := map[string]struct {
		args []string
		want string
	}{
		"defaults to the working copy": {args: nil, want: "@"},
		"takes a positional revision":  {args: []string{"@-"}, want: "@-"},
		"takes a revset expression":    {args: []string{"trunk()..@"}, want: "trunk()..@"},
		"takes -r":                     {args: []string{"-r", "xyz"}, want: "xyz"},
		"takes --revision":             {args: []string{"--revision", "xyz"}, want: "xyz"},
		"takes -R before the revision": {args: []string{"-R", "/repo", "abc"}, want: "abc"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := parseArgs(tt.args)

			if err != nil {
				t.Fatalf("parseArgs(%q) error = %v", tt.args, err)
			}
			if got.rev != tt.want {
				t.Errorf("parseArgs(%q) rev = %q, want %q", tt.args, got.rev, tt.want)
			}
		})
	}
}

func TestParseArgs_failed(t *testing.T) {
	tests := map[string][]string{
		"two positional revisions":   {"a", "b"},
		"positional revision and -r": {"-r", "a", "b"},
	}

	for name, args := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := parseArgs(args)

			if !errors.Is(err, errRevision) {
				t.Errorf("parseArgs(%q) error = %v, want %v", args, err, errRevision)
			}
		})
	}
}

func TestEditorCommand_success(t *testing.T) {
	tests := map[string]struct {
		visual, editor string
		want           []string
	}{
		"prefers VISUAL":         {visual: "code -w", editor: "nano", want: []string{"code", "-w", "/r/a.go"}},
		"falls back to EDITOR":   {editor: "nvim", want: []string{"nvim", "/r/a.go"}},
		"falls back to vi":       {want: []string{"vi", "/r/a.go"}},
		"ignores blank settings": {visual: "  ", editor: "hx", want: []string{"hx", "/r/a.go"}},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			cmd := editorCommand(tt.visual, tt.editor)(t.Context(), "/r/a.go")

			if !slices.Equal(cmd.Args, tt.want) {
				t.Errorf("editorCommand(%q, %q) args = %q, want %q", tt.visual, tt.editor, cmd.Args, tt.want)
			}
		})
	}
}

func TestResetColorScheme_success(t *testing.T) {
	tests := map[string]struct {
		mode theme.Mode
		want string
	}{
		"auto turns colour scheme reports off": {mode: theme.ModeAuto, want: ansi.ResetModeLightDark},
		"light writes nothing":                 {mode: theme.ModeLight, want: ""},
		"dark writes nothing":                  {mode: theme.ModeDark, want: ""},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			var out bytes.Buffer

			err := resetColorScheme(&out, tt.mode)

			if err != nil {
				t.Fatalf("resetColorScheme(%q) error = %v", tt.mode, err)
			}
			if got := out.String(); got != tt.want {
				t.Errorf("resetColorScheme(%q) output = %q, want %q", tt.mode, got, tt.want)
			}
		})
	}
}

func TestFinishProgram_success(t *testing.T) {
	tests := map[string]error{
		"normal quit":   nil,
		"ctrl+c signal": fmt.Errorf("%w: %w", tea.ErrProgramKilled, tea.ErrInterrupted),
		"killed":        tea.ErrProgramKilled,
		"cancelled":     fmt.Errorf("%w: %w", tea.ErrProgramKilled, context.Canceled),
	}

	for name, runErr := range tests {
		t.Run(name, func(t *testing.T) {
			var out bytes.Buffer

			err := finishProgram(&out, theme.ModeAuto, runErr)

			if err != nil {
				t.Fatalf("finishProgram(%v) error = %v", runErr, err)
			}
			if got := out.String(); got != ansi.ResetModeLightDark {
				t.Errorf("finishProgram(%v) output = %q, want %q", runErr, got, ansi.ResetModeLightDark)
			}
		})
	}
}

func TestFinishProgram_failed(t *testing.T) {
	tests := map[string]struct {
		runErr  error
		wantOut string
	}{
		"before taking over the terminal": {runErr: fmt.Errorf("bubbletea: error opening TTY: %w", os.ErrNotExist)},
		"panic":                           {runErr: fmt.Errorf("%w: %w", tea.ErrProgramKilled, tea.ErrProgramPanic), wantOut: ansi.ResetModeLightDark},
		"read error":                      {runErr: fmt.Errorf("%w: %w", tea.ErrProgramKilled, syscall.EIO), wantOut: ansi.ResetModeLightDark},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			var out bytes.Buffer

			err := finishProgram(&out, theme.ModeAuto, tt.runErr)

			if !errors.Is(err, tt.runErr) {
				t.Errorf("finishProgram(%v) error = %v, want %v", tt.runErr, err, tt.runErr)
			}
			if got := out.String(); got != tt.wantOut {
				t.Errorf("finishProgram(%v) output = %q, want %q", tt.runErr, got, tt.wantOut)
			}
		})
	}
}

func TestProvider_none(t *testing.T) {
	t.Parallel()

	gen, err := provider(config.Config{Provider: config.ProviderNone})

	if err != nil {
		t.Fatalf("provider() error = %v", err)
	}
	if gen != nil {
		t.Errorf("provider() = %T, want no generator", gen)
	}
}
