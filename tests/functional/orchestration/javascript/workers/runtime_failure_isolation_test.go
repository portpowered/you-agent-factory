package workers_test

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// F16-14 observes a real normalized command failure after Worker admission,
// separately from the pre-dispatch permission rejection in the existing matrix.
func runJavaScriptProviderFailureBesideLivePeer(t *testing.T, fixture *javascriptSharedProcessFixture) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	peer := &runtimeJavaScriptCommandRunner{marker: "failure witness live peer", started: make(chan struct{}), release: make(chan struct{})}
	failure := &javascriptFailureCommandRunner{}
	const failedPrompt = "failure witness admitted child"
	for marker, runner := range map[string]platformprocess.CommandRunner{peer.marker: peer, failedPrompt: failure} {
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
	peerResult := make(chan concurrentJavaScriptResult, 1)
	go func() {
		workflow := strings.ReplaceAll(liveProviderChildWorkflow, "use the live provider command edge", peer.marker)
		response, err := postOverridesWorkflow(ctx, fixture.baseURL, "provider-failure-live-peer", workflow)
		peerResult <- concurrentJavaScriptResult{response: response, err: err}
	}()
	select {
	case <-peer.started:
	case result := <-peerResult:
		t.Fatalf("peer returned before admission: %+v", result)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	workflow := strings.ReplaceAll(liveProviderChildWorkflow, "use the live provider command edge", failedPrompt)
	response, err := postOverridesWorkflow(ctx, fixture.baseURL, "provider-failure-admitted-child", workflow)
	if err != nil {
		t.Fatal(err)
	}
	assertJavaScriptAdmittedProviderFailure(t, fixture, response, failure.calls.Load())
	select {
	case result := <-peerResult:
		t.Fatalf("failed child completed its gated peer: %+v", result)
	default:
	}
	peer.unblock()
	result, err := awaitConcurrentJavaScriptResult(ctx, peerResult, "provider failure peer")
	if err != nil {
		t.Fatal(err)
	}
	assertRuntimeJavaScriptChild(t, fixture, result, peer.marker, failedPrompt)
}

type javascriptFailureCommandRunner struct{ calls atomic.Int32 }

func (r *javascriptFailureCommandRunner) Run(_ context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	r.calls.Add(1)
	if request.Command != "codex" {
		return platformprocess.CommandResult{}, fmt.Errorf("unexpected failed command %q", request.Command)
	}
	return platformprocess.CommandResult{ExitCode: 1, Stderr: []byte("controlled provider execution failure")}, nil
}

func assertJavaScriptAdmittedProviderFailure(t *testing.T, fixture *javascriptSharedProcessFixture, response factoryapi.FactorySessionSyncExecutionResponse, calls int32) {
	t.Helper()
	if response.Status != factoryapi.FactorySessionDurableLifecycleStatusFailed || calls != 1 {
		t.Fatalf("failed child status=%s command calls=%d, want FAILED and one command", response.Status, calls)
	}
	fixture.trackSession(t, response.SessionId)
	assertUnavailableFactoryResult(t, response.Result)
	session := readOverridesDurableSession(t, fixture.baseURL, response.SessionId)
	if session.FailureDetail == nil || session.FailureDetail.Reason != "unknown" || !strings.Contains(session.FailureDetail.Message, "Codex: provider error: unknown: provider execution failed") {
		t.Fatalf("failed session detail=%+v", session.FailureDetail)
	}
	dispatches := support.GetJSON[factoryapi.ListFactorySessionDispatchesResponse](t, fixture.baseURL+"/factory-sessions/"+response.SessionId+"/dispatches")
	if len(dispatches.Dispatches) != 1 || dispatches.Dispatches[0].Status != factoryapi.FactoryDispatchStatusFAILED {
		t.Fatalf("failed dispatches=%+v", dispatches.Dispatches)
	}
	worker := readJavaScriptFailedWorker(t, fixture, response.SessionId)
	assertJavaScriptEventsDoNotContain(t, support.GetFactoryEventsForSessionAt(t, fixture.baseURL, response.SessionId), "failure witness live peer")
	t.Logf("F16-14 failed Worker=%s attempt=%s failure=%+v", worker.WorkerSessionId, worker.AttemptId, worker.Failure)
}

func readJavaScriptFailedWorker(t *testing.T, fixture *javascriptSharedProcessFixture, sessionID string) factoryapi.WorkerSessionObservation {
	t.Helper()
	observations := support.GetJSON[factoryapi.ListWorkerSessionsResponse](t, fixture.baseURL+"/worker-sessions")
	var owned []factoryapi.WorkerSessionObservation
	for _, observation := range observations.Sessions {
		if observation.FactorySessionId != nil && *observation.FactorySessionId == sessionID {
			owned = append(owned, observation)
		}
	}
	if len(owned) != 1 || owned[0].State != factoryapi.WorkerSessionObservationStateFailed || owned[0].EndedAt == nil || owned[0].Failure == nil {
		t.Fatalf("failed Worker observations=%+v", owned)
	}
	worker := owned[0]
	assertJavaScriptFailedWorkerIdentity(t, worker, sessionID)
	return worker
}

func assertJavaScriptFailedWorkerIdentity(t *testing.T, worker factoryapi.WorkerSessionObservation, sessionID string) {
	t.Helper()
	if worker.Direct || worker.WorkerSessionId != sessionID+"/dispatch-1" || worker.AttemptId != worker.WorkerSessionId+"/attempt/1" || worker.Model == nil || *worker.Model != "live-child-model" || worker.Failure.Kind != "WORKERS_EXECUTION_FAILURE" {
		t.Fatalf("failed Worker identity/selection/classification=%+v", worker)
	}
}
