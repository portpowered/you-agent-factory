package factory_test

import (
	"testing"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

func TestProjectCurrentlyInFlightDispatchCount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		reportedCount int
		activeIDs     map[string]struct{}
		completedIDs  []string
		wantCount     int
	}{
		{
			name:          "active dispatch",
			reportedCount: 1,
			activeIDs:     map[string]struct{}{"dispatch-active": {}},
			wantCount:     1,
		},
		{
			name:          "backing off after terminal response",
			reportedCount: 1,
			activeIDs:     map[string]struct{}{"dispatch-throttled": {}},
			completedIDs:  []string{"dispatch-throttled"},
			wantCount:     0,
		},
		{
			name:          "quiescent",
			reportedCount: 0,
			wantCount:     0,
		},
		{
			name:          "retained-count-without-active-dispatch",
			reportedCount: 1,
			completedIDs:  []string{"dispatch-throttled"},
			wantCount:     0,
		},
		{
			name:          "unmatched response stays conservative",
			reportedCount: 1,
			activeIDs:     map[string]struct{}{"dispatch-active": {}},
			completedIDs:  []string{"dispatch-unknown"},
			wantCount:     1,
		},
		{
			name:          "result and history do not double count one dispatch",
			reportedCount: 2,
			activeIDs: map[string]struct{}{
				"dispatch-throttled": {},
				"dispatch-active":    {},
			},
			completedIDs: []string{"dispatch-throttled", "dispatch-throttled"},
			wantCount:    1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := factoryruntime.ProjectCurrentlyInFlightDispatchCount(test.reportedCount, test.activeIDs, test.completedIDs); got != test.wantCount {
				t.Fatalf("ProjectCurrentlyInFlightDispatchCount() = %d, want %d", got, test.wantCount)
			}
		})
	}
}
