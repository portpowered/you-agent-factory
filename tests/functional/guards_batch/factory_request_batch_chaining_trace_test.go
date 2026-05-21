//go:build functionallong

package guards_batch

import (
	"context"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factory/projections"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestFactoryRequestBatch_ChainingTraceFanIn_EndToEndSmoke(t *testing.T) {
	support.SkipLongFunctional(t, "slow request-batch chaining-trace fan-in sweep")
	h, provider := newChainingTraceFanInHarness(t)
	submitChainingTraceFanInWork(t, h)
	assertChainingTraceFanInHarnessState(t, h, provider)
	events, err := h.GetFactoryEvents(context.Background())
	if err != nil {
		t.Fatalf("GetFactoryEvents: %v", err)
	}
	dispatchID, currentChainingTraceID := assertChainingTraceFanInEvents(t, events)
	assertChainingTraceFanInWorldState(t, events, dispatchID, currentChainingTraceID)
}

func TestFactoryRequestBatch_ChainingTraceThreeWorkstationFunctionalSmoke(t *testing.T) {
	support.SkipLongFunctional(t, "slow request-batch three-workstation chaining-trace sweep")

	h := newThreeWorkstationChainingTraceHarness(t)
	submitThreeWorkstationChainingTraceWork(t, h)
	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		PlaceTokenCount("merged:complete", 1).
		PlaceTokenCount("slot:available", 1).
		HasNoTokenInPlace("seed:init").
		HasNoTokenInPlace("prepared:ready").
		HasNoTokenInPlace("left:init").
		HasNoTokenInPlace("right:init")

	events, err := h.GetFactoryEvents(context.Background())
	if err != nil {
		t.Fatalf("GetFactoryEvents: %v", err)
	}

	splitDispatchID, splitParentWorkID := assertThreeWorkstationGeneratedBatchEvents(t, events)
	mergeDispatchID := assertThreeWorkstationMergeEvents(t, events, splitDispatchID)
	assertThreeWorkstationWorldStateAndResources(t, h, events, splitDispatchID, mergeDispatchID, splitParentWorkID)
}

func newChainingTraceFanInHarness(t *testing.T) (*testutil.ServiceTestHarness, *testutil.MockWorkerMapProvider) {
	t.Helper()

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "factory_request_batch"))
	support.UpdateFactoryConfig(t, dir, func(cfg map[string]any) {
		workTypes := cfg["workTypes"].([]any)
		cfg["workTypes"] = append(workTypes, chainingTraceFanInWorkTypes()...)
		workstations := cfg["workstations"].([]any)
		cfg["workstations"] = append(workstations, chainingTraceFanInWorkstation())
	})
	support.WriteWorkstationConfig(t, dir, "merge", "---\ntype: MODEL_WORKSTATION\n---\nMerge the completed work.\n")

	provider := testutil.NewMockWorkerMapProvider(map[string][]interfaces.InferenceResponse{
		"processor": {{Content: "merged lineage COMPLETE"}},
	})
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)
	return h, provider
}

func newThreeWorkstationChainingTraceHarness(t *testing.T) *testutil.ServiceTestHarness {
	t.Helper()

	dir := testutil.ScaffoldFactoryDir(t, threeWorkstationChainingTraceFactoryConfig())
	h := testutil.NewServiceTestHarness(t, dir)
	h.MockWorker("preparer", interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted})
	h.SetCustomExecutor("splitter", newGeneratedBatchExecutor(t, threeWorkstationChainingTraceGeneratedBatch()))
	h.MockWorker("merger", interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted})
	return h
}

func threeWorkstationChainingTraceFactoryConfig() *interfaces.FactoryConfig {
	return &interfaces.FactoryConfig{
		Name: "three-workstation-chaining-trace",
		Resources: []interfaces.ResourceConfig{{
			Name:     "slot",
			Capacity: 1,
		}},
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "seed",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
				},
			},
			{
				Name: "prepared",
				States: []interfaces.StateConfig{
					{Name: "ready", Type: interfaces.StateTypeInitial},
					{Name: "complete", Type: interfaces.StateTypeTerminal},
				},
			},
			{
				Name: "left",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
				},
			},
			{
				Name: "right",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
				},
			},
			{
				Name: "merged",
				States: []interfaces.StateConfig{
					{Name: "complete", Type: interfaces.StateTypeTerminal},
				},
			},
		},
		Workers: []interfaces.WorkerConfig{
			{Name: "preparer"},
			{Name: "splitter"},
			{Name: "merger"},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name:           "prepare",
				WorkerTypeName: "preparer",
				Inputs:         []interfaces.IOConfig{{WorkTypeName: "seed", StateName: "init"}},
				Outputs:        []interfaces.IOConfig{{WorkTypeName: "prepared", StateName: "ready"}},
			},
			{
				Name:           "split",
				WorkerTypeName: "splitter",
				Inputs:         []interfaces.IOConfig{{WorkTypeName: "prepared", StateName: "ready"}},
				Outputs:        []interfaces.IOConfig{{WorkTypeName: "prepared", StateName: "complete"}},
			},
			{
				Name:           "merge",
				WorkerTypeName: "merger",
				Inputs: []interfaces.IOConfig{
					{WorkTypeName: "left", StateName: "init"},
					{WorkTypeName: "right", StateName: "init"},
				},
				Outputs:   []interfaces.IOConfig{{WorkTypeName: "merged", StateName: "complete"}},
				Resources: []interfaces.ResourceConfig{{Name: "slot", Capacity: 1}},
			},
		},
	}
}

func threeWorkstationChainingTraceGeneratedBatch() interfaces.GeneratedSubmissionBatch {
	return interfaces.GeneratedSubmissionBatch{
		Request: interfaces.WorkRequest{
			RequestID: "request-generated-branches",
			Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
			Works: []interfaces.Work{
				{
					Name:                   "branch-z",
					WorkID:                 "work-branch-z",
					WorkTypeID:             "left",
					CurrentChainingTraceID: "chain-z",
					TraceID:                "chain-z",
					Payload:                "branch z payload",
				},
				{
					Name:                   "branch-a",
					WorkID:                 "work-branch-a",
					WorkTypeID:             "right",
					CurrentChainingTraceID: "chain-a",
					TraceID:                "chain-a",
					Payload:                "branch a payload",
				},
			},
		},
		Metadata: interfaces.GeneratedSubmissionBatchMetadata{
			Source: "generator:three-workstation",
		},
		Submissions: []interfaces.SubmitRequest{
			{Name: "branch-z", WorkID: "work-branch-z", Tags: map[string]string{"branch": "z"}},
			{Name: "branch-a", WorkID: "work-branch-a", Tags: map[string]string{"branch": "a"}},
		},
	}
}

func chainingTraceFanInWorkTypes() []any {
	return []any{
		map[string]any{
			"name": "left",
			"states": []any{
				map[string]any{"name": "init", "type": "INITIAL"},
			},
		},
		map[string]any{
			"name": "right",
			"states": []any{
				map[string]any{"name": "init", "type": "INITIAL"},
			},
		},
		map[string]any{
			"name": "merged",
			"states": []any{
				map[string]any{"name": "complete", "type": "TERMINAL"},
			},
		},
	}
}

func chainingTraceFanInWorkstation() map[string]any {
	return map[string]any{
		"name": "merge",
		"inputs": []any{
			map[string]any{"state": "init", "workType": "left"},
			map[string]any{"state": "init", "workType": "right"},
		},
		"outputs": []any{
			map[string]any{"state": "complete", "workType": "merged"},
		},
		"worker": "processor",
	}
}

func submitChainingTraceFanInWork(t *testing.T, h *testutil.ServiceTestHarness) {
	t.Helper()

	h.SubmitWorkRequest(context.Background(), interfaces.WorkRequest{
		RequestID: "request-chaining-fan-in-smoke",
		Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
		Works: []interfaces.Work{
			{
				Name:                   "lineage-z",
				WorkID:                 "work-lineage-z",
				WorkTypeID:             "left",
				CurrentChainingTraceID: "chain-z",
				TraceID:                "chain-z",
				Payload:                "lineage z",
			},
			{
				Name:                   "lineage-a",
				WorkID:                 "work-lineage-a",
				WorkTypeID:             "right",
				CurrentChainingTraceID: "chain-a",
				TraceID:                "chain-a",
				Payload:                "lineage a",
			},
		},
	})
	h.RunUntilComplete(t, 10*time.Second)
}

func submitThreeWorkstationChainingTraceWork(t *testing.T, h *testutil.ServiceTestHarness) {
	t.Helper()

	h.SubmitWorkRequest(context.Background(), interfaces.WorkRequest{
		RequestID: "request-three-workstation-lineage",
		Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
		Works: []interfaces.Work{{
			Name:                   "seed-root",
			WorkID:                 "work-seed-root",
			WorkTypeID:             "seed",
			CurrentChainingTraceID: "chain-root",
			TraceID:                "chain-root",
			Payload:                "seed payload",
		}},
	})
}

func assertChainingTraceFanInHarnessState(
	t *testing.T,
	h *testutil.ServiceTestHarness,
	provider *testutil.MockWorkerMapProvider,
) {
	t.Helper()

	h.Assert().
		PlaceTokenCount("merged:complete", 1).
		HasNoTokenInPlace("left:init").
		HasNoTokenInPlace("right:init")
	if provider.CallCount("processor") != 1 {
		t.Fatalf("processor call count = %d, want 1", provider.CallCount("processor"))
	}
}

func assertThreeWorkstationGeneratedBatchEvents(t *testing.T, events []factoryapi.FactoryEvent) (string, string) {
	t.Helper()

	var prepareRequest *factoryapi.DispatchRequestEventPayload
	var generatedPayload *factoryapi.WorkRequestEventPayload
	splitDispatchID := ""
	splitParentWorkID := ""

	for _, event := range events {
		switch event.Type {
		case factoryapi.FactoryEventTypeDispatchRequest:
			payload, err := event.Payload.AsDispatchRequestEventPayload()
			if err != nil {
				continue
			}
			switch payload.TransitionId {
			case "prepare":
				prepareRequest = &payload
			case "split":
				splitDispatchID = eventString(event.Context.DispatchId)
				workIDs := eventStringSlice(event.Context.WorkIds)
				if len(workIDs) != 1 {
					t.Fatalf("split dispatch request work IDs = %#v, want one prepared work item", workIDs)
				}
				splitParentWorkID = workIDs[0]
			}
		case factoryapi.FactoryEventTypeWorkRequest:
			payload, err := event.Payload.AsWorkRequestEventPayload()
			if err != nil || eventString(event.Context.RequestId) != "request-generated-branches" {
				continue
			}
			generatedPayload = &payload
		}
	}

	if prepareRequest == nil {
		t.Fatal("missing prepare dispatch request event")
	}
	if eventString(prepareRequest.CurrentChainingTraceId) != "chain-root" {
		t.Fatalf("prepare dispatch current chaining trace ID = %q, want chain-root", eventString(prepareRequest.CurrentChainingTraceId))
	}
	assertStringSliceEqual(t, "prepare dispatch previous chaining trace IDs", eventStringSlice(prepareRequest.PreviousChainingTraceIds), []string{"chain-root"})
	if splitDispatchID == "" {
		t.Fatal("missing split dispatch request event")
	}
	if splitParentWorkID == "" {
		t.Fatal("missing split parent work ID")
	}
	if generatedPayload == nil {
		t.Fatal("missing generated branch WORK_REQUEST event")
	}
	if generatedPayload.Type != factoryapi.WorkRequestTypeFactoryRequestBatch {
		t.Fatalf("generated request type = %q, want FACTORY_REQUEST_BATCH", generatedPayload.Type)
	}
	if eventString(generatedPayload.Source) != "generator:three-workstation" {
		t.Fatalf("generated request source = %q, want generator:three-workstation", eventString(generatedPayload.Source))
	}
	if eventWorkCount(generatedPayload.Works) != 2 {
		t.Fatalf("generated request work items = %d, want 2", eventWorkCount(generatedPayload.Works))
	}

	works := eventWorks(generatedPayload.Works)
	if len(works) != 2 {
		t.Fatalf("generated request work items = %#v, want two generated branches", works)
	}

	assertGeneratedBranchWork(t, works[0], "work-branch-z", "left", "chain-z", "z", splitDispatchID, splitParentWorkID)
	assertGeneratedBranchWork(t, works[1], "work-branch-a", "right", "chain-a", "a", splitDispatchID, splitParentWorkID)

	return splitDispatchID, splitParentWorkID
}

func assertGeneratedBranchWork(
	t *testing.T,
	work factoryapi.Work,
	wantWorkID string,
	wantWorkType string,
	wantTrace string,
	wantBranch string,
	wantDispatchID string,
	wantParentWorkID string,
) {
	t.Helper()

	if eventString(work.WorkId) != wantWorkID {
		t.Fatalf("generated work ID = %q, want %q", eventString(work.WorkId), wantWorkID)
	}
	if eventString(work.WorkTypeName) != wantWorkType {
		t.Fatalf("generated work type = %q, want %q", eventString(work.WorkTypeName), wantWorkType)
	}
	if eventString(work.CurrentChainingTraceId) != wantTrace {
		t.Fatalf("generated work %q current chaining trace ID = %q, want %q", wantWorkID, eventString(work.CurrentChainingTraceId), wantTrace)
	}
	if eventInt(work.ChainingTraceDepth) != 3 {
		t.Fatalf("generated work %q chaining trace depth = %d, want 3", wantWorkID, eventInt(work.ChainingTraceDepth))
	}
	assertStringSliceEqual(t, "generated work previous chaining trace IDs", eventStringSlice(work.PreviousChainingTraceIds), []string{"chain-root"})

	tags := eventTags(work.Tags)
	if tags["branch"] != wantBranch {
		t.Fatalf("generated work %q branch tag = %q, want %q", wantWorkID, tags["branch"], wantBranch)
	}
	if tags["_parent_request_id"] != "request-three-workstation-lineage" {
		t.Fatalf("generated work %q parent request tag = %q, want request-three-workstation-lineage", wantWorkID, tags["_parent_request_id"])
	}
	if tags["_parent_work_id"] != wantParentWorkID {
		t.Fatalf("generated work %q parent work tag = %q, want %q", wantWorkID, tags["_parent_work_id"], wantParentWorkID)
	}
	if tags["_source_dispatch_id"] != wantDispatchID {
		t.Fatalf("generated work %q source dispatch tag = %q, want %q", wantWorkID, tags["_source_dispatch_id"], wantDispatchID)
	}
	if tags["_source_transition_id"] != "split" {
		t.Fatalf("generated work %q source transition tag = %q, want split", wantWorkID, tags["_source_transition_id"])
	}
}

func assertThreeWorkstationMergeEvents(t *testing.T, events []factoryapi.FactoryEvent, splitDispatchID string) string {
	t.Helper()

	var requestPayload *factoryapi.DispatchRequestEventPayload
	var responsePayload *factoryapi.DispatchResponseEventPayload
	mergeDispatchID := ""

	for _, event := range events {
		switch event.Type {
		case factoryapi.FactoryEventTypeDispatchRequest:
			payload, err := event.Payload.AsDispatchRequestEventPayload()
			if err != nil || payload.TransitionId != "merge" {
				continue
			}
			mergeDispatchID = eventString(event.Context.DispatchId)
			requestPayload = &payload
		case factoryapi.FactoryEventTypeDispatchResponse:
			payload, err := event.Payload.AsDispatchResponseEventPayload()
			if err != nil || payload.TransitionId != "merge" {
				continue
			}
			responsePayload = &payload
		}
	}

	if requestPayload == nil {
		t.Fatal("missing merge dispatch request event")
	}
	if responsePayload == nil {
		t.Fatal("missing merge dispatch response event")
	}
	if mergeDispatchID == "" {
		t.Fatal("missing merge dispatch ID")
	}
	if len(requestPayload.Inputs) != 2 {
		t.Fatalf("merge dispatch request inputs = %#v, want two generated work inputs", requestPayload.Inputs)
	}
	if requestPayload.Resources == nil || len(*requestPayload.Resources) != 1 || (*requestPayload.Resources)[0].Name != "slot" {
		t.Fatalf("merge dispatch request resources = %#v, want slot", requestPayload.Resources)
	}
	if eventString(requestPayload.CurrentChainingTraceId) != "chain-z" {
		t.Fatalf("merge dispatch request current chaining trace ID = %q, want chain-z", eventString(requestPayload.CurrentChainingTraceId))
	}
	assertStringSliceEqual(t, "merge dispatch request previous chaining trace IDs", eventStringSlice(requestPayload.PreviousChainingTraceIds), []string{"chain-a", "chain-z"})

	if !dispatchCreatedIncludesWork(*requestPayload, "work-branch-z") || !dispatchCreatedIncludesWork(*requestPayload, "work-branch-a") {
		t.Fatalf("merge dispatch request inputs = %#v, want generated branch work IDs", requestPayload.Inputs)
	}

	if eventString(responsePayload.CurrentChainingTraceId) != "chain-z" {
		t.Fatalf("merge dispatch response current chaining trace ID = %q, want chain-z", eventString(responsePayload.CurrentChainingTraceId))
	}
	assertStringSliceEqual(t, "merge dispatch response previous chaining trace IDs", eventStringSlice(responsePayload.PreviousChainingTraceIds), []string{"chain-a", "chain-z"})
	if responsePayload.OutputWork == nil || len(*responsePayload.OutputWork) != 1 {
		t.Fatalf("merge dispatch response output work = %#v, want one merged output work item", responsePayload.OutputWork)
	}

	output := (*responsePayload.OutputWork)[0]
	if eventString(output.CurrentChainingTraceId) != "chain-z" {
		t.Fatalf("merge output current chaining trace ID = %q, want chain-z", eventString(output.CurrentChainingTraceId))
	}
	if eventInt(output.ChainingTraceDepth) != 4 {
		t.Fatalf("merge output chaining trace depth = %d, want 4", eventInt(output.ChainingTraceDepth))
	}
	assertStringSliceEqual(t, "merge output previous chaining trace IDs", eventStringSlice(output.PreviousChainingTraceIds), []string{"chain-a", "chain-z"})
	if eventString(output.WorkTypeName) != "merged" {
		t.Fatalf("merge output work type = %q, want merged", eventString(output.WorkTypeName))
	}
	if splitDispatchID == "" {
		t.Fatal("split dispatch ID should not be empty when asserting merge events")
	}

	return mergeDispatchID
}

func assertThreeWorkstationWorldStateAndResources(
	t *testing.T,
	h *testutil.ServiceTestHarness,
	events []factoryapi.FactoryEvent,
	splitDispatchID string,
	mergeDispatchID string,
	splitParentWorkID string,
) {
	t.Helper()

	worldState, err := projections.ReconstructFactoryWorldState(events, support.LastFactoryEventTick(events))
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}

	assertProjectedGeneratedBranch(t, worldState.WorkItemsByID["work-branch-z"], "work-branch-z", "left", "chain-z", splitDispatchID, splitParentWorkID, "z")
	assertProjectedGeneratedBranch(t, worldState.WorkItemsByID["work-branch-a"], "work-branch-a", "right", "chain-a", splitDispatchID, splitParentWorkID, "a")

	var completion *interfaces.FactoryWorldDispatchCompletion
	for i := range worldState.CompletedDispatches {
		if worldState.CompletedDispatches[i].DispatchID == mergeDispatchID {
			completion = &worldState.CompletedDispatches[i]
			break
		}
	}
	if completion == nil {
		t.Fatalf("world state missing merge completed dispatch %q", mergeDispatchID)
	}
	if completion.CurrentChainingTraceID != "chain-z" {
		t.Fatalf("world state merge completion current chaining trace ID = %q, want chain-z", completion.CurrentChainingTraceID)
	}
	assertStringSliceEqual(t, "world state merge completion previous chaining trace IDs", completion.PreviousChainingTraceIDs, []string{"chain-a", "chain-z"})
	if len(completion.ConsumedInputs) != 2 {
		t.Fatalf("world state merge consumed inputs = %#v, want two generated work inputs", completion.ConsumedInputs)
	}
	if len(completion.InputWorkItems) != 2 {
		t.Fatalf("world state merge input work items = %#v, want two generated work items", completion.InputWorkItems)
	}
	if len(completion.OutputWorkItems) != 1 {
		t.Fatalf("world state merge output work items = %#v, want one merged output", completion.OutputWorkItems)
	}

	merged := completion.OutputWorkItems[0]
	if merged.CurrentChainingTraceID != "chain-z" {
		t.Fatalf("world state merged output current chaining trace ID = %q, want chain-z", merged.CurrentChainingTraceID)
	}
	if merged.ChainingTraceDepth != 4 {
		t.Fatalf("world state merged output chaining trace depth = %d, want 4", merged.ChainingTraceDepth)
	}
	assertStringSliceEqual(t, "world state merged output previous chaining trace IDs", merged.PreviousChainingTraceIDs, []string{"chain-a", "chain-z"})

	projectedMerged, ok := worldState.WorkItemsByID[merged.ID]
	if !ok {
		t.Fatalf("world state missing projected merged work item %q", merged.ID)
	}
	if projectedMerged.ChainingTraceDepth != 4 {
		t.Fatalf("projected merged work chaining trace depth = %d, want 4", projectedMerged.ChainingTraceDepth)
	}
	assertStringSliceEqual(t, "projected merged work previous chaining trace IDs", projectedMerged.PreviousChainingTraceIDs, []string{"chain-a", "chain-z"})

	mergedTokens := h.Marking().TokensInPlace("merged:complete")
	if len(mergedTokens) != 1 {
		t.Fatalf("merged tokens in merged:complete = %d, want 1", len(mergedTokens))
	}
	if mergedTokens[0].Color.ChainingTraceDepth != 4 {
		t.Fatalf("merged token chaining trace depth = %d, want 4", mergedTokens[0].Color.ChainingTraceDepth)
	}
	if mergedTokens[0].Color.CurrentChainingTraceID != "chain-z" {
		t.Fatalf("merged token current chaining trace ID = %q, want chain-z", mergedTokens[0].Color.CurrentChainingTraceID)
	}
	assertStringSliceEqual(t, "merged token previous chaining trace IDs", mergedTokens[0].Color.PreviousChainingTraceIDs, []string{"chain-a", "chain-z"})

	resourceTokens := h.Marking().TokensInPlace("slot:available")
	if len(resourceTokens) != 1 {
		t.Fatalf("resource tokens in slot:available = %d, want 1", len(resourceTokens))
	}
	resource := resourceTokens[0]
	if resource.Color.TraceID != "" || resource.Color.CurrentChainingTraceID != "" || len(resource.Color.PreviousChainingTraceIDs) != 0 {
		t.Fatalf("released resource token lineage = %#v, want empty work chaining lineage", resource.Color)
	}
}

func assertProjectedGeneratedBranch(
	t *testing.T,
	item interfaces.FactoryWorkItem,
	wantWorkID string,
	wantWorkType string,
	wantTrace string,
	wantDispatchID string,
	wantParentWorkID string,
	wantBranch string,
) {
	t.Helper()

	if item.ID != wantWorkID {
		t.Fatalf("projected generated work ID = %q, want %q", item.ID, wantWorkID)
	}
	if item.WorkTypeID != wantWorkType {
		t.Fatalf("projected generated work type = %q, want %q", item.WorkTypeID, wantWorkType)
	}
	if item.CurrentChainingTraceID != wantTrace {
		t.Fatalf("projected generated work %q current chaining trace ID = %q, want %q", wantWorkID, item.CurrentChainingTraceID, wantTrace)
	}
	if item.ChainingTraceDepth != 3 {
		t.Fatalf("projected generated work %q chaining trace depth = %d, want 3", wantWorkID, item.ChainingTraceDepth)
	}
	assertStringSliceEqual(t, "projected generated work previous chaining trace IDs", item.PreviousChainingTraceIDs, []string{"chain-root"})
	if item.Tags["branch"] != wantBranch {
		t.Fatalf("projected generated work %q branch tag = %q, want %q", wantWorkID, item.Tags["branch"], wantBranch)
	}
	if item.Tags["_source_dispatch_id"] != wantDispatchID {
		t.Fatalf("projected generated work %q source dispatch tag = %q, want %q", wantWorkID, item.Tags["_source_dispatch_id"], wantDispatchID)
	}
	if item.Tags["_parent_work_id"] != wantParentWorkID {
		t.Fatalf("projected generated work %q parent work tag = %q, want %q", wantWorkID, item.Tags["_parent_work_id"], wantParentWorkID)
	}
}
