package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConsolidationService_BatchInsertMemories_Empty(t *testing.T) {
	svc := &ConsolidationService{}
	err := svc.batchInsertMemories(context.Background(), nil, "user-1", "team-1", "private")
	assert.NoError(t, err, "batch insert of empty memories should not error")
}

func TestConsolidationService_BatchInsertLessons_Empty(t *testing.T) {
	svc := &ConsolidationService{}
	err := svc.batchInsertLessons(context.Background(), nil)
	assert.NoError(t, err, "batch insert of empty lessons should not error")
}
