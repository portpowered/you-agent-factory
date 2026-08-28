package acp_test

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const acpSharedProcessTimeout = 20 * time.Second

// TestACPSharedProcess establishes the first c06 shared-process slice. The
// host is built once, while two explicit non-default Factory Sessions exercise
// independent success and failure projections through the same public server.
// The controlled peer keys its protocol failure to the failure scenario, while
// each explicit Factory Session retains its own peer boundary.
func TestACPSharedProcess(t *testing.T) {
	t.Setenv(acpHelperEnvironment, "shared-spine")
	t.Setenv("YOU_TEST_ACP_SESSION_ID", "")
	fixture := newACPSharedProcessFixture(t)
	runACPSharedEligibleBehavior(t, fixture)
}

type acpSharedProcessFixture struct {
	process    support.ApplicationProcess
	command    *support.ProcessCommand
	api        *support.ProcessAPIServer
	baseURL    string
	homeDir    string
	legacy     *legacyProvider
	rootBuilds atomic.Int32
	peerStarts atomic.Int32
	sessionMu  sync.Mutex
	opened     map[string]struct{}
	closed     map[string]struct{}
	closeOnce  sync.Once
}

type acpSharedSession struct {
	fixture *acpSharedProcessFixture
	id      string
	closed  bool
}

type acpSharedRun struct {
	session        factoryapi.FactorySession
	workID         string
	listed         factoryapi.ListWorkResponse
	events         []factoryapi.FactoryEvent
	responseEvents []factoryapi.FactoryResponseEvent
	workerEvents   []factoryapi.WorkerSessionEvent
}

func newACPSharedProcessFixture(t *testing.T) *acpSharedProcessFixture {
	t.Helper()
	hostDir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	homeDir := t.TempDir()
	installSharedACPIntegration(t, homeDir)
	api := support.NewProcessAPIServer()
	legacy := &legacyProvider{response: providers.ExecuteResult{Content: "legacy route COMPLETE"}}
	fixture := &acpSharedProcessFixture{
		api:     api,
		homeDir: homeDir,
		legacy:  legacy,
		opened:  make(map[string]struct{}),
		closed:  make(map[string]struct{}),
	}

	fixture.rootBuilds.Add(1)
	fixture.process = support.BuildProcess(t, serviceedges.Edges{
		APIServerStarter:              api.Start,
		PlatformProcessCommandFactory: acpHelperCommandFactory(&fixture.peerStarts),
		ProvidersExecutableLocator:    availableExecutableLocator{},
		ProviderOverride:              legacy,
	})

	environment := append(os.Environ(),
		"HOME="+homeDir,
		"USERPROFILE="+homeDir,
	)
	inputs := support.FakeInputs(context.Background(), []string{
		"you", "run", "--dir", hostDir, "--continuously", "--with-server", "--quiet", "--no-record",
	})
	inputs.Input.Env = environment
	inputs.Input.WorkingDirectory = hostDir
	fixture.command = support.StartProcessCommand(t, fixture.process, inputs.Input)
	fixture.baseURL = api.WaitForURL(t)
	support.WaitForStatus(t, fixture.baseURL, acpSharedProcessTimeout, func(status factoryapi.StatusResponse) bool {
		return status.RuntimeStatus != ""
	})
	t.Cleanup(func() { fixture.close(t) })
	return fixture
}

func (fixture *acpSharedProcessFixture) openSession(t *testing.T) *acpSharedSession {
	return fixture.openSessionWith(t, func(t *testing.T, dir string) {
		writeACPWorker(t, dir, "cursor-acp")
	})
}

func (fixture *acpSharedProcessFixture) openSessionWith(
	t *testing.T,
	configure func(*testing.T, string),
) *acpSharedSession {
	t.Helper()
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	if configure != nil {
		configure(t, dir)
	}
	return fixture.openSessionDir(t, dir)
}

func (fixture *acpSharedProcessFixture) openSessionDir(t *testing.T, dir string) *acpSharedSession {
	t.Helper()
	opened := support.OpenFactorySessionAt(t, fixture.baseURL, dir)
	if opened.Session == nil || opened.Session.Id == "" {
		t.Fatalf("opened shared ACP Factory Session = %#v, want identity", opened)
	}
	if opened.Session.Id == factorysessions.DefaultSessionID {
		t.Fatalf("shared ACP child session id = %q, want explicit non-default session", opened.Session.Id)
	}
	session := &acpSharedSession{fixture: fixture, id: opened.Session.Id}
	fixture.sessionMu.Lock()
	if _, exists := fixture.opened[session.id]; exists {
		fixture.sessionMu.Unlock()
		t.Fatalf("shared ACP Factory Session id %q was reused", session.id)
	}
	fixture.opened[session.id] = struct{}{}
	fixture.sessionMu.Unlock()
	t.Cleanup(func() { session.close(t) })
	return session
}

func (session *acpSharedSession) run(t *testing.T, name, title string) acpSharedRun {
	return session.runRequest(t, factoryapi.SubmitWorkRequest{
		Name:         &name,
		WorkTypeName: "task",
		Payload:      map[string]string{"title": title},
	}, false)
}

func (session *acpSharedSession) runRequest(
	t *testing.T,
	request factoryapi.SubmitWorkRequest,
	captureWorkerEvents bool,
) acpSharedRun {
	t.Helper()
	submitted := support.SubmitSessionWorkAt(t, session.fixture.baseURL, session.id, request)
	workID := support.StringPointerValue(submitted.WorkId)
	if workID == "" {
		t.Fatalf("shared ACP submission = %#v, want Work ID", submitted)
	}
	support.WaitForSessionTerminalStatus(t, session.fixture.baseURL, session.id, acpSharedProcessTimeout)
	var workerEvents []factoryapi.WorkerSessionEvent
	if captureWorkerEvents {
		workerEvents = getACPSharedWorkerSessionEvents(t, session.fixture.baseURL, session.id, workID)
	}
	return acpSharedRun{
		session:        sharedACPFactorySession(t, session.fixture.baseURL, session.id),
		workID:         workID,
		listed:         sharedACPWork(t, session.fixture.baseURL, session.id),
		events:         support.GetFactoryEventsForSessionAt(t, session.fixture.baseURL, session.id),
		responseEvents: support.GetFactoryResponseEventsAt(t, session.fixture.baseURL, session.id),
		workerEvents:   workerEvents,
	}
}

func sharedACPFactorySession(t testing.TB, baseURL, sessionID string) factoryapi.FactorySession {
	t.Helper()
	response := support.GetJSON[factoryapi.FactorySessionGetResponse](
		t,
		baseURL+"/factory-sessions/"+url.PathEscape(sessionID),
	)
	session, err := response.AsFactorySession()
	if err != nil {
		t.Fatalf("decode shared ACP Factory Session %q: %v", sessionID, err)
	}
	return session
}

func sharedACPWork(t testing.TB, baseURL, sessionID string) factoryapi.ListWorkResponse {
	t.Helper()
	endpoint := baseURL + "/factory-sessions/" + url.PathEscape(sessionID) + "/work"
	return support.GetJSON[factoryapi.ListWorkResponse](t, endpoint)
}

func assertACPSharedSuccess(t *testing.T, run acpSharedRun) {
	t.Helper()
	assertBTRCACPEventOrder(t, run.events)
	workID, dispatchID := assertBTRCACPDispatch(t, run.events, factoryapi.WorkOutcomeAccepted, "done")
	if workID != run.workID {
		t.Fatalf("shared ACP success event Work ID = %q, want submitted %q", workID, run.workID)
	}
	assertBTRCACPProviderSession(t, run.events, factoryapi.InferenceOutcomeSucceeded)
	assertBTRCACPResponseTerminal(t, run.responseEvents, "COMPLETED")
	assertBTRCACPCompletedTarget(t, run.session, run.listed, workID, dispatchID)
}

func assertACPSharedFailure(t *testing.T, run acpSharedRun) {
	t.Helper()
	assertBTRCACPEventOrder(t, run.events)
	workID, dispatchID := assertBTRCACPDispatch(t, run.events, factoryapi.WorkOutcomeFailed, "failed")
	if workID != run.workID {
		t.Fatalf("shared ACP failure event Work ID = %q, want submitted %q", workID, run.workID)
	}
	assertBTRCACPProviderSession(t, run.events, factoryapi.InferenceOutcomeFailed)
	assertBTRCACPFailureDetail(t, run.events)
	assertBTRCACPResponseTerminal(t, run.responseEvents, "FAILED")
	assertBTRCACPFailedTarget(t, run.session, run.listed, workID, dispatchID)
	for _, event := range run.events {
		if event.Type != factoryapi.FactoryEventTypeDispatchResponse {
			continue
		}
		payload, err := event.Payload.AsDispatchResponseEventPayload()
		if err != nil {
			t.Fatalf("decode shared ACP failure dispatch response: %v", err)
		}
		if payload.Output != nil && strings.Contains(*payload.Output, "execution COMPLETE") {
			t.Fatalf("shared ACP failure retained success output: %q", *payload.Output)
		}
	}
}

func assertACPSharedSessionIsolation(
	t *testing.T,
	success, failure *acpSharedSession,
	successRun, failureRun acpSharedRun,
) {
	t.Helper()
	if success.id == failure.id {
		t.Fatalf("shared ACP Factory Session IDs = %q and %q, want distinct identities", success.id, failure.id)
	}
	assertACPEventSession(t, successRun.events, success.id)
	assertACPEventSession(t, failureRun.events, failure.id)
	assertACPResponseSession(t, successRun.responseEvents, success.id)
	assertACPResponseSession(t, failureRun.responseEvents, failure.id)
	if len(successRun.events) != len(failureRun.events) {
		t.Fatalf("shared ACP event counts = success:%d failure:%d, want equal first-slice shapes", len(successRun.events), len(failureRun.events))
	}
}

func assertACPEventSession(t *testing.T, events []factoryapi.FactoryEvent, sessionID string) {
	t.Helper()
	scopedCount := 0
	for index, event := range events {
		if event.Context.SessionId == nil {
			// The public Factory Event timeline includes unscoped lifecycle
			// initialization records before request-scoped events.
			continue
		}
		if *event.Context.SessionId != sessionID {
			t.Fatalf("Factory Event[%d] type=%q session = %#v, want %q", index, event.Type, event.Context.SessionId, sessionID)
		}
		scopedCount++
	}
	if scopedCount == 0 {
		t.Fatalf("Factory Event history for %q contains no session-scoped records", sessionID)
	}
}

func assertACPResponseSession(t *testing.T, events []factoryapi.FactoryResponseEvent, sessionID string) {
	t.Helper()
	if len(events) == 0 {
		t.Fatalf("Factory Response Events for %q = 0, want terminal observations", sessionID)
	}
	for index, event := range events {
		if event.FactorySessionId != sessionID {
			t.Fatalf("Factory Response Event[%d] session = %q, want %q", index, event.FactorySessionId, sessionID)
		}
	}
}

func (session *acpSharedSession) close(t testing.TB) {
	t.Helper()
	if session == nil || session.closed {
		return
	}
	support.CloseFactorySessionAt(t, session.fixture.baseURL, session.id)
	assertACPSharedSessionDeleted(t, session.fixture.baseURL, session.id)
	session.fixture.sessionMu.Lock()
	session.fixture.closed[session.id] = struct{}{}
	session.fixture.sessionMu.Unlock()
	session.closed = true
}

func assertACPSharedSessionDeleted(t testing.TB, baseURL, sessionID string) {
	t.Helper()
	endpoint := baseURL + "/factory-sessions/" + url.PathEscape(sessionID)
	client := &http.Client{Timeout: 2 * time.Second}
	defer client.CloseIdleConnections()
	response, err := client.Get(endpoint)
	if err != nil {
		t.Fatalf("GET deleted shared ACP Factory Session %q: %v", sessionID, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("GET deleted shared ACP Factory Session %q status = %d, want 404: %s", sessionID, response.StatusCode, strings.TrimSpace(string(body)))
	}
}

func (fixture *acpSharedProcessFixture) close(t testing.TB) {
	t.Helper()
	fixture.closeOnce.Do(func() {
		if fixture.command != nil {
			fixture.command.Stop(t)
			select {
			case <-fixture.command.Done():
			default:
				t.Errorf("shared ACP Process.Execute did not join after cancellation")
			}
		}
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if fixture.process != nil {
			if err := fixture.process.Close(closeCtx); err != nil {
				t.Errorf("close shared ACP application process: %v", err)
			}
		}
		fixture.assertSessionTopology(t)
		assertACPSharedListenerClosed(t, fixture.baseURL)
	})
}

func (fixture *acpSharedProcessFixture) assertSessionTopology(t testing.TB) {
	t.Helper()
	fixture.sessionMu.Lock()
	defer fixture.sessionMu.Unlock()
	if len(fixture.opened) != len(fixture.closed) {
		t.Errorf("shared ACP Factory Session lifecycle = opened:%d closed:%d, want equal", len(fixture.opened), len(fixture.closed))
	}
	for sessionID := range fixture.opened {
		if _, ok := fixture.closed[sessionID]; !ok {
			t.Errorf("shared ACP Factory Session %q was not deleted", sessionID)
		}
	}
}

func assertACPSharedListenerClosed(t testing.TB, baseURL string) {
	t.Helper()
	// ProcessCommand.Stop has already joined the invocation. One bounded probe
	// is sufficient to prove the injected listener was released; polling here
	// would only hide a shutdown race.
	client := &http.Client{Timeout: 250 * time.Millisecond}
	defer client.CloseIdleConnections()
	response, err := client.Get(baseURL + "/status")
	if err != nil {
		return
	}
	defer response.Body.Close()
	body, readErr := io.ReadAll(response.Body)
	t.Errorf("shared ACP listener remains reachable after cleanup: status=%d body=%q readError=%v", response.StatusCode, strings.TrimSpace(string(body)), readErr)
}
