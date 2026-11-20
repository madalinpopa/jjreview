package llm_test

import (
	"bufio"
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net/textproto"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	sdk "github.com/microsoft/copilot-sdk/go/v2"

	"github.com/madalinpopa/jjreview/internal/llm"
	"github.com/madalinpopa/jjreview/internal/prompt"
)

const (
	envFakeReply      = "JJR_FAKE_COPILOT_REPLY"
	envFakeLog        = "JJR_FAKE_COPILOT_LOG"
	envFakeSkipDelete = "JJR_FAKE_COPILOT_SKIP_DELETE"
	replyError        = "!error"
	replyWait         = "!wait"
	replyMCPError     = "!mcp-error"
	copilotPatch      = "diff --git a/main.go b/main.go\n--- a/main.go\n+++ b/main.go\n@@ -1 +1 @@\n-package old\n+package main\n"
)

func TestMain(m *testing.M) {
	if reply, ok := os.LookupEnv(envFakeReply); ok {
		if err := runFakeRuntime(os.Stdin, os.Stdout, reply, os.Getenv(envFakeLog)); err != nil && !errors.Is(err, io.EOF) {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

type rpcRequest struct {
	ID     jsontext.Value `json:"id,omitzero"`
	Method string         `json:"method"`
	Params map[string]any `json:"params"`
}

func runFakeRuntime(r io.Reader, w io.Writer, reply, logPath string) (err error) {
	logFile, err := os.Create(logPath)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, logFile.Close()) }()

	in := textproto.NewReader(bufio.NewReader(r))
	for {
		header, err := in.ReadMIMEHeader()
		if err != nil {
			return err
		}
		n, err := strconv.Atoi(header.Get("Content-Length"))
		if err != nil {
			return err
		}
		body := make([]byte, n)
		if _, err := io.ReadFull(in.R, body); err != nil {
			return err
		}

		var req rpcRequest
		if err := json.Unmarshal(body, &req); err != nil {
			return err
		}
		if req.ID == nil {
			continue
		}
		if err := json.MarshalWrite(logFile, req); err != nil {
			return err
		}
		if _, err := logFile.WriteString("\n"); err != nil {
			return err
		}

		resp := map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": nil}
		switch req.Method {
		case "connect":
			resp["result"] = map[string]any{"ok": true, "protocolVersion": sdk.SDKProtocolVersion}
		case "session.create":
			resp["result"] = map[string]any{"sessionId": req.Params["sessionId"]}
		case "session.sendAndWait":
			if reply == replyWait {
				continue
			}
			if reply == replyError {
				delete(resp, "result")
				resp["error"] = map[string]any{"code": -32603, "message": "model unavailable"}
				break
			}
			resp["result"] = map[string]any{
				"messageId": "request-1",
				"aborted":   false,
				"response": map[string]any{
					"id":        "event-1",
					"timestamp": "2026-01-01T00:00:00Z",
					"type":      "assistant.message",
					"data":      map[string]any{"messageId": "response-1", "content": reply},
				},
			}
		case "mcp.config.list":
			if reply == replyMCPError {
				delete(resp, "result")
				resp["error"] = map[string]any{"code": -32603, "message": "config unavailable"}
				break
			}
			resp["result"] = map[string]any{"servers": map[string]any{
				"playwright": map[string]any{"command": "npx"},
				"context7":   map[string]any{"command": "npx"},
			}}
		case "session.delete":
			if os.Getenv(envFakeSkipDelete) == "1" {
				continue
			}
			resp["result"] = map[string]any{"success": true}
		case "session.options.update", "session.detach":
			resp["result"] = map[string]any{"success": true}
		}

		out, err := json.Marshal(resp)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "Content-Length: %d\r\n\r\n%s", len(out), out); err != nil {
			return err
		}
	}
}

func newFakeCopilot(t *testing.T, model, reply string) (*llm.Copilot, string) {
	t.Helper()
	logPath := filepath.Join(t.TempDir(), "requests.jsonl")
	t.Setenv("COPILOT_CLI_PATH", os.Args[0])
	t.Setenv(envFakeReply, reply)
	t.Setenv(envFakeLog, logPath)

	c := llm.NewCopilot(model)
	t.Cleanup(func() {
		if err := c.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})
	return c, logPath
}

func readRequests(t *testing.T, logPath string) map[string]map[string]any {
	t.Helper()
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	requests := make(map[string]map[string]any)
	for line := range strings.Lines(string(data)) {
		var req rpcRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			t.Fatalf("decode request %q: %v", line, err)
		}
		requests[req.Method] = req.Params
	}
	return requests
}

func TestCopilot_success(t *testing.T) {
	t.Run("sends an isolated session and cleans the reply", func(t *testing.T) {
		c, logPath := newFakeCopilot(t, "", "```\nfeat(main): rename package\n```")

		got, err := c.Generate(t.Context(), copilotPatch)
		if err != nil {
			t.Fatalf("Generate() error = %v", err)
		}
		if want := "feat(main): rename package"; got != want {
			t.Errorf("Generate() = %q, want %q", got, want)
		}
		if err := c.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}

		requests := readRequests(t, logPath)
		create := requests["session.create"]
		if create["model"] != llm.DefaultCopilotModel {
			t.Errorf("model = %v, want %s", create["model"], llm.DefaultCopilotModel)
		}
		tools, _ := create["tools"].(map[string]any)
		if available, ok := tools["availableTools"].([]any); !ok || len(available) != 0 {
			t.Errorf("tools.availableTools = %v, want []", tools["availableTools"])
		}
		mcp, _ := create["mcp"].(map[string]any)
		if got, want := fmt.Sprint(mcp["disabledServers"]), "[github-mcp-server githubiq context7 playwright]"; got != want {
			t.Errorf("mcp.disabledServers = %s, want %s", got, want)
		}
		instructions, _ := create["instructions"].(map[string]any)
		if instructions["skipCustomInstructions"] != true {
			t.Errorf("instructions = %v, want skipCustomInstructions", instructions)
		}
		system, _ := create["systemMessage"].(map[string]any)
		if system["mode"] != "replace" || system["content"] != prompt.System {
			t.Errorf("systemMessage = %v", system)
		}
		if got := requests["session.sendAndWait"]["prompt"]; got != prompt.Build(copilotPatch) {
			t.Errorf("prompt = %v", got)
		}
		if _, ok := requests["session.delete"]; !ok {
			t.Error("session.delete was not called")
		}
	})

	t.Run("uses the configured model", func(t *testing.T) {
		c, logPath := newFakeCopilot(t, "gpt-5-mini", "fix: handle input")

		if _, err := c.Generate(t.Context(), copilotPatch); err != nil {
			t.Fatalf("Generate() error = %v", err)
		}
		if got := readRequests(t, logPath)["session.create"]["model"]; got != "gpt-5-mini" {
			t.Errorf("model = %v, want gpt-5-mini", got)
		}
	})

	t.Run("keeps the message when session cleanup times out", func(t *testing.T) {
		t.Setenv(envFakeSkipDelete, "1")
		c, logPath := newFakeCopilot(t, "", "fix: handle input")

		got, err := c.Generate(t.Context(), copilotPatch)
		if err != nil {
			t.Fatalf("Generate() error = %v", err)
		}
		if want := "fix: handle input"; got != want {
			t.Errorf("Generate() = %q, want %q", got, want)
		}
		if _, ok := readRequests(t, logPath)["session.delete"]; !ok {
			t.Error("session.delete was not called")
		}
	})
}

func TestCopilot_failure(t *testing.T) {
	for name, tc := range map[string]struct{ reply, want string }{
		"empty response":   {reply: "  ", want: llm.ErrEmptyResponse.Error()},
		"runtime error":    {reply: replyError, want: "model unavailable"},
		"mcp config error": {reply: replyMCPError, want: "config unavailable"},
	} {
		t.Run(name, func(t *testing.T) {
			c, _ := newFakeCopilot(t, "", tc.reply)

			if _, err := c.Generate(t.Context(), copilotPatch); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Generate() error = %v, want %q", err, tc.want)
			}
		})
	}

	t.Run("empty patch", func(t *testing.T) {
		c, _ := newFakeCopilot(t, "", "unused")

		if _, err := c.Generate(t.Context(), " \n"); !errors.Is(err, llm.ErrEmptyPatch) {
			t.Errorf("Generate() error = %v, want %v", err, llm.ErrEmptyPatch)
		}
	})

	t.Run("missing CLI", func(t *testing.T) {
		t.Setenv("COPILOT_CLI_PATH", "")
		t.Setenv("PATH", t.TempDir())
		c := llm.NewCopilot("")
		t.Cleanup(func() {
			if err := c.Close(); err != nil {
				t.Errorf("Close() error = %v", err)
			}
		})

		if _, err := c.Generate(t.Context(), copilotPatch); !errors.Is(err, llm.ErrCopilotNotFound) {
			t.Errorf("Generate() error = %v, want %v", err, llm.ErrCopilotNotFound)
		}
	})

	t.Run("canceled request still deletes the session", func(t *testing.T) {
		c, logPath := newFakeCopilot(t, "", replyWait)
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()

		if _, err := c.Generate(ctx, copilotPatch); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Generate() error = %v, want %v", err, context.DeadlineExceeded)
		}
		if _, ok := readRequests(t, logPath)["session.delete"]; !ok {
			t.Error("session.delete was not called after cancellation")
		}
	})

	t.Run("close cancels an in-flight request", func(t *testing.T) {
		c, _ := newFakeCopilot(t, "", replyWait)
		closed := make(chan error, 1)
		time.AfterFunc(time.Second, func() { closed <- c.Close() })

		if _, err := c.Generate(t.Context(), copilotPatch); !errors.Is(err, context.Canceled) {
			t.Errorf("Generate() error = %v, want %v", err, context.Canceled)
		}
		if err := <-closed; err != nil {
			t.Errorf("Close() error = %v", err)
		}
		if _, err := c.Generate(t.Context(), copilotPatch); err == nil || !strings.Contains(err.Error(), "closed") {
			t.Errorf("Generate() after Close error = %v, want closed", err)
		}
	})
}
