package llm

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"time"

	sdk "github.com/microsoft/copilot-sdk/go/v2"

	"github.com/madalinpopa/jjreview/internal/prompt"
)

const (
	DefaultCopilotModel = "auto"
	cleanupTimeout      = 5 * time.Second
	shutdownTimeout     = 3 * time.Second
)

var (
	ErrCopilotNotFound = errors.New("GitHub Copilot CLI not found in PATH; install it or set COPILOT_CLI_PATH")
	errCopilotClosed   = errors.New("copilot: provider is closed")
	errCopilotTimeout  = fmt.Errorf("copilot did not respond within %s", generateTimeout)
)

var _ Provider = (*Copilot)(nil)

type Copilot struct {
	client  *sdk.Client
	model   string
	cliErr  error
	mu      sync.RWMutex
	closing context.Context
	close   context.CancelFunc
}

func NewCopilot(model string) *Copilot {
	path, err := copilotPath()
	closing, cancel := context.WithCancel(context.Background())
	return &Copilot{
		client: sdk.NewClient(&sdk.ClientOptions{
			Mode:          sdk.ModeCodingAgent,
			Runtime:       sdk.SpawnedStdioConnection{Path: path},
			Observability: sdk.ClientObservabilityOptions{LogLevel: "error"},
		}),
		model:   cmp.Or(model, DefaultCopilotModel),
		cliErr:  err,
		closing: closing,
		close:   cancel,
	}
}

func copilotPath() (string, error) {
	if path := os.Getenv("COPILOT_CLI_PATH"); path != "" {
		return path, nil
	}
	path, err := exec.LookPath("copilot")
	if err != nil {
		return "", ErrCopilotNotFound
	}
	return path, nil
}

func (c *Copilot) Generate(ctx context.Context, patch string) (string, error) {
	if strings.TrimSpace(patch) == "" {
		return "", ErrEmptyPatch
	}
	if c.cliErr != nil {
		return "", fmt.Errorf("generate commit message: %w", c.cliErr)
	}

	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.closing.Err() != nil {
		return "", errCopilotClosed
	}
	ctx, cancel := context.WithTimeoutCause(ctx, generateTimeout, errCopilotTimeout)
	defer cancel()
	defer context.AfterFunc(c.closing, cancel)() //nolint:contextcheck

	msg, err := c.generate(ctx, patch)
	if err != nil {
		return "", fmt.Errorf("generate commit message: %w", cmp.Or(context.Cause(ctx), err))
	}
	return msg, nil
}

func (c *Copilot) generate(ctx context.Context, patch string) (msg string, err error) {
	if err := c.client.Start(ctx); err != nil {
		return "", fmt.Errorf("start Copilot runtime: %w", err)
	}
	disabled, err := c.disabledMCPServers(ctx)
	if err != nil {
		return "", err
	}

	session, err := c.client.CreateSession(ctx, &sdk.CreateSessionOptions{
		ClientName:            "jjr",
		Model:                 c.model,
		EnableConfigDiscovery: new(false),
		Tools:                 sdk.SessionToolOptions{AvailableTools: []string{}},
		MCP:                   sdk.SessionMCPOptions{DisabledServers: disabled},
		Instructions:          sdk.CreateSessionInstructionOptions{SkipCustomInstructions: new(true)},
		SystemMessage:         &sdk.SystemMessageConfig{Mode: "replace", Content: prompt.System},
	})
	if err != nil {
		return "", fmt.Errorf("create Copilot session: %w", err)
	}
	defer func() {
		cleanupErr := c.deleteSession(ctx, session.SessionID)
		if err != nil {
			err = errors.Join(err, cleanupErr)
		}
	}()

	event, err := session.SendAndWait(ctx, sdk.MessageOptions{Prompt: prompt.Build(patch)})
	if err != nil {
		return "", fmt.Errorf("send prompt to Copilot: %w", err)
	}
	if event == nil {
		return "", ErrEmptyResponse
	}
	data, ok := event.Data.(*sdk.AssistantMessageData)
	if !ok {
		return "", ErrEmptyResponse
	}
	msg = clean(data.Content)
	if msg == "" {
		return "", ErrEmptyResponse
	}
	return msg, nil
}

func (c *Copilot) disabledMCPServers(ctx context.Context) ([]string, error) {
	config, err := c.client.RPC.MCP.Config().List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list Copilot MCP servers: %w", err)
	}
	return slices.Concat(builtinMCPServers(), slices.Sorted(maps.Keys(config.Servers))), nil
}

func builtinMCPServers() []string {
	return []string{"github-mcp-server", "githubiq"}
}

func (c *Copilot) deleteSession(ctx context.Context, id string) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cleanupTimeout)
	defer cancel()
	if err := c.client.DeleteSession(ctx, id); err != nil {
		return fmt.Errorf("delete Copilot session: %w", err)
	}
	return nil
}

func (c *Copilot) Close() error {
	c.close()
	c.mu.Lock()
	defer c.mu.Unlock()

	stopped := make(chan error, 1)
	go func() { stopped <- c.client.Stop() }()
	select {
	case err := <-stopped:
		if err != nil {
			return fmt.Errorf("copilot: stop runtime: %w", err)
		}
		return nil
	case <-time.After(shutdownTimeout):
		c.client.ForceStop()
		return fmt.Errorf("copilot: runtime did not stop within %s; forced shutdown", shutdownTimeout)
	}
}
