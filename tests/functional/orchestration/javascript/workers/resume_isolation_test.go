package workers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// API-owned lifecycle control resumes a cached child in the shared public
// process. Immutable command routes isolate the interrupted attempt and peer;
// no process rebuild or provider selector replacement is needed for recovery.
func runJavaScriptResumeBesideLivePeer(t *testing.T, fixture *javascriptSharedProcessFixture) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	first := support.NewRecordingCommandRunner("resume cached output")
	remaining := &resumingJavaScriptRunner{started: make(chan struct{}), canceled: make(chan struct{}), resumed: make(chan struct{})}
	peer := &runtimeJavaScriptCommandRunner{marker: "resume live peer", started: make(chan struct{}), release: make(chan struct{})}
	for marker, runner := range map[string]platformprocess.CommandRunner{"resume cached child": first, "resume unfinished child": remaining, peer.marker: peer} {
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
	peerResults := make(chan concurrentJavaScriptResult, 1)
	go func() {
		workflow := strings.ReplaceAll(liveProviderChildWorkflow, "use the live provider command edge", peer.marker)
		response, err := postOverridesWorkflow(ctx, fixture.baseURL, "resume-live-peer", workflow)
		peerResults <- concurrentJavaScriptResult{response: response, err: err}
	}()
	awaitJavaScriptResumeSignal(t, ctx, peer.started)
	workflow := `return (async function () {
  const resumed = workflow.resumeState();
  let first = resumed && resumed.first;
  if (!resumed) {
    first = await agent.run({prompt: "resume cached child", modelProvider: "codex", model: "cached-model"});
    workflow.checkpoint({label: "cached-child-complete", state: {step: 1, first}});
  }
  const second = await agent.run({prompt: "resume unfinished child", modelProvider: "codex", model: "remaining-model"});
  return {first, second};
})();`
	started := postJavaScriptResumeJSON[factoryapi.FactorySessionExecutionResponse](t, fixture.baseURL+"/factory-sessions/async", factoryapi.FactorySessionExecutionRequest{
		RequestId: "resume-cached-beside-peer", Source: factoryapi.FactorySessionExecutionSource{
			Kind: factoryapi.FactorySessionExecutionSourceKindInlineWorkflow,
			InlineWorkflow: &factoryapi.FactorySessionExecutionInlineWorkflow{InlineSource: factoryapi.FactoryOrchestratorJavaScriptInlineSource{
				Encoding: factoryapi.FactoryOrchestratorJavaScriptInlineSourceEncodingUtf8, Inline: workflow,
			}},
		},
	})
	fixture.trackSession(t, started.SessionId)
	stream := support.OpenFactoryResponseEventStreamAt(t, support.SessionResponseEventsURL(fixture.baseURL, started.SessionId))
	t.Cleanup(stream.Close)
	awaitJavaScriptResumeSignal(t, ctx, remaining.started)
	before := javascriptResumeDispatches(t, fixture, started.SessionId)
	interrupted := postJavaScriptResumeJSON[factoryapi.FactorySessionLifecycleControlResponse](t,
		fixture.baseURL+"/factory-sessions/"+started.SessionId+"/interrupt-dispatch",
		factoryapi.FactorySessionInterruptDispatchRequest{DispatchId: "dispatch-2"})
	if interrupted.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted || interrupted.SessionId != started.SessionId {
		t.Fatalf("interrupt outcome: %+v", interrupted)
	}
	awaitJavaScriptResumeSignal(t, ctx, remaining.canceled)
	stream.WaitClosed(30 * time.Second)
	prior := readJavaScriptSharedDurableSession(t, fixture.baseURL, started.SessionId)
	if prior.Status != factoryapi.FactorySessionDurableLifecycleStatusInterrupted || prior.LatestCheckpoint == nil {
		t.Fatalf("interrupted public checkpoint: %+v", prior)
	}
	assertJavaScriptResumePeerLive(t, peerResults)
	resumeJavaScriptAndAwaitDurableResult(t, fixture, started.SessionId)
	assertJavaScriptResumeResult(t, fixture, prior)
	awaitJavaScriptResumeSignal(t, ctx, remaining.resumed)
	after := javascriptResumeDispatches(t, fixture, started.SessionId)
	assertJavaScriptResumedChildren(t, before, after, first.CallCount(), remaining.calls.Load())
	assertJavaScriptResumePeerLive(t, peerResults)
	peer.unblock()
	result, err := awaitConcurrentJavaScriptResult(ctx, peerResults, "resume peer")
	if err != nil {
		t.Fatal(err)
	}
	assertRuntimeJavaScriptChild(t, fixture, result, peer.marker, "resume unfinished child")
}

type resumingJavaScriptRunner struct {
	started, canceled, resumed chan struct{}
	calls                      atomic.Int32
}

func (r *resumingJavaScriptRunner) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	if request.Command != "codex" || !bytes.Contains(request.Stdin, []byte("resume unfinished child")) {
		return platformprocess.CommandResult{}, fmt.Errorf("unexpected resumed command %q", request.Command)
	}
	switch r.calls.Add(1) {
	case 1:
		close(r.started)
		<-ctx.Done()
		close(r.canceled)
		return platformprocess.CommandResult{}, ctx.Err()
	case 2:
		close(r.resumed)
		stdout := append([]byte("{\"type\":\"thread.started\",\"thread_id\":\"provider-resumed-child\"}\n"), support.CodexSuccessStdout("resume remaining output")...)
		return platformprocess.CommandResult{Stdout: stdout}, nil
	default:
		return platformprocess.CommandResult{}, fmt.Errorf("unexpected repeated resumed child")
	}
}

func awaitJavaScriptResumeSignal(t *testing.T, ctx context.Context, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}

func postJavaScriptResumeJSON[T any](t *testing.T, endpoint string, body any) T {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var decoded T
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusAccepted {
		var diagnostic bytes.Buffer
		_, _ = diagnostic.ReadFrom(response.Body)
		t.Fatalf("POST %s status %d: %s", endpoint, response.StatusCode, diagnostic.String())
	}
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		t.Fatal(err)
	}
	return decoded
}

func javascriptResumeDispatches(t *testing.T, fixture *javascriptSharedProcessFixture, sessionID string) []factoryapi.FactorySessionDispatchSummary {
	t.Helper()
	response := support.GetJSON[factoryapi.ListFactorySessionDispatchesResponse](t, fixture.baseURL+"/factory-sessions/"+sessionID+"/dispatches")
	if response.SessionId != sessionID || len(response.Dispatches) != 2 {
		t.Fatalf("resumable dispatch scope: %+v", response)
	}
	return response.Dispatches
}

func assertJavaScriptResumePeerLive(t *testing.T, results <-chan concurrentJavaScriptResult) {
	t.Helper()
	select {
	case result := <-results:
		t.Fatalf("resume control completed gated peer: %+v", result)
	default:
	}
}

func assertJavaScriptResumedChildren(t *testing.T, before, after []factoryapi.FactorySessionDispatchSummary, cachedCalls int, remainingCalls int32) {
	t.Helper()
	if cachedCalls != 1 || remainingCalls != 2 {
		t.Fatalf("child commands: cached=%d remaining=%d, want 1/2", cachedCalls, remainingCalls)
	}
	if before[0].Id != "dispatch-1" || before[0].Status != factoryapi.FactoryDispatchStatusCOMPLETED || !reflect.DeepEqual(before[0], after[0]) {
		t.Fatalf("cached child changed: before=%+v after=%+v", before[0], after[0])
	}
	if before[1].Id != "dispatch-2" || after[1].Id != before[1].Id || after[1].Status != factoryapi.FactoryDispatchStatusCOMPLETED {
		t.Fatalf("unfinished child identity/outcome: before=%+v after=%+v", before[1], after[1])
	}
	if after[1].ProviderSessionRefs == nil || len(*after[1].ProviderSessionRefs) != 1 || (*after[1].ProviderSessionRefs)[0].Id != "provider-resumed-child" {
		t.Fatalf("resumed provider association: %+v", after[1])
	}
}

func assertJavaScriptResumeResult(t *testing.T, fixture *javascriptSharedProcessFixture, prior factoryapi.FactorySessionDurableReadModel) {
	t.Helper()
	after := readJavaScriptSharedDurableSession(t, fixture.baseURL, prior.SessionId)
	if !reflect.DeepEqual(prior.LatestCheckpoint, after.LatestCheckpoint) || prior.Lifecycle == nil || after.Lifecycle == nil ||
		prior.Lifecycle.InterruptedAt == nil || after.Lifecycle.InterruptedAt == nil || after.Lifecycle.ResumedAt == nil ||
		!prior.Lifecycle.InterruptedAt.Equal(*after.Lifecycle.InterruptedAt) {
		t.Fatalf("resume changed checkpoint/lifecycle continuity: before=%+v after=%+v", prior, after)
	}
	result := support.GetJSON[factoryapi.FactorySessionResult](t, fixture.baseURL+"/factory-sessions/"+prior.SessionId+"/results")
	if result.SessionId != prior.SessionId || result.ResultStatus != factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("resumed public result scope/status: %+v", result)
	}
	encoded, err := json.Marshal(result.PrimaryResult)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(encoded, []byte("resume cached output")) || !bytes.Contains(encoded, []byte("resume remaining output")) || bytes.Contains(encoded, []byte("resume live peer")) {
		t.Fatalf("restored output attribution: %s", encoded)
	}
	assertJavaScriptResumedWorker(t, fixture, prior.SessionId)
	t.Logf("F16 resume: Factory=%s checkpoint retained; cached commands=1 unfinished commands=2; scoped final output includes both children", prior.SessionId)
}

func assertJavaScriptResumedWorker(t *testing.T, fixture *javascriptSharedProcessFixture, sessionID string) {
	t.Helper()
	workers := support.GetJSON[factoryapi.ListWorkerSessionsResponse](t, fixture.baseURL+"/worker-sessions")
	var resumed []factoryapi.WorkerSessionObservation
	for _, worker := range workers.Sessions {
		if worker.FactorySessionId != nil && *worker.FactorySessionId == sessionID && worker.ProviderSession != nil && worker.ProviderSession.Id == "provider-resumed-child" {
			resumed = append(resumed, worker)
		}
	}
	if len(resumed) != 1 {
		t.Fatalf("resumed Worker provider association: %+v", resumed)
	}
	worker := resumed[0]
	if worker.Direct || worker.State != "COMPLETED" || worker.WorkerSessionId == "" || worker.AttemptId == "" || worker.Model == nil || *worker.Model != "remaining-model" {
		t.Fatalf("resumed Worker outcome/selected model: %+v", worker)
	}
	t.Logf("F16 resumed association: Factory=%s Worker=%s physical=%s provider=%s", sessionID, worker.WorkerSessionId, worker.AttemptId, worker.ProviderSession.Id)
}

func resumeJavaScriptAndAwaitDurableResult(t *testing.T, fixture *javascriptSharedProcessFixture, sessionID string) {
	t.Helper()
	path := filepath.Join(fixture.hostDir, ".you-agent-factory", "durable-sessions", sessionID+".json")
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatal(err)
	}
	defer watcher.Close()
	if err := watcher.Add(filepath.Dir(path)); err != nil {
		t.Fatal(err)
	}
	resumed := postJavaScriptResumeJSON[factoryapi.FactorySessionLifecycleControlResponse](t, fixture.baseURL+"/factory-sessions/"+sessionID+"/resume", factoryapi.FactorySessionLifecycleControlRequest{})
	if resumed.SessionId != sessionID || resumed.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("resume outcome: %+v", resumed)
	}
	// Interrupted-session history reads are finite. Atomic snapshot replacement
	// supplies the readiness signal; only the public session read proves success.
	deadline := time.NewTimer(30 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case event := <-watcher.Events:
			if !strings.EqualFold(filepath.Clean(event.Name), filepath.Clean(path)) {
				continue
			}
			data, err := os.ReadFile(path)
			var snapshot struct{ Session struct{ Status string } }
			if err != nil || json.Unmarshal(data, &snapshot) != nil || snapshot.Session.Status != "SUCCEEDED" {
				continue
			}
			session := readJavaScriptSharedDurableSession(t, fixture.baseURL, sessionID)
			if session.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
				t.Fatalf("resumed public outcome: %+v", session)
			}
			return
		case err := <-watcher.Errors:
			t.Fatal(err)
		case <-deadline.C:
			t.Fatalf("resumed session did not publish terminal snapshot: %s", path)
		}
	}
}
