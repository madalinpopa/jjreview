package llm_test

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"google.golang.org/genai"

	"github.com/madalinpopa/jjreview/internal/llm"
	"github.com/madalinpopa/jjreview/internal/prompt"
)

type geminiPart struct {
	Text    string `json:"text"`
	Thought bool   `json:"thought"`
}

type geminiContent struct {
	Role  string       `json:"role"`
	Parts []geminiPart `json:"parts"`
}

type geminiRequest struct {
	Path              string
	Contents          []geminiContent `json:"contents"`
	SystemInstruction geminiContent   `json:"systemInstruction"`
	GenerationConfig  struct {
		MaxOutputTokens int            `json:"maxOutputTokens"`
		ThinkingConfig  jsontext.Value `json:"thinkingConfig"`
	} `json:"generationConfig"`
}

func (r geminiRequest) userText() string {
	var b strings.Builder
	for _, c := range r.Contents {
		if c.Role != genai.RoleUser {
			continue
		}
		for _, p := range c.Parts {
			b.WriteString(p.Text)
		}
	}
	return b.String()
}

func geminiResponse(finishReason string, parts ...geminiPart) string {
	body, err := json.Marshal(map[string]any{
		"candidates": []map[string]any{{
			"content":      geminiContent{Role: "model", Parts: parts},
			"finishReason": finishReason,
		}},
	})
	if err != nil {
		panic(err)
	}
	return string(body)
}

type fakeGemini struct {
	t        *testing.T
	status   int
	body     string
	calls    atomic.Int32
	captured chan geminiRequest
}

func newFakeGemini(t *testing.T, model string, status int, body string) (*fakeGemini, *llm.Gemini) {
	t.Helper()
	api := &fakeGemini{t: t, status: status, body: body, captured: make(chan geminiRequest, 1)}
	server := httptest.NewServer(api)
	t.Cleanup(server.Close)
	gen := llm.NewGemini(t.Context(), model, "test-key", genai.HTTPOptions{
		BaseURL:      server.URL,
		RetryOptions: &genai.HTTPRetryOptions{Attempts: new(int32(1))},
	})
	return api, gen
}

func (f *fakeGemini) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.calls.Add(1)
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		f.t.Errorf("read body: %v", err)
	}
	var req geminiRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		f.t.Errorf("decode body: %v", err)
	}
	req.Path = r.Method + " " + r.URL.Path
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

func (f *fakeGemini) request(t *testing.T) geminiRequest {
	t.Helper()
	select {
	case req := <-f.captured:
		return req
	default:
		t.Fatal("no request captured")
		return geminiRequest{}
	}
}

func TestGeminiGenerate_success(t *testing.T) {
	t.Parallel()
	api, gen := newFakeGemini(t, "", http.StatusOK, geminiResponse("STOP", geminiPart{Text: "  feat(main): rename package\n"}))

	got, err := gen.Generate(t.Context(), samplePatch)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if want := "feat(main): rename package"; got != want {
		t.Errorf("Generate() = %q, want %q", got, want)
	}

	req := api.request(t)
	if want := "POST /v1beta/models/gemini-3.5-flash-lite:generateContent"; req.Path != want {
		t.Errorf("request = %q, want %q", req.Path, want)
	}
	if len(req.SystemInstruction.Parts) != 1 || req.SystemInstruction.Parts[0].Text != prompt.System {
		t.Errorf("systemInstruction = %+v, want the system prompt", req.SystemInstruction)
	}
	if user := req.userText(); !strings.Contains(user, samplePatch) {
		t.Errorf("user content does not contain the patch: %q", user)
	}
	if req.GenerationConfig.MaxOutputTokens <= 0 {
		t.Errorf("maxOutputTokens = %d, want positive", req.GenerationConfig.MaxOutputTokens)
	}
	if !strings.Contains(string(req.GenerationConfig.ThinkingConfig), `"LOW"`) {
		t.Errorf("thinkingConfig = %s, want a low thinking level", req.GenerationConfig.ThinkingConfig)
	}
}

func TestGeminiGenerate_customModel(t *testing.T) {
	t.Parallel()
	api, gen := newFakeGemini(t, "gemini-3.6-pro", http.StatusOK, geminiResponse("STOP", geminiPart{Text: "feat: x"}))

	if _, err := gen.Generate(t.Context(), samplePatch); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	req := api.request(t)
	if want := "POST /v1beta/models/gemini-3.6-pro:generateContent"; req.Path != want {
		t.Errorf("request = %q, want %q", req.Path, want)
	}
	if req.GenerationConfig.ThinkingConfig != nil {
		t.Errorf("thinkingConfig = %s, want absent for a custom model", req.GenerationConfig.ThinkingConfig)
	}
}

func TestGeminiGenerate_skipsThoughts(t *testing.T) {
	t.Parallel()
	_, gen := newFakeGemini(t, "", http.StatusOK, geminiResponse("STOP",
		geminiPart{Text: "let me think", Thought: true},
		geminiPart{Text: "fix(diff): handle empty hunks"},
	))

	got, err := gen.Generate(t.Context(), samplePatch)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if want := "fix(diff): handle empty hunks"; got != want {
		t.Errorf("Generate() = %q, want %q", got, want)
	}
}

func TestGeminiGenerate_stripsCodeFence(t *testing.T) {
	t.Parallel()
	fenced := "```text\nfix(diff): handle empty hunks\n\n- skip blank lines\n```\n"
	_, gen := newFakeGemini(t, "", http.StatusOK, geminiResponse("STOP", geminiPart{Text: fenced}))

	got, err := gen.Generate(t.Context(), samplePatch)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if want := "fix(diff): handle empty hunks\n\n- skip blank lines"; got != want {
		t.Errorf("Generate() = %q, want %q", got, want)
	}
}

func TestGeminiGenerate_failed(t *testing.T) {
	t.Parallel()
	keyInvalid := `{"error":{"code":400,"message":"API key not valid.","status":"INVALID_ARGUMENT","details":[{"@type":"type.googleapis.com/google.rpc.ErrorInfo","reason":"API_KEY_INVALID","domain":"googleapis.com"}]}}`
	tests := map[string]struct {
		status   int
		body     string
		wantErr  error
		wantText string
	}{
		"blocked prompt": {
			status:   http.StatusOK,
			body:     `{"promptFeedback":{"blockReason":"SAFETY"}}`,
			wantErr:  llm.ErrRefused,
			wantText: "SAFETY",
		},
		"no candidates": {
			status:  http.StatusOK,
			body:    `{}`,
			wantErr: llm.ErrEmptyResponse,
		},
		"max tokens": {
			status:  http.StatusOK,
			body:    geminiResponse("MAX_TOKENS", geminiPart{Text: "feat(tui): add"}),
			wantErr: llm.ErrTruncated,
		},
		"safety": {
			status:   http.StatusOK,
			body:     geminiResponse("SAFETY"),
			wantErr:  llm.ErrRefused,
			wantText: "SAFETY",
		},
		"prohibited content": {
			status:   http.StatusOK,
			body:     geminiResponse("PROHIBITED_CONTENT"),
			wantErr:  llm.ErrRefused,
			wantText: "PROHIBITED_CONTENT",
		},
		"other finish reason": {
			status:   http.StatusOK,
			body:     geminiResponse("MALFORMED_FUNCTION_CALL", geminiPart{Text: "feat: x"}),
			wantText: "MALFORMED_FUNCTION_CALL",
		},
		"empty text": {
			status:  http.StatusOK,
			body:    geminiResponse("STOP", geminiPart{Text: "  "}),
			wantErr: llm.ErrEmptyResponse,
		},
		"invalid API key": {
			status:   http.StatusBadRequest,
			body:     keyInvalid,
			wantText: "GOOGLE_API_KEY takes precedence",
		},
		"bad request": {
			status:   http.StatusBadRequest,
			body:     `{"error":{"code":400,"message":"bad","status":"INVALID_ARGUMENT"}}`,
			wantText: "status 400",
		},
		"permission denied": {
			status:   http.StatusForbidden,
			body:     `{"error":{"code":403,"message":"denied","status":"PERMISSION_DENIED"}}`,
			wantText: "check that the API key is enabled and allowed to use the Gemini API and model",
		},
		"rate limited": {
			status:   http.StatusTooManyRequests,
			body:     `{"error":{"code":429,"message":"slow down","status":"RESOURCE_EXHAUSTED"}}`,
			wantText: "rate limit",
		},
		"server error": {
			status:   http.StatusInternalServerError,
			body:     `{"error":{"code":500,"message":"boom","status":"INTERNAL"}}`,
			wantText: "status 500",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, gen := newFakeGemini(t, "", tt.status, tt.body)

			_, err := gen.Generate(t.Context(), samplePatch)

			if err == nil {
				t.Fatal("Generate() error = nil, want error")
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Errorf("Generate() error = %v, want %v", err, tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantText) {
				t.Errorf("Generate() error = %q, want it to contain %q", err, tt.wantText)
			}
		})
	}
}

func TestGeminiGenerate_emptyPatch(t *testing.T) {
	t.Parallel()
	api, gen := newFakeGemini(t, "", http.StatusOK, geminiResponse("STOP", geminiPart{Text: "unused"}))

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

func TestGeminiGenerate_missingKey(t *testing.T) {
	t.Setenv("GOOGLE_API_KEY", "")
	t.Setenv("GEMINI_API_KEY", "")
	api := &fakeGemini{t: t, status: http.StatusOK, body: geminiResponse("STOP", geminiPart{Text: "unused"}), captured: make(chan geminiRequest, 1)}
	server := httptest.NewServer(api)
	t.Cleanup(server.Close)
	gen := llm.NewGemini(t.Context(), "", "", genai.HTTPOptions{BaseURL: server.URL})

	_, err := gen.Generate(t.Context(), samplePatch)

	if err == nil || !strings.Contains(err.Error(), "GOOGLE_API_KEY takes precedence") {
		t.Errorf("Generate() error = %v, want a missing API key hint", err)
	}
	if err != nil && strings.Contains(err.Error(), "ClientConfig") {
		t.Errorf("Generate() error = %q, want no ClientConfig dump", err)
	}
	if n := api.calls.Load(); n != 0 {
		t.Errorf("API calls = %d, want 0", n)
	}
}

type sequenceGemini struct {
	t        *testing.T
	statuses []int
	calls    atomic.Int32
}

func (f *sequenceGemini) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	call := int(f.calls.Add(1))
	if _, err := io.Copy(io.Discard, r.Body); err != nil {
		f.t.Errorf("read body: %v", err)
	}
	status := f.statuses[min(call, len(f.statuses))-1]
	body := geminiResponse("STOP", geminiPart{Text: "feat: retried"})
	if status != http.StatusOK {
		body = fmt.Sprintf(`{"error":{"code":%d,"message":"try again","status":"UNAVAILABLE"}}`, status)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if _, err := io.WriteString(w, body); err != nil {
		f.t.Errorf("write response: %v", err)
	}
}

func TestGeminiGenerate_retries(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		statuses  []int
		wantCalls int32
		wantErr   string
	}{
		"retries a server error": {
			statuses:  []int{http.StatusServiceUnavailable, http.StatusOK},
			wantCalls: 2,
		},
		"does not retry a rate limit": {
			statuses:  []int{http.StatusTooManyRequests, http.StatusOK},
			wantCalls: 1,
			wantErr:   "rate limit",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			api := &sequenceGemini{t: t, statuses: tt.statuses}
			server := httptest.NewServer(api)
			t.Cleanup(server.Close)
			gen := llm.NewGemini(t.Context(), "", "test-key", genai.HTTPOptions{BaseURL: server.URL})

			got, err := gen.Generate(t.Context(), samplePatch)

			if tt.wantErr == "" && (err != nil || got != "feat: retried") {
				t.Errorf("Generate() = %q, %v, want %q", got, err, "feat: retried")
			}
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Errorf("Generate() error = %v, want it to contain %q", err, tt.wantErr)
			}
			if n := api.calls.Load(); n != tt.wantCalls {
				t.Errorf("API calls = %d, want %d", n, tt.wantCalls)
			}
		})
	}
}
