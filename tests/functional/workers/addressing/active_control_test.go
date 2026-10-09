package addressing_test

import (
	"context"
	"encoding/json"
	"fmt"

	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// The legacy peer enters through production replay. The active source enters
// through public invocation; only the provider command effect is controlled.
func TestActiveLegacyCollisionControls(t *testing.T) {
	t.Parallel()
	runner := &addressingRunner{started: make(chan struct{}, 2), release: make(chan struct{})}
	f := newReplayFixtureWithRunner(t, runner, 1)
	t.Cleanup(func() { runner.releaseOnce.Do(func() { close(runner.release) }) })
	live := openAddressingOwner(t, f)
	invokeAddressingSource(t, f, live)
	awaitAddressingStart(t, runner, f, live)
	assertAddressingState(t, f, live, "RUNNING")
	status, raw := f.http(t, "GET", "/worker-sessions/"+f.worker, nil)
	if status != http.StatusConflict {
		t.Fatalf("active show = %d: %s", status, raw)
	}
	assertActiveCandidates(t, f, live, raw, false)

	// F2-04: both public adapters refuse before cancel, launch or admission.
	for _, operation := range []string{"continue", "interrupt"} {
		body, flags := controlInput(operation)
		status, raw := f.http(t, "POST", "/worker-sessions/"+f.worker+"/"+operation, body)
		if status != http.StatusConflict {
			t.Fatalf("ambiguous active %s = %d: %s", operation, status, raw)
		}
		assertActiveCandidates(t, f, live, raw, operation == "interrupt")
		assertActiveCandidates(t, f, live, f.cli(t, true, append([]string{operation, f.worker}, flags...)...), operation == "interrupt")
		assertNotFoundSuccessor(t, f, body["successorWorkerSessionId"].(string))
		assertAddressingState(t, f, live, "RUNNING")
		assertAddressingEffects(t, runner, 1, 0)
	}

	// F2-08: scope selects the live source, HTTP and CLI replay the same tuple,
	// and changing owner/input/successor cannot return a peer's cached success.
	body, flags := controlInput("interrupt")
	body["factorySessionId"] = live.session
	body["resumeMode"] = "recorded"
	path := "/worker-sessions/" + f.worker + "/interrupt"
	status, first := f.http(t, "POST", path, body)
	if status != http.StatusAccepted {
		t.Fatalf("scoped interrupt = %d: %s", status, first)
	}
	awaitAddressingStart(t, runner, f, live)
	assertAddressingEffects(t, runner, 2, 1)
	assertAddressingState(t, f, live, "CANCELED")
	successor := body["successorWorkerSessionId"].(string)
	assertAddressingSuccessor(t, f, live, successor)
	cliArgs := append([]string{"interrupt", f.worker}, flags...)
	cliArgs = append(cliArgs, "--session", live.session, "--resume-mode", "recorded")
	assertSameJSON(t, first, f.cli(t, false, cliArgs...))
	status, repeated := f.http(t, "POST", path, body)
	if status != http.StatusAccepted {
		t.Fatalf("HTTP replay = %d: %s", status, repeated)
	}
	assertSameJSON(t, first, repeated)
	for _, change := range []map[string]any{
		{"factorySessionId": f.owners[0].session},
		{"replacementMessage": "different input"},
		{"successorWorkerSessionId": uuid.NewString()},
	} {
		changed := make(map[string]any, len(body))
		for key, value := range body {
			changed[key] = value
		}
		for key, value := range change {
			changed[key] = value
		}
		status, raw := f.http(t, "POST", path, changed)
		if status != http.StatusConflict {
			t.Fatalf("changed control tuple = %d: %s", status, raw)
		}
		assertErrorCode(t, raw, "WORKER_SESSION_INTERRUPT_REQUEST_ID_CONFLICT")
		assertValidationPhase(t, raw)
		assertAddressingEffects(t, runner, 2, 1)
	}
	status, peer := f.http(t, "GET", "/worker-sessions/"+f.worker+"?factorySessionId="+f.owners[0].session, nil)
	if status != http.StatusOK {
		t.Fatalf("peer = %d: %s", status, peer)
	}
	assertOwner(t, f, f.owners[0], peer)
	assertInterruptedSourceContinuationConflict(t, f, runner, live)
}

func assertInterruptedSourceContinuationConflict(t *testing.T, f *replayFixture, runner *addressingRunner, live replayOwner) {
	t.Helper()
	body, flags := controlInput("continue")
	body["factorySessionId"] = live.session
	path := "/worker-sessions/" + f.worker + "/continue"
	// Interrupt has already claimed this source's successor. Explicit scope
	// preserves that lifecycle conflict instead of choosing the replay peer.
	status, raw := f.http(t, "POST", path, body)
	if status != http.StatusConflict {
		t.Fatalf("already replaced source continuation = %d: %s", status, raw)
	}
	assertErrorCode(t, raw, "WORKER_SESSION_CONTINUATION_CONFLICT")
	cliArgs := append([]string{"continue", f.worker}, flags...)
	cliArgs = append(cliArgs, "--session", live.session)
	assertErrorCode(t, f.cli(t, true, cliArgs...), "WORKER_SESSION_CONTINUATION_CONFLICT")
	assertNotFoundSuccessor(t, f, body["successorWorkerSessionId"].(string))
	assertAddressingEffects(t, runner, 2, 1)
	assertAddressingState(t, f, live, "CANCELED")
}

func openAddressingOwner(t *testing.T, f *replayFixture) replayOwner {
	t.Helper()
	owner := replayOwner{dir: t.TempDir(), work: "work-" + uuid.NewString()}
	config, err := os.ReadFile(filepath.Join(support.LegacyFixtureDir(t, "executor_success"), "factory.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(owner.dir, "factory.json"), config, 0o600); err != nil {
		t.Fatal(err)
	}
	owner.session = support.OpenFactorySessionAt(t, f.url, owner.dir).Session.Id
	// The fixture Process.Close cleanup cancels and joins every owned session.
	return owner
}

func invokeAddressingSource(t *testing.T, f *replayFixture, owner replayOwner) {
	t.Helper()
	body := map[string]any{"requestId": uuid.NewString(), "workerSessionId": f.worker, "execution": map[string]any{
		"factorySessionId": owner.session, "workstationName": workers.ProviderInvocationRoute, "workerType": "direct-worker",
		"runnerId": "codex", "executorProvider": "codex", "modelProvider": "codex", "model": "functional-model",
		"workingDirectory": owner.dir, "workingDirectoryAuthored": true, "userMessage": "active source",
		"dispatch": map[string]any{"dispatchId": uuid.NewString(), "workstationName": workers.ProviderInvocationRoute, "workerType": "direct-worker",
			"execution": map[string]any{"workIds": []string{owner.work}}}}}
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	f.cli(t, false, "invoke", "--request-id", body["requestId"].(string), "--worker-session-id", f.worker, "--execution", string(encoded), "--async")
}

func assertActiveCandidates(t *testing.T, f *replayFixture, live replayOwner, raw []byte, interrupt bool) {
	t.Helper()
	var result struct {
		Code    string
		Details struct {
			Candidates []struct{ FactorySessionID, WorkerSessionID, WorkID, State string }
		}
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	if result.Code != "WORKER_SESSION_AMBIGUOUS" {
		t.Fatalf("ambiguity: %s", raw)
	}
	if interrupt {
		assertValidationPhase(t, raw)
	}
	var got []string
	for _, candidate := range result.Details.Candidates {
		got = append(got, candidate.FactorySessionID+"/"+candidate.WorkerSessionID+"/"+candidate.WorkID+"/"+candidate.State)
	}
	want := []string{live.session + "/" + f.worker + "/" + live.work + "/RUNNING", f.owners[0].session + "/" + f.worker + "/" + f.owners[0].work + "/COMPLETED"}
	sort.Strings(got)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("active candidates = %v, want %v: %s", got, want, raw)
	}
}

func assertAddressingState(t *testing.T, f *replayFixture, owner replayOwner, state string) {
	t.Helper()
	status, raw := f.http(t, "GET", "/worker-sessions/"+f.worker+"?factorySessionId="+owner.session, nil)
	var observation factoryapi.WorkerSessionObservation
	if err := json.Unmarshal(raw, &observation); err != nil {
		t.Fatal(err)
	}
	if status != http.StatusOK || string(observation.State) != state || observation.FactorySessionId == nil || *observation.FactorySessionId != owner.session {
		t.Fatalf("selected source want %s: %d %s", state, status, raw)
	}
}

func assertAddressingSuccessor(t *testing.T, f *replayFixture, owner replayOwner, successor string) {
	t.Helper()
	status, raw := f.http(t, "GET", "/worker-sessions/"+successor, nil)
	var observation factoryapi.WorkerSessionObservation
	if err := json.Unmarshal(raw, &observation); err != nil {
		t.Fatal(err)
	}
	if status != http.StatusOK || observation.FactorySessionId == nil || *observation.FactorySessionId != owner.session ||
		observation.PredecessorWorkerSessionId == nil || *observation.PredecessorWorkerSessionId != f.worker ||
		!reflect.DeepEqual(observation.WorkIds, []string{owner.work}) {
		t.Fatalf("selected successor lineage: %d %s", status, raw)
	}
}

func assertNotFoundSuccessor(t *testing.T, f *replayFixture, successor string) {
	t.Helper()
	status, raw := f.http(t, "GET", "/worker-sessions/"+successor, nil)
	assertNotFound(t, status, raw)
}

func assertSameJSON(t *testing.T, first, second []byte) {
	t.Helper()
	var a, b map[string]any
	if err := json.Unmarshal(first, &a); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(second, &b); err != nil {
		t.Fatal(err)
	}
	// CLI adds its documented observation command to the shared response.
	if hint, ok := b["observation"]; ok {
		want := "you worker-sessions show --worker-session-id " + a["successorWorkerSessionId"].(string)
		if hint != want {
			t.Fatalf("CLI observation command = %v, want %s", hint, want)
		}
		delete(b, "observation")
	}
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("control replay differs: %s / %s", first, second)
	}
}

type addressingRunner struct {
	mu                   sync.Mutex
	requests             []platformprocess.CommandRequest
	started              chan struct{}
	release              chan struct{}
	releaseOnce          sync.Once
	calls, cancellations atomic.Int32
}

func (r *addressingRunner) Run(ctx context.Context, req platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	return r.RunStreaming(ctx, req, nil)
}

func (r *addressingRunner) RunStreaming(ctx context.Context, req platformprocess.CommandRequest, observe platformprocess.OutputChunkObserver) (platformprocess.CommandResult, error) {
	r.mu.Lock()
	req.Args = append([]string(nil), req.Args...)
	r.requests = append(r.requests, platformprocess.CommandRequest{Command: req.Command, Args: req.Args,
		Stdin: append([]byte(nil), req.Stdin...), WorkDir: req.WorkDir})
	r.mu.Unlock()
	r.calls.Add(1)
	thread := []byte("{\"type\":\"thread.started\",\"thread_id\":\"addressing-native\"}\n")
	if observe != nil {
		observe(platformprocess.OutputStreamStdout, thread)
	}
	r.started <- struct{}{}
	select {
	case <-ctx.Done():
		r.cancellations.Add(1)
		return platformprocess.CommandResult{}, ctx.Err()
	case <-r.release:
		output := []byte("{\"type\":\"item.completed\",\"item\":{\"id\":\"answer\",\"type\":\"agent_message\",\"text\":\"COMPLETE\"}}\n{\"type\":\"turn.completed\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}\n")
		if observe != nil {
			observe(platformprocess.OutputStreamStdout, output)
		}
		return platformprocess.CommandResult{Stdout: append(thread, output...)}, nil
	}
}

// F2-05/F2-08: the terminal direct source shares its public ID with a retained
// replay. Exact scope must resume its captured provider association only once.
func TestSelectedLegacyCollisionContinuation(t *testing.T) {
	t.Parallel()
	runner := &addressingRunner{started: make(chan struct{}, 2), release: make(chan struct{})}
	f := newReplayFixtureWithRunner(t, runner, 1)
	t.Cleanup(func() { runner.releaseOnce.Do(func() { close(runner.release) }) })
	live := openAddressingOwner(t, f)
	invokeAddressingSource(t, f, live)
	awaitAddressingStart(t, runner, f, live)
	runner.releaseOnce.Do(func() { close(runner.release) })
	waitAddressingCompleted(t, f, f.worker, live.session)

	body, flags := controlInput("continue")
	body["factorySessionId"] = live.session
	path := "/worker-sessions/" + f.worker + "/continue"
	status, first := f.http(t, "POST", path, body)
	if status != http.StatusAccepted {
		t.Fatalf("selected continuation = %d: %s", status, first)
	}
	awaitAddressingStart(t, runner, f, live)
	successor := body["successorWorkerSessionId"].(string)
	waitAddressingCompleted(t, f, successor, live.session)
	assertAddressingSuccessor(t, f, live, successor)
	status, repeat := f.http(t, "POST", path, body)
	if status != http.StatusAccepted {
		t.Fatalf("repeat continuation = %d: %s", status, repeat)
	}
	assertSameJSON(t, first, repeat)
	args := append([]string{"continue", f.worker}, flags...)
	args = append(args, "--session", live.session)
	assertSameJSON(t, first, f.cli(t, false, args...))
	for _, change := range []map[string]any{
		{"factorySessionId": f.owners[0].session},
		{"followUpInput": "different input"},
		{"successorWorkerSessionId": uuid.NewString()},
	} {
		changed := make(map[string]any, len(body))
		for key, value := range body {
			changed[key] = value
		}
		for key, value := range change {
			changed[key] = value
		}
		status, raw := f.http(t, "POST", path, changed)
		if status != http.StatusConflict {
			t.Fatalf("changed continuation = %d: %s", status, raw)
		}
		assertErrorCode(t, raw, "WORKER_SESSION_CONTINUATION_REQUEST_ID_CONFLICT")
		changedFlags := []string{"continue", f.worker, "--session", changed["factorySessionId"].(string),
			"--request-id", changed["requestId"].(string), "--successor-worker-session-id", changed["successorWorkerSessionId"].(string),
			"--user-message", changed["followUpInput"].(string), "--async"}
		assertErrorCode(t, f.cli(t, true, changedFlags...), "WORKER_SESSION_CONTINUATION_REQUEST_ID_CONFLICT")
	}
	assertAddressingEffects(t, runner, 2, 0)
	runner.mu.Lock()
	requests := append([]platformprocess.CommandRequest(nil), runner.requests...)
	runner.mu.Unlock()
	if len(requests) != 2 || strings.Contains(strings.Join(requests[0].Args, " "), "resume") {
		t.Fatalf("initial provider requests: %#v", requests)
	}
	resume := strings.Join(requests[1].Args, " ")
	if !strings.Contains(resume, "resume") || !strings.Contains(resume, "addressing-native") ||
		string(requests[1].Stdin) != "replacement" || requests[1].WorkDir != live.dir {
		t.Fatalf("selected provider continuation: %#v", requests[1])
	}
	assertAddressingState(t, f, live, "COMPLETED")
	status, peer := f.http(t, "GET", "/worker-sessions/"+f.worker+"?factorySessionId="+f.owners[0].session, nil)
	if status != http.StatusOK {
		t.Fatalf("peer = %d: %s", status, peer)
	}
	assertOwner(t, f, f.owners[0], peer)
}

func waitAddressingCompleted(t *testing.T, f *replayFixture, worker, owner string) {
	t.Helper()
	raw, err := support.WaitForObservation(15*time.Second, func() ([]byte, error) {
		status, raw := f.http(t, "GET", "/worker-sessions/"+worker+"?factorySessionId="+owner, nil)
		if status != http.StatusOK {
			return raw, fmt.Errorf("observation status %d", status)
		}
		return raw, nil
	}, func(raw []byte) bool {
		var observation factoryapi.WorkerSessionObservation
		return json.Unmarshal(raw, &observation) == nil && observation.State == "COMPLETED" &&
			observation.FactorySessionId != nil && *observation.FactorySessionId == owner
	})
	if err != nil {
		t.Fatalf("selected terminal observation: %s: %v", raw, err)
	}
}

func awaitAddressingStart(t *testing.T, runner *addressingRunner, f *replayFixture, owner replayOwner) {
	t.Helper()
	select {
	case <-runner.started:
	case <-time.After(15 * time.Second):
		status, raw := f.http(t, "GET", "/worker-sessions/"+f.worker+"?factorySessionId="+owner.session, nil)
		t.Fatalf("provider did not start: %d %s", status, raw)
	}
}

func assertAddressingEffects(t *testing.T, runner *addressingRunner, calls, cancellations int32) {
	t.Helper()
	if runner.calls.Load() != calls || runner.cancellations.Load() != cancellations {
		t.Fatalf("provider calls/cancellations = %d/%d, want %d/%d", runner.calls.Load(), runner.cancellations.Load(), calls, cancellations)
	}
}

// F2-06: the real Providers policy denies resume while still admitting the
// initial invocation. Scope must preserve that refusal beside a legacy peer.
func TestSelectedLegacyCollisionUnsupportedProvider(t *testing.T) {
	t.Parallel()
	runner := &addressingRunner{started: make(chan struct{}, 2), release: make(chan struct{})}
	f := newReplayFixtureWithEdges(t, runner, 1, serviceedges.Edges{
		ProviderCatalogCapabilityOverrides: []providerswire.CatalogCapabilityOverride{{Provider: providers.IDCodex,
			Capabilities: []providers.Capability{providers.CapabilityPromptSubmission,
				providers.CapabilityNativeStreaming, providers.CapabilityMessageDeltas, providers.CapabilityUsage}}},
	})
	t.Cleanup(func() { runner.releaseOnce.Do(func() { close(runner.release) }) })
	live := openAddressingOwner(t, f)
	invokeAddressingSource(t, f, live)
	awaitAddressingStart(t, runner, f, live)
	body, flags := controlInput("interrupt")
	body["factorySessionId"] = live.session
	status, raw := f.http(t, "POST", "/worker-sessions/"+f.worker+"/interrupt", body)
	if status != http.StatusConflict {
		t.Fatalf("unsupported scoped provider interrupt = %d: %s", status, raw)
	}
	assertErrorCode(t, raw, "PROVIDER_UNSUPPORTED")
	assertValidationPhase(t, raw)
	args := append([]string{"interrupt", f.worker}, flags...)
	args = append(args, "--session", live.session, "--resume-mode", "provider")
	cli := f.cli(t, true, args...)
	assertErrorCode(t, cli, "PROVIDER_UNSUPPORTED")
	assertValidationPhase(t, cli)
	assertNotFoundSuccessor(t, f, body["successorWorkerSessionId"].(string))
	assertAddressingState(t, f, live, "RUNNING")
	assertAddressingEffects(t, runner, 1, 0)
	status, peer := f.http(t, "GET", "/worker-sessions/"+f.worker+"?factorySessionId="+f.owners[0].session, nil)
	if status != http.StatusOK {
		t.Fatalf("unsupported control peer = %d: %s", status, peer)
	}
	assertOwner(t, f, f.owners[0], peer)
	// Policy refusal leaves the selected source able to finish normally.
	runner.releaseOnce.Do(func() { close(runner.release) })
	waitAddressingCompleted(t, f, f.worker, live.session)
	assertAddressingEffects(t, runner, 1, 0)
}
