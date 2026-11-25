package llm_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/madalinpopa/jjreview/internal/llm"
	"github.com/madalinpopa/jjreview/internal/prompt"
)

func newFakeCodex(t *testing.T, model, reply string) (llm.Codex, string) {
	t.Helper()
	logPath := filepath.Join(t.TempDir(), "call.json")
	t.Setenv("GORACE", "atexit_sleep_ms=0")
	t.Setenv("CODEX_CLI_PATH", os.Args[0])
	t.Setenv(envFakeCLIReply, reply)
	t.Setenv(envFakeCLILog, logPath)
	return llm.NewCodex(model), logPath
}

func TestCodex_success(t *testing.T) {
	t.Run("runs a read-only ephemeral session and cleans the reply", func(t *testing.T) {
		c, logPath := newFakeCodex(t, "", "```\nfeat(main): rename package\n```\n")

		got, err := c.Generate(t.Context(), samplePatch)
		if err != nil {
			t.Fatalf("Generate() error = %v", err)
		}

		if want := "feat(main): rename package"; got != want {
			t.Errorf("Generate() = %q, want %q", got, want)
		}
		call := readCLICall(t, logPath)
		want := []string{
			"exec", "--ephemeral", "--skip-git-repo-check", "--ignore-user-config", "-s", "read-only",
			"--color", "never", "-c", `web_search="disabled"`, "--disable", "apps", "--disable", "plugins",
			"--disable", "hooks", "--disable", "shell_tool", "--disable", "unified_exec", "--disable", "memories",
			"-c", `model_reasoning_effort="low"`, "-",
		}
		if !slices.Equal(call.Args, want) {
			t.Errorf("args = %q, want %q", call.Args, want)
		}
		if want := prompt.System + "\n\n" + prompt.Build(samplePatch); call.Stdin != want {
			t.Errorf("stdin = %q, want %q", call.Stdin, want)
		}
	})

	t.Run("passes the configured model before the stdin marker", func(t *testing.T) {
		c, logPath := newFakeCodex(t, "gpt-5.4-mini", "fix: handle input")

		if _, err := c.Generate(t.Context(), samplePatch); err != nil {
			t.Fatalf("Generate() error = %v", err)
		}

		args := readCLICall(t, logPath).Args
		if got, want := args[len(args)-3:], []string{"-m", "gpt-5.4-mini", "-"}; !slices.Equal(got, want) {
			t.Errorf("args = %q, want them to end with %q", args, want)
		}
	})

	t.Run("strips the OpenAI API keys from the environment", func(t *testing.T) {
		t.Setenv("CODEX_API_KEY", "secret")
		t.Setenv("OPENAI_API_KEY", "secret")
		t.Setenv("JJR_FAKE_CLI_KEEP", "kept")
		c, logPath := newFakeCodex(t, "", "fix: handle input")

		if _, err := c.Generate(t.Context(), samplePatch); err != nil {
			t.Fatalf("Generate() error = %v", err)
		}

		env := readCLICall(t, logPath).Env
		for _, name := range []string{"CODEX_API_KEY", "OPENAI_API_KEY"} {
			if _, ok := env[name]; ok {
				t.Errorf("%s was passed to the CLI", name)
			}
		}
		if env["JJR_FAKE_CLI_KEEP"] != "kept" {
			t.Errorf("env = %v, want other variables kept", env)
		}
	})
}

func TestCodex_failure(t *testing.T) {
	t.Run("empty patch", func(t *testing.T) {
		c, _ := newFakeCodex(t, "", "unused")

		if _, err := c.Generate(t.Context(), " \n"); !errors.Is(err, llm.ErrEmptyPatch) {
			t.Errorf("Generate() error = %v, want %v", err, llm.ErrEmptyPatch)
		}
	})

	t.Run("missing CLI", func(t *testing.T) {
		t.Setenv("CODEX_CLI_PATH", "")
		t.Setenv("PATH", t.TempDir())
		c := llm.NewCodex("")

		if _, err := c.Generate(t.Context(), samplePatch); !errors.Is(err, llm.ErrCodexNotFound) {
			t.Errorf("Generate() error = %v, want %v", err, llm.ErrCodexNotFound)
		}
	})

	t.Run("non-zero exit surfaces stderr error lines", func(t *testing.T) {
		t.Setenv(envFakeCLIExit, "1")
		t.Setenv(envFakeCLIStderr, "Reading prompt from stdin...\nERROR: Not signed in. Run codex login\n")
		c, _ := newFakeCodex(t, "", "")

		_, err := c.Generate(t.Context(), samplePatch)

		if err == nil || !strings.Contains(err.Error(), "ERROR: Not signed in. Run codex login") {
			t.Fatalf("Generate() error = %v, want the stderr error line", err)
		}
		if strings.Contains(err.Error(), "Reading prompt") {
			t.Errorf("Generate() error = %v, want only the error lines", err)
		}
	})

	t.Run("reports a repeated error line once", func(t *testing.T) {
		t.Setenv(envFakeCLIExit, "1")
		t.Setenv(envFakeCLIStderr, "ERROR: model is not supported\nuser\nERROR: model is not supported\n")
		c, _ := newFakeCodex(t, "", "")

		_, err := c.Generate(t.Context(), samplePatch)

		if err == nil || strings.Count(err.Error(), "model is not supported") != 1 {
			t.Errorf("Generate() error = %v, want the error line once", err)
		}
	})

	t.Run("non-zero exit without error lines reports the exit status", func(t *testing.T) {
		t.Setenv(envFakeCLIExit, "3")
		t.Setenv(envFakeCLIStderr, "user\n"+prompt.Build(samplePatch))
		c, _ := newFakeCodex(t, "", "")

		_, err := c.Generate(t.Context(), samplePatch)

		if err == nil || !strings.Contains(err.Error(), "exit status 3") {
			t.Fatalf("Generate() error = %v, want the exit status", err)
		}
		if strings.Contains(err.Error(), "package main") {
			t.Errorf("Generate() error = %v, want no patch content", err)
		}
	})

	t.Run("omits the prompt echoed on stderr", func(t *testing.T) {
		t.Setenv(envFakeCLIExit, "1")
		t.Setenv(envFakeCLIStderr, "user\n"+prompt.System+"\n\n"+prompt.Build(samplePatch)+"\nERROR: model is not supported\n")
		c, _ := newFakeCodex(t, "", "")

		_, err := c.Generate(t.Context(), samplePatch)

		if err == nil || !strings.Contains(err.Error(), "model is not supported") {
			t.Fatalf("Generate() error = %v, want the stderr text", err)
		}
		if strings.Contains(err.Error(), "package main") {
			t.Errorf("Generate() error = %v, want no patch content", err)
		}
	})

	t.Run("empty output reports the stderr diagnostic", func(t *testing.T) {
		t.Setenv(envFakeCLIStderr, "warning: noise\nERROR: model is not supported\n")
		c, _ := newFakeCodex(t, "nope", " \n")

		_, err := c.Generate(t.Context(), samplePatch)

		if !errors.Is(err, llm.ErrEmptyResponse) || !strings.Contains(err.Error(), "model is not supported") {
			t.Errorf("Generate() error = %v, want %v with the stderr diagnostic", err, llm.ErrEmptyResponse)
		}
		if strings.Contains(err.Error(), "noise") {
			t.Errorf("Generate() error = %v, want only the error lines", err)
		}
	})

	t.Run("cancellation stops the CLI", func(t *testing.T) {
		c, _ := newFakeCodex(t, "", replyWait)
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()

		start := time.Now()
		_, err := c.Generate(ctx, samplePatch)

		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("Generate() error = %v, want %v", err, context.DeadlineExceeded)
		}
		if elapsed := time.Since(start); elapsed > 10*time.Second {
			t.Errorf("Generate() took %s after cancellation", elapsed)
		}
	})
}
