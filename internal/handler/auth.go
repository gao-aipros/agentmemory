package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/agentmemory/agentmemory/internal/auth"
	"github.com/agentmemory/agentmemory/internal/config"
	"github.com/agentmemory/agentmemory/internal/service"
	"github.com/go-chi/chi/v5"
)

// AuthHandler handles authentication endpoints: login, API key management.
type AuthHandler struct {
	cfg       *config.Config
	userSvc   *service.UserService
	teamSvc   *service.TeamService
	memberSvc *service.TeamMembersService
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(cfg *config.Config, userSvc *service.UserService, teamSvc *service.TeamService, memberSvc *service.TeamMembersService) *AuthHandler {
	return &AuthHandler{
		cfg:       cfg,
		userSvc:   userSvc,
		teamSvc:   teamSvc,
		memberSvc: memberSvc,
	}
}

// loginRequest is the JSON body for POST /v1/auth/login.
type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	TOTPCode string `json:"totp_code,omitempty"`
}

// loginResponse is the JSON response for a successful login.
type loginResponse struct {
	Token     string       `json:"token"`
	ExpiresAt string       `json:"expires_at"`
	User      userResponse `json:"user"`
}

type userResponse struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

// HandleLogin handles POST /v1/auth/login — authenticates a user and returns a JWT.
func (h *AuthHandler) HandleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "email and password are required")
		return
	}

	// Look up user by email
	user, err := h.userSvc.GetUserByEmail(r.Context(), req.Email)
	if err != nil {
		slog.Warn("login failed: user not found", "email", req.Email)
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	// Validate password
	if !auth.CheckPassword(user.PasswordHash, req.Password) {
		slog.Warn("login failed: invalid password", "email", req.Email)
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	// Check TOTP if enabled
	if user.TotpEnabled {
		if req.TOTPCode == "" {
			writeError(w, http.StatusUnauthorized, "TOTP code required")
			return
		}
		if user.TotpSecret == nil || !auth.ValidateTOTP(*user.TotpSecret, req.TOTPCode) {
			writeError(w, http.StatusUnauthorized, "invalid TOTP code")
			return
		}
	}

	// Validate JWT secret is configured
	if h.cfg.JWTSecret == "" {
		slog.Error("JWT secret not configured")
		writeError(w, http.StatusInternalServerError, "authentication configuration error")
		return
	}

	// Generate JWT
	token, err := auth.GenerateToken(user.ID, h.cfg.JWTExpiry, h.cfg.JWTSecret)
	if err != nil {
		slog.Error("failed to generate JWT", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	slog.Info("user logged in", "user_id", user.ID, "email", user.Email)

	writeJSON(w, http.StatusOK, loginResponse{
		Token: token,
		ExpiresAt: time.Now().Add(h.cfg.JWTExpiry).Format(time.RFC3339),
		User: userResponse{
			ID:    user.ID,
			Email: user.Email,
			Name:  user.Name,
		},
	})
}

// HandleRegister handles POST /v1/auth/register — creates a new user account.
func (h *AuthHandler) HandleRegister(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Name     string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.Email == "" || req.Password == "" || req.Name == "" {
		writeError(w, http.StatusBadRequest, "email, password, and name are required")
		return
	}

	user, err := h.userSvc.CreateUser(r.Context(), req.Email, req.Password, req.Name)
	if err != nil {
		slog.Warn("registration failed", "error", err)
		writeError(w, http.StatusConflict, "registration failed")
		return
	}

	// Validate JWT secret is configured
	if h.cfg.JWTSecret == "" {
		slog.Error("JWT secret not configured")
		writeError(w, http.StatusInternalServerError, "authentication configuration error")
		return
	}

	// Auto-login: generate JWT for the new user
	token, err := auth.GenerateToken(user.ID, h.cfg.JWTExpiry, h.cfg.JWTSecret)
	if err != nil {
		slog.Error("failed to generate JWT", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	slog.Info("user registered", "user_id", user.ID, "email", user.Email)

	writeJSON(w, http.StatusCreated, loginResponse{
		Token: token,
		ExpiresAt: time.Now().Add(h.cfg.JWTExpiry).Format(time.RFC3339),
		User: userResponse{
			ID:    user.ID,
			Email: user.Email,
			Name:  user.Name,
		},
	})
}

// HandleGetMe handles GET /v1/auth/me — returns the authenticated user's profile.
func (h *AuthHandler) HandleGetMe(w http.ResponseWriter, r *http.Request) {
	userID := GetUserIDFromContext(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	user, err := h.userSvc.GetUser(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	// Get user's team if any
	team, _ := h.memberSvc.GetUserTeam(r.Context(), userID)

	type teamInfo struct {
		ID   string `json:"id,omitempty"`
		Name string `json:"name,omitempty"`
	}

	var tInfo *teamInfo
	if team != nil {
		tInfo = &teamInfo{ID: team.ID, Name: team.Name}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"user": userResponse{
			ID:    user.ID,
			Email: user.Email,
			Name:  user.Name,
		},
		"team": tInfo,
	})
}

// listAPIKeysResponse is the JSON response for listing API keys.
type apiKeyResponse struct {
	ID         string  `json:"id"`
	Label      string  `json:"label"`
	Prefix     string  `json:"prefix"`
	LastUsedAt *string `json:"last_used_at,omitempty"`
	CreatedAt  string  `json:"created_at"`
	ExpiresAt  *string `json:"expires_at,omitempty"`
}

// HandleListAPIKeys handles GET /v1/auth/keys — lists the user's API keys.
func (h *AuthHandler) HandleListAPIKeys(w http.ResponseWriter, r *http.Request) {
	userID := GetUserIDFromContext(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	keys, err := h.userSvc.ListAPIKeys(r.Context(), userID)
	if err != nil {
		slog.Error("failed to list API keys", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list API keys")
		return
	}

	response := make([]apiKeyResponse, 0, len(keys))
	for _, k := range keys {
		prefixPart, err := auth.SafeSlice(k.KeyHash, auth.APIKeyPrefixLength)
		if err != nil {
			slog.Error("API key has invalid hash, skipping", "key_id", k.ID, "error", err)
			continue
		}
		akr := apiKeyResponse{
			ID:        k.ID,
			Label:     k.Label,
			Prefix:    auth.APIKeyPrefix + prefixPart,
			CreatedAt: k.CreatedAt.Time.Format(time.RFC3339),
		}
		if k.LastUsedAt.Valid {
			t := k.LastUsedAt.Time.Format(time.RFC3339)
			akr.LastUsedAt = &t
		}
		if k.ExpiresAt.Valid {
			t := k.ExpiresAt.Time.Format(time.RFC3339)
			akr.ExpiresAt = &t
		}
		response = append(response, akr)
	}

	writeJSON(w, http.StatusOK, map[string]any{"keys": response})
}

// createAPIKeyRequest is the JSON body for POST /v1/auth/keys.
type createAPIKeyRequest struct {
	Label     string `json:"label"`
	ExpiresAt string `json:"expires_at,omitempty"`
}

// createAPIKeyResponse is the JSON response for a newly created API key.
type createAPIKeyResponse struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Prefix   string `json:"prefix"`
	Key      string `json:"key"`
	KeyHash  string `json:"-"`
}

// HandleCreateAPIKey handles POST /v1/auth/keys — creates a new API key.
func (h *AuthHandler) HandleCreateAPIKey(w http.ResponseWriter, r *http.Request) {
	userID := GetUserIDFromContext(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var req createAPIKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.Label == "" {
		writeError(w, http.StatusBadRequest, "label is required")
		return
	}

	// Create the API key via the user service
	apiKey, fullKey, err := h.userSvc.CreateAPIKey(r.Context(), userID, req.Label, req.ExpiresAt)
	if err != nil {
		slog.Error("failed to create API key", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create API key")
		return
	}

	prefixPart, err := auth.SafeSlice(apiKey.KeyHash, auth.APIKeyPrefixLength)
	if err != nil {
		slog.Error("newly created API key has invalid hash", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create API key")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":       apiKey.ID,
		"label":    apiKey.Label,
		"prefix":   auth.APIKeyPrefix + prefixPart,
		"key": fullKey,
	})
}

// HandleDeleteAPIKey handles DELETE /v1/auth/keys/{key_id} — revokes an API key.
func (h *AuthHandler) HandleDeleteAPIKey(w http.ResponseWriter, r *http.Request) {
	userID := GetUserIDFromContext(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	keyID := chi.URLParam(r, "key_id")
	if keyID == "" {
		writeError(w, http.StatusBadRequest, "key_id is required")
		return
	}

	if err := h.userSvc.DeleteAPIKey(r.Context(), userID, keyID); err != nil {
		slog.Error("failed to delete API key", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to delete API key")
		return
	}

	writeJSON(w, http.StatusNoContent, nil)
}
