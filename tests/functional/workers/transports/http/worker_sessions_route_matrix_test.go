package http_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestWorkerSessionRouteFunctionalKnownEmptyWork proves the known-Work empty
// contract through the root-built CLI and its matching public REST route. The
// paused runtime is the synchronization boundary that keeps the admitted Work
// from producing a Worker Session before either read.
func TestWorkerSessionRouteFunctionalKnownEmptyWork(t *testing.T) {
	dir := support.ScaffoldFactory(t, map[string]any{
		"name": "worker-sessions-route-empty",
		"workTypes": []any{map[string]any{
			"name": "manual",
			"states": []any{
				map[string]any{"name": "init", "type": "INITIAL"},
				map[string]any{"name": "complete", "type": "TERMINAL"},
			},
		}},
	})
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
	})
	t.Cleanup(func() { server.Stop(t) })

	name := "worker-sessions-route-empty-work"
	submitted := support.SubmitDefaultSessionWork(t, server.URL(), factoryapi.SubmitWorkRequest{
		Name:         &name,
		WorkTypeName: "manual",
		Payload:      map[string]string{"title": "known Work without a Worker Session"},
	})
	workID := support.StringPointerValue(submitted.WorkId)
	if workID == "" {
		t.Fatalf("submitted Work = %#v, want Work ID", submitted)
	}
	listedWork := support.ListDefaultSessionWork(t, server.URL())
	known := false
	for _, work := range listedWork.Results {
		if support.StringPointerValue(work.WorkId) == workID {
			known = true
			break
		}
	}
	if !known {
		t.Fatalf("Work list = %#v, want submitted Work %q", listedWork.Results, workID)
	}

	rest := support.ListDefaultSessionWorkerSessions(t, server.URL(), workID)
	if rest.Sessions == nil || len(rest.Sessions) != 0 {
		t.Fatalf("REST empty Work response = %#v, want non-nil empty sessions", rest)
	}

	humanInputs := support.FakeInputs(t.Context(), []string{
		"you", "worker-sessions", "list", "--work-id", workID, "--server", server.URL(),
	})
	if err := server.Execute(t, humanInputs.Input); err != nil {
		t.Fatalf("human empty Work list: %v\nstderr:\n%s", err, humanInputs.Stderr())
	}
	if got := humanInputs.Stdout(); got != "No worker sessions found.\n" {
		t.Fatalf("human empty Work output = %q, want exact empty message", got)
	}

	cli := executeDefaultWorkerSessionsListJSON(t, server, workID)
	if cli.Sessions == nil || len(cli.Sessions) != 0 {
		t.Fatalf("CLI empty Work response = %#v, want non-nil empty sessions", cli)
	}
	if got := support.CountFactoryEvents(support.GetFactoryEventsAt(t, server.URL()), factoryapi.FactoryEventTypeDispatchWorkerSessionAssociation); got != 0 {
		t.Fatalf("Worker Session associations before empty reads = %d, want zero", got)
	}
}

// TestWorkerSessionRouteFunctionalConcurrentReadsPreserveScope releases a
// synchronized pair of in-flight dispatches and four simultaneous CLI reads.
// Each result is checked against its independent public association event, so
// a successful response cannot hide a Work or session mix-up.
func TestWorkerSessionRouteFunctionalConcurrentReadsPreserveScope(t *testing.T) {
	dir := support.ScaffoldSingleStepFactory(t, "worker-sessions-route-concurrent")
	support.WriteAgentConfig(t, dir, "processor", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "test-model"))

	gate := make(chan struct{})
	runner := newFunctionalWorkerGate(gate)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		Edges:                     serviceedges.Edges{ProviderCommandRunner: runner},
	})
	t.Cleanup(func() { server.Stop(t) })

	stream := support.OpenFactoryEventStreamAt(t, support.DefaultSessionEventsURL(server.URL()))
	expected := submitConcurrentWorkerSessionDispatches(t, server, stream)
	runner.waitCallCount(t, 2)

	readConcurrentWorkerSessions(t, server, expected)

	close(gate)
	support.WaitForSessionTerminalStatus(t, server.URL(), factorysessions.DefaultSessionID, routeCharacterizationTimeout)
	assertConcurrentWorkerSessionDispatchesCompleted(t, server, runner, expected)
}

type concurrentWorkerSessionListResult struct {
	workID   string
	response factoryapi.ListWorkerSessionsResponse
	err      error
	stderr   string
}

func submitConcurrentWorkerSessionDispatches(
	t *testing.T,
	server *support.FunctionalAPIServer,
	stream *support.FactoryEventStream,
) map[string]routeCharacterizationDispatch {
	t.Helper()
	expected := make(map[string]routeCharacterizationDispatch, 2)
	for index := 0; index < 2; index++ {
		name := "worker-sessions-route-concurrent-work-" + string(rune('1'+index))
		submitted := support.SubmitDefaultSessionWork(t, server.URL(), factoryapi.SubmitWorkRequest{
			Name:         &name,
			WorkTypeName: "task",
			Payload:      map[string]string{"title": name},
		})
		workID := support.StringPointerValue(submitted.WorkId)
		if workID == "" {
			t.Fatalf("concurrent Work %d submission = %#v, want Work ID", index+1, submitted)
		}
		expected[workID] = waitForRouteCharacterizationAssociation(t, stream, workID)
	}
	return expected
}

func readConcurrentWorkerSessions(
	t *testing.T,
	server *support.FunctionalAPIServer,
	expected map[string]routeCharacterizationDispatch,
) {
	t.Helper()
	workIDs := make([]string, 0, len(expected))
	for _, dispatch := range expected {
		workIDs = append(workIDs, dispatch.workID)
	}
	reads := []string{workIDs[0], workIDs[1], workIDs[0], workIDs[1]}
	start := make(chan struct{})
	results := make(chan concurrentWorkerSessionListResult, len(reads))
	var waitGroup sync.WaitGroup
	for _, workID := range reads {
		workID := workID
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			<-start
			inputs := support.FakeInputs(context.Background(), []string{
				"you", "worker-sessions", "list", "--work-id", workID,
				"--server", server.URL(), "--output", "json",
			})
			err := server.Execute(t, inputs.Input)
			var response factoryapi.ListWorkerSessionsResponse
			if err == nil {
				err = json.Unmarshal([]byte(strings.TrimSpace(inputs.Stdout())), &response)
			}
			results <- concurrentWorkerSessionListResult{workID: workID, response: response, err: err, stderr: inputs.Stderr()}
		}()
	}
	close(start)
	waitGroup.Wait()
	close(results)

	for result := range results {
		if result.err != nil {
			t.Fatalf("concurrent JSON list for Work %q: %v\nstderr:\n%s", result.workID, result.err, result.stderr)
		}
		assertConcurrentWorkerSessionObservation(t, result.response, expected[result.workID])
	}
}

func assertConcurrentWorkerSessionDispatchesCompleted(
	t *testing.T,
	server *support.FunctionalAPIServer,
	runner *functionalWorkerGate,
	expected map[string]routeCharacterizationDispatch,
) {
	t.Helper()
	if runner.callCount() != 2 {
		t.Fatalf("provider calls after concurrent reads = %d, want exactly 2", runner.callCount())
	}
	for _, dispatch := range expected {
		response := support.ListDefaultSessionWorkerSessions(t, server.URL(), dispatch.workID)
		assertRouteCharacterizationObservation(t, "REST after concurrent reads", response, dispatch)
	}
}

// TestWorkerSessionRouteFunctionalBadInputAndUnknownWork exercises the two
// API-owned failure rows through the live root-built server and checks the
// same unknown-Work outcome through Process.Execute.
func TestWorkerSessionRouteFunctionalBadInputAndUnknownWork(t *testing.T) {
	dir := support.ScaffoldSingleStepFactory(t, "worker-sessions-route-failures")
	support.WriteAgentConfig(t, dir, "processor", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "test-model"))
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
	})
	t.Cleanup(func() { server.Stop(t) })

	baseEndpoint := strings.TrimSuffix(server.URL(), "/") + "/factory-sessions/" + factorysessions.DefaultSessionID + "/worker-sessions"
	for _, endpoint := range []string{baseEndpoint, baseEndpoint + "?workId="} {
		errorResponse := getWorkerSessionRouteError(t, endpoint, http.StatusBadRequest)
		if errorResponse.Code != factoryapi.ErrorResponseCodeBADREQUEST {
			t.Fatalf("blank Work endpoint %q error code = %q, want BAD_REQUEST", endpoint, errorResponse.Code)
		}
	}

	unknownWork := getWorkerSessionRouteError(t, baseEndpoint+"?workId=unknown-work", http.StatusNotFound)
	if unknownWork.Code != factoryapi.ErrorResponseCodeNOTFOUND {
		t.Fatalf("unknown Work REST error code = %q, want NOT_FOUND", unknownWork.Code)
	}
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "worker-sessions", "list", "--work-id", "unknown-work",
		"--server", server.URL(), "--output", "json",
	})
	if err := server.Execute(t, inputs.Input); err == nil {
		t.Fatal("unknown Work CLI error = nil, want non-success")
	}
	cliOutput := strings.TrimSpace(inputs.Stdout())
	if cliOutput == "" {
		cliOutput = strings.TrimSpace(inputs.Stderr())
	}
	var cliError struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal([]byte(cliOutput), &cliError); err != nil {
		t.Fatalf("decode unknown Work CLI error: %v; stdout=%s; stderr=%s", err, inputs.Stdout(), inputs.Stderr())
	}
	if cliError.Code != "WORK_NOT_FOUND" {
		t.Fatalf("unknown Work CLI error code = %q, want WORK_NOT_FOUND", cliError.Code)
	}
	if strings.Contains(cliOutput, `"sessions"`) {
		t.Fatalf("unknown Work CLI output contains a success collection: %s", cliOutput)
	}
}

func getWorkerSessionRouteError(t *testing.T, endpoint string, wantStatus int) factoryapi.ErrorResponse {
	t.Helper()
	response, err := http.Get(endpoint)
	if err != nil {
		t.Fatalf("GET %s: %v", endpoint, err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read GET %s response: %v", endpoint, err)
	}
	if response.StatusCode != wantStatus {
		t.Fatalf("GET %s status = %d, want %d; body=%s", endpoint, response.StatusCode, wantStatus, strings.TrimSpace(string(body)))
	}
	var errorResponse factoryapi.ErrorResponse
	if err := json.Unmarshal(body, &errorResponse); err != nil {
		t.Fatalf("decode GET %s error: %v; body=%s", endpoint, err, string(body))
	}
	return errorResponse
}

func assertConcurrentWorkerSessionObservation(
	t *testing.T,
	response factoryapi.ListWorkerSessionsResponse,
	expected routeCharacterizationDispatch,
) {
	t.Helper()
	if len(response.Sessions) != 1 {
		t.Fatalf("concurrent response for Work %q = %#v, want one observation", expected.workID, response.Sessions)
	}
	observation := response.Sessions[0]
	if observation.WorkerSessionId != expected.workerSessionID || observation.AttemptId != expected.dispatchID {
		t.Fatalf("concurrent Worker Session identity for Work %q = worker:%q attempt:%q, want worker:%q attempt:%q", expected.workID, observation.WorkerSessionId, observation.AttemptId, expected.workerSessionID, expected.dispatchID)
	}
	if observation.State != factoryapi.WorkerSessionObservationStateStarting && observation.State != factoryapi.WorkerSessionObservationStateRunning {
		t.Fatalf("concurrent Worker Session state for Work %q = %q, want STARTING or RUNNING", expected.workID, observation.State)
	}
	if observation.FactorySessionId == nil || *observation.FactorySessionId != factorysessions.DefaultSessionID {
		t.Fatalf("concurrent Factory Session identity for Work %q = %#v, want %q", expected.workID, observation.FactorySessionId, factorysessions.DefaultSessionID)
	}
	if observation.WorkId == nil || *observation.WorkId != expected.workID || len(observation.WorkIds) != 1 || observation.WorkIds[0] != expected.workID {
		t.Fatalf("concurrent Work identity for Work %q = workId:%v workIds:%v, want exact Work", expected.workID, observation.WorkId, observation.WorkIds)
	}
}
