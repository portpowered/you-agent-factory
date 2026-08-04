package mapping

import (
	"encoding/json"
	"errors"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"

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

func TestProjectChildOpeningUsesStoredItemIdentityAndAssociation(t *testing.T) {
	t.Parallel()

	opening := childSessionDraft(workers.PhaseStarted, "child-item-1", "", "STARTING")
	update, err := ProjectChildOpening(opening, ChildAssociation{DispatchID: "dispatch-1", WorkerSessionID: "worker-1"})
	if err != nil {
		t.Fatalf("ProjectChildOpening() error = %v", err)
	}
	if update == nil || update.ToolCall == nil {
		t.Fatalf("ProjectChildOpening() update = %#v, want tool call", update)
	}
	if err := update.Validate(); err != nil {
		t.Fatalf("ProjectChildOpening() update validation error = %v", err)
	}
	call := update.ToolCall
	if call.ToolCallId != acpsdk.ToolCallId(opening.ItemID) {
		t.Errorf("ToolCallId = %q, want stored item %q", call.ToolCallId, opening.ItemID)
	}
	if call.Status != acpsdk.ToolCallStatusPending {
		t.Errorf("Status = %q, want pending", call.Status)
	}
	if call.Kind != acpsdk.ToolKindExecute {
		t.Errorf("Kind = %q, want execute", call.Kind)
	}
	metadata, ok := call.Meta[workerToolCallMetaKey].(map[string]string)
	if !ok {
		t.Fatalf("metadata = %#v, want associated child metadata", call.Meta)
	}
	if metadata["dispatchId"] != "dispatch-1" || metadata["workerSessionId"] != "worker-1" {
		t.Errorf("metadata = %#v, want canonical association", metadata)
	}
}

func TestProjectChildOpeningRejectsMissingIdentityOrAssociation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		draft       workers.Draft
		association ChildAssociation
		want        error
	}{
		{"missing association", childSessionDraft(workers.PhaseStarted, "item-1", "", "RESERVED"), ChildAssociation{}, ErrMissingChildAssociation},
		{"missing item", childSessionDraft(workers.PhaseStarted, "", "", "RESERVED"), ChildAssociation{DispatchID: "dispatch-1", WorkerSessionID: "worker-1"}, ErrMissingChildItemID},
		{"opening has parent", childSessionDraft(workers.PhaseStarted, "item-1", "other-item", "RESERVED"), ChildAssociation{DispatchID: "dispatch-1", WorkerSessionID: "worker-1"}, ErrMalformedRecord},
		{"opening already running", childSessionDraft(workers.PhaseStarted, "item-1", "", "RUNNING"), ChildAssociation{DispatchID: "dispatch-1", WorkerSessionID: "worker-1"}, ErrMalformedRecord},
		{"malformed payload", workers.Draft{Kind: workers.KindSession, Phase: workers.PhaseStarted, ItemID: "item-1", Payload: json.RawMessage(`not-json`)}, ChildAssociation{DispatchID: "dispatch-1", WorkerSessionID: "worker-1"}, ErrMalformedRecord},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			update, err := ProjectChildOpening(tt.draft, tt.association)
			if update != nil {
				t.Fatalf("ProjectChildOpening() update = %#v, want no partial update", update)
			}
			if !errors.Is(err, tt.want) {
				t.Fatalf("ProjectChildOpening() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestProjectChildLifecycleTargetsStoredParentAndMapsTerminals(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		phase  workers.Phase
		status string
		want   acpsdk.ToolCallStatus
	}{
		{"running", workers.PhaseUpdated, "RUNNING", acpsdk.ToolCallStatusInProgress},
		{"completed", workers.PhaseCompleted, "COMPLETED", acpsdk.ToolCallStatusCompleted},
		{"failed", workers.PhaseFailed, "FAILED", acpsdk.ToolCallStatusFailed},
		{"canceled", workers.PhaseCanceled, "CANCELED", acpsdk.ToolCallStatusFailed},
		{"terminated", workers.PhaseCanceled, "TERMINATED", acpsdk.ToolCallStatusFailed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			draft := childSessionDraft(tt.phase, "record-item", "child-tool-call", tt.status)
			update, err := ProjectChildLifecycle(draft)
			if err != nil {
				t.Fatalf("ProjectChildLifecycle() error = %v", err)
			}
			if update == nil || update.ToolCallUpdate == nil || update.ToolCallUpdate.Status == nil {
				t.Fatalf("ProjectChildLifecycle() update = %#v, want tool call update", update)
			}
			if err := update.Validate(); err != nil {
				t.Fatalf("ProjectChildLifecycle() update validation error = %v", err)
			}
			if update.ToolCallUpdate.ToolCallId != acpsdk.ToolCallId(draft.ParentItemID) {
				t.Errorf("ToolCallId = %q, want stored parent %q", update.ToolCallUpdate.ToolCallId, draft.ParentItemID)
			}
			if *update.ToolCallUpdate.Status != tt.want {
				t.Errorf("Status = %q, want %q", *update.ToolCallUpdate.Status, tt.want)
			}
		})
	}
}

func TestProjectChildLifecycleKeepsPendingStatusAndRejectsMalformedLineage(t *testing.T) {
	t.Parallel()

	pending, err := ProjectChildLifecycle(childSessionDraft(workers.PhaseUpdated, "record-item", "child-tool-call", "STARTING"))
	if err != nil || pending != nil {
		t.Fatalf("pending lifecycle = (%#v, %v), want declared no output", pending, err)
	}

	tests := []struct {
		name  string
		draft workers.Draft
		want  error
	}{
		{"missing parent", childSessionDraft(workers.PhaseUpdated, "record-item", "", "RUNNING"), ErrMissingChildParent},
		{"bad terminal status", childSessionDraft(workers.PhaseCompleted, "record-item", "child-tool-call", "FAILED"), ErrMalformedRecord},
		{"opening sent as update", childSessionDraft(workers.PhaseStarted, "record-item", "child-tool-call", "STARTING"), ErrMalformedRecord},
		{"non lifecycle record", workers.Draft{Kind: workers.KindMessage, Phase: workers.PhaseDelta, ParentItemID: "child-tool-call", Payload: json.RawMessage(`{}`)}, ErrMalformedRecord},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			update, err := ProjectChildLifecycle(tt.draft)
			if update != nil {
				t.Fatalf("ProjectChildLifecycle() update = %#v, want no partial update", update)
			}
			if !errors.Is(err, tt.want) {
				t.Fatalf("ProjectChildLifecycle() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func childSessionDraft(phase workers.Phase, itemID, parentItemID, status string) workers.Draft {
	payload, _ := json.Marshal(workers.SessionPayload{Status: status})
	return workers.Draft{
		Kind: workers.KindSession, Phase: phase, ItemID: itemID, ParentItemID: parentItemID, Payload: payload,
	}
}
