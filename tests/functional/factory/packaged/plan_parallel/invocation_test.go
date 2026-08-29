package plan_parallel

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const parallelDAG = `{"request":{"type":"FACTORY_REQUEST_BATCH","works":[{"name":"task-a","workTypeName":"planned-task","payload":"TASK_A_UNIQUE_OBJECTIVE"},{"name":"task-b","workTypeName":"planned-task","payload":"TASK_B_UNIQUE_OBJECTIVE"},{"name":"task-c","workTypeName":"planned-task","payload":"TASK_C_UNIQUE_OBJECTIVE"}],"relations":[{"type":"DEPENDS_ON","sourceWorkName":"task-c","targetWorkName":"task-a"},{"type":"DEPENDS_ON","sourceWorkName":"task-c","targetWorkName":"task-b"}]}}`

const planParallelFanInChildCount = 10

func TestPackagedPlanParallel(t *testing.T) {
	fixture := newPlanParallelSharedFixture(t)
	t.Run("TestPackagedPlanParallelMergerReceivesEveryUniqueCompletedChildResult", func(t *testing.T) {
		testPackagedPlanParallelMergerReceivesEveryUniqueCompletedChildResult(t, fixture)
	})
	t.Run("TestPackagedPlanParallelExecutesReadyDAGConcurrentlyAndMerges", func(t *testing.T) {
		testPackagedPlanParallelExecutesReadyDAGConcurrentlyAndMerges(t, fixture)
	})
	t.Run("TestPackagedPlanParallelExecutorEffortCanBeOverridden", func(t *testing.T) {
		testPackagedPlanParallelExecutorEffortCanBeOverridden(t, fixture)
	})
	t.Run("TestPackagedPlanParallelRejectsUnsupportedEffortBeforeExecutorProviderExecution", func(t *testing.T) {
		testPackagedPlanParallelRejectsUnsupportedEffortBeforeExecutorProviderExecution(t, fixture)
	})
	t.Run("TestPackagedPlanParallelRejectsPlannerBatchAboveCeilingAtomically", func(t *testing.T) {
		testPackagedPlanParallelRejectsPlannerBatchAboveCeilingAtomically(t, fixture)
	})
	t.Run("TestPackagedPlanParallelChildFailureFansInWithoutMerge", func(t *testing.T) {
		testPackagedPlanParallelChildFailureFansInWithoutMerge(t, fixture)
	})
	t.Run("reuse after child failure", func(t *testing.T) {
		testPackagedPlanParallelReusesProcessAfterChildFailure(t, fixture)
	})
}

func testPackagedPlanParallelMergerReceivesEveryUniqueCompletedChildResult(
	t *testing.T,
	fixture *planParallelSharedFixture,
) {
	const originalRequest = "FANIN_ORIGINAL_REQUEST_MARKER"
	runner := newPlanParallelFanInRunner(planParallelFanInDAG())
	scenario := fixture.newScenario(t, runner)
	scenario.open(t)
	response, err := runPlanParallelInvocation(t, scenario, map[string]any{"request": originalRequest})
	if err != nil {
		t.Fatalf("plan-parallel invocation: %v", err)
	}
	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("response = %#v, want completed fan-in invocation", response)
	}
	if runner.executionCount() != planParallelFanInChildCount || runner.mergeCount() != 1 {
		t.Fatalf("executor calls = %d, merge calls = %d; want %d and 1", runner.executionCount(), runner.mergeCount(), planParallelFanInChildCount)
	}

	mergerPrompt := planParallelMergerPrompt(t, runner.requestsSnapshot())
	if strings.Count(mergerPrompt, originalRequest) != 1 {
		t.Fatalf("merger prompt contains original request %d times, want exactly once: %q", strings.Count(mergerPrompt, originalRequest), mergerPrompt)
	}
	previousSectionStart := -1
	for index := 1; index <= planParallelFanInChildCount; index++ {
		childName := planParallelFanInChildName(index)
		childInput := planParallelFanInChildInput(index)
		childResult := planParallelFanInChildResult(index)
		if strings.Count(mergerPrompt, childResult) != 1 {
			t.Fatalf("merger prompt contains %s %d times, want exactly once: %q", childResult, strings.Count(mergerPrompt, childResult), mergerPrompt)
		}
		if strings.Contains(mergerPrompt, childInput) {
			t.Fatalf("merger prompt contains planner payload %s instead of completed child results: %q", childInput, mergerPrompt)
		}
		sectionStart := strings.Index(mergerPrompt, fmt.Sprintf("--- %s (planned-task) ---", childName))
		if sectionStart < 0 {
			t.Fatalf("merger prompt does not identify generated Work %s: %q", childName, mergerPrompt)
		}
		if sectionStart <= previousSectionStart {
			t.Fatalf("generated Work %s appears out of deterministic input order at offset %d after %d: %q", childName, sectionStart, previousSectionStart, mergerPrompt)
		}
		previousSectionStart = sectionStart
		sectionEnd := strings.Index(mergerPrompt[sectionStart+1:], "\n--- ")
		if sectionEnd < 0 {
			sectionEnd = len(mergerPrompt) - sectionStart - 1
		}
		section := mergerPrompt[sectionStart : sectionStart+1+sectionEnd]
		if strings.Count(section, childResult) != 1 {
			t.Fatalf("generated Work %s section contains result %s %d times, want exactly once: %q", childName, childResult, strings.Count(section, childResult), section)
		}
		if strings.Contains(section, childInput) {
			t.Fatalf("generated Work %s section contains planner payload %s instead of only the completed result: %q", childName, childInput, section)
		}
	}
}

func planParallelFanInDAG() string {
	works := make([]string, 0, planParallelFanInChildCount)
	for index := 1; index <= planParallelFanInChildCount; index++ {
		works = append(works, fmt.Sprintf(`{"name":"%s","workTypeName":"planned-task","payload":"%s"}`, planParallelFanInChildName(index), planParallelFanInChildInput(index)))
	}
	return `{"request":{"type":"FACTORY_REQUEST_BATCH","works":[` + strings.Join(works, ",") + `]}}`
}

func planParallelFanInChildName(index int) string {
	return fmt.Sprintf("fan-in-task-%02d", index)
}

func planParallelFanInChildInput(index int) string {
	return fmt.Sprintf("FANIN_CHILD_INPUT_%02d", index)
}

func planParallelFanInChildResult(index int) string {
	return fmt.Sprintf("FANIN_CHILD_RESULT_%02d", index)
}

func testPackagedPlanParallelExecutesReadyDAGConcurrentlyAndMerges(
	t *testing.T,
	fixture *planParallelSharedFixture,
) {
	// This cell intentionally uses the public API because it owns retained
	// Factory Event and reconnect/replay assertions below.
	runner := newPlanParallelRunner(parallelDAG)
	scenario := fixture.newScenario(t, runner)
	scenario.open(t)
	response := invokePlanParallel(t, scenario, map[string]any{
		"request":       "implement three dependent tasks",
		"executorModel": "gpt-5.6-luna",
	})

	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		message := ""
		if response.Message != nil {
			message = *response.Message
		}
		requests := runner.requestsSnapshot()
		args := make([][]string, 0, len(requests))
		for _, request := range requests {
			args = append(args, request.Args)
		}
		errors := make([]string, 0)
		for _, event := range support.GetFactoryEventsForSessionAt(t, scenario.fixture.baseURL, scenario.sessionID) {
			if event.Type != factoryapi.FactoryEventTypeDispatchResponse {
				continue
			}
			payload, err := event.Payload.AsDispatchResponseEventPayload()
			if err == nil && payload.Error != nil {
				errors = append(errors, *payload.Error)
			}
			if err == nil && payload.FailureDetail != nil {
				errors = append(errors, payload.FailureDetail.Message)
			}
		}
		t.Fatalf("status = %q, want COMPLETED; message = %q; dispatch errors = %#v; request args = %#v; response = %#v", response.Status, message, errors, args, response)
	}
	if got := planParallelPrimaryText(t, response); !strings.Contains(got, "merged parallel result") {
		t.Fatalf("primary result = %q, want merger output", got)
	}
	if runner.maxConcurrentExecutors() < 2 {
		t.Fatalf("maximum concurrent task executors = %d, want at least 2 dependency-ready tasks", runner.maxConcurrentExecutors())
	}
	if runner.executionCount() != 3 || runner.mergeCount() != 1 {
		t.Fatalf("executor calls = %d, merge calls = %d; want 3 and 1", runner.executionCount(), runner.mergeCount())
	}
	assertPlanParallelPromptsContainInputs(t, runner.requestsSnapshot())
	assertPlanParallelProviderSelection(t, runner.requestsSnapshot())

	events := support.GetFactoryEventsForSessionAt(t, scenario.fixture.baseURL, scenario.sessionID)
	dispatches := support.ObserveDispatchEvents(t, events)
	if len(dispatches) != 5 || dispatches[0].Request.TransitionId != "plan-parallel-work" ||
		dispatches[len(dispatches)-1].Request.TransitionId != "merge-plan-results" {
		t.Fatalf("dispatches = %#v, want planner, three tasks, and terminal merger", dispatches)
	}
	assertPlanParallelGeneratedDAGEvent(t, events)
	assertPlanParallelRetainedReplay(t, scenario, events)
}

func testPackagedPlanParallelExecutorEffortCanBeOverridden(
	t *testing.T,
	fixture *planParallelSharedFixture,
) {
	runner := newPlanParallelRunner(parallelDAG)
	scenario := fixture.newScenario(t, runner)
	scenario.open(t)
	response, err := runPlanParallelInvocation(t, scenario, map[string]any{
		"request":                 "implement three dependent tasks",
		"executorModel":           "gpt-5.6-luna",
		"executorReasoningEffort": "low",
	})
	if err != nil {
		t.Fatalf("plan-parallel invocation: %v", err)
	}
	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("response = %#v, want completed low-effort override", response)
	}
	executors := 0
	for _, request := range runner.requestsSnapshot() {
		prompt := string(request.Stdin)
		if strings.Contains(prompt, "Plan an executable Work DAG") ||
			strings.Contains(prompt, "Treat the original request and all completed generated Work inputs") {
			continue
		}
		executors++
		if !planParallelHasArgPair(request.Args, "--config", `model_reasoning_effort="low"`) {
			t.Fatalf("executor args = %#v, want low effort override", request.Args)
		}
	}
	if executors != 3 {
		t.Fatalf("executor requests = %d, want 3", executors)
	}
}

func testPackagedPlanParallelRejectsUnsupportedEffortBeforeExecutorProviderExecution(
	t *testing.T,
	fixture *planParallelSharedFixture,
) {
	runner := newPlanParallelRunner(parallelDAG)
	scenario := fixture.newScenario(t, runner)
	scenario.open(t)
	response, err := runPlanParallelInvocation(t, scenario, map[string]any{
		"request":                 "implement three dependent tasks",
		"executorReasoningEffort": "extreme",
	})
	if err != nil {
		t.Fatalf("plan-parallel invocation: %v", err)
	}
	if response.Status != factoryapi.InvocationTerminalStatusFailed || response.PrimaryResult != nil {
		t.Fatalf("response = %#v, want failed invocation without primary result", response)
	}
	requests := runner.requestsSnapshot()
	if len(requests) != 1 || !strings.Contains(string(requests[0].Stdin), "Plan an executable Work DAG") {
		t.Fatalf("provider requests = %#v, want only the prerequisite planner and no executor process", requests)
	}
}

func assertPlanParallelProviderSelection(t *testing.T, requests []platformprocess.CommandRequest) {
	t.Helper()
	executors := 0
	for index, request := range requests {
		if request.Command != "codex" ||
			!planParallelHasArg(request.Args, "--dangerously-bypass-approvals-and-sandbox") {
			t.Fatalf("provider request[%d] = command %q args %#v, want Codex with packaged skip-permissions", index, request.Command, request.Args)
		}
		prompt := string(request.Stdin)
		isExecutor := !strings.Contains(prompt, "Plan an executable Work DAG") &&
			!strings.Contains(prompt, "Treat the original request and all completed generated Work inputs")
		if isExecutor {
			executors++
			if !planParallelHasArgPair(request.Args, "--model", "gpt-5.6-luna") ||
				!planParallelHasArgPair(request.Args, "--config", `model_reasoning_effort="xhigh"`) {
				t.Fatalf("executor request[%d] args = %#v, want Luna xhigh", index, request.Args)
			}
			continue
		}
		if !planParallelHasArgPair(request.Args, "--model", "operator-model") ||
			planParallelHasArg(request.Args, "--config") {
			t.Fatalf("planner/merger request[%d] args = %#v, want operator model without effort override", index, request.Args)
		}
	}
	if executors != 3 {
		t.Fatalf("executor requests = %d, want 3", executors)
	}
}

func assertPlanParallelPromptsContainInputs(t *testing.T, requests []platformprocess.CommandRequest) {
	t.Helper()
	executorPrompts := make([]string, 0, 3)
	var mergerPrompt string
	for _, request := range requests {
		prompt := string(request.Stdin)
		switch {
		case strings.Contains(prompt, "Plan an executable Work DAG"):
		case strings.Contains(prompt, "Treat the original request and all completed generated Work inputs"):
			mergerPrompt = prompt
		default:
			executorPrompts = append(executorPrompts, prompt)
		}
	}
	if len(executorPrompts) != 3 {
		t.Fatalf("executor prompts = %d, want 3", len(executorPrompts))
	}
	markers := []string{"TASK_A_UNIQUE_OBJECTIVE", "TASK_B_UNIQUE_OBJECTIVE", "TASK_C_UNIQUE_OBJECTIVE"}
	for index, prompt := range executorPrompts {
		matches := 0
		for _, marker := range markers {
			if strings.Contains(prompt, marker) {
				matches++
			}
		}
		if matches != 1 || strings.Contains(prompt, "implement three dependent tasks") {
			t.Fatalf("executor prompt[%d] did not isolate exactly one assigned payload: %q", index, prompt)
		}
	}
	if mergerPrompt == "" || !strings.Contains(mergerPrompt, "implement three dependent tasks") ||
		strings.Count(mergerPrompt, "planned task completed") != 3 {
		t.Fatalf("merger prompt does not contain the parent request and all three child results: %q", mergerPrompt)
	}
}

func planParallelMergerPrompt(t *testing.T, requests []platformprocess.CommandRequest) string {
	t.Helper()
	var mergerPrompt string
	for _, request := range requests {
		prompt := string(request.Stdin)
		if !strings.Contains(prompt, "Treat the original request and all completed generated Work inputs") {
			continue
		}
		if mergerPrompt != "" {
			t.Fatalf("captured more than one merger prompt")
		}
		mergerPrompt = prompt
	}
	if mergerPrompt == "" {
		t.Fatalf("captured no merge-plan-results prompt")
	}
	return mergerPrompt
}

func assertPlanParallelGeneratedDAGEvent(t *testing.T, events []factoryapi.FactoryEvent) {
	t.Helper()
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeWorkRequest {
			continue
		}
		payload, err := event.Payload.AsWorkRequestEventPayload()
		if err != nil || payload.Works == nil || len(*payload.Works) != 3 || payload.Relations == nil {
			continue
		}
		names := make([]string, 0, len(*payload.Works))
		for _, item := range *payload.Works {
			names = append(names, item.Name)
		}
		if strings.Join(names, ",") != "task-a,task-b,task-c" || len(*payload.Relations) != 5 {
			t.Fatalf("replayed generated DAG = works %v relations %#v", names, *payload.Relations)
		}
		dependencies := 0
		parentChildren := 0
		for _, relation := range *payload.Relations {
			switch relation.Type {
			case factoryapi.RelationTypeDependsOn:
				if relation.SourceWorkName != "task-c" ||
					(relation.TargetWorkName != "task-a" && relation.TargetWorkName != "task-b") {
					t.Fatalf("replayed generated dependency = %#v", relation)
				}
				dependencies++
			case factoryapi.RelationTypeParentChild:
				parentChildren++
			}
		}
		if dependencies != 2 || parentChildren != 3 {
			t.Fatalf("replayed generated relation counts = dependencies %d parent-child %d", dependencies, parentChildren)
		}
		return
	}
	t.Fatal("retained event history did not reconstruct the generated Work DAG")
}

func assertPlanParallelRetainedReplay(
	t *testing.T,
	scenario *planParallelScenario,
	events []factoryapi.FactoryEvent,
) {
	t.Helper()
	if len(events) < 2 {
		t.Fatalf("events = %d, want retained history", len(events))
	}
	sequence := support.ReconnectSequenceForFactoryEvent(events[0])
	replayed := support.GetFactoryEventsAfterForSessionAt(t, scenario.fixture.baseURL, scenario.sessionID, support.FactoryEventReadCursor{
		AfterEventID:  events[0].Id,
		AfterSequence: &sequence,
	})
	if len(replayed) != len(events)-1 {
		t.Fatalf("retained replay events = %d, want %d", len(replayed), len(events)-1)
	}
	for index := range replayed {
		if replayed[index].Id != events[index+1].Id {
			t.Fatalf("retained replay event %d = %q, want %q", index, replayed[index].Id, events[index+1].Id)
		}
	}
}

func testPackagedPlanParallelRejectsPlannerBatchAboveCeilingAtomically(
	t *testing.T,
	fixture *planParallelSharedFixture,
) {
	works := make([]string, 13)
	for index := range works {
		works[index] = fmt.Sprintf(`{"name":"task-%02d","workTypeName":"planned-task"}`, index+1)
	}
	runner := newPlanParallelRunner(`{"request":{"type":"FACTORY_REQUEST_BATCH","works":[` + strings.Join(works, ",") + `]}}`)
	scenario := fixture.newScenario(t, runner)
	scenario.open(t)
	response := invokePlanParallel(t, scenario, map[string]any{"request": "produce too many tasks"})

	if response.Status != factoryapi.InvocationTerminalStatusFailed || response.PrimaryResult != nil {
		t.Fatalf("response = %#v, want failed invocation without primary result", response)
	}
	if runner.executionCount() != 0 || runner.mergeCount() != 0 {
		t.Fatalf("executor calls = %d, merge calls = %d; want atomic rejection", runner.executionCount(), runner.mergeCount())
	}
}

func testPackagedPlanParallelChildFailureFansInWithoutMerge(
	t *testing.T,
	fixture *planParallelSharedFixture,
) {
	runner := newPlanParallelRunner(`{"request":{"type":"FACTORY_REQUEST_BATCH","works":[{"name":"failing-task","workTypeName":"planned-task"}]}}`)
	runner.failExecutors = true
	scenario := fixture.newScenario(t, runner)
	scenario.open(t)
	response := invokePlanParallel(t, scenario, map[string]any{"request": "surface a child failure"})

	if response.Status != factoryapi.InvocationTerminalStatusFailed || response.PrimaryResult != nil {
		t.Fatalf("response = %#v, want failed invocation without primary result", response)
	}
	// A native provider failure that never established a Provider Session is
	// terminal at the Workers boundary, so the child executes exactly once and
	// the fan-in still reports failure without merging. This previously
	// asserted 3: the agent-run harness stringified the typed ProviderError, so
	// the generic {terminal, internal_server_error} fallback classified a
	// terminal failure as retryable and requeued the Work twice more. Keep the
	// comparison exact so an unbounded retry regression still fails here.
	if runner.executionCount() != 1 || runner.mergeCount() != 0 {
		t.Fatalf("executor calls = %d, merge calls = %d; want one terminal child attempt and no merge", runner.executionCount(), runner.mergeCount())
	}
}

func testPackagedPlanParallelReusesProcessAfterChildFailure(
	t *testing.T,
	fixture *planParallelSharedFixture,
) {
	runner := newPlanParallelRunner(parallelDAG)
	scenario := fixture.newScenario(t, runner)
	scenario.open(t)
	response := invokePlanParallel(t, scenario, map[string]any{
		"request": "reuse the plan-parallel process after a child failure",
	})
	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("response = %#v, want completed invocation after prior child failure", response)
	}
	if runner.executionCount() != 3 || runner.mergeCount() != 1 {
		t.Fatalf("executor calls = %d, merge calls = %d; want 3 and 1 after reuse", runner.executionCount(), runner.mergeCount())
	}
}

type planParallelRunner struct {
	mu             sync.Mutex
	plannerOutput  string
	uniqueResults  bool
	active         int
	maxActive      int
	executions     int
	merges         int
	failExecutors  bool
	readyExecutors chan struct{}
	requests       []platformprocess.CommandRequest
}

func newPlanParallelRunner(plannerOutput string) *planParallelRunner {
	return &planParallelRunner{plannerOutput: plannerOutput, readyExecutors: make(chan struct{})}
}

func newPlanParallelFanInRunner(plannerOutput string) *planParallelRunner {
	runner := newPlanParallelRunner(plannerOutput)
	runner.uniqueResults = true
	return runner
}

func (runner *planParallelRunner) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	runner.mu.Lock()
	runner.requests = append(runner.requests, request)
	runner.mu.Unlock()
	prompt := string(request.Stdin)
	switch {
	case strings.Contains(prompt, "Plan an executable Work DAG"):
		return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout(runner.plannerOutput)}, nil
	case strings.Contains(prompt, "Treat the original request and all completed generated Work inputs"):
		runner.mu.Lock()
		runner.merges++
		runner.mu.Unlock()
		return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout("merged parallel result")}, nil
	default:
		runner.mu.Lock()
		runner.executions++
		if runner.failExecutors {
			runner.mu.Unlock()
			return platformprocess.CommandResult{}, errors.New("planned task provider failure")
		}
		runner.active++
		if runner.active > runner.maxActive {
			runner.maxActive = runner.active
		}
		if runner.active >= 2 {
			select {
			case <-runner.readyExecutors:
			default:
				close(runner.readyExecutors)
			}
		}
		runner.mu.Unlock()

		select {
		case <-runner.readyExecutors:
		case <-time.After(2 * time.Second):
		case <-ctx.Done():
			return platformprocess.CommandResult{}, ctx.Err()
		}
		runner.mu.Lock()
		runner.active--
		runner.mu.Unlock()
		output := "planned task completed"
		if runner.uniqueResults {
			for index := 1; index <= planParallelFanInChildCount; index++ {
				if strings.Contains(prompt, planParallelFanInChildInput(index)) {
					output = planParallelFanInChildResult(index)
					break
				}
			}
		}
		return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout(output)}, nil
	}
}

func (runner *planParallelRunner) requestsSnapshot() []platformprocess.CommandRequest {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return append([]platformprocess.CommandRequest(nil), runner.requests...)
}

func planParallelHasArgPair(args []string, name, value string) bool {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == name && args[index+1] == value {
			return true
		}
	}
	return false
}

func planParallelHasArg(args []string, name string) bool {
	for _, arg := range args {
		if arg == name {
			return true
		}
	}
	return false
}

func (runner *planParallelRunner) maxConcurrentExecutors() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.maxActive
}

func (runner *planParallelRunner) executionCount() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.executions
}

func (runner *planParallelRunner) mergeCount() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.merges
}

func runPlanParallelInvocation(
	t *testing.T,
	scenario *planParallelScenario,
	args map[string]any,
) (factoryapi.InvocationResponse, error) {
	t.Helper()
	statusCode, responseBody := postPlanParallel(t, scenario, args)
	if statusCode != http.StatusOK {
		return factoryapi.InvocationResponse{}, fmt.Errorf(
			"POST plan-parallel invocation status = %d: %s",
			statusCode,
			strings.TrimSpace(string(responseBody)),
		)
	}
	var decoded factoryapi.InvocationResponse
	if err := json.Unmarshal(responseBody, &decoded); err != nil {
		return factoryapi.InvocationResponse{}, fmt.Errorf("decode plan-parallel invocation: %w", err)
	}
	return decoded, nil
}

func invokePlanParallel(t *testing.T, scenario *planParallelScenario, args map[string]any) factoryapi.InvocationResponse {
	t.Helper()
	statusCode, responseBody := postPlanParallel(t, scenario, args)
	if statusCode != http.StatusOK {
		t.Fatalf("POST invocation status = %d; body = %s", statusCode, responseBody)
	}
	var decoded factoryapi.InvocationResponse
	if err := json.Unmarshal(responseBody, &decoded); err != nil {
		t.Fatalf("decode invocation: %v", err)
	}
	return decoded
}

func postPlanParallel(
	t *testing.T,
	scenario *planParallelScenario,
	args map[string]any,
) (int, []byte) {
	t.Helper()
	requestID := fmt.Sprintf("plan-parallel-%d", time.Now().UnixNano())
	payload, err := json.Marshal(factoryapi.InvocationRequest{RequestId: &requestID, Args: &args})
	if err != nil {
		t.Fatalf("marshal invocation: %v", err)
	}
	endpoint := strings.TrimSuffix(scenario.fixture.baseURL, "/") +
		"/factory-sessions/" + url.PathEscape(scenario.sessionID) + "/invocations"
	response, err := http.Post(endpoint, "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("POST invocation: %v", err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read invocation response: %v", err)
	}
	return response.StatusCode, responseBody
}

func planParallelPrimaryText(t *testing.T, response factoryapi.InvocationResponse) string {
	t.Helper()
	if response.PrimaryResult == nil || len(*response.PrimaryResult) != 1 {
		t.Fatalf("primary result = %#v, want one part", response.PrimaryResult)
	}
	part, err := (*response.PrimaryResult)[0].AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("decode primary text: %v", err)
	}
	return part.Text
}
