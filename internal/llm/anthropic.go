package llm

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/anthropics/anthropic-sdk-go/v2/option"

	"github.com/madalinpopa/jjreview/internal/prompt"
)

const (
	DefaultAnthropicModel = "claude-haiku-5-5"
	maxTokens             = 16000
	fence                 = "```"
)

var (
	ErrEmptyPatch    = errors.New("commit message: empty patch")
	ErrRefused       = errors.New("commit message: model refused the request")
	ErrTruncated     = errors.New("commit message: response hit the token limit")
	ErrEmptyResponse = errors.New("commit message: model returned no text")
)

var _ Provider = Anthropic{}

type Anthropic struct {
	client anthropic.Client
	model  string
}

func NewAnthropic(model string, opts ...option.RequestOption) Anthropic {
	if model == "" {
		model = DefaultAnthropicModel
	}
	return Anthropic{client: anthropic.NewClient(opts...), model: model}
}

func (g Anthropic) Generate(ctx context.Context, patch string) (string, error) {
	if strings.TrimSpace(patch) == "" {
		return "", ErrEmptyPatch
	}
	resp, err := g.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     g.model,
		MaxTokens: maxTokens,
		System:    []anthropic.TextBlockParam{{Text: prompt.System}},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(prompt.Build(patch))),
		},
	})
	if err != nil {
		return "", describe(err)
	}
	switch resp.StopReason {
	case anthropic.StopReasonRefusal:
		return "", ErrRefused
	case anthropic.StopReasonMaxTokens:
		return "", ErrTruncated
	}
	var text strings.Builder
	for _, block := range resp.Content {
		if b, ok := block.AsAny().(anthropic.TextBlock); ok {
			text.WriteString(b.Text)
		}
	}
	msg := clean(text.String())
	if msg == "" {
		return "", ErrEmptyResponse
	}
	return msg, nil
}

func clean(text string) string {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, fence) {
		return text
	}
	_, body, found := strings.Cut(text, "\n")
	if !found {
		return ""
	}
	body = strings.TrimSpace(body)
	body = strings.TrimSuffix(body, fence)
	return strings.TrimSpace(body)
}

func describe(err error) error {
	apiErr, ok := errors.AsType[*anthropic.Error](err)
	if !ok {
		return fmt.Errorf("generate commit message: %w", err)
	}
	switch apiErr.StatusCode {
	case http.StatusUnauthorized:
		return fmt.Errorf("generate commit message: Anthropic API key is missing or invalid; set providers.anthropic.api_key in the config file, set ANTHROPIC_API_KEY or run 'ant auth login': %w", err)
	case http.StatusTooManyRequests:
		return fmt.Errorf("generate commit message: Anthropic API rate limit reached, try again later: %w", err)
	default:
		return fmt.Errorf("generate commit message: Anthropic API error (status %d): %w", apiErr.StatusCode, err)
	}
}
