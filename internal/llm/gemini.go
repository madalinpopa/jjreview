package llm

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strings"

	"google.golang.org/genai"

	"github.com/madalinpopa/jjreview/internal/prompt"
)

const DefaultGeminiModel = "gemini-3.5-flash-lite"

var errGeminiTimeout = fmt.Errorf("the Gemini API did not respond within %s", generateTimeout)

var errGeminiMissingKey = errors.New("the Gemini API key is missing; set providers.gemini.api_key in the config file or set GEMINI_API_KEY (GOOGLE_API_KEY takes precedence)")

var _ Provider = (*Gemini)(nil)

type Gemini struct {
	client *genai.Client
	model  string
	err    error
}

func NewGemini(ctx context.Context, model, apiKey string, httpOpts genai.HTTPOptions) *Gemini {
	if httpOpts.RetryOptions == nil {
		httpOpts.RetryOptions = &genai.HTTPRetryOptions{
			Attempts:        new(int32(3)),
			HTTPStatusCodes: []int32{http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout},
		}
	}
	model = cmp.Or(model, DefaultGeminiModel)
	if cmp.Or(apiKey, os.Getenv("GOOGLE_API_KEY"), os.Getenv("GEMINI_API_KEY")) == "" {
		return &Gemini{model: model, err: errGeminiMissingKey}
	}
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:      apiKey,
		Backend:     genai.BackendGeminiAPI,
		HTTPOptions: httpOpts,
	})
	if err != nil {
		err = fmt.Errorf("create Gemini client: %w", err)
	}
	return &Gemini{client: client, model: model, err: err}
}

func (g *Gemini) Generate(ctx context.Context, patch string) (string, error) {
	if strings.TrimSpace(patch) == "" {
		return "", ErrEmptyPatch
	}
	if g.err != nil {
		return "", fmt.Errorf("generate commit message: %w", g.err)
	}
	ctx, cancel := context.WithTimeoutCause(ctx, generateTimeout, errGeminiTimeout)
	defer cancel()

	config := &genai.GenerateContentConfig{
		SystemInstruction: genai.NewContentFromText(prompt.System, genai.RoleUser),
		MaxOutputTokens:   maxTokens,
	}
	if g.model == DefaultGeminiModel {
		config.ThinkingConfig = &genai.ThinkingConfig{ThinkingLevel: genai.ThinkingLevelLow}
	}
	resp, err := g.client.Models.GenerateContent(ctx, g.model, genai.Text(prompt.Build(patch)), config)
	if err != nil {
		if cause := context.Cause(ctx); cause != nil {
			return "", fmt.Errorf("generate commit message: %w", cause)
		}
		return "", describeGemini(err)
	}
	return geminiText(resp)
}

func geminiText(resp *genai.GenerateContentResponse) (string, error) {
	if fb := resp.PromptFeedback; fb != nil && fb.BlockReason != "" {
		return "", fmt.Errorf("%w: prompt blocked: %s", ErrRefused, fb.BlockReason)
	}
	if len(resp.Candidates) == 0 {
		return "", ErrEmptyResponse
	}
	switch reason := resp.Candidates[0].FinishReason; reason {
	case genai.FinishReasonStop, genai.FinishReasonUnspecified, "":
	case genai.FinishReasonMaxTokens:
		return "", ErrTruncated
	case genai.FinishReasonSafety, genai.FinishReasonProhibitedContent, genai.FinishReasonBlocklist,
		genai.FinishReasonSPII, genai.FinishReasonRecitation:
		return "", fmt.Errorf("%w: response blocked: %s", ErrRefused, reason)
	default:
		return "", fmt.Errorf("generate commit message: Gemini stopped early: %s", reason)
	}
	var text strings.Builder
	if content := resp.Candidates[0].Content; content != nil {
		for _, part := range content.Parts {
			if !part.Thought {
				text.WriteString(part.Text)
			}
		}
	}
	msg := clean(text.String())
	if msg == "" {
		return "", ErrEmptyResponse
	}
	return msg, nil
}

func describeGemini(err error) error {
	apiErr, ok := errors.AsType[genai.APIError](err)
	if !ok {
		return fmt.Errorf("generate commit message: %w", err)
	}
	switch {
	case apiErr.Code == http.StatusUnauthorized, apiErr.Code == http.StatusBadRequest && invalidGeminiKey(apiErr):
		return fmt.Errorf("generate commit message: Gemini API key is missing or invalid; set providers.gemini.api_key in the config file or set GEMINI_API_KEY (GOOGLE_API_KEY takes precedence): %w", err)
	case apiErr.Code == http.StatusForbidden:
		return fmt.Errorf("generate commit message: Gemini API permission denied; check that the API key is enabled and allowed to use the Gemini API and model: %w", err)
	case apiErr.Code == http.StatusTooManyRequests:
		return fmt.Errorf("generate commit message: Gemini API rate limit reached, try again later: %w", err)
	default:
		return fmt.Errorf("generate commit message: Gemini API error (status %d): %w", apiErr.Code, err)
	}
}

func invalidGeminiKey(apiErr genai.APIError) bool {
	return slices.ContainsFunc(apiErr.Details, func(detail map[string]any) bool {
		return detail["reason"] == "API_KEY_INVALID"
	})
}
