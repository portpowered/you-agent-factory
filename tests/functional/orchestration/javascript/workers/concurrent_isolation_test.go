package workers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const concurrentJavaScriptWaitTimeout = 15 * time.Second

type concurrentJavaScriptResult struct {
	response factoryapi.FactorySessionSyncExecutionResponse
	err      error
}

func runJavaScriptConcurrentIsolation(t *testing.T, fixture *javascriptSharedProcessFixture) {
	successPrompt := "shared concurrent success"
	successGate := make(chan struct{})
	successRunner := support.NewGatedSuccessCommandRunner("concurrent success output", successGate)
	if err := fixture.router.register(successPrompt, successRunner); err != nil {
		t.Fatalf("register concurrent success route: %v", err)
	}
	t.Cleanup(func() {
		select {
		case <-successGate:
		default:
			close(successGate)
		}
		if err := fixture.router.unregister(successPrompt); err != nil {
			t.Errorf("unregister concurrent success route: %v", err)
		}
	})

	successWorkflow := strings.ReplaceAll(liveProviderChildWorkflow, "use the live provider command edge", successPrompt)
	failureWorkflow := invalidPermissionsOverrideWorkflowWithValue("true", "shared concurrent failure")
	requestBase := fixture.requestSequence.Load() + 1
	successRequestID := fmt.Sprintf("shared-javascript-concurrent-success-%d", requestBase)
	failureRequestID := fmt.Sprintf("shared-javascript-concurrent-failure-%d", requestBase+1)
	beforeCalls := fixture.router.callCount()

	successCh := make(chan concurrentJavaScriptResult, 1)
	go func() {
		response, err := postOverridesWorkflow(context.Background(), fixture.baseURL, successRequestID, successWorkflow)
		successCh <- concurrentJavaScriptResult{response: response, err: err}
	}()

	// The success command is intentionally held at the controlled edge while
	// the failure request runs. This bounded context is required because the
	// producers are real HTTP/Process.Execute calls and the gated edge must
	// prove ordering; channel receives below are the deterministic completion
	// barriers, so a sleep or timeout-padded polling helper cannot substitute.
	waitContext, cancelWait := context.WithTimeout(context.Background(), concurrentJavaScriptWaitTimeout)
	defer cancelWait()
	if err := fixture.router.waitForCall(waitContext, beforeCalls+1); err != nil {
		t.Fatalf("wait for concurrent success provider request: %v", err)
	}

	failureCh := make(chan concurrentJavaScriptResult, 1)
	go func() {
		response, err := postOverridesWorkflow(context.Background(), fixture.baseURL, failureRequestID, failureWorkflow)
		failureCh <- concurrentJavaScriptResult{response: response, err: err}
	}()

	failure, err := awaitConcurrentJavaScriptResult(waitContext, failureCh, "failure")
	if err != nil {
		t.Fatal(err)
	}
	if failure.err != nil {
		t.Fatalf("concurrent failure workflow: %v", failure.err)
	}
	if failure.response.Status != factoryapi.FactorySessionDurableLifecycleStatusFailed {
		t.Fatalf("concurrent failure status = %q, want FAILED", failure.response.Status)
	}

	close(successGate)
	success, err := awaitConcurrentJavaScriptResult(waitContext, successCh, "success")
	if err != nil {
		t.Fatal(err)
	}
	assertConcurrentJavaScriptIsolation(t, fixture, successPrompt, success, failure)
}

func assertConcurrentJavaScriptIsolation(
	t *testing.T,
	fixture *javascriptSharedProcessFixture,
	successPrompt string,
	success, failure concurrentJavaScriptResult,
) {
	t.Helper()
	if success.err != nil {
		t.Fatalf("concurrent success workflow: %v", success.err)
	}
	if success.response.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("concurrent success status = %q, want SUCCEEDED", success.response.Status)
	}
	if success.response.SessionId == failure.response.SessionId {
		t.Fatalf("concurrent workflows reused Factory Session ID %q", success.response.SessionId)
	}
	fixture.trackSession(t, failure.response.SessionId)
	fixture.trackSession(t, success.response.SessionId)
	assertSucceededPrimaryContains(t, success.response, "concurrent success output")
	assertUnavailableFactoryResult(t, failure.response.Result)

	failureSession := readOverridesDurableSession(t, fixture.baseURL, failure.response.SessionId)
	if failureSession.FailureDetail == nil || !strings.Contains(strings.ToLower(failureSession.FailureDetail.Message), "permissions") {
		t.Fatalf("concurrent failure detail = %#v, want permissions diagnostic", failureSession.FailureDetail)
	}
	successEvents := support.GetFactoryEventsForSessionAt(t, fixture.baseURL, success.response.SessionId)
	failureEvents := support.GetFactoryEventsForSessionAt(t, fixture.baseURL, failure.response.SessionId)
	assertJavaScriptEventsDoNotContain(t, successEvents, "shared concurrent failure")
	assertJavaScriptEventsDoNotContain(t, failureEvents, "concurrent success output")

	assertConcurrentDispatches(t, fixture, success.response.SessionId, failure.response.SessionId)
	requests := fixture.router.requestRecords()
	if len(requests) == 0 || !bytes.Contains(requests[len(requests)-1].Stdin, []byte(successPrompt)) || bytes.Contains(requests[len(requests)-1].Stdin, []byte("shared concurrent failure")) {
		t.Fatalf("concurrent command requests = %#v, want only success request content", requests)
	}
}

func assertConcurrentDispatches(t *testing.T, fixture *javascriptSharedProcessFixture, successSessionID, failureSessionID string) {
	t.Helper()
	baseURL := strings.TrimSuffix(fixture.baseURL, "/")
	dispatches := support.GetJSON[factoryapi.ListFactorySessionDispatchesResponse](
		t,
		baseURL+"/factory-sessions/"+successSessionID+"/dispatches",
	)
	if len(dispatches.Dispatches) != 1 || dispatches.Dispatches[0].Status != factoryapi.FactoryDispatchStatusCOMPLETED {
		t.Fatalf("concurrent success dispatches = %#v, want one completed dispatch", dispatches.Dispatches)
	}
	failureDispatches := support.GetJSON[factoryapi.ListFactorySessionDispatchesResponse](
		t,
		baseURL+"/factory-sessions/"+failureSessionID+"/dispatches",
	)
	if len(failureDispatches.Dispatches) != 0 {
		t.Fatalf("concurrent failure dispatches = %#v, want no provider dispatch", failureDispatches.Dispatches)
	}
}

func awaitConcurrentJavaScriptResult(
	ctx context.Context,
	results <-chan concurrentJavaScriptResult,
	name string,
) (concurrentJavaScriptResult, error) {
	select {
	case result := <-results:
		return result, nil
	case <-ctx.Done():
		return concurrentJavaScriptResult{}, fmt.Errorf("timed out waiting for concurrent %s workflow: %w", name, ctx.Err())
	}
}

func assertJavaScriptEventsDoNotContain(t *testing.T, events []factoryapi.FactoryEvent, forbidden string) {
	t.Helper()
	encoded, err := json.Marshal(events)
	if err != nil {
		t.Fatalf("marshal Factory Events: %v", err)
	}
	if strings.Contains(string(encoded), forbidden) {
		t.Fatalf("Factory Events contain foreign content %q: %s", forbidden, encoded)
	}
}

// Both first children are held at the command edge before either completes.
// Public Worker observations prove actual scoped admission; collector strings
// alone do not establish Worker identity or canonical recording association.
func runJavaScriptRuntimeChildren(t *testing.T, fixture *javascriptSharedProcessFixture) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	var runners [2]*runtimeJavaScriptCommandRunner
	var results [2]chan concurrentJavaScriptResult
	var requestIDs, workflows [2]string
	for i := range runners {
		marker := fmt.Sprintf("runtime javascript child %d", i)
		runner := &runtimeJavaScriptCommandRunner{
			marker: marker, started: make(chan struct{}), release: make(chan struct{}),
		}
		runners[i] = runner
		if err := fixture.router.register(marker, runner); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			runner.unblock()
			if err := fixture.router.unregister(marker); err != nil {
				t.Error(err)
			}
		})
		workflow := strings.ReplaceAll(liveProviderChildWorkflow, "use the live provider command edge", marker)
		requestID := fmt.Sprintf("runtime-javascript-%d", fixture.requestSequence.Add(1))
		requestIDs[i], workflows[i] = requestID, workflow
		result := make(chan concurrentJavaScriptResult, 1)
		results[i] = result
		go func() {
			response, err := postOverridesWorkflow(ctx, fixture.baseURL, requestID, workflow)
			result <- concurrentJavaScriptResult{response: response, err: err}
		}()
	}
	for _, runner := range runners {
		select {
		case <-runner.started:
		case <-ctx.Done():
			t.Fatalf("first children did not overlap: %v", ctx.Err())
		}
	}
	// Release one child while its peer remains admitted at the selected edge.
	runners[0].unblock()
	first, err := awaitConcurrentJavaScriptResult(ctx, results[0], "first runtime child")
	if err != nil {
		t.Fatal(err)
	}
	firstWorker := assertRuntimeJavaScriptChild(t, fixture, first, runners[0].marker, runners[1].marker)
	assertJavaScriptRetainedRequest(t, ctx, fixture, requestIDs[0], workflows[0], first.response, runners[0])
	select {
	case peer := <-results[1]:
		t.Fatalf("peer completed before its own command release: %#v", peer)
	default:
	}
	runners[1].unblock()
	second, err := awaitConcurrentJavaScriptResult(ctx, results[1], "second runtime child")
	if err != nil {
		t.Fatal(err)
	}
	secondWorker := assertRuntimeJavaScriptChild(t, fixture, second, runners[1].marker, runners[0].marker)
	if first.response.SessionId == second.response.SessionId || firstWorker == secondWorker {
		t.Fatalf("children reused identities: Factory %q/%q Worker %q/%q", first.response.SessionId, second.response.SessionId, firstWorker, secondWorker)
	}
	for _, runner := range runners {
		if runner.calls.Load() != 1 {
			t.Fatalf("child %q command calls = %d, want one", runner.marker, runner.calls.Load())
		}
	}
}

func assertJavaScriptRetainedRequest(t *testing.T, ctx context.Context, fixture *javascriptSharedProcessFixture, requestID, workflow string, first factoryapi.FactorySessionSyncExecutionResponse, runner *runtimeJavaScriptCommandRunner) {
	t.Helper()
	// A customer retry of the same normalized public request returns its
	// retained session/result while the other child's command is still live.
	// This proves request replay, separately from workflow child resume.
	replayed, replayErr := postOverridesWorkflow(ctx, fixture.baseURL, requestID, workflow)
	if replayErr != nil {
		t.Fatal(replayErr)
	}
	if replayed.SessionId != first.SessionId || replayed.Status != first.Status {
		t.Fatalf("request replay changed session/outcome: first=%+v replay=%+v", first, replayed)
	}
	assertSucceededPrimaryContains(t, replayed, runner.marker+" output")
	if runner.calls.Load() != 1 {
		t.Fatalf("request replay started another child command: calls=%d", runner.calls.Load())
	}
}

type runtimeJavaScriptCommandRunner struct {
	marker  string
	started chan struct{}
	release chan struct{}
	calls   atomic.Int32
}

func (r *runtimeJavaScriptCommandRunner) unblock() {
	select {
	case <-r.release:
	default:
		close(r.release)
	}
}

func (r *runtimeJavaScriptCommandRunner) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	if r.calls.Add(1) != 1 {
		return platformprocess.CommandResult{}, fmt.Errorf("unexpected repeat child command %q", r.marker)
	}
	if request.Command != "codex" || !bytes.Contains(request.Stdin, []byte(r.marker)) {
		return platformprocess.CommandResult{}, fmt.Errorf("unexpected selected command %q", request.Command)
	}
	close(r.started)
	select {
	case <-r.release:
	case <-ctx.Done():
		return platformprocess.CommandResult{}, ctx.Err()
	}
	stdout := append([]byte(fmt.Sprintf("{\"type\":\"thread.started\",\"thread_id\":%q}\n", "provider-"+r.marker)), support.CodexSuccessStdout(r.marker+" output")...)
	return platformprocess.CommandResult{Stdout: stdout}, nil
}

func assertRuntimeJavaScriptChild(t *testing.T, fixture *javascriptSharedProcessFixture, result concurrentJavaScriptResult, own, foreign string) string {
	t.Helper()
	if result.err != nil {
		t.Fatal(result.err)
	}
	assertSucceededPrimaryContains(t, result.response, own+" output")
	sessionID := result.response.SessionId
	fixture.trackSession(t, sessionID)
	assertJavaScriptSharedCompletedDispatch(t, fixture, sessionID, "codex", "live-child-model", "live-provider-child")
	factoryEvents := support.GetFactoryEventsForSessionAt(t, fixture.baseURL, sessionID)
	for _, event := range factoryEvents {
		if event.Context.SessionId == nil || *event.Context.SessionId != sessionID {
			t.Fatalf("foreign Factory event: %#v", event)
		}
	}
	assertJavaScriptEventsDoNotContain(t, factoryEvents, foreign)
	dispatches := support.GetJSON[factoryapi.ListFactorySessionDispatchesResponse](t,
		strings.TrimSuffix(fixture.baseURL, "/")+"/factory-sessions/"+sessionID+"/dispatches")
	dispatch := dispatches.Dispatches[0]
	if dispatch.Id != "dispatch-1" || dispatch.ProviderSessionRefs == nil || len(*dispatch.ProviderSessionRefs) != 1 || (*dispatch.ProviderSessionRefs)[0].Id != "provider-"+own {
		t.Fatalf("runtime first-child dispatch = %#v", dispatch)
	}
	observations := support.GetJSON[factoryapi.ListWorkerSessionsResponse](t,
		strings.TrimSuffix(fixture.baseURL, "/")+"/worker-sessions")
	var owned []factoryapi.WorkerSessionObservation
	for _, observation := range observations.Sessions {
		if observation.FactorySessionId != nil && *observation.FactorySessionId == sessionID {
			owned = append(owned, observation)
		}
	}
	if len(owned) != 1 {
		t.Fatalf("runtime child observations = %#v, want one", owned)
	}
	worker := owned[0]
	assertJavaScriptWorkerIdentity(t, worker, sessionID, dispatch.Id, own)
	t.Logf("F16-08 public observation: Factory=%s collector=%s Worker=%s physical=%s provider=%s", sessionID, dispatch.Id, worker.WorkerSessionId, worker.AttemptId, worker.ProviderSession.Id)
	return worker.WorkerSessionId
}

// Local CLI execution returns its own durable JavaScript identity. Keep a
// runtime child live to prove both routes preserve their selected execution.
func runJavaScriptLocalCLIBesideRuntime(t *testing.T, fixture *javascriptSharedProcessFixture) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	runtimeMarker := "local cli witness runtime peer"
	directMarker := "local cli witness child"
	peer := &runtimeJavaScriptCommandRunner{marker: runtimeMarker, started: make(chan struct{}), release: make(chan struct{})}
	direct := &runtimeJavaScriptCommandRunner{marker: directMarker, started: make(chan struct{}), release: make(chan struct{})}
	direct.unblock()
	for marker, runner := range map[string]platformprocess.CommandRunner{runtimeMarker: peer, directMarker: direct} {
		if err := fixture.router.register(marker, runner); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			peer.unblock()
			if err := fixture.router.unregister(marker); err != nil {
				t.Error(err)
			}
		})
	}
	resultCh := make(chan concurrentJavaScriptResult, 1)
	go func() {
		workflow := strings.ReplaceAll(liveProviderChildWorkflow, "use the live provider command edge", runtimeMarker)
		response, err := postOverridesWorkflow(ctx, fixture.baseURL, "local-cli-runtime-peer", workflow)
		resultCh <- concurrentJavaScriptResult{response: response, err: err}
	}()
	select {
	case <-peer.started:
	case result := <-resultCh:
		t.Fatalf("runtime returned before admission: %+v", result)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	runLocalJavaScriptChild(t, fixture, ctx, directMarker, direct)
	select {
	case result := <-resultCh:
		t.Fatalf("local CLI invocation completed its gated runtime peer: %+v", result)
	default:
	}
	peer.unblock()
	result, err := awaitConcurrentJavaScriptResult(ctx, resultCh, "local CLI runtime peer")
	if err != nil {
		t.Fatal(err)
	}
	assertRuntimeJavaScriptChild(t, fixture, result, runtimeMarker, directMarker)
}

func runLocalJavaScriptChild(t *testing.T, fixture *javascriptSharedProcessFixture, ctx context.Context, marker string, runner *runtimeJavaScriptCommandRunner) {
	t.Helper()
	workflow := strings.ReplaceAll(liveProviderChildWorkflow, "use the live provider command edge", marker)
	dir := support.ScaffoldFactory(t, permissionMatrixFactoryConfig(workflow))
	sessionID := uuid.NewString()
	inputs := support.FakeInputs(ctx, []string{"you", "--json", "run", "--session", sessionID, "--factory", filepath.Join(dir, "factory.json"), "--output", "primary", marker})
	inputs.Input.Env = append([]string(nil), fixture.environment...)
	inputs.Input.WorkingDirectory = dir
	if err := fixture.process.Execute(inputs.Input); err != nil {
		t.Fatalf("local CLI JavaScript: %v stdout=%s stderr=%s", err, inputs.Stdout(), inputs.Stderr())
	}
	if !strings.Contains(inputs.Stdout(), marker+" output") || runner.calls.Load() != 1 {
		t.Fatalf("local CLI output=%s command calls=%d", inputs.Stdout(), runner.calls.Load())
	}
	var result factoryapi.InvocationResponse
	if err := json.Unmarshal([]byte(inputs.Stdout()), &result); err != nil {
		t.Fatal(err)
	}
	if result.SessionId == nil || *result.SessionId == "" || result.Status != "COMPLETED" {
		t.Fatalf("local CLI invocation outcome: %+v", result)
	}
	fixture.trackSession(t, *result.SessionId)
	observations := support.GetJSON[factoryapi.ListWorkerSessionsResponse](t, fixture.baseURL+"/worker-sessions")
	var owned []factoryapi.WorkerSessionObservation
	for _, worker := range observations.Sessions {
		if worker.FactorySessionId != nil && *worker.FactorySessionId == *result.SessionId {
			owned = append(owned, worker)
		}
	}
	if len(owned) != 1 {
		t.Fatalf("local CLI Worker observations: %+v", owned)
	}
	worker := owned[0]
	assertJavaScriptWorkerIdentity(t, worker, *result.SessionId, "dispatch-1", marker)
	assertJavaScriptSharedCompletedDispatch(t, fixture, *result.SessionId, "codex", "live-child-model", "live-provider-child")
	t.Logf("F16-08 CLI/API parity: supplied scope=%s returned Factory=%s Worker=%s physical=%s provider=%s", sessionID, *result.SessionId, worker.WorkerSessionId, worker.AttemptId, worker.ProviderSession.Id)
}

func assertJavaScriptWorkerIdentity(t *testing.T, worker factoryapi.WorkerSessionObservation, sessionID, dispatchID, marker string) {
	t.Helper()
	if worker.Direct || worker.State != "COMPLETED" || worker.WorkerSessionId != sessionID+"/"+dispatchID || worker.AttemptId != worker.WorkerSessionId+"/attempt/1" || worker.ProviderSession == nil || worker.ProviderSession.Id != "provider-"+marker {
		t.Fatalf("JavaScript Worker identity/outcome: %+v", worker)
	}
}
