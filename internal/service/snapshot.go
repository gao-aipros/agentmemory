package service

import (
	"context"
	"log"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/agentmemory/agentmemory/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Snapshot represents a versioned snapshot of the system's memory state.
type Snapshot struct {
	ID               string    `json:"id"`
	Message          string    `json:"message"`
	Timestamp        time.Time `json:"timestamp"`
	GitSHA           string    `json:"git_sha"`
	ObservationCount int       `json:"observation_count"`
}

// SnapshotService manages snapshot creation and retrieval with git versioning.
type SnapshotService struct {
	queries   *store.Queries
	mu        sync.RWMutex
	snapshots map[string]*Snapshot
}

// NewSnapshotService creates a new SnapshotService backed by the given connection pool.
func NewSnapshotService(pool *pgxpool.Pool) *SnapshotService {
	return &SnapshotService{
		queries:   store.New(pool),
		snapshots: make(map[string]*Snapshot),
	}
}

// resolveGitSHA attempts to get the current HEAD commit SHA via git rev-parse.
// Returns "unknown" if git is not available or the command fails.
func resolveGitSHA() string {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		log.Printf("snapshot: unable to resolve git SHA: %v (set to 'unknown')", err)
		return "unknown"
	}
	return strings.TrimSpace(string(output))
}

// CreateSnapshot creates a new versioned snapshot of the current memory state.
// It resolves the current git HEAD SHA for versioning and stores the snapshot
// metadata in-memory for MVP. Returns the created snapshot.
func (s *SnapshotService) CreateSnapshot(ctx context.Context, message string) (*Snapshot, error) {
	gitSHA := resolveGitSHA()

	snap := &Snapshot{
		ID:               uuid.New().String(),
		Message:          message,
		Timestamp:        time.Now().UTC(),
		GitSHA:           gitSHA,
		ObservationCount: 0, // MVP: observation count tracking deferred to DB integration
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.snapshots[snap.ID] = snap

	log.Printf("snapshot created: id=%s message=%s git_sha=%.8s",
		snap.ID, snap.Message, snap.GitSHA)

	return snap, nil
}
