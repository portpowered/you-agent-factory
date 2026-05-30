package projections_test

import (
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	. "github.com/portpowered/infinite-you/pkg/factory/projections"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestBuildFactoryWorldView_EnrichesActiveExecutionWorkItemRefsWithSelectedWorkPayload(t *testing.T) {
	t0 := time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC)
	events := workPayloadLineageProjectionEvents(t, t0)

	activeState, err := ReconstructFactoryWorldState(events, 2)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState active tick: %v", err)
	}
	activeView := BuildFactoryWorldView(activeState)
	execution := activeView.Runtime.ActiveExecutionsByDispatchID["dispatch-1"]
	if len(execution.WorkItems) != 1 || execution.WorkItems[0].WorkID != "work-1" {
		t.Fatalf("active execution work items = %#v, want work-1", execution.WorkItems)
	}
	if execution.WorkItems[0].PayloadStatus != string(interfaces.WorkPayloadResolutionResolved) {
		t.Fatalf("active work payload status = %q, want RESOLVED", execution.WorkItems[0].PayloadStatus)
	}
	assertWorkItemRefLineageTextContent(t, execution.WorkItems[0], "draft-v1")

	latestState, err := ReconstructFactoryWorldState(events, 4)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState latest tick: %v", err)
	}
	latestView := BuildFactoryWorldView(latestState)
	var work1Ref *interfaces.FactoryWorldWorkItemRef
	for _, refs := range latestView.Runtime.PlaceOccupancyWorkItemsByPlaceID {
		for i := range refs {
			if refs[i].WorkID == "work-1" {
				work1Ref = &refs[i]
				break
			}
		}
	}
	if work1Ref == nil {
		t.Fatalf("place occupancy refs = %#v, want work-1 somewhere", latestView.Runtime.PlaceOccupancyWorkItemsByPlaceID)
	}
	if work1Ref.PayloadStatus != string(interfaces.WorkPayloadResolutionResolved) {
		t.Fatalf("work-1 payload status = %q, want RESOLVED", work1Ref.PayloadStatus)
	}
	assertWorkItemRefLineageTextContent(t, *work1Ref, "draft-v3")
}

func TestBuildFactoryWorldView_MarksActiveExecutionWorkItemRefsUnavailableWithoutLineageSnapshot(t *testing.T) {
	t0 := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	events := []factoryapi.FactoryEvent{
		initialStructureEvent(t0),
		generatedProjectionEvent(
			factoryapi.FactoryEventTypeDispatchRequest,
			"request/dispatch-missing",
			1,
			t0.Add(time.Second),
			factoryapi.FactoryEventContext{
				DispatchId: stringPtrForProjectionTest("dispatch-missing"),
				TraceIds:   stringSlicePtrForProjectionTest([]string{"trace-missing"}),
				WorkIds:    stringSlicePtrForProjectionTest([]string{"work-missing"}),
			},
			factoryapi.DispatchRequestEventPayload{
				TransitionId: "t-review",
				Inputs:       []factoryapi.DispatchConsumedWorkRef{{WorkId: "work-missing"}},
			},
		),
	}

	state, err := ReconstructFactoryWorldState(events, 1)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}
	state.WorkItemsByID["work-missing"] = interfaces.FactoryWorkItem{
		ID:         "work-missing",
		WorkTypeID: "task",
	}

	view := BuildFactoryWorldView(state)
	execution := view.Runtime.ActiveExecutionsByDispatchID["dispatch-missing"]
	if len(execution.WorkItems) != 1 {
		t.Fatalf("active execution work items = %#v, want one unavailable ref", execution.WorkItems)
	}
	ref := execution.WorkItems[0]
	if ref.PayloadStatus != string(interfaces.WorkPayloadResolutionUnavailable) {
		t.Fatalf("payload status = %q, want UNAVAILABLE", ref.PayloadStatus)
	}
	if ref.PayloadUnavailableReason == "" {
		t.Fatalf("payload unavailable reason = %q, want explicit reason", ref.PayloadUnavailableReason)
	}
	if len(ref.Content) != 0 {
		t.Fatalf("content = %#v, want no fabricated content parts", ref.Content)
	}
}

func assertWorkItemRefLineageTextContent(t *testing.T, ref interfaces.FactoryWorldWorkItemRef, wantText string) {
	t.Helper()
	if len(ref.Content) != 1 {
		t.Fatalf("content parts = %#v, want one text part", ref.Content)
	}
	if ref.Content[0].Type != interfaces.WorkContentPartTypeText || ref.Content[0].Text != wantText {
		t.Fatalf("content = %#v, want text %q", ref.Content, wantText)
	}
}
