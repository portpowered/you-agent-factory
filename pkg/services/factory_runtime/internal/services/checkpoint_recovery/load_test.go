package checkpoint_recovery_test

import (
	"testing"

	checkpointrecovery "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/checkpoint_recovery"
)

func TestCompatibilityForSchema(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		stored    int
		expected  int
		compatible bool
	}{
		{name: "zero expected skips compatibility", stored: 1, expected: 0, compatible: false},
		{name: "matching schema", stored: 1, expected: 1, compatible: true},
		{name: "mismatched schema", stored: 1, expected: 2, compatible: false},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := checkpointrecovery.CompatibilityForSchema(tc.stored, tc.expected)
			if got != tc.compatible {
				t.Fatalf("CompatibilityForSchema(%d, %d) = %v, want %v", tc.stored, tc.expected, got, tc.compatible)
			}
		})
	}
}
