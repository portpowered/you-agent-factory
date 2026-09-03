package codex

import (
	"strings"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func (fixture *codexConductorFixture) runScenario(
	t *testing.T,
	scenario codexConductorScenario,
) {
	t.Helper()

	opened := support.OpenFactorySessionAt(t, fixture.baseURL, scenario.factoryDir)
	if opened.Session == nil {
		t.Fatalf("%s open response missing session: %#v", scenario.name, opened)
	}
	sessionID := opened.Session.Id
	if sessionID == "" || sessionID == factorysessions.DefaultSessionID {
		t.Fatalf("%s session id = %q, want unique non-default explicit session", scenario.name, sessionID)
	}
	fixture.opened.Add(1)
	closed := false
	closeSession := func() {
		if closed {
			return
		}
		support.CloseFactorySessionAt(t, fixture.baseURL, sessionID)
		closed = true
		fixture.closed.Add(1)
	}
	t.Cleanup(closeSession)
	t.Cleanup(scenario.runner.Release)

	responseStream := support.OpenFactoryResponseEventStreamAt(
		t,
		support.SessionResponseEventsURL(fixture.baseURL, sessionID),
	)
	fixture.streamsOpened.Add(1)
	// Fast controlled outcomes, especially context.Canceled, can publish their
	// terminal response event before a just-opened SSE subscription is attached.
	// Release the external command edge only after the public stream is ready so
	// concurrent execution observes the same event boundary on every schedule.
	scenario.runner.Release()
	support.WaitForSessionTerminalStatus(t, fixture.baseURL, sessionID, codexConductorRunTimeout)
	session := getCodexSession(t, fixture.baseURL, sessionID)
	listed := listCodexSessionWork(t, fixture.baseURL, sessionID)
	events := support.GetFactoryEventsForSessionAt(t, fixture.baseURL, sessionID)
	responseEvents := readCodexResponseEventsUntilTerminal(t, responseStream, codexConductorRunTimeout)

	assertCodexWork(t, scenario, listed)
	dispatchIDs := assertCodexDispatch(t, scenario, sessionID, events)
	assertCodexCommand(t, fixture.router, scenario)
	providerSessionID := assertCodexProviderSession(t, scenario, events)
	assertCodexEventScope(t, scenario, sessionID, events)
	responseEventIDs := assertCodexResponseEvents(t, scenario, sessionID, responseEvents)

	closeSession()
	responseStream.WaitClosed(codexConductorRunTimeout)
	fixture.streamsClosed.Add(1)
	assertCodexSessionDeleted(t, fixture.baseURL, sessionID)
	fixture.recordObservation(codexScenarioObservation{
		sessionID:         session.Id,
		workID:            scenario.workID,
		requestID:         scenario.requestID,
		dispatchIDs:       dispatchIDs,
		providerSessionID: providerSessionID,
		responseEventIDs:  responseEventIDs,
	})
}

func (fixture *codexConductorFixture) assertSharedProcessCleanup(t *testing.T) {
	t.Helper()

	expectedSessions := fixture.opened.Load()
	if expectedSessions == 0 {
		t.Fatal("cleanup probe observed no executed Factory Session")
	}
	if got := fixture.closed.Load(); got != expectedSessions {
		t.Fatalf("closed Factory Sessions = %d, want %d", got, expectedSessions)
	}
	if got := fixture.streamsOpened.Load(); got != expectedSessions {
		t.Fatalf("opened response collectors = %d, want %d", got, expectedSessions)
	}
	if got := fixture.streamsClosed.Load(); got != expectedSessions {
		t.Fatalf("closed response collectors = %d, want %d", got, expectedSessions)
	}
	for _, scenario := range fixture.scenarios {
		if got := scenario.runner.ActiveCallCount(); got != 0 {
			t.Fatalf("%s active Codex command calls after process cleanup = %d, want 0", scenario.name, got)
		}
	}
}

func (fixture *codexConductorFixture) recordObservation(
	observation codexScenarioObservation,
) {
	fixture.ledgerMu.Lock()
	defer fixture.ledgerMu.Unlock()
	fixture.ledger[observation.requestID] = observation
}

func (fixture *codexConductorFixture) assertSharedIdentityLedger(t *testing.T) {
	t.Helper()

	fixture.ledgerMu.Lock()
	observations := make([]codexScenarioObservation, 0, len(fixture.ledger))
	for _, observation := range fixture.ledger {
		observations = append(observations, observation)
	}
	fixture.ledgerMu.Unlock()
	if len(observations) == 0 {
		t.Fatal("shared-process scenario observations are empty")
	}

	seenSessions := make(map[string]string, len(observations))
	seenWorks := make(map[string]string, len(observations))
	seenRequests := make(map[string]string, len(observations))
	seenDispatches := make(map[string]string, len(observations))
	seenProviderSessions := make(map[string]string, len(observations))
	seenResponseEvents := make(map[string]string)
	for _, observation := range observations {
		assertCodexUniqueIdentity(t, seenSessions, observation.sessionID, observation.requestID, "Factory Session")
		assertCodexUniqueIdentity(t, seenWorks, observation.workID, observation.requestID, "Work")
		assertCodexUniqueIdentity(t, seenRequests, observation.requestID, observation.requestID, "request")
		for _, dispatchID := range observation.dispatchIDs {
			assertCodexUniqueIdentity(t, seenDispatches, dispatchID, observation.requestID, "dispatch")
		}
		if observation.providerSessionID != "" {
			assertCodexUniqueIdentity(t, seenProviderSessions, observation.providerSessionID, observation.requestID, "Provider Session")
		}
		for _, responseEventID := range observation.responseEventIDs {
			assertCodexUniqueIdentity(t, seenResponseEvents, responseEventID, observation.requestID, "response event")
		}
	}
	if len(observations) != len(fixture.scenarios) {
		// An anchored subtest intentionally exercises only the selected route;
		// the full parent gate below is the evidence for both scenarios.
		return
	}
	if got := fixture.opened.Load(); got != int32(len(fixture.scenarios)) {
		t.Fatalf("Factory Session opens = %d, want %d", got, len(fixture.scenarios))
	}
	if got := fixture.closed.Load(); got != int32(len(fixture.scenarios)) {
		t.Fatalf("Factory Session closes = %d, want %d", got, len(fixture.scenarios))
	}
	if got := fixture.identities.sessionCount(); got < uint64(len(fixture.scenarios)) {
		t.Fatalf("Factory Session IDs generated = %d, want at least %d explicit sessions", got, len(fixture.scenarios))
	}
	if got := fixture.apiStarts.Load(); got != 1 {
		t.Fatalf("API server starts = %d, want exactly one shared process server", got)
	}
}

func assertCodexUniqueIdentity(
	t *testing.T,
	seen map[string]string,
	value string,
	owner string,
	kind string,
) {
	t.Helper()
	if strings.TrimSpace(value) == "" {
		t.Fatalf("%s identity for %s is empty", kind, owner)
	}
	if previous, ok := seen[value]; ok {
		t.Fatalf("%s identity %q is shared by %s and %s", kind, value, previous, owner)
	}
	seen[value] = owner
}
