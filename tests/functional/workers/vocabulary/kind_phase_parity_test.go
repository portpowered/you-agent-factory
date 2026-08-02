package vocabulary

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// TestKnownKindsAndAllowedPhasesValidate proves every Kind the public
// vocabulary declares, and every Phase declared legal for that Kind, is
// itself accepted by Kind.Validate/Phase.Validate — the live vocabulary and
// its own validation cannot silently diverge.
func TestKnownKindsAndAllowedPhasesValidate(t *testing.T) {
	kinds := workers.KnownKinds()
	if len(kinds) == 0 {
		t.Fatal("KnownKinds() returned no kinds")
	}

	for _, kind := range kinds {
		if err := kind.Validate(); err != nil {
			t.Fatalf("KnownKinds() returned kind %q that fails Validate(): %v", kind, err)
		}

		phases, ok := workers.AllowedPhasesForKind(kind)
		if !ok {
			t.Fatalf("AllowedPhasesForKind(%q) reported kind unknown", kind)
		}
		if len(phases) == 0 {
			t.Fatalf("AllowedPhasesForKind(%q) returned no allowed phases", kind)
		}
		for _, phase := range phases {
			if err := phase.Validate(); err != nil {
				t.Fatalf("AllowedPhasesForKind(%q) returned phase %q that fails Validate(): %v", kind, phase, err)
			}
		}
	}
}

// TestWorkerDraftACPMappingParityHasNoDrift proves the declared ACP mapping
// outcome table stays exhaustive against the live Kind/Phase vocabulary: a
// future Kind or Phase added to the vocabulary without a declared mapping
// outcome fails this functional check, not just the unit suite.
func TestWorkerDraftACPMappingParityHasNoDrift(t *testing.T) {
	if err := workers.ValidateWorkerDraftACPMappingParity(); err != nil {
		t.Fatalf("ValidateWorkerDraftACPMappingParity(): %v", err)
	}

	drift := workers.CompareWorkerDraftACPMappingParity()
	if len(drift.UndeclaredPairs) != 0 {
		t.Fatalf("CompareWorkerDraftACPMappingParity() reported drift: %#v", drift.UndeclaredPairs)
	}

	declared := workers.DeclaredWorkerDraftACPMappingOutcomes()
	if len(declared) == 0 {
		t.Fatal("DeclaredWorkerDraftACPMappingOutcomes() returned no declared outcomes")
	}
	for _, outcome := range declared {
		if outcome.Kind == "" || outcome.Phase == "" || outcome.Outcome == "" || outcome.Evidence == "" {
			t.Fatalf("declared ACP mapping outcome missing a required field: %#v", outcome)
		}
	}
}
