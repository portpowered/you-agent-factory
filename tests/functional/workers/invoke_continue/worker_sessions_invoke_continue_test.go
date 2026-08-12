package acceptance

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
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

func TestDirectWorkerSessionInvokeContinueLocalPreservesSessionAndLineage(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	runner := testutil.NewProviderCommandRunner(
		platformprocess.CommandResult{Stdout: directCodexSessionOutput("local-source-thread", "initial direct output COMPLETE")},
		platformprocess.CommandResult{Stdout: directCodexSessionOutput("local-source-thread", "continued direct output COMPLETE")},
	)
	process := support.BuildProcess(t, serviceedges.Edges{ProviderCommandRunner: runner})
	support.CleanupProcess(t, process)

	homeDir := t.TempDir()
	workingDirectory := t.TempDir()
	env := append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)

	invoke := support.FakeInputs(ctx, []string{
		"you", "--json", "worker-sessions", "invoke",
		"--request-id", "local-invoke-request",
		"--worker-session-id", "local-source-session",
		"--dispatch-id", "local-source-dispatch",
		"--workstation", "direct",
		"--worker-type", "direct-worker",
		"--runner", "codex",
		"--provider", "codex",
		"--model", "functional-model",
		"--user-message", "initial direct prompt",
	})
	invoke.Input.Env = env
	invoke.Input.WorkingDirectory = workingDirectory
	if err := process.Execute(invoke.Input); err != nil {
		t.Fatalf("local invoke: %v\nrequests:%#v\nstdout:\n%s\nstderr:\n%s", err, runner.Requests(), invoke.Stdout(), invoke.Stderr())
	}
	var invoked directWorkerSessionCLIResult
	decodeDirectWorkerSessionResult(t, invoke.Stdout(), &invoked)
	if !invoked.Accepted || invoked.RequestID != "local-invoke-request" ||
		invoked.WorkerSessionID != "local-source-session" || invoked.State != "COMPLETED" {
		t.Fatalf("local invoke result = %#v, want accepted completed source", invoked)
	}
	if !strings.Contains(invoked.Output, "initial direct output COMPLETE") {
		t.Fatalf("local invoke output = %q, want provider output\nstdout:\n%s\nstderr:\n%s", invoked.Output, invoke.Stdout(), invoke.Stderr())
	}

	cont := support.FakeInputs(ctx, []string{
		"you", "--json", "worker-sessions", "continue", "local-source-session",
		"--request-id", "local-continue-request",
		"--successor-worker-session-id", "local-successor-session",
		"--user-message", "continued direct prompt",
	})
	cont.Input.Env = env
	cont.Input.WorkingDirectory = workingDirectory
	if err := process.Execute(cont.Input); err != nil {
		t.Fatalf("local continuation: %v\nrequests:%#v\nstdout:\n%s\nstderr:\n%s", err, runner.Requests(), cont.Stdout(), cont.Stderr())
	}
	var continued directWorkerSessionCLIResult
	decodeDirectWorkerSessionResult(t, cont.Stdout(), &continued)
	if !continued.Accepted || continued.RequestID != "local-continue-request" ||
		continued.SourceWorkerSessionID != "local-source-session" ||
		continued.SuccessorWorkerSessionID != "local-successor-session" || continued.State != "COMPLETED" {
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
	conflict.Input.Env = env
	conflict.Input.WorkingDirectory = workingDirectory
	if err := process.Execute(conflict.Input); err == nil {
		t.Fatal("continuation request-id reuse succeeded, want conflict")
	}
	assertDirectWorkerSessionCLIError(t, conflict, string(factoryapi.ErrorResponseCodeWORKERSESSIONCONTINUATIONREQUESTIDCONFLICT))
	if requests := runner.Requests(); len(requests) != 2 {
		t.Fatalf("provider command requests after idempotency conflict = %d, want two", len(requests))
	}
	functionalevidence.Covers(t, "cli/you.worker-sessions.continue", "cli/you.worker-sessions.invoke")
}

func TestDirectWorkerSessionRemoteInterruptUsesExactRouteAndAdmissionSnapshots(t *testing.T) {
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
			RequestId:                "remote-interrupt-request",
			SourceWorkerSessionId:    "source-session",
			SuccessorWorkerSessionId: "successor-session",
			Phase:                    factoryapi.WorkerSessionInterruptResponsePhaseSuccessorAdmission,
			Accepted:                 true,
			Source: factoryapi.WorkerSessionInterruptSnapshot{
				WorkerSessionId: "source-session", State: factoryapi.WorkerSessionInterruptSnapshotStateCanceled,
				EventTopic: "worker-session/source-session/events",
			},
			Successor: factoryapi.WorkerSessionInterruptSnapshot{
				WorkerSessionId: "successor-session", State: factoryapi.WorkerSessionInterruptSnapshotStateRunning,
				EventTopic: "worker-session/successor-session/events",
			},
		})
	}))
	defer server.Close()

	process := support.BuildProcess(t, serviceedges.Edges{ProviderCommandRunner: testutil.NewProviderCommandRunner()})
	support.CleanupProcess(t, process)
	inputs := support.FakeInputs(context.Background(), []string{
		"you", "--remote", "--server", server.URL, "--json", "worker-sessions", "interrupt", "source-session",
		"--request-id", "remote-interrupt-request", "--successor-worker-session-id", "successor-session",
		"--replacement-message", "replace the active work", "--async",
	})
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("remote Worker Session interrupt: %v\nstdout:\n%s\nstderr:\n%s", err, inputs.Stdout(), inputs.Stderr())
	}
	var result directWorkerSessionInterruptCLIResult
	decodeDirectWorkerSessionResult(t, inputs.Stdout(), &result)
	if received.RequestId != "remote-interrupt-request" || received.SuccessorWorkerSessionId != "successor-session" ||
		received.ReplacementMessage != "replace the active work" {
		t.Fatalf("remote interrupt request = %#v, want exact request tuple", received)
	}
	if !result.Accepted || result.Phase != string(factoryapi.WorkerSessionInterruptResponsePhaseSuccessorAdmission) ||
		result.SourceWorkerSessionID != "source-session" || result.SuccessorWorkerSessionID != "successor-session" ||
		result.Source.State != string(factoryapi.WorkerSessionInterruptSnapshotStateCanceled) ||
		result.Successor.State != string(factoryapi.WorkerSessionInterruptSnapshotStateRunning) ||
		result.Source.EventTopic == "" || result.Successor.EventTopic == "" {
		t.Fatalf("remote interrupt result = %#v, want admitted source/successor snapshots", result)
	}
	functionalevidence.Covers(t, "cli/you.worker-sessions.interrupt")
}

func TestDirectWorkerSessionContinueUnknownSourceReturnsNotFoundWithoutProviderCall(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	runner := testutil.NewProviderCommandRunner()
	process := support.BuildProcess(t, serviceedges.Edges{ProviderCommandRunner: runner})
	support.CleanupProcess(t, process)

	inputs := support.FakeInputs(ctx, []string{
		"you", "--json", "worker-sessions", "continue", "unknown-worker-session",
		"--request-id", "unknown-continue-request", "--successor-worker-session-id", "unknown-successor",
		"--user-message", "unknown source", "--async",
	})
	if err := process.Execute(inputs.Input); err == nil {
		t.Fatal("unknown Worker Session continuation succeeded, want not found")
	}
	assertDirectWorkerSessionCLIError(t, inputs, string(factoryapi.ErrorResponseCodeNOTFOUND))
	if runner.CallCount() != 0 {
		t.Fatalf("provider calls after unknown source = %d, want zero", runner.CallCount())
	}
}

func TestDirectWorkerSessionContinueUnassociatedSourceRejectsWithoutProviderContinuation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stdout: directCodexOutputWithoutSession("completed without a Provider Session"),
	})
	process := support.BuildProcess(t, serviceedges.Edges{ProviderCommandRunner: runner})
	support.CleanupProcess(t, process)
	env := append(os.Environ(), "HOME="+t.TempDir(), "USERPROFILE="+t.TempDir())
	workingDirectory := t.TempDir()

	invoke := support.FakeInputs(ctx, []string{
		"you", "--json", "worker-sessions", "invoke", "--request-id", "unassociated-invoke-request",
		"--worker-session-id", "unassociated-source", "--dispatch-id", "unassociated-dispatch", "--workstation", "direct",
		"--worker-type", "direct-worker", "--runner", "codex", "--provider", "codex", "--model", "functional-model",
		"--user-message", "complete without a session",
	})
	invoke.Input.Env = env
	invoke.Input.WorkingDirectory = workingDirectory
	if err := process.Execute(invoke.Input); err != nil {
		t.Fatalf("unassociated source invoke: %v\nstdout:%s\nstderr:%s", err, invoke.Stdout(), invoke.Stderr())
	}

	continuation := support.FakeInputs(ctx, []string{
		"you", "--json", "worker-sessions", "continue", "unassociated-source", "--request-id", "unassociated-continue-request",
		"--successor-worker-session-id", "unassociated-successor", "--user-message", "must not resume", "--async",
	})
	continuation.Input.Env = env
	continuation.Input.WorkingDirectory = workingDirectory
	if err := process.Execute(continuation.Input); err == nil {
		t.Fatal("unassociated source continuation succeeded, want provider continuation invalid")
	}
	assertDirectWorkerSessionCLIError(t, continuation, "WORKER_SESSION_PROVIDER_CONTINUATION_INVALID")
	if runner.CallCount() != 1 {
		t.Fatalf("provider calls after unassociated continuation = %d, want one initial call", runner.CallCount())
	}
}

func TestDirectWorkerSessionContinueStaleProviderSessionDoesNotFreshStart(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	runner := testutil.NewProviderCommandRunner(
		platformprocess.CommandResult{Stdout: directCodexSessionOutput("stale-source-thread", "initial output")},
		platformprocess.CommandResult{
			Stderr:   []byte("Error: thread/resume failed: no rollout found for thread id stale-source-thread"),
			ExitCode: 1,
		},
	)
	process := support.BuildProcess(t, serviceedges.Edges{ProviderCommandRunner: runner})
	support.CleanupProcess(t, process)

	invoke := support.FakeInputs(ctx, []string{
		"you", "--json", "worker-sessions", "invoke", "--request-id", "stale-invoke-request",
		"--worker-session-id", "stale-source", "--dispatch-id", "stale-dispatch", "--workstation", "direct",
		"--worker-type", "direct-worker", "--runner", "codex", "--provider", "codex", "--model", "functional-model",
		"--user-message", "initial output",
	})
	if err := process.Execute(invoke.Input); err != nil {
		t.Fatalf("stale source invoke: %v\nstdout:%s\nstderr:%s", err, invoke.Stdout(), invoke.Stderr())
	}

	continuation := support.FakeInputs(ctx, []string{
		"you", "--json", "worker-sessions", "continue", "stale-source", "--request-id", "stale-continue-request",
		"--successor-worker-session-id", "stale-successor", "--user-message", "resume stale session",
	})
	if err := process.Execute(continuation.Input); err == nil {
		t.Fatal("stale Provider Session continuation succeeded, want terminal failure")
	}
	assertDirectWorkerSessionCLIError(t, continuation, "WORKER_SESSION_FAILED")
	requests := runner.Requests()
	if len(requests) != 2 {
		t.Fatalf("provider calls after stale continuation = %d, want initial plus one exact continuation", len(requests))
	}
	continuationArgs := strings.Join(requests[1].Args, " ")
	if !strings.Contains(continuationArgs, "resume") || !strings.Contains(continuationArgs, "stale-source-thread") {
		t.Fatalf("stale continuation provider command = %#v, want exact resume identity and no fresh start", requests[1].Args)
	}
}

func TestDirectWorkerSessionRemoteContinueProviderFailuresDoNotFallback(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		code    string
		message string
	}{
		{name: "foreign provider session", status: http.StatusConflict, code: "WORKER_SESSION_PROVIDER_CONTINUATION_INVALID", message: "foreign Provider Session"},
		{name: "stale provider session", status: http.StatusConflict, code: "WORKER_SESSION_PROVIDER_CONTINUATION_INVALID", message: "stale Provider Session"},
		{name: "unsupported continuation", status: http.StatusConflict, code: "WORKER_SESSION_PROVIDER_CONTINUATION_INVALID", message: "unsupported Provider Session continuation"},
		{name: "admission failure", status: http.StatusServiceUnavailable, code: "WORKER_SESSION_CONTINUATION_ADMISSION_FAILED", message: "continuation admission failed"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/worker-sessions/source-session/continue" {
					http.NotFound(w, r)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(test.status)
				_ = json.NewEncoder(w).Encode(map[string]string{"code": test.code, "message": test.message})
			}))
			defer server.Close()

			runner := testutil.NewProviderCommandRunner()
			process := support.BuildProcess(t, serviceedges.Edges{ProviderCommandRunner: runner})
			support.CleanupProcess(t, process)
			inputs := support.FakeInputs(context.Background(), []string{
				"you", "--remote", "--server", server.URL, "--json", "worker-sessions", "continue", "source-session",
				"--request-id", "provider-failure-request", "--successor-worker-session-id", "provider-failure-successor",
				"--user-message", "provider failure", "--async",
			})
			if err := process.Execute(inputs.Input); err == nil {
				t.Fatal("remote provider continuation failure succeeded, want typed error")
			}
			assertDirectWorkerSessionCLIError(t, inputs, test.code)
			if runner.CallCount() != 0 {
				t.Fatalf("remote %s caused local provider fallback: %d calls", test.name, runner.CallCount())
			}
		})
	}
}

func TestDirectWorkerSessionRemoteInvokeStreamSourceFailureThroughRootProcess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost && r.URL.Path == "/worker-sessions" {
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(factoryapi.WorkerSessionStartResponse{
				RequestId: "remote-failure-request", WorkerSessionId: "remote-failure-session", Accepted: true,
				State: factoryapi.WorkerSessionStartResponseStateRunning,
			})
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

	process := support.BuildProcess(t, serviceedges.Edges{})
	support.CleanupProcess(t, process)
	inputs := support.FakeInputs(context.Background(), []string{
		"you", "--remote", "--server", server.URL, "--json", "worker-sessions", "invoke",
		"--request-id", "remote-failure-request", "--worker-session-id", "remote-failure-session",
		"--dispatch-id", "remote-failure-dispatch", "--workstation", "direct", "--worker-type", "direct-worker",
		"--runner", "codex", "--provider", "codex", "--model", "functional-model", "--user-message", "stream failure",
	})
	if err := process.Execute(inputs.Input); err == nil {
		t.Fatal("remote stream source failure succeeded, want typed failure")
	}
	assertDirectWorkerSessionCLIError(t, inputs, "WORKER_SESSION_STREAM_SOURCE_FAILURE")
}

func TestDirectWorkerSessionRemoteInvokeCallerCancellationThroughRootProcess(t *testing.T) {
	requestStarted := make(chan struct{})
	releaseHandler := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(requestStarted)
		<-releaseHandler
	}))
	defer server.Close()
	defer close(releaseHandler)

	process := support.BuildProcess(t, serviceedges.Edges{})
	support.CleanupProcess(t, process)
	ctx, cancel := context.WithCancel(context.Background())
	inputs := support.FakeInputs(ctx, []string{
		"you", "--remote", "--server", server.URL, "--json", "worker-sessions", "invoke",
		"--request-id", "cancel-request", "--worker-session-id", "cancel-session", "--dispatch-id", "cancel-dispatch",
		"--workstation", "direct", "--worker-type", "direct-worker", "--runner", "codex", "--provider", "codex",
		"--model", "functional-model", "--user-message", "cancel this request",
	})
	executeDone := make(chan error, 1)
	go func() { executeDone <- process.Execute(inputs.Input) }()
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
	thread, _ := json.Marshal(map[string]any{
		"type":      "thread.started",
		"thread_id": sessionID,
	})
	item, _ := json.Marshal(map[string]any{
		"type": "item.completed",
		"item": map[string]any{
			"id":   sessionID + "-message",
			"type": "agent_message",
			"text": content,
		},
	})
	completed := []byte(`{"type":"turn.completed","usage":{"input_tokens":1,"output_tokens":1}}`)
	return append(append(append(thread, '\n'), item...), append([]byte{'\n'}, completed...)...)
}

func directCodexOutputWithoutSession(content string) []byte {
	item, _ := json.Marshal(map[string]any{
		"type": "item.completed",
		"item": map[string]any{
			"id": "unassociated-message", "type": "agent_message", "text": content,
		},
	})
	completed := []byte(`{"type":"turn.completed","usage":{"input_tokens":1,"output_tokens":1}}`)
	return append(item, append([]byte{'\n'}, completed...)...)
}
