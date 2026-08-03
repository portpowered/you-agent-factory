package workersessions_test

import (
	"testing"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

func TestState_Valid_AcceptsExactlyTheEightLifecycleStates(t *testing.T) {
	accepted := []workersessions.State{
		workersessions.StateReserved,
		workersessions.StateStarting,
		workersessions.StateRunning,
		workersessions.StatePaused,
		workersessions.StateCompleted,
		workersessions.StateFailed,
		workersessions.StateCanceled,
		workersessions.StateTerminated,
	}
	if len(accepted) != 8 {
		t.Fatalf("accepted state vocabulary has %d entries, want exactly 8", len(accepted))
	}
	for _, state := range accepted {
		if !state.Valid() {
			t.Errorf("State(%q).Valid() = false, want true", state)
		}
	}
}

func TestState_Valid_RejectsUnknownEmptyAndInterrupted(t *testing.T) {
	rejected := []workersessions.State{"", "INTERRUPTED", "running", "unknown", "RESERVED "}
	for _, state := range rejected {
		if state.Valid() {
			t.Errorf("State(%q).Valid() = true, want false", state)
		}
	}
}

func TestState_Terminal_IdentifiesExactlyTheFourAbsorbingStates(t *testing.T) {
	terminal := map[workersessions.State]bool{
		workersessions.StateReserved:   false,
		workersessions.StateStarting:   false,
		workersessions.StateRunning:    false,
		workersessions.StatePaused:     false,
		workersessions.StateCompleted:  true,
		workersessions.StateFailed:     true,
		workersessions.StateCanceled:   true,
		workersessions.StateTerminated: true,
	}
	for state, want := range terminal {
		if got := state.Terminal(); got != want {
			t.Errorf("State(%q).Terminal() = %v, want %v", state, got, want)
		}
	}
}

func TestState_Terminal_RejectsUnknownAndInterruptedAsNonTerminal(t *testing.T) {
	for _, state := range []workersessions.State{"", "INTERRUPTED", "unknown"} {
		if state.Terminal() {
			t.Errorf("State(%q).Terminal() = true, want false for an unrecognized state", state)
		}
	}
}
