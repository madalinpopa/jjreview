package llm_test

import (
	"encoding/json/v2"
	"fmt"
	"io"
	"os"
	"strconv"
	"testing"
	"time"
)

const (
	envFakeCLIReply  = "JJR_FAKE_CLI_REPLY"
	envFakeCLILog    = "JJR_FAKE_CLI_LOG"
	envFakeCLIStderr = "JJR_FAKE_CLI_STDERR"
	envFakeCLIExit   = "JJR_FAKE_CLI_EXIT"
)

type cliCall struct {
	Args  []string          `json:"args"`
	Stdin string            `json:"stdin"`
	Dir   string            `json:"dir"`
	Env   map[string]string `json:"env"`
}

func runFakeCLI(reply string) (err error) {
	stdin, err := io.ReadAll(os.Stdin)
	if err != nil {
		return err
	}
	dir, err := os.Getwd()
	if err != nil {
		return err
	}
	call := cliCall{Args: os.Args[1:], Stdin: string(stdin), Dir: dir, Env: map[string]string{}}
	for _, name := range []string{"ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN", "CODEX_API_KEY", "OPENAI_API_KEY", "JJR_FAKE_CLI_KEEP"} {
		if value, ok := os.LookupEnv(name); ok {
			call.Env[name] = value
		}
	}
	data, err := json.Marshal(call)
	if err != nil {
		return err
	}
	if err := os.WriteFile(os.Getenv(envFakeCLILog), data, 0o600); err != nil {
		return err
	}

	if reply == replyWait {
		time.Sleep(time.Minute)
	}
	if _, err := os.Stderr.WriteString(os.Getenv(envFakeCLIStderr)); err != nil {
		return err
	}
	if _, err := os.Stdout.WriteString(reply); err != nil {
		return err
	}
	if code := os.Getenv(envFakeCLIExit); code != "" {
		n, err := strconv.Atoi(code)
		if err != nil {
			return err
		}
		os.Exit(n)
	}
	return nil
}

func fakeCLIMain(reply string) {
	if err := runFakeCLI(reply); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(0)
}

func readCLICall(t *testing.T, logPath string) cliCall {
	t.Helper()
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	var call cliCall
	if err := json.Unmarshal(data, &call); err != nil {
		t.Fatalf("decode CLI call %q: %v", data, err)
	}
	return call
}
