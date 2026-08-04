package mapping

import (
	"encoding/json"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// dispatchOutcome names the SessionUpdate variant family Project is
// expected to produce for a legal Kind/Phase pair.
type dispatchOutcome string

const (
	outcomeMessageChunk dispatchOutcome = "MESSAGE_CHUNK"
	outcomeThoughtChunk dispatchOutcome = "THOUGHT_CHUNK"
	outcomeUsageUpdate  dispatchOutcome = "USAGE_UPDATE"
	outcomeSessionInfo  dispatchOutcome = "SESSION_INFO_UPDATE"
	outcomeNoUpdate     dispatchOutcome = "NO_UPDATE"
)

// dispatchPair is a comparable (Kind, Phase) pair used to key
// declaredDispatchOutcomes.
type dispatchPair struct {
	Kind  workers.Kind
	Phase workers.Phase
}

// declaredDispatchOutcomes assigns exactly one Project outcome family to
// every legal workers.Kind/Phase pair, matching
// draftvalidation.declaredACPL1Outcomes: MESSAGE->MESSAGE_CHUNK,
// REASONING->THOUGHT_CHUNK, USAGE->USAGE_UPDATE, STREAM_GAP->THOUGHT_CHUNK
// (this package's chosen gap representation, see gap.go),
// SESSION/PhaseUpdated->SESSION_INFO_UPDATE (see session.go; every other
// SESSION phase is a lifecycle transition), everything else -- including
// ERROR, declared ERROR_OUT_OF_BAND upstream -- resolves to NO_UPDATE from
// this pure record->SessionUpdate projector. Keyed by pair rather than Kind
// alone because SESSION is the one Kind whose outcome varies by phase.
var declaredDispatchOutcomes = map[dispatchPair]dispatchOutcome{
	{workers.KindSession, workers.PhaseStarted}:   outcomeNoUpdate,
	{workers.KindSession, workers.PhaseUpdated}:   outcomeSessionInfo,
	{workers.KindSession, workers.PhaseCompleted}: outcomeNoUpdate,
	{workers.KindSession, workers.PhaseFailed}:    outcomeNoUpdate,
	{workers.KindSession, workers.PhaseCanceled}:  outcomeNoUpdate,

	{workers.KindRun, workers.PhaseStarted}:   outcomeNoUpdate,
	{workers.KindRun, workers.PhaseCompleted}: outcomeNoUpdate,
	{workers.KindRun, workers.PhaseFailed}:    outcomeNoUpdate,
	{workers.KindRun, workers.PhaseCanceled}:  outcomeNoUpdate,

	{workers.KindTurn, workers.PhaseStarted}:   outcomeNoUpdate,
	{workers.KindTurn, workers.PhaseCompleted}: outcomeNoUpdate,
	{workers.KindTurn, workers.PhaseFailed}:    outcomeNoUpdate,
	{workers.KindTurn, workers.PhaseCanceled}:  outcomeNoUpdate,

	{workers.KindMessage, workers.PhaseStarted}:   outcomeMessageChunk,
	{workers.KindMessage, workers.PhaseDelta}:     outcomeMessageChunk,
	{workers.KindMessage, workers.PhaseCompleted}: outcomeMessageChunk,

	{workers.KindReasoning, workers.PhaseStarted}:   outcomeThoughtChunk,
	{workers.KindReasoning, workers.PhaseDelta}:     outcomeThoughtChunk,
	{workers.KindReasoning, workers.PhaseCompleted}: outcomeThoughtChunk,

	{workers.KindTool, workers.PhaseStarted}:   outcomeNoUpdate,
	{workers.KindTool, workers.PhaseDelta}:     outcomeNoUpdate,
	{workers.KindTool, workers.PhaseCompleted}: outcomeNoUpdate,
	{workers.KindTool, workers.PhaseFailed}:    outcomeNoUpdate,
	{workers.KindTool, workers.PhaseCanceled}:  outcomeNoUpdate,

	{workers.KindFileChange, workers.PhaseUpdated}: outcomeNoUpdate,
	{workers.KindPlan, workers.PhaseUpdated}:       outcomeNoUpdate,
	{workers.KindProgress, workers.PhaseUpdated}:   outcomeNoUpdate,

	{workers.KindUsage, workers.PhaseUpdated}: outcomeUsageUpdate,

	{workers.KindError, workers.PhaseUpdated}: outcomeNoUpdate,
	{workers.KindError, workers.PhaseFailed}:  outcomeNoUpdate,

	{workers.KindStreamGap, workers.PhaseUpdated}: outcomeThoughtChunk,
}

// allKinds and allPhases are the complete current enum vocabularies,
// independent of legality, so TestProjectDispatch_HandlesEveryCurrentPair
// exercises the full cross-product and fails closed when a new Kind or
// Phase constant is added without this package's dispatch table being
// updated to match.
var allKinds = []workers.Kind{
	workers.KindSession, workers.KindRun, workers.KindTurn, workers.KindMessage,
	workers.KindReasoning, workers.KindTool, workers.KindFileChange, workers.KindPlan,
	workers.KindProgress, workers.KindUsage, workers.KindError, workers.KindStreamGap,
}

var allPhases = []workers.Phase{
	workers.PhaseStarted, workers.PhaseDelta, workers.PhaseUpdated,
	workers.PhaseCompleted, workers.PhaseFailed, workers.PhaseCanceled,
}

// TestProjectDispatch_HandlesEveryCurrentPair exhaustively drives Project
// over the full Kind x Phase cross-product. Legal pairs must resolve to
// their declared outcome family; every other pair must be rejected as
// ErrMalformedRecord, never silently classified.
func TestProjectDispatch_HandlesEveryCurrentPair(t *testing.T) {
	t.Parallel()

	wantLegalPairs := 0
	for _, phases := range legalPhasesByKind {
		wantLegalPairs += len(phases)
	}
	if len(declaredDispatchOutcomes) != wantLegalPairs {
		t.Fatalf("declaredDispatchOutcomes has %d pairs, want %d (one per legalPhasesByKind entry)", len(declaredDispatchOutcomes), wantLegalPairs)
	}

	for _, kind := range allKinds {
		for _, phase := range allPhases {
			t.Run(string(kind)+"/"+string(phase), func(t *testing.T) {
				t.Parallel()

				draft := workers.Draft{Kind: kind, Phase: phase, Payload: fixturePayloadFor(t, kind, phase), ItemID: "item-1"}
				update, err := Project(draft)

				if !isLegalKindPhase(kind, phase) {
					requireMalformed(t, update, err)
					return
				}
				if err != nil {
					t.Fatalf("Project() unexpected err = %v", err)
				}

				switch declaredDispatchOutcomes[dispatchPair{Kind: kind, Phase: phase}] {
				case outcomeNoUpdate:
					requireNoUpdate(t, update)
				case outcomeMessageChunk:
					if update == nil || update.AgentMessageChunk == nil {
						t.Fatalf("Project() update = %+v, want a populated AgentMessageChunk", update)
					}
				case outcomeThoughtChunk:
					if update == nil || update.AgentThoughtChunk == nil {
						t.Fatalf("Project() update = %+v, want a populated AgentThoughtChunk", update)
					}
				case outcomeUsageUpdate:
					if update == nil || update.UsageUpdate == nil {
						t.Fatalf("Project() update = %+v, want a populated UsageUpdate", update)
					}
				case outcomeSessionInfo:
					if update == nil || update.SessionInfoUpdate == nil {
						t.Fatalf("Project() update = %+v, want a populated SessionInfoUpdate", update)
					}
				default:
					t.Fatalf("no declared dispatch outcome for kind %q phase %q", kind, phase)
				}
			})
		}
	}
}

// fixturePayloadFor returns a payload that, for a legal (kind, phase) pair,
// actually produces that kind's declared outcome -- not merely a
// schema-valid payload that might still resolve to no-output (e.g. a blank
// MESSAGE role). For a no-output kind, or for an illegal or unknown pair,
// the payload content is irrelevant: Project either never decodes it (a
// no-output Kind) or rejects on the pair itself before decoding.
func fixturePayloadFor(t *testing.T, kind workers.Kind, phase workers.Phase) json.RawMessage {
	t.Helper()

	switch kind {
	case workers.KindSession:
		if phase == workers.PhaseUpdated {
			title := "Renamed session"
			return mustMarshal(t, workers.SessionPayload{Title: &title})
		}
		return json.RawMessage(`{}`)
	case workers.KindMessage:
		if phase == workers.PhaseDelta {
			return mustMarshal(t, workers.MessageDeltaPayload{ContentBlockKind: workers.ContentBlockText, TextDelta: "hi"})
		}
		return mustMarshal(t, workers.MessagePayload{
			Role:          "assistant",
			ContentBlocks: []workers.ContentBlock{{Kind: workers.ContentBlockText, Text: "hi"}},
		})
	case workers.KindReasoning:
		if phase == workers.PhaseDelta {
			return mustMarshal(t, workers.ReasoningPayload{SummaryDelta: "thinking"})
		}
		return mustMarshal(t, workers.ReasoningPayload{Summary: "thinking"})
	case workers.KindUsage:
		return mustMarshal(t, workers.UsagePayload{TotalTokens: 10})
	case workers.KindStreamGap:
		return mustMarshal(t, workers.StreamGapPayload{FromSequence: 1, ToSequence: 2, FirstAvailableSequence: 3})
	default:
		return json.RawMessage(`{}`)
	}
}

// TestProjectDispatch_RejectsUnknownKind proves an unrecognized Kind value
// is rejected the same bounded, typed way as a malformed payload, never
// silently treated as no-output.
func TestProjectDispatch_RejectsUnknownKind(t *testing.T) {
	t.Parallel()

	update, err := Project(workers.Draft{Kind: "NOT_A_KIND", Phase: workers.PhaseUpdated, Payload: json.RawMessage(`{}`)})
	requireMalformed(t, update, err)
}

// TestProjectDispatch_RejectsIllegalPhaseForKnownKind proves a phase that
// is not legal for an otherwise-known Kind (e.g. SESSION/DELTA) is
// rejected, matching draftvalidation's own rejection of that pair.
func TestProjectDispatch_RejectsIllegalPhaseForKnownKind(t *testing.T) {
	t.Parallel()

	update, err := Project(workers.Draft{Kind: workers.KindSession, Phase: workers.PhaseDelta, Payload: json.RawMessage(`{}`)})
	requireMalformed(t, update, err)
}
