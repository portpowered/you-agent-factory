package workers

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	workstationdraftvalidation "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/draftvalidation"
)

// This file is contract-test-only evidence, deliberately not part of the
// production package: final-proposal.md section 6.1 places outbound ACP
// mapping ownership at pkg/transports/acp/internal/mapping, not the Workers
// service root, and this packet implements no ACP projection. Keeping the
// declared-outcome vocabulary and its parity check confined to a _test.go
// file means none of it is compiled into the production binary or reachable
// by other packages -- it exists only to prove, at test time, that the
// existing Kind/Phase vocabulary this story hardens has an exhaustively
// declared future ACP mapping outcome for every legal pair.
//
// legalPairs is sourced directly from the internal draftvalidation package's
// exported KnownKinds()/AllowedPhases() (not a workers-root wrapper): those
// two functions have no production caller anywhere in the repository, so
// publishing them as workers.KnownKinds()/workers.AllowedPhasesForKind()
// would be speculative production API that exists only to support this test.
// Importing workstationdraftvalidation directly here keeps that inventory
// test-only while still reusing the single existing allow-list rather than
// re-deriving it.

// knownKindsFromInternal returns every declared response draft Kind exactly
// as workstationdraftvalidation.KnownKinds() declares them, converted to the
// workers-root Kind type.
func knownKindsFromInternal() []Kind {
	internalKinds := workstationdraftvalidation.KnownKinds()
	out := make([]Kind, len(internalKinds))
	for i, kind := range internalKinds {
		out[i] = Kind(kind)
	}
	return out
}

// allowedPhasesForKind returns the declared legal phases for kind and whether
// kind is a declared response draft kind, converted from
// workstationdraftvalidation.AllowedPhases().
func allowedPhasesForKind(kind Kind) ([]Phase, bool) {
	internalPhases, ok := workstationdraftvalidation.AllowedPhases(workstationdraftvalidation.Kind(kind))
	if !ok {
		return nil, false
	}
	out := make([]Phase, len(internalPhases))
	for i, phase := range internalPhases {
		out[i] = Phase(phase)
	}
	return out, true
}

// acpMappingOutcome names the declared future ACP session-update projection
// outcome for one (Kind, Phase) response draft pair. This test declares the
// outcome only; it proves no ACP projection, transport, or Worker Session
// behavior.
type acpMappingOutcome string

const (
	// acpMappingOutcomeAgentMessageChunk is the declared outcome for MESSAGE
	// drafts: projects onto ACP agent_message_chunk.
	acpMappingOutcomeAgentMessageChunk acpMappingOutcome = "AGENT_MESSAGE_CHUNK"
	// acpMappingOutcomeAgentThoughtChunk is the declared outcome for
	// REASONING drafts: projects onto ACP agent_thought_chunk.
	acpMappingOutcomeAgentThoughtChunk acpMappingOutcome = "AGENT_THOUGHT_CHUNK"
	// acpMappingOutcomeUsageUpdate is the declared outcome for USAGE drafts
	// carrying a meaningful primary context: projects onto ACP usage_update.
	acpMappingOutcomeUsageUpdate acpMappingOutcome = "USAGE_UPDATE"
	// acpMappingOutcomeGapNotice is the declared outcome for STREAM_GAP
	// drafts: surfaced as an explicit ACP gap notice, content is never
	// fabricated to fill it.
	acpMappingOutcomeGapNotice acpMappingOutcome = "GAP_NOTICE"
	// acpMappingOutcomeOutOfBandError is the declared outcome for ERROR
	// drafts. Errors surface through the JSON-RPC error channel (proposal
	// section 6.4), not the ACP SessionUpdate stream, so this is declared as
	// an explicit non-stream outcome rather than silently reusing NoOutput.
	acpMappingOutcomeOutOfBandError acpMappingOutcome = "OUT_OF_BAND_ERROR"
	// acpMappingOutcomeNoOutput is the declared "no output in L1" outcome:
	// declared, not dropped, distinct from an undeclared/missing pair.
	acpMappingOutcomeNoOutput acpMappingOutcome = "NO_OUTPUT"
)

// workerDraftACPMappingOutcome names one legal (Kind, Phase) response draft
// pair together with its declared ACP mapping outcome and supporting
// evidence.
type workerDraftACPMappingOutcome struct {
	Kind     Kind
	Phase    Phase
	Outcome  acpMappingOutcome
	Evidence string
}

// kindACPMappingOutcome declares the ACP mapping outcome per Kind. Phase does
// not change the projection target for any declared Kind per proposal
// section 6.2, so one entry per Kind covers every legal phase for that Kind.
var kindACPMappingOutcome = map[Kind]struct {
	Outcome  acpMappingOutcome
	Evidence string
}{
	KindMessage: {
		Outcome:  acpMappingOutcomeAgentMessageChunk,
		Evidence: "docs/internal/projects/acp-client/final-proposal.md section 6.2: Factory response MESSAGE -> agent_message_chunk",
	},
	KindReasoning: {
		Outcome:  acpMappingOutcomeAgentThoughtChunk,
		Evidence: "final-proposal.md section 6.2: Factory response REASONING -> agent_thought_chunk",
	},
	KindUsage: {
		Outcome:  acpMappingOutcomeUsageUpdate,
		Evidence: "final-proposal.md section 6.2: USAGE with meaningful primary context -> usage_update",
	},
	KindStreamGap: {
		Outcome:  acpMappingOutcomeGapNotice,
		Evidence: "final-proposal.md section 6.2: STREAM_GAP -> surfaced as an explicit gap notice, never fabricated content",
	},
	KindError: {
		Outcome:  acpMappingOutcomeOutOfBandError,
		Evidence: "final-proposal.md section 6.4: errors surface via the JSON-RPC error channel, not the ACP SessionUpdate stream; declared explicitly so ERROR is never conflated with an undeclared or dropped pair",
	},
	KindTool: {
		Outcome:  acpMappingOutcomeNoOutput,
		Evidence: "final-proposal.md section 6.2: TOOL -> no output in L1 — declared, not dropped",
	},
	KindFileChange: {
		Outcome:  acpMappingOutcomeNoOutput,
		Evidence: "final-proposal.md section 6.2: FILE_CHANGE -> no output in L1 — declared, not dropped",
	},
	KindPlan: {
		Outcome:  acpMappingOutcomeNoOutput,
		Evidence: "final-proposal.md section 6.2: PLAN -> no output in L1 — declared, not dropped",
	},
	KindProgress: {
		Outcome:  acpMappingOutcomeNoOutput,
		Evidence: "final-proposal.md section 6.2: PROGRESS -> no output in L1 — declared, not dropped",
	},
	KindSession: {
		Outcome:  acpMappingOutcomeNoOutput,
		Evidence: "final-proposal.md section 6.2: SESSION -> no output in L1 — declared, not dropped",
	},
	KindRun: {
		Outcome:  acpMappingOutcomeNoOutput,
		Evidence: "final-proposal.md section 6.2: RUN -> no output in L1 — declared, not dropped",
	},
	KindTurn: {
		Outcome:  acpMappingOutcomeNoOutput,
		Evidence: "final-proposal.md section 6.2: TURN -> no output in L1 — declared, not dropped. Distinct from the Chat Sessions turn-terminal ACP projection (\"Factory turn terminal\" -> prompt result), which is not a workers.Draft Kind.",
	},
}

// declaredDraftKinds is an independently hand-authored inventory of every
// Kind this package's Kind.Validate() accepts (mirrored from the const block
// and switch in response_drafts.go), maintained separately from
// KnownKinds()/workstationdraftvalidation's allow-list. Comparing this list
// against KnownKinds() (see TestDeclaredDraftKindsMatchesKnownKinds) and
// using it -- not KnownKinds() -- as the "legal" side of the ACP mapping
// parity check means a Kind added to the Validate() switch but never
// reflected into the internal draftvalidation allow-list is observable as
// drift, instead of silently vanishing because both sides of the comparison
// shared the same single source.
var declaredDraftKinds = []Kind{
	KindSession, KindRun, KindTurn, KindMessage, KindReasoning, KindTool,
	KindFileChange, KindPlan, KindProgress, KindUsage, KindError, KindStreamGap,
}

// workerDraftACPMappingPair names one legal (Kind, Phase) response draft
// pair, independent of any declared outcome.
type workerDraftACPMappingPair struct {
	Kind  Kind
	Phase Phase
}

// workerDraftACPMappingParityInput carries the independently supplied
// inventories a parity comparison checks against each other, mirroring
// pkg/services/recordings/internal/events/kinds/parity.go's
// FactoryEventKindParityInput shape: the legal-pair inventory and the
// declared-outcome table are supplied separately rather than one being
// derived from the other.
type workerDraftACPMappingParityInput struct {
	LegalPairs []workerDraftACPMappingPair
	Declared   []workerDraftACPMappingOutcome
}

// workerDraftACPMappingParityDrift names legal (Kind, Phase) response draft
// pairs with no declared ACP mapping outcome. Any drift means the ACP
// mapping table silently dropped a case instead of declaring an explicit
// outcome, including "no output."
type workerDraftACPMappingParityDrift struct {
	UndeclaredPairs []workerDraftACPMappingPair
}

func (d workerDraftACPMappingParityDrift) Error() string {
	pairs := make([]string, 0, len(d.UndeclaredPairs))
	for _, pair := range d.UndeclaredPairs {
		pairs = append(pairs, fmt.Sprintf("%s/%s", pair.Kind, pair.Phase))
	}
	return fmt.Sprintf("undeclared ACP mapping outcome for response draft pairs: %s", strings.Join(pairs, ", "))
}

// legalWorkerDraftACPMappingPairs expands declaredDraftKinds (the
// independently authored inventory, not knownKindsFromInternal()) across
// every phase allowedPhasesForKind declares legal for that kind.
func legalWorkerDraftACPMappingPairs(t *testing.T) []workerDraftACPMappingPair {
	t.Helper()

	var pairs []workerDraftACPMappingPair
	for _, kind := range declaredDraftKinds {
		phases, ok := allowedPhasesForKind(kind)
		if !ok {
			t.Fatalf("allowedPhasesForKind(%q) reported kind unknown for a declaredDraftKinds entry", kind)
		}
		for _, phase := range phases {
			pairs = append(pairs, workerDraftACPMappingPair{Kind: kind, Phase: phase})
		}
	}
	return pairs
}

// declaredWorkerDraftACPMappingOutcomes returns the declared ACP mapping
// outcome for every pair in legalPairs that has an entry in
// kindACPMappingOutcome.
func declaredWorkerDraftACPMappingOutcomes(legalPairs []workerDraftACPMappingPair) []workerDraftACPMappingOutcome {
	var out []workerDraftACPMappingOutcome
	for _, pair := range legalPairs {
		declared, ok := kindACPMappingOutcome[pair.Kind]
		if !ok {
			continue
		}
		out = append(out, workerDraftACPMappingOutcome{
			Kind:     pair.Kind,
			Phase:    pair.Phase,
			Outcome:  declared.Outcome,
			Evidence: declared.Evidence,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Phase < out[j].Phase
	})
	return out
}

// compareWorkerDraftACPMappingParity returns drift between input.LegalPairs
// and input.Declared: legal pairs with no declared outcome.
func compareWorkerDraftACPMappingParity(input workerDraftACPMappingParityInput) workerDraftACPMappingParityDrift {
	declared := make(map[workerDraftACPMappingPair]struct{}, len(input.Declared))
	for _, entry := range input.Declared {
		declared[workerDraftACPMappingPair{Kind: entry.Kind, Phase: entry.Phase}] = struct{}{}
	}

	var undeclared []workerDraftACPMappingPair
	for _, pair := range input.LegalPairs {
		if _, ok := declared[pair]; !ok {
			undeclared = append(undeclared, pair)
		}
	}
	sort.Slice(undeclared, func(i, j int) bool {
		if undeclared[i].Kind != undeclared[j].Kind {
			return undeclared[i].Kind < undeclared[j].Kind
		}
		return undeclared[i].Phase < undeclared[j].Phase
	})
	return workerDraftACPMappingParityDrift{UndeclaredPairs: undeclared}
}

// TestKnownKindsFromInternalMatchesDeclaredKindSet proves
// knownKindsFromInternal() returns exactly the twelve declared response
// draft kinds, each individually valid.
func TestKnownKindsFromInternalMatchesDeclaredKindSet(t *testing.T) {
	t.Parallel()

	kinds := knownKindsFromInternal()
	if len(kinds) != 12 {
		t.Fatalf("knownKindsFromInternal() returned %d kinds, want 12", len(kinds))
	}
	for _, kind := range kinds {
		if err := kind.Validate(); err != nil {
			t.Fatalf("knownKindsFromInternal() returned %q which fails Validate(): %v", kind, err)
		}
	}
}

// TestAllowedPhasesForKindInternalReportsUnknownKind proves
// allowedPhasesForKind() reports an unknown kind and returns a non-empty,
// individually valid phase set for every declared kind.
func TestAllowedPhasesForKindInternalReportsUnknownKind(t *testing.T) {
	t.Parallel()

	if _, ok := allowedPhasesForKind("NOT_A_KIND"); ok {
		t.Fatalf("allowedPhasesForKind(unknown) ok = true, want false")
	}

	for _, kind := range knownKindsFromInternal() {
		phases, ok := allowedPhasesForKind(kind)
		if !ok {
			t.Fatalf("allowedPhasesForKind(%q) ok = false, want true", kind)
		}
		if len(phases) == 0 {
			t.Fatalf("allowedPhasesForKind(%q) returned no phases", kind)
		}
		for _, phase := range phases {
			if err := phase.Validate(); err != nil {
				t.Fatalf("allowedPhasesForKind(%q) returned invalid phase %q: %v", kind, phase, err)
			}
		}
	}
}

// TestDeclaredDraftKindsMatchesKnownKinds proves the independently
// hand-authored declaredDraftKinds inventory and the internal
// draftvalidation allow-list (via knownKindsFromInternal()) agree on the
// exact same Kind set. If a Kind is ever added to one and not the other,
// this test -- not the ACP mapping parity check -- is what catches it, so
// the mapping check below can safely trust declaredDraftKinds as
// authoritative.
func TestDeclaredDraftKindsMatchesKnownKinds(t *testing.T) {
	t.Parallel()

	want := make(map[Kind]struct{}, len(declaredDraftKinds))
	for _, kind := range declaredDraftKinds {
		if err := kind.Validate(); err != nil {
			t.Fatalf("declaredDraftKinds entry %q fails Kind.Validate(): %v", kind, err)
		}
		want[kind] = struct{}{}
	}

	got := make(map[Kind]struct{})
	for _, kind := range knownKindsFromInternal() {
		got[kind] = struct{}{}
	}

	for kind := range want {
		if _, ok := got[kind]; !ok {
			t.Errorf("declaredDraftKinds contains %q but knownKindsFromInternal() does not", kind)
		}
	}
	for kind := range got {
		if _, ok := want[kind]; !ok {
			t.Errorf("knownKindsFromInternal() contains %q but declaredDraftKinds does not", kind)
		}
	}
}

// TestValidateWorkerDraftACPMappingParityIsClean proves the declared outcome
// table currently has zero drift against the legal (Kind, Phase) pairs
// derived from the independently authored declaredDraftKinds inventory.
func TestValidateWorkerDraftACPMappingParityIsClean(t *testing.T) {
	t.Parallel()

	legalPairs := legalWorkerDraftACPMappingPairs(t)
	drift := compareWorkerDraftACPMappingParity(workerDraftACPMappingParityInput{
		LegalPairs: legalPairs,
		Declared:   declaredWorkerDraftACPMappingOutcomes(legalPairs),
	})
	if len(drift.UndeclaredPairs) != 0 {
		t.Fatalf("compareWorkerDraftACPMappingParity() UndeclaredPairs = %v, want empty", drift.UndeclaredPairs)
	}
}

// TestCompareWorkerDraftACPMappingParityDetectsAnUndeclaredPair proves the
// parity check actually fails closed when a legal (Kind, Phase) pair loses
// its declared outcome, rather than only ever observing the already-clean
// state.
func TestCompareWorkerDraftACPMappingParityDetectsAnUndeclaredPair(t *testing.T) {
	original, ok := kindACPMappingOutcome[KindTool]
	if !ok {
		t.Fatalf("kindACPMappingOutcome missing baseline entry for KindTool")
	}
	delete(kindACPMappingOutcome, KindTool)
	t.Cleanup(func() {
		kindACPMappingOutcome[KindTool] = original
	})

	legalPairs := legalWorkerDraftACPMappingPairs(t)
	drift := compareWorkerDraftACPMappingParity(workerDraftACPMappingParityInput{
		LegalPairs: legalPairs,
		Declared:   declaredWorkerDraftACPMappingOutcomes(legalPairs),
	})
	if len(drift.UndeclaredPairs) == 0 {
		t.Fatalf("compareWorkerDraftACPMappingParity() UndeclaredPairs is empty, want drift for every legal TOOL phase")
	}

	toolPhases, ok := allowedPhasesForKind(KindTool)
	if !ok || len(toolPhases) == 0 {
		t.Fatalf("allowedPhasesForKind(KindTool) returned no phases")
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
	if drift.Error() == "" {
		t.Fatalf("drift error message is empty")
	}
}

// TestDeclaredWorkerDraftACPMappingOutcomesCoversEveryLegalPair proves the
// declared outcome table is exhaustive over every (Kind, Phase) pair the
// existing draft validator treats as legal, including an explicit outcome
// for every phase of every kind, not just one representative phase per kind.
func TestDeclaredWorkerDraftACPMappingOutcomesCoversEveryLegalPair(t *testing.T) {
	t.Parallel()

	legalPairs := legalWorkerDraftACPMappingPairs(t)
	declared := declaredWorkerDraftACPMappingOutcomes(legalPairs)

	declaredSet := make(map[Kind]map[Phase]acpMappingOutcome)
	for _, entry := range declared {
		if entry.Outcome == "" {
			t.Fatalf("declared entry for %s/%s has empty Outcome", entry.Kind, entry.Phase)
		}
		if entry.Evidence == "" {
			t.Fatalf("declared entry for %s/%s has empty Evidence", entry.Kind, entry.Phase)
		}
		if declaredSet[entry.Kind] == nil {
			declaredSet[entry.Kind] = make(map[Phase]acpMappingOutcome)
		}
		declaredSet[entry.Kind][entry.Phase] = entry.Outcome
	}

	for _, pair := range legalPairs {
		if _, ok := declaredSet[pair.Kind][pair.Phase]; !ok {
			t.Fatalf("no declared ACP mapping outcome for legal pair %s/%s", pair.Kind, pair.Phase)
		}
	}
	if len(declared) != len(legalPairs) {
		t.Fatalf("declaredWorkerDraftACPMappingOutcomes() returned %d entries, want %d (exactly the legal pairs, no extras)", len(declared), len(legalPairs))
	}
}

// TestDeclaredWorkerDraftACPMappingOutcomesDeclaresExplicitNoOutput proves the
// "declared, not dropped" no-output kinds from proposal section 6.2 resolve
// to the explicit NoOutput outcome rather than being silently absent.
func TestDeclaredWorkerDraftACPMappingOutcomesDeclaresExplicitNoOutput(t *testing.T) {
	t.Parallel()

	noOutputKinds := []Kind{
		KindTool, KindFileChange, KindPlan, KindProgress, KindSession, KindRun, KindTurn,
	}
	legalPairs := legalWorkerDraftACPMappingPairs(t)
	byKind := make(map[Kind][]workerDraftACPMappingOutcome)
	for _, entry := range declaredWorkerDraftACPMappingOutcomes(legalPairs) {
		byKind[entry.Kind] = append(byKind[entry.Kind], entry)
	}

	for _, kind := range noOutputKinds {
		entries, ok := byKind[kind]
		if !ok || len(entries) == 0 {
			t.Fatalf("no declared entries for %q", kind)
		}
		for _, entry := range entries {
			if entry.Outcome != acpMappingOutcomeNoOutput {
				t.Fatalf("%s/%s outcome = %q, want %q", entry.Kind, entry.Phase, entry.Outcome, acpMappingOutcomeNoOutput)
			}
		}
	}
}

// TestDeclaredWorkerDraftACPMappingOutcomesStreamingContentKinds proves the
// content-bearing kinds resolve to their declared non-empty ACP projection
// outcome rather than NoOutput or OutOfBandError.
func TestDeclaredWorkerDraftACPMappingOutcomesStreamingContentKinds(t *testing.T) {
	t.Parallel()

	want := map[Kind]acpMappingOutcome{
		KindMessage:   acpMappingOutcomeAgentMessageChunk,
		KindReasoning: acpMappingOutcomeAgentThoughtChunk,
		KindUsage:     acpMappingOutcomeUsageUpdate,
		KindStreamGap: acpMappingOutcomeGapNotice,
		KindError:     acpMappingOutcomeOutOfBandError,
	}
	legalPairs := legalWorkerDraftACPMappingPairs(t)
	byKind := make(map[Kind][]workerDraftACPMappingOutcome)
	for _, entry := range declaredWorkerDraftACPMappingOutcomes(legalPairs) {
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
