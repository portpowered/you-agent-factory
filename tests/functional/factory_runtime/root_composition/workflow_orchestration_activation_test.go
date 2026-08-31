package root_composition_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	factoryRuntimeJavaScriptFactoryName   = "factory-runtime-js-workflow"
	factoryRuntimeJavaScriptSuccessResult = "factory-runtime-js:<SYNC_SUCCESS>"
)

// TestFactoryRuntimeJavaScriptWorkflowActivatesThroughRootBuildProcessAfterLifecycle
// proves an existing public JavaScript workflow surface produces a successful
// outcome after runtime lifecycle on a process composed only through
// root.BuildProcess with Factory Runtime effects replaced via edges.Edges.
func TestFactoryRuntimeJavaScriptWorkflowActivatesThroughRootBuildProcessAfterLifecycle(
	t *testing.T,
) {
	t.Parallel()

	enterSharedRootCompositionScenario(t)
	fixture := sharedRootCompositionFixtureForTest(t)
	workflowBefore := fixture.recorder.totalJavaScriptOrchestration()
	session, response := openSharedRootCompositionJavaScriptSession(t)
	if response.Result == nil || response.Result.ResultStatus != factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("sync result = %#v, want FINAL primary outcome", response.Result)
	}
	if response.Result.PrimaryResult == nil || len(*response.Result.PrimaryResult) != 1 {
		t.Fatalf("primary result = %#v, want one content part", response.Result.PrimaryResult)
	}
	part, err := (*response.Result.PrimaryResult)[0].AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("decode primary result: %v", err)
	}
	if part.Text != factoryRuntimeJavaScriptSuccessResult {
		t.Fatalf("primary result = %q, want %q", part.Text, factoryRuntimeJavaScriptSuccessResult)
	}
	if got := fixture.router.providerCallsFor(session.factoryDir); got != 0 {
		t.Fatalf("provider command runner call count = %d, want 0 for inline JavaScript workflow", got)
	}
	if got := fixture.recorder.totalJavaScriptOrchestration() - workflowBefore; got <= 0 {
		t.Fatalf("JavaScript workflow effect calls after sync execution = %d, want > 0 via edges", got)
	}
	session.close(t)
}

// TestFactoryRuntimePetriOrchestrationActivatesThroughRootBuildProcessAfterLifecycle
// proves an existing public orchestration surface routes multi-transition work
// to documented terminals after runtime lifecycle on the same public-process path.
// Complements the CLI-owned proofs in tests/functional/factory_runtime/orchestrators/petri/routing.
func TestFactoryRuntimePetriOrchestrationActivatesThroughRootBuildProcessAfterLifecycle(
	t *testing.T,
) {
	t.Parallel()

	enterSharedRootCompositionScenario(t)
	fixture := sharedRootCompositionFixtureForTest(t)
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "service_simple"))
	session := openSharedRootCompositionLiveSession(t, dir)
	dispatchBefore := fixture.recorder.totalDispatchPlan()
	traceID := "factory-runtime-orchestration-activation"
	submitted := support.SubmitSessionWorkAt(t, fixture.baseURL, session.sessionID, factoryapi.SubmitWorkRequest{
		Name:         stringPointer("factory-runtime-orchestration-activation"),
		WorkTypeName: "task",
		TraceId:      &traceID,
		Payload:      map[string]string{"title": "activate orchestration through public process"},
	})
	workID := stringPointerValue(submitted.WorkId)
	if workID == "" {
		t.Fatalf("submit work missing work id: %#v", submitted)
	}

	support.WaitForSessionTerminalStatus(t, fixture.baseURL, session.sessionID, 15*time.Second)
	if got := fixture.recorder.totalDispatchPlan() - dispatchBefore; got <= 0 {
		t.Fatalf("dispatch-plan effect calls after orchestration = %d, want > 0 via edges", got)
	}
	if got := fixture.router.providerCallsFor(session.factoryDir); got != 2 {
		t.Fatalf("provider command call count = %d, want 2 for two-stage routing", got)
	}

	terminal := support.WorkCustomerLocation("task", "complete")
	listed := sharedRootCompositionSessionWork(t, fixture, session.sessionID)
	if !support.HasWorkAtCustomerState(listed, workID, terminal) {
		t.Fatalf("work %q did not reach %s: %#v", workID, terminal, listed.Results)
	}
	if support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("task", "processing")) != 0 {
		t.Fatalf("processing Work still present after orchestration completion: %#v", listed.Results)
	}

	events := support.GetFactoryEventsForSessionAt(t, fixture.baseURL, session.sessionID)
	dispatches := support.ObserveDispatchEvents(t, events)
	foundCompleted := false
	for _, dispatch := range dispatches {
		if !support.DispatchObservationIncludesWork(dispatch, workID) || dispatch.Response == nil {
			continue
		}
		foundCompleted = true
		break
	}
	if !foundCompleted {
		t.Fatalf("public Factory Events missing completed dispatch for orchestrated work %q", workID)
	}
	session.close(t)
}

func postFactoryRuntimeJavaScriptSyncExecution(
	t *testing.T,
	baseURL string,
	args map[string]any,
) factoryapi.FactorySessionSyncExecutionResponse {
	t.Helper()

	factoryID := factoryRuntimeJavaScriptFactoryName
	payload, err := json.Marshal(factoryapi.FactorySessionExecutionRequest{
		RequestId: "factory-runtime-js-workflow-sync",
		Args:      &args,
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:      factoryapi.FactorySessionExecutionSourceKindFactoryId,
			FactoryId: &factoryID,
		},
	})
	if err != nil {
		t.Fatalf("marshal sync execution request: %v", err)
	}

	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/sync"
	response, err := http.Post(endpoint, "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("POST %s: %v", endpoint, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("POST %s status = %d, want 200: %s", endpoint, response.StatusCode, body)
	}

	var decoded factoryapi.FactorySessionSyncExecutionResponse
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode sync execution response: %v", err)
	}
	return decoded
}
