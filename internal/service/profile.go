package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/agentmemory/agentmemory/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tmc/langchaingo/llms"
)

// profileQuerier defines the database methods ProfileService needs.
// The concrete *store.Queries satisfies this interface, enabling mock-based unit testing.
type profileQuerier interface {
	ListObservationsByProject(ctx context.Context, projectSlug string) ([]store.Observation, error)
	UpsertProfile(ctx context.Context, params store.UpsertProfileParams) error
	GetProfile(ctx context.Context, projectSlug string) (store.ProjectProfile, error)
}

// conceptJSON is the JSON structure for a single concept entry stored in the
// top_concepts JSONB column.
type conceptJSON struct {
	Name                 string    `json:"name"`
	Frequency            float64   `json:"frequency"`
	SessionsSeen         int       `json:"sessions_seen"`
	SessionsSinceLastSeen int      `json:"sessions_since_last_seen"`
	LastSeenAt           time.Time `json:"last_seen_at"`
}

// fileJSON is the JSON structure for a single file entry stored in the
// top_files JSONB column.
type fileJSON struct {
	Path                 string    `json:"path"`
	Count                int       `json:"count"`
	SessionsSeen         int       `json:"sessions_seen"`
	SessionsSinceLastSeen int      `json:"sessions_since_last_seen"`
	LastSeenAt           time.Time `json:"last_seen_at"`
}

// commonErrorJSON is the JSON structure for a single common error entry stored
// in the common_errors JSONB column.
type commonErrorJSON struct {
	Pattern              string    `json:"pattern"`
	Files                []string  `json:"files"`
	Occurrences          int       `json:"occurrences"`
	SessionsSeen         int       `json:"sessions_seen"`
	SessionsSinceLastSeen int      `json:"sessions_since_last_seen"`
	LastSeenAt           time.Time `json:"last_seen_at"`
}

// stopWords is a set of common English stop words filtered out during
// description similarity analysis for common error clustering.
var stopWords = map[string]bool{
	"the": true, "a": true, "an": true, "is": true, "are": true,
	"was": true, "were": true, "be": true, "been": true, "being": true,
	"have": true, "has": true, "had": true, "do": true, "does": true,
	"did": true, "but": true, "if": true, "or": true, "because": true,
	"as": true, "until": true, "while": true, "of": true, "at": true,
	"by": true, "for": true, "with": true, "about": true, "against": true,
	"between": true, "into": true, "through": true, "during": true,
	"before": true, "after": true, "above": true, "below": true,
	"to": true, "from": true, "up": true, "down": true, "in": true,
	"out": true, "on": true, "off": true, "over": true, "under": true,
	"again": true, "further": true, "then": true, "once": true,
	"here": true, "there": true, "when": true, "where": true, "why": true,
	"how": true, "all": true, "any": true, "both": true, "each": true,
	"few": true, "more": true, "most": true, "other": true, "some": true,
	"such": true, "no": true, "nor": true, "not": true, "only": true,
	"own": true, "same": true, "so": true, "than": true, "too": true,
	"very": true, "just": true, "it": true, "its": true, "that": true,
	"this": true, "these": true, "those": true, "we": true, "our": true,
	"you": true, "your": true, "they": true, "them": true, "their": true,
	"what": true, "which": true, "who": true, "whom": true,
}

// extractSignificantWords splits a string into lowercase tokens, filtering out
// stop words and single-character tokens.
func extractSignificantWords(s string) []string {
	raw := strings.Fields(strings.ToLower(s))
	words := make([]string, 0, len(raw))
	for _, w := range raw {
		// Strip punctuation
		w = strings.Trim(w, ".,!?;:\"'-()[]{}")
		if len(w) <= 1 || stopWords[w] {
			continue
		}
		words = append(words, w)
	}
	return words
}

// descriptionsSimilar returns true when two strings share enough significant
// words (Jaccard similarity > 0.3) to be considered the same error pattern.
func descriptionsSimilar(a, b string) bool {
	wordsA := extractSignificantWords(a)
	wordsB := extractSignificantWords(b)

	if len(wordsA) == 0 || len(wordsB) == 0 {
		return false
	}

	// Build a set for wordsA.
	setA := make(map[string]bool, len(wordsA))
	for _, w := range wordsA {
		setA[w] = true
	}

	// Count intersection (deduplicating wordsB — Jaccard operates on sets).
	intersection := 0
	for _, w := range wordsB {
		if setA[w] {
			intersection++
			delete(setA, w) // remove so duplicates in wordsB don't inflate score
		}
	}

	union := len(wordsA) + len(wordsB) - intersection
	if union == 0 {
		return false
	}

	return float64(intersection)/float64(union) > 0.3
}

// bugCluster holds a group of bug observations that share file paths and have
// similar error descriptions.
type bugCluster struct {
	observations []store.Observation
	files        []string
}

// clusterBugObservations groups bug observations by overlapping file paths and
// then clusters by similar error descriptions within each file group.
func clusterBugObservations(obs []store.Observation) []bugCluster {
	if len(obs) == 0 {
		return nil
	}

	// Step 1: build adjacency by file overlap (connected components).
	n := len(obs)
	adj := make([][]int, n)
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			if shareAnyFile(obs[i].Files, obs[j].Files) {
				adj[i] = append(adj[i], j)
				adj[j] = append(adj[j], i)
			}
		}
	}

	// Find connected components (file-overlap groups).
	visited := make([]bool, n)
	var groups [][]int
	for i := 0; i < n; i++ {
		if visited[i] {
			continue
		}
		var group []int
		stack := []int{i}
		visited[i] = true
		for len(stack) > 0 {
			v := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			group = append(group, v)
			for _, u := range adj[v] {
				if !visited[u] {
					visited[u] = true
					stack = append(stack, u)
				}
			}
		}
		groups = append(groups, group)
	}

	// Step 2: within each file group, cluster by description similarity.
	var clusters []bugCluster
	for _, group := range groups {
		// Build description adjacency within this file group.
		descAdj := make([][]int, len(group))
		for i := 0; i < len(group); i++ {
			for j := i + 1; j < len(group); j++ {
				if descriptionsSimilar(obs[group[i]].Narrative, obs[group[j]].Narrative) {
					descAdj[i] = append(descAdj[i], j)
					descAdj[j] = append(descAdj[j], i)
				}
			}
		}

		// Find description-similarity sub-components.
		descVisited := make([]bool, len(group))
		for i := 0; i < len(group); i++ {
			if descVisited[i] {
				continue
			}
			var subGroup []int
			stack := []int{i}
			descVisited[i] = true
			for len(stack) > 0 {
				v := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				subGroup = append(subGroup, group[v])
				for _, u := range descAdj[v] {
					if !descVisited[u] {
						descVisited[u] = true
						stack = append(stack, u)
					}
				}
			}

			// Collect unique files across all observations in this cluster.
			fileSet := make(map[string]bool)
			for _, idx := range subGroup {
				for _, f := range obs[idx].Files {
					fileSet[f] = true
				}
			}
			clusterFiles := make([]string, 0, len(fileSet))
			for f := range fileSet {
				clusterFiles = append(clusterFiles, f)
			}

			clusterObs := make([]store.Observation, len(subGroup))
			for k, idx := range subGroup {
				clusterObs[k] = obs[idx]
			}

			clusters = append(clusters, bugCluster{
				observations: clusterObs,
				files:        clusterFiles,
			})
		}
	}

	return clusters
}

// derivePattern extracts a representative error pattern from a cluster of
// bug observations. Uses the most common significant words or the narrative
// with the most significant words.
func derivePattern(observations []store.Observation) string {
	if len(observations) == 0 {
		return ""
	}

	// Pick the observation with the longest narrative as the representative.
	best := observations[0]
	for _, obs := range observations[1:] {
		if len(obs.Narrative) > len(best.Narrative) {
			best = obs
		}
	}
	return best.Narrative
}

// shareAnyFile returns true when at least one file path exists in both slices.
func shareAnyFile(a, b []string) bool {
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	set := make(map[string]bool, len(a))
	for _, f := range a {
		set[f] = true
	}
	for _, f := range b {
		if set[f] {
			return true
		}
	}
	return false
}

// ProfileService manages auto-learned project profiles that track top concepts,
// key files, conventions, and common errors from observation patterns across
// all sessions within a project.
//
// Profile updates are serialized per project (via muMap) to prevent concurrent
// consolidation cycles from producing inconsistent profile state.
type ProfileService struct {
	pool    *pgxpool.Pool
	queries profileQuerier
	model   llms.Model

	// muMap provides per-project mutexes for serializing profile updates.
	muMap    map[string]*sync.Mutex
	muMapLock sync.RWMutex

	// Metrics counters (atomic).
	updateSuccess    int64
	updateFailure    int64
	updateLatencyMs  int64
}

// NewProfileService creates a new ProfileService.
// The model parameter is reserved for convention extraction (FR-005) and is
// currently unused in the skeleton.
func NewProfileService(pool *pgxpool.Pool, model llms.Model) *ProfileService {
	return &ProfileService{
		pool:    pool,
		queries: store.New(pool),
		model:   model,
		muMap:   make(map[string]*sync.Mutex),
	}
}

// newProfileServiceWithQuerier creates a ProfileService with a custom querier (for testing).
func newProfileServiceWithQuerier(q profileQuerier) *ProfileService {
	return &ProfileService{
		queries: q,
		muMap:   make(map[string]*sync.Mutex),
	}
}

// UpdateProfile queries observations for the given project, computes concept
// frequencies and file reference counts, and stores the result via UpsertProfile.
// Returns nil (no error, no upsert) when there are no observations to process.
func (s *ProfileService) UpdateProfile(ctx context.Context, projectSlug string) error {
	startTime := time.Now()

	// Obtain per-project lock to serialize profile updates.
	s.muMapLock.Lock()
	mu, ok := s.muMap[projectSlug]
	if !ok {
		mu = &sync.Mutex{}
		s.muMap[projectSlug] = mu
	}
	s.muMapLock.Unlock()

	mu.Lock()
	defer mu.Unlock()

	// Query observations for the project.
	observations, err := s.queries.ListObservationsByProject(ctx, projectSlug)
	if err != nil {
		elapsed := time.Since(startTime).Milliseconds()
		atomic.AddInt64(&s.updateFailure, 1)
		atomic.StoreInt64(&s.updateLatencyMs, elapsed)
		slog.Warn("profile update failed",
			"project_slug", projectSlug,
			"error", err.Error(),
			"outcome", "failure",
		)
		return fmt.Errorf("failed to list observations: %w", err)
	}

	if len(observations) == 0 {
		return nil // no observations to process, skip upsert
	}


	// --- Load existing profile for sessions_since_last_seen tracking ---

	var existingConcepts []conceptJSON
	var existingFiles []fileJSON
	var existingErrors []commonErrorJSON

	existingProfile, err := s.queries.GetProfile(ctx, projectSlug)
	if err == nil {
		_ = json.Unmarshal(existingProfile.TopConcepts, &existingConcepts)
		_ = json.Unmarshal(existingProfile.TopFiles, &existingFiles)
		_ = json.Unmarshal(existingProfile.CommonErrors, &existingErrors)
	}

	// --- Build "seen in current batch" sets ---

	seenConcepts := make(map[string]bool)
	seenFiles := make(map[string]bool)
	for _, obs := range observations {
		for _, c := range obs.Concepts {
			seenConcepts[c] = true
		}
		for _, f := range obs.Files {
			seenFiles[f] = true
		}
	}

	// --- Build existing concept/file lookup maps ---

	existingConceptMap := make(map[string]conceptJSON, len(existingConcepts))
	for _, ec := range existingConcepts {
		existingConceptMap[ec.Name] = ec
	}
	existingFileMap := make(map[string]fileJSON, len(existingFiles))
	for _, ef := range existingFiles {
		existingFileMap[ef.Path] = ef
	}

	// --- Concept frequency counting ---

	conceptCount := make(map[string]int)               // raw occurrence count per concept
	conceptSessions := make(map[string]map[string]bool) // distinct sessions per concept
	var totalConceptOccurrences int

	for _, obs := range observations {
		for _, c := range obs.Concepts {
			conceptCount[c]++
			totalConceptOccurrences++
			if conceptSessions[c] == nil {
				conceptSessions[c] = make(map[string]bool)
			}
			conceptSessions[c][obs.SessionID] = true
		}
	}

	// --- File reference counting ---

	fileCount := make(map[string]int)
	fileSessions := make(map[string]map[string]bool)

	for _, obs := range observations {
		for _, f := range obs.Files {
			fileCount[f]++
			if fileSessions[f] == nil {
				fileSessions[f] = make(map[string]bool)
			}
			fileSessions[f][obs.SessionID] = true
		}
	}

	// --- Build concept list with recency decay ---

	concepts := make([]conceptJSON, 0)

	// Concepts seen in the current batch: fresh score, ssls = 0.
	for name, count := range conceptCount {
		rawFreq := float64(count) / float64(totalConceptOccurrences)
		score := rawFreq * (1.0 / (1.0 + 0.0)) // weight = 1.0 when ssls=0

		concepts = append(concepts, conceptJSON{
			Name:                 name,
			Frequency:            score,
			SessionsSeen:         len(conceptSessions[name]),
			SessionsSinceLastSeen: 0,
			LastSeenAt:           time.Now(),
		})
	}

	// Existing concepts NOT seen in the current batch: carry over with
	// incremented sessions_since_last_seen and compound decay. Drop if
	// sessions_since_last_seen >= 3.
	for name, existing := range existingConceptMap {
		if seenConcepts[name] {
			continue // already processed above
		}

		ssls := existing.SessionsSinceLastSeen + 1
		if ssls >= 3 {
			continue // drop — decay threshold reached
		}

		score := existing.Frequency * (1.0 / (1.0 + float64(ssls)))

		concepts = append(concepts, conceptJSON{
			Name:                 name,
			Frequency:            score,
			SessionsSeen:         existing.SessionsSeen,
			SessionsSinceLastSeen: ssls,
			LastSeenAt:           existing.LastSeenAt,
		})
	}

	// Sort by recency-weighted score descending, then name ascending.
	sort.Slice(concepts, func(i, j int) bool {
		if concepts[i].Frequency != concepts[j].Frequency {
			return concepts[i].Frequency > concepts[j].Frequency
		}
		return concepts[i].Name < concepts[j].Name
	})

	topConcepts, err := json.Marshal(concepts)
	if err != nil {
		elapsed := time.Since(startTime).Milliseconds()
		atomic.AddInt64(&s.updateFailure, 1)
		atomic.StoreInt64(&s.updateLatencyMs, elapsed)
		slog.Warn("profile update failed",
			"project_slug", projectSlug,
			"error", err.Error(),
			"outcome", "failure",
		)
		return fmt.Errorf("failed to marshal concepts: %w", err)
	}

	// --- Build file list with recency decay ---

	files := make([]fileJSON, 0)

	// Files seen in the current batch: fresh count, ssls = 0.
	for path, count := range fileCount {
		files = append(files, fileJSON{
			Path:                 path,
			Count:                count,
			SessionsSeen:         len(fileSessions[path]),
			SessionsSinceLastSeen: 0,
			LastSeenAt:           time.Now(),
		})
	}

	// Existing files NOT seen in the current batch: carry over with
	// incremented sessions_since_last_seen. Drop if ssls >= 3.
	for path, existing := range existingFileMap {
		if seenFiles[path] {
			continue // already processed above
		}

		ssls := existing.SessionsSinceLastSeen + 1
		if ssls >= 3 {
			continue // drop — decay threshold reached
		}

		files = append(files, fileJSON{
			Path:                 path,
			Count:                existing.Count,
			SessionsSeen:         existing.SessionsSeen,
			SessionsSinceLastSeen: ssls,
			LastSeenAt:           existing.LastSeenAt,
		})
	}

	// Sort by recency-weighted score (count * weight) descending, then path ascending.
	sort.Slice(files, func(i, j int) bool {
		wi := float64(files[i].Count) * (1.0 / (1.0 + float64(files[i].SessionsSinceLastSeen)))
		wj := float64(files[j].Count) * (1.0 / (1.0 + float64(files[j].SessionsSinceLastSeen)))
		if wi != wj {
			return wi > wj
		}
		return files[i].Path < files[j].Path
	})

	topFiles, err := json.Marshal(files)
	if err != nil {
		elapsed := time.Since(startTime).Milliseconds()
		atomic.AddInt64(&s.updateFailure, 1)
		atomic.StoreInt64(&s.updateLatencyMs, elapsed)
		slog.Warn("profile update failed",
			"project_slug", projectSlug,
			"error", err.Error(),
			"outcome", "failure",
		)
		return fmt.Errorf("failed to marshal files: %w", err)
	}

	// --- Common error detection ---

	commonErrors := make([]commonErrorJSON, 0)

	// Filter bug-type observations from the current batch.
	var bugObs []store.Observation
	for _, obs := range observations {
		if obs.Type == "bug" {
			bugObs = append(bugObs, obs)
		}
	}

	if len(bugObs) == 0 {
		// No bug observations in the current batch. Carry over existing errors
		// with incremented sessions_since_last_seen and drop decayed entries.
		for i := range existingErrors {
			existingErrors[i].SessionsSinceLastSeen++
		}
		for _, e := range existingErrors {
			if e.SessionsSinceLastSeen < 5 {
				commonErrors = append(commonErrors, e)
			}
		}
	} else {
		// Cluster bug observations by overlapping file paths and similar descriptions.
		clusters := clusterBugObservations(bugObs)

		// Track which existing errors have been matched by a new cluster.
		matchedExisting := make(map[int]bool)

		for _, cluster := range clusters {
			if len(cluster.observations) < 3 {
				continue // below minimum 3-occurrence threshold
			}

			// Count distinct sessions in the cluster.
			sessionsSet := make(map[string]bool)
			for _, obs := range cluster.observations {
				sessionsSet[obs.SessionID] = true
			}

			// Determine the error pattern from the cluster.
			pattern := derivePattern(cluster.observations)

			newErr := commonErrorJSON{
				Pattern:              pattern,
				Files:                cluster.files,
				Occurrences:          len(cluster.observations),
				SessionsSeen:         len(sessionsSet),
				SessionsSinceLastSeen: 0,
				LastSeenAt:           time.Now(),
			}

			// Try to match against existing errors for ssls tracking.
			for i, existing := range existingErrors {
				if matchedExisting[i] {
					continue
				}
				if descriptionsSimilar(pattern, existing.Pattern) && shareAnyFile(cluster.files, existing.Files) {
					newErr.SessionsSinceLastSeen = 0
					newErr.LastSeenAt = time.Now()
					matchedExisting[i] = true
					break
				}
			}

			commonErrors = append(commonErrors, newErr)
		}

		// Carry over unmatched existing errors with incremented ssls.
		for i, existing := range existingErrors {
			if matchedExisting[i] {
				continue
			}
			existing.SessionsSinceLastSeen++
			if existing.SessionsSinceLastSeen >= 5 {
				continue // drop — decay threshold reached
			}
			commonErrors = append(commonErrors, existing)
		}
	}

	commonErrorsBytes, err := json.Marshal(commonErrors)
	if err != nil {
		elapsed := time.Since(startTime).Milliseconds()
		atomic.AddInt64(&s.updateFailure, 1)
		atomic.StoreInt64(&s.updateLatencyMs, elapsed)
		slog.Warn("profile update failed",
			"project_slug", projectSlug,
			"error", err.Error(),
			"outcome", "failure",
		)
		return fmt.Errorf("failed to marshal common errors: %w", err)
	}

	// --- Convention extraction (FR-005) ---

	conventions := s.extractConventions(ctx, observations)

	// --- Upsert profile ---

	params := store.UpsertProfileParams{
		ProjectSlug:  projectSlug,
		TopConcepts:  topConcepts,
		TopFiles:     topFiles,
		Conventions:  conventions,
		CommonErrors: commonErrorsBytes,
	}

	err = s.queries.UpsertProfile(ctx, params)
	elapsed := time.Since(startTime).Milliseconds()
	if err != nil {
		atomic.AddInt64(&s.updateFailure, 1)
		atomic.StoreInt64(&s.updateLatencyMs, elapsed)
		slog.Warn("profile update failed",
			"project_slug", projectSlug,
			"error", err.Error(),
			"outcome", "failure",
		)
		return fmt.Errorf("failed to upsert profile: %w", err)
	}

	atomic.AddInt64(&s.updateSuccess, 1)
	atomic.StoreInt64(&s.updateLatencyMs, elapsed)
	slog.Info("profile updated",
		"project_slug", projectSlug,
		"observations_processed", len(observations),
		"outcome", "success",
	)
	return nil
}

// GetProfile retrieves the profile for a project by project_slug.
// Returns nil and an error when the profile is not found.
func (s *ProfileService) GetProfile(ctx context.Context, projectSlug string) (*store.ProjectProfile, error) {
	profile, err := s.queries.GetProfile(ctx, projectSlug)
	if err != nil {
		return nil, err
	}
	return &profile, nil
}

// UpdateSuccessCount returns the number of successful profile updates.
func (s *ProfileService) UpdateSuccessCount() int64 {
	return atomic.LoadInt64(&s.updateSuccess)
}

// UpdateFailureCount returns the number of failed profile updates.
func (s *ProfileService) UpdateFailureCount() int64 {
	return atomic.LoadInt64(&s.updateFailure)
}

// LastUpdateLatencyMs returns the latency (in milliseconds) of the last profile update.
func (s *ProfileService) LastUpdateLatencyMs() int64 {
	return atomic.LoadInt64(&s.updateLatencyMs)
}

// extractConventions extracts repeated procedural patterns from observation
// narratives by calling the LLM. Feature-gated behind PROFILE_CONVENTION_EXTRACTION
// env var (default "false"). Returns nil if the feature is disabled, model is nil,
// or fewer than 5 observations exist (heuristic pre-filter to reduce token cost).
func (s *ProfileService) extractConventions(ctx context.Context, observations []store.Observation) []string {
	if os.Getenv("PROFILE_CONVENTION_EXTRACTION") != "true" {
		return nil
	}
	if s.model == nil {
		return nil
	}
	if len(observations) < 5 {
		return nil
	}

	// Collect non-empty narratives.
	var narratives []string
	for _, obs := range observations {
		if obs.Narrative != "" {
			narratives = append(narratives, obs.Narrative)
		}
	}
	if len(narratives) < 5 {
		return nil
	}

	prompt := fmt.Sprintf(`Extract repeated procedural patterns and coding conventions from these session observations.
Return a JSON array of strings. Maximum 5 conventions.
Observations:
%s`, strings.Join(narratives, "\n"))

	resp, err := s.model.Call(ctx, prompt)
	if err != nil {
		return nil // graceful degradation
	}

	// Try JSON array parsing first.
	var conventions []string
	if err := json.Unmarshal([]byte(resp), &conventions); err != nil {
		// Fallback: line-by-line, stripping common LLM artifacts.
		conventions = parseConventionsFromText(resp)
	}

	// Cap at 5.
	if len(conventions) > 5 {
		conventions = conventions[:5]
	}

	return conventions
}

// parseConventionsFromText attempts to extract convention strings from free-text
// LLM output when JSON parsing fails.
func parseConventionsFromText(text string) []string {
	lines := strings.Split(text, "\n")
	var result []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Skip empty lines, markdown code fences, and JSON brackets/array markers.
		if line == "" || line == "```" || line == "```json" || line == "[" || line == "]" {
			continue
		}
		// Strip list markers.
		line = strings.TrimPrefix(line, "- ")
		line = strings.TrimPrefix(line, "* ")
		// Strip surrounding quotes and commas.
		line = strings.Trim(line, `",`)
		line = strings.TrimSpace(line)
		if line != "" {
			result = append(result, line)
		}
	}
	return result
}

// buildProfileSection formats profile data as a Markdown section suitable for
// context injection. Returns an empty string when profile is nil or contains
// no data. The output is constrained to PROFILE_TOKEN_BUDGET tokens (default 200).
func buildProfileSection(profile *store.ProjectProfile) string {
	if profile == nil {
		return ""
	}

	// Unmarshal concepts.
	type conceptEntry struct {
		Name      string  `json:"name"`
		Frequency float64 `json:"frequency"`
	}
	var concepts []conceptEntry
	if err := json.Unmarshal(profile.TopConcepts, &concepts); err != nil {
		return ""
	}

	// Unmarshal files.
	type fileEntry struct {
		Path  string `json:"path"`
		Count int    `json:"count"`
	}
	var files []fileEntry
	if err := json.Unmarshal(profile.TopFiles, &files); err != nil {
		return ""
	}

	// Unmarshal common errors.
	type errorEntry struct {
		Pattern     string   `json:"pattern"`
		Files       []string `json:"files"`
		Occurrences int      `json:"occurrences"`
	}
	var commonErrors []errorEntry
	if len(profile.CommonErrors) > 0 {
		_ = json.Unmarshal(profile.CommonErrors, &commonErrors)
	}

	if len(concepts) == 0 && len(files) == 0 && len(commonErrors) == 0 && len(profile.Conventions) == 0 {
		return ""
	}

	// Determine token budget from env var (default 200).
	budget := 200
	if v := os.Getenv("PROFILE_TOKEN_BUDGET"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			budget = n
		}
	}

	var b strings.Builder

	if len(concepts) > 0 {
		b.WriteString("### concepts\n")
		for _, c := range concepts {
			b.WriteString(fmt.Sprintf("- %s (frequency: %.3f)\n", c.Name, c.Frequency))
		}
	}

	if len(files) > 0 {
		if len(concepts) > 0 {
			b.WriteString("\n")
		}
		b.WriteString("### files\n")
		for _, f := range files {
			b.WriteString(fmt.Sprintf("- %s (count: %d)\n", f.Path, f.Count))
		}
	}

	if len(profile.Conventions) > 0 {
		if len(concepts) > 0 || len(files) > 0 {
			b.WriteString("\n")
		}
		b.WriteString("### Conventions\n")
		for _, c := range profile.Conventions {
			b.WriteString(fmt.Sprintf("- %s\n", c))
		}
	}

	if len(commonErrors) > 0 {
		if len(concepts) > 0 || len(files) > 0 || len(profile.Conventions) > 0 {
			b.WriteString("\n")
		}
		b.WriteString("<agentmemory-past-errors>\n")
		for _, e := range commonErrors {
			filesStr := strings.Join(e.Files, ", ")
			b.WriteString(fmt.Sprintf("- %s (files: %s) — %d occurrences\n", e.Pattern, filesStr, e.Occurrences))
		}
		b.WriteString("</agentmemory-past-errors>\n")
	}

	result := b.String()

	// Truncate to budget if needed.
	if EstimateTokens(result) > budget {
		result = truncateToTokens(result, budget)
	}

	return result
}
