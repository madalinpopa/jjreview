package llm

import "context"

type Provider interface {
	Generate(ctx context.Context, patch string) (string, error)
}
