package acceptance

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/internal/functionalevidence"
)

type directWorkerSessionCLIResult struct {
	RequestID                string `json:"requestId"`
	WorkerSessionID          string `json:"workerSessionId"`
	SourceWorkerSessionID    string `json:"sourceWorkerSessionId"`
	SuccessorWorkerSessionID string `json:"successorWorkerSessionId"`
	Accepted                 bool   `json:"accepted"`
	State                    string `json:"state"`
	Output                   string `json:"output"`
}

type directWorkerSessionInterruptCLIResult struct {
	RequestID                string `json:"requestId"`
	SourceWorkerSessionID    string `json:"sourceWorkerSessionId"`
	SuccessorWorkerSessionID string `json:"successorWorkerSessionId"`
	Phase                    string `json:"phase"`
	Accepted                 bool   `json:"accepted"`
	Source                   struct {
		WorkerSessionID string `json:"workerSessionId"`
		State           string `json:"state"`
		EventTopic      string `json:"eventTopic"`
	} `json:"source"`
	Successor struct {
		WorkerSessionID string `json:"workerSessionId"`
		State           string `json:"state"`
		EventTopic      string `json:"eventTopic"`
	} `json:"successor"`
}

func TestInvokeContinueSharedProcess(t *testing.T) {
	fixture := ensureInvokeContinuePackageFixture(t)
	tests := []struct {
		name string
		run  func(*testing.T, *invokeContinuePackageFixture)
	}{
		{"InvokeContinueLocal", runDirectWorkerSessionInvokeContinueLocal},
		{"InvokeExecutionFileFutureFields", runDirectWorkerSessionInvokeExecutionFileToleratesFutureFields},
		{"ResumeRecordedProviderSession", runWSRFT015DirectWorkerSessionResumeUsesExactRecordedProviderSession},
		{"UnsupportedProviderContinuation", runDirectWorkerSessionContinueUnsupportedProvider},
		{"RemoteInterrupt", runDirectWorkerSessionRemoteInterruptUsesExactRouteAndAdmissionSnapshots},
		{"RemoteControls", runDirectWorkerSessionRemoteControlsUseExactRoutesWithoutFallback},
		{"ContinueUnknownSource", runDirectWorkerSessionContinueUnknownSourceReturnsNotFoundWithoutProviderCall},
		{"ContinueUnassociatedSource", runDirectWorkerSessionContinueUnassociatedSourceRejectsWithoutProviderContinuation},
		{"ContinueStaleProviderSession", runDirectWorkerSessionContinueStaleProviderSessionDoesNotFreshStart},
		{"RemoteContinueProviderFailures", runDirectWorkerSessionRemoteContinueProviderFailuresDoNotFallback},
		{"RemoteInvokeStreamFailure", runDirectWorkerSessionRemoteInvokeStreamSourceFailureThroughRootProcess},
		{"RemoteInvokeCancellation", runDirectWorkerSessionRemoteInvokeCallerCancellationThroughRootProcess},
		{"EmptyUserMessage", runDirectWorkerSessionEmptyUserMessageRejectsWithoutProviderCall},
		{"ExactDuplicateContinuation", runDirectWorkerSessionExactDuplicateContinuationIsIdempotent},
		{"DependencyCancellationRecovery", runDirectWorkerSessionDependencyCancellationCancelsOnceAndRecovers},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) { test.run(t, fixture) })
	}
	fixture.assertSpine(t)
	functionalevidence.Covers(t,
		"cli/you.worker-sessions.continue",
		"cli/you.worker-sessions.invoke",
		"cli/you.worker-sessions.interrupt",
		"cli/you.worker-sessions.cancel",
		"cli/you.worker-sessions.pause",
		"cli/you.worker-sessions.resume",
		"cli/you.worker-sessions.terminate",
	)
}

func runDirectWorkerSessionInvokeContinueLocal(t *testing.T, fixture *invokeContinuePackageFixture) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	scenario := fixture.scenario(t, "local")
	runner := scenario.providerRunner
	process := fixture.process
	if got := runner.CallCount(); got != 0 {
		t.Fatalf("provider command runner calls before CASE-01 = %d, want zero", got)
	}
	executionPath := filepath.Join(fixture.rootDir, "local-invoke-execution.json")
	writeInvokeContinueExecution(t, executionPath, scenario.session.id, scenario.workingDirectory)

	invoke := support.FakeInputs(ctx, []string{"you", "--json", "worker-sessions", "invoke", "--execution", executionPath})
	invoke.Input.Env = invokeContinueEnvironment(fixture.homeDir)
	invoke.Input.WorkingDirectory = scenario.workingDirectory
	if err := process.Execute(invoke.Input); err != nil {
		t.Fatalf("local invoke: %v\nrequests:%#v\nstdout:\n%s\nstderr:\n%s", err, runner.Requests(), invoke.Stdout(), invoke.Stderr())
	}
	var invoked directWorkerSessionCLIResult
	decodeDirectWorkerSessionResult(t, invoke.Stdout(), &invoked)
	if !invoked.Accepted || invoked.RequestID != "local-invoke-request" || invoked.WorkerSessionID != "local-source-session" || invoked.State != "COMPLETED" {
		t.Fatalf("local invoke result = %#v, want accepted completed source", invoked)
	}
	if !strings.Contains(invoked.Output, "initial direct output COMPLETE") {
		t.Fatalf("local invoke output = %q, want provider output; calls=%d requests=%#v\nstdout:\n%s\nstderr:\n%s", invoked.Output, runner.CallCount(), runner.Requests(), invoke.Stdout(), invoke.Stderr())
	}

	cont := support.FakeInputs(ctx, []string{
		"you", "--json", "worker-sessions", "continue", "local-source-session",
		"--request-id", "local-continue-request", "--successor-worker-session-id", "local-successor-session",
		"--user-message", "continued direct prompt",
	})
	cont.Input.Env = scenario.environment()
	cont.Input.WorkingDirectory = scenario.workingDirectory
	if err := process.Execute(cont.Input); err != nil {
		t.Fatalf("local continuation: %v\nrequests:%#v\nstdout:\n%s\nstderr:\n%s", err, runner.Requests(), cont.Stdout(), cont.Stderr())
	}
	var continued directWorkerSessionCLIResult
	decodeDirectWorkerSessionResult(t, cont.Stdout(), &continued)
	if !continued.Accepted || continued.RequestID != "local-continue-request" || continued.SourceWorkerSessionID != "local-source-session" || continued.SuccessorWorkerSessionID != "local-successor-session" || continued.State != "COMPLETED" {
		t.Fatalf("local continuation result = %#v, want accepted successor lineage", continued)
	}
	if !strings.Contains(continued.Output, "continued direct output COMPLETE") {
		t.Fatalf("local continuation output = %q, want provider output", continued.Output)
	}

	requests := runner.Requests()
	if len(requests) != 2 {
		t.Fatalf("provider command requests = %d, want initial and continuation only", len(requests))
	}
	if strings.Contains(strings.Join(requests[0].Args, " "), "resume") {
		t.Fatalf("initial provider command unexpectedly resumed a session: %#v", requests[0].Args)
	}
	continuationArgs := strings.Join(requests[1].Args, " ")
	if !strings.Contains(continuationArgs, "resume") || !strings.Contains(continuationArgs, "local-source-thread") {
		t.Fatalf("continuation provider command = %#v, want resume local-source-thread", requests[1].Args)
	}

	conflict := support.FakeInputs(ctx, []string{
		"you", "--json", "worker-sessions", "continue", "local-source-session",
		"--request-id", "local-continue-request", "--successor-worker-session-id", "different-successor",
		"--user-message", "different immutable input", "--async",
	})
	conflict.Input.Env = scenario.environment()
	conflict.Input.WorkingDirectory = scenario.workingDirectory
	if err := process.Execute(conflict.Input); err == nil {
		t.Fatal("continuation request-id reuse succeeded, want conflict")
	}
	assertDirectWorkerSessionCLIError(t, conflict, string(factoryapi.ErrorResponseCodeWORKERSESSIONCONTINUATIONREQUESTIDCONFLICT))
	if requests := runner.Requests(); len(requests) != 2 {
		t.Fatalf("provider command requests after idempotency conflict = %d, want two", len(requests))
	}
	assertLocalTerminalWorkerSessionControls(t, ctx, process, scenario.environment(), scenario.workingDirectory)
	scenario.close(t)
}

func runDirectWorkerSessionInvokeExecutionFileToleratesFutureFields(t *testing.T, fixture *invokeContinuePackageFixture) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	scenario := fixture.scenario(t, "future-fields")
	document := invokeContinueExecutionDocument(invokeContinueExecutionSpec{
		requestID: "future-file-request", workerSessionID: "future-file-session", dispatchID: "future-file-dispatch",
		factorySessionID: scenario.session.id, workingDirectory: scenario.workingDirectory, userMessage: "future-file prompt",
	})
	document["futureTopLevel"] = "top-secret"
	execution := document["execution"].(map[string]any)
	execution["futureExecution"] = map[string]any{"value": "execution-secret"}
	execution["dispatch"].(map[string]any)["futureDispatch"] = "dispatch-secret"
	executionPath := filepath.Join(fixture.rootDir, "future-execution.json")
	writeInvokeContinueJSON(t, executionPath, document)

	inputs := support.FakeInputs(ctx, []string{"you", "--json", "worker-sessions", "invoke", "--execution", executionPath})
	inputs.Input.Env = scenario.environment()
	inputs.Input.WorkingDirectory = scenario.workingDirectory
	if err := fixture.process.Execute(inputs.Input); err != nil {
		t.Fatalf("future direct Worker execution: %v\nstdout:\n%s\nstderr:\n%s", err, inputs.Stdout(), inputs.Stderr())
	}
	var result directWorkerSessionCLIResult
	decodeDirectWorkerSessionResult(t, inputs.Stdout(), &result)
	if !result.Accepted || result.RequestID != "future-file-request" || result.WorkerSessionID != "future-file-session" || result.State != "COMPLETED" || !strings.Contains(result.Output, "future-file output COMPLETE") {
		t.Fatalf("future direct Worker result = %#v, want successful known execution", result)
	}
	wantWarning := "warning: ignored unknown direct Worker execution fields at $.execution.dispatch.futureDispatch, $.execution.futureExecution, $.futureTopLevel"
	if !strings.Contains(inputs.Stderr(), wantWarning) {
		t.Fatalf("stderr = %q, want sorted compatibility warning %q", inputs.Stderr(), wantWarning)
	}
	if strings.Contains(inputs.Stderr(), "secret") {
		t.Fatalf("stderr compatibility warning leaked ignored field values: %q", inputs.Stderr())
	}
	if strings.TrimSpace(inputs.Stdout()) == "" || strings.Contains(inputs.Stdout(), "futureTopLevel") {
		t.Fatalf("stdout = %q, want normal structured result without ignored fields", inputs.Stdout())
	}
	scenario.close(t)
}

// WSR-FT-015 proves the shared root-built process admits a paused Worker
// Session through the exact recorded Provider Session and refuses an unknown
// resume without another provider command side effect.
func runWSRFT015DirectWorkerSessionResumeUsesExactRecordedProviderSession(t *testing.T, fixture *invokeContinuePackageFixture) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	scenario := fixture.scenario(t, "recorded-provider-session")
	runner := scenario.streamingRunner
	executionPath := filepath.Join(fixture.rootDir, "wsr-015-execution.json")
	writeInvokeContinueExecutionSpec(t, executionPath, invokeContinueExecutionSpec{
		requestID: "wsr-015-invoke-request", workerSessionID: "wsr-015-session", dispatchID: "wsr-015-dispatch",
		factorySessionID: scenario.session.id, workingDirectory: scenario.workingDirectory, userMessage: "hold the recorded provider session",
	})
	invoke := support.FakeInputs(ctx, []string{"you", "--json", "worker-sessions", "invoke", "--execution", executionPath})
	invoke.Input.Env = scenario.environment()
	invoke.Input.WorkingDirectory = scenario.workingDirectory
	invokeDone := make(chan error, 1)
	go func() { invokeDone <- fixture.process.Execute(invoke.Input) }()
	select {
	case <-runner.initialSessionObserved:
	case <-ctx.Done():
		t.Fatalf("WSR-FT-015 initial Provider Session was not observed: %v", ctx.Err())
	}

	pause := support.FakeInputs(ctx, []string{"you", "--json", "worker-sessions", "pause", "wsr-015-session"})
	pause.Input.Env = scenario.environment()
	pause.Input.WorkingDirectory = scenario.workingDirectory
	if err := fixture.process.Execute(pause.Input); err != nil {
		t.Fatalf("WSR-FT-015 pause: %v\nstdout:%s\nstderr:%s", err, pause.Stdout(), pause.Stderr())
	}
	var paused struct {
		Outcome string `json:"outcome"`
		State   string `json:"state"`
	}
	if err := json.Unmarshal([]byte(pause.Stdout()), &paused); err != nil {
		t.Fatalf("decode WSR-FT-015 pause: %v; stdout=%s", err, pause.Stdout())
	}
	if paused.Outcome != "APPLIED" || paused.State != "PAUSED" {
		t.Fatalf("WSR-FT-015 pause result = %#v, want APPLIED/PAUSED", paused)
	}

	resume := support.FakeInputs(ctx, []string{"you", "--json", "worker-sessions", "resume", "wsr-015-session"})
	resume.Input.Env = scenario.environment()
	resume.Input.WorkingDirectory = scenario.workingDirectory
	if err := fixture.process.Execute(resume.Input); err != nil {
		t.Fatalf("WSR-FT-015 resume: %v\nstdout:%s\nstderr:%s", err, resume.Stdout(), resume.Stderr())
	}
	var resumed struct {
		Outcome string `json:"outcome"`
		State   string `json:"state"`
	}
	if err := json.Unmarshal([]byte(resume.Stdout()), &resumed); err != nil {
		t.Fatalf("decode WSR-FT-015 resume: %v; stdout=%s", err, resume.Stdout())
	}
	if resumed.Outcome != "APPLIED" || (resumed.State != "RUNNING" && resumed.State != "COMPLETED") {
		t.Fatalf("WSR-FT-015 resume result = %#v, want APPLIED/RUNNING or COMPLETED", resumed)
	}
	select {
	case err := <-invokeDone:
		if err != nil {
			t.Fatalf("WSR-FT-015 invoke after resume: %v\nstdout:%s\nstderr:%s", err, invoke.Stdout(), invoke.Stderr())
		}
	case <-ctx.Done():
		t.Fatalf("WSR-FT-015 invoke did not finish after resume: %v", ctx.Err())
	}
	var invoked directWorkerSessionCLIResult
	decodeDirectWorkerSessionResult(t, invoke.Stdout(), &invoked)
	if !invoked.Accepted || invoked.State != "COMPLETED" || !strings.Contains(invoked.Output, "resumed exact output") {
		t.Fatalf("WSR-FT-015 final invoke result = %#v, want completed resumed output", invoked)
	}

	requests := runner.Requests()
	if len(requests) != 2 {
		t.Fatalf("WSR-FT-015 provider command requests = %d, want initial plus exact resume", len(requests))
	}
	if strings.Contains(strings.Join(requests[0].Args, " "), "resume") {
		t.Fatalf("WSR-FT-015 initial provider command unexpectedly resumed: %#v", requests[0].Args)
	}
	resumeArgs := strings.Join(requests[1].Args, " ")
	if !strings.Contains(resumeArgs, "resume") || !strings.Contains(resumeArgs, "wsr-015-recorded-thread") {
		t.Fatalf("WSR-FT-015 resume provider command = %#v, want exact recorded session", requests[1].Args)
	}
	refusal := support.FakeInputs(ctx, []string{"you", "--json", "worker-sessions", "resume", "wsr-015-unknown-session"})
	refusal.Input.Env = scenario.environment()
	refusal.Input.WorkingDirectory = scenario.workingDirectory
	if err := fixture.process.Execute(refusal.Input); err == nil {
		t.Fatal("WSR-FT-015 unknown resume succeeded, want NOT_FOUND without provider side effect")
	}
	assertDirectWorkerSessionCLIError(t, refusal, string(factoryapi.ErrorResponseCodeNOTFOUND))
	if runner.CallCount() != 2 {
		t.Fatalf("WSR-FT-015 provider command calls after refused resume = %d, want 2", runner.CallCount())
	}
	scenario.close(t)
}

type wsrFT015StreamingProviderRunner struct {
	initialSessionObserved chan struct{}
	initialOnce            sync.Once
	mu                     sync.Mutex
	requests               []platformprocess.CommandRequest
}

func newWSRFT015StreamingProviderRunner() *wsrFT015StreamingProviderRunner {
	return &wsrFT015StreamingProviderRunner{initialSessionObserved: make(chan struct{})}
}

func (runner *wsrFT015StreamingProviderRunner) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	return runner.RunStreaming(ctx, request, nil)
}

func (runner *wsrFT015StreamingProviderRunner) RunStreaming(ctx context.Context, request platformprocess.CommandRequest, observe platformprocess.OutputChunkObserver) (platformprocess.CommandResult, error) {
	runner.mu.Lock()
	runner.requests = append(runner.requests, platformprocess.CommandRequest{Command: request.Command, Args: append([]string(nil), request.Args...)})
	runner.mu.Unlock()
	args := strings.Join(request.Args, " ")
	if strings.Contains(args, "resume") {
		emitWSRFT015CodexOutput(observe, directCodexSessionOutput("wsr-015-recorded-thread", "resumed exact output"))
		return platformprocess.CommandResult{}, nil
	}
	initial := directCodexSessionOutput("wsr-015-recorded-thread", "initial output")
	if observe != nil {
		line := strings.SplitN(string(initial), "\n", 2)[0] + "\n"
		observe(platformprocess.OutputStreamStdout, []byte(line))
	}
	runner.initialOnce.Do(func() { close(runner.initialSessionObserved) })
	<-ctx.Done()
	return platformprocess.CommandResult{}, ctx.Err()
}

func (runner *wsrFT015StreamingProviderRunner) Requests() []platformprocess.CommandRequest {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	requests := make([]platformprocess.CommandRequest, len(runner.requests))
	for index, request := range runner.requests {
		requests[index] = platformprocess.CommandRequest{Command: request.Command, Args: append([]string(nil), request.Args...)}
	}
	return requests
}

func (runner *wsrFT015StreamingProviderRunner) CallCount() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return len(runner.requests)
}

func emitWSRFT015CodexOutput(observe platformprocess.OutputChunkObserver, output []byte) {
	if observe == nil {
		return
	}
	for _, line := range strings.Split(strings.TrimSuffix(string(output), "\n"), "\n") {
		observe(platformprocess.OutputStreamStdout, []byte(line+"\n"))
	}
}

var _ platformprocess.CommandRunner = (*wsrFT015StreamingProviderRunner)(nil)

func runDirectWorkerSessionContinueUnsupportedProvider(t *testing.T, fixture *invokeContinuePackageFixture) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	scenario := fixture.scenario(t, "unsupported-provider")
	executionPath := filepath.Join(fixture.rootDir, "unsupported-execution.json")
	document := invokeContinueExecutionDocument(invokeContinueExecutionSpec{
		requestID: "unsupported-invoke-request", workerSessionID: "unsupported-source", dispatchID: "unsupported-dispatch",
		factorySessionID: scenario.session.id, workingDirectory: scenario.workingDirectory, userMessage: "initial provider output",
	})
	delete(document["execution"].(map[string]any), "executorProvider")
	writeInvokeContinueJSON(t, executionPath, document)
	invoke := support.FakeInputs(ctx, []string{"you", "--json", "worker-sessions", "invoke", "--execution", executionPath})
	invoke.Input.Env = scenario.environment()
	invoke.Input.WorkingDirectory = scenario.workingDirectory
	if err := fixture.process.Execute(invoke.Input); err != nil {
		t.Fatalf("unsupported continuation source invoke: %v\nstdout:%s\nstderr:%s", err, invoke.Stdout(), invoke.Stderr())
	}
	continuation := support.FakeInputs(ctx, []string{
		"you", "--json", "worker-sessions", "continue", "unsupported-source", "--request-id", "unsupported-continue-request",
		"--successor-worker-session-id", "unsupported-successor", "--user-message", "must not fresh start",
	})
	continuation.Input.Env = scenario.environment()
	continuation.Input.WorkingDirectory = scenario.workingDirectory
	if err := fixture.process.Execute(continuation.Input); err == nil {
		t.Fatal("unsupported provider continuation succeeded, want one terminal failure")
	}
	assertDirectWorkerSessionCLIError(t, continuation, "WORKER_SESSION_FAILED")
	if got := scenario.providerRunner.CallCount(); got != 1 {
		t.Fatalf("provider command calls after unsupported continuation = %d, want initial call only", got)
	}
	requests := scenario.providerRunner.Requests()
	if len(requests) != 1 || strings.Contains(strings.Join(requests[0].Args, " "), "resume") {
		t.Fatalf("unsupported continuation provider requests = %#v, want one non-resume initial command", requests)
	}
	scenario.close(t)
}

func assertLocalTerminalWorkerSessionControls(t *testing.T, ctx context.Context, process support.Process, env []string, workingDirectory string) {
	t.Helper()
	for _, action := range []string{"pause", "resume", "cancel", "terminate"} {
		control := support.FakeInputs(ctx, []string{"you", "--json", "worker-sessions", action, "local-successor-session"})
		control.Input.Env = env
		control.Input.WorkingDirectory = workingDirectory
		if err := process.Execute(control.Input); err != nil {
			t.Fatalf("local Worker Session %s: %v\nstdout:\n%s\nstderr:\n%s", action, err, control.Stdout(), control.Stderr())
		}
		var result struct {
			Outcome string `json:"outcome"`
			State   string `json:"state"`
		}
		if err := json.Unmarshal([]byte(control.Stdout()), &result); err != nil {
			t.Fatalf("decode local Worker Session %s result: %v; stdout=%s", action, err, control.Stdout())
		}
		if result.Outcome != "NOOP" || result.State != "COMPLETED" {
			t.Fatalf("local Worker Session %s result = %#v, want terminal no-op", action, result)
		}
	}
}

func runDirectWorkerSessionRemoteInterruptUsesExactRouteAndAdmissionSnapshots(t *testing.T, fixture *invokeContinuePackageFixture) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	scenario := fixture.scenario(t, "remote-interrupt")
	var received factoryapi.WorkerSessionInterruptRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/worker-sessions/source-session/interrupt" {
			t.Fatalf("unexpected remote interrupt request %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode remote interrupt request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(factoryapi.WorkerSessionInterruptResponse{
			RequestId: "remote-interrupt-request", SourceWorkerSessionId: "source-session", SuccessorWorkerSessionId: "successor-session",
			Phase: factoryapi.WorkerSessionInterruptResponsePhaseSuccessorAdmission, Accepted: true,
			Source:    factoryapi.WorkerSessionInterruptSnapshot{WorkerSessionId: "source-session", State: factoryapi.WorkerSessionInterruptSnapshotStateCanceled, EventTopic: "worker-session/source-session/events"},
			Successor: factoryapi.WorkerSessionInterruptSnapshot{WorkerSessionId: "successor-session", State: factoryapi.WorkerSessionInterruptSnapshotStateRunning, EventTopic: "worker-session/successor-session/events"},
		})
	}))
	defer server.Close()
	inputs := support.FakeInputs(ctx, []string{
		"you", "--remote", "--server", server.URL, "--json", "worker-sessions", "interrupt", "source-session",
		"--request-id", "remote-interrupt-request", "--successor-worker-session-id", "successor-session",
		"--replacement-message", "replace the active work", "--async",
	})
	inputs.Input.Env = scenario.environment()
	inputs.Input.WorkingDirectory = scenario.workingDirectory
	if err := fixture.process.Execute(inputs.Input); err != nil {
		t.Fatalf("remote Worker Session interrupt: %v\nstdout:\n%s\nstderr:\n%s", err, inputs.Stdout(), inputs.Stderr())
	}
	var result directWorkerSessionInterruptCLIResult
	decodeDirectWorkerSessionResult(t, inputs.Stdout(), &result)
	if received.RequestId != "remote-interrupt-request" || received.SuccessorWorkerSessionId != "successor-session" || received.ReplacementMessage != "replace the active work" {
		t.Fatalf("remote interrupt request = %#v, want exact request tuple", received)
	}
	if !result.Accepted || result.Phase != string(factoryapi.WorkerSessionInterruptResponsePhaseSuccessorAdmission) || result.SourceWorkerSessionID != "source-session" || result.SuccessorWorkerSessionID != "successor-session" || result.Source.State != string(factoryapi.WorkerSessionInterruptSnapshotStateCanceled) || result.Successor.State != string(factoryapi.WorkerSessionInterruptSnapshotStateRunning) || result.Source.EventTopic == "" || result.Successor.EventTopic == "" {
		t.Fatalf("remote interrupt result = %#v, want admitted source/successor snapshots", result)
	}
	scenario.close(t)
}

func runDirectWorkerSessionRemoteControlsUseExactRoutesWithoutFallback(t *testing.T, fixture *invokeContinuePackageFixture) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	scenario := fixture.scenario(t, "remote-controls")
	controls := []struct {
		name   string
		action factoryapi.WorkerSessionControlResponseAction
		state  factoryapi.WorkerSessionControlResponseState
	}{
		{"pause", factoryapi.WorkerSessionControlResponseActionPause, factoryapi.WorkerSessionControlResponseStatePaused},
		{"resume", factoryapi.WorkerSessionControlResponseActionResume, factoryapi.WorkerSessionControlResponseStateRunning},
		{"cancel", factoryapi.WorkerSessionControlResponseActionCancel, factoryapi.WorkerSessionControlResponseStateCanceled},
		{"terminate", factoryapi.WorkerSessionControlResponseActionTerminate, factoryapi.WorkerSessionControlResponseStateTerminated},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("remote Worker Session control method = %q, want POST", r.Method)
			http.NotFound(w, r)
			return
		}
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		var matched *struct {
			action factoryapi.WorkerSessionControlResponseAction
			state  factoryapi.WorkerSessionControlResponseState
		}
		for index := range controls {
			if len(parts) == 3 && parts[0] == "worker-sessions" && parts[1] != "" && parts[2] == controls[index].name {
				matched = &struct {
					action factoryapi.WorkerSessionControlResponseAction
					state  factoryapi.WorkerSessionControlResponseState
				}{controls[index].action, controls[index].state}
				break
			}
		}
		if matched == nil {
			t.Errorf("remote Worker Session control path = %q, want known action route", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read remote Worker Session control body: %v", err)
		}
		if len(body) != 0 {
			t.Errorf("remote Worker Session control body = %q, want empty POST body", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(factoryapi.WorkerSessionControlResponse{
			WorkerSessionId: "remote-control-" + parts[2], Action: matched.action,
			Outcome: factoryapi.WorkerSessionControlResponseOutcomeApplied, State: matched.state,
			DispatchId: "remote-dispatch-" + parts[2],
		})
	}))
	defer server.Close()
	for _, control := range controls {
		control := control
		t.Run(control.name, func(t *testing.T) {
			workerSessionID := "remote-control-" + control.name
			inputs := support.FakeInputs(ctx, []string{"you", "--remote", "--server", server.URL, "--json", "worker-sessions", control.name, workerSessionID})
			inputs.Input.Env = scenario.environment()
			inputs.Input.WorkingDirectory = scenario.workingDirectory
			if err := fixture.process.Execute(inputs.Input); err != nil {
				t.Fatalf("remote Worker Session %s: %v\nstdout:%s\nstderr:%s", control.name, err, inputs.Stdout(), inputs.Stderr())
			}
			var result factoryapi.WorkerSessionControlResponse
			decodeDirectWorkerSessionResult(t, inputs.Stdout(), &result)
			if result.WorkerSessionId != workerSessionID || result.Action != control.action || result.Outcome != factoryapi.WorkerSessionControlResponseOutcomeApplied || result.State != control.state {
				t.Fatalf("remote Worker Session %s result = %#v, want exact typed response", control.name, result)
			}
		})
	}
	if got := scenario.providerRunner.CallCount(); got != 0 {
		t.Fatalf("remote Worker Session controls caused local provider fallback: %d calls", got)
	}
	scenario.close(t)
}

func runDirectWorkerSessionContinueUnknownSourceReturnsNotFoundWithoutProviderCall(t *testing.T, fixture *invokeContinuePackageFixture) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	scenario := fixture.scenario(t, "unknown-source")
	inputs := support.FakeInputs(ctx, []string{
		"you", "--json", "worker-sessions", "continue", "unknown-worker-session", "--request-id", "unknown-continue-request",
		"--successor-worker-session-id", "unknown-successor", "--user-message", "unknown source", "--async",
	})
	inputs.Input.Env = scenario.environment()
	inputs.Input.WorkingDirectory = scenario.workingDirectory
	if err := fixture.process.Execute(inputs.Input); err == nil {
		t.Fatal("unknown Worker Session continuation succeeded, want not found")
	}
	assertDirectWorkerSessionCLIError(t, inputs, string(factoryapi.ErrorResponseCodeNOTFOUND))
	if got := scenario.providerRunner.CallCount(); got != 0 {
		t.Fatalf("provider calls after unknown source = %d, want zero", got)
	}
	scenario.close(t)
}

func runDirectWorkerSessionContinueUnassociatedSourceRejectsWithoutProviderContinuation(t *testing.T, fixture *invokeContinuePackageFixture) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	scenario := fixture.scenario(t, "unassociated-source")
	executionPath := filepath.Join(fixture.rootDir, "unassociated-execution.json")
	writeInvokeContinueExecutionSpec(t, executionPath, invokeContinueExecutionSpec{
		requestID: "unassociated-invoke-request", workerSessionID: "unassociated-source", dispatchID: "unassociated-dispatch",
		factorySessionID: scenario.session.id, workingDirectory: scenario.workingDirectory, userMessage: "complete without a session",
	})
	invoke := support.FakeInputs(ctx, []string{"you", "--json", "worker-sessions", "invoke", "--execution", executionPath})
	invoke.Input.Env = scenario.environment()
	invoke.Input.WorkingDirectory = scenario.workingDirectory
	if err := fixture.process.Execute(invoke.Input); err != nil {
		t.Fatalf("unassociated source invoke: %v\nstdout:%s\nstderr:%s", err, invoke.Stdout(), invoke.Stderr())
	}
	continuation := support.FakeInputs(ctx, []string{
		"you", "--json", "worker-sessions", "continue", "unassociated-source", "--request-id", "unassociated-continue-request",
		"--successor-worker-session-id", "unassociated-successor", "--user-message", "must not resume", "--async",
	})
	continuation.Input.Env = scenario.environment()
	continuation.Input.WorkingDirectory = scenario.workingDirectory
	if err := fixture.process.Execute(continuation.Input); err == nil {
		t.Fatal("unassociated source continuation succeeded, want provider continuation invalid")
	}
	assertDirectWorkerSessionCLIError(t, continuation, "WORKER_SESSION_PROVIDER_CONTINUATION_INVALID")
	if got := scenario.providerRunner.CallCount(); got != 1 {
		t.Fatalf("provider calls after unassociated continuation = %d, want one initial call", got)
	}
	scenario.close(t)
}

func runDirectWorkerSessionContinueStaleProviderSessionDoesNotFreshStart(t *testing.T, fixture *invokeContinuePackageFixture) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	scenario := fixture.scenario(t, "stale-provider-session")
	executionPath := filepath.Join(fixture.rootDir, "stale-execution.json")
	writeInvokeContinueExecutionSpec(t, executionPath, invokeContinueExecutionSpec{
		requestID: "stale-invoke-request", workerSessionID: "stale-source", dispatchID: "stale-dispatch",
		factorySessionID: scenario.session.id, workingDirectory: scenario.workingDirectory, userMessage: "initial output",
	})
	invoke := support.FakeInputs(ctx, []string{"you", "--json", "worker-sessions", "invoke", "--execution", executionPath})
	invoke.Input.Env = scenario.environment()
	invoke.Input.WorkingDirectory = scenario.workingDirectory
	if err := fixture.process.Execute(invoke.Input); err != nil {
		t.Fatalf("stale source invoke: %v\nstdout:%s\nstderr:%s", err, invoke.Stdout(), invoke.Stderr())
	}
	continuation := support.FakeInputs(ctx, []string{
		"you", "--json", "worker-sessions", "continue", "stale-source", "--request-id", "stale-continue-request",
		"--successor-worker-session-id", "stale-successor", "--user-message", "resume stale session",
	})
	continuation.Input.Env = scenario.environment()
	continuation.Input.WorkingDirectory = scenario.workingDirectory
	if err := fixture.process.Execute(continuation.Input); err == nil {
		t.Fatal("stale Provider Session continuation succeeded, want terminal failure")
	}
	assertDirectWorkerSessionCLIError(t, continuation, "WORKER_SESSION_FAILED")
	requests := scenario.providerRunner.Requests()
	if len(requests) != 2 {
		t.Fatalf("provider calls after stale continuation = %d, want initial plus one exact continuation", len(requests))
	}
	continuationArgs := strings.Join(requests[1].Args, " ")
	if !strings.Contains(continuationArgs, "resume") || !strings.Contains(continuationArgs, "stale-source-thread") {
		t.Fatalf("stale continuation provider command = %#v, want exact resume identity and no fresh start", requests[1].Args)
	}
	scenario.close(t)
}

func runDirectWorkerSessionRemoteContinueProviderFailuresDoNotFallback(t *testing.T, fixture *invokeContinuePackageFixture) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	scenario := fixture.scenario(t, "remote-continue-failures")
	cases := []struct {
		name    string
		status  int
		code    string
		message string
	}{
		{"foreign-provider-session", http.StatusConflict, "WORKER_SESSION_PROVIDER_CONTINUATION_INVALID", "foreign Provider Session"},
		{"stale-provider-session", http.StatusConflict, "WORKER_SESSION_PROVIDER_CONTINUATION_INVALID", "stale Provider Session"},
		{"unsupported-continuation", http.StatusConflict, "WORKER_SESSION_PROVIDER_CONTINUATION_INVALID", "unsupported Provider Session continuation"},
		{"admission-failure", http.StatusServiceUnavailable, "WORKER_SESSION_CONTINUATION_ADMISSION_FAILED", "continuation admission failed"},
	}
	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/worker-sessions/source-session/continue" {
					http.NotFound(w, r)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(testCase.status)
				_ = json.NewEncoder(w).Encode(map[string]string{"code": testCase.code, "message": testCase.message})
			}))
			defer server.Close()
			inputs := support.FakeInputs(ctx, []string{
				"you", "--remote", "--server", server.URL, "--json", "worker-sessions", "continue", "source-session",
				"--request-id", "provider-failure-request", "--successor-worker-session-id", "provider-failure-successor",
				"--user-message", "provider failure", "--async",
			})
			inputs.Input.Env = scenario.environment()
			inputs.Input.WorkingDirectory = scenario.workingDirectory
			if err := fixture.process.Execute(inputs.Input); err == nil {
				t.Fatal("remote provider continuation failure succeeded, want typed error")
			}
			assertDirectWorkerSessionCLIError(t, inputs, testCase.code)
			if got := scenario.providerRunner.CallCount(); got != 0 {
				t.Fatalf("remote %s caused local provider fallback: %d calls", testCase.name, got)
			}
		})
	}
	scenario.close(t)
}

func runDirectWorkerSessionRemoteInvokeStreamSourceFailureThroughRootProcess(t *testing.T, fixture *invokeContinuePackageFixture) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	scenario := fixture.scenario(t, "remote-stream-failure")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost && r.URL.Path == "/worker-sessions" {
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(factoryapi.WorkerSessionStartResponse{RequestId: "remote-failure-request", WorkerSessionId: "remote-failure-session", Accepted: true, State: factoryapi.WorkerSessionStartResponseStateRunning})
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == "/worker-sessions/remote-failure-session/events" {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "data: {\"delivery\":\"SOURCE_FAILURE\",\"errorCode\":\"WORKER_SESSION_STREAM_SOURCE_FAILURE\",\"errorMessage\":\"deterministic source failure\"}\n\n")
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	executionPath := filepath.Join(fixture.rootDir, "remote-failure-execution.json")
	writeInvokeContinueExecutionSpec(t, executionPath, invokeContinueExecutionSpec{
		requestID: "remote-failure-request", workerSessionID: "remote-failure-session", dispatchID: "remote-failure-dispatch",
		factorySessionID: scenario.session.id, workingDirectory: scenario.workingDirectory, userMessage: "stream failure",
	})
	inputs := support.FakeInputs(ctx, []string{"you", "--remote", "--server", server.URL, "--json", "worker-sessions", "invoke", "--execution", executionPath})
	inputs.Input.Env = scenario.environment()
	inputs.Input.WorkingDirectory = scenario.workingDirectory
	if err := fixture.process.Execute(inputs.Input); err == nil {
		t.Fatal("remote stream source failure succeeded, want typed failure")
	}
	assertDirectWorkerSessionCLIError(t, inputs, "WORKER_SESSION_STREAM_SOURCE_FAILURE")
	if got := scenario.providerRunner.CallCount(); got != 0 {
		t.Fatalf("remote stream failure caused local provider fallback: %d calls", got)
	}
	scenario.close(t)
}

func runDirectWorkerSessionRemoteInvokeCallerCancellationThroughRootProcess(t *testing.T, fixture *invokeContinuePackageFixture) {
	scenario := fixture.scenario(t, "remote-cancellation")
	requestStarted := make(chan struct{})
	releaseHandler := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(requestStarted)
		<-releaseHandler
	}))
	defer server.Close()
	defer close(releaseHandler)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	executionPath := filepath.Join(fixture.rootDir, "remote-cancel-execution.json")
	writeInvokeContinueExecutionSpec(t, executionPath, invokeContinueExecutionSpec{
		requestID: "cancel-request", workerSessionID: "cancel-session", dispatchID: "cancel-dispatch",
		factorySessionID: scenario.session.id, workingDirectory: scenario.workingDirectory, userMessage: "cancel this request",
	})
	inputs := support.FakeInputs(ctx, []string{"you", "--remote", "--server", server.URL, "--json", "worker-sessions", "invoke", "--execution", executionPath})
	inputs.Input.Env = scenario.environment()
	inputs.Input.WorkingDirectory = scenario.workingDirectory
	executeDone := make(chan error, 1)
	go func() { executeDone <- fixture.process.Execute(inputs.Input) }()
	select {
	case <-requestStarted:
		cancel()
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for remote invoke request")
	}
	select {
	case err := <-executeDone:
		if err == nil {
			t.Fatal("canceled remote invoke succeeded, want interruption")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for canceled remote invoke")
	}
	assertDirectWorkerSessionCLIError(t, inputs, "WORKER_SESSION_INVOKE_INTERRUPTED")
	if got := scenario.providerRunner.CallCount(); got != 0 {
		t.Fatalf("remote cancellation caused local provider fallback: %d calls", got)
	}
	scenario.close(t)
}

func runDirectWorkerSessionEmptyUserMessageRejectsWithoutProviderCall(t *testing.T, fixture *invokeContinuePackageFixture) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	scenario := fixture.scenario(t, "empty-input")
	executionPath := filepath.Join(fixture.rootDir, "empty-input-execution.json")
	writeInvokeContinueExecutionSpec(t, executionPath, invokeContinueExecutionSpec{
		requestID: "empty-input-request", workerSessionID: "empty-input-session", dispatchID: "empty-input-dispatch",
		factorySessionID: scenario.session.id, workingDirectory: scenario.workingDirectory, userMessage: "",
	})
	inputs := support.FakeInputs(ctx, []string{"you", "--json", "worker-sessions", "invoke", "--execution", executionPath})
	inputs.Input.Env = scenario.environment()
	inputs.Input.WorkingDirectory = scenario.workingDirectory
	if err := fixture.process.Execute(inputs.Input); err == nil {
		t.Fatal("empty Worker Session user message succeeded, want validation failure")
	}
	assertDirectWorkerSessionCLIError(t, inputs, "WORKER_SESSION_FAILED")
	if got := scenario.providerRunner.CallCount(); got != 0 {
		t.Fatalf("provider calls after empty user message = %d, want zero", got)
	}
	scenario.close(t)
}

func runDirectWorkerSessionExactDuplicateContinuationIsIdempotent(t *testing.T, fixture *invokeContinuePackageFixture) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	scenario := fixture.scenario(t, "duplicate-continuation")
	sourceID := scenarioScopedID(scenario, "duplicate-source")
	executionPath := filepath.Join(fixture.rootDir, "duplicate-execution.json")
	writeInvokeContinueExecutionSpec(t, executionPath, invokeContinueExecutionSpec{
		requestID: scenarioScopedID(scenario, "duplicate-invoke-request"), workerSessionID: sourceID, dispatchID: scenarioScopedID(scenario, "duplicate-dispatch"),
		factorySessionID: scenario.session.id, workingDirectory: scenario.workingDirectory, userMessage: "duplicate initial prompt",
	})
	invoke := support.FakeInputs(ctx, []string{"you", "--json", "worker-sessions", "invoke", "--execution", executionPath})
	invoke.Input.Env = scenario.environment()
	invoke.Input.WorkingDirectory = scenario.workingDirectory
	if err := fixture.process.Execute(invoke.Input); err != nil {
		t.Fatalf("duplicate source invoke: %v\nstdout:%s\nstderr:%s", err, invoke.Stdout(), invoke.Stderr())
	}
	continueArgs := []string{
		"you", "--json", "worker-sessions", "continue", sourceID, "--request-id", scenarioScopedID(scenario, "duplicate-continue-request"),
		"--successor-worker-session-id", scenarioScopedID(scenario, "duplicate-successor"), "--user-message", "duplicate continuation",
	}
	first := support.FakeInputs(ctx, continueArgs)
	first.Input.Env = scenario.environment()
	first.Input.WorkingDirectory = scenario.workingDirectory
	if err := fixture.process.Execute(first.Input); err != nil {
		t.Fatalf("first duplicate continuation: %v\nstdout:%s\nstderr:%s", err, first.Stdout(), first.Stderr())
	}
	var firstResult directWorkerSessionCLIResult
	decodeDirectWorkerSessionResult(t, first.Stdout(), &firstResult)
	second := support.FakeInputs(ctx, continueArgs)
	second.Input.Env = scenario.environment()
	second.Input.WorkingDirectory = scenario.workingDirectory
	if err := fixture.process.Execute(second.Input); err != nil {
		t.Fatalf("exact duplicate continuation: %v\nstdout:%s\nstderr:%s", err, second.Stdout(), second.Stderr())
	}
	var secondResult directWorkerSessionCLIResult
	decodeDirectWorkerSessionResult(t, second.Stdout(), &secondResult)
	if secondResult != firstResult {
		t.Fatalf("exact duplicate continuation result = %#v, want original result %#v", secondResult, firstResult)
	}
	if got := scenario.providerRunner.CallCount(); got != 2 {
		t.Fatalf("provider calls after exact duplicate continuation = %d, want initial plus one continuation", got)
	}
	conflict := support.FakeInputs(ctx, []string{
		"you", "--json", "worker-sessions", "continue", sourceID, "--request-id", scenarioScopedID(scenario, "duplicate-continue-request"),
		"--successor-worker-session-id", scenarioScopedID(scenario, "different-successor"), "--user-message", "changed immutable continuation", "--async",
	})
	conflict.Input.Env = scenario.environment()
	conflict.Input.WorkingDirectory = scenario.workingDirectory
	if err := fixture.process.Execute(conflict.Input); err == nil {
		t.Fatal("changed duplicate continuation succeeded, want request-id conflict")
	}
	assertDirectWorkerSessionCLIError(t, conflict, string(factoryapi.ErrorResponseCodeWORKERSESSIONCONTINUATIONREQUESTIDCONFLICT))
	if got := scenario.providerRunner.CallCount(); got != 2 {
		t.Fatalf("provider calls after changed duplicate continuation = %d, want two", got)
	}
	scenario.close(t)
}

func runDirectWorkerSessionDependencyCancellationCancelsOnceAndRecovers(t *testing.T, fixture *invokeContinuePackageFixture) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	scenario := fixture.scenario(t, "dependency-cancellation")
	blocking := scenario.blockingRunner
	executionPath := filepath.Join(fixture.rootDir, "dependency-cancellation-execution.json")
	writeInvokeContinueExecutionSpec(t, executionPath, invokeContinueExecutionSpec{
		requestID: scenarioScopedID(scenario, "dependency-cancellation-request"), workerSessionID: scenarioScopedID(scenario, "dependency-cancellation-session"), dispatchID: scenarioScopedID(scenario, "dependency-cancellation-dispatch"),
		factorySessionID: scenario.session.id, workingDirectory: scenario.workingDirectory, userMessage: "wait for controlled dependency cancellation",
	})
	inputs := support.FakeInputs(ctx, []string{"you", "--json", "worker-sessions", "invoke", "--async", "--execution", executionPath})
	inputs.Input.Env = scenario.environment()
	inputs.Input.WorkingDirectory = scenario.workingDirectory
	if err := fixture.process.Execute(inputs.Input); err != nil {
		t.Fatalf("dependency cancellation invoke admission: %v\nstdout:%s\nstderr:%s", err, inputs.Stdout(), inputs.Stderr())
	}
	select {
	case <-blocking.started:
	case <-ctx.Done():
		t.Fatalf("dependency cancellation runner did not start: %v", ctx.Err())
	}
	cancelInputs := support.FakeInputs(ctx, []string{"you", "--json", "worker-sessions", "cancel", scenarioScopedID(scenario, "dependency-cancellation-session")})
	cancelInputs.Input.Env = scenario.environment()
	cancelInputs.Input.WorkingDirectory = scenario.workingDirectory
	if err := fixture.process.Execute(cancelInputs.Input); err != nil {
		t.Fatalf("dependency cancellation control: %v\nstdout:%s\nstderr:%s", err, cancelInputs.Stdout(), cancelInputs.Stderr())
	}
	var cancelResult struct {
		Action  string `json:"action"`
		Outcome string `json:"outcome"`
		State   string `json:"state"`
	}
	decodeDirectWorkerSessionResult(t, cancelInputs.Stdout(), &cancelResult)
	if cancelResult.Action != "CANCEL" || cancelResult.Outcome != "APPLIED" || cancelResult.State != "CANCELED" {
		t.Fatalf("dependency cancellation result = %#v, want applied CANCEL/CANCELED", cancelResult)
	}
	select {
	case <-blocking.CancellationObserved():
	case <-ctx.Done():
		t.Fatalf("dependency cancellation provider was not canceled: %v", ctx.Err())
	}
	if got := blocking.CallCount(); got != 1 {
		t.Fatalf("dependency cancellation provider calls = %d, want one", got)
	}
	if got := blocking.CancellationCount(); got != 1 {
		t.Fatalf("dependency cancellation provider cancellations = %d, want one", got)
	}
	scenario.close(t)

	recovery := fixture.scenario(t, "cancellation-recovery")
	recoveryPath := filepath.Join(fixture.rootDir, "cancellation-recovery-execution.json")
	writeInvokeContinueExecutionSpec(t, recoveryPath, invokeContinueExecutionSpec{
		requestID: scenarioScopedID(recovery, "cancellation-recovery-request"), workerSessionID: scenarioScopedID(recovery, "cancellation-recovery-session"), dispatchID: scenarioScopedID(recovery, "cancellation-recovery-dispatch"),
		factorySessionID: recovery.session.id, workingDirectory: recovery.workingDirectory, userMessage: "recover after dependency cancellation",
	})
	recovered := support.FakeInputs(ctx, []string{"you", "--json", "worker-sessions", "invoke", "--execution", recoveryPath})
	recovered.Input.Env = recovery.environment()
	recovered.Input.WorkingDirectory = recovery.workingDirectory
	if err := fixture.process.Execute(recovered.Input); err != nil {
		t.Fatalf("invoke after dependency cancellation: %v\nstdout:%s\nstderr:%s", err, recovered.Stdout(), recovered.Stderr())
	}
	var result directWorkerSessionCLIResult
	decodeDirectWorkerSessionResult(t, recovered.Stdout(), &result)
	if result.State != "COMPLETED" || !strings.Contains(result.Output, "timeout recovery output COMPLETE") {
		t.Fatalf("invoke after dependency cancellation result = %#v, want completed recovery", result)
	}
	recovery.close(t)
}

func scenarioScopedID(scenario *invokeContinueScenario, base string) string {
	if scenario == nil || scenario.runNumber == 0 {
		return base
	}
	return base + "-" + strconv.FormatUint(scenario.runNumber, 10)
}

func assertDirectWorkerSessionCLIError(t *testing.T, inputs *support.CapturedInputs, wantCode string) {
	t.Helper()
	for _, output := range []string{inputs.Stderr(), inputs.Stdout()} {
		var response factoryapi.ErrorResponse
		if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &response); err == nil && response.Code != "" {
			if string(response.Code) != wantCode {
				t.Fatalf("direct CLI error code = %q, want %q; stderr=%s stdout=%s", response.Code, wantCode, inputs.Stderr(), inputs.Stdout())
			}
			return
		}
	}
	t.Fatalf("direct CLI emitted no typed error response; stderr=%s stdout=%s", inputs.Stderr(), inputs.Stdout())
}

func decodeDirectWorkerSessionResult(t *testing.T, stdout string, result any) {
	t.Helper()
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), result); err != nil {
		t.Fatalf("decode direct Worker Session result: %v\nstdout:\n%s", err, stdout)
	}
}

func directCodexSessionOutput(sessionID, content string) []byte {
	thread, _ := json.Marshal(map[string]any{"type": "thread.started", "thread_id": sessionID})
	item, _ := json.Marshal(map[string]any{"type": "item.completed", "item": map[string]any{"id": sessionID + "-message", "type": "agent_message", "text": content}})
	completed := []byte(`{"type":"turn.completed","usage":{"input_tokens":1,"output_tokens":1}}`)
	return append(append(append(thread, '\n'), item...), append([]byte{'\n'}, completed...)...)
}

func directCodexOutputWithoutSession(content string) []byte {
	item, _ := json.Marshal(map[string]any{"type": "item.completed", "item": map[string]any{"id": "unassociated-message", "type": "agent_message", "text": content}})
	completed := []byte(`{"type":"turn.completed","usage":{"input_tokens":1,"output_tokens":1}}`)
	return append(item, append([]byte{'\n'}, completed...)...)
}
