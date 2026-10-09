package workersessions_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessionscli "github.com/portpowered/infinite-you/pkg/services/worker_sessions/transports/cli"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// Only kill-journal acknowledgements fail. Opening, activity, failure capture
// and ordinary controls delegate to the production profile-owned store.
type forcePersistenceStore struct {
	recordings.WorkerRecordingStore
	phase    string
	failures atomic.Int32
}

func (s *forcePersistenceStore) BeginWorkerControlOperation(ctx context.Context, record recordings.WorkerControlOperationRecord) (recordings.WorkerControlOperationRecord, bool, error) {
	if s.phase == "intent" && record.Operation.Action == "kill" {
		s.failures.Add(1)
		return recordings.WorkerControlOperationRecord{}, false, errors.New("secret-from-host")
	}
	return s.WorkerRecordingStore.BeginWorkerControlOperation(ctx, record)
}

func (s *forcePersistenceStore) AdvanceWorkerControlOperation(ctx context.Context, record recordings.WorkerControlOperationRecord, revision uint64) (recordings.WorkerControlOperationRecord, error) {
	if s.phase == "result" && record.Operation.Action == "kill" {
		s.failures.Add(1)
		return recordings.WorkerControlOperationRecord{}, errors.New("secret-from-host")
	}
	return s.WorkerRecordingStore.AdvanceWorkerControlOperation(ctx, record, revision)
}

func runForcePersistenceFailure(t *testing.T, process support.Process, phase, mode string) {
	t.Helper()
	dir := support.ScaffoldSingleStepFactory(t, "mcp-force-persistence-"+phase)
	support.WriteAgentConfig(t, dir, "processor", support.BuildModelWorkerConfig(models.ProviderCodex, "test-model"))
	runner := &forceHostRunner{started: make(chan (<-chan struct{}), 4), mode: mode}
	store := &forcePersistenceStore{phase: phase}
	host := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir, WaitForServiceModeRuntime: true,
		Edges: serviceedges.Edges{ProviderCommandRunner: runner, FactorySessionsWorkingDirectory: historyWorkingDirectory(dir),
			WorkerRecordingWriter: store, WorkerRecordingStoreObserver: func(base recordings.WorkerRecordingStore) { store.WorkerRecordingStore = base }},
	})
	session, ctx := startMCP(t, process, host.URL())
	admission := controlHostRunner{started: runner.started}
	sibling := admitControlWorker(t, ctx, host.URL(), "persistence-sibling", admission)
	for index, first := range []string{"http", "cli", "mcp"} {
		id := "persistence-" + first
		done := admitControlWorker(t, ctx, host.URL(), id, admission)
		observation := getHost(t, host.URL()+"/worker-sessions/"+id).(map[string]any)
		attempt := observation["attemptId"].(string)
		// Each transport admits a fresh request; the other transports repeat it.
		assertForcePersistenceError(t, host, session, ctx, first, id, attempt)
		if mode == "confirmed" {
			waitControlSignal(t, done)
		}
		for _, transport := range []string{"http", "cli", "mcp"} {
			assertForcePersistenceError(t, host, session, ctx, transport, id, attempt)
		}
		assertDegradedForceObservation(t, host, id, mode)
		wantSignals := int32(index + 1)
		if mode == "failed" {
			wantSignals = 0
			assertForceStillActive(t, host, id, done)
			callWorker(t, ctx, session, "control", map[string]any{"workerSessionId": id, "operation": "TERMINATE"})
			waitControlSignal(t, done)
			assertForcePersistenceError(t, host, session, ctx, first, id, attempt)
		}
		if runner.calls.Load() != int32(index+1) || runner.signals.Load() != wantSignals {
			t.Fatalf("degraded force repeated its effect: calls=%d signals=%d", runner.calls.Load(), runner.signals.Load())
		}
		assertForceStillActive(t, host, "persistence-sibling", sibling)
	}
	if store.failures.Load() == 0 {
		t.Fatal("force did not reach the injected persistence failure")
	}
	// A sibling can still use the ordinary durable terminate path.
	callWorker(t, ctx, session, "control", map[string]any{"workerSessionId": "persistence-sibling", "operation": "TERMINATE"})
	waitControlSignal(t, sibling)
}

func assertForcePersistenceError(t *testing.T, host *support.FunctionalAPIServer, session *mcp.ClientSession, ctx context.Context, transport, id, attempt string) {
	t.Helper()
	switch transport {
	case "http":
		result := postHostJSON(t, ctx, host.URL()+"/worker-sessions/"+id+"/terminate",
			map[string]any{"force": true, "requestId": "kill-" + id, "expectedAttemptId": attempt}, http.StatusServiceUnavailable).(map[string]any)
		if result["code"] != "WORKER_SESSION_CONTROL_FAILED" {
			t.Fatalf("degraded force HTTP error = %v", result)
		}
		assertNoForceSecret(t, result)
	case "cli":
		assertForceCLIError(t, host, id, attempt)
	case "mcp":
		result := callAction(t, ctx, session, "CONTROL",
			map[string]any{"operation": "KILL", "workerSessionId": id, "requestId": "kill-" + id, "expectedAttemptId": attempt})
		assertToolError(t, result, "worker_session.unavailable", true)
		assertNoForceSecret(t, result)
	}
}

func assertDegradedForceObservation(t *testing.T, host *support.FunctionalAPIServer, id, mode string) {
	t.Helper()
	observation := getHost(t, host.URL()+"/worker-sessions/"+id).(map[string]any)
	state := "TERMINATED"
	health := "DEGRADED"
	if mode == "failed" {
		state = "RUNNING"
		health = "INCOMPLETE"
	}
	if observation["state"] != state || observation["terminalCause"] == "OPERATOR_KILL" {
		t.Fatalf("uncommitted force observation = %v", observation)
	}
	if observation["recordingHealth"] != health || observation["recordingHealthReason"] != "CONTROL_OPERATION_PERSISTENCE_FAILED" {
		t.Fatalf("force persistence loss omitted capture degradation: %v", observation)
	}
}

// Only the process effect is controlled. Providers, Workers, recording,
// supervision and the three customer transports use the production graph.
type forceHostRunner struct {
	started chan (<-chan struct{})
	mode    string
	entered chan struct{}
	release <-chan struct{}
	signals atomic.Int32
	calls   atomic.Int32
}

func (r *forceHostRunner) Run(ctx context.Context, req platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	control := &forceHostControl{stop: make(chan struct{}), done: make(chan struct{}), signals: &r.signals, calls: &r.calls, mode: r.mode,
		entered: r.entered, release: r.release}
	defer close(control.done)
	if req.OwnedProcessObserver != nil && r.mode != "unattached" {
		req.OwnedProcessObserver(control)
	}
	r.started <- control.done
	select {
	case <-control.stop:
		return platformprocess.CommandResult{ExitCode: -1}, nil
	case <-ctx.Done():
		return platformprocess.CommandResult{}, ctx.Err()
	}
}

type forceHostControl struct {
	stop, done chan struct{}
	signals    *atomic.Int32
	calls      *atomic.Int32
	mode       string
	entered    chan struct{}
	release    <-chan struct{}
	once       sync.Once
}

func (c *forceHostControl) ForceKill(ctx context.Context) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	c.calls.Add(1)
	switch c.mode {
	case "declined":
		return false, nil
	case "failed":
		return false, errors.New("secret-from-host")
	}
	if c.release != nil {
		close(c.entered)
		select {
		case <-c.release:
		case <-ctx.Done():
			return false, ctx.Err()
		}
	}
	c.once.Do(func() { c.signals.Add(1); close(c.stop) })
	select {
	case <-c.done:
		return true, nil
	case <-ctx.Done():
		return false, ctx.Err()
	}
}

func runRealHostForceControls(t *testing.T, process support.Process) {
	t.Helper()
	dir := support.ScaffoldSingleStepFactory(t, "mcp-force-host")
	support.WriteAgentConfig(t, dir, "processor", support.BuildModelWorkerConfig(models.ProviderCodex, "test-model"))
	runner := &forceHostRunner{started: make(chan (<-chan struct{}), 4)}
	config := support.FunctionalAPIServerConfig{
		FactoryDir: dir, WaitForServiceModeRuntime: true,
		Edges: serviceedges.Edges{ProviderCommandRunner: runner, FactorySessionsWorkingDirectory: historyWorkingDirectory(dir)},
	}
	host := support.StartFunctionalAPIServer(t, config)
	session, ctx := startMCP(t, process, host.URL())
	var completed []completedForceControl
	admission := controlHostRunner{started: runner.started}
	sibling := admitControlWorker(t, ctx, host.URL(), "force-sibling", admission)
	assertForeignForceRefused(t, process, host, runner, sibling)
	for index, transport := range []string{"http", "cli", "mcp"} {
		id := "force-" + transport
		done := admitControlWorker(t, ctx, host.URL(), id, admission)
		observation := getHost(t, host.URL()+"/worker-sessions/"+id).(map[string]any)
		if observation["providerSessionAvailable"] != false {
			t.Fatalf("force fixture unexpectedly published a provider reference: %v", observation)
		}
		attempt := observation["attemptId"].(string)
		endpoint := host.URL() + "/worker-sessions/" + id + "/terminate"
		assertForceInputsRejected(t, host, session, ctx, id, endpoint)
		if runner.signals.Load() != int32(index) {
			t.Fatal("stale force signaled a provider attempt")
		}
		payload := map[string]any{"force": true, "requestId": "kill-" + id, "expectedAttemptId": attempt}
		args := map[string]any{"operation": "KILL", "workerSessionId": id, "requestId": "kill-" + id, "expectedAttemptId": attempt}
		var result any
		switch transport {
		case "http":
			result = postHostJSON(t, ctx, endpoint, payload, http.StatusOK)
		case "cli":
			result = executeForceCLI(t, host, id, attempt)
		case "mcp":
			result = callWorker(t, ctx, session, "control", args)["result"]
		}
		assertOwnedControl(t, result.(map[string]any), id, "TERMINATE", "APPLIED")
		if result.(map[string]any)["forced"] != true || result.(map[string]any)["state"] != "TERMINATED" {
			t.Fatalf("force response omitted joined force facts: %v", result)
		}
		waitControlSignal(t, done)
		assertJSONEqual(t, result, postHostJSON(t, ctx, endpoint, payload, http.StatusOK))
		assertJSONEqual(t, result, executeForceCLI(t, host, id, attempt))
		assertJSONEqual(t, result, callWorker(t, ctx, session, "control", args)["result"])
		postHostJSON(t, ctx, endpoint, map[string]any{"force": true, "requestId": "kill-" + id, "expectedAttemptId": "changed-attempt"}, http.StatusConflict)
		assertForceObservation(t, host, id)
		completed = append(completed, completedForceControl{id: id, attempt: attempt, result: result})
		select {
		case <-sibling:
			t.Fatal("force stopped sibling execution")
		default:
		}
	}
	if runner.signals.Load() != 3 {
		t.Fatalf("force/replays signaled %d times, want one per exact attempt", runner.signals.Load())
	}
	completed = append(completed, assertFactoryForceControl(t, host, dir, runner))
	select {
	case <-sibling:
		t.Fatal("Factory force stopped direct sibling execution")
	default:
	}
	callWorker(t, ctx, session, "control", map[string]any{"workerSessionId": "force-sibling", "operation": "TERMINATE"})
	waitControlSignal(t, sibling)
	// Join the original owner before reopening its durable profile. Recovery
	// must read saved outcomes without reconstructing process capabilities.
	host.Close(t)
	reopened := support.StartFunctionalAPIServer(t, config)
	recoveredSession, recoveredCtx := startMCP(t, process, reopened.URL())
	for _, saved := range completed {
		saved.assertRecovered(t, reopened, recoveredSession, recoveredCtx)
	}
	if runner.signals.Load() != 4 || runner.calls.Load() != 4 {
		t.Fatalf("restart repeated force effects: calls=%d signals=%d", runner.calls.Load(), runner.signals.Load())
	}
}

func assertFactoryForceControl(t *testing.T, host *support.FunctionalAPIServer, dir string, runner *forceHostRunner) completedForceControl {
	t.Helper()
	opened := support.OpenFactorySessionAt(t, host.URL(), dir)
	stream := support.OpenFactoryEventStreamAt(t, support.SessionEventsURL(host.URL(), opened.Session.Id))
	defer stream.Close()
	support.SubmitSessionWorkAt(t, host.URL(), opened.Session.Id, factoryapi.SubmitWorkRequest{WorkTypeName: "task", Payload: "hold Factory force worker"})
	var done <-chan struct{}
	select {
	case done = <-runner.started:
	case <-t.Context().Done():
		t.Fatal("Factory provider boundary not reached")
	}
	fleet := getHost(t, host.URL()+"/worker-sessions?scope=factory").(map[string]any)
	workers := fleet["sessions"].([]any)
	if len(workers) != 1 {
		t.Fatalf("Factory force fleet = %v", fleet)
	}
	observation := workers[0].(map[string]any)
	id, attempt := observation["workerSessionId"].(string), observation["attemptId"].(string)
	if observation["factorySessionId"] != opened.Session.Id || observation["direct"] != false || observation["providerSessionAvailable"] != false {
		t.Fatalf("Factory force lost exact scope: %v", observation)
	}
	endpoint := host.URL() + "/worker-sessions/" + id + "/terminate"
	payload := map[string]any{"force": true, "requestId": "kill-" + id, "expectedAttemptId": attempt}
	result := postHostJSON(t, t.Context(), endpoint, payload, http.StatusOK)
	control := result.(map[string]any)
	if control["workerSessionId"] != id || control["dispatchId"] != id || control["action"] != "TERMINATE" || control["outcome"] != "APPLIED" || control["forced"] != true {
		t.Fatalf("Factory force result lost exact dispatch: %v", control)
	}
	waitControlSignal(t, done)
	assertJSONEqual(t, result, executeForceCLI(t, host, id, attempt))
	assertForceObservation(t, host, id)
	assertFactoryForceEvents(t, host.URL(), opened.Session.Id, id, stream)
	if runner.signals.Load() != 4 {
		t.Fatalf("Factory force/replay signaled %d times, want 4 total", runner.signals.Load())
	}
	return completedForceControl{id: id, attempt: attempt, result: result}
}

// The live response is the synchronization boundary for canonical completion.
// Read retained history only after that event, then reconnect from its request
// to prove clients recover the same cancellation without another dispatch.
func assertFactoryForceEvents(t *testing.T, baseURL, sessionID, dispatchID string, stream *support.FactoryEventStream) {
	t.Helper()
	var request, response factoryapi.FactoryEvent
	for response.Id == "" {
		event := stream.NextEventContext(t.Context())
		if event.Context.DispatchId == nil || *event.Context.DispatchId != dispatchID {
			continue
		}
		switch event.Type {
		case factoryapi.FactoryEventTypeDispatchRequest:
			if request.Id != "" {
				t.Fatal("force admitted another request for the exact dispatch")
			}
			request = event
		case factoryapi.FactoryEventTypeDispatchResponse:
			response = event
		}
	}
	if request.Id == "" || support.ReconnectSequenceForFactoryEvent(request) >= support.ReconnectSequenceForFactoryEvent(response) {
		t.Fatal("force response did not follow its canonical dispatch request")
	}
	assertCanceledForcePayload(t, response)
	assertForceRestoredInput(t, request, response)
	retained := support.GetFactoryEventsForSessionAt(t, baseURL, sessionID)
	sequence := support.ReconnectSequenceForFactoryEvent(request)
	replayed := support.GetFactoryEventsAfterForSessionAt(t, baseURL, sessionID, support.FactoryEventReadCursor{
		AfterEventID: request.Id, AfterSequence: &sequence,
	})
	assertForceEventHistory(t, retained, dispatchID, response, 1)
	assertForceEventHistory(t, replayed, dispatchID, response, 0)
}

func assertCanceledForcePayload(t *testing.T, event factoryapi.FactoryEvent) {
	t.Helper()
	payload, err := event.Payload.AsDispatchResponseEventPayload()
	if err != nil {
		t.Fatal(err)
	}
	if payload.Outcome != factoryapi.WorkOutcomeCanceled || payload.Cancellation == nil || payload.Cancellation.Reason != factoryapi.DispatchCancellationReasonCANCELED {
		t.Fatalf("force canonical response = %#v, want explicit cancellation", payload)
	}
	if payload.Output != nil && *payload.Output != "" || payload.StructuredResult != nil ||
		payload.Feedback != nil && *payload.Feedback != "" ||
		payload.SelectedClassificationLabel != nil && *payload.SelectedClassificationLabel != "" || payload.FailureDetail != nil || payload.ProviderFailure != nil {
		t.Fatalf("force cancellation retained routing or failure authority: %#v", payload)
	}
}

func assertForceRestoredInput(t *testing.T, request, response factoryapi.FactoryEvent) {
	t.Helper()
	input, err := request.Payload.AsDispatchRequestEventPayload()
	if err != nil {
		t.Fatal(err)
	}
	output, err := response.Payload.AsDispatchResponseEventPayload()
	if err != nil {
		t.Fatal(err)
	}
	// Confirmed force preserves the original Work at the authored FAILED
	// placement, without provider partial output or replacement IDs.
	if len(input.Inputs) != 1 || output.OutputWork == nil || len(*output.OutputWork) != 1 {
		t.Fatalf("force did not restore its one consumed input: input=%#v output=%#v", input, output)
	}
	restored := (*output.OutputWork)[0]
	if restored.State == nil || restored.State.Name != "failed" || restored.State.Type != factoryapi.WorkStateTypeFAILED {
		t.Fatalf("forced Work placement = %#v, want authored FAILED", restored.State)
	}
	if restored.WorkId == nil || *restored.WorkId != input.Inputs[0].WorkId || restored.FailureDetail != nil || restored.Content == nil || len(*restored.Content) != 1 {
		t.Fatalf("force changed restored Work identity/content: %#v", restored)
	}
	text, err := (*restored.Content)[0].AsWorkTextContentPart()
	if err != nil || text.Text != "hold Factory force worker" {
		t.Fatalf("force changed restored Work text: %#v, %v", text, err)
	}
}

func assertForeignForceRefused(t *testing.T, process support.Process, source *support.FunctionalAPIServer, runner *forceHostRunner, done <-chan struct{}) {
	t.Helper()
	dir := support.ScaffoldSingleStepFactory(t, "mcp-force-foreign-profile")
	foreign := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir, WaitForServiceModeRuntime: true,
		Edges: serviceedges.Edges{ProviderCommandRunner: runner, FactorySessionsWorkingDirectory: historyWorkingDirectory(dir)},
	})
	session, ctx := startMCP(t, process, foreign.URL())
	before := getHost(t, source.URL()+"/worker-sessions/force-sibling").(map[string]any)
	attempt := before["attemptId"].(string)
	for _, id := range []string{"force-sibling", "unknown-force-worker"} {
		payload := map[string]any{"force": true, "requestId": "kill-" + id, "expectedAttemptId": attempt}
		result := postHostJSON(t, ctx, foreign.URL()+"/worker-sessions/"+id+"/terminate", payload, http.StatusNotFound).(map[string]any)
		if result["code"] != "NOT_FOUND" {
			t.Fatalf("foreign/unknown force error = %v", result)
		}
		assertForceCLIErrorCode(t, foreign, id, "NOT_FOUND", "--force", "--request-id", "kill-"+id, "--expected-attempt-id", attempt)
		args := map[string]any{"operation": "KILL", "workerSessionId": id, "requestId": "kill-" + id, "expectedAttemptId": attempt}
		assertToolError(t, callAction(t, ctx, session, "CONTROL", args), "worker_session.not_found", false)
	}
	after := getHost(t, source.URL()+"/worker-sessions/force-sibling").(map[string]any)
	for _, field := range []string{"workerSessionId", "attemptId", "state", "terminalCause", "endedAt"} {
		assertJSONEqual(t, before[field], after[field])
	}
	assertForceStillActive(t, source, "force-sibling", done)
	if runner.calls.Load() != 0 || runner.signals.Load() != 0 {
		t.Fatal("foreign/unknown force reached the source execution")
	}
}

func assertForceEventHistory(t *testing.T, events []factoryapi.FactoryEvent, dispatchID string, response factoryapi.FactoryEvent, wantRequests int) {
	t.Helper()
	requests, responses := 0, 0
	for _, event := range events {
		if event.Context.DispatchId == nil || *event.Context.DispatchId != dispatchID {
			continue
		}
		switch event.Type {
		case factoryapi.FactoryEventTypeDispatchRequest:
			requests++
		case factoryapi.FactoryEventTypeDispatchResponse:
			responses++
			assertJSONEqual(t, response, event)
			assertCanceledForcePayload(t, event)
		}
	}
	if requests != wantRequests || responses != 1 {
		t.Fatalf("force history requests=%d responses=%d, want %d/1", requests, responses, wantRequests)
	}
}

type completedForceControl struct {
	id, attempt string
	result      any
}

func (saved completedForceControl) assertRecovered(t *testing.T, host *support.FunctionalAPIServer, session *mcp.ClientSession, ctx context.Context) {
	t.Helper()
	assertForceObservation(t, host, saved.id)
	endpoint := host.URL() + "/worker-sessions/" + saved.id + "/terminate"
	payload := map[string]any{"force": true, "requestId": "kill-" + saved.id, "expectedAttemptId": saved.attempt}
	args := map[string]any{"operation": "KILL", "workerSessionId": saved.id, "requestId": "kill-" + saved.id, "expectedAttemptId": saved.attempt}
	assertJSONEqual(t, saved.result, postHostJSON(t, ctx, endpoint, payload, http.StatusOK))
	assertJSONEqual(t, saved.result, executeForceCLI(t, host, saved.id, saved.attempt))
	assertJSONEqual(t, saved.result, callWorker(t, ctx, session, "control", args)["result"])
	assertForceObservation(t, host, saved.id)
	// A committed outcome grants read-only authority for the original tuple.
	// It cannot validate a changed tuple or create a fresh effect on an archive.
	payload["expectedAttemptId"] = "changed-attempt"
	postHostJSON(t, ctx, endpoint, payload, http.StatusConflict)
	args["expectedAttemptId"] = "changed-attempt"
	assertToolError(t, callAction(t, ctx, session, "CONTROL", args), "worker_session.conflict", false)
	assertForceCLIErrorCode(t, host, saved.id, "WORKER_SESSION_CONTROL_CONFLICT",
		"--force", "--request-id", "kill-"+saved.id, "--expected-attempt-id", "changed-attempt")
	payload["expectedAttemptId"], payload["requestId"] = saved.attempt, "fresh-"+saved.id
	postHostJSON(t, ctx, endpoint, payload, http.StatusNotFound)
	args["expectedAttemptId"], args["requestId"] = saved.attempt, "fresh-"+saved.id
	assertToolError(t, callAction(t, ctx, session, "CONTROL", args), "worker_session.not_found", false)
	assertForceCLIErrorCode(t, host, saved.id, "NOT_FOUND",
		"--force", "--request-id", "fresh-"+saved.id, "--expected-attempt-id", saved.attempt)
}

func executeForceCLI(t *testing.T, host *support.FunctionalAPIServer, id, attempt string) any {
	t.Helper()
	inputs := support.FakeInputs(t.Context(), []string{"you", "worker-sessions", "terminate", id,
		"--force", "--request-id", "kill-" + id, "--expected-attempt-id", attempt, "--server", host.URL(), "--json"})
	if err := host.Execute(t, inputs.Input); err != nil {
		t.Fatalf("CLI force: %v stderr=%s", err, inputs.Stderr())
	}
	var result any
	if err := json.Unmarshal([]byte(inputs.Stdout()), &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func assertForceObservation(t *testing.T, host *support.FunctionalAPIServer, id string) {
	t.Helper()
	observation := getHost(t, host.URL()+"/worker-sessions/"+id).(map[string]any)
	if observation["state"] != "TERMINATED" || observation["terminalCause"] != "OPERATOR_KILL" || observation["providerSessionAvailable"] != false {
		t.Fatalf("joined force observation = %v", observation)
	}
}

type forceFailureFixture struct {
	host    *support.FunctionalAPIServer
	session *mcp.ClientSession
	ctx     context.Context
	runner  *forceHostRunner
}

// Each immutable failure edge owns its host/profile. The three transports
// reuse that host and replay each intent. This controls only command effects
// and does not prove any OS-tree boundary.
func runForceHostFailure(t *testing.T, process support.Process, mode string) {
	t.Helper()
	dir := support.ScaffoldSingleStepFactory(t, "mcp-force-"+mode)
	support.WriteAgentConfig(t, dir, "processor", support.BuildModelWorkerConfig(models.ProviderCodex, "test-model"))
	runner := &forceHostRunner{started: make(chan (<-chan struct{}), 4), mode: mode}
	host := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir, WaitForServiceModeRuntime: true,
		Edges: serviceedges.Edges{ProviderCommandRunner: runner, FactorySessionsWorkingDirectory: historyWorkingDirectory(dir)},
	})
	session, ctx := startMCP(t, process, host.URL())
	f := forceFailureFixture{host: host, session: session, ctx: ctx, runner: runner}
	sibling := admitControlWorker(t, ctx, host.URL(), "failure-sibling", controlHostRunner{started: runner.started})
	for index, transport := range []string{"http", "cli", "mcp"} {
		f.check(t, index, transport)
		assertForceStillActive(t, host, "failure-sibling", sibling)
	}
	callWorker(t, ctx, session, "control", map[string]any{"workerSessionId": "failure-sibling", "operation": "TERMINATE"})
	waitControlSignal(t, sibling)
}

func (f forceFailureFixture) check(t *testing.T, index int, transport string) {
	t.Helper()
	id := "failure-" + transport
	done := admitControlWorker(t, f.ctx, f.host.URL(), id, controlHostRunner{started: f.runner.started})
	before := getHost(t, f.host.URL()+"/worker-sessions/"+id).(map[string]any)
	attempt := before["attemptId"].(string)
	// Rotate the first transport so each admits a fresh operation, then recover
	// that same operation through the others, with no additional command effect.
	for offset := range 3 {
		selected := []string{"http", "cli", "mcp"}[(index+offset)%3]
		f.control(t, selected, id, attempt, before["state"])
	}
	assertForceStillActive(t, f.host, id, done)
	wantCalls := int32(index + 1)
	if f.runner.mode == "unattached" {
		wantCalls = 0
	}
	if f.runner.calls.Load() != wantCalls || f.runner.signals.Load() != 0 {
		t.Fatalf("force calls=%d signals=%d; want calls=%d signals=0", f.runner.calls.Load(), f.runner.signals.Load(), wantCalls)
	}
	// Unconfirmed force must leave ordinary safety stopping usable.
	ordinary := callWorker(t, f.ctx, f.session, "control", map[string]any{"workerSessionId": id, "operation": "TERMINATE"})["result"].(map[string]any)
	assertOwnedControl(t, ordinary, id, "TERMINATE", "APPLIED")
	if _, present := ordinary["forced"]; present {
		t.Fatalf("ordinary terminate acquired force semantics: %v", ordinary)
	}
	waitControlSignal(t, done)
	observation := getHost(t, f.host.URL()+"/worker-sessions/"+id).(map[string]any)
	if observation["terminalCause"] == "OPERATOR_KILL" {
		t.Fatalf("unconfirmed force invented kill causation: %v", observation)
	}
	// Terminality cannot rewrite a committed unsupported/failed operation or
	// confer fresh effect authority on a retry of that operation.
	for _, selected := range []string{"http", "cli", "mcp"} {
		f.control(t, selected, id, attempt, before["state"])
	}
	if f.runner.calls.Load() != wantCalls || f.runner.signals.Load() != 0 {
		t.Fatal("terminal recovery repeated an unconfirmed force effect")
	}
}

func (f forceFailureFixture) control(t *testing.T, transport, id, attempt string, state any) {
	t.Helper()
	failed := f.runner.mode == "failed"
	args := map[string]any{"operation": "KILL", "workerSessionId": id, "requestId": "kill-" + id, "expectedAttemptId": attempt}
	var result map[string]any
	switch transport {
	case "http":
		status := http.StatusOK
		if failed {
			status = http.StatusServiceUnavailable
		}
		payload := map[string]any{"force": true, "requestId": "kill-" + id, "expectedAttemptId": attempt}
		result = postHostJSON(t, f.ctx, f.host.URL()+"/worker-sessions/"+id+"/terminate", payload, status).(map[string]any)
		if failed {
			if result["code"] != "WORKER_SESSION_CONTROL_FAILED" {
				t.Fatalf("force HTTP failure = %v", result)
			}
			assertNoForceSecret(t, result)
			return
		}
	case "cli":
		if failed {
			assertForceCLIError(t, f.host, id, attempt)
			return
		}
		result = executeForceCLI(t, f.host, id, attempt).(map[string]any)
	case "mcp":
		if failed {
			failure := callAction(t, f.ctx, f.session, "CONTROL", args)
			assertToolError(t, failure, "worker_session.unavailable", true)
			return
		}
		result = callWorker(t, f.ctx, f.session, "control", args)["result"].(map[string]any)
	}
	assertOwnedControl(t, result, id, "TERMINATE", "UNSUPPORTED")
	if result["forced"] != true || result["state"] != state {
		t.Fatalf("unsupported force changed state or omitted force facts: %v", result)
	}
}

func assertForceCLIError(t *testing.T, host *support.FunctionalAPIServer, id, attempt string) {
	t.Helper()
	assertForceCLIErrorCode(t, host, id, "WORKER_SESSION_CONTROL_FAILED",
		"--force", "--request-id", "kill-"+id, "--expected-attempt-id", attempt)
}

func assertForceCLIErrorCode(t *testing.T, host *support.FunctionalAPIServer, id, code string, flags ...string) {
	t.Helper()
	argv := append([]string{"you", "worker-sessions", "terminate", id, "--server", host.URL(), "--json"}, flags...)
	inputs := support.FakeInputs(t.Context(), argv)
	err := host.Execute(t, inputs.Input)
	var typed *workersessionscli.CLIError
	if !errors.As(err, &typed) || typed.Code != code {
		t.Fatalf("CLI force failure = %v stdout=%s stderr=%s", err, inputs.Stdout(), inputs.Stderr())
	}
	assertNoForceSecret(t, []any{err.Error(), inputs.Stdout(), inputs.Stderr()})
}

func assertForceInputsRejected(t *testing.T, host *support.FunctionalAPIServer, session *mcp.ClientSession, ctx context.Context, id, endpoint string) {
	t.Helper()
	postHostJSON(t, ctx, endpoint, map[string]any{"force": true, "requestId": "stale-" + id, "expectedAttemptId": "stale"}, http.StatusConflict)
	assertForceCLIErrorCode(t, host, id, "WORKER_SESSION_CONTROL_CONFLICT",
		"--force", "--request-id", "stale-"+id, "--expected-attempt-id", "stale")
	stale := map[string]any{"operation": "KILL", "workerSessionId": id, "requestId": "stale-" + id, "expectedAttemptId": "stale"}
	assertToolError(t, callAction(t, ctx, session, "CONTROL", stale), "worker_session.conflict", false)
	postHostJSON(t, ctx, endpoint, map[string]any{"force": true, "requestId": "invalid-" + id}, http.StatusBadRequest)
	assertForceCLIErrorCode(t, host, id, "WORKER_SESSION_CONTROL_INVALID", "--request-id", "invalid-"+id)
	invalid := map[string]any{"operation": "KILL", "workerSessionId": id, "expectedAttemptId": "stale"}
	assertToolError(t, callAction(t, ctx, session, "CONTROL", invalid), "worker_session.invalid_request", false)
}

func assertNoForceSecret(t *testing.T, result any) {
	t.Helper()
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "secret-from-host") {
		t.Fatalf("force error leaked command details: %s", encoded)
	}
}

func assertForceStillActive(t *testing.T, host *support.FunctionalAPIServer, id string, done <-chan struct{}) {
	t.Helper()
	observation := getHost(t, host.URL()+"/worker-sessions/"+id).(map[string]any)
	if observation["state"] != "RUNNING" || observation["terminalCause"] == "OPERATOR_KILL" {
		t.Fatalf("unconfirmed force changed live observation: %v", observation)
	}
	select {
	case <-done:
		t.Fatal("unconfirmed force stopped an execution")
	default:
	}
}

func runDetachedForceControl(t *testing.T, process support.Process) {
	t.Helper()
	dir := support.ScaffoldSingleStepFactory(t, "mcp-force-detached")
	support.WriteAgentConfig(t, dir, "processor", support.BuildModelWorkerConfig(models.ProviderCodex, "test-model"))
	release := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	// Release the effect before host cleanup even when an assertion aborts.
	defer unblock()
	runner := &forceHostRunner{started: make(chan (<-chan struct{}), 1), entered: make(chan struct{}), release: release}
	host := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir, WaitForServiceModeRuntime: true,
		Edges: serviceedges.Edges{ProviderCommandRunner: runner, FactorySessionsWorkingDirectory: historyWorkingDirectory(dir)},
	})
	session, ctx := startMCP(t, process, host.URL())
	id := "detached-force"
	done := admitControlWorker(t, ctx, host.URL(), id, controlHostRunner{started: runner.started})
	before := getHost(t, host.URL()+"/worker-sessions/"+id).(map[string]any)
	attempt := before["attemptId"].(string)
	payload := map[string]any{"force": true, "requestId": "kill-" + id, "expectedAttemptId": attempt}
	endpoint := host.URL() + "/worker-sessions/" + id + "/terminate"
	cancel, response := beginForceHTTPRequest(t, ctx, endpoint, payload)
	defer cancel()
	waitControlSignal(t, runner.entered)
	// The owned effect has entered but cannot complete before the explicit
	// release. A successful HTTP response here would promise false terminality.
	select {
	case err := <-response:
		t.Fatalf("force returned before owned completion: %v", err)
	default:
	}
	cancel()
	select {
	case err := <-response:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("force observer disconnect = %v", err)
		}
	case <-ctx.Done():
		t.Fatal("force HTTP observer did not detach")
	}
	assertForceStillActive(t, host, id, done)
	unblock()
	args := map[string]any{"operation": "KILL", "workerSessionId": id, "requestId": "kill-" + id, "expectedAttemptId": attempt}
	result := callWorker(t, ctx, session, "control", args)["result"].(map[string]any)
	assertOwnedControl(t, result, id, "TERMINATE", "APPLIED")
	if result["forced"] != true || result["state"] != "TERMINATED" {
		t.Fatalf("detached force replay lost joined facts: %v", result)
	}
	waitControlSignal(t, done)
	assertJSONEqual(t, result, executeForceCLI(t, host, id, attempt))
	assertJSONEqual(t, result, postHostJSON(t, ctx, endpoint, payload, http.StatusOK))
	assertForceObservation(t, host, id)
	if runner.calls.Load() != 1 || runner.signals.Load() != 1 {
		t.Fatalf("detached/replayed force calls=%d signals=%d, want one", runner.calls.Load(), runner.signals.Load())
	}
}

// HTTP cancellation models observer disconnect without terminating the host.
// The goroutine returns errors through a channel and owns its response body.
func beginForceHTTPRequest(t *testing.T, ctx context.Context, endpoint string, payload any) (context.CancelFunc, <-chan error) {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	observer, cancel := context.WithCancel(ctx)
	request, err := http.NewRequestWithContext(observer, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response := make(chan error, 1)
	go func() {
		result, requestErr := http.DefaultClient.Do(request)
		if result != nil {
			_ = result.Body.Close()
		}
		response <- requestErr
	}()
	return cancel, response
}

// Preserve the real store's bounded read and activation capabilities while
// keeping this decorator's controlled write/activity faults.
func (store *forcePersistenceStore) LookupWorkerSessionSummary(ctx context.Context, id string) (recordings.WorkerCapturedSummary, error) {
	return store.WorkerRecordingStore.(recordings.WorkerCapturedSummaryReader).LookupWorkerSessionSummary(ctx, id)
}

func (store *forcePersistenceStore) RecoverWorkerOwners(ctx context.Context) error {
	return store.WorkerRecordingStore.(interface{ RecoverWorkerOwners(context.Context) error }).RecoverWorkerOwners(ctx)
}
