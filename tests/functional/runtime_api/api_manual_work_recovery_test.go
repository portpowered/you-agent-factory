package runtime_api

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestManualWorkRecovery_CascadeFailureThenAPIMovesResumeProgress(t *testing.T) {
	if testing.Short() {
		t.Skip("slow manual work recovery functional test")
	}

	const (
		parentWorkID = "recovery-parent-work-id"
		childWorkID  = "recovery-child-work-id"
		traceID      = "trace-manual-work-recovery"
		requestID    = "request-manual-work-recovery"
	)

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "cascading_failure"))
	provider := testutil.NewMockWorkerMapProviderWithDefault(map[string][]testutil.WorkResponse{
		"starter": {
			{Content: "COMPLETE"},
			{Content: "COMPLETE"},
		},
		"finisher": {
			{Error: errors.New("upstream service down")},
			{Content: "COMPLETE"},
			{Content: "COMPLETE"},
		},
	})

	server := startFunctionalServerWithConfig(
		t,
		dir,
		false,
		func(cfg *service.FactoryServiceConfig) {
			cfg.ProviderOverride = provider
		},
		factory.WithServiceMode(),
	)

	requiredState := "complete"
	workTypeName := "task"
	putGeneratedWorkRequest(t, server.URL(), requestID, factoryapi.WorkRequest{
		RequestId: requestID,
		Type:      factoryapi.WorkRequestTypeFactoryRequestBatch,
		Works: &[]factoryapi.Work{
			{
				Name:         "parent",
				WorkId:       stringPointer(parentWorkID),
				WorkTypeName: &workTypeName,
				TraceId:      stringPointer(traceID),
				Payload:      map[string]string{"role": "parent"},
			},
			{
				Name:         "child",
				WorkId:       stringPointer(childWorkID),
				WorkTypeName: &workTypeName,
				TraceId:      stringPointer(traceID),
				Payload:      map[string]string{"role": "child"},
			},
		},
		Relations: &[]factoryapi.Relation{{
			Type:           factoryapi.RelationTypeDependsOn,
			SourceWorkName: "child",
			TargetWorkName: "parent",
			RequiredState:  &requiredState,
		}},
	})

	waitForGeneratedWorkIDsAtState(t, server.URL(), []string{parentWorkID, childWorkID}, "failed", 15*time.Second)

	parentMoved := postGeneratedMoveWork(t, server.URL(), parentWorkID, "processing")
	if generatedWorkStateName(parentMoved.State) != "processing" {
		t.Fatalf("parent move response = %#v, want processing", parentMoved)
	}

	childMoved := postGeneratedMoveWork(t, server.URL(), childWorkID, "init")
	if generatedWorkStateName(childMoved.State) != "init" {
		t.Fatalf("child move response = %#v, want init", childMoved)
	}

	assertManualRecoveryWorkStateChangeEvents(t, server, parentWorkID, childWorkID)

	completed := waitForGeneratedWorkIDsComplete(t, server.URL(), []string{childWorkID}, 15*time.Second)
	if len(completed) != 1 || generatedWorkStateName(completed[0].State) != "complete" {
		t.Fatalf("child completion = %#v, want complete", completed)
	}

	parent := requireGeneratedWorkByID(t, server.URL(), parentWorkID)
	if generatedWorkStateName(parent.State) != "complete" {
		t.Fatalf("parent after recovery = %#v, want complete", parent)
	}

	snapshot := server.GetEngineStateSnapshot(t)
	if !markingContainsWorkAtPlace(&snapshot.Marking, childWorkID, "task:complete") {
		t.Fatalf("marking = %#v, want child at task:complete", snapshot.Marking.Tokens)
	}
	if !markingContainsWorkAtPlace(&snapshot.Marking, parentWorkID, "task:complete") {
		t.Fatalf("marking = %#v, want parent at task:complete", snapshot.Marking.Tokens)
	}
}

func postGeneratedMoveWork(t *testing.T, baseURL, workID, stateName string) factoryapi.Work {
	t.Helper()

	body, err := json.Marshal(factoryapi.MoveWorkRequest{StateName: stateName})
	if err != nil {
		t.Fatalf("marshal move request: %v", err)
	}
	resp, err := http.Post(baseURL+"/work/"+workID+"/move", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /work/%s/move: %v", workID, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST /work/%s/move status = %d, want 200: %s", workID, resp.StatusCode, string(payload))
	}
	var work factoryapi.Work
	if err := json.NewDecoder(resp.Body).Decode(&work); err != nil {
		t.Fatalf("decode move response: %v", err)
	}
	return work
}

func waitForGeneratedWorkIDsAtState(t *testing.T, baseURL string, workIDs []string, stateName string, timeout time.Duration) {
	t.Helper()

	want := make(map[string]bool, len(workIDs))
	for _, workID := range workIDs {
		want[workID] = true
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		work := getGeneratedJSON[factoryapi.ListWorkResponse](t, baseURL+"/work")
		found := 0
		for _, item := range work.Results {
			workID := stringPointerValue(item.WorkId)
			if want[workID] && generatedWorkStateName(item.State) == stateName {
				found++
			}
		}
		if found == len(want) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	work := getGeneratedJSON[factoryapi.ListWorkResponse](t, baseURL+"/work")
	t.Fatalf("timed out waiting for work IDs %v at state %q; last work response: %#v", workIDs, stateName, work)
}

func requireGeneratedWorkByID(t *testing.T, baseURL, workID string) factoryapi.Work {
	t.Helper()

	work := getGeneratedJSON[factoryapi.ListWorkResponse](t, baseURL+"/work")
	for _, item := range work.Results {
		if stringPointerValue(item.WorkId) == workID {
			return item
		}
	}
	t.Fatalf("work ID %q missing from generated work response: %#v", workID, work)
	return factoryapi.Work{}
}

func assertManualRecoveryWorkStateChangeEvents(t *testing.T, server *functionalAPIServer, parentWorkID, childWorkID string) {
	t.Helper()

	events := server.GetFactoryEvents(t)
	parentSeen := false
	childSeen := false
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeWorkStateChange {
			continue
		}
		payload, err := event.Payload.AsWorkStateChangeEventPayload()
		if err != nil {
			t.Fatalf("decode WORK_STATE_CHANGE payload: %v", err)
		}
		if payload.Source != factoryapi.WorkStateChangeSourceAPI {
			continue
		}
		switch payload.WorkId {
		case parentWorkID:
			if payload.FromState == "failed" && payload.ToState == "processing" {
				parentSeen = true
			}
		case childWorkID:
			if payload.FromState == "failed" && payload.ToState == "init" {
				childSeen = true
			}
		}
	}
	if !parentSeen {
		t.Fatalf("events missing parent WORK_STATE_CHANGE failed->processing: %#v", events)
	}
	if !childSeen {
		t.Fatalf("events missing child WORK_STATE_CHANGE failed->init: %#v", events)
	}
}

func markingContainsWorkAtPlace(marking *petri.MarkingSnapshot, workID, placeID string) bool {
	for _, token := range marking.TokensInPlace(placeID) {
		if token.Color.WorkID == workID {
			return true
		}
	}
	return false
}
