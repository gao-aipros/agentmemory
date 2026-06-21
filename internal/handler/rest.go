package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/agentmemory/agentmemory/internal/service"
)

// RESTHandler holds the service dependencies for REST API endpoints.
type RESTHandler struct {
	obsSvc       *service.ObservationService
	sessionSvc   *service.SessionService
	sessionEndH  *service.SessionEndHandler
}

// NewRESTHandler creates a new RESTHandler with the given service dependencies.
func NewRESTHandler(
	obsSvc *service.ObservationService,
	sessionSvc *service.SessionService,
	sessionEndH *service.SessionEndHandler,
) *RESTHandler {
	return &RESTHandler{
		obsSvc:      obsSvc,
		sessionSvc:  sessionSvc,
		sessionEndH: sessionEndH,
	}
}

// observeRequest is the JSON body for POST /v1/api/observe.
type observeRequest struct {
	SessionID   string   `json:"session_id"`
	OwnerType   string   `json:"owner_type,omitempty"`
	OwnerUserID string   `json:"owner_user_id,omitempty"`
	OwnerTeamID string   `json:"owner_team_id,omitempty"`
	Type        string   `json:"type"`
	Title       string   `json:"title"`
	Narrative   string   `json:"narrative"`
	Facts       string   `json:"facts,omitempty"`
	Concepts    []string `json:"concepts,omitempty"`
	Files       []string `json:"files,omitempty"`
	Importance  *float64 `json:"importance,omitempty"`
}

// observeResponse is the JSON response for a successful observation recording.
type observeResponse struct {
	ObservationID string `json:"observation_id"`
	Status        string `json:"status"`
}

// HandleObserve handles POST /v1/api/observe — record a new observation.
func (h *RESTHandler) HandleObserve(w http.ResponseWriter, r *http.Request) {
	if h.obsSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "observation service not configured",
		})
		return
	}

	var req observeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid JSON body",
		})
		return
	}

	// Validate required fields
	if req.Type == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "type is required",
		})
		return
	}
	if req.Title == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "title is required",
		})
		return
	}
	if req.Narrative == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "narrative is required",
		})
		return
	}
	if req.SessionID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "session_id is required",
		})
		return
	}

	// Default importance if not set
		importance := 0.5
		if req.Importance != nil {
			importance = *req.Importance
		}

	// Default ownership to the authenticated user if not provided in the request
	ownerType := req.OwnerType
	ownerUserID := req.OwnerUserID
	ownerTeamID := req.OwnerTeamID

	if ownerType == "" && ownerUserID == "" {
		if userID := GetUserIDFromContext(r.Context()); userID != "" {
			ownerType = "user"
			ownerUserID = userID
		}
	}

	input := service.RecordObservationInput{
		SessionID:   req.SessionID,
		OwnerType:   ownerType,
		OwnerUserID: ownerUserID,
		OwnerTeamID: ownerTeamID,
		Type:        req.Type,
		Title:       req.Title,
		Narrative:   req.Narrative,
		Facts:       req.Facts,
		Concepts:    req.Concepts,
		Files:       req.Files,
		Importance:  &importance,
	}

	obs, err := h.obsSvc.RecordObservation(r.Context(), input)
	if err != nil {
		slog.Warn("failed to record observation", "error", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusCreated, observeResponse{
		ObservationID: obs.ID,
		Status:        "recorded",
	})
}

// endSessionRequest is the JSON body for POST /v1/api/session/end.
type endSessionRequest struct {
	SessionID string `json:"session_id"`
}

// endSessionResponse is the JSON response for a successful session end.
type endSessionResponse struct {
	SessionID string `json:"session_id"`
	Status    string `json:"status"`
}

// HandleEndSession handles POST /v1/api/session/end — end a session and trigger the memory pipeline.
func (h *RESTHandler) HandleEndSession(w http.ResponseWriter, r *http.Request) {
	if h.sessionEndH == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "session end handler not configured",
		})
		return
	}

	var req endSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid JSON body",
		})
		return
	}

	if req.SessionID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "session_id is required",
		})
		return
	}

	if err := h.sessionEndH.HandleSessionEnd(r.Context(), req.SessionID); err != nil {
		slog.Warn("failed to end session", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, endSessionResponse{
		SessionID: req.SessionID,
		Status:    "ended",
	})
}

// commitRequest is the JSON body for POST /v1/api/session/commit.
type commitRequest struct {
	SessionID string `json:"session_id"`
	SHA       string `json:"sha"`
	Branch    string `json:"branch"`
	Message   string `json:"message"`
}

// commitResponse is the JSON response for a successful commit link.
type commitResponse struct {
	SessionID string `json:"session_id"`
	SHA       string `json:"sha"`
	Status    string `json:"status"`
}

// HandleCommitSession handles POST /v1/api/session/commit — link a git commit to a session.
func (h *RESTHandler) HandleCommitSession(w http.ResponseWriter, r *http.Request) {
	var req commitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid JSON body",
		})
		return
	}

	if req.SessionID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "session_id is required",
		})
		return
	}
	if req.SHA == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "sha is required",
		})
		return
	}

	// In MVP, the commit link is recorded as an observation of type post_commit.
	// Full commit tracking table will be added in a future phase.
	slog.Info("commit linked to session",
		"session_id", req.SessionID,
		"sha", req.SHA,
		"branch", req.Branch,
	)

	writeJSON(w, http.StatusOK, commitResponse{
		SessionID: req.SessionID,
		SHA:       req.SHA,
		Status:    "linked",
	})
}

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("failed to encode JSON response", "error", err)
	}
}
