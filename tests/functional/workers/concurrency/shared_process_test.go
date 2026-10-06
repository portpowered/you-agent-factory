package concurrency_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	concurrencySharedProcessTimeout   = 20 * time.Second
	concurrencyForcedCleanupChildEnv  = "YOU_CONCURRENCY_FORCED_CLEANUP_CHILD"
	concurrencyForcedCleanupReportEnv = "YOU_CONCURRENCY_FORCED_CLEANUP_REPORT"
	concurrencyFailureMessage         = "concurrency controlled authentication failure"
)

type concurrencyRunnerBehavior string

const (
	concurrencyRunnerSuccess       concurrencyRunnerBehavior = "success"
	concurrencyRunnerHold          concurrencyRunnerBehavior = "hold"
	concurrencyRunnerLateOutput    concurrencyRunnerBehavior = "late-output"
	concurrencyRunnerFailureHold   concurrencyRunnerBehavior = "failure-hold"
	concurrencyRunnerTimeoutMarker concurrencyRunnerBehavior = "timeout-marker"
)

// TestConcurrencySharedProcess keeps the retained two-session witness and the
// concurrency matrix on one root-built process. Each scenario owns a distinct
// explicit Factory Session and an immutable factory-directory route; barriers
// live at the controlled command edge so the scheduler remains genuinely
// concurrent and no fixture-wide scenario lock can hide capacity behavior.
func TestConcurrencySharedProcess(t *testing.T) {
	if os.Getenv(concurrencyForcedCleanupChildEnv) == "1" {
		runConcurrencyForcedCleanupChild(t)
		return
	}
	t.Parallel()

	fixture := newConcurrencySharedProcessFixture(t)
	fixture.start(t)

	t.Run("Capacity", func(t *testing.T) {
		t.Parallel()
		t.Run("CC-01", func(t *testing.T) { t.Parallel(); fixture.runCapacityOne(t) })
		t.Run("CC-02", func(t *testing.T) { t.Parallel(); fixture.runCapacityTwo(t) })
		t.Run("CC-06", func(t *testing.T) { t.Parallel(); fixture.runIdempotentRequest(t) })
		t.Run("CC-07", func(t *testing.T) { t.Parallel(); fixture.runDuplicateConflict(t) })
		t.Run("CC-08", func(t *testing.T) { t.Parallel(); fixture.runEmptyRequest(t) })
	})
	t.Run("Concurrent", func(t *testing.T) {
		t.Parallel()
		t.Run("CC-03", func(t *testing.T) { t.Parallel(); fixture.runConcurrentSessionIsolation(t) })
		t.Run("CC-10", func(t *testing.T) { t.Parallel(); fixture.runPartialFailure(t) })
		t.Run("CC-12", func(t *testing.T) { t.Parallel(); fixture.runSessionOrdering(t) })
	})
	t.Run("Cancel", func(t *testing.T) {
		t.Parallel()
		t.Run("AdmittedWorkCancellation", func(t *testing.T) { t.Parallel(); fixture.runAdmittedWorkCancellation(t) })
		t.Run("CC-04", func(t *testing.T) { t.Parallel(); fixture.runSessionCancellationIsolation(t) })
		t.Run("CC-05", func(t *testing.T) { t.Parallel(); fixture.runWorkerSessionCancellation(t) })
		t.Run("CC-13", func(t *testing.T) { t.Parallel(); fixture.runRecovery(t) })
	})
	t.Run("Timeout", func(t *testing.T) { t.Parallel(); fixture.runTimeoutRecovery(t) })
	t.Run("Cleanup", func(t *testing.T) { t.Parallel(); runConcurrencyForcedCleanupParent(t) })
}

type concurrencySharedProcessFixture struct {
	process    support.ApplicationProcess
	command    *support.ProcessCommand
	api        *support.ProcessAPIServer
	apiClosed  chan struct{}
	apiClose   sync.Once
	baseURL    string
	hostDir    string
	homeDir    string
	router     *concurrencyCommandRouter
	identities *concurrencyIdentityGenerator

	processBuilds   atomic.Int32
	apiStarts       atomic.Int32
	processClosed   atomic.Bool
	processCloseMu  sync.Mutex
	processCloseErr string

	sessionsMu sync.Mutex
	opened     map[string]string
	closed     map[string]struct{}
	ownedDirs  []string
	sessions   map[string]*concurrencySession
}

type concurrencySession struct {
	fixture   *concurrencySharedProcessFixture
	name      string
	dir       string
	marker    string
	runner    *concurrencyScenarioRunner
	id        string
	stream    factoryapi.FactorySessionStreamIdentity
	closeOnce sync.Once
}

func newConcurrencySharedProcessFixture(t *testing.T) *concurrencySharedProcessFixture {
	t.Helper()

	hostDir := scaffoldConcurrencyFactory(t, "concurrency-host", 1, 1)
	support.ClearSeedInputs(t, hostDir)
	support.WriteAgentConfig(t, hostDir, "worker-a", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "concurrency-host-model"))
	support.WriteWorkstationConfig(t, hostDir, "process", concurrencyWorkstationConfig(1))

	fixture := &concurrencySharedProcessFixture{
		api:        support.NewProcessAPIServer(),
		apiClosed:  make(chan struct{}),
		hostDir:    hostDir,
		homeDir:    t.TempDir(),
		router:     newConcurrencyCommandRouter(),
		identities: &concurrencyIdentityGenerator{},
		opened:     make(map[string]string),
		closed:     make(map[string]struct{}),
		sessions:   make(map[string]*concurrencySession),
		ownedDirs:  []string{hostDir},
	}

	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		APIServerStarter: func(ctx context.Context, request platformhttpserver.StartRequest) error {
			fixture.apiStarts.Add(1)
			err := fixture.api.Start(ctx, request)
			fixture.apiClose.Do(func() { close(fixture.apiClosed) })
			return err
		},
		ProviderCommandRunner:                  fixture.router,
		FactorySessionsWorkingDirectory:        platformfilesystem.Local{WorkingDirectory: hostDir},
		FactorySessionIDGenerator:              fixture.identities.nextSessionID,
		FactorySessionResponseEventIDGenerator: fixture.identities.nextResponseEventID,
	})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
	fixture.process = process
	fixture.processBuilds.Add(1)
	t.Cleanup(func() { fixture.close(t) })
	return fixture
}

func (fixture *concurrencySharedProcessFixture) start(t *testing.T) {
	t.Helper()
	if fixture.command != nil {
		t.Fatal("shared concurrency process started more than once")
	}
	env := append(os.Environ(), "HOME="+fixture.homeDir, "USERPROFILE="+fixture.homeDir)

	inputs := support.FakeInputs(context.Background(), []string{
		"you", "run",
		"--dir", fixture.hostDir,
		"--continuously",
		"--with-server",
		"--server", "http://127.0.0.1:1",
		"--quiet",
		"--no-record",
	})
	inputs.Input.Env = env
	inputs.Input.WorkingDirectory = fixture.hostDir
	fixture.command = support.StartProcessCommand(t, fixture.process, inputs.Input)
	t.Cleanup(func() {
		fixture.command.Stop(t)
		if fixture.apiStarts.Load() == 0 {
			t.Logf("concurrency host returned without API startup: error=%v stdout=%q stderr=%q", fixture.command.Err(), inputs.Stdout(), inputs.Stderr())
		}
	})
	fixture.baseURL = fixture.api.WaitForURL(t)
	defaultSession := support.GetDefaultSession(t, fixture.baseURL)
	if !defaultSession.IsDefault || strings.TrimSpace(defaultSession.Id) == "" {
		t.Fatalf("default Factory Session = %#v, want default identity", defaultSession)
	}
}

func (fixture *concurrencySharedProcessFixture) openCase(
	t *testing.T,
	name string,
	capacity int,
	behavior concurrencyRunnerBehavior,
	marker string,
	failMarker string,
	maxRetries int,
) *concurrencySession {
	t.Helper()
	dir := scaffoldConcurrencyFactory(t, name, capacity, maxRetries)
	support.ClearSeedInputs(t, dir)
	support.WriteAgentConfig(t, dir, "worker-a", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "concurrency-"+strings.ToLower(name)))
	support.WriteWorkstationConfig(t, dir, "process", concurrencyWorkstationConfig(maxRetries))
	runner := newConcurrencyScenarioRunner(behavior, marker, failMarker)
	fixture.router.register(t, dir, runner)
	fixture.addOwnedDir(dir)
	opened := support.OpenFactorySessionAt(t, fixture.baseURL, dir)
	if opened.Session == nil || strings.TrimSpace(opened.Session.Id) == "" {
		t.Fatalf("opened Factory Session for %q = %#v, want identity", dir, opened)
	}
	sessionID := opened.Session.Id
	if sessionID == factorysessions.DefaultSessionID {
		t.Fatalf("opened Factory Session for %q = %q, want explicit session", dir, sessionID)
	}
	publicSession := getConcurrencyFactorySession(t, fixture.baseURL, sessionID)
	if publicSession.Runtime.StreamIdentity == nil {
		t.Fatalf("opened Factory Session for %q runtime stream identity = nil, want public identity", dir)
	}
	streamIdentity := *publicSession.Runtime.StreamIdentity
	for label, value := range map[string]string{
		"backend scope":     streamIdentity.BackendScopeID,
		"logical session":   streamIdentity.LogicalSessionKeyID,
		"factory session":   streamIdentity.FactorySessionID,
		"stream generation": streamIdentity.StreamGenerationID,
	} {
		if strings.TrimSpace(value) == "" {
			t.Fatalf("opened Factory Session %q %s stream identity = %#v, want non-empty public identity", dir, label, streamIdentity)
		}
	}
	if streamIdentity.FactorySessionID != sessionID {
		t.Fatalf("opened Factory Session %q stream identity session = %q, want %q", dir, streamIdentity.FactorySessionID, sessionID)
	}
	fixture.sessionsMu.Lock()
	if _, exists := fixture.opened[sessionID]; exists {
		fixture.sessionsMu.Unlock()
		t.Fatalf("Factory Session id %q was reused", sessionID)
	}
	session := &concurrencySession{fixture: fixture, name: name, dir: dir, marker: marker, runner: runner, id: sessionID, stream: streamIdentity}
	fixture.opened[sessionID] = dir
	fixture.sessions[sessionID] = session
	fixture.sessionsMu.Unlock()
	t.Cleanup(func() { session.close(t) })
	return session
}

func (fixture *concurrencySharedProcessFixture) addOwnedDir(dir string) {
	fixture.sessionsMu.Lock()
	defer fixture.sessionsMu.Unlock()
	fixture.ownedDirs = append(fixture.ownedDirs, dir)
}

func (session *concurrencySession) close(t testing.TB) {
	t.Helper()
	session.closeOnce.Do(func() {
		support.CloseFactorySessionAt(t, session.fixture.baseURL, session.id)
		session.fixture.sessionsMu.Lock()
		session.fixture.closed[session.id] = struct{}{}
		session.fixture.sessionsMu.Unlock()
	})
}

func (session *concurrencySession) closeAndAssertGone(t *testing.T) {
	t.Helper()
	session.close(t)
	assertConcurrencySessionDeleted(t, session.fixture.baseURL, session.id)
}

func awaitAdmittedWorkEvent(t *testing.T, stream *support.FactoryEventStream, matches func(factoryapi.FactoryEvent) bool) factoryapi.FactoryEvent {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), concurrencySharedProcessTimeout)
	defer cancel()
	for {
		event := stream.NextEventContext(ctx)
		if matches(event) {
			return event
		}
	}
}

func admittedWorkDispatch(t *testing.T, session *concurrencySession, response factoryapi.SubmitWorkResponse) string {
	t.Helper()
	events := concurrencySessionEvents(t, session.fixture.baseURL, session.id)
	hasRequest := false
	dispatchID := ""
	for _, event := range events {
		if event.Context.SessionId == nil || *event.Context.SessionId != session.id ||
			event.Context.RequestId == nil || *event.Context.RequestId != response.RequestId || !concurrencyEventHasWork(event, stringPointerValue(response.WorkId)) {
			continue
		}
		if event.Type == factoryapi.FactoryEventTypeWorkRequest {
			hasRequest = true
		}
		if event.Type == factoryapi.FactoryEventTypeDispatchRequest {
			if dispatchID != "" {
				t.Fatal("AWC Work has more than one dispatch request")
			}
			dispatchID = stringPointerValue(event.Context.DispatchId)
		}
	}
	if !hasRequest || dispatchID == "" {
		t.Fatalf("AWC session=%s request=%s work=%s missing admission/dispatch correlation: %s", session.id, response.RequestId, stringPointerValue(response.WorkId), concurrencyEventSummary(events))
	}
	return dispatchID
}

func assertAdmittedWorkCanceled(t *testing.T, session *concurrencySession, response factoryapi.SubmitWorkResponse, dispatchID string) {
	t.Helper()
	publicSession := getConcurrencyFactorySession(t, session.fixture.baseURL, session.id)
	if publicSession.Runtime.LifecycleControlStatus == nil || *publicSession.Runtime.LifecycleControlStatus != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("AWC selected session lifecycle = %#v, want existing SUCCEEDED mapping", publicSession.Runtime.LifecycleControlStatus)
	}
	work := concurrencyWorkByID(t, session, response.WorkId)
	if work.State == nil || work.State.Type == factoryapi.WorkStateTypeTERMINAL || work.State.Type == factoryapi.WorkStateTypeFAILED || strings.Contains(workContentText(t, work), session.marker+" output") {
		t.Fatalf("AWC canceled Work became a business success/failure: %#v", work)
	}
	if got := admittedWorkDispatch(t, session, response); got != dispatchID || session.runner.callCount() != 1 || session.runner.activeCallCount() != 0 || session.runner.canceledCount() != 1 {
		t.Fatalf("AWC selected dispatch/command changed: dispatch=%s calls=%d active=%d canceled=%d", got, session.runner.callCount(), session.runner.activeCallCount(), session.runner.canceledCount())
	}
	// Session control stops the runtime; it must not synthesize an ACCEPTED
	// business completion for the held admitted Work.
	for _, dispatch := range support.ObserveDispatchEvents(t, concurrencySessionEvents(t, session.fixture.baseURL, session.id)) {
		if dispatch.DispatchID == dispatchID && dispatch.Response != nil && dispatch.Response.Outcome == factoryapi.WorkOutcomeAccepted {
			t.Fatalf("AWC canceled dispatch has accepted success: %#v", dispatch)
		}
	}
	t.Logf("AWC selected-session mapping: session=%s SUCCEEDED after CANCEL; work=%s state=%s; no accepted Work success/output", session.id, stringPointerValue(response.WorkId), work.State.Name)
}

func awaitAdmittedWorkTerminalPublication(t *testing.T, stream *support.FactoryResponseEventStream) {
	t.Helper()
	// The public response stream completes only after canonical SESSION_COMPLETED
	// publication. Command return and the CANCEL acknowledgement do not join it;
	// the Factory Event live stream can stop earlier. Drain buffered response
	// frames before requiring natural EOF, then inspect the retained ledger.
	deadline := time.Now().Add(concurrencySharedProcessTimeout)
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			t.Fatal("AWC timed out waiting for terminal publication")
		}
		result := stream.TryNextFrameResult(remaining)
		switch result.Outcome {
		case support.FactoryResponseEventStreamOutcomeFrame:
			continue
		case support.FactoryResponseEventStreamOutcomeEOF:
			return
		default:
			t.Fatalf("AWC terminal publication: %s", result.Diagnostic())
		}
	}
}

func admittedWorkCancellationEvent(t *testing.T, session *concurrencySession) factoryapi.FactoryEvent {
	t.Helper()
	// Selected-session cancellation currently maps to a SUCCEEDED session
	// bracket and leaves admitted Work incomplete. Keep that terminal mapping;
	// the typed public CANCEL acknowledgement and command cancellation establish
	// causality separately. The stopped live SSE stream is not the ledger.
	events := concurrencySessionEvents(t, session.fixture.baseURL, session.id)
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeSessionCompleted {
			continue
		}
		payload, err := event.Payload.AsSessionCompletedEventPayload()
		if err == nil && payload.FinalStatus == factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
			if event.Context.SessionId == nil || *event.Context.SessionId != session.id {
				t.Fatalf("AWC cancel terminal context = %#v, want selected session", event.Context)
			}
			return event
		}
	}
	t.Fatalf("AWC session %s has no attributable canonical cancellation fact: %s", session.id, concurrencyEventSummary(events))
	return factoryapi.FactoryEvent{}
}

func cancelAdmittedWorkSession(t *testing.T, session *concurrencySession) factoryapi.FactorySessionLifecycleControlResponse {
	t.Helper()
	endpoint := strings.TrimSuffix(session.fixture.baseURL, "/") + "/factory-sessions/" + url.PathEscape(session.id) + "/cancel"
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, endpoint, strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("AWC build cancellation request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("AWC public session cancellation: %v", err)
	}
	defer response.Body.Close()
	var control factoryapi.FactorySessionLifecycleControlResponse
	if err := json.NewDecoder(response.Body).Decode(&control); err != nil {
		t.Fatalf("AWC decode cancellation response: %v", err)
	}
	if (response.StatusCode != http.StatusOK && response.StatusCode != http.StatusAccepted) || control.SessionId != session.id ||
		control.Operation != factoryapi.FactorySessionLifecycleControlKindCancel || control.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted ||
		control.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("AWC cancel status=%d response=%#v, want exact target ACCEPTED CANCEL with existing SUCCEEDED mapping", response.StatusCode, control)
	}
	return control
}

func completeAdmittedWork(t *testing.T, session *concurrencySession, stream *support.FactoryEventStream, response factoryapi.SubmitWorkResponse, call concurrencyStartedCall, marker string) {
	t.Helper()
	dispatchID := admittedWorkDispatch(t, session, response)
	session.runner.releaseCall(call.index)
	// Ordinary dispatch completion carries terminal Work in DISPATCH_RESPONSE;
	// WORK_STATE_CHANGE is the separate operator-move contract.
	awaitAdmittedWorkEvent(t, stream, func(event factoryapi.FactoryEvent) bool {
		return event.Type == factoryapi.FactoryEventTypeDispatchResponse && stringPointerValue(event.Context.DispatchId) == dispatchID
	})
	session.runner.joinCalls(t)
	want := marker + " output COMPLETE"
	// Dispatch publication precedes updating the public Work projection. Await
	// the exact admitted Work rather than treating the event as a read barrier.
	work, err := support.WaitForObservation(concurrencySharedProcessTimeout, func() (factoryapi.Work, error) {
		return concurrencyWorkByID(t, session, response.WorkId), nil
	}, func(work factoryapi.Work) bool {
		return work.State != nil && work.State.Type == factoryapi.WorkStateTypeTERMINAL && work.State.Name == "complete" && workContentText(t, work) == want
	})
	if err != nil {
		t.Fatalf("AWC surviving Work did not publish completion: %v", err)
	}
	if work.State == nil || work.State.Type != factoryapi.WorkStateTypeTERMINAL || work.State.Name != "complete" || workContentText(t, work) != want || session.runner.canceledCount() != 0 {
		t.Fatalf("AWC surviving Work = %#v content=%q canceled=%d, want complete with exact %q", work, workContentText(t, work), session.runner.canceledCount(), want)
	}
	events := concurrencySessionEvents(t, session.fixture.baseURL, session.id)
	assertConcurrencyRequestDispatchTerminalCorrelation(t, session, events, response)
	for _, dispatch := range support.ObserveDispatchEvents(t, events) {
		if dispatch.DispatchID == dispatchID && (dispatch.Response == nil || support.StringPointerValue(dispatch.Response.Output) != want) {
			t.Fatalf("AWC surviving dispatch output = %#v, want exact %q", dispatch, want)
		}
	}
	t.Logf("AWC completion session=%s request=%s work=%s dispatch=%s output=%q command-returned", session.id, response.RequestId, stringPointerValue(response.WorkId), dispatchID, want)
}

func assertAdmittedWorkEventIsolation(t *testing.T, first, second *concurrencySession) {
	t.Helper()
	for _, pair := range [][2]*concurrencySession{{first, second}, {second, first}} {
		own, peer := pair[0], pair[1]
		events := concurrencySessionEvents(t, own.fixture.baseURL, own.id)
		for _, event := range events {
			if event.Context.SessionId != nil && *event.Context.SessionId != own.id {
				t.Fatalf("AWC event escaped session %s: %#v", own.id, event.Context)
			}
		}
		encoded, err := json.Marshal(events)
		if err != nil || strings.Contains(string(encoded), peer.marker) {
			t.Fatalf("AWC session %s events contain peer marker %q (marshal error %v)", own.id, peer.marker, err)
		}
	}
}
