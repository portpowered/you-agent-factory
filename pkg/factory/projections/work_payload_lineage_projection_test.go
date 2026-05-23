package projections

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testpath"
	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestReconstructFactoryWorldState_BuildsCanonicalWorkPayloadLineage(t *testing.T) {
	t0 := time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC)
	events := workPayloadLineageProjectionEvents(t, t0)

	state, err := ReconstructFactoryWorldState(events, 4)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}

	lineage := state.PayloadLineage
	if len(lineage.SnapshotsByID) != 4 {
		t.Fatalf("payload lineage snapshot count = %d, want 4", len(lineage.SnapshotsByID))
	}

	initial := lineage.ResolveInitialSubmittedSnapshot("work-1")
	if initial.Status != interfaces.WorkPayloadResolutionResolved || initial.Snapshot == nil {
		t.Fatalf("initial snapshot = %#v, want resolved work-request snapshot", initial)
	}
	if initial.Snapshot.SourceKind != interfaces.WorkPayloadSnapshotKindWorkRequest {
		t.Fatalf("initial source kind = %q, want WORK_REQUEST", initial.Snapshot.SourceKind)
	}
	assertLineageTextContent(t, initial.Snapshot.WorkItem, "draft-v1")

	consumed := lineage.ResolveConsumedInputSnapshot("dispatch-1", "work-1")
	if consumed.Status != interfaces.WorkPayloadResolutionResolved || consumed.Snapshot == nil {
		t.Fatalf("consumed snapshot = %#v, want resolved dispatch-time snapshot", consumed)
	}
	assertLineageTextContent(t, consumed.Snapshot.WorkItem, "draft-v1")

	selected := lineage.ResolveSelectedWorkSnapshot("work-1")
	if selected.Status != interfaces.WorkPayloadResolutionResolved || selected.Snapshot == nil {
		t.Fatalf("selected snapshot = %#v, want resolved latest snapshot", selected)
	}
	assertLineageTextContent(t, selected.Snapshot.WorkItem, "draft-v3")
	if selected.Snapshot.SourceKind != interfaces.WorkPayloadSnapshotKindWorkRequest {
		t.Fatalf("selected source kind = %q, want later WORK_REQUEST snapshot", selected.Snapshot.SourceKind)
	}

	sameWorkOutput := lineage.ResolveOutputWorkSnapshot("dispatch-1", "work-1")
	if sameWorkOutput.Status != interfaces.WorkPayloadResolutionResolved || sameWorkOutput.Snapshot == nil {
		t.Fatalf("same-work output snapshot = %#v, want resolved response output snapshot", sameWorkOutput)
	}
	assertLineageTextContent(t, sameWorkOutput.Snapshot.WorkItem, "draft-v2")
	if sameWorkOutput.Snapshot.Continuity != interfaces.WorkPayloadContinuitySameWorkID {
		t.Fatalf("same-work output continuity = %q, want SAME_WORK_ID_CONTINUATION", sameWorkOutput.Snapshot.Continuity)
	}
	if sameWorkOutput.Snapshot.LogicalWorkID != "work-1" {
		t.Fatalf("same-work logical work ID = %q, want work-1", sameWorkOutput.Snapshot.LogicalWorkID)
	}

	newDownstreamOutput := lineage.ResolveOutputWorkSnapshot("dispatch-1", "work-2")
	if newDownstreamOutput.Status != interfaces.WorkPayloadResolutionResolved || newDownstreamOutput.Snapshot == nil {
		t.Fatalf("new downstream output snapshot = %#v, want resolved response output snapshot", newDownstreamOutput)
	}
	assertLineageTextContent(t, newDownstreamOutput.Snapshot.WorkItem, "follow-up-v1")
	if newDownstreamOutput.Snapshot.Continuity != interfaces.WorkPayloadContinuityNewDownstreamWork {
		t.Fatalf("new downstream continuity = %q, want NEW_DOWNSTREAM_WORK", newDownstreamOutput.Snapshot.Continuity)
	}
	if len(newDownstreamOutput.Snapshot.ParentWorkIDs) != 1 || newDownstreamOutput.Snapshot.ParentWorkIDs[0] != "work-1" {
		t.Fatalf("new downstream parent work IDs = %#v, want [work-1]", newDownstreamOutput.Snapshot.ParentWorkIDs)
	}
	if newDownstreamOutput.Snapshot.LogicalWorkID != "work-2" {
		t.Fatalf("new downstream logical work ID = %q, want work-2", newDownstreamOutput.Snapshot.LogicalWorkID)
	}
}

func TestReconstructFactoryWorldState_ResolvesDownstreamOutputForLaterSelectionAndChainedConsumption(t *testing.T) {
	t0 := time.Date(2026, 5, 1, 11, 0, 0, 0, time.UTC)
	initial := interfaces.FactoryWorkItem{
		ID:          "work-root",
		WorkTypeID:  "task",
		DisplayName: "Root",
		TraceID:     "trace-root",
		PlaceID:     "task:init",
		Content: []interfaces.WorkContentPart{{
			Type: interfaces.WorkContentPartTypeText,
			Text: "root-v1",
		}},
	}
	downstream := interfaces.FactoryWorkItem{
		ID:          "work-child",
		WorkTypeID:  "task",
		DisplayName: "Child",
		TraceID:     "trace-child",
		PlaceID:     "task:review",
		Content: []interfaces.WorkContentPart{{
			Type: interfaces.WorkContentPartTypeText,
			Text: "child-v1",
		}},
	}
	laterSelected := downstream
	laterSelected.PlaceID = "task:done"
	laterSelected.Content = []interfaces.WorkContentPart{{
		Type: interfaces.WorkContentPartTypeText,
		Text: "child-v2",
	}}

	events := []factoryapi.FactoryEvent{
		initialStructureEvent(t0),
		generatedProjectionEvent(
			factoryapi.FactoryEventTypeWorkRequest,
			"work-input/root-v1",
			1,
			t0.Add(time.Second),
			factoryapi.FactoryEventContext{
				RequestId: stringPtrForProjectionTest("request/root-v1"),
				TraceIds:  stringSlicePtrForProjectionTest([]string{"trace-root"}),
				WorkIds:   stringSlicePtrForProjectionTest([]string{"work-root"}),
			},
			factoryapi.WorkRequestEventPayload{
				Type:  factoryapi.WorkRequestTypeFactoryRequestBatch,
				Works: &[]factoryapi.Work{generatedLineageWorkForProjectionTest(t, initial, "request/root-v1")},
			},
		),
		workstationRequestEvent(2, t0.Add(2*time.Second), interfaces.WorkstationRequestPayload{
			DispatchID:   "dispatch-create-child",
			TransitionID: "t-review",
			Workstation:  interfaces.FactoryWorkstationRef{ID: "t-review", Name: "Review"},
			Inputs: []interfaces.WorkstationInput{{
				TokenID:  "tok-root",
				PlaceID:  "task:init",
				WorkItem: &initial,
			}},
		}),
		generatedProjectionEvent(
			factoryapi.FactoryEventTypeDispatchResponse,
			"response/dispatch-create-child",
			3,
			t0.Add(3*time.Second),
			factoryapi.FactoryEventContext{
				DispatchId: stringPtrForProjectionTest("dispatch-create-child"),
				TraceIds:   stringSlicePtrForProjectionTest([]string{"trace-root", "trace-child"}),
				WorkIds:    stringSlicePtrForProjectionTest([]string{"work-root", "work-child"}),
			},
			factoryapi.DispatchResponseEventPayload{
				TransitionId: "t-review",
				Outcome:      factoryapi.WorkOutcomeAccepted,
				OutputWork: &[]factoryapi.Work{
					generatedLineageWorkForProjectionTest(t, downstream, ""),
				},
			},
		),
		workstationRequestEvent(4, t0.Add(4*time.Second), interfaces.WorkstationRequestPayload{
			DispatchID:   "dispatch-consume-child",
			TransitionID: "t-follow-up",
			Workstation:  interfaces.FactoryWorkstationRef{ID: "t-follow-up", Name: "Follow Up"},
			Inputs: []interfaces.WorkstationInput{{
				TokenID:  "tok-child",
				PlaceID:  "task:review",
				WorkItem: &downstream,
			}},
		}),
		generatedProjectionEvent(
			factoryapi.FactoryEventTypeWorkRequest,
			"work-input/child-v2",
			5,
			t0.Add(5*time.Second),
			factoryapi.FactoryEventContext{
				RequestId: stringPtrForProjectionTest("request/child-v2"),
				TraceIds:  stringSlicePtrForProjectionTest([]string{"trace-child"}),
				WorkIds:   stringSlicePtrForProjectionTest([]string{"work-child"}),
			},
			factoryapi.WorkRequestEventPayload{
				Type:  factoryapi.WorkRequestTypeFactoryRequestBatch,
				Works: &[]factoryapi.Work{generatedLineageWorkForProjectionTest(t, laterSelected, "request/child-v2")},
			},
		),
	}

	state, err := ReconstructFactoryWorldState(events, 5)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}

	selected := state.PayloadLineage.ResolveSelectedWorkSnapshot("work-child")
	if selected.Status != interfaces.WorkPayloadResolutionResolved || selected.Snapshot == nil {
		t.Fatalf("selected downstream snapshot = %#v, want resolved latest snapshot", selected)
	}
	assertLineageTextContent(t, selected.Snapshot.WorkItem, "child-v2")
	if selected.Snapshot.LogicalWorkID != "work-child" {
		t.Fatalf("selected downstream logical work ID = %q, want work-child", selected.Snapshot.LogicalWorkID)
	}

	consumed := state.PayloadLineage.ResolveConsumedInputSnapshot("dispatch-consume-child", "work-child")
	if consumed.Status != interfaces.WorkPayloadResolutionResolved || consumed.Snapshot == nil {
		t.Fatalf("consumed downstream snapshot = %#v, want resolved dispatch-time snapshot", consumed)
	}
	assertLineageTextContent(t, consumed.Snapshot.WorkItem, "child-v1")
	if consumed.Snapshot.SourceKind != interfaces.WorkPayloadSnapshotKindDispatchOutput {
		t.Fatalf("consumed downstream source kind = %q, want DISPATCH_RESPONSE_OUTPUT", consumed.Snapshot.SourceKind)
	}
	if consumed.Snapshot.Continuity != interfaces.WorkPayloadContinuityNewDownstreamWork {
		t.Fatalf("consumed downstream continuity = %q, want NEW_DOWNSTREAM_WORK", consumed.Snapshot.Continuity)
	}
	if len(consumed.Snapshot.ParentWorkIDs) != 1 || consumed.Snapshot.ParentWorkIDs[0] != "work-root" {
		t.Fatalf("consumed downstream parent work IDs = %#v, want [work-root]", consumed.Snapshot.ParentWorkIDs)
	}
}

func TestReconstructFactoryWorldState_WorkPayloadLineageMarksIncompleteConsumedHistoryUnavailable(t *testing.T) {
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

	consumed := state.PayloadLineage.ResolveConsumedInputSnapshot("dispatch-missing", "work-missing")
	if consumed.Status != interfaces.WorkPayloadResolutionUnavailable {
		t.Fatalf("consumed missing snapshot = %#v, want unavailable", consumed)
	}
	if consumed.Reason == "" {
		t.Fatalf("consumed missing reason = %q, want explicit unavailable reason", consumed.Reason)
	}
}

func TestReconstructFactoryWorldState_ReplayFixturePreservesConsumedAndChainedPayloadLineage(t *testing.T) {
	events := loadProjectionReplayFixtureEvents(t, "testdata", "work-payload-lineage-replay.jsonl")

	state, err := ReconstructFactoryWorldState(events, lastProjectionFixtureTick(events))
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}

	consumed := state.PayloadLineage.ResolveConsumedInputSnapshot("dispatch-follow-up", "work-child")
	if consumed.Status != interfaces.WorkPayloadResolutionResolved || consumed.Snapshot == nil {
		t.Fatalf("consumed child snapshot = %#v, want resolved dispatch-time snapshot", consumed)
	}
	assertLineageTextContent(t, consumed.Snapshot.WorkItem, "child-v1")
	if consumed.Snapshot.SourceKind != interfaces.WorkPayloadSnapshotKindDispatchOutput {
		t.Fatalf("consumed child source kind = %q, want DISPATCH_RESPONSE_OUTPUT", consumed.Snapshot.SourceKind)
	}

	selected := state.PayloadLineage.ResolveSelectedWorkSnapshot("work-child")
	if selected.Status != interfaces.WorkPayloadResolutionResolved || selected.Snapshot == nil {
		t.Fatalf("selected child snapshot = %#v, want resolved latest snapshot", selected)
	}
	assertLineageTextContent(t, selected.Snapshot.WorkItem, "child-v2")

	output := state.PayloadLineage.ResolveOutputWorkSnapshot("dispatch-review", "work-child")
	if output.Status != interfaces.WorkPayloadResolutionResolved || output.Snapshot == nil {
		t.Fatalf("output child snapshot = %#v, want resolved output snapshot", output)
	}
	if output.Snapshot.Continuity != interfaces.WorkPayloadContinuityNewDownstreamWork {
		t.Fatalf("output child continuity = %q, want NEW_DOWNSTREAM_WORK", output.Snapshot.Continuity)
	}
	if output.Snapshot.LogicalWorkID != "work-child" {
		t.Fatalf("output child logical work ID = %q, want work-child", output.Snapshot.LogicalWorkID)
	}
	if len(output.Snapshot.ParentWorkIDs) != 1 || output.Snapshot.ParentWorkIDs[0] != "work-root" {
		t.Fatalf("output child parent work IDs = %#v, want [work-root]", output.Snapshot.ParentWorkIDs)
	}

	missing := state.PayloadLineage.ResolveConsumedInputSnapshot("dispatch-missing", "work-missing")
	if missing.Status != interfaces.WorkPayloadResolutionUnavailable {
		t.Fatalf("missing consumed snapshot = %#v, want unavailable", missing)
	}
	if missing.Reason == "" {
		t.Fatalf("missing consumed reason = %q, want explicit unavailable reason", missing.Reason)
	}
}

func workPayloadLineageProjectionEvents(t *testing.T, t0 time.Time) []factoryapi.FactoryEvent {
	t.Helper()
	initial := interfaces.FactoryWorkItem{
		ID:          "work-1",
		WorkTypeID:  "task",
		DisplayName: "Draft",
		TraceID:     "trace-1",
		PlaceID:     "task:init",
		Content: []interfaces.WorkContentPart{
			{Type: interfaces.WorkContentPartTypeText, Text: "draft-v1"},
		},
	}
	continued := interfaces.FactoryWorkItem{
		ID:          "work-1",
		WorkTypeID:  "task",
		DisplayName: "Draft",
		TraceID:     "trace-1",
		PlaceID:     "task:complete",
		Content: []interfaces.WorkContentPart{
			{Type: interfaces.WorkContentPartTypeText, Text: "draft-v2"},
		},
	}
	downstream := interfaces.FactoryWorkItem{
		ID:          "work-2",
		WorkTypeID:  "task",
		DisplayName: "Follow up",
		TraceID:     "trace-2",
		PlaceID:     "task:complete",
		Content: []interfaces.WorkContentPart{
			{Type: interfaces.WorkContentPartTypeText, Text: "follow-up-v1"},
		},
	}
	laterSelected := interfaces.FactoryWorkItem{
		ID:          "work-1",
		WorkTypeID:  "task",
		DisplayName: "Draft",
		TraceID:     "trace-1",
		PlaceID:     "task:complete",
		Content: []interfaces.WorkContentPart{
			{Type: interfaces.WorkContentPartTypeText, Text: "draft-v3"},
		},
	}

	return []factoryapi.FactoryEvent{
		initialStructureEvent(t0),
		generatedProjectionEvent(
			factoryapi.FactoryEventTypeWorkRequest,
			"work-input/work-1-v1",
			1,
			t0.Add(time.Second),
			factoryapi.FactoryEventContext{
				RequestId: stringPtrForProjectionTest("request/work-1-v1"),
				TraceIds:  stringSlicePtrForProjectionTest([]string{"trace-1"}),
				WorkIds:   stringSlicePtrForProjectionTest([]string{"work-1"}),
			},
			factoryapi.WorkRequestEventPayload{
				Type:  factoryapi.WorkRequestTypeFactoryRequestBatch,
				Works: &[]factoryapi.Work{generatedLineageWorkForProjectionTest(t, initial, "request/work-1-v1")},
			},
		),
		workstationRequestEvent(2, t0.Add(2*time.Second), interfaces.WorkstationRequestPayload{
			DispatchID:   "dispatch-1",
			TransitionID: "t-review",
			Workstation:  interfaces.FactoryWorkstationRef{ID: "t-review", Name: "Review"},
			Inputs: []interfaces.WorkstationInput{{
				TokenID:  "tok-work-1",
				PlaceID:  "task:init",
				WorkItem: &initial,
			}},
		}),
		generatedProjectionEvent(
			factoryapi.FactoryEventTypeDispatchResponse,
			"response/dispatch-1",
			3,
			t0.Add(3*time.Second),
			factoryapi.FactoryEventContext{
				DispatchId: stringPtrForProjectionTest("dispatch-1"),
				TraceIds:   stringSlicePtrForProjectionTest([]string{"trace-1", "trace-2"}),
				WorkIds:    stringSlicePtrForProjectionTest([]string{"work-1", "work-2"}),
			},
			factoryapi.DispatchResponseEventPayload{
				TransitionId: "t-review",
				Outcome:      factoryapi.WorkOutcomeAccepted,
				OutputWork: &[]factoryapi.Work{
					generatedLineageWorkForProjectionTest(t, continued, ""),
					generatedLineageWorkForProjectionTest(t, downstream, ""),
				},
			},
		),
		generatedProjectionEvent(
			factoryapi.FactoryEventTypeWorkRequest,
			"work-input/work-1-v3",
			4,
			t0.Add(4*time.Second),
			factoryapi.FactoryEventContext{
				RequestId: stringPtrForProjectionTest("request/work-1-v3"),
				TraceIds:  stringSlicePtrForProjectionTest([]string{"trace-1"}),
				WorkIds:   stringSlicePtrForProjectionTest([]string{"work-1"}),
			},
			factoryapi.WorkRequestEventPayload{
				Type:  factoryapi.WorkRequestTypeFactoryRequestBatch,
				Works: &[]factoryapi.Work{generatedLineageWorkForProjectionTest(t, laterSelected, "request/work-1-v3")},
			},
		),
	}
}

func loadProjectionReplayFixtureEvents(t *testing.T, rel ...string) []factoryapi.FactoryEvent {
	t.Helper()

	path := testpath.MustRepoPathFromCaller(t, 0, append([]string{"pkg", "factory", "projections"}, rel...)...)
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open replay fixture %s: %v", path, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	events := make([]factoryapi.FactoryEvent, 0)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var event factoryapi.FactoryEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("parse replay fixture line %q: %v", line, err)
		}
		events = append(events, event)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan replay fixture %s: %v", path, err)
	}
	return events
}

func lastProjectionFixtureTick(events []factoryapi.FactoryEvent) int {
	tick := 0
	for _, event := range events {
		if event.Context.Tick > tick {
			tick = event.Context.Tick
		}
	}
	return tick
}

func generatedLineageWorkForProjectionTest(t *testing.T, item interfaces.FactoryWorkItem, requestID string) factoryapi.Work {
	t.Helper()
	work := generatedWorkForProjectionTest(item, requestID)
	if len(item.Content) == 0 {
		return work
	}
	parts := make([]factoryapi.WorkContentPart, 0, len(item.Content))
	for _, part := range item.Content {
		switch part.Type {
		case interfaces.WorkContentPartTypeText:
			parts = append(parts, workTextContentPartForProjectionTest(t, part.Text))
		case interfaces.WorkContentPartTypeImage:
			parts = append(parts, workImageContentPartForProjectionTest(t, part.File))
		default:
			t.Fatalf("unsupported test content part type %q", part.Type)
		}
	}
	work.Content = workContentPtrForProjectionTest(t, parts...)
	return work
}

func assertLineageTextContent(t *testing.T, item interfaces.FactoryWorkItem, want string) {
	t.Helper()
	if len(item.Content) != 1 || item.Content[0].Type != interfaces.WorkContentPartTypeText || item.Content[0].Text != want {
		t.Fatalf("work item content = %#v, want one text part %q", item.Content, want)
	}
}
