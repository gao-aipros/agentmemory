package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/agentmemory/agentmemory/internal/auth"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// WSHub manages connected WebSocket clients and broadcasts messages.
type WSHub struct {
	mu      sync.RWMutex
	clients map[string]map[*websocket.Conn]bool // userID -> set of connections
}

// NewWSHub creates a new WebSocket hub.
func NewWSHub() *WSHub {
	return &WSHub{
		clients: make(map[string]map[*websocket.Conn]bool),
	}
}

// Register adds a WebSocket connection for a user.
func (h *WSHub) Register(userID string, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.clients[userID] == nil {
		h.clients[userID] = make(map[*websocket.Conn]bool)
	}
	h.clients[userID][conn] = true
	slog.Info("WebSocket client connected", "user_id", userID)
}

// Unregister removes a WebSocket connection for a user.
func (h *WSHub) Unregister(userID string, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if conns, ok := h.clients[userID]; ok {
		delete(conns, conn)
		if len(conns) == 0 {
			delete(h.clients, userID)
		}
	}
	slog.Info("WebSocket client disconnected", "user_id", userID)
}

// SendToUser sends a message to all WebSocket connections for a specific user.
func (h *WSHub) SendToUser(userID string, message interface{}) error {
	h.mu.RLock()
	conns, ok := h.clients[userID]
	h.mu.RUnlock()

	if !ok || len(conns) == 0 {
		return nil // No connections for this user
	}

	data, err := json.Marshal(message)
	if err != nil {
		return err
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	for conn := range conns {
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			slog.Warn("failed to send WebSocket message", "user_id", userID, "error", err)
		}
	}

	return nil
}

// Broadcast sends a message to all connected clients.
func (h *WSHub) Broadcast(message interface{}) error {
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	for userID, conns := range h.clients {
		for conn := range conns {
			if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
				slog.Warn("failed to broadcast WebSocket message", "user_id", userID, "error", err)
			}
		}
	}

	return nil
}

// TokenValidator re-validates a session token and returns an error if invalid.
type TokenValidator interface {
	ValidateToken(token string) error
}

// SessionTokenValidator implements TokenValidator for JWT session tokens.
type SessionTokenValidator struct {
	secret  string
	queries interface {
		GetUserByID(ctx interface{}, id string) (interface{}, error)
	}
}

// NewSessionTokenValidator creates a new SessionTokenValidator.
func NewSessionTokenValidator(secret string) *SessionTokenValidator {
	return &SessionTokenValidator{secret: secret}
}

// ValidateToken re-validates a JWT session token.
func (v *SessionTokenValidator) ValidateToken(token string) error {
	_, err := auth.ValidateToken(token, v.secret)
	return err
}

// WSHandler handles WebSocket connections at /v1/socket.
// It requires session token authentication (rejects API keys).
type WSHandler struct {
	hub             *WSHub
	validator       TokenValidator
	recheckInterval time.Duration
}

// NewWSHandler creates a new WebSocket handler.
func NewWSHandler(hub *WSHub, validator TokenValidator) *WSHandler {
	return &WSHandler{
		hub:             hub,
		validator:       validator,
		recheckInterval: 5 * time.Minute,
	}
}

// ServeHTTP handles the WebSocket upgrade request.
func (h *WSHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Verify authentication — must be a session token (not API key)
	userID := GetUserIDFromContext(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	// Check that the token is a session token (the middleware already validates,
	// but we double-check that API keys are rejected)
	token := extractToken(r)
	if token != "" {
		if len(token) >= len(auth.APIKeyPrefix) && token[:len(auth.APIKeyPrefix)] == auth.APIKeyPrefix {
			writeError(w, http.StatusForbidden, "API keys are not allowed for WebSocket; use a session token")
			return
		}
	}

	// Upgrade HTTP connection to WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("WebSocket upgrade failed", "error", err)
		return
	}

	// Register the connection
	h.hub.Register(userID, conn)
	defer h.hub.Unregister(userID, conn)

	// Start background token re-validation goroutine
	// Periodically re-checks the session token to detect expiry or revocation.
	if h.validator != nil && token != "" {
		recheckInterval := h.recheckInterval
		if recheckInterval <= 0 {
			recheckInterval = 5 * time.Minute
		}
		go func() {
			ticker := time.NewTicker(recheckInterval)
			defer ticker.Stop()

			for range ticker.C {
				if err := h.validator.ValidateToken(token); err != nil {
					slog.Warn("token validation failed, closing WebSocket",
						"user_id", userID, "error", err)
					conn.Close()
					return
				}
			}
		}()
	}

	// Send initial session summary
	welcome := map[string]interface{}{
		"type":      "session_summary",
		"user_id":   userID,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"message":   "Connected to AgentMemory v2.0.0",
	}
	if err := conn.WriteJSON(welcome); err != nil {
		slog.Warn("failed to send welcome message", "error", err)
		return
	}

	// Read loop — keep connection alive and handle incoming messages
	for {
		messageType, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				slog.Warn("WebSocket read error", "error", err)
			}
			break
		}

		// Handle ping/pong
		if messageType == websocket.PingMessage {
			if err := conn.WriteMessage(websocket.PongMessage, nil); err != nil {
				slog.Warn("failed to send pong", "error", err)
				break
			}
			continue
		}

		// Log received messages for debugging
		slog.Debug("WebSocket message received", "user_id", userID, "size", len(message))
	}
}
