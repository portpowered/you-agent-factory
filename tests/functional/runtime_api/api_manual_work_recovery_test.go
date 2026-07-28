package runtime_api

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func assertManualWorkMoveRootRuntimeAndScopedStatusStayAligned(t *testing.T) {
	dir := support.ScaffoldFactory(t, map[string]any{
		"name": "manual-root-work-move",
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]string{{"name": "unused-worker"}},
	})
	server := startFunctionalServer(t, dir, true)

	submitted := server.SubmitRuntimeWork(t, work.SubmitRequest{
		Name: "manual move", WorkTypeID: "task", Payload: []byte(`{"title":"move me"}`),
	})
	if len(submitted) != 1 || submitted[0].WorkID == "" {
		t.Fatalf("submitted Work = %#v, want one public Work ID", submitted)
	}

	const moveRequestID = "manual-root-work-move-request"
	moved := postGeneratedMoveWorkWithRequestID(
		t,
		server.URL(),
		submitted[0].WorkID,
		"complete",
		moveRequestID,
	)
	if got := generatedWorkStateName(moved.State); got != "complete" {
		t.Fatalf("moved Work state = %q, want complete", got)
	}
	assertGeneratedMoveWorkConflict(
		t,
		server.URL(),
		submitted[0].WorkID,
		"complete",
		moveRequestID,
	)

	current := getGeneratedJSON[factoryapi.StatusResponse](t, server.URL()+"/status")
	scoped := getGeneratedJSON[factoryapi.StatusResponse](
		t,
		server.URL()+"/factory-sessions/~default/status",
	)
	if current.TotalTokens != 1 || current.Categories.Terminal != 1 {
		t.Fatalf("current root status = %#v, want one terminal Work", current)
	}
	if scoped.TotalTokens != current.TotalTokens || scoped.Categories != current.Categories {
		t.Fatalf("scoped status = %#v, want current root status %#v", scoped, current)
	}

	opened := postJSON[factoryapi.OpenFactorySessionResponse](
		t,
		server.URL()+"/factory-sessions",
		factoryapi.OpenFactorySessionRequest{FolderPath: dir},
		"open a second root-contract Factory Session",
	)
	if opened.Session == nil || opened.Session.Id == "" {
		t.Fatalf("opened Factory Session = %#v, want a public session ID", opened)
	}
	_ = getGeneratedJSON[factoryapi.StatusResponse](
		t,
		server.URL()+"/factory-sessions/"+opened.Session.Id+"/status",
	)
	closeFactorySession(t, server.URL(), opened.Session.Id)
	// Do not close ~default here; service-mode hosts tear down when the last live
	// runtime closes, so DELETE on the default session returns EOF before 204.
}

func closeFactorySession(t *testing.T, baseURL, sessionID string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodDelete, baseURL+"/factory-sessions/"+sessionID, nil)
	if err != nil {
		t.Fatalf("construct close session request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE default Factory Session: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		payload, _ := io.ReadAll(resp.Body)
		t.Fatalf("DELETE Factory Session %q status = %d, want 204: %s", sessionID, resp.StatusCode, payload)
	}
}

func TestManualWorkRecovery_CascadeFailureThenAPIMovesResumeProgress(t *testing.T) {
	t.Run("root runtime and scoped status stay aligned", assertManualWorkMoveRootRuntimeAndScopedStatusStayAligned)

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

	server := startFunctionalServerWithArgs(
		t,
		dir,
		false,
		nil,
		withProvider(provider),
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

	listed := support.ListDefaultSessionWork(t, server.URL())
	if !support.HasWorkAtCustomerState(listed, childWorkID, "task:complete") {
		t.Fatalf("work listing = %#v, want child at task:complete", listed.Results)
	}
	if !support.HasWorkAtCustomerState(listed, parentWorkID, "task:complete") {
		t.Fatalf("work listing = %#v, want parent at task:complete", listed.Results)
	}
}

func postGeneratedMoveWork(t *testing.T, baseURL, workID, stateName string) factoryapi.Work {
	t.Helper()
	return postGeneratedMoveWorkWithRequestID(t, baseURL, workID, stateName, "")
}

func postGeneratedMoveWorkWithRequestID(
	t *testing.T,
	baseURL string,
	workID string,
	stateName string,
	requestID string,
) factoryapi.Work {
	t.Helper()
	request := factoryapi.MoveWorkRequest{StateName: stateName}
	if requestID != "" {
		request.RequestId = &requestID
	}
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal move request: %v", err)
	}
	resp, err := http.Post(support.DefaultSessionWorkURL(baseURL, "/work/"+workID+"/move"), "application/json", bytes.NewReader(body))
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

func assertGeneratedMoveWorkConflict(
	t *testing.T,
	baseURL string,
	workID string,
	stateName string,
	requestID string,
) {
	t.Helper()
	body, err := json.Marshal(factoryapi.MoveWorkRequest{
		StateName: stateName,
		RequestId: &requestID,
	})
	if err != nil {
		t.Fatalf("marshal duplicate move request: %v", err)
	}
	resp, err := http.Post(
		support.DefaultSessionWorkURL(baseURL, "/work/"+workID+"/move"),
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("POST duplicate /work/%s/move: %v", workID, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		payload, _ := io.ReadAll(resp.Body)
		t.Fatalf("duplicate move status = %d, want 409: %s", resp.StatusCode, payload)
	}
}

func waitForGeneratedWorkIDsAtState(t *testing.T, baseURL string, workIDs []string, stateName string, timeout time.Duration) {
	t.Helper()

	want := make(map[string]bool, len(workIDs))
	for _, workID := range workIDs {
		want[workID] = true
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		work := getGeneratedJSON[factoryapi.ListWorkResponse](t, support.DefaultSessionWorkURL(baseURL, "/work"))
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
	work := getGeneratedJSON[factoryapi.ListWorkResponse](t, support.DefaultSessionWorkURL(baseURL, "/work"))
	t.Fatalf("timed out waiting for work IDs %v at state %q; last work response: %#v", workIDs, stateName, work)
}

func requireGeneratedWorkByID(t *testing.T, baseURL, workID string) factoryapi.Work {
	t.Helper()

	work := getGeneratedJSON[factoryapi.ListWorkResponse](t, support.DefaultSessionWorkURL(baseURL, "/work"))
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
