package workers_test

import (
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestKindValidateAcceptsExactlyTheDeclaredKinds(t *testing.T) {
	t.Parallel()

	valid := []workers.Kind{
		workers.KindSession,
		workers.KindRun,
		workers.KindTurn,
		workers.KindMessage,
		workers.KindReasoning,
		workers.KindTool,
		workers.KindFileChange,
		workers.KindPlan,
		workers.KindProgress,
		workers.KindUsage,
		workers.KindError,
		workers.KindStreamGap,
	}
	if len(valid) != 12 {
		t.Fatalf("declared Kind list has %d entries, want 12", len(valid))
	}
	for _, kind := range valid {
		t.Run(string(kind), func(t *testing.T) {
			t.Parallel()
			if err := kind.Validate(); err != nil {
				t.Fatalf("Validate() error = %v, want nil for declared kind %q", err, kind)
			}
		})
	}
}

func TestKindValidateRejectsZeroAndUnknownValues(t *testing.T) {
	t.Parallel()

	cases := []workers.Kind{
		"",
		"session",
		"UNKNOWN_KIND",
		"MESSAGES",
	}
	for _, kind := range cases {
		t.Run("kind_"+string(kind), func(t *testing.T) {
			t.Parallel()
			err := kind.Validate()
			if err == nil {
				t.Fatalf("Validate() error = nil, want error for kind %q", kind)
			}
			if !errors.Is(err, workers.ErrUnknownDraftKind) {
				t.Fatalf("Validate() error = %v, want errors.Is ErrUnknownDraftKind", err)
			}
			var invalid *workers.InvalidDraftKindError
			if !errors.As(err, &invalid) {
				t.Fatalf("Validate() error = %v, want *InvalidDraftKindError", err)
			}
			if invalid.Kind != kind {
				t.Fatalf("InvalidDraftKindError.Kind = %q, want %q", invalid.Kind, kind)
			}
		})
	}
}

func TestPhaseValidateAcceptsExactlyTheDeclaredPhases(t *testing.T) {
	t.Parallel()

	valid := []workers.Phase{
		workers.PhaseStarted,
		workers.PhaseDelta,
		workers.PhaseUpdated,
		workers.PhaseCompleted,
		workers.PhaseFailed,
		workers.PhaseCanceled,
	}
	if len(valid) != 6 {
		t.Fatalf("declared Phase list has %d entries, want 6", len(valid))
	}
	for _, phase := range valid {
		t.Run(string(phase), func(t *testing.T) {
			t.Parallel()
			if err := phase.Validate(); err != nil {
				t.Fatalf("Validate() error = %v, want nil for declared phase %q", err, phase)
			}
		})
	}
}

func TestPhaseValidateRejectsZeroAndUnknownValues(t *testing.T) {
	t.Parallel()

	cases := []workers.Phase{
		"",
		"started",
		"UNKNOWN_PHASE",
		"COMPLETE",
	}
	for _, phase := range cases {
		t.Run("phase_"+string(phase), func(t *testing.T) {
			t.Parallel()
			err := phase.Validate()
			if err == nil {
				t.Fatalf("Validate() error = nil, want error for phase %q", phase)
			}
			if !errors.Is(err, workers.ErrUnknownDraftPhase) {
				t.Fatalf("Validate() error = %v, want errors.Is ErrUnknownDraftPhase", err)
			}
			var invalid *workers.InvalidDraftPhaseError
			if !errors.As(err, &invalid) {
				t.Fatalf("Validate() error = %v, want *InvalidDraftPhaseError", err)
			}
			if invalid.Phase != phase {
				t.Fatalf("InvalidDraftPhaseError.Phase = %q, want %q", invalid.Phase, phase)
			}
		})
	}
}

func TestKnownKindsMatchesDeclaredKindSet(t *testing.T) {
	t.Parallel()

	kinds := workers.KnownKinds()
	if len(kinds) != 12 {
		t.Fatalf("KnownKinds() returned %d kinds, want 12", len(kinds))
	}
	for _, kind := range kinds {
		if err := kind.Validate(); err != nil {
			t.Fatalf("KnownKinds() returned %q which fails Validate(): %v", kind, err)
		}
	}
}

func TestAllowedPhasesForKindReportsUnknownKind(t *testing.T) {
	t.Parallel()

	if _, ok := workers.AllowedPhasesForKind("NOT_A_KIND"); ok {
		t.Fatalf("AllowedPhasesForKind(unknown) ok = true, want false")
	}

	for _, kind := range workers.KnownKinds() {
		phases, ok := workers.AllowedPhasesForKind(kind)
		if !ok {
			t.Fatalf("AllowedPhasesForKind(%q) ok = false, want true", kind)
		}
		if len(phases) == 0 {
			t.Fatalf("AllowedPhasesForKind(%q) returned no phases", kind)
		}
		for _, phase := range phases {
			if err := phase.Validate(); err != nil {
				t.Fatalf("AllowedPhasesForKind(%q) returned invalid phase %q: %v", kind, phase, err)
			}
		}
	}
}
