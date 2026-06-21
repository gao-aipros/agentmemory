package service

import (
	"context"
	"fmt"
)

// LLMService wraps the langchaingo LLM interface for provider-agnostic
// LLM calls. In production this uses langchaingo; for testing, a mock
// satisfies the same interface.
type LLMService struct {
	// provider is the LLM backend (e.g., langchaingo/llms.Model).
	// In MVP, this is configured via env vars (LLM_PROVIDER, LLM_MODEL).
	// When nil, Call returns a descriptive error.
	provider LLMProvider
}

// LLMProvider abstracts the LLM backend so we stay provider-agnostic.
type LLMProvider interface {
	Call(ctx context.Context, prompt string) (string, error)
}

// NewLLMService creates a new LLMService with the given provider.
// If provider is nil, calls to Call will return an error.
func NewLLMService(provider LLMProvider) *LLMService {
	return &LLMService{provider: provider}
}

// Call sends a prompt to the LLM and returns the response text.
func (s *LLMService) Call(ctx context.Context, prompt string) (string, error) {
	if s.provider == nil {
		return "", fmt.Errorf("LLM service not configured — set LLM_PROVIDER and LLM_MODEL env vars")
	}
	return s.provider.Call(ctx, prompt)
}
