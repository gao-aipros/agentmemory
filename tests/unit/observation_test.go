package unit

import (
	"testing"

	"github.com/agentmemory/agentmemory/internal/service"
	"github.com/stretchr/testify/assert"
)

func TestValidateHookType_AllThirteenTypesAccepted(t *testing.T) {
	validTypes := service.ValidHookTypes
	assert.Len(t, validTypes, 13, "expected exactly 13 valid hook types")

	for _, hookType := range validTypes {
		t.Run(hookType, func(t *testing.T) {
			assert.True(t, service.ValidateHookType(hookType),
				"expected %q to be a valid hook type", hookType)
		})
	}
}

func TestValidateHookType_InvalidTypeRejected(t *testing.T) {
	invalidTypes := []string{
		"",
		"invalid_type",
		"SESSION_START", // case-sensitive
		"random_event",
		"tool_use",             // missing pre_ or post_ prefix
		"pre_tool_use_success", // not in the 13
	}

	for _, hookType := range invalidTypes {
		t.Run("invalid_"+hookType, func(t *testing.T) {
			assert.False(t, service.ValidateHookType(hookType),
				"expected %q to be rejected as invalid hook type", hookType)
		})
	}
}

func TestValidateImportance_InRange(t *testing.T) {
	validValues := []float64{0.0, 0.1, 0.5, 0.99, 1.0}
	for _, v := range validValues {
		assert.True(t, service.ValidateImportance(v),
			"expected importance %f to be valid (in range [0.0, 1.0])", v)
	}
}

func TestValidateImportance_OutsideRangeRejected(t *testing.T) {
	invalidValues := []float64{-0.1, -1.0, -100.0, 1.1, 2.0, 100.0}
	for _, v := range invalidValues {
		assert.False(t, service.ValidateImportance(v),
			"expected importance %f to be rejected (outside range [0.0, 1.0])", v)
	}
}
