package llm

import (
	"cmp"
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"strings"

	"github.com/madalinpopa/jjreview/internal/prompt"
)

const DefaultClaudeModel = "haiku"

var (
	ErrClaudeNotFound = errors.New("Claude Code CLI not found in PATH; install it or set CLAUDE_CLI_PATH")
	errClaudeTimeout  = fmt.Errorf("Claude Code did not respond within %s", generateTimeout)
)

var _ Provider = Claude{}

type Claude struct {
	path  string
	model string
	err   error
}

type claudeResult struct {
	Subtype    string `json:"subtype"`
	IsError    bool   `json:"is_error"`
	Result     string `json:"result"`
	StopReason string `json:"stop_reason"`
}

func NewClaude(model string) Claude {
	path, err := cliPath("CLAUDE_CLI_PATH", "claude", ErrClaudeNotFound)
	return Claude{path: path, model: cmp.Or(model, DefaultClaudeModel), err: err}
}

func (c Claude) Generate(ctx context.Context, patch string) (string, error) {
	if strings.TrimSpace(patch) == "" {
		return "", ErrEmptyPatch
	}
	if c.err != nil {
		return "", fmt.Errorf("generate commit message: %w", c.err)
	}
	ctx, cancel := context.WithTimeoutCause(ctx, generateTimeout, errClaudeTimeout)
	defer cancel()

	out, stderr, err := runCLI(ctx, c.path, c.args(), prompt.Build(patch), "ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN")
	var res claudeResult
	decodeErr := json.Unmarshal(out, &res)
	if err != nil {
		if cause := context.Cause(ctx); cause != nil {
			return "", fmt.Errorf("generate commit message: %w", cause)
		}
		diag := sanitize(stderr)
		if decodeErr == nil && res.IsError && strings.TrimSpace(res.Result) != "" {
			diag = sanitize([]byte(res.Result))
		}
		return "", fmt.Errorf("generate commit message: %w", withStderr(err, diag))
	}
	if decodeErr != nil {
		return "", withStderr(fmt.Errorf("generate commit message: decode Claude Code output: %w", decodeErr), sanitize(stderr))
	}
	if res.IsError || res.Subtype != "success" {
		return "", withStderr(fmt.Errorf("generate commit message: Claude Code failed (%s): %s", res.Subtype, sanitize([]byte(res.Result))), sanitize(stderr))
	}
	if res.StopReason == "max_tokens" {
		return "", ErrTruncated
	}
	msg := clean(res.Result)
	if msg == "" {
		return "", withStderr(ErrEmptyResponse, sanitize(stderr))
	}
	return msg, nil
}

func (c Claude) args() []string {
	args := []string{
		"-p", "--output-format", "json", "--tools", "", "--safe-mode",
		"--no-session-persistence", "--strict-mcp-config", "--permission-prompts", "none",
		"--system-prompt", prompt.System, "--model", c.model,
	}
	if c.model == DefaultClaudeModel {
		args = append(args, "--effort", "low")
	}
	return args
}
