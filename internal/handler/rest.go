package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/agentmemory/agentmemory/internal/service"
	"github.com/jackc/pgx/v5/pgtype"
)

// RESTHandler holds the service dependencies for REST API endpoints.
type RESTHandler struct {
	obsSvc         *service.ObservationService
	sessionSvc     *service.SessionService
	sessionEndH    *service.SessionEndHandler
	contextHookMgr *service.ContextHookManager
}

// NewRESTHandler creates a new RESTHandler with the given service dependencies.
func NewRESTHandler(
	obsSvc *service.ObservationService,
	sessionSvc *service.SessionService,
	sessionEndH *service.SessionEndHandler,
	contextHookMgr *service.ContextHookManager,
) *RESTHandler {
	return &RESTHandler{
		obsSvc:         obsSvc,
		sessionSvc:     sessionSvc,
		sessionEndH:    sessionEndH,
		contextHookMgr: contextHookMgr,
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
	Inject      bool     `json:"inject,omitempty"`
}

// observeResponse is the JSON response for a successful observation recording.
type observeResponse struct {
	ObservationID string `json:"observation_id"`
	Status        string `json:"status"`
}

// observeInjectResponse is the JSON response when inject:true includes
// context injection fields alongside the standard observation response.
type observeInjectResponse struct {
	ObservationID string `json:"observation_id"`
	Status        string `json:"status"`
	ContextText   string `json:"context_text"`
	Skipped       bool   `json:"skipped"`
	SkipReason    string `json:"skip_reason,omitempty"`
}

// HandleObserve handles POST /v1/api/observe — record a new observation.
func (h *RESTHandler) HandleObserve(w http.ResponseWriter, r *http.Request) {
	if h.obsSvc == nil {
		writeError(w, http.StatusServiceUnavailable, "observation service not configured")
		return
	}

	var req observeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	// Validate required fields
	if req.Type == "" {
		writeError(w, http.StatusBadRequest, "type is required")
		return
	}
	if req.Title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}
	if req.Narrative == "" {
		writeError(w, http.StatusBadRequest, "narrative is required")
		return
	}
	if req.SessionID == "" {
		writeError(w, http.StatusBadRequest, "session_id is required")
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
		writeError(w, http.StatusBadRequest, "observation failed")
		return
	}

	// If inject is true, attempt context injection based on the trigger type.
	if req.Inject {
		if h.contextHookMgr == nil {
			slog.Warn("context injection requested but context injection not configured")
			writeJSON(w, http.StatusCreated, observeInjectResponse{
				ObservationID: obs.ID,
				Status:        "recorded",
				Skipped:       true,
				SkipReason:    "gate_disabled",
			})
			return
		}

		var result *service.ContextHookResult

		switch req.Type {
		case "session_start":
			userID := GetUserIDFromContext(r.Context())
			result = h.contextHookMgr.TriggerSessionStart(r.Context(), userID)
		case "pre_tool_use":
			userID := GetUserIDFromContext(r.Context())
			result = h.contextHookMgr.TriggerPreToolUse(r.Context(), userID, req.Files)
		case "pre_compact":
			userID := GetUserIDFromContext(r.Context())
			result = h.contextHookMgr.TriggerPreCompact(r.Context(), userID)
		default:
			result = &service.ContextHookResult{
				Skipped:    true,
				SkipReason: "non_context_trigger_type",
			}
		}

		resp := observeInjectResponse{
			ObservationID: obs.ID,
			Status:        "recorded",
			ContextText:   result.ContextText,
			Skipped:       result.Skipped,
		}
		if result.Skipped {
			resp.SkipReason = result.SkipReason
		}
		writeJSON(w, http.StatusCreated, resp)
		return
	}

	writeJSON(w, http.StatusCreated, observeResponse{
		ObservationID: obs.ID,
		Status:        "recorded",
	})
}

// startSessionRequest is the JSON body for POST /v1/api/session/start.
type startSessionRequest struct {
	TeamID string `json:"team_id,omitempty"`
}

// startSessionResponse is the JSON response for a successful session start.
type startSessionResponse struct {
	SessionID string `json:"session_id"`
	StartedAt string `json:"started_at"`
	Status    string `json:"status"`
}

// HandleStartSession handles POST /v1/api/session/start — start a new session.
func (h *RESTHandler) HandleStartSession(w http.ResponseWriter, r *http.Request) {
	if h.sessionSvc == nil {
		writeError(w, http.StatusServiceUnavailable, "session service not configured")
		return
	}

	userID := GetUserIDFromContext(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	var req startSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	session, err := h.sessionSvc.CreateSession(r.Context(), userID, req.TeamID)
	if err != nil {
		slog.Warn("failed to start session", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to start session")
		return
	}

	startedAt := formatTimestamptz(session.StartedAt)

	slog.Info("session started",
		"session_id", session.ID,
		"user_id", userID,
		"team_id", req.TeamID,
	)

	writeJSON(w, http.StatusCreated, startSessionResponse{
		SessionID: session.ID,
		StartedAt: startedAt,
		Status:    session.Status,
	})
}

// formatTimestamptz formats a pgtype.Timestamptz to an RFC3339 string.
// Returns an empty string if the timestamptz is not valid.
func formatTimestamptz(ts pgtype.Timestamptz) string {
	if !ts.Valid {
		return ""
	}
	return ts.Time.Format(time.RFC3339)
}

// endSessionRequest is the JSON body for POST /v1/api/session/end.
type endSessionRequest struct {
	SessionID string `json:"session_id"`
}

// endSessionResponse is the JSON response for a successful session end.
type endSessionResponse struct {
	SessionID            string `json:"session_id"`
	Status               string `json:"status"`
	SummaryQueued        bool   `json:"summary_queued"`
	ConsolidationQueued  bool   `json:"consolidation_queued"`
}

// HandleEndSession handles POST /v1/api/session/end — end a session and trigger the memory pipeline.
func (h *RESTHandler) HandleEndSession(w http.ResponseWriter, r *http.Request) {
	if h.sessionEndH == nil {
		writeError(w, http.StatusServiceUnavailable, "session end handler not configured")
		return
	}

	var req endSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.SessionID == "" {
		writeError(w, http.StatusBadRequest, "session_id is required")
		return
	}

	if err := h.sessionEndH.HandleSessionEnd(r.Context(), req.SessionID); err != nil {
		slog.Warn("failed to end session", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to end session")
		return
	}

	writeJSON(w, http.StatusOK, endSessionResponse{
		SessionID:           req.SessionID,
		Status:              "ended",
		SummaryQueued:       true,
		ConsolidationQueued: true,
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
	CommitSHA  string `json:"commit_sha"`
	Status    string `json:"status"`
}

// HandleCommitSession handles POST /v1/api/session/commit — link a git commit to a session.
// The commit link is recorded as an observation of type post_commit.
func (h *RESTHandler) HandleCommitSession(w http.ResponseWriter, r *http.Request) {
	if h.obsSvc == nil {
		writeError(w, http.StatusServiceUnavailable, "observation service not configured")
		return
	}

	var req commitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.SessionID == "" {
		writeError(w, http.StatusBadRequest, "session_id is required")
		return
	}
	if req.SHA == "" {
		writeError(w, http.StatusBadRequest, "sha is required")
		return
	}

	// Derive ownership from auth context
	ownerType := ""
	ownerUserID := ""
	if userID := GetUserIDFromContext(r.Context()); userID != "" {
		ownerType = "user"
		ownerUserID = userID
	}

	narrative := "Commit linked to session"
	if req.Branch != "" {
		narrative += " on branch " + req.Branch
	}
	narrative += ": " + req.SHA
	if req.Message != "" {
		narrative += " (" + req.Message + ")"
	}

	input := service.RecordObservationInput{
		SessionID:   req.SessionID,
		OwnerType:   ownerType,
		OwnerUserID: ownerUserID,
		Type:        "post_commit",
		Title:       "Commit linked to session",
		Narrative:   narrative,
	}

	_, err := h.obsSvc.RecordObservation(r.Context(), input)
	if err != nil {
		slog.Warn("failed to record commit observation", "error", err)
		writeError(w, http.StatusBadRequest, "observation failed")
		return
	}

	slog.Info("commit linked to session",
		"session_id", req.SessionID,
		"sha", req.SHA,
		"branch", req.Branch,
	)

	writeJSON(w, http.StatusOK, commitResponse{
		SessionID: req.SessionID,
		CommitSHA:  req.SHA,
		Status:    "linked",
	})
}

// ErrorResponse is the standard error response body for all REST API endpoints.
type ErrorResponse struct {
	Error string `json:"error"`
	Code  string `json:"code"`
}

// httpStatusToCode maps an HTTP status code to an API error code string.
func httpStatusToCode(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "BAD_REQUEST"
	case http.StatusUnauthorized:
		return "UNAUTHORIZED"
	case http.StatusForbidden:
		return "FORBIDDEN"
	case http.StatusNotFound:
		return "NOT_FOUND"
	case http.StatusConflict:
		return "CONFLICT"
	case http.StatusTooManyRequests:
		return "RATE_LIMITED"
	case http.StatusInternalServerError:
		return "INTERNAL_ERROR"
	case http.StatusServiceUnavailable:
		return "SERVICE_UNAVAILABLE"
	default:
		return "INTERNAL_ERROR"
	}
}

// writeError writes a standard error response with both "error" and "code" fields.
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, ErrorResponse{
		Error: message,
		Code:  httpStatusToCode(status),
	})
}

// writeJSON writes a JSON response with the given status code.
// Marshal happens BEFORE WriteHeader so a failed encode does not leave a
// committed status with a truncated body.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	buf, err := json.Marshal(v)
	if err != nil {
		slog.Error("failed to encode JSON response", "error", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"internal server error","code":"INTERNAL_ERROR"}`))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(buf)
}

// contextInjectRequest is the JSON body for POST /v1/api/context/inject.
type contextInjectRequest struct {
	Trigger   string   `json:"trigger"`
	FilePaths []string `json:"file_paths,omitempty"`
}

// contextInjectResponse is the JSON response for context injection.
type contextInjectResponse struct {
	ContextText string `json:"context_text"`
	Trigger     string `json:"trigger"`
	Skipped     bool   `json:"skipped"`
	SkipReason  string `json:"skip_reason,omitempty"`
}

// HandleContextInject handles POST /v1/api/context/inject — return assembled context.
func (h *RESTHandler) HandleContextInject(w http.ResponseWriter, r *http.Request) {
	if h.contextHookMgr == nil {
		writeError(w, http.StatusServiceUnavailable, "context injection not configured")
		return
	}

	var req contextInjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.Trigger == "" {
		writeError(w, http.StatusBadRequest, "trigger is required")
		return
	}

	userID := GetUserIDFromContext(r.Context())
	if userID == "" {
		userID = "default"
	}

	var result *service.ContextHookResult
	switch req.Trigger {
	case "session_start":
		result = h.contextHookMgr.TriggerSessionStart(r.Context(), userID)
	case "pre_tool_use":
		result = h.contextHookMgr.TriggerPreToolUse(r.Context(), userID, req.FilePaths)
	case "pre_compact":
		result = h.contextHookMgr.TriggerPreCompact(r.Context(), userID)
	default:
		writeError(w, http.StatusBadRequest,
			"invalid trigger: must be one of: session_start, pre_tool_use, pre_compact")
		return
	}

	writeJSON(w, http.StatusOK, contextInjectResponse{
		ContextText: result.ContextText,
		Trigger:     string(result.HookType),
		Skipped:     result.Skipped,
		SkipReason:  result.SkipReason,
	})
}
