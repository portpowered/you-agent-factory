package lifecycle_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// One application and HTTP host serve parallel explicit-session scenarios.
// Provider gates, profiles, directories, histories and output are scenario-owned.
func TestCanceledReplayRecordedBoundary(t *testing.T) {
	t.Parallel()
	runner := &boundaryCommandRunner{routes: make(map[string]*boundaryCommandRoute)}
	publication := &boundaryRecordingWriter{published: make(map[string]chan struct{})}
	var ledger recordings.Service
	bound := make(chan http.Handler, 1)
	process := support.BuildProcess(t, serviceedges.Edges{
		ProviderCommandRunner:  runner,
		RecordingWriteFile:     publication.write,
		RecordingsRootObserver: func(service recordings.Service) { ledger = service },
		APIServerStarter: func(ctx context.Context, request platformhttpserver.StartRequest) error {
			bound <- request.Handler
			if request.OnBound != nil {
				request.OnBound(platformhttpserver.Binding{Port: request.Port})
			}
			<-ctx.Done()
			return nil
		},
	})
	hostDir := support.ScaffoldFactory(t, map[string]any{"name": "replay-boundary-host"})
	host := recordingContinuationInputs(t, hostDir, t.TempDir(), []string{"--session", uuid.NewString(), "--continuously", "--with-server", "--no-record"}, false)
	command := support.StartProcessCommand(t, process, host.Input)
	t.Cleanup(func() { command.Stop(t) })
	var handler http.Handler
	select {
	case handler = <-bound:
	case <-time.After(30 * time.Second):
		t.Fatal("boundary host did not bind")
	}
	sessions := process.FactorySessions().(factorysessions.Service)
	for _, name := range []string{"canceled-json", "http-canceled-jsonl", "success-json", "success-jsonl", "resume-and-recorded-retry", "mismatch", "invalid", "peer"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			runReplayBoundaryCase(t, process, sessions, ledger, runner, publication, handler, name)
		})
	}
}

type boundaryRecordingWriter struct {
	mu        sync.Mutex
	published map[string]chan struct{}
}

func (writer *boundaryRecordingWriter) watch(path string) <-chan struct{} {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	published := make(chan struct{})
	writer.published[path] = published
	return published
}

func (writer *boundaryRecordingWriter) write(path string, data []byte) error {
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return err
	}
	writer.mu.Lock()
	defer writer.mu.Unlock()
	if published := writer.published[path]; published != nil {
		close(published)
		delete(writer.published, path)
	}
	return nil
}

type boundaryCommandRoute struct {
	calls   atomic.Int32
	hold    bool
	entered chan struct{}
	joined  chan struct{}
	release chan struct{}
}

type boundaryCommandRunner struct {
	mu     sync.Mutex
	routes map[string]*boundaryCommandRoute
}

func (runner *boundaryCommandRunner) route(dir string, hold bool) *boundaryCommandRoute {
	route := &boundaryCommandRoute{hold: hold, entered: make(chan struct{}), joined: make(chan struct{}), release: make(chan struct{})}
	runner.mu.Lock()
	runner.routes[dir] = route
	runner.mu.Unlock()
	return route
}

func (runner *boundaryCommandRunner) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	runner.mu.Lock()
	route := runner.routes[request.WorkDir]
	runner.mu.Unlock()
	if route == nil {
		return platformprocess.CommandResult{}, fmt.Errorf("unexpected live provider directory %q", request.WorkDir)
	}
	if route.calls.Add(1) == 1 {
		close(route.entered)
		defer close(route.joined)
		if route.hold {
			select {
			case <-ctx.Done():
				return platformprocess.CommandResult{}, ctx.Err()
			case <-route.release:
			}
		}
	}
	return support.NewStaticSuccessCommandRunner("boundary recorded result COMPLETE").Run(ctx, request)
}

type boundarySource struct {
	dir, home, path, id string
	route               *boundaryCommandRoute
	bytes               []byte
	history             recordings.HistoricalRecordingQueryResult
}

func recordBoundarySource(t *testing.T, process support.Process, ledger recordings.Service, runner *boundaryCommandRunner, publication *boundaryRecordingWriter, handler http.Handler, cancelSource, httpStop, jsonl bool) boundarySource {
	t.Helper()
	extension := ".json"
	if jsonl {
		extension = ".jsonl"
	}
	inputs := cancellationRecordingInputs(t, filepath.Join(t.TempDir(), "source"+extension))
	source := boundarySource{dir: inputs.Args[3], home: t.TempDir(), path: inputs.Args[len(inputs.Args)-1], id: uuid.NewString()}
	inputs.Env = isolatedHomeEnvironment(source.home)
	inputs.WorkingDirectory = source.dir
	inputs.Args = append(inputs.Args, "--session", source.id)
	source.route = runner.route(source.dir, cancelSource)
	var published <-chan struct{}
	if cancelSource && !jsonl {
		published = publication.watch(source.path)
	}
	// CLI opening owns this explicitly selected UUID and its recording.
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	inputs.Context = ctx
	done := executeGatedRecordingCommand(t, process, inputs, cancel)
	select {
	case <-source.route.entered:
	case err := <-done:
		t.Fatalf("source before provider: %v stdout=%s stderr=%s", err, inputs.Stdout(), inputs.Stderr())
	case <-time.After(30 * time.Second):
		t.Fatal("source provider admission unavailable")
	}
	waitBoundaryPublication(t, published, done)
	if cancelSource {
		if httpStop {
			response := recordingPeerResponse(t.Context(), handler, http.MethodPost, "/factory-sessions/"+source.id+"/cancel", []byte(`{}`))
			if response.Code != http.StatusOK {
				t.Fatalf("HTTP canceled source: %d %s", response.Code, response.Body)
			}
		} else {
			cancel()
		}
	}
	joinBoundarySource(t, done, cancelSource, httpStop)
	waitRecordingPeerSignal(t, source.route.joined, "source provider join")
	var err error
	source.bytes, err = os.ReadFile(source.path)
	if err != nil {
		t.Fatal(err)
	}
	source.history = boundaryHistory(t, ledger, source.path)
	if !jsonl && !recordingWriteHasTerminalEvent(source.bytes) {
		t.Fatal("joined native source has no recorded run closeout")
	}
	if len(source.history.Dispatches) != 1 {
		t.Fatalf("source dispatches: %#v", source.history.Dispatches)
	}
	assertBoundaryWork(t, source.history, cancelSource)
	return source
}

func waitBoundaryPublication(t *testing.T, published <-chan struct{}, done <-chan error) {
	t.Helper()
	if published == nil {
		return
	}
	// Provider admission can precede startup's first recording publication.
	// A successful artifact write commits recording activation; cancellation
	// then joins its final write rather than aborting an unopened recording.
	select {
	case <-published:
	case err := <-done:
		t.Fatalf("source before recording publication: %v", err)
	case <-time.After(30 * time.Second):
		t.Fatal("source recording was not published")
	}
}

func joinBoundarySource(t *testing.T, done <-chan error, canceled, httpStop bool) {
	t.Helper()
	select {
	case err := <-done:
		if !httpStop && canceled && !errors.Is(err, context.Canceled) {
			t.Fatalf("source cancellation = %v", err)
		}
		if !canceled && err != nil {
			t.Fatalf("record successful source: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("source invocation did not join")
	}
}

func boundaryHistory(t *testing.T, ledger recordings.Service, path string) recordings.HistoricalRecordingQueryResult {
	t.Helper()
	history, err := ledger.QueryHistoricalRecording(recordings.HistoricalRecordingQueryRequest{
		Recording: recordings.HistoricalRecordingIdentity{RecordingID: "boundary-history", Artifact: recordings.RecordingArtifactReference(path)}, InferFactorySessionScope: true,
	})
	if err != nil {
		t.Fatalf("read boundary history: %v", err)
	}
	return history
}

func assertBoundaryWork(t *testing.T, history recordings.HistoricalRecordingQueryResult, canceled bool) {
	t.Helper()
	var world factorydefinitions.FactoryWorldState
	if err := json.Unmarshal([]byte(history.WorldState.Payload), &world); err != nil {
		t.Fatal(err)
	}
	if canceled {
		if history.Dispatches[0].Status != recordings.FactoryDispatchStatusInterrupted || len(world.TerminalWorkByID) != 0 || len(world.WorkItemsByID) != 1 {
			t.Fatalf("canceled dispatch/unfinished Work: %#v %+v", history.Dispatches, world)
		}
		for _, item := range world.WorkItemsByID {
			if item.State != "init" {
				t.Fatalf("canceled Work state: %+v", item)
			}
		}
	} else if len(world.TerminalWorkByID) != 1 || history.Dispatches[len(history.Dispatches)-1].Status != recordings.FactoryDispatchStatusCompleted {
		t.Fatalf("successful history: %#v %+v", history.Dispatches, world)
	}
}

func runReplayBoundaryCase(t *testing.T, process support.Process, sessions factorysessions.Service, ledger recordings.Service, runner *boundaryCommandRunner, publication *boundaryRecordingWriter, handler http.Handler, name string) {
	canceled := !strings.HasPrefix(name, "success") && name != "mismatch" && name != "invalid"
	source := recordBoundarySource(t, process, ledger, runner, publication, handler, canceled, name == "http-canceled-jsonl", strings.HasSuffix(name, "jsonl"))
	if name == "mismatch" || name == "invalid" {
		assertBoundaryRejected(t, process, source, name)
		return
	}
	if name == "peer" {
		assertBoundaryPeer(t, process, sessions, ledger, runner, source)
	} else {
		replayBoundarySource(t, process, sessions, ledger, source, canceled)
	}
	if name == "resume-and-recorded-retry" {
		successor := filepath.Join(t.TempDir(), "retry.json")
		inputs := recordingContinuationInputs(t, source.dir, t.TempDir(), []string{"--session", uuid.NewString(), "--resume", source.path, "--record", successor}, false)
		inputs.WorkingDirectory = source.dir
		if err := process.Execute(inputs.Input); err != nil {
			t.Fatalf("live resume: %v", err)
		}
		if source.route.calls.Load() != 2 {
			t.Fatalf("resume calls = %d, want one retry", source.route.calls.Load())
		}
		history := boundaryHistory(t, ledger, successor)
		if len(history.Dispatches) != 2 || history.Dispatches[0].Status != recordings.FactoryDispatchStatusInterrupted {
			t.Fatalf("recorded retry: %#v", history.Dispatches)
		}
		assertBoundaryWork(t, history, false)
		if !reflect.DeepEqual(source.bytes, mustBoundaryBytes(t, source.path)) {
			t.Fatal("live resume rewrote its canceled source")
		}
		assertBoundaryWorkIdentity(t, source.history, history)
		source.path, source.bytes, source.history = successor, mustBoundaryBytes(t, successor), history
		replayBoundarySource(t, process, sessions, ledger, source, false)
	}
}

func mustBoundaryBytes(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func replayBoundarySource(t *testing.T, process support.Process, sessions factorysessions.Service, ledger recordings.Service, source boundarySource, canceled bool) {
	t.Helper()
	inputs := recordingContinuationInputs(t, source.dir, t.TempDir(), []string{"--session", uuid.NewString(), "--replay", source.path, "--no-record"}, false)
	inputs.WorkingDirectory = source.dir
	beforeCalls := source.route.calls.Load()
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("replay boundary: %v\n%s", err, inputs.Stderr())
	}
	if source.route.calls.Load() != beforeCalls || !reflect.DeepEqual(source.bytes, mustBoundaryBytes(t, source.path)) {
		t.Fatal("CLI replay had a live effect or changed source")
	}
	retained := boundaryHistory(t, ledger, source.path)
	if !reflect.DeepEqual(source.history.Events, retained.Events) {
		t.Fatal("replay changed ordered canonical recording identities/facts")
	}
	assertBoundaryWork(t, retained, canceled)
	// JSONL follows the existing read-only historical path, with no executable
	// live lifecycle to inspect. Native JSON also reconstructs a live runtime.
	if strings.HasSuffix(source.path, ".jsonl") {
		return
	}
	assertBoundaryRuntime(t, sessions, source, canceled)
	if source.route.calls.Load() != beforeCalls || !reflect.DeepEqual(source.bytes, mustBoundaryBytes(t, source.path)) {
		t.Fatal("observable replay had a live effect or changed source")
	}
}

func openBoundaryRuntime(t *testing.T, sessions factorysessions.Service, source boundarySource) string {
	t.Helper()
	// Keep a second public replay opening alive for selected state/history
	// reads. CLI command cleanup otherwise retires its runtime before reads.
	id := uuid.NewString()
	_, err := sessions.Start(t.Context(), factorysessions.SessionStartRequest{
		SessionID: id, Mode: factorysessions.SessionOperationModeLive, FolderPath: source.dir, ActivationOnly: true,
		RuntimeSelection: &factorysessions.SessionRuntimeSelection{Mode: factorysessions.SessionRuntimeModeService,
			SystemConfigHome: t.TempDir(), RuntimeInstanceID: uuid.NewString(), ExecutionBaseDir: source.dir,
			Recording: factorysessions.SessionRecordingSelection{ReplayPath: source.path}},
	})
	if err != nil {
		t.Fatalf("open observable replay: %v", err)
	}
	t.Cleanup(func() {
		_, err := sessions.Control(context.WithoutCancel(t.Context()), factorysessions.SessionControlRequest{SessionID: id, Mode: factorysessions.SessionOperationModeLive, Operation: factorysessions.SessionControlClose})
		if err != nil {
			t.Errorf("join observable replay: %v", err)
		}
	})
	return id
}

func assertBoundaryRuntime(t *testing.T, sessions factorysessions.Service, source boundarySource, canceled bool) {
	t.Helper()
	id := openBoundaryRuntime(t, sessions, source)
	// Replay exposes the immutable recorded terminal events immediately, so
	// their subscription cannot signal that this runtime has finished applying
	// results. Observe the selected public runtime status for that distinction.
	read, err := support.WaitForObservation(15*time.Second, func() (factorysessions.SessionProjection, error) {
		return sessions.GetFactorySession(t.Context(), id)
	}, func(read factorysessions.SessionProjection) bool { return read.Runtime.Status == "FINISHED" })
	if err != nil {
		t.Fatalf("replay completion: status=%s progress=%+v err=%v", read.Runtime.Status, read.Runtime.Progress, err)
	}
	assertBoundaryRuntimeWork(t, read, source.history, canceled)
	stream, err := sessions.SubscribeFactoryEventsForSession(t.Context(), id, nil)
	if err != nil || stream == nil {
		t.Fatalf("replayed history absent: %v", err)
	}
	assertBoundaryEventOrder(t, stream.History, source.history.Events)
}

func assertBoundaryRuntimeWork(t *testing.T, read factorysessions.SessionProjection, history recordings.HistoricalRecordingQueryResult, canceled bool) {
	t.Helper()
	wantState := "task:complete"
	if canceled {
		wantState = "task:init"
	}
	if read.Runtime.Petri == nil || len(read.Runtime.Petri.Marking) != 1 || read.Runtime.Petri.Marking[0].PlaceID != wantState || read.Runtime.Progress.InFlightCount != 0 {
		t.Fatalf("replayed Work/inflight: %+v", read.Runtime)
	}
	var world factorydefinitions.FactoryWorldState
	if err := json.Unmarshal([]byte(history.WorldState.Payload), &world); err != nil {
		t.Fatal(err)
	}
	workID := read.Runtime.Petri.Marking[0].WorkID
	if _, exists := world.WorkItemsByID[workID]; !exists {
		if _, terminal := world.TerminalWorkByID[workID]; !terminal {
			t.Fatalf("replay invented Work identity %s", workID)
		}
	}
}

func boundaryWorkOrDispatch(kind string) bool {
	return kind == "WORK_REQUEST" || kind == "WORK_STATE_CHANGE" || kind == "DISPATCH_REQUEST" || kind == "DISPATCH_RESPONSE"
}

func assertBoundaryEventOrder(t *testing.T, observed []factorydefinitions.FactoryEvent, expected []recordings.CanonicalEvent) {
	t.Helper()
	// The live stream regenerates run/session closeout and includes Worker
	// observations; immutable canonical history is checked through Recordings.
	// Verify actual Work/dispatch facts retain identity, outcome and order.
	var wanted []recordings.CanonicalEvent
	for _, event := range expected {
		if boundaryWorkOrDispatch(string(event.Kind)) {
			wanted = append(wanted, event)
		}
	}
	next, previous := 0, -1
	for _, event := range observed {
		if event.Context.Sequence <= previous {
			t.Fatal("live replay lost event order")
		}
		previous = event.Context.Sequence
		if !boundaryWorkOrDispatch(string(event.Type)) {
			continue
		}
		if next >= len(wanted) || string(event.Type) != string(wanted[next].Kind) {
			t.Fatalf("unrecorded/reordered Work or dispatch event %s/%s, expected index %d", event.Id, event.Type, next)
		}
		if event.Type == factorydefinitions.FactoryEventTypeWorkStateChange {
			// Relocation event IDs include the regenerated live tick/sequence;
			// their customer-visible Work identity and state must stay identical.
			assertBoundaryRelocation(t, event.Payload, []byte(wanted[next].Payload))
		} else if event.Id != string(wanted[next].ID) {
			t.Fatalf("replay changed Work/dispatch identity %s, want %s", event.Id, wanted[next].ID)
		}
		if event.Type == factorydefinitions.FactoryEventTypeDispatchResponse {
			assertBoundaryDispatchOutcome(t, event.Payload, []byte(wanted[next].Payload))
		}
		next++
	}
	if next != len(wanted) {
		t.Fatalf("replay retained %d/%d ordered Work/dispatch identities", next, len(wanted))
	}
}

func assertBoundaryDispatchOutcome(t *testing.T, observed, expected []byte) {
	t.Helper()
	var got, want struct {
		Outcome string `json:"outcome"`
	}
	if err := json.Unmarshal(observed, &got); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(expected, &want); err != nil {
		t.Fatal(err)
	}
	if got.Outcome != want.Outcome {
		t.Fatalf("replay dispatch outcome = %s, want %s", got.Outcome, want.Outcome)
	}
}

func assertBoundaryRelocation(t *testing.T, observed, expected []byte) {
	t.Helper()
	var got, want factorydefinitions.WorkStateChangeEventPayload
	if err := json.Unmarshal(observed, &got); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(expected, &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("replay changed Work relocation: got=%+v want=%+v", got, want)
	}
}

func assertBoundaryWorkIdentity(t *testing.T, before, after recordings.HistoricalRecordingQueryResult) {
	t.Helper()
	var left, right factorydefinitions.FactoryWorldState
	if err := json.Unmarshal([]byte(before.WorldState.Payload), &left); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(after.WorldState.Payload), &right); err != nil {
		t.Fatal(err)
	}
	for id := range left.WorkItemsByID {
		if _, exists := right.WorkItemsByID[id]; !exists {
			if _, terminal := right.TerminalWorkByID[id]; !terminal {
				t.Fatalf("replay/resume lost Work identity %s", id)
			}
		}
	}
	for id := range left.TerminalWorkByID {
		if _, exists := right.TerminalWorkByID[id]; !exists {
			t.Fatalf("replay lost terminal Work identity %s", id)
		}
	}
}

func assertBoundaryRejected(t *testing.T, process support.Process, source boundarySource, name string) {
	t.Helper()
	path := writeBoundaryInvalidInput(t, source, name)
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	inputs := recordingContinuationInputs(t, source.dir, t.TempDir(), []string{"--session", uuid.NewString(), "--replay", path, "--no-record"}, false)
	inputs.Context, inputs.WorkingDirectory = ctx, source.dir
	err := process.Execute(inputs.Input)
	if err == nil || errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("%s rejection = %v", name, err)
	}
	if name == "mismatch" && (!strings.Contains(err.Error(), "category=dispatch_mismatch") || !strings.Contains(err.Error(), "expected=") || !strings.Contains(err.Error(), "observed=")) {
		t.Fatalf("genuine mismatch = %v", err)
	}
	if name == "invalid" {
		malformed := filepath.Join(t.TempDir(), "malformed.json")
		if err := os.WriteFile(malformed, []byte(`{"schemaVersion":"agent-factory.replay.v1","events":[`), 0o600); err != nil {
			t.Fatal(err)
		}
		for i, arg := range inputs.Args {
			if arg == path {
				inputs.Args[i] = malformed
			}
		}
		if err := process.Execute(inputs.Input); err == nil || errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("malformed replay rejection = %v", err)
		}
	}
	if source.route.calls.Load() != 1 {
		t.Fatal("invalid replay invoked live provider")
	}
	if !reflect.DeepEqual(source.bytes, mustBoundaryBytes(t, source.path)) {
		t.Fatal("negative replay changed original")
	}
}

func writeBoundaryInvalidInput(t *testing.T, source boundarySource, name string) string {
	t.Helper()
	var artifact factorydefinitions.ReplayArtifact
	if err := json.Unmarshal(source.bytes, &artifact); err != nil {
		t.Fatal(err)
	}
	for i, event := range artifact.Events {
		if event.Type != factorydefinitions.FactoryEventTypeDispatchRequest {
			continue
		}
		if name == "mismatch" {
			var payload map[string]any
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatal(err)
			}
			payload["transitionId"] = "incompatible-recorded-transition"
			artifact.Events[i].Payload, _ = json.Marshal(payload)
		} else {
			artifact.Events = artifact.Events[:i+1]
		}
		break
	}
	path := filepath.Join(t.TempDir(), name+".json")
	encoded, err := json.Marshal(artifact)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func assertBoundaryPeer(t *testing.T, process support.Process, sessions factorysessions.Service, ledger recordings.Service, runner *boundaryCommandRunner, source boundarySource) {
	t.Helper()
	peerInputs := cancellationRecordingInputs(t, filepath.Join(t.TempDir(), "peer.json"))
	peerDir, peerPath := peerInputs.Args[3], peerInputs.Args[len(peerInputs.Args)-1]
	peerInputs.WorkingDirectory = peerDir
	peerID := uuid.NewString()
	peerInputs.Args = append(peerInputs.Args, "--session", peerID)
	peerRoute := runner.route(peerDir, true)
	var once sync.Once
	release := func() { once.Do(func() { close(peerRoute.release) }) }
	done := executeGatedRecordingCommand(t, process, peerInputs, release)
	waitRecordingPeerSignal(t, peerRoute.entered, "live peer admission")
	replayBoundarySource(t, process, sessions, ledger, source, true)
	assertBoundaryRejected(t, process, source, "mismatch")
	release()
	assertSelectedReplayCommandJoined(t, done)
	waitRecordingPeerSignal(t, peerRoute.joined, "live peer join")
	if peerRoute.calls.Load() != 1 || peerInputs.Stdout() != "Batch completed successfully.\n" {
		t.Fatalf("peer output/calls: %q %d", peerInputs.Stdout(), peerRoute.calls.Load())
	}
	assertBoundaryWork(t, boundaryHistory(t, ledger, peerPath), false)
}
