package workers

import "testing"

// TestCompareWorkerDraftACPMappingParityDetectsAnUndeclaredPair proves the
// parity check actually fails closed when a legal (Kind, Phase) pair loses
// its declared outcome, rather than only ever observing the already-clean
// state. This is a white-box test because it must reach the package-private
// declaration table to remove one entry and restore it afterward.
func TestCompareWorkerDraftACPMappingParityDetectsAnUndeclaredPair(t *testing.T) {
	original, ok := kindACPMappingOutcome[KindTool]
	if !ok {
		t.Fatalf("kindACPMappingOutcome missing baseline entry for KindTool")
	}
	delete(kindACPMappingOutcome, KindTool)
	t.Cleanup(func() {
		kindACPMappingOutcome[KindTool] = original
	})

	drift := CompareWorkerDraftACPMappingParity()
	if len(drift.UndeclaredPairs) == 0 {
		t.Fatalf("CompareWorkerDraftACPMappingParity() UndeclaredPairs is empty, want drift for every legal TOOL phase")
	}

	toolPhases, ok := AllowedPhasesForKind(KindTool)
	if !ok || len(toolPhases) == 0 {
		t.Fatalf("AllowedPhasesForKind(KindTool) returned no phases")
	}
	gotToolPairs := 0
	for _, pair := range drift.UndeclaredPairs {
		if pair.Kind != KindTool {
			t.Fatalf("UndeclaredPairs contains non-TOOL pair %+v after only removing the TOOL entry", pair)
		}
		gotToolPairs++
	}
	if gotToolPairs != len(toolPhases) {
		t.Fatalf("UndeclaredPairs has %d TOOL pairs, want %d (one per legal TOOL phase)", gotToolPairs, len(toolPhases))
	}

	err := ValidateWorkerDraftACPMappingParity()
	if err == nil {
		t.Fatalf("ValidateWorkerDraftACPMappingParity() error = nil, want drift error")
	}
	if err.Error() == "" {
		t.Fatalf("drift error message is empty")
	}
}
