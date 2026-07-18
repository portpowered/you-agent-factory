package runtime_api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/wire"
	"github.com/portpowered/infinite-you/pkg/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/internal/functionalevidence"
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
		finishID     = "finish"
	)

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "cascading_failure"))
	runner := testutil.NewProviderCommandRunner(
		workers.CommandResult{Stdout: []byte("COMPLETE")},
		workers.CommandResult{Stderr: []byte("upstream service down"), ExitCode: 1},
		workers.CommandResult{Stdout: []byte("COMPLETE")},
		workers.CommandResult{Stdout: []byte("COMPLETE")},
		workers.CommandResult{Stdout: []byte("COMPLETE")},
	)
	host, err := support.StartRootRunFunctionalHost(context.Background(), support.RootRunFunctionalHostConfig{
		FactoryRoot:        dir,
		SystemRoot:         t.TempDir(),
		DisableMockWorkers: true,
		FunctionalEdges: wire.FunctionalEdges{
			ProviderCommandRunner: runner,
		},
	})
	if err != nil {
		t.Fatalf("StartRootRunFunctionalHost() error = %v", err)
	}
	t.Cleanup(func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, shutdownErr := host.Shutdown(shutdownCtx); shutdownErr != nil {
			t.Errorf("Shutdown() error = %v", shutdownErr)
		}
	})
	stream := openRootRunFactoryEventHTTPStream(t, host)
	requireFunctionalEventStreamPrelude(t, stream)

	requiredState := "complete"
	workTypeName := "task"
	putGeneratedWorkRequest(t, host.Endpoint(), requestID, factoryapi.WorkRequest{
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

	waitForInitialRecoveryFailure(t, stream, parentWorkID)
	assertGeneratedWorkStates(t, host.Endpoint(), map[string]string{
		parentWorkID: "failed",
		childWorkID:  "failed",
	})

	parentMoved := postGeneratedMoveWork(t, host.Endpoint(), parentWorkID, "processing")
	if generatedWorkStateName(parentMoved.State) != "processing" {
		t.Fatalf("parent move response = %#v, want processing", parentMoved)
	}

	childMoved := postGeneratedMoveWork(t, host.Endpoint(), childWorkID, "init")
	if generatedWorkStateName(childMoved.State) != "init" {
		t.Fatalf("child move response = %#v, want init", childMoved)
	}

	waitForManualRecoveryEvents(t, stream, parentWorkID, childWorkID, finishID)
	assertGeneratedWorkStates(t, host.Endpoint(), map[string]string{
		parentWorkID: "complete",
		childWorkID:  "complete",
	})
	functionalevidence.Covers(t, "rest/moveWorkBySessionId")
}

func postGeneratedMoveWork(t *testing.T, baseURL, workID, stateName string) factoryapi.Work {
	t.Helper()

	body, err := json.Marshal(factoryapi.MoveWorkRequest{StateName: stateName})
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

func assertGeneratedWorkStates(t *testing.T, baseURL string, want map[string]string) {
	t.Helper()

	work := getGeneratedJSON[factoryapi.ListWorkResponse](t, support.DefaultSessionWorkURL(baseURL, "/work"))
	found := make(map[string]string, len(want))
	for _, item := range work.Results {
		workID := stringPointerValue(item.WorkId)
		if _, expected := want[workID]; expected {
			found[workID] = generatedWorkStateName(item.State)
		}
	}
	for workID, state := range want {
		if found[workID] != state {
			t.Fatalf("GET /work state for %q = %q, want %q; response = %#v", workID, found[workID], state, work)
		}
	}
}

func waitForInitialRecoveryFailure(t *testing.T, stream *factoryEventHTTPStream, parentWorkID string) {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		event := stream.next(time.Until(deadline))
		if event.Type != factoryapi.FactoryEventTypeDispatchResponse || event.Context.WorkIds == nil {
			continue
		}
		payload, err := event.Payload.AsDispatchResponseEventPayload()
		if err != nil {
			t.Fatalf("decode DISPATCH_RESPONSE payload: %v", err)
		}
		for _, workID := range *event.Context.WorkIds {
			if workID == parentWorkID && payload.Outcome == factoryapi.WorkOutcomeFailed {
				if payload.Error == nil || *payload.Error == "" {
					t.Fatalf("failed recovery DISPATCH_RESPONSE error = %#v, want customer-readable failure", payload.Error)
				}
				return
			}
		}
	}
	t.Fatalf("canonical session stream missing failed DISPATCH_RESPONSE for %q", parentWorkID)
}

func waitForManualRecoveryEvents(t *testing.T, stream *factoryEventHTTPStream, parentWorkID, childWorkID, finishTransitionID string) {
	t.Helper()

	wantMoves := map[string]string{parentWorkID: "processing", childWorkID: "init"}
	wantCompletions := map[string]bool{parentWorkID: false, childWorkID: false}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		event := stream.next(time.Until(deadline))
		switch event.Type {
		case factoryapi.FactoryEventTypeWorkStateChange:
			payload, err := event.Payload.AsWorkStateChangeEventPayload()
			if err != nil {
				t.Fatalf("decode WORK_STATE_CHANGE payload: %v", err)
			}
			if payload.Source == factoryapi.WorkStateChangeSourceAPI && wantMoves[payload.WorkId] == payload.ToState {
				delete(wantMoves, payload.WorkId)
			}
		case factoryapi.FactoryEventTypeDispatchResponse:
			payload, err := event.Payload.AsDispatchResponseEventPayload()
			if err != nil {
				t.Fatalf("decode DISPATCH_RESPONSE payload: %v", err)
			}
			if payload.Outcome != factoryapi.WorkOutcomeAccepted || payload.TransitionId != finishTransitionID || event.Context.WorkIds == nil {
				continue
			}
			for _, workID := range *event.Context.WorkIds {
				if _, expected := wantCompletions[workID]; expected {
					wantCompletions[workID] = true
				}
			}
		}
		if len(wantMoves) == 0 && wantCompletions[parentWorkID] && wantCompletions[childWorkID] {
			return
		}
	}
	t.Fatalf("canonical session stream missing recovery evidence: moves=%v completions=%v", wantMoves, wantCompletions)
}
