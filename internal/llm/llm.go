package llm

import (
	"context"
	"errors"
	"strings"
	"time"
)

const (
	maxTokens       = 16000
	generateTimeout = 2 * time.Minute
	fence           = "```"
)

var (
	ErrEmptyPatch    = errors.New("commit message: empty patch")
	ErrRefused       = errors.New("commit message: model refused the request")
	ErrTruncated     = errors.New("commit message: response hit the token limit")
	ErrEmptyResponse = errors.New("commit message: model returned no text")
)

type Provider interface {
	Generate(ctx context.Context, patch string) (string, error)
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
