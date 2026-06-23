package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockFileHistoryQuerier implements fileHistoryQuerier for testing.
type mockFileHistoryQuerier struct {
	entries []FileHistoryEntry
	err     error
}

func (m *mockFileHistoryQuerier) getFileHistory(ctx context.Context, files []string, excludeSessionID string) ([]FileHistoryEntry, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.entries, nil
}

func TestFileHistory_GetFileHistory_ReturnsEntries(t *testing.T) {
	now := time.Now().UTC()
	mock := &mockFileHistoryQuerier{
		entries: []FileHistoryEntry{
			{File: "file1.go", ObservationID: "obs-1", Title: "added feature", Narrative: "added login handler", Timestamp: now, SessionID: "sess-1"},
			{File: "file2.go", ObservationID: "obs-2", Title: "fixed bug", Narrative: "fixed nil pointer", Timestamp: now, SessionID: "sess-2"},
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
		entries: []FileHistoryEntry{},
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
