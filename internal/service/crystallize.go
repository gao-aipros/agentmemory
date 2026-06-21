package service

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CrystalResult is the output of crystallizing a set of completed actions.
type CrystalResult struct {
	ID             string    `json:"id"`
	ActionIDs      []string  `json:"action_ids"`
	Narrative      string    `json:"narrative"`
	KeyOutcomes    string    `json:"key_outcomes"`
	FilesAffected  []string  `json:"files_affected"`
	Lessons        []string  `json:"lessons"`
	Project        string    `json:"project"`
	SessionID      string    `json:"session_id"`
	CreatedAt      time.Time `json:"created_at"`
}

// CrystallizeService compresses completed action chains into compact
// crystal digests for long-term retention.
type CrystallizeService struct {
	pool *pgxpool.Pool
}

// NewCrystallizeService creates a new CrystallizeService.
func NewCrystallizeService(pool *pgxpool.Pool) *CrystallizeService {
	return &CrystallizeService{
		pool: pool,
	}
}

// Crystallize compresses a set of completed actions into a crystal digest.
// The crystal summarizes the narrative across actions, key outcomes, affected
// files, and lessons learned.
func (s *CrystallizeService) Crystallize(ctx context.Context, actionIDs []string, project, sessionID string) (*CrystalResult, error) {
	if len(actionIDs) == 0 {
		return nil, fmt.Errorf("at least one action ID is required")
	}

	log.Printf("[INFO] crystallize: processing %d actions for project=%s session=%s",
		len(actionIDs), project, sessionID)

	// For MVP, build a crystal from the action IDs without LLM summarization.
	// Future: invoke an LLM to summarize the action chain narrative.
	narrative := fmt.Sprintf("Crystal digest of %d completed actions", len(actionIDs))
	keyOutcomes := fmt.Sprintf("Actions %v completed successfully", actionIDs)
	lessons := []string{
		"Auto-generated crystal from action chain",
		"Full LLM summarization pending in future release",
	}

	now := time.Now().UTC()

	result := &CrystalResult{
		ID:            uuid.New().String(),
		ActionIDs:     actionIDs,
		Narrative:     narrative,
		KeyOutcomes:   keyOutcomes,
		FilesAffected: []string{},
		Lessons:       lessons,
		Project:       project,
		SessionID:     sessionID,
		CreatedAt:     now,
	}

	log.Printf("[INFO] crystallize: created crystal %s from %d actions",
		result.ID, len(actionIDs))

	return result, nil
}
