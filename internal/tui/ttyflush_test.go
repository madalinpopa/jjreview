//go:build darwin || linux

package tui

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"testing"
	"time"
)

func readWithin(t *testing.T, f *os.File) []byte {
	t.Helper()
	type result struct {
		data []byte
		err  error
	}
	done := make(chan result, 1)
	go func() {
		buf := make([]byte, 64)
		n, err := f.Read(buf)
		done <- result{data: buf[:n], err: err}
	}()
	select {
	case r := <-done:
		if r.err != nil {
			t.Fatalf("Read(%s) error = %v", f.Name(), r.err)
		}
		return r.data
	case <-time.After(5 * time.Second):
		t.Fatalf("Read(%s) did not return within 5s", f.Name())
		return nil
	}
}

func TestEditorCommand_discardsPendingInput(t *testing.T) {
	t.Parallel()
	master, tty := openPTY(t)
	if _, err := master.Write([]byte("\x1b]11;rgb:0000/0000/0000\x07")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	var echo []byte
	for !bytes.Contains(echo, []byte("0000")) {
		echo = append(echo, readWithin(t, master)...)
	}
	cmd := editorCommand{Cmd: exec.CommandContext(t.Context(), "true"), ctx: t.Context()}
	cmd.SetStdin(tty)
	cmd.SetStdout(io.Discard)
	cmd.SetStderr(io.Discard)

	if err := cmd.Run(); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if _, err := master.Write([]byte("\n")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if got := string(readWithin(t, tty)); got != "\n" {
		t.Errorf("Read(tty) = %q, want only the newline typed after the editor started", got)
	}
}

func TestEditorCommand_cancelledLaunchKeepsInputAndStartsNothing(t *testing.T) {
	t.Parallel()
	master, tty := openPTY(t)
	if _, err := master.Write([]byte("pending")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	var echo []byte
	for !bytes.Contains(echo, []byte("pending")) {
		echo = append(echo, readWithin(t, master)...)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	cmd := editorCommand{Cmd: exec.CommandContext(context.WithoutCancel(ctx), "true"), ctx: ctx}
	cmd.SetStdin(tty)
	cmd.SetStdout(io.Discard)
	cmd.SetStderr(io.Discard)

	err := cmd.Run()

	if !errors.Is(err, context.Canceled) {
		t.Errorf("Run() error = %v, want %v", err, context.Canceled)
	}
	if cmd.Process != nil {
		t.Errorf("Run() started process %d, want none", cmd.Process.Pid)
	}
	if _, err := master.Write([]byte("\n")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if got := string(readWithin(t, tty)); got != "pending\n" {
		t.Errorf("Read(tty) = %q, want the pending input kept", got)
	}
}

func TestEditorCommand_toleratesNonTerminalInput(t *testing.T) {
	t.Parallel()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe() error = %v", err)
	}
	t.Cleanup(func() {
		if err := r.Close(); err != nil {
			t.Errorf("close pipe: %v", err)
		}
	})
	if err := w.Close(); err != nil {
		t.Fatalf("close pipe writer: %v", err)
	}
	cmd := editorCommand{Cmd: exec.CommandContext(t.Context(), "true"), ctx: t.Context()}
	cmd.SetStdin(r)
	cmd.SetStdout(io.Discard)
	cmd.SetStderr(io.Discard)

	if err := cmd.Run(); err != nil {
		t.Errorf("Run() error = %v, want nil", err)
	}
}
