package guards_batch

import (
	"context"
	"errors"
	"testing"
	"time"

	factoryapi "github.com/portpowered/agent-factory/pkg/api/generated"
	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/testutil"
	"github.com/portpowered/agent-factory/tests/functional/internal/support"
)

// TestFileWatcherParentChildBatch_SubmittedFanInSmoke proves the documented
// watched-file BATCH input path accepts a canonical submitted PARENT_CHILD
// batch and drives the expected parent-aware failure route.
func TestFileWatcherParentChildBatch_SubmittedFanInSmoke(t *testing.T) {
	dir := seedSubmittedParentChildBatch(t)
	provider := newSubmittedParentChildProvider()

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 15*time.Second)
	assertSubmittedParentChildRuntimeOutcome(t, h, provider)
}

func seedSubmittedParentChildBatch(t *testing.T) string {
	t.Helper()

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "submitted_parent_child_filewatcher"))
	testutil.WriteSeedBatchFile(t, dir, interfaces.WorkRequest{
		RequestID: "release-story-set",
		Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
		Works: []interfaces.Work{
			{
				Name:       "story-set",
				WorkTypeID: "story-set",
				State:      "waiting",
				Payload: map[string]string{
					"title": "April release story set",
				},
				Tags: map[string]string{
					"project": "sample-service",
					"branch":  "ralph/april-release",
				},
			},
			{
				Name:       "story-auth",
				WorkTypeID: "story",
				Payload: map[string]string{
					"title": "Harden auth session handling",
				},
				Tags: map[string]string{
					"project": "sample-service",
					"branch":  "ralph/april-release",
				},
			},
			{
				Name:       "story-billing",
				WorkTypeID: "story",
				Payload: map[string]string{
					"title": "Polish billing retry UX",
				},
				Tags: map[string]string{
					"project": "sample-service",
					"branch":  "ralph/april-release",
				},
			},
		},
		Relations: []interfaces.WorkRelation{
			{
				Type:           interfaces.WorkRelationParentChild,
				SourceWorkName: "story-auth",
				TargetWorkName: "story-set",
			},
			{
				Type:           interfaces.WorkRelationParentChild,
				SourceWorkName: "story-billing",
				TargetWorkName: "story-set",
			},
		},
	})
	return dir
}

func newSubmittedParentChildProvider() *testutil.MockWorkerMapProvider {
	return testutil.NewMockWorkerMapProviderWithDefault(map[string][]testutil.WorkResponse{
		"story-worker": {
			{Content: "Story completed. COMPLETE"},
			{Error: errors.New("story processing failed")},
		},
		"story-set-failure-handler": {
			{Content: "Story set failed. COMPLETE"},
		},
	})
}

func assertSubmittedParentChildRuntimeOutcome(t *testing.T, h *testutil.ServiceTestHarness, provider *testutil.MockWorkerMapProvider) {
	t.Helper()

	h.Assert().
		PlaceTokenCount("story:complete", 1).
		PlaceTokenCount("story:failed", 1).
		PlaceTokenCount("story-set:failed", 1).
		HasNoTokenInPlace("story:init").
		HasNoTokenInPlace("story-set:waiting").
		HasNoTokenInPlace("story-set:complete")

	if provider.CallCount("story-worker") != 2 {
		t.Fatalf("story-worker calls = %d, want 2", provider.CallCount("story-worker"))
	}
	if provider.CallCount("story-set-failure-handler") != 1 {
		t.Fatalf("story-set-failure-handler calls = %d, want 1", provider.CallCount("story-set-failure-handler"))
	}

	events, err := h.GetFactoryEvents(context.Background())
	if err != nil {
		t.Fatalf("GetFactoryEvents: %v", err)
	}
	assertWatchedParentChildRequestRecorded(t, events)
	assertParentFailedOnlyAfterChildFailure(t, events)
}

func assertWatchedParentChildRequestRecorded(t *testing.T, events []factoryapi.FactoryEvent) {
	t.Helper()

	requestIndex := -1
	requestEvents := 0
	firstRelationIndex := -1
	parentChildRelations := map[string]bool{}

	for i, event := range events {
		switch event.Type {
		case factoryapi.FactoryEventTypeWorkRequest:
			if support.StringPointerValue(event.Context.RequestId) != "release-story-set" {
				continue
			}
			payload, err := event.Payload.AsWorkRequestEventPayload()
			if err != nil {
				t.Fatalf("decode WORK_REQUEST event %q: %v", event.Id, err)
			}
			requestIndex = i
			requestEvents++
			if payload.Type != factoryapi.WorkRequestTypeFactoryRequestBatch {
				t.Fatalf("request type = %q, want FACTORY_REQUEST_BATCH", payload.Type)
			}
			if support.StringPointerValue(payload.Source) != "external-submit" {
				t.Fatalf("request source = %q, want external-submit", support.StringPointerValue(payload.Source))
			}
			works := support.FactoryWorksValue(payload.Works)
			if len(works) != 3 {
				t.Fatalf("request work items = %d, want 3", len(works))
			}
			assertRequestIncludesParent(t, works)
		case factoryapi.FactoryEventTypeRelationshipChangeRequest:
			if support.StringPointerValue(event.Context.RequestId) != "release-story-set" {
				continue
			}
			payload, err := event.Payload.AsRelationshipChangeRequestEventPayload()
			if err != nil {
				t.Fatalf("decode RELATIONSHIP_CHANGE event %q: %v", event.Id, err)
			}
			if payload.Relation.Type != factoryapi.RelationTypeParentChild {
				t.Fatalf("relation type = %q, want PARENT_CHILD", payload.Relation.Type)
			}
			if payload.Relation.TargetWorkName != "story-set" {
				t.Fatalf("relation target = %q, want story-set", payload.Relation.TargetWorkName)
			}
			parentChildRelations[payload.Relation.SourceWorkName] = true
			if firstRelationIndex == -1 {
				firstRelationIndex = i
			}
		}
	}

	if requestEvents != 1 {
		t.Fatalf("WORK_REQUEST events for release-story-set = %d, want 1", requestEvents)
	}
	if !parentChildRelations["story-auth"] || !parentChildRelations["story-billing"] || len(parentChildRelations) != 2 {
		t.Fatalf("PARENT_CHILD relations = %#v, want story-auth and story-billing under story-set", parentChildRelations)
	}
	if firstRelationIndex <= requestIndex {
		t.Fatalf("WORK_REQUEST index %d should precede RELATIONSHIP_CHANGE index %d", requestIndex, firstRelationIndex)
	}
}

func assertRequestIncludesParent(t *testing.T, works []factoryapi.Work) {
	t.Helper()

	for _, work := range works {
		if work.Name != "story-set" {
			continue
		}
		if support.StringPointerValue(work.WorkTypeName) != "story-set" {
			t.Fatalf("story-set work_type_name = %q, want story-set", support.StringPointerValue(work.WorkTypeName))
		}
		return
	}

	t.Fatal("WORK_REQUEST missing story-set parent work item")
}

func assertParentFailedOnlyAfterChildFailure(t *testing.T, events []factoryapi.FactoryEvent) {
	t.Helper()

	childFailureIndex := -1
	childFailureDispatchID := ""
	parentFailureDispatchIndex := -1
	parentFailureCompletionIndex := -1

	for i, event := range events {
		switch event.Type {
		case factoryapi.FactoryEventTypeDispatchResponse:
			payload, err := event.Payload.AsDispatchResponseEventPayload()
			if err != nil {
				t.Fatalf("decode DISPATCH_COMPLETED event %q: %v", event.Id, err)
			}
			switch {
			case payload.TransitionId == "process-story" &&
				payload.Outcome == factoryapi.WorkOutcomeFailed &&
				support.StringPointerValue(event.Context.DispatchId) == childFailureDispatchID:
				childFailureIndex = i
			case payload.TransitionId == "fail-story-set-from-child" &&
				payload.Outcome == factoryapi.WorkOutcomeAccepted:
				parentFailureCompletionIndex = i
			}
		case factoryapi.FactoryEventTypeDispatchRequest:
			payload, err := event.Payload.AsDispatchRequestEventPayload()
			if err != nil {
				t.Fatalf("decode DISPATCH_CREATED event %q: %v", event.Id, err)
			}
			if payload.TransitionId == "process-story" &&
				support.DispatchInputsIncludeWorkNameFromHistory(t, events, event, payload, "story-billing") {
				childFailureDispatchID = support.StringPointerValue(event.Context.DispatchId)
			}
			if payload.TransitionId == "fail-story-set-from-child" &&
				support.DispatchInputsIncludeWorkNameFromHistory(t, events, event, payload, "story-set") {
				parentFailureDispatchIndex = i
			}
		}
	}

	if childFailureIndex == -1 {
		t.Fatal("missing failed child dispatch completion for story-billing")
	}
	if parentFailureDispatchIndex == -1 {
		t.Fatal("missing parent failure dispatch creation")
	}
	if parentFailureCompletionIndex == -1 {
		t.Fatal("missing parent failure dispatch completion")
	}
	if parentFailureDispatchIndex <= childFailureIndex {
		t.Fatalf("parent failure dispatch index %d should be after child failure index %d", parentFailureDispatchIndex, childFailureIndex)
	}
	if parentFailureCompletionIndex <= childFailureIndex {
		t.Fatalf("parent failure completion index %d should be after child failure index %d", parentFailureCompletionIndex, childFailureIndex)
	}
}
