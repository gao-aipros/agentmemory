package store

import (
	"context"
)

// ListObservationsByProject returns all observations.
// NOTE: The observations table has NO project column, so this function
// currently returns ALL observations without project-scoped filtering.
// The projectSlug parameter is accepted but ignored.
//
// To implement proper project-level filtering (FR-013), a schema migration
// is required to add a project_slug column to the observations table
// (or to the sessions table with a JOIN). Until then, this function
// returns all observations and relies on the caller's UpdateProfile logic
// to derive project-relevant concepts from the full set.
func (q *Queries) ListObservationsByProject(ctx context.Context, projectSlug string) ([]Observation, error) {
	rows, err := q.db.Query(ctx, `SELECT id, session_id, owner_type, owner_user_id, owner_team_id, visibility, type, title, narrative, facts, concepts, files, importance, timestamp, created_at, compressed_at FROM observations ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []Observation
	for rows.Next() {
		var i Observation
		if err := rows.Scan(
			&i.ID,
			&i.SessionID,
			&i.OwnerType,
			&i.OwnerUserID,
			&i.OwnerTeamID,
			&i.Visibility,
			&i.Type,
			&i.Title,
			&i.Narrative,
			&i.Facts,
			&i.Concepts,
			&i.Files,
			&i.Importance,
			&i.Timestamp,
			&i.CreatedAt,
			&i.CompressedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}
