package service

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/anthropic"
	"github.com/tmc/langchaingo/llms/openai"
)

// LLMService wraps a langchaingo llms.Model, providing provider-agnostic
// LLM calls. The model is selected via the LLM_PROVIDER environment variable.
// LLM_MODEL controls the model name.
// The API key is resolved with this priority:
//  1. LLM_API_KEY (unified, provider-agnostic)
//  2. OPENAI_API_KEY (openai provider) or ANTHROPIC_API_KEY (anthropic provider)
//
// LLM_BASE_URL sets a custom base URL for either provider.
type LLMService struct {
	model llms.Model
}

// NewLLMService creates an LLMService from environment variables.
// LLM_PROVIDER: "openai" (default) or "anthropic".
// LLM_MODEL: the model name (provider-specific default if unset).
// LLM_API_KEY: unified API key for any provider (takes priority).
// OPENAI_API_KEY: fallback when LLM_PROVIDER=openai.
// ANTHROPIC_API_KEY: fallback when LLM_PROVIDER=anthropic.
// LLM_BASE_URL: custom base URL for any provider (optional).
func NewLLMService() (*LLMService, error) {
	provider := strings.ToLower(os.Getenv("LLM_PROVIDER"))
	if provider == "" {
		provider = "openai"
	}

	switch provider {
	case "openai":
		return newOpenAILLM()
	case "anthropic":
		return newAnthropicLLM()
	default:
		return nil, fmt.Errorf("unsupported LLM_PROVIDER %q: must be \"openai\" or \"anthropic\"", provider)
	}
}

// NewLLMServiceWithModel creates an LLMService with a pre-built model.
// Used in tests to inject mock models without requiring real API keys.
func NewLLMServiceWithModel(model llms.Model) *LLMService {
	return &LLMService{model: model}
}

// Model returns the underlying langchaingo llms.Model (for testing/assertions).
func (s *LLMService) Model() llms.Model {
	return s.model
}

// Call sends a prompt to the LLM and returns the response text.
func (s *LLMService) Call(ctx context.Context, prompt string) (string, error) {
	if s.model == nil {
		return "", fmt.Errorf("LLM service not configured — set LLM_PROVIDER and required API key env vars")
	}
	return s.model.Call(ctx, prompt)
}

// resolveLLMAPIKey returns the effective API key for the given provider.
// Priority: LLM_API_KEY > provider-specific env var (OPENAI_API_KEY or
// ANTHROPIC_API_KEY) > empty string.
func resolveLLMAPIKey(provider string) string {
	if key := os.Getenv("LLM_API_KEY"); key != "" {
		return key
	}
	switch provider {
	case "openai":
		return os.Getenv("OPENAI_API_KEY")
	case "anthropic":
		return os.Getenv("ANTHROPIC_API_KEY")
	}
	return ""
}

// resolveLLMBaseURL returns the LLM_BASE_URL environment variable, or empty
// if not set.
func resolveLLMBaseURL() string {
	return os.Getenv("LLM_BASE_URL")
}

// newOpenAILLM creates an OpenAI LLM from environment variables.
func newOpenAILLM() (*LLMService, error) {
	opts := []openai.Option{}

	if token := resolveLLMAPIKey("openai"); token != "" {
		opts = append(opts, openai.WithToken(token))
	}

	if baseURL := resolveLLMBaseURL(); baseURL != "" {
		opts = append(opts, openai.WithBaseURL(baseURL))
	}

	if model := os.Getenv("LLM_MODEL"); model != "" {
		opts = append(opts, openai.WithModel(model))
	}

	llm, err := openai.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create OpenAI LLM: %w", err)
	}

	return &LLMService{model: llm}, nil
}

// newAnthropicLLM creates an Anthropic LLM from environment variables.
func newAnthropicLLM() (*LLMService, error) {
	opts := []anthropic.Option{}

	if token := resolveLLMAPIKey("anthropic"); token != "" {
		opts = append(opts, anthropic.WithToken(token))
	}

	if baseURL := resolveLLMBaseURL(); baseURL != "" {
		opts = append(opts, anthropic.WithBaseURL(baseURL))
	}

	if model := os.Getenv("LLM_MODEL"); model != "" {
		opts = append(opts, anthropic.WithModel(model))
	}

	llm, err := anthropic.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create Anthropic LLM: %w", err)
	}

	return &LLMService{model: llm}, nil
}
