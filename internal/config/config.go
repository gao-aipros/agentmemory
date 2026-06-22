package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"
)

// Config holds all configuration for the AgentMemory v2 server.
type Config struct {
	// Database settings
	DBURL      string
	DBMaxConns int
	DBMinConns int

	// Server settings
	Port int

	// Auth settings
	JWTSecret string
	JWTExpiry time.Duration

	// Feature flags
	InjectContext       bool
	ShareConsolidated   bool

	// Logging
	LogLevel string

	// LLM / Embedding providers
	// Deprecated: Use LLMAPIKey instead.
	OpenAIAPIKey  string
	// Deprecated: Use LLMAPIKey instead.
	AnthropicAPIKey string
	EmbeddingProvider string
	EmbeddingModel    string
	LLMProvider       string
	LLMModel          string
	LLMAPIKey         string
	LLMBaseURL        string
	EmbeddingAPIKey   string
	EmbeddingBaseURL  string
}

// Load reads configuration from environment variables with sensible defaults.
func Load() *Config {
	return &Config{
		DBURL:             getEnv("DB_URL", ""),
		DBMaxConns:        getEnvInt("DB_MAX_CONNS", 25),
		DBMinConns:        getEnvInt("DB_MIN_CONNS", 5),
		Port:              getEnvInt("PORT", 8080),
		JWTSecret:         getEnv("JWT_SECRET", ""),
		JWTExpiry:         getEnvDuration("JWT_EXPIRY", 24*time.Hour),
		InjectContext:     getEnvBool("AGENTMEMORY_INJECT_CONTEXT", false),
		ShareConsolidated: getEnvBool("AGENTMEMORY_SHARE_CONSOLIDATED", false),
		LogLevel:          getEnv("LOG_LEVEL", "info"),
		OpenAIAPIKey:      getEnv("OPENAI_API_KEY", ""),
		AnthropicAPIKey:   getEnv("ANTHROPIC_API_KEY", ""),
		EmbeddingProvider: getEnv("EMBEDDING_PROVIDER", ""),
		EmbeddingModel:    getEnv("EMBEDDING_MODEL", ""),
		LLMProvider:       getEnv("LLM_PROVIDER", ""),
		LLMModel:          getEnv("LLM_MODEL", ""),
		LLMAPIKey:         getEnv("LLM_API_KEY", ""),
		LLMBaseURL:        getEnv("LLM_BASE_URL", ""),
		EmbeddingAPIKey:   getEnv("EMBEDDING_API_KEY", ""),
		EmbeddingBaseURL:  getEnv("EMBEDDING_BASE_URL", ""),
	}
}

// getEnv returns the value of the environment variable named by key,
// or fallback if the variable is not set.
func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return fallback
}

// getEnvInt returns the integer value of the environment variable named by key,
// or fallback if the variable is not set or cannot be parsed.
func getEnvInt(key string, fallback int) int {
	s := os.Getenv(key)
	if s == "" {
		return fallback
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		slog.Warn("failed to parse int env var, using fallback",
			"key", key, "value", s, "error", err)
		return fallback
	}
	return v
}

// getEnvBool returns the boolean value of the environment variable named by key,
// or fallback if the variable is not set or cannot be parsed.
func getEnvBool(key string, fallback bool) bool {
	s := os.Getenv(key)
	if s == "" {
		return fallback
	}
	v, err := strconv.ParseBool(s)
	if err != nil {
		slog.Warn("failed to parse bool env var, using fallback",
			"key", key, "value", s, "error", err)
		return fallback
	}
	return v
}

// GetJWTSecret returns the JWT secret from the environment.
// Returns an error if JWT_SECRET is not set — there is no hardcoded fallback.
func GetJWTSecret() (string, error) {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		return "", fmt.Errorf("JWT_SECRET environment variable is required but not set")
	}
	return secret, nil
}

// getEnvDuration returns the duration value of the environment variable named by key,
// or fallback if the variable is not set or cannot be parsed.
func getEnvDuration(key string, fallback time.Duration) time.Duration {
	s := os.Getenv(key)
	if s == "" {
		return fallback
	}
	v, err := time.ParseDuration(s)
	if err != nil {
		slog.Warn("failed to parse duration env var, using fallback",
			"key", key, "value", s, "error", err)
		return fallback
	}
	return v
}
