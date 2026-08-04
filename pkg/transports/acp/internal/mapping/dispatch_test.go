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
	outcomeNoUpdate     dispatchOutcome = "NO_UPDATE"
)

// declaredDispatchOutcomes assigns exactly one Project outcome family to
// every current workers.Kind, matching
// draftvalidation.declaredACPL1Outcomes' per-Kind families:
// MESSAGE->MESSAGE_CHUNK, REASONING->THOUGHT_CHUNK, USAGE->USAGE_UPDATE,
// STREAM_GAP->THOUGHT_CHUNK (this package's chosen gap representation, see
// gap.go), everything else -- including ERROR, declared ERROR_OUT_OF_BAND
// upstream -- resolves to NO_UPDATE from this pure record->SessionUpdate
// projector.
var declaredDispatchOutcomes = map[workers.Kind]dispatchOutcome{
	workers.KindSession:    outcomeNoUpdate,
	workers.KindRun:        outcomeNoUpdate,
	workers.KindTurn:       outcomeNoUpdate,
	workers.KindMessage:    outcomeMessageChunk,
	workers.KindReasoning:  outcomeThoughtChunk,
	workers.KindTool:       outcomeNoUpdate,
	workers.KindFileChange: outcomeNoUpdate,
	workers.KindPlan:       outcomeNoUpdate,
	workers.KindProgress:   outcomeNoUpdate,
	workers.KindUsage:      outcomeUsageUpdate,
	workers.KindError:      outcomeNoUpdate,
	workers.KindStreamGap:  outcomeThoughtChunk,
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

	if len(declaredDispatchOutcomes) != len(allKinds) {
		t.Fatalf("declaredDispatchOutcomes has %d kinds, want %d (one per allKinds entry)", len(declaredDispatchOutcomes), len(allKinds))
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

				switch declaredDispatchOutcomes[kind] {
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
				default:
					t.Fatalf("no declared dispatch outcome for kind %q", kind)
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
