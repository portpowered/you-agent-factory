package subsystems_test

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/factory/subsystems"
)

func TestActiveRuntimeTickGroupsAreOrdered(t *testing.T) {
	activeRuntimeGroups := []subsystems.TickGroup{
		subsystems.CircuitBreaker,
		subsystems.Dispatcher,
		subsystems.History,
		subsystems.Transitioner,
		subsystems.CascadingFailure,
		subsystems.TerminationCheck,
	}

	for i := 1; i < len(activeRuntimeGroups); i++ {
		if activeRuntimeGroups[i] <= activeRuntimeGroups[i-1] {
			t.Fatalf("tick group %d = %d, want greater than previous %d", i, activeRuntimeGroups[i], activeRuntimeGroups[i-1])
		}
	}
}
