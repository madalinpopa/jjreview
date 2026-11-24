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

	"github.com/openai/openai-go/v3/option"

	"github.com/madalinpopa/jjreview/internal/llm"
	"github.com/madalinpopa/jjreview/internal/prompt"
)

type openAIRequest struct {
	Model           string         `json:"model"`
	Instructions    string         `json:"instructions"`
	Input           string         `json:"input"`
	MaxOutputTokens int64          `json:"max_output_tokens"`
	Store           *bool          `json:"store"`
	Reasoning       jsontext.Value `json:"reasoning"`
	Text            jsontext.Value `json:"text"`
}

type openAIMessage struct {
	phase string
	parts []map[string]any
}

func openAIText(text string) map[string]any {
	return map[string]any{"type": "output_text", "text": text, "annotations": []any{}}
}

func openAIRefusal(text string) map[string]any {
	return map[string]any{"type": "refusal", "refusal": text}
}

func openAIResponse(status, incompleteReason string, msgs ...openAIMessage) string {
	output := make([]map[string]any, 0, len(msgs))
	for _, m := range msgs {
		item := map[string]any{
			"id":      "msg_test",
			"type":    "message",
			"role":    "assistant",
			"status":  "completed",
			"content": m.parts,
		}
		if m.phase != "" {
			item["phase"] = m.phase
		}
		output = append(output, item)
	}
	resp := map[string]any{
		"id":         "resp_test",
		"object":     "response",
		"created_at": 0,
		"model":      llm.DefaultOpenAIModel,
		"status":     status,
		"output":     output,
	}
	if incompleteReason != "" {
		resp["incomplete_details"] = map[string]any{"reason": incompleteReason}
	}
	if status == "failed" {
		resp["error"] = map[string]any{"code": "server_error", "message": "upstream exploded"}
	}
	body, err := json.Marshal(resp)
	if err != nil {
		panic(err)
	}
	return string(body)
}

func openAICompleted(parts ...map[string]any) string {
	return openAIResponse("completed", "", openAIMessage{parts: parts})
}

type fakeOpenAI struct {
	t        *testing.T
	status   int
	body     string
	calls    atomic.Int32
	captured chan openAIRequest
}

func newFakeOpenAI(t *testing.T, model string, status int, body string) (*fakeOpenAI, llm.OpenAI) {
	t.Helper()
	api := &fakeOpenAI{t: t, status: status, body: body, captured: make(chan openAIRequest, 1)}
	server := httptest.NewServer(api)
	t.Cleanup(server.Close)
	gen := llm.NewOpenAI(model,
		option.WithBaseURL(server.URL),
		option.WithAPIKey("test-key"),
		option.WithMaxRetries(0),
	)
	return api, gen
}

func (f *fakeOpenAI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.calls.Add(1)
	if r.Method != http.MethodPost || r.URL.Path != "/responses" {
		f.t.Errorf("request = %s %s, want POST /responses", r.Method, r.URL.Path)
		http.NotFound(w, r)
		return
	}
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		f.t.Errorf("read body: %v", err)
	}
	var req openAIRequest
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

func (f *fakeOpenAI) request(t *testing.T) openAIRequest {
	t.Helper()
	select {
	case req := <-f.captured:
		return req
	default:
		t.Fatal("no request captured")
		return openAIRequest{}
	}
}

func TestOpenAIGenerate_success(t *testing.T) {
	t.Parallel()
	api, gen := newFakeOpenAI(t, "", http.StatusOK, openAICompleted(openAIText("  feat(main): rename package\n")))

	got, err := gen.Generate(t.Context(), samplePatch)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if want := "feat(main): rename package"; got != want {
		t.Errorf("Generate() = %q, want %q", got, want)
	}

	req := api.request(t)
	if req.Model != "gpt-6-luna" {
		t.Errorf("model = %q, want %q", req.Model, "gpt-6-luna")
	}
	if req.Instructions != prompt.System {
		t.Errorf("instructions = %q, want prompt.System", req.Instructions)
	}
	if !strings.Contains(req.Input, samplePatch) {
		t.Errorf("input does not contain the patch: %q", req.Input)
	}
	if req.Store == nil || *req.Store {
		t.Errorf("store = %v, want false", req.Store)
	}
	if req.MaxOutputTokens <= 0 {
		t.Errorf("max_output_tokens = %d, want positive", req.MaxOutputTokens)
	}
	var reasoning struct {
		Effort string `json:"effort"`
	}
	if err := json.Unmarshal(req.Reasoning, &reasoning); err != nil || reasoning.Effort != "low" {
		t.Errorf("reasoning = %s, want effort low", req.Reasoning)
	}
	if req.Text != nil {
		t.Errorf("text = %s, want absent", req.Text)
	}
}

func TestOpenAIGenerate_customModel(t *testing.T) {
	t.Parallel()
	api, gen := newFakeOpenAI(t, "gpt-4.1-mini", http.StatusOK, openAICompleted(openAIText("feat: x")))

	if _, err := gen.Generate(t.Context(), samplePatch); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	req := api.request(t)
	if req.Model != "gpt-4.1-mini" {
		t.Errorf("model = %q, want %q", req.Model, "gpt-4.1-mini")
	}
	if req.Reasoning != nil {
		t.Errorf("reasoning = %s, want absent for a custom model", req.Reasoning)
	}
}

func TestOpenAIGenerate_output(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		body string
		want string
	}{
		"strips a code fence": {
			body: openAICompleted(openAIText("```text\nfix(diff): handle empty hunks\n\n- skip blank lines\n```\n")),
			want: "fix(diff): handle empty hunks\n\n- skip blank lines",
		},
		"joins text parts": {
			body: openAICompleted(openAIText("fix(diff): "), openAIText("handle empty hunks")),
			want: "fix(diff): handle empty hunks",
		},
		"skips commentary messages": {
			body: openAIResponse("completed", "",
				openAIMessage{phase: "commentary", parts: []map[string]any{openAIText("thinking out loud")}},
				openAIMessage{phase: "final_answer", parts: []map[string]any{openAIText("feat: final")}},
			),
			want: "feat: final",
		},
		"accepts a response without a status": {
			body: openAIResponse("", "", openAIMessage{parts: []map[string]any{openAIText("feat: no status")}}),
			want: "feat: no status",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, gen := newFakeOpenAI(t, "", http.StatusOK, tt.body)

			got, err := gen.Generate(t.Context(), samplePatch)
			if err != nil {
				t.Fatalf("Generate() error = %v", err)
			}

			if got != tt.want {
				t.Errorf("Generate() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestOpenAIGenerate_failed(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		status   int
		body     string
		wantErr  error
		contains string
	}{
		"refusal": {
			status:  http.StatusOK,
			body:    openAICompleted(openAIRefusal("I cannot help with that")),
			wantErr: llm.ErrRefused,
		},
		"max output tokens": {
			status:  http.StatusOK,
			body:    openAIResponse("incomplete", "max_output_tokens", openAIMessage{parts: []map[string]any{openAIText("feat(tui): add")}}),
			wantErr: llm.ErrTruncated,
		},
		"other incomplete reason": {
			status:   http.StatusOK,
			body:     openAIResponse("incomplete", "content_filter"),
			contains: "content_filter",
		},
		"failed response": {
			status:   http.StatusOK,
			body:     openAIResponse("failed", ""),
			contains: "upstream exploded",
		},
		"no text": {
			status:  http.StatusOK,
			body:    openAICompleted(openAIText("  ")),
			wantErr: llm.ErrEmptyResponse,
		},
		"only commentary": {
			status:  http.StatusOK,
			body:    openAIResponse("completed", "", openAIMessage{phase: "commentary", parts: []map[string]any{openAIText("hmm")}}),
			wantErr: llm.ErrEmptyResponse,
		},
		"unauthorized": {
			status:   http.StatusUnauthorized,
			body:     `{"error":{"message":"Incorrect API key provided","type":"invalid_request_error","code":"invalid_api_key"}}`,
			contains: "OPENAI_API_KEY",
		},
		"rate limited": {
			status:   http.StatusTooManyRequests,
			body:     `{"error":{"message":"slow down","type":"requests","code":"rate_limit_exceeded"}}`,
			contains: "rate limit",
		},
		"quota exhausted": {
			status:   http.StatusTooManyRequests,
			body:     `{"error":{"message":"You exceeded your current quota","type":"insufficient_quota","code":"insufficient_quota"}}`,
			contains: "quota or billing limit",
		},
		"server error": {
			status:   http.StatusBadRequest,
			body:     `{"error":{"message":"bad","type":"invalid_request_error"}}`,
			contains: "status 400",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, gen := newFakeOpenAI(t, "", tt.status, tt.body)

			_, err := gen.Generate(t.Context(), samplePatch)

			if err == nil {
				t.Fatal("Generate() error = nil, want error")
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Errorf("Generate() error = %v, want %v", err, tt.wantErr)
			}
			if tt.contains != "" && !strings.Contains(err.Error(), tt.contains) {
				t.Errorf("Generate() error = %q, want it to contain %q", err, tt.contains)
			}
		})
	}
}

func TestOpenAIGenerate_emptyPatch(t *testing.T) {
	t.Parallel()
	api, gen := newFakeOpenAI(t, "", http.StatusOK, openAICompleted(openAIText("unused")))

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
