package response_events

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// This HTTP parity cell owns the lifecycle-control and event-cursor contracts.
// One root-built Process hosts four explicitly opened Factory Sessions. Only
// the provider command effect is controlled; Work and events use real public
// serialization and reads. All four admissions precede cancellation/release.
func TestFourExplicitSessionsKeepPeerOutputsAfterSelectedCancellation(t *testing.T) {
	t.Parallel()
	runner := newLifecycleIsolationRunner()
	hostDir := support.ScaffoldFactory(t, map[string]any{"name": "keyed-lifecycle-idle-host"})
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: hostDir, WaitForServiceModeRuntime: true,
		Edges: serviceedges.Edges{ProviderCommandRunner: runner},
	})
	t.Cleanup(func() { server.Stop(t) })
	assertLifecycleIsolationUnknownControl(t, server.URL())
	var sessions [4]string
	var streams [4]*support.FactoryResponseEventStream
	prompts := make(map[string]string)
	for i, gate := range runner.gates {
		dir := scaffoldConcurrentIsolationFactory(t, gate.marker)
		opened := support.OpenFactorySessionAt(t, server.URL(), dir)
		if opened.Session == nil || opened.Session.Id == "" {
			t.Fatalf("open explicit session %d = %#v", i, opened)
		}
		sessions[i] = opened.Session.Id
		prompts[sessions[i]] = gate.marker
		t.Cleanup(func() { support.CloseFactorySessionAt(t, server.URL(), sessions[i]) })
		if i > 0 {
			streams[i] = support.OpenFactoryResponseEventStreamAt(t, support.SessionResponseEventsURL(server.URL(), sessions[i]))
			t.Cleanup(streams[i].Close)
		}
	}
	// A paused invocation returns INVOCATION_PAUSED even if later resumed.
	// Exercise pause/resume before submitting the independent cancel journey.
	for _, action := range []string{"pause", "resume"} {
		control := lifecycleIsolationControl(t, server.URL(), sessions[0], action)
		if control.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
			t.Fatalf("%s = %#v, want ACCEPTED", action, control)
		}
	}
	// Live cancel stops Work execution, but does not promise to complete an
	// outstanding invocation wait. Admit A as Work directly; B/C/D own result
	// waits. Durable CANCELED status remains a separate execution-session cell.
	delete(prompts, sessions[0])
	selectedWork := support.SubmitSessionWorkAt(t, server.URL(), sessions[0], factoryapi.SubmitWorkRequest{
		WorkTypeName: "task", Payload: map[string]string{"title": runner.gates[0].marker},
	})
	if selectedWork.WorkId == nil || *selectedWork.WorkId == "" {
		t.Fatal("selected Work admission omitted its identity")
	}
	invocations := startConcurrentIsolationInvocations(t, server.URL(), prompts)
	for _, gate := range runner.gates {
		select {
		case <-gate.started:
		case <-t.Context().Done():
			t.Fatal("provider admission interrupted")
		}
	}
	canceled := lifecycleIsolationControl(t, server.URL(), sessions[0], "cancel")
	if canceled.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("cancel = %#v, want ACCEPTED", canceled)
	}
	select {
	case <-runner.gates[0].canceled:
	case <-t.Context().Done():
		t.Fatal("selected provider did not observe cancellation")
	}
	for i := 1; i < len(runner.gates); i++ {
		select {
		case <-runner.gates[i].canceled:
			t.Fatalf("cancel A reached peer %d", i)
		default:
		}
		close(runner.gates[i].release)
	}
	support.WaitForSessionStopped(t, server.URL(), sessions[0], concurrentIsolationTimeout)
	assertLifecycleIsolationSelectedCursor(t, server.URL(), sessions[0], runner.gates[0], runner.gates)
	for i := 1; i < len(sessions); i++ {
		result := awaitConcurrentIsolationInvocation(t, invocations[sessions[i]])
		assertConcurrentIsolationInvocationCompleted(t, result, runner.gates[i].output)
		frames := collectConcurrentIsolationFrames(t, streams[i], runner.gates[i].output, concurrentIsolationTimeout)
		assertLifecycleIsolationPeerReads(t, server.URL(), sessions[i], frames, runner.gates[i], runner.gates)
		assertLifecycleIsolationRecordedWork(t, server.URL(), sessions[i], runner.gates[i], runner.gates)
	}
	assertLifecycleIsolationDurableCancellation(t, server.URL(), runner.durable)
}

func assertLifecycleIsolationUnknownControl(t *testing.T, baseURL string) {
	t.Helper()
	for _, action := range []string{"pause", "resume", "cancel"} {
		var response factoryapi.ErrorResponse
		lifecycleIsolationPost(t, baseURL+"/factory-sessions/4e5f2c32-6d2e-4f10-93ea-974f657e7001/"+action, map[string]any{}, http.StatusNotFound, &response)
		if response.Code != "NOT_FOUND" || response.Family != factoryapi.ErrorFamilyNotFound {
			t.Fatalf("unknown %s = %#v, want typed NOT_FOUND", action, response)
		}
	}
}

func assertLifecycleIsolationSelectedCursor(t *testing.T, baseURL, sessionID string, gate *lifecycleIsolationGate, gates [4]*lifecycleIsolationGate) {
	t.Helper()
	retained := retainedFactoryResponseEventsWithoutGaps(support.GetFactoryResponseEventsAt(t, baseURL, sessionID))
	if len(retained) == 0 {
		t.Fatalf("selected response events = %d, want retained cursor suffix", len(retained))
	}
	assertResponseEventsAscendingSequence(t, retained)
	stream := support.OpenFactoryResponseEventStreamAt(t, support.SessionResponseEventsURLWithAfterSequence(baseURL, sessionID, 0))
	t.Cleanup(stream.Close)
	for _, want := range retained {
		got := stream.NextFrame(concurrentIsolationTimeout)
		if got.Event.EventId != want.EventId || got.Event.Sequence != want.Sequence || got.Event.FactorySessionId != sessionID {
			t.Fatalf("selected response reconnect = %#v, want %#v", got.Event, want)
		}
		assertLifecycleIsolationNoPeerMarker(t, got.Event, gate, gates)
	}
}

func assertLifecycleIsolationDurableCancellation(t *testing.T, baseURL string, gate *lifecycleIsolationGate) {
	t.Helper()
	dir := scaffoldSessionExpiryWorkflow(t)
	workflowPath := filepath.Join(dir, sessionExpiryChildWorkflowFile)
	source := strings.ReplaceAll(sessionExpiryChildWorkflowSource, "summarize session expiry", gate.marker)
	if err := os.WriteFile(workflowPath, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	input := factoryapi.FactorySessionExecutionRequest{
		RequestId: "t14-owned-durable-cancel",
		Source:    factoryapi.FactorySessionExecutionSource{Kind: factoryapi.FactorySessionExecutionSourceKindWorkflowFile, WorkflowFile: &workflowPath},
	}
	var started factoryapi.FactorySessionExecutionResponse
	lifecycleIsolationPost(t, baseURL+"/factory-sessions/async", input, http.StatusOK, &started)
	select {
	case <-gate.started:
	case <-t.Context().Done():
		t.Fatal("durable provider admission interrupted")
	}
	control := lifecycleIsolationControl(t, baseURL, started.SessionId, "cancel")
	if control.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted || control.Status != factoryapi.FactorySessionDurableLifecycleStatusCanceling {
		t.Fatalf("durable cancel = %#v, want accepted CANCELING", control)
	}
	select {
	case <-gate.canceled:
	case <-t.Context().Done():
		t.Fatal("durable provider cancellation interrupted")
	}
	// Provider cancellation precedes publication of the durable projection.
	// Observe that public projection through the shared bounded observer.
	session, err := support.WaitForObservation(concurrentIsolationTimeout, func() (factoryapi.FactorySessionDurableReadModel, error) {
		read := support.GetJSON[factoryapi.FactorySessionGetResponse](t, baseURL+"/factory-sessions/"+started.SessionId)
		return read.AsFactorySessionDurableReadModel()
	}, func(session factoryapi.FactorySessionDurableReadModel) bool {
		return session.Status == factoryapi.FactorySessionDurableLifecycleStatusCanceled
	})
	if err != nil || session.Status != factoryapi.FactorySessionDurableLifecycleStatusCanceled {
		t.Fatalf("durable terminal status = %#v, %v", session, err)
	}
	var rejected factoryapi.FactorySessionLifecycleControlResponse
	lifecycleIsolationPost(t, baseURL+"/factory-sessions/"+started.SessionId+"/resume", map[string]any{}, http.StatusConflict, &rejected)
	if rejected.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeTerminalSession || rejected.Status != factoryapi.FactorySessionDurableLifecycleStatusCanceled {
		t.Fatalf("terminal resume = %#v, want TERMINAL_SESSION/CANCELED", rejected)
	}
	read := support.GetJSON[factoryapi.FactorySessionGetResponse](t, baseURL+"/factory-sessions/"+started.SessionId)
	session, err = read.AsFactorySessionDurableReadModel()
	if err != nil || session.Status != factoryapi.FactorySessionDurableLifecycleStatusCanceled {
		t.Fatalf("durable status after rejected resume = %#v, %v", session, err)
	}
}

func lifecycleIsolationPost(t *testing.T, endpoint string, input any, status int, output any) {
	t.Helper()
	payload, err := json.Marshal(input)
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
	if err := json.NewDecoder(response.Body).Decode(output); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != status {
		t.Fatalf("POST %s status = %d, want %d: %#v", endpoint, response.StatusCode, status, output)
	}
}

func assertLifecycleIsolationRecordedWork(t *testing.T, baseURL, sessionID string, gate *lifecycleIsolationGate, gates [4]*lifecycleIsolationGate) {
	t.Helper()
	listed := support.GetJSON[factoryapi.ListWorkResponse](t, support.SessionWorkURL(baseURL, sessionID, "/work"))
	if len(listed.Results) != 1 {
		t.Fatalf("peer Work count = %d, want one", len(listed.Results))
	}
	work := listed.Results[0]
	if work.WorkId == nil || work.State == nil || work.State.Name != "complete" {
		t.Fatalf("peer Work = %#v, want identified complete Work", work)
	}
	assertLifecycleIsolationNoPeerMarker(t, work, gate, gates)
	workers := support.ListSessionWorkerSessions(t, baseURL, sessionID, *work.WorkId)
	if len(workers.Sessions) != 1 {
		t.Fatalf("peer Worker Sessions = %d, want one recording association", len(workers.Sessions))
	}
	worker := workers.Sessions[0]
	if worker.FactorySessionId == nil || *worker.FactorySessionId != sessionID {
		t.Fatalf("Worker Session scope = %#v, want %s", worker.FactorySessionId, sessionID)
	}
	// replayOnly crosses the public Recordings-backed transcript surface;
	// private artifact storage and decoders never enter this functional cell.
	replay := support.GetWorkerSessionEventsForSessionByIDAt(t, baseURL, sessionID, worker.WorkerSessionId)
	encoded, err := json.Marshal(replay)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), gate.output) {
		t.Fatalf("public recorded Worker transcript omitted %q: %s", gate.output, encoded)
	}
	assertLifecycleIsolationNoPeerMarker(t, replay, gate, gates)
}

func lifecycleIsolationControl(t *testing.T, baseURL, sessionID, action string) factoryapi.FactorySessionLifecycleControlResponse {
	t.Helper()
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + sessionID + "/" + action
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, endpoint, bytes.NewBufferString("{}"))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	result, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer result.Body.Close()
	var response factoryapi.FactorySessionLifecycleControlResponse
	if err := json.NewDecoder(result.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if result.StatusCode != http.StatusOK && result.StatusCode != http.StatusAccepted {
		t.Fatalf("%s status = %d: %#v", action, result.StatusCode, response)
	}
	if response.SessionId != sessionID {
		t.Fatalf("%s response session = %q, want %q", action, response.SessionId, sessionID)
	}
	return response
}

func assertLifecycleIsolationPeerReads(t *testing.T, baseURL, sessionID string, frames []support.FactoryResponseEventFrame, gate *lifecycleIsolationGate, gates [4]*lifecycleIsolationGate) {
	t.Helper()
	assertResponseEventFramesAscendingSequence(t, frames)
	for _, frame := range frames {
		if frame.Event.FactorySessionId != sessionID {
			t.Fatalf("response session = %q, want %q", frame.Event.FactorySessionId, sessionID)
		}
		assertLifecycleIsolationNoPeerMarker(t, frame.Event, gate, gates)
	}
	// Reconnect from an acknowledged response cursor and drain the known suffix.
	if len(frames) < 2 {
		t.Fatalf("peer response frames = %d, want replayable suffix", len(frames))
	}
	reconnected := support.OpenFactoryResponseEventStreamAt(t,
		support.SessionResponseEventsURLWithAfterSequence(baseURL, sessionID, frames[0].Event.Sequence))
	t.Cleanup(reconnected.Close)
	for _, want := range frames[1:] {
		got := reconnected.NextFrame(concurrentIsolationTimeout)
		if got.Event.EventId != want.Event.EventId || got.Event.Sequence != want.Event.Sequence {
			t.Fatalf("response reconnect = %#v, want %#v", got, want)
		}
	}
	events := support.GetFactoryEventsForSessionAt(t, baseURL, sessionID)
	if len(events) < 2 {
		t.Fatalf("peer canonical events = %d", len(events))
	}
	outputSeen := false
	for i, event := range events {
		assertLifecycleIsolationNoPeerMarker(t, event, gate, gates)
		encoded, _ := json.Marshal(event)
		outputSeen = outputSeen || strings.Contains(string(encoded), gate.output)
		// Canonical append order is Sequence. SessionSequence is present only
		// on lifecycle frames in the current live Petri event contract; mixing
		// the two counters does not define an ordering across every event kind.
		if i > 0 && event.Context.Sequence <= events[i-1].Context.Sequence {
			t.Fatalf("canonical append sequences = %d after %d", event.Context.Sequence, events[i-1].Context.Sequence)
		}
	}
	if !outputSeen {
		t.Fatal("canonical peer history omitted its provider output")
	}
	suffix := support.GetFactoryEventsAfterForSessionAt(t, baseURL, sessionID, support.FactoryEventReadCursor{AfterEventID: events[0].Id})
	if len(suffix) != len(events)-1 {
		t.Fatalf("canonical reconnect events = %d, want %d", len(suffix), len(events)-1)
	}
	for i, event := range suffix {
		if event.Id != events[i+1].Id {
			t.Fatal("canonical reconnect changed retained order")
		}
	}
}

func assertLifecycleIsolationNoPeerMarker(t *testing.T, value any, gate *lifecycleIsolationGate, gates [4]*lifecycleIsolationGate) {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	for _, peer := range gates {
		if peer != gate && (strings.Contains(string(encoded), peer.marker) || strings.Contains(string(encoded), peer.output)) {
			t.Fatalf("session %s contains peer marker %s", gate.marker, peer.marker)
		}
	}
}

type lifecycleIsolationGate struct {
	marker, output             string
	started, release, canceled chan struct{}
}

type lifecycleIsolationRunner struct {
	gates   [4]*lifecycleIsolationGate
	durable *lifecycleIsolationGate
}

func newLifecycleIsolationRunner() *lifecycleIsolationRunner {
	runner := &lifecycleIsolationRunner{}
	runner.durable = &lifecycleIsolationGate{marker: "t14-durable-cancel-prompt", started: make(chan struct{}), release: make(chan struct{}), canceled: make(chan struct{})}
	for i := range runner.gates {
		runner.gates[i] = &lifecycleIsolationGate{
			marker: fmt.Sprintf("t14-session-%d-prompt", i), output: fmt.Sprintf("t14-session-%d-output COMPLETE", i),
			started: make(chan struct{}), release: make(chan struct{}), canceled: make(chan struct{}),
		}
	}
	return runner
}

func (runner *lifecycleIsolationRunner) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	prompt := string(request.Stdin) + "\n" + strings.Join(request.Args, "\n")
	for _, gate := range append(runner.gates[:], runner.durable) {
		if !strings.Contains(prompt, gate.marker) {
			continue
		}
		close(gate.started)
		select {
		case <-ctx.Done():
			close(gate.canceled)
			return platformprocess.CommandResult{}, ctx.Err()
		case <-gate.release:
			return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout(gate.output)}, nil
		}
	}
	return platformprocess.CommandResult{}, fmt.Errorf("provider command has no owned session marker")
}
