package handler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agentmemory/agentmemory/internal/auth"
	"github.com/gorilla/websocket"
)

// mockValidator implements TokenValidator for testing.
type mockValidator struct {
	mu     sync.Mutex
	count  int
	fn     func(token string) error
}

func (m *mockValidator) ValidateToken(token string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.count++
	if m.fn != nil {
		return m.fn(token)
	}
	return nil
}

func (m *mockValidator) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.count
}

// =============================================================================
// Issue #98: WebSocket mid-connection token re-validation
// =============================================================================

func TestWSHandlerRevalidatesTokenPeriodically(t *testing.T) {
	hub := NewWSHub()
	validator := &mockValidator{}

	handler := &WSHandler{
		hub:             hub,
		validator:       validator,
		recheckInterval: 50 * time.Millisecond,
	}

	// Wrap with auth context
	wrapped := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), auth.UserIDKey, "test-user")
		handler.ServeHTTP(w, r.WithContext(ctx))
	})

	server := httptest.NewServer(wrapped)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/v1/socket?token=st_test-token"

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	// Wait for multiple recheck intervals to pass
	time.Sleep(200 * time.Millisecond)

	if validator.CallCount() < 2 {
		t.Errorf("Expected validator to be called at least 2 times, got %d", validator.CallCount())
	}
}

func TestWSHandlerClosesOnInvalidToken(t *testing.T) {
	hub := NewWSHub()

	var callCount int
	var mu sync.Mutex
	validator := &mockValidator{
		fn: func(token string) error {
			mu.Lock()
			defer mu.Unlock()
			callCount++
			// Fail on third recheck
			if callCount >= 3 {
				return fmt.Errorf("token expired")
			}
			return nil
		},
	}

	handler := &WSHandler{
		hub:             hub,
		validator:       validator,
		recheckInterval: 30 * time.Millisecond,
	}

	// Wrap with auth context
	wrapped := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), auth.UserIDKey, "test-user")
		handler.ServeHTTP(w, r.WithContext(ctx))
	})

	server := httptest.NewServer(wrapped)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/v1/socket?token=st_test-token"

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	// Wait for validation to fail (3rd recheck at 30ms intervals)
	time.Sleep(200 * time.Millisecond)

	// Read the welcome message first (may be buffered)
	conn.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
	_, _, _ = conn.ReadMessage()

	// Connection should be closed — subsequent ReadMessage should return an error
	_, _, err = conn.ReadMessage()
	if err == nil {
		t.Error("Expected connection to be closed after token invalidation, but ReadMessage succeeded")
	}
}
