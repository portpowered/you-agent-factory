package draftvalidation

import "testing"

// This file proves, entirely from test code, that every legal Kind/Phase
// pair declared by allowedPhasesByKind (the one existing legal-pair policy
// owner) has exactly one declared ACP L1 outcome, and that no illegal or
// unknown pair has one. It adds no production surface: acpOutcome,
// declaredACPL1Outcomes, and compareACPOutcomeParity all live in this _test.go
// file and are unreachable from non-test code.

// acpOutcome names the ACP L1 treatment applied to a legal Kind/Phase pair.
type acpOutcome string

const (
	acpOutcomeMessageChunk   acpOutcome = "MESSAGE_CHUNK"
	acpOutcomeThoughtChunk   acpOutcome = "THOUGHT_CHUNK"
	acpOutcomeUsageUpdate    acpOutcome = "USAGE_UPDATE"
	acpOutcomeGapNotice      acpOutcome = "GAP_NOTICE"
	acpOutcomeErrorOutOfBand acpOutcome = "ERROR_OUT_OF_BAND"
	acpOutcomeNoOutput       acpOutcome = "NO_OUTPUT"
)

// kindPhase is a comparable (Kind, Phase) pair used as a set element.
type kindPhase struct {
	Kind  Kind
	Phase Phase
}

// acpOutcomeDeclaration is one test-owned outcome assignment. Declarations
// are held as a slice, not a map, so a mutated fixture can represent a
// duplicate declaration for the same pair.
type acpOutcomeDeclaration struct {
	Kind    Kind
	Phase   Phase
	Outcome acpOutcome
}

// declaredACPL1Outcomes assigns exactly one ACP L1 outcome to every legal
// Kind/Phase pair, matching the pairs declared by allowedPhasesByKind.
var declaredACPL1Outcomes = []acpOutcomeDeclaration{
	{KindSession, PhaseStarted, acpOutcomeNoOutput},
	{KindSession, PhaseCompleted, acpOutcomeNoOutput},
	{KindSession, PhaseFailed, acpOutcomeNoOutput},
	{KindSession, PhaseCanceled, acpOutcomeNoOutput},

	{KindRun, PhaseStarted, acpOutcomeNoOutput},
	{KindRun, PhaseCompleted, acpOutcomeNoOutput},
	{KindRun, PhaseFailed, acpOutcomeNoOutput},
	{KindRun, PhaseCanceled, acpOutcomeNoOutput},

	{KindTurn, PhaseStarted, acpOutcomeNoOutput},
	{KindTurn, PhaseCompleted, acpOutcomeNoOutput},
	{KindTurn, PhaseFailed, acpOutcomeNoOutput},
	{KindTurn, PhaseCanceled, acpOutcomeNoOutput},

	{KindMessage, PhaseStarted, acpOutcomeMessageChunk},
	{KindMessage, PhaseDelta, acpOutcomeMessageChunk},
	{KindMessage, PhaseCompleted, acpOutcomeMessageChunk},

	{KindReasoning, PhaseStarted, acpOutcomeThoughtChunk},
	{KindReasoning, PhaseDelta, acpOutcomeThoughtChunk},
	{KindReasoning, PhaseCompleted, acpOutcomeThoughtChunk},

	{KindTool, PhaseStarted, acpOutcomeNoOutput},
	{KindTool, PhaseDelta, acpOutcomeNoOutput},
	{KindTool, PhaseCompleted, acpOutcomeNoOutput},
	{KindTool, PhaseFailed, acpOutcomeNoOutput},
	{KindTool, PhaseCanceled, acpOutcomeNoOutput},

	{KindFileChange, PhaseUpdated, acpOutcomeNoOutput},
	{KindPlan, PhaseUpdated, acpOutcomeNoOutput},
	{KindProgress, PhaseUpdated, acpOutcomeNoOutput},

	{KindUsage, PhaseUpdated, acpOutcomeUsageUpdate},

	{KindError, PhaseUpdated, acpOutcomeErrorOutOfBand},
	{KindError, PhaseFailed, acpOutcomeErrorOutOfBand},

	{KindStreamGap, PhaseUpdated, acpOutcomeGapNotice},
}

// acpParityReport is the drift report produced by compareACPOutcomeParity.
type acpParityReport struct {
	// Missing lists legal pairs that have no declared outcome.
	Missing []kindPhase
	// Duplicate lists pairs declared more than once.
	Duplicate []kindPhase
	// Extra lists declared pairs that are not legal (illegal or unknown).
	Extra []kindPhase
}

func (r acpParityReport) isClean() bool {
	return len(r.Missing) == 0 && len(r.Duplicate) == 0 && len(r.Extra) == 0
}

// legalKindPhasePairs reads the one existing legal-pair policy owner and
// returns its pairs as a set, without mutating it.
func legalKindPhasePairs() map[kindPhase]struct{} {
	pairs := make(map[kindPhase]struct{})
	for kind, phases := range allowedPhasesByKind {
		for _, phase := range phases {
			pairs[kindPhase{Kind: kind, Phase: phase}] = struct{}{}
		}
	}
	return pairs
}

// compareACPOutcomeParity reports every legal pair missing a declaration,
// every pair declared more than once, and every declared pair that is not
// legal. It performs no I/O and mutates neither input.
func compareACPOutcomeParity(declarations []acpOutcomeDeclaration, legal map[kindPhase]struct{}) acpParityReport {
	seen := make(map[kindPhase]int, len(declarations))
	var report acpParityReport

	for _, decl := range declarations {
		pair := kindPhase{Kind: decl.Kind, Phase: decl.Phase}
		seen[pair]++
		if seen[pair] == 2 {
			report.Duplicate = append(report.Duplicate, pair)
		}
		if _, ok := legal[pair]; !ok {
			report.Extra = append(report.Extra, pair)
		}
	}
	for pair := range legal {
		if seen[pair] == 0 {
			report.Missing = append(report.Missing, pair)
		}
	}

	sortKindPhasePairs(report.Missing)
	sortKindPhasePairs(report.Duplicate)
	sortKindPhasePairs(report.Extra)
	return report
}

func sortKindPhasePairs(pairs []kindPhase) {
	for i := 1; i < len(pairs); i++ {
		for j := i; j > 0 && kindPhaseLess(pairs[j], pairs[j-1]); j-- {
			pairs[j], pairs[j-1] = pairs[j-1], pairs[j]
		}
	}
}

func kindPhaseLess(a, b kindPhase) bool {
	if a.Kind != b.Kind {
		return a.Kind < b.Kind
	}
	return a.Phase < b.Phase
}

// TestACPL1OutcomeParity_MatchesLegalPairsExactly proves the production
// legal-pair set and the test-owned outcome table are in exact 1:1
// correspondence: nothing missing, nothing duplicated, nothing extra.
func TestACPL1OutcomeParity_MatchesLegalPairsExactly(t *testing.T) {
	t.Parallel()

	report := compareACPOutcomeParity(declaredACPL1Outcomes, legalKindPhasePairs())
	if !report.isClean() {
		t.Fatalf("ACP L1 outcome parity drift detected: missing=%v duplicate=%v extra=%v", report.Missing, report.Duplicate, report.Extra)
	}
}

// TestACPL1OutcomeParity_DeclaresExpectedOutcomeFamilies proves the outcome
// assigned to each Kind matches the required ACP L1 treatment across every
// phase that Kind allows.
func TestACPL1OutcomeParity_DeclaresExpectedOutcomeFamilies(t *testing.T) {
	t.Parallel()

	wantOutcomeByKind := map[Kind]acpOutcome{
		KindSession:    acpOutcomeNoOutput,
		KindRun:        acpOutcomeNoOutput,
		KindTurn:       acpOutcomeNoOutput,
		KindMessage:    acpOutcomeMessageChunk,
		KindReasoning:  acpOutcomeThoughtChunk,
		KindTool:       acpOutcomeNoOutput,
		KindFileChange: acpOutcomeNoOutput,
		KindPlan:       acpOutcomeNoOutput,
		KindProgress:   acpOutcomeNoOutput,
		KindUsage:      acpOutcomeUsageUpdate,
		KindError:      acpOutcomeErrorOutOfBand,
		KindStreamGap:  acpOutcomeGapNotice,
	}

	if len(declaredACPL1Outcomes) == 0 {
		t.Fatalf("declaredACPL1Outcomes must not be empty")
	}
	for _, decl := range declaredACPL1Outcomes {
		want, ok := wantOutcomeByKind[decl.Kind]
		if !ok {
			t.Fatalf("declaration for unexpected kind %q", decl.Kind)
		}
		if decl.Outcome != want {
			t.Fatalf("kind %q phase %q outcome = %q, want %q", decl.Kind, decl.Phase, decl.Outcome, want)
		}
	}
}

// TestACPL1OutcomeParity_DetectsMissingLegalPair proves the comparator flags
// a legal pair that has no declaration, without mutating the production
// declaration table.
func TestACPL1OutcomeParity_DetectsMissingLegalPair(t *testing.T) {
	t.Parallel()

	missingVariant := make([]acpOutcomeDeclaration, 0, len(declaredACPL1Outcomes)-1)
	var removed kindPhase
	for i, decl := range declaredACPL1Outcomes {
		if i == 0 {
			removed = kindPhase{Kind: decl.Kind, Phase: decl.Phase}
			continue
		}
		missingVariant = append(missingVariant, decl)
	}

	report := compareACPOutcomeParity(missingVariant, legalKindPhasePairs())
	if len(report.Missing) != 1 || report.Missing[0] != removed {
		t.Fatalf("expected exactly one missing pair %v, got %v", removed, report.Missing)
	}
	if len(report.Duplicate) != 0 || len(report.Extra) != 0 {
		t.Fatalf("expected no duplicate/extra defects for a missing-only fixture, got duplicate=%v extra=%v", report.Duplicate, report.Extra)
	}

	// The production table itself remains unaffected by building the variant.
	if len(declaredACPL1Outcomes) == len(missingVariant) {
		t.Fatalf("test fixture did not actually remove an entry")
	}
}

// TestACPL1OutcomeParity_DetectsDuplicateDeclaration proves the comparator
// flags a pair declared more than once.
func TestACPL1OutcomeParity_DetectsDuplicateDeclaration(t *testing.T) {
	t.Parallel()

	duplicateVariant := make([]acpOutcomeDeclaration, len(declaredACPL1Outcomes), len(declaredACPL1Outcomes)+1)
	copy(duplicateVariant, declaredACPL1Outcomes)
	duplicated := duplicateVariant[0]
	duplicateVariant = append(duplicateVariant, duplicated)

	report := compareACPOutcomeParity(duplicateVariant, legalKindPhasePairs())
	wantPair := kindPhase{Kind: duplicated.Kind, Phase: duplicated.Phase}
	if len(report.Duplicate) != 1 || report.Duplicate[0] != wantPair {
		t.Fatalf("expected exactly one duplicate pair %v, got %v", wantPair, report.Duplicate)
	}
	if len(report.Missing) != 0 || len(report.Extra) != 0 {
		t.Fatalf("expected no missing/extra defects for a duplicate-only fixture, got missing=%v extra=%v", report.Missing, report.Extra)
	}
}

// TestACPL1OutcomeParity_DetectsExtraIllegalPair proves the comparator flags
// a declared pair that the legal-pair policy does not allow.
func TestACPL1OutcomeParity_DetectsExtraIllegalPair(t *testing.T) {
	t.Parallel()

	// SESSION never allows DELTA per allowedPhasesByKind.
	extraDecl := acpOutcomeDeclaration{Kind: KindSession, Phase: PhaseDelta, Outcome: acpOutcomeNoOutput}
	extraVariant := make([]acpOutcomeDeclaration, len(declaredACPL1Outcomes), len(declaredACPL1Outcomes)+1)
	copy(extraVariant, declaredACPL1Outcomes)
	extraVariant = append(extraVariant, extraDecl)

	report := compareACPOutcomeParity(extraVariant, legalKindPhasePairs())
	wantPair := kindPhase{Kind: extraDecl.Kind, Phase: extraDecl.Phase}
	if len(report.Extra) != 1 || report.Extra[0] != wantPair {
		t.Fatalf("expected exactly one extra pair %v, got %v", wantPair, report.Extra)
	}
	if len(report.Missing) != 0 || len(report.Duplicate) != 0 {
		t.Fatalf("expected no missing/duplicate defects for an extra-only fixture, got missing=%v duplicate=%v", report.Missing, report.Duplicate)
	}
}

// TestACPL1OutcomeParity_DetectsUnknownPairAndCombinedDefects proves the
// comparator flags a declared pair for an unknown Kind/Phase combination that
// is not even a member of the declared vocabulary, and that missing,
// duplicate, and extra defects are all reported together in one fixture.
func TestACPL1OutcomeParity_DetectsUnknownPairAndCombinedDefects(t *testing.T) {
	t.Parallel()

	combined := make([]acpOutcomeDeclaration, 0, len(declaredACPL1Outcomes)+1)
	var removed kindPhase
	for i, decl := range declaredACPL1Outcomes {
		if i == 0 {
			removed = kindPhase{Kind: decl.Kind, Phase: decl.Phase}
			continue
		}
		combined = append(combined, decl)
	}
	duplicated := combined[0]
	combined = append(combined, duplicated)
	unknownDecl := acpOutcomeDeclaration{Kind: "NOT_A_KIND", Phase: "NOT_A_PHASE", Outcome: acpOutcomeNoOutput}
	combined = append(combined, unknownDecl)

	report := compareACPOutcomeParity(combined, legalKindPhasePairs())

	wantDuplicate := kindPhase{Kind: duplicated.Kind, Phase: duplicated.Phase}
	wantExtra := kindPhase{Kind: unknownDecl.Kind, Phase: unknownDecl.Phase}

	if len(report.Missing) != 1 || report.Missing[0] != removed {
		t.Fatalf("expected exactly one missing pair %v, got %v", removed, report.Missing)
	}
	if len(report.Duplicate) != 1 || report.Duplicate[0] != wantDuplicate {
		t.Fatalf("expected exactly one duplicate pair %v, got %v", wantDuplicate, report.Duplicate)
	}
	if len(report.Extra) != 1 || report.Extra[0] != wantExtra {
		t.Fatalf("expected exactly one extra pair %v, got %v", wantExtra, report.Extra)
	}
}
