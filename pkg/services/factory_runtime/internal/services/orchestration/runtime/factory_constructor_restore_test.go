package runtime

import (
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestRestoredDispatchRequestEventPreservesRestartMetadataAndResources(t *testing.T) {
	now := time.Date(2026, time.April, 10, 12, 0, 0, 0, time.UTC)
	cfg := &runtimeConfig{
		clock:              platformclock.NewDeterministic(now, time.Second),
		restoredWorldState: &interfaces.FactoryWorldState{Tick: 9},
	}
	dispatch := interfaces.FactoryWorldDispatch{
		DispatchID:               "dispatch-restart",
		TransitionID:             "t-process",
		StartedTick:              11,
		StartedAt:                now.Add(-time.Minute),
		RunnerID:                 "runner-restart",
		RunnerSelectionSource:    workerexecution.RunnerSelectionSourceFactory,
		WorkItemIDs:              []string{"work-restart", "work-restart"},
		CurrentChainingTraceID:   "trace-current",
		PreviousChainingTraceIDs: []string{"trace-previous", "trace-previous", ""},
		TraceIDs:                 []string{"trace-restart", "trace-restart", ""},
		Inputs: []interfaces.WorkstationInput{
			{TokenID: "work-restart", PlaceID: "task:init", WorkItem: &work.FactoryWorkItem{ID: "work-restart"}},
			{TokenID: "resource-token", PlaceID: "gpu:available", Resource: &interfaces.FactoryResourceUnit{ResourceID: "gpu", TokenID: "resource-token"}},
		},
		Resources: []interfaces.FactoryResourceUnit{
			{ResourceID: "gpu", TokenID: "resource-token"},
			{ResourceID: " ", TokenID: "ignored"},
		},
	}

	event, err := restoredDispatchRequestEvent(cfg, dispatch)
	if err != nil {
		t.Fatalf("restoredDispatchRequestEvent: %v", err)
	}
	var payload interfaces.DispatchRequestEventPayload
	if err := event.DecodePayload(&payload); err != nil {
		t.Fatalf("decode restored dispatch request: %v", err)
	}
	assertRestoredDispatchRequestEnvelope(t, event, dispatch)
	assertRestoredDispatchRequestMetadata(t, payload, dispatch)
	assertRestoredDispatchRequestResources(t, payload)
	assertRestoredDispatchRequestInputs(t, payload)
	assertEmptyRestoredDispatchResourceRefs(t)
}

func assertRestoredDispatchRequestEnvelope(t *testing.T, event interfaces.FactoryEvent, dispatch interfaces.FactoryWorldDispatch) {
	t.Helper()
	if event.Type != interfaces.FactoryEventTypeDispatchRequest {
		t.Fatalf("restored dispatch request type = %q, want %q", event.Type, interfaces.FactoryEventTypeDispatchRequest)
	}
	if stringPointerValue(event.Context.DispatchID) != dispatch.DispatchID {
		t.Fatalf("restored dispatch request dispatch ID = %q, want %q", stringPointerValue(event.Context.DispatchID), dispatch.DispatchID)
	}
}

func assertRestoredDispatchRequestMetadata(t *testing.T, payload interfaces.DispatchRequestEventPayload, dispatch interfaces.FactoryWorldDispatch) {
	t.Helper()
	if payload.TransitionID != dispatch.TransitionID {
		t.Fatalf("restored dispatch transition = %q, want %q", payload.TransitionID, dispatch.TransitionID)
	}
	if payload.Metadata == nil {
		t.Fatal("restored dispatch metadata = nil, want runner facts")
	}
	if stringPointerValue(payload.Metadata.RunnerID) != dispatch.RunnerID {
		t.Fatalf("restored dispatch runner ID = %q, want %q", stringPointerValue(payload.Metadata.RunnerID), dispatch.RunnerID)
	}
	if payload.Metadata.RunnerSelectionSource == nil {
		t.Fatal("restored dispatch runner selection source = nil")
	}
	if *payload.Metadata.RunnerSelectionSource != dispatch.RunnerSelectionSource {
		t.Fatalf("restored dispatch runner selection source = %q, want %q", *payload.Metadata.RunnerSelectionSource, dispatch.RunnerSelectionSource)
	}
}

func assertRestoredDispatchRequestResources(t *testing.T, payload interfaces.DispatchRequestEventPayload) {
	t.Helper()
	if payload.Resources == nil {
		t.Fatal("restored dispatch resources = nil, want one gpu resource")
	}
	if len(*payload.Resources) != 1 {
		t.Fatalf("restored dispatch resource count = %d, want one", len(*payload.Resources))
	}
	if (*payload.Resources)[0].Name != "gpu" {
		t.Fatalf("restored dispatch resource name = %q, want gpu", (*payload.Resources)[0].Name)
	}
}

func assertRestoredDispatchRequestInputs(t *testing.T, payload interfaces.DispatchRequestEventPayload) {
	t.Helper()
	if len(payload.Inputs) != 1 {
		t.Fatalf("restored dispatch input count = %d, want one deduplicated Work input", len(payload.Inputs))
	}
	if payload.Inputs[0].WorkID != "work-restart" {
		t.Fatalf("restored dispatch Work input = %q, want work-restart", payload.Inputs[0].WorkID)
	}
}

func assertEmptyRestoredDispatchResourceRefs(t *testing.T) {
	t.Helper()
	if got := restoredDispatchResourceRefs(nil); got != nil {
		t.Fatalf("empty restored resource refs = %#v, want nil", got)
	}
	if got := restoredDispatchResourceRefs([]interfaces.FactoryResourceUnit{{ResourceID: " "}}); got != nil {
		t.Fatalf("blank restored resource refs = %#v, want nil", got)
	}
}

func TestRestoredWorkPlacementHandlesApprovalAndTokenIdentityCollisions(t *testing.T) {
	net := buildSimpleNet()
	net.Transitions["t-approval"] = &petri.Transition{ID: "t-approval", Type: petri.TransitionHumanApproval}
	item := work.FactoryWorkItem{ID: "work-restored", WorkTypeID: "task", State: "init"}
	pendingApproval := interfaces.FactoryWorldDispatch{DispatchID: "dispatch-approval", TransitionID: "t-process"}
	approvalWorld := &interfaces.FactoryWorldState{
		PendingHumanApprovalsByID: map[string]interfaces.FactoryWorldHumanApproval{
			"approval": {ApprovalID: "approval", DispatchID: pendingApproval.DispatchID},
		},
	}
	if !restoredDispatchIsHumanApproval(approvalWorld, net, pendingApproval) {
		t.Fatal("pending approval dispatch was not recognized as human approval")
	}
	if !restoredDispatchIsHumanApproval(nil, net, interfaces.FactoryWorldDispatch{TransitionID: "t-approval"}) {
		t.Fatal("human-approval transition was not recognized")
	}
	if restoredDispatchIsHumanApproval(nil, nil, interfaces.FactoryWorldDispatch{DispatchID: "dispatch-normal"}) {
		t.Fatal("ordinary dispatch was recognized as human approval")
	}

	marking := petri.NewMarking("restart-test")
	existing := restoredWorkToken(item, "task:init", "", nil, time.Unix(0, 0).UTC())
	existing.ID = "restored-work:" + item.ID
	marking.AddToken(existing)
	first := uniqueRestoredWorkTokenID(marking, item.ID)
	if first != "restored-work:work-restored:2" {
		t.Fatalf("first collision-safe token ID = %q, want restored-work:work-restored:2", first)
	}
	second := restoredWorkToken(item, "task:init", "", nil, time.Unix(0, 0).UTC())
	second.ID = first
	marking.AddToken(second)
	if got := uniqueRestoredWorkTokenID(marking, item.ID); got != "restored-work:work-restored:3" {
		t.Fatalf("repeated collision-safe token ID = %q, want restored-work:work-restored:3", got)
	}
}
