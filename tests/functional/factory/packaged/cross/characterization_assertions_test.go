package cross

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	crossCharacterizationExpectedScenarios = 8
	crossCharacterizationExpectedRoots     = 9
	crossCharacterizationExpectedServers   = 3
	crossCharacterizationExpectedSessions  = 5
)

var crossCharacterization = newCrossCharacterizationLedger()

type crossCharacterizationLedger struct {
	mu sync.Mutex

	rootStarts   map[string]int
	rootCloses   map[string]int
	serverStarts int
	serverCloses int

	sessionsOpened  map[string]struct{}
	sessionsClosed  map[string]struct{}
	sessionsDeleted map[string]struct{}

	completedScenarios map[string]struct{}
	pathProbes         int
	sharedFixtureClean bool
}

func newCrossCharacterizationLedger() *crossCharacterizationLedger {
	return &crossCharacterizationLedger{
		rootStarts:         make(map[string]int),
		rootCloses:         make(map[string]int),
		sessionsOpened:     make(map[string]struct{}),
		sessionsClosed:     make(map[string]struct{}),
		sessionsDeleted:    make(map[string]struct{}),
		completedScenarios: make(map[string]struct{}),
	}
}

func (ledger *crossCharacterizationLedger) recordRootStart(kind string) {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	ledger.rootStarts[kind]++
}

func (ledger *crossCharacterizationLedger) recordRootClose(kind string) {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	ledger.rootCloses[kind]++
}

func (ledger *crossCharacterizationLedger) recordServerStart() {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	ledger.serverStarts++
}

func (ledger *crossCharacterizationLedger) recordServerClose() {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	ledger.serverCloses++
}

func (ledger *crossCharacterizationLedger) recordSessionOpened(sessionID string) error {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if strings.TrimSpace(sessionID) == "" {
		return fmt.Errorf("Factory Session ID is empty")
	}
	if _, exists := ledger.sessionsOpened[sessionID]; exists {
		return fmt.Errorf("Factory Session ID %q opened more than once", sessionID)
	}
	ledger.sessionsOpened[sessionID] = struct{}{}
	return nil
}

func (ledger *crossCharacterizationLedger) recordSessionClosed(sessionID string) error {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if _, exists := ledger.sessionsOpened[sessionID]; !exists {
		return fmt.Errorf("Factory Session %q was closed before it was opened", sessionID)
	}
	if _, exists := ledger.sessionsClosed[sessionID]; exists {
		return fmt.Errorf("Factory Session %q closed more than once", sessionID)
	}
	ledger.sessionsClosed[sessionID] = struct{}{}
	return nil
}

func (ledger *crossCharacterizationLedger) recordSessionDeleted(sessionID string) error {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if _, exists := ledger.sessionsClosed[sessionID]; !exists {
		return fmt.Errorf("Factory Session %q was deleted before it was closed", sessionID)
	}
	if _, exists := ledger.sessionsDeleted[sessionID]; exists {
		return fmt.Errorf("Factory Session %q deleted more than once", sessionID)
	}
	ledger.sessionsDeleted[sessionID] = struct{}{}
	return nil
}

func (ledger *crossCharacterizationLedger) recordScenarioComplete(name string) {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	ledger.completedScenarios[name] = struct{}{}
}

func (ledger *crossCharacterizationLedger) completedScenarioCount() int {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	return len(ledger.completedScenarios)
}

func (ledger *crossCharacterizationLedger) recordPathProbe() {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	ledger.pathProbes++
}

func (ledger *crossCharacterizationLedger) recordSharedFixtureCleanup() {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	ledger.sharedFixtureClean = true
}

func (ledger *crossCharacterizationLedger) validateAfterSuite() error {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if len(ledger.completedScenarios) != crossCharacterizationExpectedScenarios {
		return nil
	}

	rootStarts := sumCrossCharacterizationCounts(ledger.rootStarts)
	rootCloses := sumCrossCharacterizationCounts(ledger.rootCloses)
	if rootStarts != crossCharacterizationExpectedRoots || rootCloses != crossCharacterizationExpectedRoots {
		return fmt.Errorf(
			"CHAR-001 scenario roots started=%d closed=%d, want %d/%d by kind starts=%v closes=%v",
			rootStarts, rootCloses,
			crossCharacterizationExpectedRoots, crossCharacterizationExpectedRoots,
			ledger.rootStarts, ledger.rootCloses,
		)
	}
	if ledger.serverStarts != crossCharacterizationExpectedServers || ledger.serverCloses != crossCharacterizationExpectedServers {
		return fmt.Errorf(
			"CHAR-001 loopback servers started=%d closed=%d, want %d/%d",
			ledger.serverStarts, ledger.serverCloses,
			crossCharacterizationExpectedServers, crossCharacterizationExpectedServers,
		)
	}
	if len(ledger.sessionsOpened) != crossCharacterizationExpectedSessions ||
		len(ledger.sessionsClosed) != crossCharacterizationExpectedSessions ||
		len(ledger.sessionsDeleted) != crossCharacterizationExpectedSessions {
		return fmt.Errorf(
			"CHAR-001 explicit Factory Sessions opened/closed/deleted=%d/%d/%d, want %d/%d/%d",
			len(ledger.sessionsOpened), len(ledger.sessionsClosed), len(ledger.sessionsDeleted),
			crossCharacterizationExpectedSessions,
			crossCharacterizationExpectedSessions,
			crossCharacterizationExpectedSessions,
		)
	}
	if !ledger.sharedFixtureClean || ledger.pathProbes == 0 {
		return fmt.Errorf(
			"CHAR-001 cleanup probes fixture_clean=%t path_probes=%d, want fixture cleanup and at least one owned path probe",
			ledger.sharedFixtureClean, ledger.pathProbes,
		)
	}
	return nil
}

func (ledger *crossCharacterizationLedger) summary() string {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	return fmt.Sprintf(
		"scenario_roots=%d/%d servers=%d/%d sessions_opened=%d sessions_closed=%d sessions_deleted=%d path_probes=%d completed=%d",
		sumCrossCharacterizationCounts(ledger.rootStarts),
		sumCrossCharacterizationCounts(ledger.rootCloses),
		ledger.serverStarts, ledger.serverCloses,
		len(ledger.sessionsOpened), len(ledger.sessionsClosed), len(ledger.sessionsDeleted),
		ledger.pathProbes, len(ledger.completedScenarios),
	)
}

func sumCrossCharacterizationCounts(counts map[string]int) int {
	total := 0
	for _, count := range counts {
		total += count
	}
	return total
}

type packagedGoalInvocationObservation struct {
	sessionID string
	listed    factoryapi.ListWorkResponse
	events    []factoryapi.FactoryEvent
}

func observePackagedGoalInvocation(
	t testing.TB,
	baseURL string,
	sessionID string,
) packagedGoalInvocationObservation {
	t.Helper()
	workEndpoint := strings.TrimSuffix(baseURL, "/") +
		"/factory-sessions/" + url.PathEscape(sessionID) + "/work"
	return packagedGoalInvocationObservation{
		sessionID: sessionID,
		listed:    support.GetJSON[factoryapi.ListWorkResponse](t, workEndpoint),
		events:    support.GetFactoryEventsForSessionAt(t, baseURL, sessionID),
	}
}

func assertPackagedGoalInvocationWorkAndEvents(
	t testing.TB,
	response factoryapi.InvocationResponse,
	observation packagedGoalInvocationObservation,
	wantState string,
) factoryapi.Work {
	t.Helper()
	if strings.TrimSpace(response.RequestId) == "" || strings.TrimSpace(response.TraceId) == "" {
		t.Fatalf("invocation identity = request %q trace %q, want both non-empty", response.RequestId, response.TraceId)
	}
	if response.SessionId != nil && strings.TrimSpace(*response.SessionId) != "" &&
		*response.SessionId != observation.sessionID {
		t.Fatalf("invocation sessionId = %q, want observed session %q", *response.SessionId, observation.sessionID)
	}

	var matches []factoryapi.Work
	for _, candidate := range observation.listed.Results {
		if candidate.WorkTypeName == nil || *candidate.WorkTypeName != "goal" {
			continue
		}
		if packagedGoalWorkCorrelatesWithCLIInvocation(candidate, response) {
			matches = append(matches, candidate)
		}
	}
	if len(matches) != 1 {
		t.Fatalf(
			"observed goal Work matches for request %q trace %q = %d, want one; listed=%#v",
			response.RequestId, response.TraceId, len(matches), observation.listed.Results,
		)
	}
	work := matches[0]
	if work.WorkId == nil || strings.TrimSpace(*work.WorkId) == "" {
		t.Fatalf("correlated goal Work ID = %#v, want non-empty", work.WorkId)
	}
	if work.State == nil {
		t.Fatalf("correlated goal Work %q state is nil", *work.WorkId)
	}
	if wantState != "" && work.State.Name != wantState {
		t.Fatalf("correlated goal Work %q state = %q, want %q", *work.WorkId, work.State.Name, wantState)
	}
	if work.State.Type != factoryapi.WorkStateTypeTERMINAL && work.State.Type != factoryapi.WorkStateTypeFAILED {
		t.Fatalf("correlated goal Work %q state type = %q, want terminal or failed", *work.WorkId, work.State.Type)
	}

	assertPackagedGoalEventOrder(t, observation.events, observation.sessionID, response, *work.WorkId, work.State.Name)
	return work
}

func assertPackagedGoalNoAdmission(
	t testing.TB,
	observation packagedGoalInvocationObservation,
) {
	t.Helper()
	if len(observation.listed.Results) != 0 {
		t.Fatalf("rejected invocation listed Work = %#v, want no admitted Work", observation.listed.Results)
	}
	for _, event := range observation.events {
		if event.Type == factoryapi.FactoryEventTypeWorkRequest {
			t.Fatalf("rejected invocation emitted WORK_REQUEST event %#v, want absent admission", event)
		}
	}
}

func assertPackagedGoalEventOrder(
	t testing.TB,
	events []factoryapi.FactoryEvent,
	sessionID string,
	response factoryapi.InvocationResponse,
	workID string,
	wantTerminalState string,
) {
	t.Helper()
	if len(events) == 0 {
		t.Fatalf("Factory Events for session %q are empty, want admission and terminal witnesses", sessionID)
	}

	seenIDs := make(map[string]struct{}, len(events))
	ordered := make([]factoryapi.FactoryEvent, 0, len(events))
	for index, event := range events {
		if strings.TrimSpace(event.Id) == "" {
			t.Fatalf("Factory Event[%d] ID is empty", index)
		}
		if _, exists := seenIDs[event.Id]; exists {
			t.Fatalf("Factory Event ID %q is duplicated", event.Id)
		}
		seenIDs[event.Id] = struct{}{}
		if event.Context.SessionId != nil && *event.Context.SessionId != sessionID {
			t.Fatalf("Factory Event[%d] type=%q session ID = %q, want %q", index, event.Type, *event.Context.SessionId, sessionID)
		}
		if event.Context.SessionId != nil {
			ordered = append(ordered, event)
		}
	}
	if len(ordered) == 0 {
		t.Fatalf("Factory Events for session %q have no session-scoped records", sessionID)
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		return crossEventSessionSequence(ordered[i]) < crossEventSessionSequence(ordered[j])
	})
	previousSequence := -1
	admissionSequence := -1
	terminalSequence := -1
	for index, event := range ordered {
		if (event.Type == factoryapi.FactoryEventTypeWorkRequest || event.Type == factoryapi.FactoryEventTypeWorkStateChange) &&
			(event.Context.SessionId == nil || *event.Context.SessionId != sessionID) {
			t.Fatalf("correlated Factory Event[%d] type=%q session ID = %#v, want %q", index, event.Type, event.Context.SessionId, sessionID)
		}
		if event.Context.SessionSequence == nil {
			t.Fatalf("Factory Event[%d] sessionSequence is nil", index)
		}
		sequence := *event.Context.SessionSequence
		if sequence <= previousSequence {
			t.Fatalf("Factory Event[%d] sessionSequence = %d after %d, want strictly increasing order", index, sequence, previousSequence)
		}
		previousSequence = sequence

		switch event.Type {
		case factoryapi.FactoryEventTypeWorkRequest:
			payload, err := event.Payload.AsWorkRequestEventPayload()
			if err != nil {
				t.Fatalf("decode WORK_REQUEST event %q: %v", event.Id, err)
			}
			if event.Context.RequestId == nil || *event.Context.RequestId != response.RequestId {
				continue
			}
			for _, requested := range support.FactoryWorksValue(payload.Works) {
				if requested.WorkId != nil && *requested.WorkId == workID {
					if admissionSequence >= 0 {
						t.Fatalf("multiple correlated WORK_REQUEST events for Work %q", workID)
					}
					admissionSequence = sequence
				}
			}
		case factoryapi.FactoryEventTypeWorkStateChange:
			payload, err := event.Payload.AsWorkStateChangeEventPayload()
			if err != nil {
				t.Fatalf("decode WORK_STATE_CHANGE event %q: %v", event.Id, err)
			}
			if payload.WorkId == workID && payload.ToState == wantTerminalState {
				if terminalSequence >= 0 {
					t.Fatalf("multiple correlated terminal WORK_STATE_CHANGE events for Work %q", workID)
				}
				terminalSequence = sequence
			}
		case factoryapi.FactoryEventTypeDispatchResponse:
			payload, err := event.Payload.AsDispatchResponseEventPayload()
			if err != nil {
				t.Fatalf("decode DISPATCH_RESPONSE event %q: %v", event.Id, err)
			}
			if crossEventContainsWorkID(event, workID) &&
				(payload.Outcome == factoryapi.WorkOutcomeAccepted || payload.Outcome == factoryapi.WorkOutcomeFailed) {
				if terminalSequence >= 0 {
					t.Fatalf("multiple correlated terminal events for Work %q", workID)
				}
				terminalSequence = sequence
			}
		}
	}
	if admissionSequence < 0 {
		t.Fatalf("missing correlated WORK_REQUEST admission for request %q Work %q", response.RequestId, workID)
	}
	if terminalSequence < 0 {
		t.Fatalf("missing correlated terminal Work event for Work %q state %q; events=%v", workID, wantTerminalState, summarizeCrossFactoryEvents(ordered))
	}
	if admissionSequence >= terminalSequence {
		t.Fatalf("Work %q admission sequence=%d terminal sequence=%d, want admission first", workID, admissionSequence, terminalSequence)
	}
}

func crossEventContainsWorkID(event factoryapi.FactoryEvent, workID string) bool {
	if event.Context.WorkIds == nil {
		return false
	}
	for _, candidate := range *event.Context.WorkIds {
		if candidate == workID {
			return true
		}
	}
	return false
}

func summarizeCrossFactoryEvents(events []factoryapi.FactoryEvent) []string {
	summaries := make([]string, 0, len(events))
	for _, event := range events {
		sequence := "-"
		if event.Context.SessionSequence != nil {
			sequence = fmt.Sprint(*event.Context.SessionSequence)
		}
		workIDs := "-"
		if event.Context.WorkIds != nil {
			workIDs = strings.Join(*event.Context.WorkIds, ",")
		}
		requestID := "-"
		if event.Context.RequestId != nil {
			requestID = *event.Context.RequestId
		}
		summaries = append(summaries, fmt.Sprintf("%s/%s seq=%s request=%s work=%s", event.Type, event.Id, sequence, requestID, workIDs))
	}
	return summaries
}

func crossEventSessionSequence(event factoryapi.FactoryEvent) int {
	if event.Context.SessionSequence == nil {
		return -1
	}
	return *event.Context.SessionSequence
}

func removeCrossOwnedPath(t testing.TB, label, path string) {
	t.Helper()
	if strings.TrimSpace(path) == "" {
		return
	}
	if err := os.RemoveAll(path); err != nil {
		t.Errorf("remove %s %q: %v", label, path, err)
		return
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("%s %q remains after cleanup; stat error: %v", label, path, err)
		return
	}
	crossCharacterization.recordPathProbe()
}

func crossHomeFromEnvironment(environment []string) string {
	for _, entry := range environment {
		name, value, ok := strings.Cut(entry, "=")
		if ok && strings.EqualFold(name, "HOME") {
			return value
		}
	}
	return ""
}

// assertCrossListenerClosed observes the local-real HTTP edge after its
// owning Process.Execute has joined. The short bounded retry covers the
// transport's asynchronous close without turning the behavior test into an
// unbounded polling loop.
func assertCrossListenerClosed(t testing.TB, baseURL string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	client := &http.Client{Timeout: 250 * time.Millisecond}
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSuffix(baseURL, "/")+"/status", nil)
		if err == nil {
			response, requestErr := client.Do(request)
			if requestErr != nil {
				cancel()
				return
			}
			_, _ = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
		}
		cancel()
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("local-real listener %s remained reachable after owning process cleanup", baseURL)
}

func assertPackagedGoalFactorySessionAbsent(t testing.TB, baseURL, sessionID string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		t.Fatalf("build deleted Factory Session probe: %v", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("GET deleted Factory Session %q: %v", sessionID, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("GET deleted Factory Session %q status = %d, want 404: %s", sessionID, response.StatusCode, strings.TrimSpace(string(body)))
	}
}
