package acceptance

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// C09 observes the file-selected prompt at the provider command boundary and
// generated request/session identities in the authoritative terminal result.
func TestT19LocalInvokeGeneratesIdentitiesFromExecutionFile(t *testing.T) {
	t.Parallel()
	fixture := ensureInvokeContinuePackageFixture(t)
	scenario := fixture.scenario(t, "t19-generated-file")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	path := filepath.Join(scenario.workingDirectory, "execution.json")
	writeInvokeContinueExecutionSpec(t, path, invokeContinueExecutionSpec{
		factorySessionID: scenario.session.id, workingDirectory: scenario.workingDirectory,
		userMessage: "selected execution-file prompt",
	})
	inputs := support.FakeInputs(ctx, []string{"you", "--json", "worker-sessions", "invoke", "--execution", path})
	inputs.Input.Env = scenario.environment()
	inputs.Input.WorkingDirectory = scenario.workingDirectory
	if err := fixture.process.Execute(inputs.Input); err != nil {
		t.Fatalf("invoke execution file: %v\nstdout:%s\nstderr:%s", err, inputs.Stdout(), inputs.Stderr())
	}
	var result directWorkerSessionCLIResult
	decodeDirectWorkerSessionResult(t, inputs.Stdout(), &result)
	if !result.Accepted || result.State != "COMPLETED" || result.RequestID == "" || result.WorkerSessionID == "" || result.RequestID == result.WorkerSessionID || !strings.Contains(result.Output, "generated-file output COMPLETE") {
		t.Fatalf("generated execution result = %#v, want distinct identities and terminal provider output", result)
	}
	requests := scenario.providerRunner.Requests()
	if len(requests) != 1 || !strings.Contains(strings.Join(requests[0].Args, " ")+string(requests[0].Stdin), "selected execution-file prompt") {
		t.Fatalf("provider requests = %#v, want exactly one file-selected prompt", requests)
	}
	scenario.close(t)
}

// C10 uses a remote admission boundary whose observation stream can never
// complete. An async invocation must return without opening that stream.
func TestT19AsyncInvokeReturnsAdmissionWithoutObservation(t *testing.T) {
	t.Parallel()
	fixture := ensureInvokeContinuePackageFixture(t)
	scenario := fixture.scenario(t, "t19-async")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	var observations atomic.Int32
	admitted := make(chan factoryapi.WorkerSessionStartRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/worker-sessions" {
			var request factoryapi.WorkerSessionStartRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			admitted <- request
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(factoryapi.WorkerSessionStartResponse{
				RequestId: request.RequestId, WorkerSessionId: request.WorkerSessionId,
				Accepted: true, State: factoryapi.WorkerSessionStartResponseStateRunning,
			})
			return
		}
		observations.Add(1)
		<-r.Context().Done()
	}))
	defer server.Close()
	defer cancel()
	path := filepath.Join(scenario.workingDirectory, "execution.json")
	writeInvokeContinueExecutionSpec(t, path, invokeContinueExecutionSpec{
		factorySessionID: scenario.session.id, workingDirectory: scenario.workingDirectory, userMessage: "async gated prompt",
	})
	inputs := support.FakeInputs(ctx, []string{"you", "--remote", "--server", server.URL, "--json", "worker-sessions", "invoke", "--async", "--execution", path})
	inputs.Input.Env = scenario.environment()
	inputs.Input.WorkingDirectory = scenario.workingDirectory
	if err := fixture.process.Execute(inputs.Input); err != nil {
		t.Fatalf("async admission: %v\nstdout:%s\nstderr:%s", err, inputs.Stdout(), inputs.Stderr())
	}
	request := <-admitted
	var result directWorkerSessionCLIResult
	decodeDirectWorkerSessionResult(t, inputs.Stdout(), &result)
	if !result.Accepted || result.State != "RUNNING" || result.Output != "" || result.RequestID != request.RequestId || result.WorkerSessionID != request.WorkerSessionId {
		t.Fatalf("async result = %#v, want admission identities/state without terminal output", result)
	}
	ids := []string{request.RequestId, request.WorkerSessionId, request.Execution.Dispatch.DispatchId}
	if ids[0] == "" || ids[1] == "" || ids[2] == "" || ids[0] == ids[1] || ids[0] == ids[2] || ids[1] == ids[2] {
		t.Fatalf("generated admission identities = %q, want three distinct nonempty values", ids)
	}
	if observations.Load() != 0 || scenario.providerRunner.CallCount() != 0 {
		t.Fatalf("async admission opened %d observations and made %d local provider calls", observations.Load(), scenario.providerRunner.CallCount())
	}
	scenario.close(t)
}

// C14 cancels an already-open SSE stream while an independent peer on the
// same process completes. The server observes closure of the owned stream.
func TestT19CancelObservationStreamPreservesCompletingPeer(t *testing.T) {
	t.Parallel()
	fixture := ensureInvokeContinuePackageFixture(t)
	canceled := fixture.scenario(t, "t19-stream-cancel")
	peer := fixture.scenario(t, "t19-stream-peer")
	ctx, stop := context.WithTimeout(context.Background(), 60*time.Second)
	streamStarted := make(chan struct{})
	streamClosed := make(chan struct{})
	peerStarted := make(chan struct{})
	peerRelease := make(chan struct{})
	server := httptest.NewServer(t19CancellationStreamHandler(streamStarted, streamClosed, peerStarted, peerRelease))
	defer server.Close()
	defer stop()
	ownedContext, cancel := context.WithCancel(ctx)
	defer cancel()
	owned := t19RemoteFileInputs(t, canceled, ownedContext, server.URL, "t19-canceled")
	done := make(chan error, 1)
	go func() { done <- fixture.process.Execute(owned.Input) }()
	t19AwaitSignal(t, ctx, streamStarted, "owned stream opening")
	other := t19RemoteFileInputs(t, peer, ctx, server.URL, "t19-peer")
	peerDone := make(chan error, 1)
	go func() { peerDone <- fixture.process.Execute(other.Input) }()
	t19AwaitSignal(t, ctx, peerStarted, "peer stream opening")
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("canceled stream succeeded")
		}
	case <-ctx.Done():
		t.Fatalf("canceled CLI did not return: %v", ctx.Err())
	}
	assertDirectWorkerSessionCLIError(t, owned, "WORKER_SESSION_INVOKE_INTERRUPTED")
	t19AwaitSignal(t, ctx, streamClosed, "owned stream closure")
	close(peerRelease)
	select {
	case err := <-peerDone:
		if err != nil {
			t.Fatalf("peer invocation: %v\nstdout:%s\nstderr:%s", err, other.Stdout(), other.Stderr())
		}
	case <-ctx.Done():
		t.Fatalf("peer did not complete: %v", ctx.Err())
	}
	var result directWorkerSessionCLIResult
	decodeDirectWorkerSessionResult(t, other.Stdout(), &result)
	if !result.Accepted || result.WorkerSessionID != "t19-peer" || result.State != "COMPLETED" || result.Output != "independent peer output" {
		t.Fatalf("peer result = %#v, want independent terminal output", result)
	}
	if canceled.providerRunner.CallCount() != 0 || peer.providerRunner.CallCount() != 0 {
		t.Fatal("remote stream invocation fell back to local provider")
	}
	canceled.close(t)
	peer.close(t)
}

func t19CancellationStreamHandler(streamStarted, streamClosed, peerStarted, peerRelease chan struct{}) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/worker-sessions" {
			var request factoryapi.WorkerSessionStartRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(factoryapi.WorkerSessionStartResponse{
				RequestId: request.RequestId, WorkerSessionId: request.WorkerSessionId,
				Accepted: true, State: factoryapi.WorkerSessionStartResponseStateRunning,
			})
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		switch r.URL.Path {
		case "/worker-sessions/t19-canceled/events":
			_, _ = io.WriteString(w, "data: {\"delivery\":\"REPLAY_SUMMARY\"}\n\n")
			w.(http.Flusher).Flush()
			close(streamStarted)
			<-r.Context().Done()
			close(streamClosed)
		case "/worker-sessions/t19-peer/events":
			_, _ = io.WriteString(w, "data: {\"delivery\":\"REPLAY_SUMMARY\"}\n\n")
			w.(http.Flusher).Flush()
			close(peerStarted)
			select {
			case <-peerRelease:
			case <-r.Context().Done():
				return
			}
			_, _ = io.WriteString(w, "data: {\"delivery\":\"TERMINAL\",\"event\":{\"schemaId\":\"worker.session.completed\",\"payload\":{\"state\":\"COMPLETED\",\"output\":\"independent peer output\"}}}\n\n")
		default:
			http.NotFound(w, r)
		}
	})
}

func t19AwaitSignal(t *testing.T, ctx context.Context, signal <-chan struct{}, description string) {
	t.Helper()
	select {
	case <-signal:
	case <-ctx.Done():
		t.Fatalf("timed out awaiting %s: %v", description, ctx.Err())
	}
}

func t19RemoteFileInputs(t *testing.T, scenario *invokeContinueScenario, ctx context.Context, serverURL, id string) *support.CapturedInputs {
	t.Helper()
	path := filepath.Join(scenario.workingDirectory, "execution.json")
	writeInvokeContinueExecutionSpec(t, path, invokeContinueExecutionSpec{
		requestID: id + "-request", workerSessionID: id, dispatchID: id + "-dispatch",
		factorySessionID: scenario.session.id, workingDirectory: scenario.workingDirectory, userMessage: id + " prompt",
	})
	inputs := support.FakeInputs(ctx, []string{"you", "--remote", "--server", serverURL, "--json", "worker-sessions", "invoke", "--execution", path})
	inputs.Input.Env = scenario.environment()
	inputs.Input.WorkingDirectory = scenario.workingDirectory
	return inputs
}
