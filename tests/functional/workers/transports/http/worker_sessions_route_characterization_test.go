package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const routeCharacterizationTimeout = 15 * time.Second

// TestWorkerSessionRouteCharacterization_AfterDefaultPauseResume records one
// association before the supported default-session pause/resume cycle and one
// after it. Both route reads continue to use the public ~default selector.
func TestWorkerSessionRouteCharacterization_AfterDefaultPauseResume(t *testing.T) {
	t.Parallel()
	dir := support.ScaffoldSingleStepFactory(t, "worker-sessions-route-resume-characterization")
	support.WriteAgentConfig(t, dir, "processor", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "test-model"))

	gate := make(chan struct{})
	close(gate)
	runner := newFunctionalWorkerGate(gate)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		Edges:                     serviceedges.Edges{ProviderCommandRunner: runner},
	})
	t.Cleanup(func() { server.Stop(t) })

	stream := support.OpenFactoryEventStreamAt(t, support.DefaultSessionEventsURL(server.URL()))
	preResume := submitAndObserveRouteCharacterizationWork(t, server, stream, "route-characterization-pre-resume")

	pause := postRouteCharacterizationLifecycleControl(t, server.URL(), "pause")
	if pause.Operation != factoryapi.FactorySessionLifecycleControlKindPause ||
		pause.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("pause response = %#v, want accepted pause", pause)
	}
	resume := postRouteCharacterizationLifecycleControl(t, server.URL(), "resume")
	if resume.Operation != factoryapi.FactorySessionLifecycleControlKindResume ||
		resume.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("resume response = %#v, want accepted resume", resume)
	}

	postResume := submitAndObserveRouteCharacterizationWork(t, server, stream, "route-characterization-post-resume")
	assertRouteCharacterizationRead(t, server, preResume, false)
	assertRouteCharacterizationRead(t, server, postResume, false)
	t.Logf(
		"public dispatch associations across default pause/resume: pre=%s/%s/%s post=%s/%s/%s",
		preResume.workID, preResume.dispatchID, preResume.workerSessionID,
		postResume.workID, postResume.dispatchID, postResume.workerSessionID,
	)

	if runner.callCount() != 2 {
		t.Fatalf("controlled provider calls across pause/resume = %d, want exactly 2", runner.callCount())
	}
}

type routeCharacterizationMultipleAttemptsFixture struct {
	server         *support.FunctionalAPIServer
	runner         *routeStepWorkerRunner
	releaseFirst   func()
	releaseCurrent func()
}

func newRouteCharacterizationMultipleAttemptsFixture(t *testing.T) routeCharacterizationMultipleAttemptsFixture {
	t.Helper()
	dir := support.ScaffoldFactory(t, map[string]any{
		"name": "worker-sessions-route-multiple-attempts",
		"workTypes": []any{map[string]any{
			"name": "task",
			"states": []any{
				map[string]any{"name": "init", "type": "INITIAL"},
				map[string]any{"name": "review", "type": "PROCESSING"},
				map[string]any{"name": "complete", "type": "TERMINAL"},
				map[string]any{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []any{
			map[string]any{"name": "processor"},
			map[string]any{"name": "reviewer"},
		},
		"workstations": []map[string]any{
			{
				"name":      "process",
				"worker":    "processor",
				"inputs":    []any{map[string]any{"workType": "task", "state": "init"}},
				"outputs":   []any{map[string]any{"workType": "task", "state": "review"}},
				"onFailure": []any{map[string]any{"workType": "task", "state": "failed"}},
			},
			{
				"name":      "review",
				"worker":    "reviewer",
				"inputs":    []any{map[string]any{"workType": "task", "state": "review"}},
				"outputs":   []any{map[string]any{"workType": "task", "state": "complete"}},
				"onFailure": []any{map[string]any{"workType": "task", "state": "failed"}},
			},
		},
	})
	support.WriteAgentConfig(t, dir, "processor", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "test-model"))
	support.WriteAgentConfig(t, dir, "reviewer", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "test-model"))

	firstGate := make(chan struct{})
	currentGate := make(chan struct{})
	openGate := make(chan struct{})
	close(openGate)
	var firstOnce, currentOnce sync.Once
	releaseFirst := func() { firstOnce.Do(func() { close(firstGate) }) }
	releaseCurrent := func() { currentOnce.Do(func() { close(currentGate) }) }
	runner := &routeStepWorkerRunner{
		gates:   []<-chan struct{}{firstGate, currentGate, openGate, openGate},
		started: make(chan int, 8),
	}
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		Edges:                     serviceedges.Edges{ProviderCommandRunner: runner},
	})
	t.Cleanup(func() { server.Stop(t) })
	t.Cleanup(func() {
		releaseFirst()
		releaseCurrent()
	})
	return routeCharacterizationMultipleAttemptsFixture{
		server: server, runner: runner, releaseFirst: releaseFirst, releaseCurrent: releaseCurrent,
	}
}

// TestWorkerSessionRouteCharacterization_MultipleAttemptsRemainScoped proves
// that HTTP and CLI return a Work's attempts without including a sibling Work.
func TestWorkerSessionRouteCharacterization_MultipleAttemptsRemainScoped(t *testing.T) {
	t.Parallel()
	fixture := newRouteCharacterizationMultipleAttemptsFixture(t)
	server := fixture.server
	runner := fixture.runner
	stream := support.OpenFactoryEventStreamAt(t, support.DefaultSessionEventsURL(server.URL()))

	targetName := "route-multiple-attempts-target"
	targetSubmission := support.SubmitDefaultSessionWork(t, server.URL(), factoryapi.SubmitWorkRequest{
		Name:         &targetName,
		WorkTypeName: "task",
		Payload:      map[string]string{"title": targetName},
	})
	targetWorkID := support.StringPointerValue(targetSubmission.WorkId)
	if targetWorkID == "" {
		t.Fatalf("target Work submission = %#v, want Work ID", targetSubmission)
	}
	runner.waitCallCount(t, 1)
	historical := waitForRouteCharacterizationAssociation(t, stream, targetWorkID)
	fixture.releaseFirst()
	runner.waitCallCount(t, 2)
	current := waitForRouteCharacterizationAssociation(t, stream, targetWorkID)

	siblingName := "route-multiple-attempts-sibling"
	siblingSubmission := support.SubmitDefaultSessionWork(t, server.URL(), factoryapi.SubmitWorkRequest{
		Name:         &siblingName,
		WorkTypeName: "task",
		Payload:      map[string]string{"title": siblingName},
	})
	siblingWorkID := support.StringPointerValue(siblingSubmission.WorkId)
	if siblingWorkID == "" {
		t.Fatalf("sibling Work submission = %#v, want Work ID", siblingSubmission)
	}
	runner.waitCallCount(t, 3)
	siblingAttempt := waitForRouteCharacterizationAssociation(t, stream, siblingWorkID)
	if siblingAttempt.workerSessionID == historical.workerSessionID || siblingAttempt.workerSessionID == current.workerSessionID {
		t.Fatalf("sibling Work %q reused target Worker Session identity %q", siblingWorkID, siblingAttempt.workerSessionID)
	}

	siblingList := support.ListDefaultSessionWorkerSessions(t, server.URL(), siblingWorkID)
	if len(siblingList.Sessions) == 0 {
		t.Fatalf("sibling Work %q list = %#v, want its own Worker Session", siblingWorkID, siblingList)
	}
	assertRouteCharacterizationAttemptList(t, server, targetWorkID, []routeCharacterizationAttemptExpectation{
		{dispatch: historical, state: factoryapi.WorkerSessionObservationStateCompleted},
		{dispatch: current, current: true},
	})

	fixture.releaseCurrent()
	support.WaitForSessionTerminalStatus(t, server.URL(), factorysessions.DefaultSessionID, routeCharacterizationTimeout)
	if runner.callCount() != 4 {
		t.Fatalf("controlled provider calls after both Works completed = %d, want four staged dispatches", runner.callCount())
	}
	assertRouteCharacterizationAttemptList(t, server, targetWorkID, []routeCharacterizationAttemptExpectation{
		{dispatch: historical, state: factoryapi.WorkerSessionObservationStateCompleted},
		{dispatch: current, state: factoryapi.WorkerSessionObservationStateCompleted},
	})
}

type routeCharacterizationDispatch struct {
	workID          string
	dispatchID      string
	workerSessionID string
}

type routeCharacterizationDispatchEvents struct {
	workIDs         map[string]bool
	workerSessionID string
}

func submitAndObserveRouteCharacterizationWork(
	t *testing.T,
	server *support.FunctionalAPIServer,
	stream *support.FactoryEventStream,
	name string,
) routeCharacterizationDispatch {
	t.Helper()
	submitted := support.SubmitDefaultSessionWork(t, server.URL(), factoryapi.SubmitWorkRequest{
		Name:         &name,
		WorkTypeName: "task",
		Payload:      map[string]string{"title": name},
	})
	workID := support.StringPointerValue(submitted.WorkId)
	if workID == "" {
		t.Fatalf("submission %q = %#v, want Work ID", name, submitted)
	}
	return waitForRouteCharacterizationDispatch(t, stream, workID)
}

func waitForRouteCharacterizationDispatch(
	t *testing.T,
	stream *support.FactoryEventStream,
	workID string,
) routeCharacterizationDispatch {
	return waitForRouteCharacterizationEvent(t, stream, workID, true)
}

func waitForRouteCharacterizationAssociation(
	t *testing.T,
	stream *support.FactoryEventStream,
	workID string,
) routeCharacterizationDispatch {
	return waitForRouteCharacterizationEvent(t, stream, workID, false)
}

func waitForRouteCharacterizationEvent(
	t *testing.T,
	stream *support.FactoryEventStream,
	workID string,
	waitForResponse bool,
) routeCharacterizationDispatch {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), routeCharacterizationTimeout)
	defer cancel()

	byDispatch := make(map[string]*routeCharacterizationDispatchEvents)
	for {
		event := stream.NextEventContext(ctx)
		if event.Context.DispatchId == nil || strings.TrimSpace(*event.Context.DispatchId) == "" {
			continue
		}
		dispatchID := strings.TrimSpace(*event.Context.DispatchId)
		entry := byDispatch[dispatchID]
		if entry == nil {
			entry = &routeCharacterizationDispatchEvents{workIDs: make(map[string]bool)}
			byDispatch[dispatchID] = entry
		}

		switch event.Type {
		case factoryapi.FactoryEventTypeDispatchRequest:
			if event.Context.WorkIds != nil {
				for _, candidate := range *event.Context.WorkIds {
					entry.workIDs[candidate] = true
				}
			}
			request, err := event.Payload.AsDispatchRequestEventPayload()
			if err != nil {
				t.Fatalf("decode public dispatch request for Work %q: %v", workID, err)
			}
			for _, input := range request.Inputs {
				entry.workIDs[input.WorkId] = true
			}
		case factoryapi.FactoryEventTypeDispatchWorkerSessionAssociation:
			association, err := event.Payload.AsDispatchWorkerSessionAssociationEventPayload()
			if err != nil {
				t.Fatalf("decode public Worker Session association for Work %q: %v", workID, err)
			}
			entry.workerSessionID = strings.TrimSpace(association.WorkerSessionId)
		case factoryapi.FactoryEventTypeDispatchResponse:
			if waitForResponse && entry.workIDs[workID] && entry.workerSessionID == "" {
				t.Fatalf("public dispatch %q completed for Work %q without a Worker Session association", dispatchID, workID)
			}
		}
		if entry.workIDs[workID] && entry.workerSessionID != "" && (!waitForResponse || event.Type == factoryapi.FactoryEventTypeDispatchResponse) {
			return routeCharacterizationDispatch{
				workID:          workID,
				dispatchID:      dispatchID,
				workerSessionID: entry.workerSessionID,
			}
		}
	}
}

func assertRouteCharacterizationRead(
	t *testing.T,
	server *support.FunctionalAPIServer,
	expected routeCharacterizationDispatch,
	includeHumanOutput bool,
) {
	t.Helper()
	if includeHumanOutput {
		inputs := support.FakeInputs(t.Context(), []string{
			"you", "worker-sessions", "list", "--work-id", expected.workID,
			"--server", server.URL(),
		})
		if err := server.Execute(t, inputs.Input); err != nil {
			t.Fatalf("human Worker Sessions list for Work %q: %v\nstderr:\n%s", expected.workID, err, inputs.Stderr())
		}
		if !strings.Contains(inputs.Stdout(), expected.workerSessionID) {
			t.Fatalf("human Worker Sessions list for Work %q = %q, want Worker Session %q", expected.workID, inputs.Stdout(), expected.workerSessionID)
		}
	}

	cliInputs := support.FakeInputs(t.Context(), []string{
		"you", "worker-sessions", "list", "--work-id", expected.workID,
		"--server", server.URL(), "--output", "json",
	})
	if err := server.Execute(t, cliInputs.Input); err != nil {
		t.Fatalf("JSON Worker Sessions list for Work %q: %v\nstderr:\n%s", expected.workID, err, cliInputs.Stderr())
	}
	var cliResponse factoryapi.ListWorkerSessionsResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(cliInputs.Stdout())), &cliResponse); err != nil {
		t.Fatalf("decode JSON Worker Sessions list for Work %q: %v\nstdout:\n%s", expected.workID, err, cliInputs.Stdout())
	}

	restResponse := support.ListDefaultSessionWorkerSessions(t, server.URL(), expected.workID)
	assertRouteCharacterizationObservation(t, "CLI", cliResponse, expected)
	assertRouteCharacterizationObservation(t, "REST", restResponse, expected)
	if !reflect.DeepEqual(cliResponse, restResponse) {
		t.Fatalf("CLI and REST Worker Session responses for Work %q differ:\nCLI: %#v\nREST: %#v", expected.workID, cliResponse, restResponse)
	}
}

type routeCharacterizationAttemptExpectation struct {
	dispatch routeCharacterizationDispatch
	state    factoryapi.WorkerSessionObservationState
	current  bool
}

func assertRouteCharacterizationAttemptList(
	t *testing.T,
	server *support.FunctionalAPIServer,
	workID string,
	expected []routeCharacterizationAttemptExpectation,
) {
	t.Helper()
	cliInputs := support.FakeInputs(t.Context(), []string{
		"you", "worker-sessions", "list", "--work-id", workID,
		"--server", server.URL(), "--output", "json",
	})
	if err := server.Execute(t, cliInputs.Input); err != nil {
		t.Fatalf("JSON Worker Sessions list for Work %q: %v\nstderr:\n%s", workID, err, cliInputs.Stderr())
	}
	var cliResponse factoryapi.ListWorkerSessionsResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(cliInputs.Stdout())), &cliResponse); err != nil {
		t.Fatalf("decode JSON Worker Sessions list for Work %q: %v\nstdout:\n%s", workID, err, cliInputs.Stdout())
	}
	restResponse := support.ListDefaultSessionWorkerSessions(t, server.URL(), workID)
	if len(cliResponse.Sessions) != len(restResponse.Sessions) {
		t.Fatalf("CLI and REST Worker Session counts for Work %q differ: CLI %d, REST %d", workID, len(cliResponse.Sessions), len(restResponse.Sessions))
	}
	for index := range cliResponse.Sessions {
		if cliResponse.Sessions[index].DurationBasis == "ACTIVE_CLOCK" || restResponse.Sessions[index].DurationBasis == "ACTIVE_CLOCK" {
			cliResponse.Sessions[index].DurationMillis = nil
			restResponse.Sessions[index].DurationMillis = nil
		}
	}
	if !reflect.DeepEqual(cliResponse, restResponse) {
		t.Fatalf("CLI and REST Worker Session responses for Work %q differ:\nCLI: %#v\nREST: %#v", workID, cliResponse, restResponse)
	}
	if len(restResponse.Sessions) != len(expected) {
		t.Fatalf("Worker Sessions for Work %q = %#v, want exactly %d attempts", workID, restResponse.Sessions, len(expected))
	}
	for index, want := range expected {
		got := restResponse.Sessions[index]
		if got.WorkerSessionId != want.dispatch.workerSessionID || got.AttemptId != want.dispatch.dispatchID {
			t.Fatalf("Work %q attempt %d identity = worker:%q attempt:%q, want worker:%q attempt:%q", workID, index, got.WorkerSessionId, got.AttemptId, want.dispatch.workerSessionID, want.dispatch.dispatchID)
		}
		if want.current {
			if got.State != factoryapi.WorkerSessionObservationStateStarting && got.State != factoryapi.WorkerSessionObservationStateRunning {
				t.Fatalf("current Work %q Worker Session state = %q, want STARTING or RUNNING", workID, got.State)
			}
		} else if got.State != want.state {
			t.Fatalf("Work %q Worker Session state = %q, want %q", workID, got.State, want.state)
		}
		if got.FactorySessionId == nil || *got.FactorySessionId != factorysessions.DefaultSessionID ||
			got.WorkId == nil || *got.WorkId != workID || !reflect.DeepEqual(got.WorkIds, []string{workID}) {
			t.Fatalf("Work %q Worker Session attribution = session:%v work:%v workIds:%v, want exact Factory Session and Work", workID, got.FactorySessionId, got.WorkId, got.WorkIds)
		}
	}
}

type routeStepWorkerRunner struct {
	gates   []<-chan struct{}
	started chan int
	mu      sync.Mutex
	calls   int
}

func (runner *routeStepWorkerRunner) Run(
	ctx context.Context,
	_ platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	runner.mu.Lock()
	call := runner.calls + 1
	runner.calls++
	runner.mu.Unlock()
	select {
	case runner.started <- call:
	default:
	}
	if call > len(runner.gates) {
		return platformprocess.CommandResult{}, fmt.Errorf("unexpected provider call %d", call)
	}
	select {
	case <-runner.gates[call-1]:
		return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout("staged Worker Session completed. COMPLETE")}, nil
	case <-ctx.Done():
		return platformprocess.CommandResult{}, ctx.Err()
	}
}

func (runner *routeStepWorkerRunner) waitCallCount(t *testing.T, want int) {
	t.Helper()
	timer := time.NewTimer(routeCharacterizationTimeout)
	defer timer.Stop()
	for runner.callCount() < want {
		select {
		case <-runner.started:
		case <-timer.C:
			t.Fatalf("staged provider calls = %d, want at least %d", runner.callCount(), want)
		}
	}
}

func (runner *routeStepWorkerRunner) callCount() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.calls
}

func assertRouteCharacterizationObservation(
	t *testing.T,
	transport string,
	response factoryapi.ListWorkerSessionsResponse,
	expected routeCharacterizationDispatch,
) {
	t.Helper()
	if len(response.Sessions) != 1 {
		t.Fatalf("%s Worker Sessions response for Work %q = %#v, want one observation", transport, expected.workID, response.Sessions)
	}
	observation := response.Sessions[0]
	if observation.WorkerSessionId != expected.workerSessionID ||
		observation.AttemptId != expected.dispatchID ||
		observation.State != factoryapi.WorkerSessionObservationStateCompleted {
		t.Fatalf("%s Worker Session identity for Work %q = %#v, want worker=%q attempt=%q state=COMPLETED", transport, expected.workID, observation, expected.workerSessionID, expected.dispatchID)
	}
	if observation.FactorySessionId == nil || *observation.FactorySessionId != factorysessions.DefaultSessionID {
		t.Fatalf("%s Factory Session identity for Work %q = %#v, want %q", transport, expected.workID, observation.FactorySessionId, factorysessions.DefaultSessionID)
	}
	if observation.WorkId == nil || *observation.WorkId != expected.workID ||
		!reflect.DeepEqual(observation.WorkIds, []string{expected.workID}) {
		t.Fatalf("%s Work identity for Worker Session %q = workId:%v workIds:%v, want %q", transport, observation.WorkerSessionId, observation.WorkId, observation.WorkIds, expected.workID)
	}
}

func postRouteCharacterizationLifecycleControl(
	t *testing.T,
	baseURL string,
	operation string,
) factoryapi.FactorySessionLifecycleControlResponse {
	t.Helper()
	payload, err := json.Marshal(factoryapi.FactorySessionLifecycleControlRequest{})
	if err != nil {
		t.Fatalf("marshal %s request: %v", operation, err)
	}
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + factorysessions.DefaultSessionID + "/" + operation
	request, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("build %s request: %v", operation, err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("POST %s: %v", endpoint, err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("POST %s status = %d: %s", endpoint, response.StatusCode, strings.TrimSpace(string(body)))
	}
	var result factoryapi.FactorySessionLifecycleControlResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatalf("decode %s response: %v", operation, err)
	}
	return result
}
