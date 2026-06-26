package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agentmemory/agentmemory/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// captureLogHandler is a slog.Handler that captures log records into a buffer.
type captureLogHandler struct {
	mu      sync.Mutex
	buf     bytes.Buffer
	enabled bool
}

func (h *captureLogHandler) Enabled(_ context.Context, _ slog.Level) bool {
	return h.enabled
}

func (h *captureLogHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	// Format the record into the buffer.
	_ = slog.NewTextHandler(&h.buf, &slog.HandlerOptions{Level: slog.LevelDebug}).Handle(context.Background(), r)
	return nil
}

func (h *captureLogHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *captureLogHandler) WithGroup(_ string) slog.Handler      { return h }

// logContains returns true if the captured log buffer contains the given string.
func (h *captureLogHandler) logContains(s string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return strings.Contains(h.buf.String(), s)
}

// reset clears the captured log buffer.
func (h *captureLogHandler) reset() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.buf.Reset()
}

// =============================================================================
// Profile Querier Interface & Mock
// =============================================================================

// mockProfileQuerier implements profileQuerier for testing.
type mockProfileQuerier struct {
	listObservationsByProject func(ctx context.Context, projectSlug string) ([]store.Observation, error)
	upsertProfile             func(ctx context.Context, params store.UpsertProfileParams) error
	getProfile                func(ctx context.Context, projectSlug string) (store.ProjectProfile, error)
}

func (m *mockProfileQuerier) ListObservationsByProject(ctx context.Context, projectSlug string) ([]store.Observation, error) {
	return m.listObservationsByProject(ctx, projectSlug)
}

func (m *mockProfileQuerier) UpsertProfile(ctx context.Context, params store.UpsertProfileParams) error {
	return m.upsertProfile(ctx, params)
}

func (m *mockProfileQuerier) GetProfile(ctx context.Context, projectSlug string) (store.ProjectProfile, error) {
	return m.getProfile(ctx, projectSlug)
}

// =============================================================================
// ConceptEntry and FileEntry are the JSON-serializable structures stored in
// ProjectProfile.TopConcepts and ProjectProfile.TopFiles respectively.
// =============================================================================

type ConceptEntry struct {
	Name                 string    `json:"name"`
	Frequency            float64   `json:"frequency"`
	SessionsSeen         int       `json:"sessions_seen"`
	SessionsSinceLastSeen int      `json:"sessions_since_last_seen"`
	LastSeenAt           time.Time `json:"last_seen_at"`
}

type FileEntry struct {
	Path                 string    `json:"path"`
	Count                int       `json:"count"`
	SessionsSeen         int       `json:"sessions_seen"`
	SessionsSinceLastSeen int      `json:"sessions_since_last_seen"`
	LastSeenAt           time.Time `json:"last_seen_at"`
}

// =============================================================================
// T006: Concept Frequency Counting
// =============================================================================

// TestProfileService_ConceptFrequencyCount verifies that multiple observations
// with the same concept produce correct frequency scores.
func TestProfileService_ConceptFrequencyCount(t *testing.T) {
	ctx := context.Background()

	var mu sync.Mutex
	var capturedParams store.UpsertProfileParams

	mockQ := &mockProfileQuerier{
		listObservationsByProject: func(ctx context.Context, projectSlug string) ([]store.Observation, error) {
			// 5 observations with overlapping concepts:
			//   database: obs1, obs2, obs3  → 3 occurrences
			//   performance: obs1, obs4      → 2 occurrences
			//   api: obs3, obs5              → 2 occurrences
			// Total concept occurrences: 3 + 2 + 2 = 7
			// Expected frequencies: database=3/7≈0.429, performance=2/7≈0.286, api=2/7≈0.286
			return []store.Observation{
				{ID: "obs1", Concepts: []string{"database", "performance"}, SessionID: "sess1"},
				{ID: "obs2", Concepts: []string{"database"}, SessionID: "sess1"},
				{ID: "obs3", Concepts: []string{"database", "api"}, SessionID: "sess2"},
				{ID: "obs4", Concepts: []string{"performance"}, SessionID: "sess2"},
				{ID: "obs5", Concepts: []string{"api"}, SessionID: "sess3"},
			}, nil
		},
		upsertProfile: func(ctx context.Context, params store.UpsertProfileParams) error {
			mu.Lock()
			defer mu.Unlock()
			capturedParams = params
			return nil
		},
		getProfile: func(ctx context.Context, projectSlug string) (store.ProjectProfile, error) {
			mu.Lock()
			defer mu.Unlock()
			return store.ProjectProfile{
				ProjectSlug:  projectSlug,
				TopConcepts:  capturedParams.TopConcepts,
				TopFiles:     capturedParams.TopFiles,
				Conventions:  capturedParams.Conventions,
				CommonErrors: capturedParams.CommonErrors,
			}, nil
		},
	}

	svc := newProfileServiceWithQuerier(mockQ)

	// UpdateProfile computes the profile from observations and stores it.
	err := svc.UpdateProfile(ctx, "test-project")
	require.NoError(t, err)

	// GetProfile retrieves the computed profile.
	profile, err := svc.GetProfile(ctx, "test-project")
	require.NoError(t, err)

	var concepts []ConceptEntry
	require.NoError(t, json.Unmarshal(profile.TopConcepts, &concepts))

	require.Len(t, concepts, 3, "should have exactly 3 distinct concepts")

	// database: 3/7 — highest frequency, always first
	assert.Equal(t, "database", concepts[0].Name, "database should be ranked first")
	assert.InDelta(t, 3.0/7.0, concepts[0].Frequency, 0.001,
		"database frequency should be 3/7 ≈ 0.429")

	// performance and api are tied at 2/7 — either order is acceptable.
	// Use set-based assertions to avoid brittle position checks.
	gotSecond := concepts[1].Name
	gotThird := concepts[2].Name
	assert.Contains(t, []string{"performance", "api"}, gotSecond,
		"second-ranked concept should be 'performance' or 'api' (both at 2/7)")
	assert.Contains(t, []string{"performance", "api"}, gotThird,
		"third-ranked concept should be the other tied entry")
	assert.NotEqual(t, gotSecond, gotThird, "second and third concepts must be distinct")
	assert.InDelta(t, 2.0/7.0, concepts[1].Frequency, 0.001,
		"concepts ranked 2nd/3rd should have frequency 2/7 ≈ 0.286")
	assert.InDelta(t, 2.0/7.0, concepts[2].Frequency, 0.001,
		"concepts ranked 2nd/3rd should have frequency 2/7 ≈ 0.286")
}

// =============================================================================
// T007: File Reference Counting
// =============================================================================

// TestProfileService_FileReferenceCount verifies that multiple observations
// referencing the same file produce correct counts.
func TestProfileService_FileReferenceCount(t *testing.T) {
	ctx := context.Background()

	var mu sync.Mutex
	var capturedParams store.UpsertProfileParams

	mockQ := &mockProfileQuerier{
		listObservationsByProject: func(ctx context.Context, projectSlug string) ([]store.Observation, error) {
			// 5 observations with file paths:
			//   /src/main.go:     obs1, obs4      → 2
			//   /src/db.go:       obs2             → 1
			//   /src/api.go:      obs3, obs5      → 2
			//   /src/handler.go:  obs5             → 1
			return []store.Observation{
				{ID: "obs1", Files: []string{"/src/main.go"}, SessionID: "sess1"},
				{ID: "obs2", Files: []string{"/src/db.go"}, SessionID: "sess1"},
				{ID: "obs3", Files: []string{"/src/api.go"}, SessionID: "sess2"},
				{ID: "obs4", Files: []string{"/src/main.go"}, SessionID: "sess2"},
				{ID: "obs5", Files: []string{"/src/api.go", "/src/handler.go"}, SessionID: "sess3"},
			}, nil
		},
		upsertProfile: func(ctx context.Context, params store.UpsertProfileParams) error {
			mu.Lock()
			defer mu.Unlock()
			capturedParams = params
			return nil
		},
		getProfile: func(ctx context.Context, projectSlug string) (store.ProjectProfile, error) {
			mu.Lock()
			defer mu.Unlock()
			return store.ProjectProfile{
				ProjectSlug:  projectSlug,
				TopConcepts:  capturedParams.TopConcepts,
				TopFiles:     capturedParams.TopFiles,
				Conventions:  capturedParams.Conventions,
				CommonErrors: capturedParams.CommonErrors,
			}, nil
		},
	}

	svc := newProfileServiceWithQuerier(mockQ)

	// UpdateProfile computes file reference counts from observations.
	err := svc.UpdateProfile(ctx, "test-project")
	require.NoError(t, err)

	profile, err := svc.GetProfile(ctx, "test-project")
	require.NoError(t, err)

	var files []FileEntry
	require.NoError(t, json.Unmarshal(profile.TopFiles, &files))

	require.Len(t, files, 4, "should have exactly 4 distinct file paths")

	// /src/main.go and /src/api.go are tied at count 2 — either order is valid.
	// Use set-based assertions to avoid brittle position checks.
	gotFirst := files[0].Path
	gotSecond := files[1].Path
	assert.Contains(t, []string{"/src/main.go", "/src/api.go"}, gotFirst,
		"first-ranked file should be main.go or api.go (both count=2)")
	assert.Contains(t, []string{"/src/main.go", "/src/api.go"}, gotSecond,
		"second-ranked file should be the other tied entry")
	assert.NotEqual(t, gotFirst, gotSecond, "first and second files must be distinct")
	assert.Equal(t, 2, files[0].Count, "top-ranked file should have count 2")
	assert.Equal(t, 2, files[1].Count, "second-ranked file should have count 2")

	// /src/db.go and /src/handler.go have count 1 — either order.
	gotThird := files[2].Path
	gotFourth := files[3].Path
	assert.Contains(t, []string{"/src/db.go", "/src/handler.go"}, gotThird)
	assert.Contains(t, []string{"/src/db.go", "/src/handler.go"}, gotFourth)
	assert.NotEqual(t, gotThird, gotFourth)
	assert.Equal(t, 1, files[2].Count, "third file should have count 1")
	assert.Equal(t, 1, files[3].Count, "fourth file should have count 1")
}

// =============================================================================
// T008: buildProfileSection Output Format
// =============================================================================

// TestProfileService_BuildProfileSection verifies that buildProfileSection
// formats concepts and files within a 200-token budget.
func TestProfileService_BuildProfileSection(t *testing.T) {
	conceptsJSON, err := json.Marshal([]ConceptEntry{
		{Name: "database", Frequency: 0.429, SessionsSeen: 3},
		{Name: "performance", Frequency: 0.286, SessionsSeen: 2},
		{Name: "api", Frequency: 0.286, SessionsSeen: 2},
	})
	require.NoError(t, err)

	filesJSON, err := json.Marshal([]FileEntry{
		{Path: "/src/main.go", Count: 2},
		{Path: "/src/api.go", Count: 2},
		{Path: "/src/db.go", Count: 1},
	})
	require.NoError(t, err)

	profile := &store.ProjectProfile{
		ProjectSlug: "test-project",
		TopConcepts: conceptsJSON,
		TopFiles:    filesJSON,
	}

	// buildProfileSection formats the profile as a text section for context injection.
	section := buildProfileSection(profile)

	// Output must contain both sections
	assert.Contains(t, section, "concepts",
		"output should contain concepts section")
	assert.Contains(t, section, "files",
		"output should contain files section")

	// Output must include concept and file names
	assert.Contains(t, section, "database",
		"output should list concept: database")
	assert.Contains(t, section, "/src/main.go",
		"output should list file: /src/main.go")

	// Output must respect 200-token budget
	tokenCount := EstimateTokens(section)
	assert.LessOrEqual(t, tokenCount, 200,
		"buildProfileSection output must be within 200-token budget (got %d tokens)", tokenCount)

	// Output must not be empty
	assert.NotEmpty(t, section, "output should not be empty")
}

// =============================================================================
// FR-009: Cold Start — buildProfileSection with nil/empty profile
// =============================================================================

// TestProfileService_BuildProfileSection_Empty verifies that buildProfileSection
// returns an empty string for nil or empty profiles (FR-009 cold start).
func TestProfileService_BuildProfileSection_Empty(t *testing.T) {
	// Nil profile should return empty string.
	section := buildProfileSection(nil)
	assert.Empty(t, section, "buildProfileSection(nil) should return empty string")

	// Profile with empty concept and file arrays should return empty string.
	profile := &store.ProjectProfile{
		ProjectSlug: "empty-project",
		TopConcepts: []byte("[]"),
		TopFiles:    []byte("[]"),
	}
	section = buildProfileSection(profile)
	assert.Empty(t, section, "buildProfileSection with empty arrays should return empty string")
}

// =============================================================================
// T009: Ranking Accuracy (SC-003)
// =============================================================================

// TestProfileService_RankingAccuracy verifies that concept frequencies computed
// from raw observation data match expected values with less than 10% error.
func TestProfileService_RankingAccuracy(t *testing.T) {
	ctx := context.Background()

	var mu sync.Mutex
	var capturedParams store.UpsertProfileParams

	// Seed observations with controlled concept distributions
	// Concept counts: database=5, performance=3, api=2, frontend=2, testing=1
	// Total: 13 concept occurrences
	observations := []store.Observation{
		{ID: "o1", Concepts: []string{"database", "performance"}, SessionID: "s1"},
		{ID: "o2", Concepts: []string{"database", "api"}, SessionID: "s1"},
		{ID: "o3", Concepts: []string{"database"}, SessionID: "s2"},
		{ID: "o4", Concepts: []string{"database", "performance"}, SessionID: "s2"},
		{ID: "o5", Concepts: []string{"database", "frontend"}, SessionID: "s3"},
		{ID: "o6", Concepts: []string{"performance", "api"}, SessionID: "s3"},
		{ID: "o7", Concepts: []string{"frontend", "testing"}, SessionID: "s4"},
	}

	// Expected frequencies: count / total
	expected := map[string]float64{
		"database":    5.0 / 13.0, // ≈ 0.385
		"performance": 3.0 / 13.0, // ≈ 0.231
		"api":         2.0 / 13.0, // ≈ 0.154
		"frontend":    2.0 / 13.0, // ≈ 0.154
		"testing":     1.0 / 13.0, // ≈ 0.077
	}

	mockQ := &mockProfileQuerier{
		listObservationsByProject: func(ctx context.Context, projectSlug string) ([]store.Observation, error) {
			return observations, nil
		},
		upsertProfile: func(ctx context.Context, params store.UpsertProfileParams) error {
			mu.Lock()
			defer mu.Unlock()
			capturedParams = params
			return nil
		},
		getProfile: func(ctx context.Context, projectSlug string) (store.ProjectProfile, error) {
			mu.Lock()
			defer mu.Unlock()
			return store.ProjectProfile{
				ProjectSlug:  projectSlug,
				TopConcepts:  capturedParams.TopConcepts,
				TopFiles:     capturedParams.TopFiles,
				Conventions:  capturedParams.Conventions,
				CommonErrors: capturedParams.CommonErrors,
			}, nil
		},
	}

	svc := newProfileServiceWithQuerier(mockQ)

	// UpdateProfile computes frequencies from the seeded observations.
	err := svc.UpdateProfile(ctx, "test-project")
	require.NoError(t, err)

	profile, err := svc.GetProfile(ctx, "test-project")
	require.NoError(t, err)

	var gotConcepts []ConceptEntry
	require.NoError(t, json.Unmarshal(profile.TopConcepts, &gotConcepts))

	// Verify all 5 concepts are present
	require.Len(t, gotConcepts, 5, "should have exactly 5 distinct concepts")

	// Build a lookup map
	got := make(map[string]float64)
	for _, c := range gotConcepts {
		got[c.Name] = c.Frequency
	}

	// Verify each concept frequency has less than 10% error
	for name, expectedFreq := range expected {
		gotFreq, ok := got[name]
		assert.True(t, ok, "concept %q should be present in profile", name)

		// Compute relative error: |actual - expected| / expected
		relErr := (gotFreq - expectedFreq) / expectedFreq
		if relErr < 0 {
			relErr = -relErr
		}
		assert.Less(t, relErr, 0.10,
			"relative error for concept %q should be < 10%% (expected %.3f, got %.3f, error %.1f%%)",
			name, expectedFreq, gotFreq, relErr*100)
	}
}

// =============================================================================
// T010: Update Idempotency (FR-011)
// =============================================================================

// TestProfileService_UpdateIdempotency verifies that calling UpdateProfile twice
// with identical observations produces identical frequency scores.
func TestProfileService_UpdateIdempotency(t *testing.T) {
	ctx := context.Background()

	var mu sync.Mutex
	var capturedParams []store.UpsertProfileParams

	observations := []store.Observation{
		{ID: "o1", Concepts: []string{"database", "performance"}, SessionID: "s1"},
		{ID: "o2", Concepts: []string{"database", "api"}, SessionID: "s1"},
		{ID: "o3", Concepts: []string{"database"}, SessionID: "s2"},
	}

	mockQ := &mockProfileQuerier{
		listObservationsByProject: func(ctx context.Context, projectSlug string) ([]store.Observation, error) {
			return observations, nil
		},
		upsertProfile: func(ctx context.Context, params store.UpsertProfileParams) error {
			mu.Lock()
			defer mu.Unlock()
			capturedParams = append(capturedParams, params)
			return nil
		},
		getProfile: func(ctx context.Context, projectSlug string) (store.ProjectProfile, error) {
			mu.Lock()
			defer mu.Unlock()
			// Return the most recently upserted profile.
			idx := len(capturedParams) - 1
			if idx < 0 {
				return store.ProjectProfile{}, fmt.Errorf("no profile upserted yet")
			}
			return store.ProjectProfile{
				ProjectSlug: projectSlug,
				TopConcepts: capturedParams[idx].TopConcepts,
				TopFiles:    capturedParams[idx].TopFiles,
			}, nil
		},
	}

	svc := newProfileServiceWithQuerier(mockQ)

	// First call — UpdateProfile computes and stores the profile.
	err := svc.UpdateProfile(ctx, "test-project")
	require.NoError(t, err)

	// Capture first result
	firstProfile, err := svc.GetProfile(ctx, "test-project")
	require.NoError(t, err)

	// Second call with identical observations — frequencies should be identical.
	err = svc.UpdateProfile(ctx, "test-project")
	require.NoError(t, err)

	// Capture second result
	secondProfile, err := svc.GetProfile(ctx, "test-project")
	require.NoError(t, err)

	// Frequency scores must be identical (compare JSON content, not raw bytes —
	// LastSeenAt timestamps differ between calls).
	var firstConcepts, secondConcepts []conceptJSON
	require.NoError(t, json.Unmarshal(firstProfile.TopConcepts, &firstConcepts))
	require.NoError(t, json.Unmarshal(secondProfile.TopConcepts, &secondConcepts))
	require.Equal(t, len(firstConcepts), len(secondConcepts))
	for i := range firstConcepts {
		assert.Equal(t, firstConcepts[i].Name, secondConcepts[i].Name)
		assert.InDelta(t, firstConcepts[i].Frequency, secondConcepts[i].Frequency, 0.001)
		assert.Equal(t, firstConcepts[i].SessionsSeen, secondConcepts[i].SessionsSeen)
		assert.Equal(t, firstConcepts[i].SessionsSinceLastSeen, secondConcepts[i].SessionsSinceLastSeen)
	}

	var firstFiles, secondFiles []fileJSON
	require.NoError(t, json.Unmarshal(firstProfile.TopFiles, &firstFiles))
	require.NoError(t, json.Unmarshal(secondProfile.TopFiles, &secondFiles))
	require.Equal(t, len(firstFiles), len(secondFiles))
	for i := range firstFiles {
		assert.Equal(t, firstFiles[i].Path, secondFiles[i].Path)
		assert.Equal(t, firstFiles[i].Count, secondFiles[i].Count)
		assert.Equal(t, firstFiles[i].SessionsSeen, secondFiles[i].SessionsSeen)
		assert.Equal(t, firstFiles[i].SessionsSinceLastSeen, secondFiles[i].SessionsSinceLastSeen)
	}

	// Verify UpdateProfile was called exactly twice (2 upserts).
	assert.Equal(t, 2, len(capturedParams), "UpsertProfile should have been called exactly twice")
}

// =============================================================================
// I1: Error-Path Tests
// =============================================================================

// TestProfileService_ListObservationsError verifies that UpdateProfile propagates
// the error when ListObservationsByProject fails.
func TestProfileService_ListObservationsError(t *testing.T) {
	ctx := context.Background()

	mockQ := &mockProfileQuerier{
		listObservationsByProject: func(ctx context.Context, projectSlug string) ([]store.Observation, error) {
			return nil, fmt.Errorf("database connection failed")
		},
		upsertProfile: func(ctx context.Context, params store.UpsertProfileParams) error {
			return nil
		},
		getProfile: func(ctx context.Context, projectSlug string) (store.ProjectProfile, error) {
			return store.ProjectProfile{}, fmt.Errorf("no rows")
		},
	}

	svc := newProfileServiceWithQuerier(mockQ)

	// UpdateProfile should propagate the error from ListObservationsByProject.
	err := svc.UpdateProfile(ctx, "test-project")
	require.Error(t, err, "UpdateProfile should propagate ListObservationsByProject error")
	assert.Contains(t, err.Error(), "database connection failed",
		"error message should include the underlying failure reason")
}

// TestProfileService_EmptyObservations verifies that UpdateProfile handles
// projects with no observations gracefully (no error, no upsert called).
func TestProfileService_EmptyObservations(t *testing.T) {
	ctx := context.Background()

	var upsertCalled bool

	mockQ := &mockProfileQuerier{
		listObservationsByProject: func(ctx context.Context, projectSlug string) ([]store.Observation, error) {
			return []store.Observation{}, nil
		},
		upsertProfile: func(ctx context.Context, params store.UpsertProfileParams) error {
			upsertCalled = true
			return nil
		},
		getProfile: func(ctx context.Context, projectSlug string) (store.ProjectProfile, error) {
			return store.ProjectProfile{}, fmt.Errorf("no rows")
		},
	}

	svc := newProfileServiceWithQuerier(mockQ)

	// UpdateProfile should return nil (no error) for empty observations.
	err := svc.UpdateProfile(ctx, "test-project")
	require.NoError(t, err, "UpdateProfile should return nil for empty observations")

	// Upsert should not be called when there are no observations to process.
	assert.False(t, upsertCalled, "UpsertProfile should not be called for empty observations")
}

// TestProfileService_GetProfileError verifies that GetProfile propagates or
// returns nil when the underlying store returns an error (e.g., no rows).
func TestProfileService_GetProfileError(t *testing.T) {
	ctx := context.Background()
	expectedErr := fmt.Errorf("project_profiles not found")

	mockQ := &mockProfileQuerier{
		listObservationsByProject: func(ctx context.Context, projectSlug string) ([]store.Observation, error) {
			return []store.Observation{
				{ID: "obs1", Concepts: []string{"test"}, SessionID: "sess1"},
			}, nil
		},
		upsertProfile: func(ctx context.Context, params store.UpsertProfileParams) error {
			return nil
		},
		getProfile: func(ctx context.Context, projectSlug string) (store.ProjectProfile, error) {
			return store.ProjectProfile{}, expectedErr
		},
	}

	svc := newProfileServiceWithQuerier(mockQ)

	// First update the profile so the store has been called.
	err := svc.UpdateProfile(ctx, "test-project")
	require.NoError(t, err)

	// GetProfile should propagate the store error.
	profile, err := svc.GetProfile(ctx, "test-project")
	require.Error(t, err, "GetProfile should propagate store error")
	assert.ErrorIs(t, err, expectedErr,
		"GetProfile error should be the original store error")
	assert.Nil(t, profile, "GetProfile should return nil profile on error")
}

// =============================================================================
// T021: Recency Weight Formula (US2)
// =============================================================================

// TestProfileService_RecencyWeightFormula verifies that the recency-weight
// formula score = raw_count * (1.0 / (1 + sessions_since_last_seen)) produces
// correct weights for ssls=0 (weight 1.0), ssls=1 (weight 0.5), and ssls=2
// (weight ~0.33) before the entry is dropped at ssls=3.
func TestProfileService_RecencyWeightFormula(t *testing.T) {
	ctx := context.Background()

	var mu sync.Mutex
	var capturedParams store.UpsertProfileParams
	callNum := 0

	// observationSets returns session-scoped data on each call.
	session1Obs := make([]store.Observation, 10)
	for i := 0; i < 10; i++ {
		session1Obs[i] = store.Observation{
			ID:        fmt.Sprintf("s1_o%d", i),
			Concepts:  []string{"test"},
			SessionID: "session1",
		}
	}
	session2Obs := []store.Observation{
		{ID: "s2_o1", Concepts: []string{"other"}, SessionID: "session2"},
	}
	session3Obs := []store.Observation{
		{ID: "s3_o1", Concepts: []string{"other"}, SessionID: "session3"},
	}
	session4Obs := []store.Observation{
		{ID: "s4_o1", Concepts: []string{"other"}, SessionID: "session4"},
	}

	observationSets := [][]store.Observation{session1Obs, session2Obs, session3Obs, session4Obs}

	mockQ := &mockProfileQuerier{
		listObservationsByProject: func(ctx context.Context, projectSlug string) ([]store.Observation, error) {
			mu.Lock()
			defer mu.Unlock()
			if callNum >= len(observationSets) {
				return observationSets[len(observationSets)-1], nil
			}
			result := observationSets[callNum]
			callNum++
			return result, nil
		},
		upsertProfile: func(ctx context.Context, params store.UpsertProfileParams) error {
			mu.Lock()
			defer mu.Unlock()
			capturedParams = params
			return nil
		},
		getProfile: func(ctx context.Context, projectSlug string) (store.ProjectProfile, error) {
			mu.Lock()
			defer mu.Unlock()
			if callNum == 0 || len(capturedParams.TopConcepts) == 0 {
				return store.ProjectProfile{}, fmt.Errorf("no profile")
			}
			return store.ProjectProfile{
				ProjectSlug:  projectSlug,
				TopConcepts:  capturedParams.TopConcepts,
				TopFiles:     capturedParams.TopFiles,
				Conventions:  capturedParams.Conventions,
				CommonErrors: capturedParams.CommonErrors,
			}, nil
		},
	}

	svc := newProfileServiceWithQuerier(mockQ)

	// Session 1: "test" appears 10 times (raw ratio = 1.0, weight = 1.0).
	err := svc.UpdateProfile(ctx, "test-project")
	require.NoError(t, err)

	profile, err := svc.GetProfile(ctx, "test-project")
	require.NoError(t, err)

	var concepts []ConceptEntry
	require.NoError(t, json.Unmarshal(profile.TopConcepts, &concepts))
	require.Len(t, concepts, 1, "session 1 should produce exactly 1 concept")
	assert.Equal(t, "test", concepts[0].Name)
	assert.Equal(t, 0, concepts[0].SessionsSinceLastSeen,
		"ssls should be 0 for concept seen in current batch")
	assert.InDelta(t, 1.0, concepts[0].Frequency, 0.001,
		"session 1: score = 10/10 * 1/(1+0) = 1.0")

	// Session 2: "test" not seen, ssls=1, weight=0.5.
	err = svc.UpdateProfile(ctx, "test-project")
	require.NoError(t, err)

	profile, err = svc.GetProfile(ctx, "test-project")
	require.NoError(t, err)

	concepts = nil
	require.NoError(t, json.Unmarshal(profile.TopConcepts, &concepts))
	require.Len(t, concepts, 2, "session 2 should have 2 concepts: test (decaying) + other")
	// "other" has freq=1.0 and ranks first; "test" has freq=0.5 and ranks second.
	testEntry := findConceptByName(concepts, "test")
	require.NotNil(t, testEntry, "test concept should exist in session 2")
	assert.Equal(t, 1, testEntry.SessionsSinceLastSeen,
		"test should have ssls=1 after session 2")
	assert.InDelta(t, 1.0*0.5, testEntry.Frequency, 0.001,
		"session 2: score = 1.0 * 1/(1+1) = 0.5")

	// Session 3: "test" not seen, ssls=2, weight=0.33...
	err = svc.UpdateProfile(ctx, "test-project")
	require.NoError(t, err)

	profile, err = svc.GetProfile(ctx, "test-project")
	require.NoError(t, err)

	concepts = nil
	require.NoError(t, json.Unmarshal(profile.TopConcepts, &concepts))
	require.Len(t, concepts, 2, "session 3 should still have 2 concepts")
	testEntry = findConceptByName(concepts, "test")
	require.NotNil(t, testEntry, "test concept should exist in session 3")
	assert.Equal(t, 2, testEntry.SessionsSinceLastSeen,
		"test should have ssls=2 after session 3")
	assert.InDelta(t, 0.5/(1.0+2.0), testEntry.Frequency, 0.001,
		"session 3: score = 0.5 * 1/(1+2) = 0.167")

	// Session 4: "test" not seen, ssls=3 -> DROPPED.
	err = svc.UpdateProfile(ctx, "test-project")
	require.NoError(t, err)

	profile, err = svc.GetProfile(ctx, "test-project")
	require.NoError(t, err)

	concepts = nil
	require.NoError(t, json.Unmarshal(profile.TopConcepts, &concepts))
	// "test" should be gone; only "other" remains.
	for _, c := range concepts {
		assert.NotEqual(t, "test", c.Name,
			"'test' should be dropped from top_concepts after ssls >= 3")
	}
}

// =============================================================================
// T022: Per-Project Mutex Serialization (US2)
// =============================================================================

// TestProfileService_MutexSerialization verifies that concurrent UpdateProfile
// calls for the same project_slug are serialized by the per-project mutex.
// Uses a flag inside listObservationsByProject to detect overlap.
func TestProfileService_MutexSerialization(t *testing.T) {
	ctx := context.Background()

	var detectionMu sync.Mutex
	var currentlyExecuting bool
	var concurrencyDetected bool

	mockQ := &mockProfileQuerier{
		listObservationsByProject: func(ctx context.Context, projectSlug string) ([]store.Observation, error) {
			detectionMu.Lock()
			if currentlyExecuting {
				concurrencyDetected = true
			}
			currentlyExecuting = true
			detectionMu.Unlock()

			// Sleep to give the second goroutine time to start if the lock fails.
			time.Sleep(100 * time.Millisecond)

			detectionMu.Lock()
			currentlyExecuting = false
			detectionMu.Unlock()

			return []store.Observation{
				{ID: "obs1", Concepts: []string{"test"}, SessionID: "s1"},
			}, nil
		},
		upsertProfile: func(ctx context.Context, params store.UpsertProfileParams) error {
			return nil
		},
		getProfile: func(ctx context.Context, projectSlug string) (store.ProjectProfile, error) {
			return store.ProjectProfile{}, fmt.Errorf("no profile")
		},
	}

	svc := newProfileServiceWithQuerier(mockQ)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		_ = svc.UpdateProfile(ctx, "test-project")
	}()
	go func() {
		defer wg.Done()
		_ = svc.UpdateProfile(ctx, "test-project")
	}()

	wg.Wait()

	assert.False(t, concurrencyDetected,
		"concurrent UpdateProfile calls for the same project must be serialized")
}

// =============================================================================
// T023: Concept Decay — 3-Session Threshold (US2)
// =============================================================================

// TestProfileService_ConceptDecay verifies that a concept mentioned 10 times
// in session 1, but not mentioned in 3 subsequent sessions, drops from
// top_concepts after the third session without mentions (ssls >= 3).
func TestProfileService_ConceptDecay(t *testing.T) {
	ctx := context.Background()

	var mu sync.Mutex
	var capturedParams store.UpsertProfileParams
	callNum := 0

	session1Obs := make([]store.Observation, 10)
	for i := 0; i < 10; i++ {
		session1Obs[i] = store.Observation{
			ID:        fmt.Sprintf("s1_o%d", i),
			Concepts:  []string{"target-concept"},
			SessionID: "session1",
		}
	}
	// Sessions 2-4: "target-concept" is NOT mentioned.
	session2Obs := []store.Observation{
		{ID: "s2_o1", Concepts: []string{"other-concept"}, SessionID: "session2"},
	}
	session3Obs := []store.Observation{
		{ID: "s3_o1", Concepts: []string{"other-concept"}, SessionID: "session3"},
	}
	session4Obs := []store.Observation{
		{ID: "s4_o1", Concepts: []string{"other-concept"}, SessionID: "session4"},
	}

	observationSets := [][]store.Observation{session1Obs, session2Obs, session3Obs, session4Obs}

	mockQ := &mockProfileQuerier{
		listObservationsByProject: func(ctx context.Context, projectSlug string) ([]store.Observation, error) {
			mu.Lock()
			defer mu.Unlock()
			if callNum >= len(observationSets) {
				return observationSets[len(observationSets)-1], nil
			}
			result := observationSets[callNum]
			callNum++
			return result, nil
		},
		upsertProfile: func(ctx context.Context, params store.UpsertProfileParams) error {
			mu.Lock()
			defer mu.Unlock()
			capturedParams = params
			return nil
		},
		getProfile: func(ctx context.Context, projectSlug string) (store.ProjectProfile, error) {
			mu.Lock()
			defer mu.Unlock()
			if callNum == 0 || len(capturedParams.TopConcepts) == 0 {
				return store.ProjectProfile{}, fmt.Errorf("no profile")
			}
			return store.ProjectProfile{
				ProjectSlug:  projectSlug,
				TopConcepts:  capturedParams.TopConcepts,
				TopFiles:     capturedParams.TopFiles,
				Conventions:  capturedParams.Conventions,
				CommonErrors: capturedParams.CommonErrors,
			}, nil
		},
	}

	svc := newProfileServiceWithQuerier(mockQ)

	// Session 1: "target-concept" mentioned 10 times.
	err := svc.UpdateProfile(ctx, "test-project")
	require.NoError(t, err)

	profile, err := svc.GetProfile(ctx, "test-project")
	require.NoError(t, err)

	var concepts []ConceptEntry
	require.NoError(t, json.Unmarshal(profile.TopConcepts, &concepts))
	require.Len(t, concepts, 1, "session 1: should have 1 concept")
	assert.Equal(t, "target-concept", concepts[0].Name)

	// Session 2: no "target-concept" mentions, ssls becomes 1.
	err = svc.UpdateProfile(ctx, "test-project")
	require.NoError(t, err)

	profile, err = svc.GetProfile(ctx, "test-project")
	require.NoError(t, err)

	concepts = nil
	require.NoError(t, json.Unmarshal(profile.TopConcepts, &concepts))
	assert.Contains(t, extractConceptNames(concepts), "target-concept",
		"session 2: 'target-concept' should still be present (ssls=1)")

	// Session 3: no "target-concept" mentions, ssls becomes 2.
	err = svc.UpdateProfile(ctx, "test-project")
	require.NoError(t, err)

	profile, err = svc.GetProfile(ctx, "test-project")
	require.NoError(t, err)

	concepts = nil
	require.NoError(t, json.Unmarshal(profile.TopConcepts, &concepts))
	assert.Contains(t, extractConceptNames(concepts), "target-concept",
		"session 3: 'target-concept' should still be present (ssls=2)")

	// Session 4: no "target-concept" mentions, ssls would be 3 -> DROPPED.
	err = svc.UpdateProfile(ctx, "test-project")
	require.NoError(t, err)

	profile, err = svc.GetProfile(ctx, "test-project")
	require.NoError(t, err)

	concepts = nil
	require.NoError(t, json.Unmarshal(profile.TopConcepts, &concepts))
	assert.NotContains(t, extractConceptNames(concepts), "target-concept",
		"session 4: 'target-concept' should be dropped (ssls >= 3)")
}

// findConceptByName is a test helper that returns the concept entry with the
// given name from a slice, or nil if not found.
func findConceptByName(entries []ConceptEntry, name string) *ConceptEntry {
	for i, e := range entries {
		if e.Name == name {
			return &entries[i]
		}
	}
	return nil
}

// extractConceptNames is a test helper that extracts concept names from a slice
// of ConceptEntry for easier assertion.
func extractConceptNames(entries []ConceptEntry) []string {
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name
	}
	return names
}

// =============================================================================
// T024: File Decay — 3-Session Threshold (US2)
// =============================================================================

// TestProfileService_FileDecay verifies that a file referenced in a session,
// then not referenced in 3 subsequent sessions, drops from top_files after
// the third session without references (ssls >= 3).
func TestProfileService_FileDecay(t *testing.T) {
	ctx := context.Background()

	var mu sync.Mutex
	var capturedParams store.UpsertProfileParams
	callNum := 0

	session1Obs := []store.Observation{
		{
			ID:        "s1_o1",
			Files:     []string{"/src/target.go"},
			SessionID: "session1",
		},
	}
	session2Obs := []store.Observation{
		{
			ID:        "s2_o1",
			Files:     []string{"/src/other.go"},
			SessionID: "session2",
		},
	}
	session3Obs := []store.Observation{
		{
			ID:        "s3_o1",
			Files:     []string{"/src/other.go"},
			SessionID: "session3",
		},
	}
	session4Obs := []store.Observation{
		{
			ID:        "s4_o1",
			Files:     []string{"/src/other.go"},
			SessionID: "session4",
		},
	}

	observationSets := [][]store.Observation{session1Obs, session2Obs, session3Obs, session4Obs}

	mockQ := &mockProfileQuerier{
		listObservationsByProject: func(ctx context.Context, projectSlug string) ([]store.Observation, error) {
			mu.Lock()
			defer mu.Unlock()
			if callNum >= len(observationSets) {
				return observationSets[len(observationSets)-1], nil
			}
			result := observationSets[callNum]
			callNum++
			return result, nil
		},
		upsertProfile: func(ctx context.Context, params store.UpsertProfileParams) error {
			mu.Lock()
			defer mu.Unlock()
			capturedParams = params
			return nil
		},
		getProfile: func(ctx context.Context, projectSlug string) (store.ProjectProfile, error) {
			mu.Lock()
			defer mu.Unlock()
			if callNum == 0 || len(capturedParams.TopConcepts) == 0 {
				return store.ProjectProfile{}, fmt.Errorf("no profile")
			}
			return store.ProjectProfile{
				ProjectSlug:  projectSlug,
				TopConcepts:  capturedParams.TopConcepts,
				TopFiles:     capturedParams.TopFiles,
				Conventions:  capturedParams.Conventions,
				CommonErrors: capturedParams.CommonErrors,
			}, nil
		},
	}

	svc := newProfileServiceWithQuerier(mockQ)

	// Session 1: "/src/target.go" referenced.
	err := svc.UpdateProfile(ctx, "test-project")
	require.NoError(t, err)

	profile, err := svc.GetProfile(ctx, "test-project")
	require.NoError(t, err)

	var files []FileEntry
	require.NoError(t, json.Unmarshal(profile.TopFiles, &files))
	assert.Contains(t, extractFilePaths(files), "/src/target.go",
		"session 1: file should be present")

	// Session 2: "/src/target.go" not referenced.
	err = svc.UpdateProfile(ctx, "test-project")
	require.NoError(t, err)

	profile, err = svc.GetProfile(ctx, "test-project")
	require.NoError(t, err)

	files = nil
	require.NoError(t, json.Unmarshal(profile.TopFiles, &files))
	assert.Contains(t, extractFilePaths(files), "/src/target.go",
		"session 2: file should still be present (ssls=1)")

	// Session 3: "/src/target.go" not referenced.
	err = svc.UpdateProfile(ctx, "test-project")
	require.NoError(t, err)

	profile, err = svc.GetProfile(ctx, "test-project")
	require.NoError(t, err)

	files = nil
	require.NoError(t, json.Unmarshal(profile.TopFiles, &files))
	assert.Contains(t, extractFilePaths(files), "/src/target.go",
		"session 3: file should still be present (ssls=2)")

	// Session 4: "/src/target.go" not referenced, ssls becomes 3 -> DROPPED.
	err = svc.UpdateProfile(ctx, "test-project")
	require.NoError(t, err)

	profile, err = svc.GetProfile(ctx, "test-project")
	require.NoError(t, err)

	files = nil
	require.NoError(t, json.Unmarshal(profile.TopFiles, &files))
	assert.NotContains(t, extractFilePaths(files), "/src/target.go",
		"session 4: file should be dropped (ssls >= 3)")
}

// extractFilePaths is a test helper that extracts file paths from a slice of
// FileEntry for easier assertion.
func extractFilePaths(entries []FileEntry) []string {
	paths := make([]string, len(entries))
	for i, e := range entries {
		paths[i] = e.Path
	}
	return paths
}

// =============================================================================
// T029: Common Error Detection (US3)
// =============================================================================

// TestProfileService_CommonErrorDetection verifies that 3 bug-type observations
// with overlapping files and similar descriptions produce a single CommonError
// entry (FR-006, SC-005).
func TestProfileService_CommonErrorDetection(t *testing.T) {
	ctx := context.Background()

	var mu sync.Mutex
	var capturedParams store.UpsertProfileParams

	mockQ := &mockProfileQuerier{
		listObservationsByProject: func(ctx context.Context, projectSlug string) ([]store.Observation, error) {
			return []store.Observation{
				{
					ID: "bug1", Type: "bug",
					Files:     []string{"/src/auth.go"},
					Narrative: "JWT expiry not checked before parsing",
					SessionID: "sess1",
				},
				{
					ID: "bug2", Type: "bug",
					Files:     []string{"/src/auth.go"},
					Narrative: "JWT token expiry check missing prior to parsing",
					SessionID: "sess1",
				},
				{
					ID: "bug3", Type: "bug",
					Files:     []string{"/src/auth.go", "/src/middleware.go"},
					Narrative: "JWT expiry not validated before use",
					SessionID: "sess2",
				},
			}, nil
		},
		upsertProfile: func(ctx context.Context, params store.UpsertProfileParams) error {
			mu.Lock()
			defer mu.Unlock()
			capturedParams = params
			return nil
		},
		getProfile: func(ctx context.Context, projectSlug string) (store.ProjectProfile, error) {
			mu.Lock()
			defer mu.Unlock()
			if len(capturedParams.TopConcepts) == 0 && len(capturedParams.CommonErrors) == 0 {
				return store.ProjectProfile{}, fmt.Errorf("no profile")
			}
			return store.ProjectProfile{
				ProjectSlug:  projectSlug,
				TopConcepts:  capturedParams.TopConcepts,
				TopFiles:     capturedParams.TopFiles,
				Conventions:  capturedParams.Conventions,
				CommonErrors: capturedParams.CommonErrors,
			}, nil
		},
	}

	svc := newProfileServiceWithQuerier(mockQ)

	err := svc.UpdateProfile(ctx, "test-project")
	require.NoError(t, err)

	profile, err := svc.GetProfile(ctx, "test-project")
	require.NoError(t, err)

	var commonErrors []commonErrorJSON
	require.NoError(t, json.Unmarshal(profile.CommonErrors, &commonErrors))

	require.Len(t, commonErrors, 1, "should have exactly 1 common error entry")
	assert.Equal(t, 3, commonErrors[0].Occurrences, "common error should have 3 occurrences")
	assert.Contains(t, commonErrors[0].Files, "/src/auth.go", "files should include /src/auth.go")
	assert.Equal(t, 0, commonErrors[0].SessionsSinceLastSeen, "ssls should be 0 for new error")
}

// =============================================================================
// T030: Below-Threshold Filtering (US3)
// =============================================================================

// TestProfileService_BelowThresholdFiltering verifies that 2 bug observations
// with similar patterns produce NO common error entry (minimum 3 occurrences
// threshold per FR-006, SC-005).
func TestProfileService_BelowThresholdFiltering(t *testing.T) {
	ctx := context.Background()

	var mu sync.Mutex
	var capturedParams store.UpsertProfileParams

	mockQ := &mockProfileQuerier{
		listObservationsByProject: func(ctx context.Context, projectSlug string) ([]store.Observation, error) {
			return []store.Observation{
				{
					ID: "bug1", Type: "bug",
					Files:     []string{"/src/auth.go"},
					Narrative: "JWT expiry not checked before parsing",
					SessionID: "sess1",
				},
				{
					ID: "bug2", Type: "bug",
					Files:     []string{"/src/auth.go"},
					Narrative: "JWT token expiry check missing prior to parsing",
					SessionID: "sess1",
				},
			}, nil
		},
		upsertProfile: func(ctx context.Context, params store.UpsertProfileParams) error {
			mu.Lock()
			defer mu.Unlock()
			capturedParams = params
			return nil
		},
		getProfile: func(ctx context.Context, projectSlug string) (store.ProjectProfile, error) {
			mu.Lock()
			defer mu.Unlock()
			if len(capturedParams.TopConcepts) == 0 && len(capturedParams.CommonErrors) == 0 {
				return store.ProjectProfile{}, fmt.Errorf("no profile")
			}
			return store.ProjectProfile{
				ProjectSlug:  projectSlug,
				TopConcepts:  capturedParams.TopConcepts,
				TopFiles:     capturedParams.TopFiles,
				Conventions:  capturedParams.Conventions,
				CommonErrors: capturedParams.CommonErrors,
			}, nil
		},
	}

	svc := newProfileServiceWithQuerier(mockQ)

	err := svc.UpdateProfile(ctx, "test-project")
	require.NoError(t, err)

	profile, err := svc.GetProfile(ctx, "test-project")
	require.NoError(t, err)

	var commonErrors []commonErrorJSON
	require.NoError(t, json.Unmarshal(profile.CommonErrors, &commonErrors))

	assert.Empty(t, commonErrors, "should have no common error entries with only 2 occurrences")
}

// =============================================================================
// T031: Common Error Surfacing in Context (US3)
// =============================================================================

// TestProfileService_CommonErrorSurfacing verifies that 3 bug observations
// mentioning the same error pattern produce a profile whose buildProfileSection
// output includes the common error warning within an <agentmemory-past-errors> block.
func TestProfileService_CommonErrorSurfacing(t *testing.T) {
	ctx := context.Background()

	var mu sync.Mutex
	var capturedParams store.UpsertProfileParams

	mockQ := &mockProfileQuerier{
		listObservationsByProject: func(ctx context.Context, projectSlug string) ([]store.Observation, error) {
			return []store.Observation{
				{
					ID: "bug1", Type: "bug",
					Files:     []string{"/src/auth.go"},
					Narrative: "JWT expiry not checked before parsing",
					SessionID: "sess1",
				},
				{
					ID: "bug2", Type: "bug",
					Files:     []string{"/src/auth.go"},
					Narrative: "JWT token expiry check missing prior to parsing",
					SessionID: "sess1",
				},
				{
					ID: "bug3", Type: "bug",
					Files:     []string{"/src/auth.go"},
					Narrative: "JWT expiry handling is missing",
					SessionID: "sess2",
				},
			}, nil
		},
		upsertProfile: func(ctx context.Context, params store.UpsertProfileParams) error {
			mu.Lock()
			defer mu.Unlock()
			capturedParams = params
			return nil
		},
		getProfile: func(ctx context.Context, projectSlug string) (store.ProjectProfile, error) {
			mu.Lock()
			defer mu.Unlock()
			if len(capturedParams.TopConcepts) == 0 && len(capturedParams.CommonErrors) == 0 {
				return store.ProjectProfile{}, fmt.Errorf("no profile")
			}
			return store.ProjectProfile{
				ProjectSlug:  projectSlug,
				TopConcepts:  capturedParams.TopConcepts,
				TopFiles:     capturedParams.TopFiles,
				Conventions:  capturedParams.Conventions,
				CommonErrors: capturedParams.CommonErrors,
			}, nil
		},
	}

	svc := newProfileServiceWithQuerier(mockQ)

	err := svc.UpdateProfile(ctx, "test-project")
	require.NoError(t, err)

	profile, err := svc.GetProfile(ctx, "test-project")
	require.NoError(t, err)

	// buildProfileSection should include common errors in the output.
	section := buildProfileSection(profile)
	assert.Contains(t, section, "<agentmemory-past-errors>",
		"buildProfileSection should include <agentmemory-past-errors> block")
	assert.Contains(t, section, "JWT",
		"buildProfileSection should include the error pattern text")
	assert.Contains(t, section, "occurrences",
		"buildProfileSection should include occurrence count")
}

// =============================================================================
// T032: Common Error Decay (US3)
// =============================================================================

// TestProfileService_CommonErrorDecay verifies that a common error with no new
// occurrences for 5 sessions is removed from common_errors.
func TestProfileService_CommonErrorDecay(t *testing.T) {
	ctx := context.Background()

	var mu sync.Mutex
	var capturedParams store.UpsertProfileParams
	callNum := 0

	// Session 1: 3 bug observations → common error created (ssls=0).
	session1Obs := []store.Observation{
		{
			ID: "bug1", Type: "bug",
			Files:     []string{"/src/auth.go"},
			Narrative: "JWT expiry not checked before parsing",
			SessionID: "session1",
		},
		{
			ID: "bug2", Type: "bug",
			Files:     []string{"/src/auth.go"},
			Narrative: "JWT token expiry check missing prior to parsing",
			SessionID: "session1",
		},
		{
			ID: "bug3", Type: "bug",
			Files:     []string{"/src/auth.go"},
			Narrative: "JWT expiry not validated before use",
			SessionID: "session1",
		},
	}

	// Sessions 2-6: non-bug observations (common error decays).
	sessionN := []store.Observation{
		{
			ID: "n1", Type: "feature",
			Concepts:  []string{"other"},
			SessionID: "",
		},
	}

	observationSets := [][]store.Observation{
		session1Obs, sessionN, sessionN, sessionN, sessionN, sessionN,
	}

	mockQ := &mockProfileQuerier{
		listObservationsByProject: func(ctx context.Context, projectSlug string) ([]store.Observation, error) {
			mu.Lock()
			defer mu.Unlock()
			if callNum >= len(observationSets) {
				return observationSets[len(observationSets)-1], nil
			}
			result := observationSets[callNum]
			callNum++
			return result, nil
		},
		upsertProfile: func(ctx context.Context, params store.UpsertProfileParams) error {
			mu.Lock()
			defer mu.Unlock()
			capturedParams = params
			return nil
		},
		getProfile: func(ctx context.Context, projectSlug string) (store.ProjectProfile, error) {
			mu.Lock()
			defer mu.Unlock()
			if len(capturedParams.TopConcepts) == 0 && len(capturedParams.CommonErrors) == 0 {
				return store.ProjectProfile{}, fmt.Errorf("no profile")
			}
			return store.ProjectProfile{
				ProjectSlug:  projectSlug,
				TopConcepts:  capturedParams.TopConcepts,
				TopFiles:     capturedParams.TopFiles,
				Conventions:  capturedParams.Conventions,
				CommonErrors: capturedParams.CommonErrors,
			}, nil
		},
	}

	svc := newProfileServiceWithQuerier(mockQ)

	// Session 1: 3 bug observations → common error should appear (ssls=0).
	err := svc.UpdateProfile(ctx, "test-project")
	require.NoError(t, err)

	profile, err := svc.GetProfile(ctx, "test-project")
	require.NoError(t, err)

	var commonErrors []commonErrorJSON
	require.NoError(t, json.Unmarshal(profile.CommonErrors, &commonErrors))
	require.Len(t, commonErrors, 1, "session 1: should have 1 common error")
	assert.Equal(t, 0, commonErrors[0].SessionsSinceLastSeen,
		"session 1: ssls should be 0 for newly created error")

	// Sessions 2-5: no bug observations → error persists with ssls incremented.
	for i := 1; i <= 4; i++ {
		err = svc.UpdateProfile(ctx, "test-project")
		require.NoError(t, err)

		profile, err = svc.GetProfile(ctx, "test-project")
		require.NoError(t, err)

		commonErrors = nil
		require.NoError(t, json.Unmarshal(profile.CommonErrors, &commonErrors))
		require.Len(t, commonErrors, 1,
			"session %d: error should still be present (ssls=%d)", i+1, i)
		assert.Equal(t, i, commonErrors[0].SessionsSinceLastSeen,
			"session %d: ssls should be %d", i+1, i)
	}

	// Session 6: no bug observations → error dropped (ssls >= 5).
	err = svc.UpdateProfile(ctx, "test-project")
	require.NoError(t, err)

	profile, err = svc.GetProfile(ctx, "test-project")
	require.NoError(t, err)

	commonErrors = nil
	require.NoError(t, json.Unmarshal(profile.CommonErrors, &commonErrors))
	assert.Empty(t, commonErrors,
		"session 6: common error should be dropped after 5 sessions without occurrences")
}

// =============================================================================
// T037+T038: Structured Logging & Metrics Counters
// =============================================================================

// TestProfileService_LoggingAndMetrics verifies that UpdateProfile produces
// structured log output (T037) and correctly increments metrics counters (T038)
// on success, failure, and empty-observations paths.
func TestProfileService_LoggingAndMetrics(t *testing.T) {
	ctx := context.Background()

	// ---- Setup: capture slog output ----
	handler := &captureLogHandler{enabled: true}
	oldLogger := slog.Default()
	slog.SetDefault(slog.New(handler))
	defer slog.SetDefault(oldLogger)

	// ---- Success path ----
	successMock := &mockProfileQuerier{
		listObservationsByProject: func(ctx context.Context, projectSlug string) ([]store.Observation, error) {
			return []store.Observation{
				{ID: "obs1", Concepts: []string{"database"}, Narrative: "testing logging", SessionID: "sess1"},
			}, nil
		},
		upsertProfile: func(ctx context.Context, params store.UpsertProfileParams) error {
			return nil
		},
		getProfile: func(ctx context.Context, projectSlug string) (store.ProjectProfile, error) {
			return store.ProjectProfile{}, fmt.Errorf("no profile")
		},
	}

	svc := newProfileServiceWithQuerier(successMock)

	// Verify initial counters are zero.
	assert.Equal(t, int64(0), svc.UpdateSuccessCount(),
		"initial success count should be 0")
	assert.Equal(t, int64(0), svc.UpdateFailureCount(),
		"initial failure count should be 0")
	assert.Equal(t, int64(0), svc.LastUpdateLatencyMs(),
		"initial latency should be 0")

	err := svc.UpdateProfile(ctx, "test-project")
	require.NoError(t, err)

	// Metrics: success incremented, failure unchanged, latency recorded.
	assert.Equal(t, int64(1), svc.UpdateSuccessCount(),
		"success count should be 1 after successful update")
	assert.Equal(t, int64(0), svc.UpdateFailureCount(),
		"failure count should remain 0 after successful update")
	assert.GreaterOrEqual(t, svc.LastUpdateLatencyMs(), int64(0),
		"latency should be >= 0 after update")

	// Logging: INFO level, outcome=success, project_slug, observations_processed.
	assert.True(t, handler.logContains("profile updated"),
		"success log should contain 'profile updated'")
	assert.True(t, handler.logContains("outcome=success"),
		"success log should contain 'outcome=success'")
	assert.True(t, handler.logContains("project_slug=test-project"),
		"success log should contain project slug")
	assert.True(t, handler.logContains("observations_processed=1"),
		"success log should contain observations_processed count")

	// ---- Failure path ----
	handler.reset()
	failMock := &mockProfileQuerier{
		listObservationsByProject: func(ctx context.Context, projectSlug string) ([]store.Observation, error) {
			return nil, fmt.Errorf("database unreachable")
		},
		upsertProfile: func(ctx context.Context, params store.UpsertProfileParams) error {
			return nil
		},
		getProfile: func(ctx context.Context, projectSlug string) (store.ProjectProfile, error) {
			return store.ProjectProfile{}, fmt.Errorf("no profile")
		},
	}

	svc2 := newProfileServiceWithQuerier(failMock)

	err = svc2.UpdateProfile(ctx, "test-project")
	require.Error(t, err)

	// Metrics: failure incremented, success stays 0, latency recorded.
	assert.Equal(t, int64(0), svc2.UpdateSuccessCount(),
		"success count should remain 0 after failed update")
	assert.Equal(t, int64(1), svc2.UpdateFailureCount(),
		"failure count should be 1 after failed update")
	assert.GreaterOrEqual(t, svc2.LastUpdateLatencyMs(), int64(0),
		"latency should be >= 0 after failed update")

	// Logging: WARN level, outcome=failure, project_slug, error message.
	assert.True(t, handler.logContains("profile update failed"),
		"failure log should contain 'profile update failed'")
	assert.True(t, handler.logContains("outcome=failure"),
		"failure log should contain 'outcome=failure'")
	assert.True(t, handler.logContains("project_slug=test-project"),
		"failure log should contain project slug")
	assert.True(t, handler.logContains("database unreachable"),
		"failure log should contain the error message")

	// ---- Empty observations path (no-op, no log, no counter change) ----
	handler.reset()
	emptyMock := &mockProfileQuerier{
		listObservationsByProject: func(ctx context.Context, projectSlug string) ([]store.Observation, error) {
			return []store.Observation{}, nil
		},
		upsertProfile: func(ctx context.Context, params store.UpsertProfileParams) error {
			t.Error("UpsertProfile should not be called for empty observations")
			return nil
		},
		getProfile: func(ctx context.Context, projectSlug string) (store.ProjectProfile, error) {
			return store.ProjectProfile{}, fmt.Errorf("no profile")
		},
	}

	svc3 := newProfileServiceWithQuerier(emptyMock)

	err = svc3.UpdateProfile(ctx, "test-project")
	require.NoError(t, err)

	// Counters unchanged.
	assert.Equal(t, int64(0), svc3.UpdateSuccessCount(),
		"success count should remain 0 for empty observations")
	assert.Equal(t, int64(0), svc3.UpdateFailureCount(),
		"failure count should remain 0 for empty observations")

	// No log output for empty observations.
	assert.False(t, handler.logContains("profile updated"),
		"no success log should appear for empty observations")
	assert.False(t, handler.logContains("profile update failed"),
		"no failure log should appear for empty observations")
}

// TestProfileService_ConventionsSectionInProfile verifies that
// buildProfileSection includes a ### Conventions section when the profile
// contains conventions, and skips it when conventions are empty (T040).
func TestProfileService_ConventionsSectionInProfile(t *testing.T) {
	// Profile with conventions.
	profile := &store.ProjectProfile{
		ProjectSlug: "test-project",
		TopConcepts: []byte("[]"),
		TopFiles:    []byte("[]"),
		Conventions: []string{"Use TDD for all new features", "Run gofmt before commits"},
	}

	section := buildProfileSection(profile)
	assert.Contains(t, section, "### Conventions",
		"buildProfileSection should include '### Conventions' section")
	assert.Contains(t, section, "Use TDD for all new features",
		"should list first convention as bullet point")
	assert.Contains(t, section, "Run gofmt before commits",
		"should list second convention as bullet point")

	// Profile with empty conventions — no conventions section.
	profileEmpty := &store.ProjectProfile{
		ProjectSlug: "test-project",
		TopConcepts: []byte("[]"),
		TopFiles:    []byte("[]"),
	}

	sectionEmpty := buildProfileSection(profileEmpty)
	assert.NotContains(t, sectionEmpty, "### Conventions",
		"empty profile should not contain conventions section")
	assert.Empty(t, sectionEmpty,
		"empty profile with no concepts/files/errors should return empty string")
}
