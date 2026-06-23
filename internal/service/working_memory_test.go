package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSlotService_Expiry(t *testing.T) {
	svc := NewSlotService(nil)
	svc.DefaultTTL = 50 * time.Millisecond

	ctx := context.Background()
	_, err := svc.CreateSlot(ctx, "test-expire", "content", "desc", "global", "", false, 2000)
	require.NoError(t, err)

	// Slot should exist now
	_, err = svc.GetSlot(ctx, "test-expire", "global", "")
	require.NoError(t, err)

	// Wait for expiry
	time.Sleep(100 * time.Millisecond)

	// Cleanup should remove it
	removed := svc.CleanupExpired(ctx)
	assert.Equal(t, 1, removed, "expired slot should be removed")

	// Slot should be gone
	_, err = svc.GetSlot(ctx, "test-expire", "global", "")
	assert.Error(t, err, "expired slot should not be found")
}

func TestSlotService_PinnedNeverExpires(t *testing.T) {
	svc := NewSlotService(nil)
	svc.DefaultTTL = 50 * time.Millisecond

	ctx := context.Background()
	_, err := svc.CreateSlot(ctx, "pinned-slot", "content", "desc", "global", "", true, 2000)
	require.NoError(t, err)

	// Wait past the default TTL
	time.Sleep(100 * time.Millisecond)

	// Cleanup should NOT remove pinned slot
	removed := svc.CleanupExpired(ctx)
	assert.Equal(t, 0, removed, "pinned slot should not be removed")

	// Slot should still exist
	_, err = svc.GetSlot(ctx, "pinned-slot", "global", "")
	assert.NoError(t, err, "pinned slot should still exist")
}

func TestSlotService_CleanupLoop(t *testing.T) {
	svc := NewSlotService(nil)
	svc.DefaultTTL = 50 * time.Millisecond

	ctx := context.Background()
	_, err := svc.CreateSlot(ctx, "loop-expire", "content", "desc", "global", "", false, 2000)
	require.NoError(t, err)

	loopCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go svc.StartCleanupLoop(loopCtx, 60*time.Millisecond)

	// Wait for cleanup to run
	time.Sleep(150 * time.Millisecond)

	// Slot should be gone after loop cleans it
	_, err = svc.GetSlot(ctx, "loop-expire", "global", "")
	assert.Error(t, err, "slot should be cleaned up by background loop")
}

func TestSlotService_CleanupLoopCancel(t *testing.T) {
	svc := NewSlotService(nil)
	svc.DefaultTTL = 50 * time.Millisecond

	ctx := context.Background()
	_, err := svc.CreateSlot(ctx, "cancel-expire", "content", "desc", "global", "", false, 2000)
	require.NoError(t, err)

	loopCtx, cancel := context.WithCancel(context.Background())
	go svc.StartCleanupLoop(loopCtx, 60*time.Millisecond)

	// Cancel immediately
	cancel()
	time.Sleep(100 * time.Millisecond)

	// Slot should still exist (loop cancelled before cleanup)
	_, err = svc.GetSlot(ctx, "cancel-expire", "global", "")
	assert.NoError(t, err, "slot should still exist after loop cancelled")
}

func TestSlotService_MaxSlots(t *testing.T) {
	svc := NewSlotService(nil)
	svc.MaxSlots = 2
	svc.DefaultTTL = 1 * time.Hour

	ctx := context.Background()
	_, err := svc.CreateSlot(ctx, "a", "content", "", "global", "", false, 100)
	require.NoError(t, err)
	_, err = svc.CreateSlot(ctx, "b", "content", "", "global", "", false, 100)
	require.NoError(t, err)

	// Third slot should fail — max reached
	_, err = svc.CreateSlot(ctx, "c", "content", "", "global", "", false, 100)
	assert.Error(t, err, "should error when MaxSlots exceeded")
	assert.Contains(t, err.Error(), "max slots")
}

func TestSlotService_MaxSlotsEvictsExpired(t *testing.T) {
	svc := NewSlotService(nil)
	svc.MaxSlots = 2
	svc.DefaultTTL = 50 * time.Millisecond

	ctx := context.Background()
	_, err := svc.CreateSlot(ctx, "exp1", "content", "", "global", "", false, 100)
	require.NoError(t, err)
	_, err = svc.CreateSlot(ctx, "exp2", "content", "", "global", "", false, 100)
	require.NoError(t, err)

	// Wait for them to expire
	time.Sleep(100 * time.Millisecond)

	// New slot should succeed because expired ones are evicted first
	_, err = svc.CreateSlot(ctx, "fresh", "content", "", "global", "", false, 100)
	assert.NoError(t, err, "should succeed after evicting expired slots")
}

func TestSlotService_DefaultTTL(t *testing.T) {
	svc := NewSlotService(nil)
	assert.Equal(t, 7*24*time.Hour, svc.DefaultTTL, "DefaultTTL should be 7 days")
}

func TestSlotService_MaxSlotsDefault(t *testing.T) {
	svc := NewSlotService(nil)
	assert.Equal(t, 0, svc.MaxSlots, "MaxSlots should default to 0 (unlimited)")
}

func TestSlotService_ExpiresAtSetOnCreate(t *testing.T) {
	svc := NewSlotService(nil)
	svc.DefaultTTL = 1 * time.Hour

	ctx := context.Background()
	slot, err := svc.CreateSlot(ctx, "has-expiry", "content", "", "global", "", false, 100)
	require.NoError(t, err)
	assert.NotNil(t, slot.ExpiresAt, "non-pinned slot should have ExpiresAt set")
}

func TestSlotService_ExpiresAtNotSetOnPinned(t *testing.T) {
	svc := NewSlotService(nil)
	svc.DefaultTTL = 1 * time.Hour

	ctx := context.Background()
	slot, err := svc.CreateSlot(ctx, "no-expiry", "content", "", "global", "", true, 100)
	require.NoError(t, err)
	assert.Nil(t, slot.ExpiresAt, "pinned slot should have nil ExpiresAt")
}

func TestSlotService_SlotKeyCollisionAcrossScopes(t *testing.T) {
	svc := NewSlotService(nil)
	ctx := context.Background()

	// Create a global slot "x".
	_, err := svc.CreateSlot(ctx, "x", "global-content", "", "global", "", false, 100)
	require.NoError(t, err)

	// Create a project-scoped slot "x" for "proj-a" — should NOT collide with global.
	_, err = svc.CreateSlot(ctx, "x", "project-content", "", "project", "proj-a", false, 100)
	require.NoError(t, err, "project-scoped slot should not collide with global slot of same label")

	// Both should exist independently.
	globalContent, err := svc.GetSlot(ctx, "x", "global", "")
	require.NoError(t, err)
	assert.Equal(t, "global-content", globalContent, "global slot content should be preserved")

	projContent, err := svc.GetSlot(ctx, "x", "project", "proj-a")
	require.NoError(t, err)
	assert.Equal(t, "project-content", projContent, "project-scoped slot content should be preserved")

	// A second project-scoped slot "x" for "proj-b" should also work independently.
	_, err = svc.CreateSlot(ctx, "x", "other-content", "", "project", "proj-b", false, 100)
	require.NoError(t, err)

	otherContent, err := svc.GetSlot(ctx, "x", "project", "proj-b")
	require.NoError(t, err)
	assert.Equal(t, "other-content", otherContent, "different project's slot content should be independent")
}
