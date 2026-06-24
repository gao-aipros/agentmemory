package service

import (
	"context"
	"testing"

	"github.com/agentmemory/agentmemory/internal/store"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockFileHistoryQuerier implements fileHistoryQuerier for testing.
type mockFileHistoryQuerier struct {
	rows []store.GetFileHistoryRow
	err  error
}

func (m *mockFileHistoryQuerier) GetFileHistory(ctx context.Context, arg store.GetFileHistoryParams) ([]store.GetFileHistoryRow, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.rows, nil
}

func TestFileHistory_GetFileHistory_ReturnsEntries(t *testing.T) {
	now := pgtype.Timestamptz{Valid: true}
	mock := &mockFileHistoryQuerier{
		rows: []store.GetFileHistoryRow{
			{ID: "obs-1", SessionID: "sess-1", Title: "added feature", Narrative: "added login handler", Files: []string{"file1.go"}, Timestamp: now},
			{ID: "obs-2", SessionID: "sess-2", Title: "fixed bug", Narrative: "fixed nil pointer", Files: []string{"file2.go"}, Timestamp: now},
		},
	}
	svc := newFileHistoryServiceWithQuerier(mock)

	entries, err := svc.GetFileHistory(context.Background(), []string{"file1.go", "file2.go"}, "sess-3")
	require.NoError(t, err)
	assert.Len(t, entries, 2)
	assert.Equal(t, "obs-1", entries[0].ObservationID)
	assert.Equal(t, "file1.go", entries[0].File)
	assert.Equal(t, "sess-2", entries[1].SessionID)
}

func TestFileHistory_GetFileHistory_EmptyResult(t *testing.T) {
	mock := &mockFileHistoryQuerier{
		rows: []store.GetFileHistoryRow{},
	}
	svc := newFileHistoryServiceWithQuerier(mock)

	entries, err := svc.GetFileHistory(context.Background(), []string{"nonexistent.go"}, "sess-1")
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestFileHistory_GetFileHistory_ErrorPropagation(t *testing.T) {
	mock := &mockFileHistoryQuerier{
		err: assert.AnError,
	}
	svc := newFileHistoryServiceWithQuerier(mock)

	_, err := svc.GetFileHistory(context.Background(), []string{"file.go"}, "sess-1")
	require.Error(t, err)
	assert.ErrorIs(t, err, assert.AnError)
}
