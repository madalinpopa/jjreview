package llm_test

import (
	"context"
	"encoding/json/v2"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/madalinpopa/jjreview/internal/llm"
	"github.com/madalinpopa/jjreview/internal/prompt"
)

func claudeReply(t *testing.T, result string) string {
	t.Helper()
	data, err := json.Marshal(map[string]any{
		"type":        "result",
		"subtype":     "success",
		"is_error":    false,
		"result":      result,
		"stop_reason": "end_turn",
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func newFakeClaude(t *testing.T, model, reply string) (llm.Claude, string) {
	t.Helper()
	logPath := filepath.Join(t.TempDir(), "call.json")
	t.Setenv("GORACE", "atexit_sleep_ms=0")
	t.Setenv("CLAUDE_CLI_PATH", os.Args[0])
	t.Setenv(envFakeCLIReply, reply)
	t.Setenv(envFakeCLILog, logPath)
	return llm.NewClaude(model), logPath
}

func TestClaude_success(t *testing.T) {
	t.Run("runs an isolated session and cleans the reply", func(t *testing.T) {
		c, logPath := newFakeClaude(t, "", claudeReply(t, "```\nfeat(main): rename package\n```"))

		got, err := c.Generate(t.Context(), samplePatch)
		if err != nil {
			t.Fatalf("Generate() error = %v", err)
		}

		if want := "feat(main): rename package"; got != want {
			t.Errorf("Generate() = %q, want %q", got, want)
		}
		call := readCLICall(t, logPath)
		want := []string{
			"-p", "--output-format", "json", "--tools", "", "--safe-mode",
			"--no-session-persistence", "--strict-mcp-config", "--permission-prompts", "none",
			"--system-prompt", prompt.System, "--model", llm.DefaultClaudeModel, "--effort", "low",
		}
		if !slices.Equal(call.Args, want) {
			t.Errorf("args = %q, want %q", call.Args, want)
		}
		if call.Stdin != prompt.Build(samplePatch) {
			t.Errorf("stdin = %q, want the built prompt", call.Stdin)
		}
		cwd, err := os.Getwd()
		if err != nil {
			t.Fatal(err)
		}
		if call.Dir == cwd {
			t.Errorf("dir = %q, want a temporary directory", call.Dir)
		}
		if _, err := os.Stat(call.Dir); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("stat %q error = %v, want the directory removed", call.Dir, err)
		}
	})

	t.Run("uses the configured model without lowering effort", func(t *testing.T) {
		c, logPath := newFakeClaude(t, "sonnet", claudeReply(t, "fix: handle input"))

		if _, err := c.Generate(t.Context(), samplePatch); err != nil {
			t.Fatalf("Generate() error = %v", err)
		}

		args := readCLICall(t, logPath).Args
		if i := slices.Index(args, "--model"); i < 0 || i+1 >= len(args) || args[i+1] != "sonnet" {
			t.Errorf("args = %q, want --model sonnet", args)
		}
		if slices.Contains(args, "--effort") {
			t.Errorf("args = %q, want no --effort", args)
		}
	})

	t.Run("sends a large patch through stdin", func(t *testing.T) {
		large := samplePatch + strings.Repeat("+line of data\n", 30_000)
		c, logPath := newFakeClaude(t, "", claudeReply(t, "chore: add data"))

		if _, err := c.Generate(t.Context(), large); err != nil {
			t.Fatalf("Generate() error = %v", err)
		}

		call := readCLICall(t, logPath)
		if call.Stdin != prompt.Build(large) {
			t.Errorf("stdin has %d bytes, want the built prompt of %d bytes", len(call.Stdin), len(prompt.Build(large)))
		}
		for _, arg := range call.Args {
			if strings.Contains(arg, "line of data") {
				t.Fatal("patch was passed as an argument")
			}
		}
	})

	t.Run("strips the Anthropic credentials from the environment", func(t *testing.T) {
		t.Setenv("ANTHROPIC_API_KEY", "secret")
		t.Setenv("ANTHROPIC_AUTH_TOKEN", "secret")
		t.Setenv("JJR_FAKE_CLI_KEEP", "kept")
		c, logPath := newFakeClaude(t, "", claudeReply(t, "fix: handle input"))

		if _, err := c.Generate(t.Context(), samplePatch); err != nil {
			t.Fatalf("Generate() error = %v", err)
		}

		env := readCLICall(t, logPath).Env
		for _, name := range []string{"ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN"} {
			if _, ok := env[name]; ok {
				t.Errorf("%s was passed to the CLI", name)
			}
		}
		if env["JJR_FAKE_CLI_KEEP"] != "kept" {
			t.Errorf("env = %v, want other variables kept", env)
		}
	})

	t.Run("resolves a relative CLI path", func(t *testing.T) {
		_, logPath := newFakeClaude(t, "", claudeReply(t, "fix: handle input"))
		t.Chdir(filepath.Dir(os.Args[0]))
		t.Setenv("CLAUDE_CLI_PATH", "."+string(filepath.Separator)+filepath.Base(os.Args[0]))
		c := llm.NewClaude("")

		if _, err := c.Generate(t.Context(), samplePatch); err != nil {
			t.Fatalf("Generate() error = %v", err)
		}

		if _, err := os.Stat(logPath); err != nil {
			t.Errorf("CLI was not run: %v", err)
		}
	})

	t.Run("looks up a bare CLI name in PATH", func(t *testing.T) {
		_, logPath := newFakeClaude(t, "", claudeReply(t, "fix: handle input"))
		t.Setenv("PATH", filepath.Dir(os.Args[0]))
		t.Setenv("CLAUDE_CLI_PATH", filepath.Base(os.Args[0]))
		c := llm.NewClaude("")

		if _, err := c.Generate(t.Context(), samplePatch); err != nil {
			t.Fatalf("Generate() error = %v", err)
		}

		if _, err := os.Stat(logPath); err != nil {
			t.Errorf("CLI was not run: %v", err)
		}
	})
}

func TestClaude_failure(t *testing.T) {
	t.Run("empty patch", func(t *testing.T) {
		c, _ := newFakeClaude(t, "", claudeReply(t, "unused"))

		if _, err := c.Generate(t.Context(), " \n"); !errors.Is(err, llm.ErrEmptyPatch) {
			t.Errorf("Generate() error = %v, want %v", err, llm.ErrEmptyPatch)
		}
	})

	t.Run("missing CLI", func(t *testing.T) {
		t.Setenv("CLAUDE_CLI_PATH", "")
		t.Setenv("PATH", t.TempDir())
		c := llm.NewClaude("")

		if _, err := c.Generate(t.Context(), samplePatch); !errors.Is(err, llm.ErrClaudeNotFound) {
			t.Errorf("Generate() error = %v, want %v", err, llm.ErrClaudeNotFound)
		}
	})

	t.Run("unknown CLI override names the bad value", func(t *testing.T) {
		t.Setenv("CLAUDE_CLI_PATH", "no-such-claude")
		t.Setenv("PATH", t.TempDir())
		c := llm.NewClaude("")

		_, err := c.Generate(t.Context(), samplePatch)

		if !errors.Is(err, exec.ErrNotFound) || !strings.Contains(err.Error(), "CLAUDE_CLI_PATH") || !strings.Contains(err.Error(), "no-such-claude") {
			t.Errorf("Generate() error = %v, want %v naming CLAUDE_CLI_PATH and its value", err, exec.ErrNotFound)
		}
	})

	t.Run("non-zero exit surfaces stderr without the patch", func(t *testing.T) {
		t.Setenv(envFakeCLIExit, "1")
		t.Setenv(envFakeCLIStderr, "Not logged in\x1b[0m\n· Please run /login\n")
		c, _ := newFakeClaude(t, "", "")

		_, err := c.Generate(t.Context(), samplePatch)

		if err == nil || !strings.Contains(err.Error(), "Not logged in") || !strings.Contains(err.Error(), "Please run /login") {
			t.Fatalf("Generate() error = %v, want the stderr text", err)
		}
		if strings.Contains(err.Error(), "package main") {
			t.Errorf("Generate() error = %v, want no patch content", err)
		}
		if strings.ContainsFunc(err.Error(), func(r rune) bool { return r < ' ' }) {
			t.Errorf("Generate() error = %q, want no control characters", err)
		}
	})

	t.Run("bounds long stderr", func(t *testing.T) {
		t.Setenv(envFakeCLIExit, "2")
		t.Setenv(envFakeCLIStderr, strings.Repeat("noise ", 1000)+"final reason")
		c, _ := newFakeClaude(t, "", "")

		_, err := c.Generate(t.Context(), samplePatch)

		if err == nil || !strings.Contains(err.Error(), "final reason") {
			t.Fatalf("Generate() error = %v, want the stderr tail", err)
		}
		if len(err.Error()) > 1000 {
			t.Errorf("Generate() error has %d bytes, want it bounded", len(err.Error()))
		}
	})

	t.Run("empty result reports the stderr diagnostic", func(t *testing.T) {
		t.Setenv(envFakeCLIStderr, "[claude-code:unrecognized_model] model not found\n")
		c, _ := newFakeClaude(t, "nope", claudeReply(t, ""))

		_, err := c.Generate(t.Context(), samplePatch)

		if !errors.Is(err, llm.ErrEmptyResponse) || !strings.Contains(err.Error(), "unrecognized_model") {
			t.Errorf("Generate() error = %v, want %v with the stderr diagnostic", err, llm.ErrEmptyResponse)
		}
	})

	t.Run("malformed JSON", func(t *testing.T) {
		t.Setenv(envFakeCLIStderr, "warning: something odd\n")
		c, _ := newFakeClaude(t, "", "not json")

		_, err := c.Generate(t.Context(), samplePatch)

		if err == nil || !strings.Contains(err.Error(), "something odd") {
			t.Errorf("Generate() error = %v, want a decode error with stderr", err)
		}
	})

	t.Run("error result on non-zero exit reports the result text", func(t *testing.T) {
		t.Setenv(envFakeCLIExit, "1")
		t.Setenv(envFakeCLIStderr, "internal stack trace\n")
		reply, err := json.Marshal(map[string]any{
			"type": "result", "subtype": "success", "is_error": true, "result": "credit balance too low\a\n",
		})
		if err != nil {
			t.Fatal(err)
		}
		c, _ := newFakeClaude(t, "", string(reply))

		_, err = c.Generate(t.Context(), samplePatch)

		if err == nil || !strings.Contains(err.Error(), "credit balance too low") {
			t.Fatalf("Generate() error = %v, want the result text", err)
		}
		if strings.Contains(err.Error(), "stack trace") {
			t.Errorf("Generate() error = %v, want no stderr when the result explains the failure", err)
		}
		if strings.ContainsFunc(err.Error(), func(r rune) bool { return r < ' ' }) {
			t.Errorf("Generate() error = %q, want no control characters", err)
		}
	})

	t.Run("truncated result", func(t *testing.T) {
		reply, err := json.Marshal(map[string]any{
			"type": "result", "subtype": "success", "is_error": false, "result": "feat: partial", "stop_reason": "max_tokens",
		})
		if err != nil {
			t.Fatal(err)
		}
		c, _ := newFakeClaude(t, "", string(reply))

		if _, err := c.Generate(t.Context(), samplePatch); !errors.Is(err, llm.ErrTruncated) {
			t.Errorf("Generate() error = %v, want %v", err, llm.ErrTruncated)
		}
	})

	t.Run("cancellation stops the CLI", func(t *testing.T) {
		c, _ := newFakeClaude(t, "", replyWait)
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
