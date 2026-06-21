package service

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/agentmemory/agentmemory/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RoutineRun represents an instantiated run of a routine template.
type RoutineRun struct {
	ID          string          `json:"id"`
	RoutineID   string          `json:"routine_id"`
	Project     string          `json:"project"`
	InitiatedBy string          `json:"initiated_by"`
	Actions     []RoutineAction `json:"actions"`
	CreatedAt   time.Time       `json:"created_at"`
}

// RoutineAction represents a single step within a routine run.
type RoutineAction struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Step        int    `json:"step"`
	DependsOn   []int  `json:"depends_on"`
}

// Pre-defined routine templates. Each maps a routine ID to a list of step titles.
var routineTemplates = map[string][]string{
	"tdd":         {"write_test", "run_test_red", "implement", "run_test_green", "refactor"},
	"code_review": {"read_diff", "analyze_changes", "check_bugs", "check_simplify", "submit_feedback"},
	"bug_fix":     {"reproduce", "diagnose", "fix", "verify", "add_tests"},
}

// RoutineService manages routine run instantiation and lifecycle.
type RoutineService struct {
	queries *store.Queries
	mu      sync.RWMutex
	runs    map[string]*RoutineRun
}

// NewRoutineService creates a new RoutineService backed by the given connection pool.
func NewRoutineService(pool *pgxpool.Pool) *RoutineService {
	return &RoutineService{
		queries: store.New(pool),
		runs:    make(map[string]*RoutineRun),
	}
}

// RunRoutine instantiates a new run of a pre-defined routine template.
// It creates RoutineActions for each step with proper Step numbers and DependsOn chains.
func (s *RoutineService) RunRoutine(ctx context.Context, routineID, project, initiatedBy string) (*RoutineRun, error) {
	titles, ok := routineTemplates[routineID]
	if !ok {
		return nil, fmt.Errorf("unknown routine: %s (available: tdd, code_review, bug_fix)", routineID)
	}

	run := &RoutineRun{
		ID:          uuid.New().String(),
		RoutineID:   routineID,
		Project:     project,
		InitiatedBy: initiatedBy,
		Actions:     make([]RoutineAction, 0, len(titles)),
		CreatedAt:   time.Now().UTC(),
	}

	for i, title := range titles {
		step := i + 1
		var dependsOn []int
		if step > 1 {
			dependsOn = []int{step - 1}
		}

		action := RoutineAction{
			ID:          uuid.New().String(),
			Title:       title,
			Description: fmt.Sprintf("Step %d: %s (routine: %s)", step, title, routineID),
			Step:        step,
			DependsOn:   dependsOn,
		}
		run.Actions = append(run.Actions, action)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.runs[run.ID] = run

	log.Printf("routine run instantiated: id=%s routine=%s project=%s initiated_by=%s actions=%d",
		run.ID, run.RoutineID, run.Project, run.InitiatedBy, len(run.Actions))

	return run, nil
}
