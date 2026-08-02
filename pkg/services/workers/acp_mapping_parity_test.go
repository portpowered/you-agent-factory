package workers_test

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestValidateWorkerDraftACPMappingParityIsClean(t *testing.T) {
	t.Parallel()

	if err := workers.ValidateWorkerDraftACPMappingParity(); err != nil {
		t.Fatalf("ValidateWorkerDraftACPMappingParity() error = %v, want nil", err)
	}
	drift := workers.CompareWorkerDraftACPMappingParity()
	if len(drift.UndeclaredPairs) != 0 {
		t.Fatalf("CompareWorkerDraftACPMappingParity() UndeclaredPairs = %v, want empty", drift.UndeclaredPairs)
	}
}

// TestDeclaredWorkerDraftACPMappingOutcomesCoversEveryLegalPair proves the
// declared outcome table is exhaustive over every (Kind, Phase) pair the
// existing draft validator treats as legal, including an explicit outcome
// for every phase of every kind, not just one representative phase per kind.
func TestDeclaredWorkerDraftACPMappingOutcomesCoversEveryLegalPair(t *testing.T) {
	t.Parallel()

	declared := workers.DeclaredWorkerDraftACPMappingOutcomes()
	declaredSet := make(map[workers.Kind]map[workers.Phase]workers.ACPMappingOutcome)
	for _, entry := range declared {
		if entry.Outcome == "" {
			t.Fatalf("declared entry for %s/%s has empty Outcome", entry.Kind, entry.Phase)
		}
		if entry.Evidence == "" {
			t.Fatalf("declared entry for %s/%s has empty Evidence", entry.Kind, entry.Phase)
		}
		if declaredSet[entry.Kind] == nil {
			declaredSet[entry.Kind] = make(map[workers.Phase]workers.ACPMappingOutcome)
		}
		declaredSet[entry.Kind][entry.Phase] = entry.Outcome
	}

	wantLegalPairCount := 0
	for _, kind := range workers.KnownKinds() {
		phases, ok := workers.AllowedPhasesForKind(kind)
		if !ok {
			t.Fatalf("AllowedPhasesForKind(%q) ok = false for a KnownKinds() entry", kind)
		}
		wantLegalPairCount += len(phases)
		for _, phase := range phases {
			if _, ok := declaredSet[kind][phase]; !ok {
				t.Fatalf("no declared ACP mapping outcome for legal pair %s/%s", kind, phase)
			}
		}
	}
	if len(declared) != wantLegalPairCount {
		t.Fatalf("DeclaredWorkerDraftACPMappingOutcomes() returned %d entries, want %d (exactly the legal pairs, no extras)", len(declared), wantLegalPairCount)
	}
}

// TestDeclaredWorkerDraftACPMappingOutcomesDeclaresExplicitNoOutput proves the
// "declared, not dropped" no-output kinds from proposal section 6.2 resolve
// to the explicit NoOutput outcome rather than being silently absent.
func TestDeclaredWorkerDraftACPMappingOutcomesDeclaresExplicitNoOutput(t *testing.T) {
	t.Parallel()

	noOutputKinds := []workers.Kind{
		workers.KindTool,
		workers.KindFileChange,
		workers.KindPlan,
		workers.KindProgress,
		workers.KindSession,
		workers.KindRun,
		workers.KindTurn,
	}
	byKind := make(map[workers.Kind][]workers.WorkerDraftACPMappingOutcome)
	for _, entry := range workers.DeclaredWorkerDraftACPMappingOutcomes() {
		byKind[entry.Kind] = append(byKind[entry.Kind], entry)
	}

	for _, kind := range noOutputKinds {
		entries, ok := byKind[kind]
		if !ok || len(entries) == 0 {
			t.Fatalf("no declared entries for %q", kind)
		}
		for _, entry := range entries {
			if entry.Outcome != workers.ACPMappingOutcomeNoOutput {
				t.Fatalf("%s/%s outcome = %q, want %q", entry.Kind, entry.Phase, entry.Outcome, workers.ACPMappingOutcomeNoOutput)
			}
		}
	}
}

// TestDeclaredWorkerDraftACPMappingOutcomesStreamingContentKinds proves the
// content-bearing kinds resolve to their declared non-empty ACP projection
// outcome rather than NoOutput or OutOfBandError.
func TestDeclaredWorkerDraftACPMappingOutcomesStreamingContentKinds(t *testing.T) {
	t.Parallel()

	want := map[workers.Kind]workers.ACPMappingOutcome{
		workers.KindMessage:   workers.ACPMappingOutcomeAgentMessageChunk,
		workers.KindReasoning: workers.ACPMappingOutcomeAgentThoughtChunk,
		workers.KindUsage:     workers.ACPMappingOutcomeUsageUpdate,
		workers.KindStreamGap: workers.ACPMappingOutcomeGapNotice,
		workers.KindError:     workers.ACPMappingOutcomeOutOfBandError,
	}
	byKind := make(map[workers.Kind][]workers.WorkerDraftACPMappingOutcome)
	for _, entry := range workers.DeclaredWorkerDraftACPMappingOutcomes() {
		byKind[entry.Kind] = append(byKind[entry.Kind], entry)
	}

	for kind, wantOutcome := range want {
		entries, ok := byKind[kind]
		if !ok || len(entries) == 0 {
			t.Fatalf("no declared entries for %q", kind)
		}
		for _, entry := range entries {
			if entry.Outcome != wantOutcome {
				t.Fatalf("%s/%s outcome = %q, want %q", entry.Kind, entry.Phase, entry.Outcome, wantOutcome)
			}
		}
	}
}
