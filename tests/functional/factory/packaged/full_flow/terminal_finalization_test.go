package fullflow

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestPackagedFullFlowDispatchTerminalFinalization(t *testing.T) {
	fixture := newFullFlowSharedFixture(t)
	t.Run("PreservesAcceptedResultWithLaterError", func(t *testing.T) {
		t.Parallel()
		testPackagedFullFlowDispatchTerminalFinalizationPreservesAcceptedResult(t, fixture)
	})
	t.Run("RoutesNoResultFailureToLead", func(t *testing.T) {
		t.Parallel()
		testPackagedFullFlowDispatchTerminalFinalizationRoutesFailureToLead(t, fixture)
	})
	t.Run("SuppressesConcurrentDuplicateInvocation", func(t *testing.T) {
		t.Parallel()
		testPackagedFullFlowDispatchTerminalFinalizationSuppressesDuplicateInvocation(t, fixture)
	})
}

func testPackagedFullFlowDispatchTerminalFinalizationPreservesAcceptedResult(
	t *testing.T,
	fixture *fullFlowSharedFixture,
) {
	runner := &fullFlowRunner{
		singleTask:              true,
		acceptedResultWithError: true,
		pauseAcceptedResult:     true,
		acceptedResultStarted:   make(chan struct{}),
		acceptedResultRelease:   make(chan struct{}),
	}
	scenario := fixture.newScenario(t, runner)
	runner.repository = scenario.repository
	scenario.open(t)
	defer func() {
		select {
		case <-runner.acceptedResultRelease:
		default:
			close(runner.acceptedResultRelease)
		}
	}()

	result := startFullFlowInvocation(t, scenario, map[string]any{
		"request":    "Preserve an accepted result when the provider exits afterward",
		"baseBranch": "main", "maxCycles": "3", "maxTasksPerCycle": "1",
	})
	select {
	case <-runner.acceptedResultStarted:
	case <-time.After(fullFlowSharedFixtureTimeout):
		t.Fatal("timed out waiting for the controlled accepted-result provider call")
	}
	processing := waitForFullFlowSessionStatus(t, scenario, fullFlowSharedFixtureTimeout, func(status factoryapi.StatusResponse) bool {
		usage, ok := fullFlowResourceUsage(status, "full-flow-task-slots")
		return ok && usage.Available < usage.Total && status.Categories.Processing > 0
	})
	if usage, ok := fullFlowResourceUsage(processing, "full-flow-task-slots"); !ok || usage.Available != usage.Total-1 {
		t.Fatalf("in-flight task resource = %#v, want one held slot", usage)
	}
	close(runner.acceptedResultRelease)
	response := receiveFullFlowInvocation(t, result)
	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("response = %#v, want completed accepted invocation", response)
	}

	works := support.GetJSON[factoryapi.ListWorkResponse](t, support.SessionWorkURL(
		fixture.baseURL, scenario.sessionID, "/work?terminal=true&includeSuperseded=true",
	))
	assertFullFlowWorkStates(t, works.Results, "project", "complete", 1)
	tasks := assertFullFlowWorkStates(t, works.Results, "delivery-task", "merged", 1)
	for _, work := range works.Results {
		if work.WorkTypeName == nil || *work.WorkTypeName != "delivery-task" {
			continue
		}
		if work.SupersededBy != nil {
			t.Fatalf("work %q (type %q) has successor %q, want no second successor", work.Name, *work.WorkTypeName, *work.SupersededBy)
		}
	}

	events := support.GetFactoryEventsForSessionAt(t, fixture.baseURL, scenario.sessionID)
	dispatches := fullFlowDispatchesForTransition(t, events, "implement-task")
	if len(dispatches) != 1 || dispatches[0].Response == nil {
		t.Fatalf("implement dispatches = %#v, want one completed observation", dispatches)
	}
	if dispatches[0].Response.Outcome != factoryapi.WorkOutcomeAccepted {
		t.Fatalf("implement dispatch outcome = %q, want ACCEPTED", dispatches[0].Response.Outcome)
	}
	if dispatches[0].Response.Output == nil || !strings.Contains(*dispatches[0].Response.Output, "<COMPLETE>") {
		t.Fatalf("implement dispatch output = %#v, want canonical completion", dispatches[0].Response.Output)
	}
	agentRuns := fullFlowAgentRunResponsesForDispatch(t, events, dispatches[0].DispatchID)
	if len(agentRuns) != 1 || agentRuns[0].Outcome != factoryapi.WorkOutcomeAccepted {
		t.Fatalf("agent-run responses = %#v, want one accepted terminal response", agentRuns)
	}
	if agentRuns[0].Diagnostics == nil || agentRuns[0].Diagnostics.Provider == nil || agentRuns[0].Diagnostics.Provider.ResponseMetadata == nil {
		t.Fatalf("accepted agent-run diagnostics = %#v, want bounded provider metadata", agentRuns[0].Diagnostics)
	}
	metadata := *agentRuns[0].Diagnostics.Provider.ResponseMetadata
	if metadata["completion_evidence"] != "agent_message" || strings.TrimSpace(metadata["failure_stage"]) == "" {
		t.Fatalf("accepted agent-run response metadata = %#v, want completion evidence and bounded later-error stage", metadata)
	}
	if len(tasks) != 1 {
		t.Fatalf("merged tasks = %d, want one", len(tasks))
	}
	terminal := waitForFullFlowSessionStatus(t, scenario, fullFlowSharedFixtureTimeout, func(status factoryapi.StatusResponse) bool {
		usage, ok := fullFlowResourceUsage(status, "full-flow-task-slots")
		return ok && usage.Available == usage.Total
	})
	if usage, ok := fullFlowResourceUsage(terminal, "full-flow-task-slots"); !ok || usage.Available != usage.Total {
		t.Fatalf("terminal task resource = %#v, want fully released resource", usage)
	}
}

func testPackagedFullFlowDispatchTerminalFinalizationRoutesFailureToLead(
	t *testing.T,
	fixture *fullFlowSharedFixture,
) {
	runner := &fullFlowRunner{singleTask: true, failImplementation: true}
	scenario := fixture.newScenario(t, runner)
	runner.repository = scenario.repository
	scenario.open(t)
	response := invokeFullFlowSession(t, scenario, map[string]any{
		"request":    "Route a provider failure to the existing project failure lead",
		"baseBranch": "main", "maxCycles": "3", "maxTasksPerCycle": "1",
	})
	if response.Status != factoryapi.InvocationTerminalStatusFailed || response.WorkState == nil || !strings.HasSuffix(*response.WorkState, ":failed") {
		t.Fatalf("response = %#v, want failed project invocation", response)
	}

	works := support.GetJSON[factoryapi.ListWorkResponse](t, support.SessionWorkURL(
		fixture.baseURL, scenario.sessionID, "/work?terminal=true&includeSuperseded=true",
	))
	assertFullFlowWorkStates(t, works.Results, "project", "failed", 1)
	tasks := assertFullFlowWorkStates(t, works.Results, "delivery-task", "failed", 1)
	for _, work := range works.Results {
		if work.State != nil && work.State.Name == "complete" {
			t.Fatalf("work %q reached complete after provider failure", work.Name)
		}
	}

	events := support.GetFactoryEventsForSessionAt(t, fixture.baseURL, scenario.sessionID)
	dispatches := fullFlowDispatchesForTransition(t, events, "implement-task")
	if len(dispatches) != 1 || dispatches[0].Response == nil {
		t.Fatalf("implement failure dispatches = %#v, want one terminal failure", dispatches)
	}
	if dispatches[0].Response.Outcome != factoryapi.WorkOutcomeFailed || dispatches[0].Response.FailureDetail == nil {
		t.Fatalf("implement failure response = %#v, want typed FAILED outcome", dispatches[0].Response)
	}
	agentRuns := fullFlowAgentRunResponsesForDispatch(t, events, dispatches[0].DispatchID)
	if len(agentRuns) != 1 || agentRuns[0].Outcome != factoryapi.WorkOutcomeFailed {
		t.Fatalf("failure agent-run responses = %#v, want one FAILED response", agentRuns)
	}
	if len(tasks) != 1 || tasks[0].State == nil || tasks[0].State.Type != factoryapi.WorkStateTypeFAILED {
		t.Fatalf("failed tasks = %#v, want one typed failed Work", tasks)
	}
	workerSessions := support.ListSessionWorkerSessions(t, fixture.baseURL, scenario.sessionID, requiredFullFlowWorkID(t, tasks[0]))
	failedSessions := 0
	for _, session := range workerSessions.Sessions {
		if session.State == factoryapi.WorkerSessionObservationStateFailed && session.Failure != nil {
			failedSessions++
		}
	}
	if failedSessions != 1 {
		t.Fatalf("worker session observations = %#v, want one failed session with failure detail", workerSessions.Sessions)
	}
	if usage, ok := fullFlowResourceUsage(waitForFullFlowSessionStatus(t, scenario, fullFlowSharedFixtureTimeout, func(status factoryapi.StatusResponse) bool {
		usage, found := fullFlowResourceUsage(status, "full-flow-task-slots")
		return found && usage.Available == usage.Total
	}), "full-flow-task-slots"); !ok || usage.Available != usage.Total {
		t.Fatalf("failed task resource = %#v, want fully released resource", usage)
	}
}

func testPackagedFullFlowDispatchTerminalFinalizationSuppressesDuplicateInvocation(
	t *testing.T,
	fixture *fullFlowSharedFixture,
) {
	runner := &fullFlowRunner{
		singleTask:              true,
		acceptedResultWithError: true,
		pauseAcceptedResult:     true,
		acceptedResultStarted:   make(chan struct{}),
		acceptedResultRelease:   make(chan struct{}),
	}
	scenario := fixture.newScenario(t, runner)
	runner.repository = scenario.repository
	scenario.open(t)
	defer func() {
		select {
		case <-runner.acceptedResultRelease:
		default:
			close(runner.acceptedResultRelease)
		}
	}()

	args := map[string]any{
		"request":    "Deliver one task while the same invocation is delivered twice",
		"baseBranch": "main", "maxCycles": "3", "maxTasksPerCycle": "1",
	}
	requestID := fmt.Sprintf("full-flow-duplicate-%d", fixture.nextRequestID())
	start := make(chan struct{})
	results := make(chan fullFlowInvocationResult, 2)
	for range 2 {
		go func() {
			<-start
			response, err := postFullFlowInvocation(scenario, args, requestID)
			results <- fullFlowInvocationResult{response: response, err: err}
		}()
	}
	close(start)
	select {
	case <-runner.acceptedResultStarted:
	case <-time.After(fullFlowSharedFixtureTimeout):
		t.Fatal("timed out waiting for duplicate invocation to reach the provider")
	}
	close(runner.acceptedResultRelease)
	for range 2 {
		result := receiveFullFlowInvocationResult(t, results)
		if result.response.Status != factoryapi.InvocationTerminalStatusCompleted || result.response.RequestId != requestID {
			t.Fatalf("duplicate invocation result = %#v, want the same completed request", result.response)
		}
	}

	works := support.GetJSON[factoryapi.ListWorkResponse](t, support.SessionWorkURL(
		fixture.baseURL, scenario.sessionID, "/work?terminal=true&includeSuperseded=true",
	))
	assertFullFlowWorkStates(t, works.Results, "project", "complete", 1)
	assertFullFlowWorkStates(t, works.Results, "delivery-task", "merged", 1)
	for _, work := range works.Results {
		if work.WorkTypeName == nil || *work.WorkTypeName != "delivery-task" {
			continue
		}
		if work.SupersededBy != nil {
			t.Fatalf("duplicate delivery created successor for %q: %q", work.Name, *work.SupersededBy)
		}
	}
	events := support.GetFactoryEventsForSessionAt(t, fixture.baseURL, scenario.sessionID)
	dispatches := fullFlowDispatchesForTransition(t, events, "implement-task")
	if len(dispatches) != 1 || dispatches[0].Response == nil || dispatches[0].Response.Outcome != factoryapi.WorkOutcomeAccepted {
		t.Fatalf("duplicate implement dispatches = %#v, want one canonical accepted dispatch", dispatches)
	}
	if got := len(fullFlowAgentRunResponsesForDispatch(t, events, dispatches[0].DispatchID)); got != 1 {
		t.Fatalf("duplicate agent-run responses = %d, want one canonical response", got)
	}
}

type fullFlowInvocationResult struct {
	response factoryapi.InvocationResponse
	err      error
}

func startFullFlowInvocation(
	t *testing.T,
	scenario *fullFlowScenario,
	args map[string]any,
) <-chan fullFlowInvocationResult {
	t.Helper()
	requestID := fmt.Sprintf("full-flow-shared-%d", scenario.fixture.nextRequestID())
	result := make(chan fullFlowInvocationResult, 1)
	go func() {
		response, err := postFullFlowInvocation(scenario, args, requestID)
		result <- fullFlowInvocationResult{response: response, err: err}
	}()
	return result
}

func receiveFullFlowInvocation(t *testing.T, result <-chan fullFlowInvocationResult) factoryapi.InvocationResponse {
	t.Helper()
	return receiveFullFlowInvocationResult(t, result).response
}

func receiveFullFlowInvocationResult(t *testing.T, result <-chan fullFlowInvocationResult) fullFlowInvocationResult {
	t.Helper()
	select {
	case received := <-result:
		if received.err != nil {
			t.Fatalf("full-flow invocation: %v", received.err)
		}
		return received
	case <-time.After(fullFlowSharedFixtureTimeout):
		t.Fatal("timed out waiting for full-flow invocation")
		return fullFlowInvocationResult{}
	}
}

func postFullFlowInvocation(
	scenario *fullFlowScenario,
	args map[string]any,
	requestID string,
) (factoryapi.InvocationResponse, error) {
	payload, err := json.Marshal(factoryapi.InvocationRequest{RequestId: &requestID, Args: &args})
	if err != nil {
		return factoryapi.InvocationResponse{}, fmt.Errorf("marshal shared full-flow invocation: %w", err)
	}
	endpoint := strings.TrimSuffix(scenario.fixture.baseURL, "/") +
		"/factory-sessions/" + url.PathEscape(scenario.sessionID) + "/invocations"
	response, err := http.Post(endpoint, "application/json", bytes.NewReader(payload))
	if err != nil {
		return factoryapi.InvocationResponse{}, fmt.Errorf("POST shared full-flow invocation: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		return factoryapi.InvocationResponse{}, fmt.Errorf("POST shared full-flow invocation status = %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	var decoded factoryapi.InvocationResponse
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		return factoryapi.InvocationResponse{}, fmt.Errorf("decode shared full-flow invocation: %w", err)
	}
	return decoded, nil
}

func waitForFullFlowSessionStatus(
	t *testing.T,
	scenario *fullFlowScenario,
	timeout time.Duration,
	accept func(factoryapi.StatusResponse) bool,
) factoryapi.StatusResponse {
	t.Helper()
	endpoint := strings.TrimSuffix(scenario.fixture.baseURL, "/") +
		"/factory-sessions/" + url.PathEscape(scenario.sessionID) + "/status"
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	var latest factoryapi.StatusResponse
	for {
		latest = support.GetJSON[factoryapi.StatusResponse](t, endpoint)
		if accept(latest) {
			return latest
		}
		select {
		case <-deadline.C:
			t.Fatalf("timed out waiting for Factory Session status at %s: %#v", endpoint, latest)
		case <-ticker.C:
		}
	}
}

func fullFlowResourceUsage(status factoryapi.StatusResponse, name string) (factoryapi.ResourceUsage, bool) {
	if status.Resources == nil {
		return factoryapi.ResourceUsage{}, false
	}
	for _, usage := range *status.Resources {
		if usage.Name == name {
			return usage, true
		}
	}
	return factoryapi.ResourceUsage{}, false
}

func assertFullFlowWorkStates(
	t *testing.T,
	works []factoryapi.Work,
	workTypeName, stateName string,
	want int,
) []factoryapi.Work {
	t.Helper()
	matched := make([]factoryapi.Work, 0)
	for _, work := range works {
		if work.WorkTypeName == nil || *work.WorkTypeName != workTypeName {
			continue
		}
		matched = append(matched, work)
		if work.State == nil || work.State.Name != stateName {
			t.Fatalf("%s work %q state = %#v, want %q", workTypeName, work.Name, work.State, stateName)
		}
	}
	if len(matched) != want {
		t.Fatalf("%s works = %#v, want %d in state %q", workTypeName, matched, want, stateName)
	}
	return matched
}

func requiredFullFlowWorkID(t *testing.T, work factoryapi.Work) string {
	t.Helper()
	if work.WorkId == nil || strings.TrimSpace(*work.WorkId) == "" {
		t.Fatalf("work %q has no public work id", work.Name)
	}
	return *work.WorkId
}

func fullFlowDispatchesForTransition(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	transition string,
) []support.DispatchEventObservation {
	t.Helper()
	all := support.ObserveDispatchEvents(t, events)
	filtered := make([]support.DispatchEventObservation, 0)
	for _, dispatch := range all {
		if dispatch.Request.TransitionId == transition {
			filtered = append(filtered, dispatch)
		}
	}
	return filtered
}

func fullFlowAgentRunResponsesForDispatch(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	dispatchID string,
) []factoryapi.AgentRunResponseEventPayload {
	t.Helper()
	responses := make([]factoryapi.AgentRunResponseEventPayload, 0)
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeAgentRunResponse || event.Context.DispatchId == nil || *event.Context.DispatchId != dispatchID {
			continue
		}
		payload, err := event.Payload.AsAgentRunResponseEventPayload()
		if err != nil {
			t.Fatalf("decode agent-run response for dispatch %q: %v", dispatchID, err)
		}
		responses = append(responses, payload)
	}
	return responses
}
