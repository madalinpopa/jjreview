package llm

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"

	"github.com/madalinpopa/jjreview/internal/prompt"
)

const DefaultOpenAIModel = openai.ChatModelGPT6Luna

var errOpenAITimeout = fmt.Errorf("OpenAI did not respond within %s", generateTimeout)

var _ Provider = OpenAI{}

type OpenAI struct {
	client openai.Client
	model  string
}

func NewOpenAI(model string, opts ...option.RequestOption) OpenAI {
	return OpenAI{client: openai.NewClient(opts...), model: cmp.Or(model, DefaultOpenAIModel)}
}

func (g OpenAI) Generate(ctx context.Context, patch string) (string, error) {
	if strings.TrimSpace(patch) == "" {
		return "", ErrEmptyPatch
	}
	ctx, cancel := context.WithTimeoutCause(ctx, generateTimeout, errOpenAITimeout)
	defer cancel()

	params := responses.ResponseNewParams{
		Model:           g.model,
		Instructions:    openai.String(prompt.System),
		Input:           responses.ResponseNewParamsInputUnion{OfString: openai.String(prompt.Build(patch))},
		MaxOutputTokens: openai.Int(maxTokens),
		Store:           openai.Bool(false),
	}
	if g.model == DefaultOpenAIModel {
		params.Reasoning = shared.ReasoningParam{Effort: shared.ReasoningEffortLow}
	}
	resp, err := g.client.Responses.New(ctx, params)
	if err != nil {
		if cause := context.Cause(ctx); cause != nil {
			return "", fmt.Errorf("generate commit message: %w", cause)
		}
		return "", describeOpenAI(err)
	}

	if err := openAIStatus(resp); err != nil {
		return "", err
	}
	return openAIText(resp)
}

func openAIStatus(resp *responses.Response) error {
	switch resp.Status {
	case responses.ResponseStatusCompleted, "":
		return nil
	case responses.ResponseStatusIncomplete:
		if resp.IncompleteDetails.Reason == "max_output_tokens" {
			return ErrTruncated
		}
		return fmt.Errorf("generate commit message: OpenAI response is incomplete: %s", resp.IncompleteDetails.Reason)
	default:
		if resp.Error.Message != "" {
			return fmt.Errorf("generate commit message: OpenAI response %s: %s: %s", resp.Status, resp.Error.Code, resp.Error.Message)
		}
		return fmt.Errorf("generate commit message: OpenAI response %s", resp.Status)
	}
}

func openAIText(resp *responses.Response) (string, error) {
	var text strings.Builder
	for _, item := range resp.Output {
		if item.Type != "message" || (item.Phase != "" && item.Phase != responses.ResponseOutputMessagePhaseFinalAnswer) {
			continue
		}
		for _, part := range item.Content {
			switch part.Type {
			case "refusal":
				return "", fmt.Errorf("%w: %s", ErrRefused, part.Refusal)
			case "output_text":
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

func describeOpenAI(err error) error {
	apiErr, ok := errors.AsType[*openai.Error](err)
	if !ok {
		return fmt.Errorf("generate commit message: %w", err)
	}
	switch apiErr.StatusCode {
	case http.StatusUnauthorized:
		return fmt.Errorf("generate commit message: OpenAI API key is missing or invalid; set providers.openai.api_key in the config file or set OPENAI_API_KEY: %w", err)
	case http.StatusTooManyRequests:
		if apiErr.Type == "insufficient_quota" {
			return fmt.Errorf("generate commit message: OpenAI API quota or billing limit reached; check your plan and billing: %w", err)
		}
		return fmt.Errorf("generate commit message: OpenAI API rate limit reached, try again later: %w", err)
	default:
		return fmt.Errorf("generate commit message: OpenAI API error (status %d): %w", apiErr.StatusCode, err)
	}
}
