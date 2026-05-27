package projections_test

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testpath"
	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	. "github.com/portpowered/infinite-you/pkg/factory/projections"
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

	assertInitialSubmittedSnapshot(t, lineage.ResolveInitialSubmittedSnapshot("work-1"))
	assertConsumedCanonicalSnapshot(t, lineage.ResolveConsumedInputSnapshot("dispatch-1", "work-1"))
	assertSelectedCanonicalSnapshot(t, lineage.ResolveSelectedWorkSnapshot("work-1"))
	assertSameWorkOutputSnapshot(t, lineage.ResolveOutputWorkSnapshot("dispatch-1", "work-1"))
	assertNewDownstreamOutputSnapshot(t, lineage.ResolveOutputWorkSnapshot("dispatch-1", "work-2"))
}

func TestReconstructFactoryWorldState_ResolvesDownstreamOutputForLaterSelectionAndChainedConsumption(t *testing.T) {
	events := downstreamLineageProjectionEvents(t, time.Date(2026, 5, 1, 11, 0, 0, 0, time.UTC))

	state, err := ReconstructFactoryWorldState(events, 5)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}

	assertSelectedDownstreamSnapshot(t, state.PayloadLineage.ResolveSelectedWorkSnapshot("work-child"))
	assertConsumedDownstreamSnapshot(t, state.PayloadLineage.ResolveConsumedInputSnapshot("dispatch-consume-child", "work-child"))
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
	initial, continued, downstream, laterSelected := canonicalLineageProjectionWorkItems()

	return []factoryapi.FactoryEvent{
		initialStructureEvent(t0),
		lineageWorkRequestEvent(t, t0, 1, "work-input/work-1-v1", "request/work-1-v1", []string{"trace-1"}, []string{"work-1"}, initial),
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
		lineageDispatchResponseEvent(t, t0, 3, "dispatch-1", "response/dispatch-1", "t-review", []string{"trace-1", "trace-2"}, []string{"work-1", "work-2"}, continued, downstream),
		lineageWorkRequestEvent(t, t0, 4, "work-input/work-1-v3", "request/work-1-v3", []string{"trace-1"}, []string{"work-1"}, laterSelected),
	}
}

func canonicalLineageProjectionWorkItems() (interfaces.FactoryWorkItem, interfaces.FactoryWorkItem, interfaces.FactoryWorkItem, interfaces.FactoryWorkItem) {
	initial := projectionWorkItem("work-1", "Draft", "trace-1", "task:init", "draft-v1")
	continued := projectionWorkItem("work-1", "Draft", "trace-1", "task:complete", "draft-v2")
	downstream := projectionWorkItem("work-2", "Follow up", "trace-2", "task:complete", "follow-up-v1")
	laterSelected := projectionWorkItem("work-1", "Draft", "trace-1", "task:complete", "draft-v3")
	return initial, continued, downstream, laterSelected
}

func downstreamLineageProjectionEvents(t *testing.T, t0 time.Time) []factoryapi.FactoryEvent {
	t.Helper()

	initial := projectionWorkItem("work-root", "Root", "trace-root", "task:init", "root-v1")
	downstream := projectionWorkItem("work-child", "Child", "trace-child", "task:review", "child-v1")
	laterSelected := projectionWorkItem("work-child", "Child", "trace-child", "task:done", "child-v2")

	return []factoryapi.FactoryEvent{
		initialStructureEvent(t0),
		lineageWorkRequestEvent(t, t0, 1, "work-input/root-v1", "request/root-v1", []string{"trace-root"}, []string{"work-root"}, initial),
		lineageWorkstationRequestEvent(2, t0, "dispatch-create-child", "t-review", "Review", "tok-root", initial),
		lineageDispatchResponseEvent(t, t0, 3, "dispatch-create-child", "response/dispatch-create-child", "t-review", []string{"trace-root", "trace-child"}, []string{"work-root", "work-child"}, downstream),
		lineageWorkstationRequestEvent(4, t0, "dispatch-consume-child", "t-follow-up", "Follow Up", "tok-child", downstream),
		lineageWorkRequestEvent(t, t0, 5, "work-input/child-v2", "request/child-v2", []string{"trace-child"}, []string{"work-child"}, laterSelected),
	}
}

func projectionWorkItem(id, displayName, traceID, placeID, text string) interfaces.FactoryWorkItem {
	return interfaces.FactoryWorkItem{
		ID:          id,
		WorkTypeID:  "task",
		DisplayName: displayName,
		TraceID:     traceID,
		PlaceID:     placeID,
		Content: []interfaces.WorkContentPart{{
			Type: interfaces.WorkContentPartTypeText,
			Text: text,
		}},
	}
}

func lineageWorkRequestEvent(t *testing.T, t0 time.Time, tick int, id, requestID string, traceIDs, workIDs []string, item interfaces.FactoryWorkItem) factoryapi.FactoryEvent {
	t.Helper()

	return generatedProjectionEvent(
		factoryapi.FactoryEventTypeWorkRequest,
		id,
		tick,
		t0.Add(time.Duration(tick)*time.Second),
		factoryapi.FactoryEventContext{
			RequestId: stringPtrForProjectionTest(requestID),
			TraceIds:  stringSlicePtrForProjectionTest(traceIDs),
			WorkIds:   stringSlicePtrForProjectionTest(workIDs),
		},
		factoryapi.WorkRequestEventPayload{
			Type:  factoryapi.WorkRequestTypeFactoryRequestBatch,
			Works: &[]factoryapi.Work{generatedLineageWorkForProjectionTest(t, item, requestID)},
		},
	)
}

func lineageWorkstationRequestEvent(tick int, t0 time.Time, dispatchID, transitionID, workstationName, tokenID string, item interfaces.FactoryWorkItem) factoryapi.FactoryEvent {
	return workstationRequestEvent(tick, t0.Add(time.Duration(tick)*time.Second), interfaces.WorkstationRequestPayload{
		DispatchID:   dispatchID,
		TransitionID: transitionID,
		Workstation:  interfaces.FactoryWorkstationRef{ID: transitionID, Name: workstationName},
		Inputs: []interfaces.WorkstationInput{{
			TokenID:  tokenID,
			PlaceID:  item.PlaceID,
			WorkItem: &item,
		}},
	})
}

func lineageDispatchResponseEvent(t *testing.T, t0 time.Time, tick int, dispatchID, id, transitionID string, traceIDs, workIDs []string, items ...interfaces.FactoryWorkItem) factoryapi.FactoryEvent {
	t.Helper()

	outputs := make([]factoryapi.Work, 0, len(items))
	for _, item := range items {
		outputs = append(outputs, generatedLineageWorkForProjectionTest(t, item, ""))
	}

	return generatedProjectionEvent(
		factoryapi.FactoryEventTypeDispatchResponse,
		id,
		tick,
		t0.Add(time.Duration(tick)*time.Second),
		factoryapi.FactoryEventContext{
			DispatchId: stringPtrForProjectionTest(dispatchID),
			TraceIds:   stringSlicePtrForProjectionTest(traceIDs),
			WorkIds:    stringSlicePtrForProjectionTest(workIDs),
		},
		factoryapi.DispatchResponseEventPayload{
			TransitionId: transitionID,
			Outcome:      factoryapi.WorkOutcomeAccepted,
			OutputWork:   &outputs,
		},
	)
}

func assertSelectedDownstreamSnapshot(t *testing.T, selected interfaces.WorkPayloadResolution) {
	t.Helper()

	if selected.Status != interfaces.WorkPayloadResolutionResolved || selected.Snapshot == nil {
		t.Fatalf("selected downstream snapshot = %#v, want resolved latest snapshot", selected)
	}
	assertLineageTextContent(t, selected.Snapshot.WorkItem, "child-v2")
	if selected.Snapshot.LogicalWorkID != "work-child" {
		t.Fatalf("selected downstream logical work ID = %q, want work-child", selected.Snapshot.LogicalWorkID)
	}
}

func assertConsumedDownstreamSnapshot(t *testing.T, consumed interfaces.WorkPayloadResolution) {
	t.Helper()

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

func assertInitialSubmittedSnapshot(t *testing.T, initial interfaces.WorkPayloadResolution) {
	t.Helper()

	if initial.Status != interfaces.WorkPayloadResolutionResolved || initial.Snapshot == nil {
		t.Fatalf("initial snapshot = %#v, want resolved work-request snapshot", initial)
	}
	if initial.Snapshot.SourceKind != interfaces.WorkPayloadSnapshotKindWorkRequest {
		t.Fatalf("initial source kind = %q, want WORK_REQUEST", initial.Snapshot.SourceKind)
	}
	assertLineageTextContent(t, initial.Snapshot.WorkItem, "draft-v1")
}

func assertConsumedCanonicalSnapshot(t *testing.T, consumed interfaces.WorkPayloadResolution) {
	t.Helper()

	if consumed.Status != interfaces.WorkPayloadResolutionResolved || consumed.Snapshot == nil {
		t.Fatalf("consumed snapshot = %#v, want resolved dispatch-time snapshot", consumed)
	}
	assertLineageTextContent(t, consumed.Snapshot.WorkItem, "draft-v1")
}

func assertSelectedCanonicalSnapshot(t *testing.T, selected interfaces.WorkPayloadResolution) {
	t.Helper()

	if selected.Status != interfaces.WorkPayloadResolutionResolved || selected.Snapshot == nil {
		t.Fatalf("selected snapshot = %#v, want resolved latest snapshot", selected)
	}
	assertLineageTextContent(t, selected.Snapshot.WorkItem, "draft-v3")
	if selected.Snapshot.SourceKind != interfaces.WorkPayloadSnapshotKindWorkRequest {
		t.Fatalf("selected source kind = %q, want later WORK_REQUEST snapshot", selected.Snapshot.SourceKind)
	}
}

func assertSameWorkOutputSnapshot(t *testing.T, resolved interfaces.WorkPayloadResolution) {
	t.Helper()

	if resolved.Status != interfaces.WorkPayloadResolutionResolved || resolved.Snapshot == nil {
		t.Fatalf("same-work output snapshot = %#v, want resolved response output snapshot", resolved)
	}
	assertLineageTextContent(t, resolved.Snapshot.WorkItem, "draft-v2")
	if resolved.Snapshot.Continuity != interfaces.WorkPayloadContinuitySameWorkID {
		t.Fatalf("same-work output continuity = %q, want SAME_WORK_ID_CONTINUATION", resolved.Snapshot.Continuity)
	}
	if resolved.Snapshot.LogicalWorkID != "work-1" {
		t.Fatalf("same-work logical work ID = %q, want work-1", resolved.Snapshot.LogicalWorkID)
	}
}

func assertNewDownstreamOutputSnapshot(t *testing.T, resolved interfaces.WorkPayloadResolution) {
	t.Helper()

	if resolved.Status != interfaces.WorkPayloadResolutionResolved || resolved.Snapshot == nil {
		t.Fatalf("new downstream output snapshot = %#v, want resolved response output snapshot", resolved)
	}
	assertLineageTextContent(t, resolved.Snapshot.WorkItem, "follow-up-v1")
	if resolved.Snapshot.Continuity != interfaces.WorkPayloadContinuityNewDownstreamWork {
		t.Fatalf("new downstream continuity = %q, want NEW_DOWNSTREAM_WORK", resolved.Snapshot.Continuity)
	}
	if len(resolved.Snapshot.ParentWorkIDs) != 1 || resolved.Snapshot.ParentWorkIDs[0] != "work-1" {
		t.Fatalf("new downstream parent work IDs = %#v, want [work-1]", resolved.Snapshot.ParentWorkIDs)
	}
	if resolved.Snapshot.LogicalWorkID != "work-2" {
		t.Fatalf("new downstream logical work ID = %q, want work-2", resolved.Snapshot.LogicalWorkID)
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

func workContentPtrForProjectionTest(t *testing.T, parts ...factoryapi.WorkContentPart) *factoryapi.WorkContent {
	t.Helper()
	content := factoryapi.WorkContent(parts)
	return &content
}

func workTextContentPartForProjectionTest(t *testing.T, text string) factoryapi.WorkContentPart {
	t.Helper()
	var part factoryapi.WorkContentPart
	if err := part.FromWorkTextContentPart(factoryapi.WorkTextContentPart{
		Type: factoryapi.WorkContentPartTypeText,
		Text: text,
	}); err != nil {
		t.Fatalf("build text part: %v", err)
	}
	return part
}

func workImageContentPartForProjectionTest(t *testing.T, file string) factoryapi.WorkContentPart {
	t.Helper()
	var part factoryapi.WorkContentPart
	if err := part.FromWorkImageContentPart(factoryapi.WorkImageContentPart{
		Type: factoryapi.WorkContentPartTypeImage,
		File: file,
	}); err != nil {
		t.Fatalf("build image part: %v", err)
	}
	return part
}
