package workers

import (
	"errors"
	"testing"
)

func TestKindValidate_AcceptsExactlyTheDeclaredValues(t *testing.T) {
	t.Parallel()

	for _, kind := range []Kind{
		KindSession, KindRun, KindTurn, KindMessage, KindReasoning, KindTool,
		KindFileChange, KindPlan, KindProgress, KindUsage, KindError, KindStreamGap,
	} {
		t.Run(string(kind), func(t *testing.T) {
			t.Parallel()
			if err := kind.Validate(); err != nil {
				t.Fatalf("Kind(%q).Validate() error = %v, want nil", kind, err)
			}
		})
	}
}

func TestKindValidate_RejectsUndeclaredValues(t *testing.T) {
	t.Parallel()

	cases := []Kind{
		"",            // zero value
		"UNKNOWN",     // unknown
		"session",     // lowercase
		"Session",     // case variant
		" SESSION",    // leading whitespace
		"SESSION ",    // trailing whitespace
		"SESSIONX",    // near miss (extra suffix)
		"MESSAG",      // near miss (truncated)
		"STREAM_GAP ", // near miss (whitespace variant of a real value)
		"STREAM GAP",  // near miss (space instead of underscore)
	}
	for _, kind := range cases {
		t.Run(string(kind), func(t *testing.T) {
			t.Parallel()
			err := kind.Validate()
			var invalidKind *InvalidKindError
			if !errors.As(err, &invalidKind) {
				t.Fatalf("Kind(%q).Validate() error = %T(%v), want *InvalidKindError", kind, err, err)
			}
			if invalidKind.Value != kind {
				t.Fatalf("InvalidKindError.Value = %q, want %q", invalidKind.Value, kind)
			}
		})
	}
}

func TestPhaseValidate_AcceptsExactlyTheDeclaredValues(t *testing.T) {
	t.Parallel()

	for _, phase := range []Phase{
		PhaseStarted, PhaseDelta, PhaseUpdated, PhaseCompleted, PhaseFailed, PhaseCanceled,
	} {
		t.Run(string(phase), func(t *testing.T) {
			t.Parallel()
			if err := phase.Validate(); err != nil {
				t.Fatalf("Phase(%q).Validate() error = %v, want nil", phase, err)
			}
		})
	}
}

func TestPhaseValidate_RejectsUndeclaredValues(t *testing.T) {
	t.Parallel()

	cases := []Phase{
		"",          // zero value
		"UNKNOWN",   // unknown
		"started",   // lowercase
		"Started",   // case variant
		" STARTED",  // leading whitespace
		"STARTED ",  // trailing whitespace
		"STARTEDX",  // near miss (extra suffix)
		"COMPLETE",  // near miss (truncated form of COMPLETED)
		"CANCELLED", // near miss (double-L variant of CANCELED)
	}
	for _, phase := range cases {
		t.Run(string(phase), func(t *testing.T) {
			t.Parallel()
			err := phase.Validate()
			var invalidPhase *InvalidPhaseError
			if !errors.As(err, &invalidPhase) {
				t.Fatalf("Phase(%q).Validate() error = %T(%v), want *InvalidPhaseError", phase, err, err)
			}
			if invalidPhase.Value != phase {
				t.Fatalf("InvalidPhaseError.Value = %q, want %q", invalidPhase.Value, phase)
			}
		})
	}
}

func TestKindPhaseValidate_ErrorsAreDistinctTypes(t *testing.T) {
	t.Parallel()

	kindErr := Kind("NOT_A_KIND").Validate()
	phaseErr := Phase("NOT_A_PHASE").Validate()

	var invalidKind *InvalidKindError
	if errors.As(phaseErr, &invalidKind) {
		t.Fatalf("Phase validation error unexpectedly matched *InvalidKindError")
	}
	var invalidPhase *InvalidPhaseError
	if errors.As(kindErr, &invalidPhase) {
		t.Fatalf("Kind validation error unexpectedly matched *InvalidPhaseError")
	}
}
