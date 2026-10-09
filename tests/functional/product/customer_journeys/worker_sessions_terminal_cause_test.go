package customer_journeys_test

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/recordings"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func assertCapturedStopCause(t *testing.T, baseURL, id, want string) {
	t.Helper()
	shown := support.GetJSON[factoryapi.WorkerSessionObservation](t, baseURL+"/worker-sessions/"+id)
	if shown.TerminalCause == nil || string(*shown.TerminalCause) != want {
		t.Fatalf("terminalCause=%v, want %s", shown.TerminalCause, want)
	}
}

// Production root composition, HTTP and real capture store; only the provider
// command is controlled. Each isolated fixture owns its direct Worker,
// readiness gate, journal, profile and terminal callback.
func TestExactStopTerminateReportsCommittedCause(t *testing.T) {
	t.Parallel()
	runner := newFunctionalWorkerGate(make(chan struct{}))
	server := startDirectWorkerSessionServer(t, runner)
	start := postDirectWorkerSession(t, t.Context(), server.URL(), "cause-request", "cause-worker", "cause-attempt")
	_ = start.Body.Close()
	if start.StatusCode != http.StatusAccepted {
		t.Fatalf("admission status=%d", start.StatusCode)
	}
	runner.waitStarted(t)
	live := support.GetJSON[factoryapi.WorkerSessionObservation](t, server.URL()+"/worker-sessions/cause-worker")
	if live.TerminalCause != nil {
		t.Fatal("live Worker has terminalCause")
	}
	stop := postWorkerSessionControl(t, server.URL(), "cause-worker", "terminate")
	_ = stop.Body.Close()
	if stop.StatusCode != http.StatusOK {
		t.Fatalf("terminate status=%d", stop.StatusCode)
	}
	runner.waitCanceled(t)
	assertCapturedStopCause(t, server.URL(), "cause-worker", "OPERATOR_TERMINATE")
	inputs := support.FakeInputs(t.Context(), []string{"you", "--server", server.URL(), "worker-sessions", "show", "--worker-session-id", "cause-worker", "--output", "json"})
	if err := server.Execute(t, inputs.Input); err != nil {
		t.Fatalf("CLI summary: %v %s", err, inputs.Stderr())
	}
	var cli factoryapi.WorkerSessionObservation
	if err := json.Unmarshal([]byte(inputs.Stdout()), &cli); err != nil || cli.TerminalCause == nil || string(*cli.TerminalCause) != "OPERATOR_TERMINATE" {
		t.Fatalf("CLI terminal cause = %v, %v", cli.TerminalCause, err)
	}
	repeat := postWorkerSessionControl(t, server.URL(), "cause-worker", "cancel")
	_ = repeat.Body.Close()
	if repeat.StatusCode != http.StatusOK {
		t.Fatalf("terminal repeat status=%d", repeat.StatusCode)
	}
	assertCapturedStopCause(t, server.URL(), "cause-worker", "OPERATOR_TERMINATE")
}

// The Factory Session close owns an ordered stop/join. Provider cancellation
// and completion are separate gates; retained reads use the same root process.
func TestShutdownTerminalCausePublicParity(t *testing.T) {
	t.Parallel()
	runner := newFleetCharacterizationRunner()
	sessionID := uuid.NewString()
	server := startShutdownCauseServer(t, runner, sessionID)
	stream := support.OpenFactoryEventStreamAt(t, support.SessionEventsURL(server.URL(), sessionID))
	name := "shutdown-cause"
	submitted := support.SubmitSessionWorkAt(t, server.URL(), sessionID, factoryapi.SubmitWorkRequest{
		Name: &name, WorkTypeName: "task", Payload: map[string]string{"title": name},
	})
	waitFleetCharacterizationSignal(t, runner.slots[0].started, "owned provider admission")
	target := waitForRouteCharacterizationAssociation(t, stream, support.StringPointerValue(submitted.WorkId))
	live := support.GetJSON[factoryapi.WorkerSessionObservation](t, server.URL()+"/worker-sessions/"+target.workerSessionID)
	if live.TerminalCause != nil || live.StartedAt == nil || live.ProviderSession != nil {
		t.Fatalf("live no-reference attempt = %+v", live)
	}
	released := make(chan struct{})
	go func() {
		select {
		case <-runner.slots[0].canceled:
			runner.slots[0].release()
		case <-runner.slots[0].returned:
		}
		close(released)
	}()
	support.CloseFactorySessionAt(t, server.URL(), sessionID)
	waitFleetCharacterizationSignal(t, released, "joined cancellation")
	waitFleetCharacterizationSignal(t, runner.slots[0].canceled, "owned cancellation effect")
	archived := support.GetJSON[factoryapi.WorkerSessionObservation](t, server.URL()+"/worker-sessions/"+target.workerSessionID)
	if archived.State != "CANCELED" || archived.TerminalCause == nil || string(*archived.TerminalCause) != "OPERATOR_CANCEL" ||
		!reflect.DeepEqual(live.StartedAt, archived.StartedAt) || archived.EndedAt == nil || archived.DurationMillis == nil {
		t.Fatalf("shutdown archive facts = %+v", archived)
	}
	assertShutdownCLIHTTPParity(t, server, target.workerSessionID, archived)
	if runner.callCount() != 1 {
		t.Fatalf("closed Runtime admitted %d executions, want one", runner.callCount())
	}
}

func assertShutdownCLIHTTPParity(t *testing.T, server *fleetCharacterizationServer, workerSessionID string, archived factoryapi.WorkerSessionObservation) {
	t.Helper()
	inputs := support.FakeInputs(t.Context(), []string{"you", "--server", server.URL(), "worker-sessions", "show", "--worker-session-id", workerSessionID, "--output", "json"})
	if err := server.Execute(t, inputs.Input); err != nil {
		t.Fatalf("CLI shutdown summary: %v %s", err, inputs.Stderr())
	}
	var shown factoryapi.WorkerSessionObservation
	if err := json.Unmarshal([]byte(inputs.Stdout()), &shown); err != nil || !reflect.DeepEqual(shown, archived) {
		t.Fatalf("CLI/HTTP shutdown summary disagree: %s (%v)", inputs.Stdout(), err)
	}
}

func startShutdownCauseServer(t *testing.T, runner *fleetCharacterizationRunner, sessionID string) *fleetCharacterizationServer {
	return startScopedShutdownServer(t, runner, nil, sessionID)
}

func startScopedShutdownServer(t *testing.T, runner *fleetCharacterizationRunner, opening *shutdownOpeningGate, sessionIDs ...string) *fleetCharacterizationServer {
	t.Helper()
	dir := support.ScaffoldSingleStepFactory(t, "shutdown-cause")
	support.WriteAgentConfig(t, dir, "processor", support.BuildModelWorkerConfig(models.ProviderCodex, "test-model"))
	home := t.TempDir()
	server := &fleetCharacterizationServer{env: []string{"HOME=" + home, "USERPROFILE=" + home}, dir: dir}
	edges := serviceedges.Edges{ProviderCommandRunner: runner, FactorySessionsWorkingDirectory: capturedRecordingDirectory(dir)}
	if opening != nil {
		edges.WorkerRecordingWriter = opening
		edges.WorkerRecordingStoreObserver = func(store recordings.WorkerRecordingStore) { opening.WorkerRecordingStore = store }
	}
	server.FunctionalAPIServer = support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir, WaitForServiceModeRuntime: true, Env: server.env,
		Edges: edges,
		BeforeStart: func(tb testing.TB, process support.Process, _ root.Input) {
			for _, sessionID := range sessionIDs {
				result, err := process.(support.ApplicationProcess).FactorySessions().Start(tb.Context(), factorysessions.SessionStartRequest{
					SessionID: sessionID, Mode: factorysessions.SessionOperationModeLive, FolderPath: dir, ActivationOnly: true,
					RuntimeSelection: &factorysessions.SessionRuntimeSelection{
						Mode: factorysessions.SessionRuntimeModeService, SystemConfigHome: home,
						DefinitionSourcePath: filepath.Join(dir, "factory.json"), ExecutionBaseDir: dir, RuntimeInstanceID: uuid.NewString(),
						Recording: factorysessions.SessionRecordingSelection{RecordPath: filepath.Join(dir, sessionID+".json")},
					},
				})
				if err != nil || result.SessionID != sessionID {
					tb.Fatalf("activate shutdown Factory Session: %+v %v", result, err)
				}
			}
		},
	})
	t.Cleanup(func() { server.Stop(t) })
	t.Cleanup(runner.releaseAll)
	if opening != nil {
		t.Cleanup(opening.release)
	}
	return server
}

// One production graph owns two pre-activated runtimes. Ordered submissions
// bind immutable provider gates; neither stop nor close may cancel the peer.
func TestShutdownTerminalCauseScopedPeers(t *testing.T) {
	t.Parallel()
	runner := newFleetCharacterizationRunner()
	for range 2 {
		runner.slots = append(runner.slots, &fleetCharacterizationSlot{started: make(chan struct{}), canceled: make(chan struct{}), returned: make(chan struct{}), gate: make(chan struct{})})
	}
	owned, peer := uuid.NewString(), uuid.NewString()
	server := startScopedShutdownServer(t, runner, nil, owned, peer)
	stream := support.OpenFactoryEventStreamAt(t, support.SessionEventsURL(server.URL(), owned))
	target := submitShutdownWork(t, server, stream, runner.slots[0], owned, "selected")
	sibling := submitShutdownWork(t, server, stream, runner.slots[1], owned, "sibling")
	joinedExactFactoryStop(t, server, runner.slots[0], target, "cancel", true)
	assertShutdownRunning(t, server, runner.slots[1], sibling)
	// Existing Runtime policy requeues canceled Work. Its distinct replacement
	// must be observed, then drained by close without canceling it a second time.
	waitFleetCharacterizationSignal(t, runner.slots[2].started, "replacement admission")
	replacement := waitForRouteCharacterizationAssociation(t, stream, target.workID)
	if replacement.workerSessionID == target.workerSessionID || replacement.dispatchID == target.dispatchID {
		t.Fatal("retry reused the canceled physical attempt")
	}
	peerStream := support.OpenFactoryEventStreamAt(t, support.SessionEventsURL(server.URL(), peer))
	peerAttempt := submitShutdownWork(t, server, peerStream, runner.slots[3], peer, "peer")
	closeShutdownScope(t, server, owned, runner.slots[1], runner.slots[2])
	for _, attempt := range []routeCharacterizationDispatch{target, sibling, replacement} {
		assertCapturedStopCause(t, server.URL(), attempt.workerSessionID, "OPERATOR_CANCEL")
		assertShutdownSingleTerminal(t, server, attempt.workerSessionID)
	}
	assertShutdownRunning(t, server, runner.slots[3], peerAttempt)
	secondPeer := submitShutdownWork(t, server, peerStream, runner.slots[4], peer, "peer-after-close")
	assertShutdownAdmissionRefused(t, server, owned)
	if runner.callCount() != 5 {
		t.Fatalf("provider calls=%d, want two owned, one retry and two peer admissions", runner.callCount())
	}
	closeShutdownScope(t, server, peer, runner.slots[3], runner.slots[4])
	assertShutdownSingleTerminal(t, server, secondPeer.workerSessionID)
}

func submitShutdownWork(t *testing.T, server *fleetCharacterizationServer, stream *support.FactoryEventStream, slot *fleetCharacterizationSlot, scope, name string) routeCharacterizationDispatch {
	t.Helper()
	submitted := support.SubmitSessionWorkAt(t, server.URL(), scope, factoryapi.SubmitWorkRequest{Name: &name, WorkTypeName: "task", Payload: map[string]string{"title": name}})
	waitFleetCharacterizationSignal(t, slot.started, name+" provider admission")
	return waitForRouteCharacterizationAssociation(t, stream, support.StringPointerValue(submitted.WorkId))
}

func assertShutdownRunning(t *testing.T, server *fleetCharacterizationServer, slot *fleetCharacterizationSlot, attempt routeCharacterizationDispatch) {
	t.Helper()
	select {
	case <-slot.canceled:
		t.Fatal("unselected attempt was canceled")
	default:
	}
	assertFleetCharacterizationSnapshot(t, server, attempt.workerSessionID, attempt.dispatchID, "RUNNING")
}

func closeShutdownScope(t *testing.T, server *fleetCharacterizationServer, scope string, slots ...*fleetCharacterizationSlot) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		for _, slot := range slots {
			select {
			case <-slot.canceled:
				slot.release()
			case <-slot.returned:
			}
		}
		close(done)
	}()
	support.CloseFactorySessionAt(t, server.URL(), scope)
	waitFleetCharacterizationSignal(t, done, "scoped joined close")
	for _, slot := range slots {
		waitFleetCharacterizationSignal(t, slot.canceled, "scoped cancellation")
		waitFleetCharacterizationSignal(t, slot.returned, "scoped execution joined")
	}
}

func assertShutdownSingleTerminal(t *testing.T, server *fleetCharacterizationServer, id string) {
	t.Helper()
	logs := support.GetJSON[factoryapi.WorkerSessionLogPage](t, server.URL()+"/worker-sessions/"+id+"/logs")
	terminals := 0
	for _, event := range logs.Events {
		if event.Event.SourceEventId == "terminal" {
			terminals++
		}
	}
	if logs.Health != factoryapi.COMPLETE || terminals != 1 {
		t.Fatalf("closed capture health=%s next=%v terminals=%d", logs.Health, logs.NextToken, terminals)
	}
}

func assertShutdownAdmissionRefused(t *testing.T, server *fleetCharacterizationServer, scope string) {
	t.Helper()
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL()+"/factory-sessions/"+scope+"/work", strings.NewReader(`{"name":"after-close","workTypeName":"task","payload":{"title":"after-close"}}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var failure factoryapi.ErrorResponse
	if err := json.NewDecoder(response.Body).Decode(&failure); err != nil || response.StatusCode != http.StatusNotFound || failure.Code != "NOT_FOUND" {
		t.Fatalf("closed admission status=%d failure=%+v error=%v", response.StatusCode, failure, err)
	}
}

// Pause the existing capture write at opening acknowledgement. Runtime close
// must wait for preparation, then journal and join the exact admitted attempt.
type shutdownOpeningGate struct {
	recordings.WorkerRecordingStore
	entered chan string
	proceed chan struct{}
	once    sync.Once
}

func (store *shutdownOpeningGate) release() { store.once.Do(func() { close(store.proceed) }) }

func (store *shutdownOpeningGate) LookupWorkerSessionSummary(ctx context.Context, id string) (recordings.WorkerCapturedSummary, error) {
	return store.WorkerRecordingStore.(recordings.WorkerCapturedSummaryReader).LookupWorkerSessionSummary(ctx, id)
}

func (store *shutdownOpeningGate) RecoverWorkerOwners(ctx context.Context) error {
	return store.WorkerRecordingStore.(interface{ RecoverWorkerOwners(context.Context) error }).RecoverWorkerOwners(ctx)
}

func (store *shutdownOpeningGate) PersistWorkerRecord(ctx context.Context, record recordings.WorkerRecordingRecord) error {
	if record.Record.ID.Position == 1 {
		select {
		case store.entered <- record.WorkerSessionID:
			select {
			case <-store.proceed:
			case <-ctx.Done():
				return ctx.Err()
			}
		default:
		}
	}
	return store.WorkerRecordingStore.PersistWorkerRecord(ctx, record)
}

func TestShutdownTerminalCauseOpeningClose(t *testing.T) {
	t.Parallel()
	runner := newFleetCharacterizationRunner()
	opening := &shutdownOpeningGate{entered: make(chan string, 1), proceed: make(chan struct{})}
	owned, peer := uuid.NewString(), uuid.NewString()
	server := startScopedShutdownServer(t, runner, opening, owned, peer)
	stream := support.OpenFactoryEventStreamAt(t, support.SessionEventsURL(server.URL(), owned))
	name := "opening-close"
	submitted := make(chan struct{})
	go func() {
		support.SubmitSessionWorkAt(t, server.URL(), owned, factoryapi.SubmitWorkRequest{Name: &name, WorkTypeName: "task", Payload: map[string]string{"title": name}})
		close(submitted)
	}()
	t.Cleanup(func() {
		opening.release()
		waitFleetCharacterizationSignal(t, submitted, "submission cleanup joined")
	})
	var id string
	select {
	case id = <-opening.entered:
	case <-time.After(functionalWorkerSignalTimeout):
		t.Fatal("opening did not reach capture")
	}
	closed := make(chan struct{})
	go func() {
		support.CloseFactorySessionAt(t, server.URL(), owned)
		close(closed)
	}()
	t.Cleanup(func() {
		opening.release()
		runner.releaseAll()
		waitFleetCharacterizationSignal(t, closed, "close cleanup joined")
	})
	// This committed public transition is emitted before close joins preparation.
	for {
		event := stream.NextEvent(functionalWorkerSignalTimeout)
		if event.Type != factoryapi.FactoryEventTypeFactoryStateResponse {
			continue
		}
		payload, err := event.Payload.AsFactoryStateResponseEventPayload()
		if err != nil {
			t.Fatal(err)
		}
		if payload.State == factoryapi.FactoryStateCompleted {
			break
		}
	}
	select {
	case <-closed:
		t.Fatal("close returned before opening acknowledgement")
	default:
	}
	if runner.callCount() != 0 {
		t.Fatal("provider admitted before opening acknowledgement")
	}
	opening.release()
	waitFleetCharacterizationSignal(t, submitted, "opening submission returned")
	waitFleetCharacterizationSignal(t, runner.slots[0].started, "admitted opening")
	waitFleetCharacterizationSignal(t, runner.slots[0].canceled, "opening canceled through scoped close")
	runner.slots[0].release()
	waitFleetCharacterizationSignal(t, closed, "opening close joined")
	assertCapturedStopCause(t, server.URL(), id, "OPERATOR_CANCEL")
	assertShutdownSingleTerminal(t, server, id)
	assertShutdownAdmissionRefused(t, server, owned)
	peerStream := support.OpenFactoryEventStreamAt(t, support.SessionEventsURL(server.URL(), peer))
	peerAttempt := submitShutdownWork(t, server, peerStream, runner.slots[1], peer, "peer-after-opening-close")
	assertShutdownRunning(t, server, runner.slots[1], peerAttempt)
	if runner.callCount() != 2 {
		t.Fatalf("opening close admitted extra execution: calls=%d", runner.callCount())
	}
	closeShutdownScope(t, server, peer, runner.slots[1])
}
