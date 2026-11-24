package llm_test

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/v2/option"

	"github.com/madalinpopa/jjreview/internal/llm"
)

const samplePatch = "diff --git a/main.go b/main.go\n--- a/main.go\n+++ b/main.go\n@@ -1 +1 @@\n-package old\n+package main\n"

type request struct {
	Model        string         `json:"model"`
	MaxTokens    int64          `json:"max_tokens"`
	System       []textBlock    `json:"system"`
	Messages     []message      `json:"messages"`
	Temperature  jsontext.Value `json:"temperature"`
	TopP         jsontext.Value `json:"top_p"`
	TopK         jsontext.Value `json:"top_k"`
	Thinking     jsontext.Value `json:"thinking"`
	OutputConfig jsontext.Value `json:"output_config"`
}

type textBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type message struct {
	Role    string      `json:"role"`
	Content []textBlock `json:"content"`
}

func (r request) userText() string {
	var b strings.Builder
	for _, m := range r.Messages {
		if m.Role != "user" {
			continue
		}
		for _, c := range m.Content {
			b.WriteString(c.Text)
		}
	}
	return b.String()
}

func messageResponse(text, stopReason string) string {
	body, err := json.Marshal(map[string]any{
		"id":            "msg_test",
		"type":          "message",
		"role":          "assistant",
		"model":         "claude-haiku-5-5",
		"content":       []map[string]string{{"type": "text", "text": text}},
		"stop_reason":   stopReason,
		"stop_sequence": nil,
		"usage":         map[string]int{"input_tokens": 10, "output_tokens": 5},
	})
	if err != nil {
		panic(err)
	}
	return string(body)
}

type fakeAPI struct {
	t        *testing.T
	status   int
	body     string
	calls    atomic.Int32
	captured chan request
}

func newFakeAPI(t *testing.T, status int, body string) (*fakeAPI, llm.Anthropic) {
	t.Helper()
	return newFakeAPIModel(t, "", status, body)
}

func newFakeAPIModel(t *testing.T, model string, status int, body string) (*fakeAPI, llm.Anthropic) {
	t.Helper()
	api := &fakeAPI{t: t, status: status, body: body, captured: make(chan request, 1)}
	server := httptest.NewServer(api)
	t.Cleanup(server.Close)
	gen := llm.NewAnthropic(model,
		option.WithBaseURL(server.URL),
		option.WithAPIKey("test-key"),
		option.WithMaxRetries(0),
	)
	return api, gen
}

func (f *fakeAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.calls.Add(1)
	if r.Method != http.MethodPost || r.URL.Path != "/v1/messages" {
		f.t.Errorf("request = %s %s, want POST /v1/messages", r.Method, r.URL.Path)
		http.NotFound(w, r)
		return
	}
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		f.t.Errorf("read body: %v", err)
	}
	var req request
	if err := json.Unmarshal(raw, &req); err != nil {
		f.t.Errorf("decode body: %v", err)
	}
	select {
	case f.captured <- req:
	default:
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(f.status)
	if _, err := io.WriteString(w, f.body); err != nil {
		f.t.Errorf("write response: %v", err)
	}
}

func (f *fakeAPI) request(t *testing.T) request {
	t.Helper()
	select {
	case req := <-f.captured:
		return req
	default:
		t.Fatal("no request captured")
		return request{}
	}
}

func TestGenerate_success(t *testing.T) {
	t.Parallel()
	api, gen := newFakeAPI(t, http.StatusOK, messageResponse("  feat(main): rename package\n", "end_turn"))

	got, err := gen.Generate(t.Context(), samplePatch)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if want := "feat(main): rename package"; got != want {
		t.Errorf("Generate() = %q, want %q", got, want)
	}

	req := api.request(t)
	if req.Model != "claude-haiku-5-5" {
		t.Errorf("model = %q, want %q", req.Model, "claude-haiku-5-5")
	}
	if req.MaxTokens <= 0 {
		t.Errorf("max_tokens = %d, want positive", req.MaxTokens)
	}
	if len(req.System) == 0 || !strings.Contains(req.System[0].Text, "Conventional Commits") {
		t.Errorf("system = %+v, want a prompt mentioning Conventional Commits", req.System)
	}
	user := req.userText()
	if !strings.Contains(user, samplePatch) {
		t.Errorf("user message does not contain the patch: %q", user)
	}
	if strings.Contains(user, "truncated") {
		t.Errorf("user message mentions truncation for a small patch: %q", user)
	}
	for name, field := range map[string]jsontext.Value{
		"temperature": req.Temperature,
		"top_p":       req.TopP,
		"top_k":       req.TopK,
		"thinking":    req.Thinking,
	} {
		if field != nil {
			t.Errorf("request sets %s = %s, want absent", name, field)
		}
	}
	var outputConfig struct {
		Effort string `json:"effort"`
	}
	if err := json.Unmarshal(req.OutputConfig, &outputConfig); err != nil || outputConfig.Effort != "low" {
		t.Errorf("output_config = %s, want effort low", req.OutputConfig)
	}
	if n := len(req.Messages); n != 1 || req.Messages[0].Role != "user" {
		t.Errorf("messages = %+v, want a single user message", req.Messages)
	}
}

func TestGenerate_customModel(t *testing.T) {
	t.Parallel()
	api, gen := newFakeAPIModel(t, "claude-sonnet-5-5", http.StatusOK, messageResponse("feat: x", "end_turn"))

	if _, err := gen.Generate(t.Context(), samplePatch); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	req := api.request(t)
	if req.Model != "claude-sonnet-5-5" {
		t.Errorf("model = %q, want %q", req.Model, "claude-sonnet-5-5")
	}
	if req.OutputConfig != nil {
		t.Errorf("output_config = %s, want absent for a custom model", req.OutputConfig)
	}
}

func TestGenerate_stripsCodeFence(t *testing.T) {
	t.Parallel()
	fenced := "```text\nfix(diff): handle empty hunks\n\n- skip blank lines\n- keep counts\n```\n"
	_, gen := newFakeAPI(t, http.StatusOK, messageResponse(fenced, "end_turn"))

	got, err := gen.Generate(t.Context(), samplePatch)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	want := "fix(diff): handle empty hunks\n\n- skip blank lines\n- keep counts"
	if got != want {
		t.Errorf("Generate() = %q, want %q", got, want)
	}
}

func TestGenerate_truncatesLargePatch(t *testing.T) {
	t.Parallel()
	api, gen := newFakeAPI(t, http.StatusOK, messageResponse("chore: update data", "end_turn"))
	large := samplePatch + strings.Repeat("+line of data\n", 30_000)

	if _, err := gen.Generate(t.Context(), large); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	user := api.request(t).userText()
	if len(user) >= len(large) {
		t.Errorf("user message length = %d, want less than patch length %d", len(user), len(large))
	}
	if !strings.Contains(user, "truncated") {
		t.Errorf("user message does not mention truncation")
	}
	if !strings.Contains(user, samplePatch) {
		t.Errorf("user message does not keep the start of the patch")
	}
}

func TestGenerate_refusal(t *testing.T) {
	t.Parallel()
	_, gen := newFakeAPI(t, http.StatusOK, messageResponse("", "refusal"))

	_, err := gen.Generate(t.Context(), samplePatch)
	if !errors.Is(err, llm.ErrRefused) {
		t.Errorf("Generate() error = %v, want %v", err, llm.ErrRefused)
	}
}

func TestGenerate_maxTokens(t *testing.T) {
	t.Parallel()
	_, gen := newFakeAPI(t, http.StatusOK, messageResponse("feat(tui): add", "max_tokens"))

	_, err := gen.Generate(t.Context(), samplePatch)
	if !errors.Is(err, llm.ErrTruncated) {
		t.Errorf("Generate() error = %v, want %v", err, llm.ErrTruncated)
	}
}

func TestGenerate_emptyResponse(t *testing.T) {
	t.Parallel()
	_, gen := newFakeAPI(t, http.StatusOK, messageResponse("  ", "end_turn"))

	_, err := gen.Generate(t.Context(), samplePatch)
	if !errors.Is(err, llm.ErrEmptyResponse) {
		t.Errorf("Generate() error = %v, want %v", err, llm.ErrEmptyResponse)
	}
}

func TestGenerate_unauthorized(t *testing.T) {
	t.Parallel()
	body := `{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key"}}`
	_, gen := newFakeAPI(t, http.StatusUnauthorized, body)

	_, err := gen.Generate(t.Context(), samplePatch)
	if err == nil {
		t.Fatal("Generate() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "API key") {
		t.Errorf("Generate() error = %q, want it to mention the API key", err)
	}
}

func TestGenerate_rateLimited(t *testing.T) {
	t.Parallel()
	body := `{"type":"error","error":{"type":"rate_limit_error","message":"slow down"}}`
	_, gen := newFakeAPI(t, http.StatusTooManyRequests, body)

	_, err := gen.Generate(t.Context(), samplePatch)
	if err == nil || !strings.Contains(err.Error(), "rate limit") {
		t.Errorf("Generate() error = %v, want it to mention the rate limit", err)
	}
}

func TestGenerate_emptyPatch(t *testing.T) {
	t.Parallel()
	api, gen := newFakeAPI(t, http.StatusOK, messageResponse("unused", "end_turn"))

	for _, patch := range []string{"", " \n\t"} {
		_, err := gen.Generate(t.Context(), patch)
		if !errors.Is(err, llm.ErrEmptyPatch) {
			t.Errorf("Generate(%q) error = %v, want %v", patch, err, llm.ErrEmptyPatch)
		}
	}
	if n := api.calls.Load(); n != 0 {
		t.Errorf("API calls = %d, want 0", n)
	}
}
