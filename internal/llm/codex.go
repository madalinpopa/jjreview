package llm

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/madalinpopa/jjreview/internal/prompt"
)

var (
	ErrCodexNotFound = errors.New("Codex CLI not found in PATH; install it or set CODEX_CLI_PATH")
	errCodexTimeout  = fmt.Errorf("Codex did not respond within %s", generateTimeout)
)

var _ Provider = Codex{}

type Codex struct {
	path  string
	model string
	err   error
}

func NewCodex(model string) Codex {
	path, err := cliPath("CODEX_CLI_PATH", "codex", ErrCodexNotFound)
	return Codex{path: path, model: model, err: err}
}

func (c Codex) Generate(ctx context.Context, patch string) (string, error) {
	if strings.TrimSpace(patch) == "" {
		return "", ErrEmptyPatch
	}
	if c.err != nil {
		return "", fmt.Errorf("generate commit message: %w", c.err)
	}
	ctx, cancel := context.WithTimeoutCause(ctx, generateTimeout, errCodexTimeout)
	defer cancel()

	stdin := prompt.System + "\n\n" + prompt.Build(patch)
	out, stderr, err := runCLI(ctx, c.path, c.args(), stdin, "CODEX_API_KEY", "OPENAI_API_KEY")
	if err != nil {
		return "", fmt.Errorf("generate commit message: %w", cmp.Or(context.Cause(ctx), withStderr(err, codexErrors(stderr))))
	}
	msg := clean(string(out))
	if msg == "" {
		return "", withStderr(ErrEmptyResponse, codexErrors(stderr))
	}
	return msg, nil
}

func codexErrors(stderr []byte) string {
	var lines []string
	for line := range strings.Lines(string(stderr)) {
		if strings.HasPrefix(line, "ERROR:") && !slices.Contains(lines, line) {
			lines = append(lines, line)
		}
	}
	return sanitize([]byte(strings.Join(lines, "")))
}

func (c Codex) args() []string {
	args := []string{
		"exec", "--ephemeral", "--skip-git-repo-check", "--ignore-user-config", "-s", "read-only",
		"--color", "never", "-c", `web_search="disabled"`, "--disable", "apps", "--disable", "plugins",
		"--disable", "hooks", "--disable", "shell_tool", "--disable", "unified_exec", "--disable", "memories",
		"-c", `model_reasoning_effort="low"`,
	}
	if c.model != "" {
		args = append(args, "-m", c.model)
	}
	return append(args, "-")
}
