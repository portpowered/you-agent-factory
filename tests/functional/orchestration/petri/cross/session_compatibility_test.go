package cross

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	busyLoopWorkflowName = "busy-loop"
	sessionCompatTimeout = 10 * time.Second
)

// TestPetriAndJavaScriptSessionsShareLifecycleControls proves the same public
// pause and resume lifecycle controls are accepted for a live Petri Factory
// Session and a live JavaScript Factory Session, and that each session's public
// status reflects paused then running states without inspecting orchestrator
// internals.
func TestPetriAndJavaScriptSessionsShareLifecycleControls(t *testing.T) {
	// CASE-X-001: both orchestrators use one production-composed host and
	// explicit session identities for the shared lifecycle-control contract.
	fixture := sharedCrossProcess(t)
	petri := openSharedCrossLiveSession(t)
	petriSession := readLiveSession(t, fixture.baseURL, petri.sessionID)
	assertLiveSessionOrchestratorKind(
		t,
		petriSession.Id,
		petriSession.Runtime.OrchestratorKind,
		factoryapi.PETRI,
	)

	javascript := startSharedCrossJavaScriptSession(t)
	javaSessionID := javascript.sessionID
	waitForDurableSessionStatus(
		t,
		fixture.baseURL,
		javaSessionID,
		factoryapi.FactorySessionDurableLifecycleStatusRunning,
		sessionCompatTimeout,
	)
	javaSession := readDurableSession(t, fixture.baseURL, javaSessionID)
	assertDurableSessionOrchestratorKind(
		t,
		javaSession.SessionId,
		javaSession.OrchestratorKind,
		factoryapi.JAVASCRIPT,
	)

	if petriSession.Id == javaSessionID {
		t.Fatalf("session ids must differ: petri=%q javascript=%q", petriSession.Id, javaSessionID)
	}

	applyAcceptedLifecycleControl(
		t,
		fixture.baseURL,
		petriSession.Id,
		factoryapi.FactorySessionLifecycleControlKindPause,
	)
	assertLiveSessionLifecycleControlStatus(
		t,
		fixture.baseURL,
		petriSession.Id,
		factoryapi.FactorySessionDurableLifecycleStatusPaused,
	)

	applyAcceptedLifecycleControl(
		t,
		fixture.baseURL,
		javaSessionID,
		factoryapi.FactorySessionLifecycleControlKindPause,
	)
	assertDurableSessionStatus(
		t,
		fixture.baseURL,
		javaSessionID,
		factoryapi.FactorySessionDurableLifecycleStatusPaused,
	)

	applyAcceptedLifecycleControl(
		t,
		fixture.baseURL,
		petriSession.Id,
		factoryapi.FactorySessionLifecycleControlKindResume,
	)
	assertLiveSessionLifecycleControlStatus(
		t,
		fixture.baseURL,
		petriSession.Id,
		factoryapi.FactorySessionDurableLifecycleStatusRunning,
	)

	applyAcceptedLifecycleControl(
		t,
		fixture.baseURL,
		javaSessionID,
		factoryapi.FactorySessionLifecycleControlKindResume,
	)
	assertDurableSessionStatus(
		t,
		fixture.baseURL,
		javaSessionID,
		factoryapi.FactorySessionDurableLifecycleStatusRunning,
	)
}

// TestPetriAndJavaScriptSessionsExposeCompatibleStatusFacts proves public session
// status reads for a live Petri Factory Session and a live JavaScript Factory
// Session expose compatible identity, orchestrator-kind, lifecycle, and shared
// runtime progress facts through the public session inspection contract without
// relying on orchestrator-private projections.
func TestPetriAndJavaScriptSessionsExposeCompatibleStatusFacts(t *testing.T) {
	// CASE-X-002: two explicit sessions remain independently readable while
	// sharing the same root-built process and API listener.
	fixture := sharedCrossProcess(t)
	petri := openSharedCrossLiveSession(t)
	petriSession := readLiveSession(t, fixture.baseURL, petri.sessionID)
	assertCompatibleLiveSessionStatusFacts(t, fixture.baseURL, petriSession, factoryapi.PETRI)

	javascript := startSharedCrossJavaScriptSession(t)
	javaSessionID := javascript.sessionID
	waitForDurableSessionStatus(
		t,
		fixture.baseURL,
		javaSessionID,
		factoryapi.FactorySessionDurableLifecycleStatusRunning,
		sessionCompatTimeout,
	)
	javaSession := readDurableSession(t, fixture.baseURL, javaSessionID)
	assertCompatibleDurableSessionStatusFacts(t, javaSession, factoryapi.JAVASCRIPT)

	if petriSession.Id == javaSession.SessionId {
		t.Fatalf("session ids must differ: petri=%q javascript=%q", petriSession.Id, javaSession.SessionId)
	}
}

func applyAcceptedLifecycleControl(
	t *testing.T,
	baseURL string,
	sessionID string,
	operation factoryapi.FactorySessionLifecycleControlKind,
	requestIDs ...string,
) factoryapi.FactorySessionLifecycleControlResponse {
	t.Helper()

	pathSegment := "pause"
	switch operation {
	case factoryapi.FactorySessionLifecycleControlKindResume:
		pathSegment = "resume"
	case factoryapi.FactorySessionLifecycleControlKindCancel:
		pathSegment = "cancel"
	case factoryapi.FactorySessionLifecycleControlKindTerminate:
		pathSegment = "terminate"
	}
	var requestID *string
	if len(requestIDs) > 0 && strings.TrimSpace(requestIDs[0]) != "" {
		requestID = &requestIDs[0]
	}
	response := postJSON[factoryapi.FactorySessionLifecycleControlResponse](
		t,
		baseURL+"/factory-sessions/"+sessionID+"/"+pathSegment,
		factoryapi.FactorySessionLifecycleControlRequest{RequestId: requestID},
		"apply "+string(operation)+" to session "+sessionID,
	)
	if response.Operation != operation {
		t.Fatalf(
			"session %s lifecycle control operation = %q, want %q",
			sessionID,
			response.Operation,
			operation,
		)
	}
	if response.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf(
			"session %s lifecycle control outcome = %q, want ACCEPTED; response=%#v",
			sessionID,
			response.Outcome,
			response,
		)
	}
	if response.SessionId != sessionID {
		t.Fatalf("lifecycle control sessionId = %q, want %q", response.SessionId, sessionID)
	}
	return response
}

func assertLiveSessionOrchestratorKind(
	t *testing.T,
	sessionID string,
	got factoryapi.FactoryOrchestratorKind,
	want factoryapi.FactoryOrchestratorKind,
) {
	t.Helper()
	if got != want {
		t.Fatalf("live session %s orchestratorKind = %q, want %q", sessionID, got, want)
	}
}

func assertDurableSessionOrchestratorKind(
	t *testing.T,
	sessionID string,
	got factoryapi.FactoryOrchestratorKind,
	want factoryapi.FactoryOrchestratorKind,
) {
	t.Helper()
	if got != want {
		t.Fatalf("durable session %s orchestratorKind = %q, want %q", sessionID, got, want)
	}
}

func assertCompatibleLiveSessionStatusFacts(
	t *testing.T,
	baseURL string,
	session factoryapi.FactorySession,
	wantOrchestrator factoryapi.FactoryOrchestratorKind,
) {
	t.Helper()

	if session.Id == "" {
		t.Fatal("live session id is empty")
	}
	assertLiveSessionOrchestratorKind(t, session.Id, session.Runtime.OrchestratorKind, wantOrchestrator)
	if session.Runtime.Status == "" {
		t.Fatalf("live session %s runtime.status is empty", session.Id)
	}
	if session.Runtime.Progress.FactoryState == "" {
		t.Fatalf("live session %s runtime.progress.factoryState is empty", session.Id)
	}
	assertReadableStatusCategories(t, session.Id, session.Runtime.Progress.Categories)

	status := readLiveSessionStatus(t, baseURL, session.Id)
	if status.FactoryState != session.Runtime.Progress.FactoryState {
		t.Fatalf(
			"GET /status factoryState = %q, session show = %q",
			status.FactoryState,
			session.Runtime.Progress.FactoryState,
		)
	}
	if status.RuntimeStatus != string(session.Runtime.Status) {
		t.Fatalf(
			"GET /status runtimeStatus = %q, session show = %q",
			status.RuntimeStatus,
			session.Runtime.Status,
		)
	}
	if status.Categories != session.Runtime.Progress.Categories {
		t.Fatalf(
			"GET /status categories = %#v, session show = %#v",
			status.Categories,
			session.Runtime.Progress.Categories,
		)
	}
	if status.TotalTokens != session.Runtime.Progress.TotalTokens {
		t.Fatalf(
			"GET /status totalTokens = %d, session show = %d",
			status.TotalTokens,
			session.Runtime.Progress.TotalTokens,
		)
	}
}

func assertCompatibleDurableSessionStatusFacts(
	t *testing.T,
	session factoryapi.FactorySessionDurableReadModel,
	wantOrchestrator factoryapi.FactoryOrchestratorKind,
) {
	t.Helper()

	if session.SessionId == "" {
		t.Fatal("durable session id is empty")
	}
	assertDurableSessionOrchestratorKind(
		t,
		session.SessionId,
		session.OrchestratorKind,
		wantOrchestrator,
	)
	if session.Status == "" {
		t.Fatalf("durable session %s status is empty", session.SessionId)
	}
	if session.ResolvedSource.Kind == "" {
		t.Fatalf("durable session %s resolvedSource.kind is empty", session.SessionId)
	}
	if session.Progress != nil {
		assertReadableDurableProgressCounts(t, session.SessionId, *session.Progress)
	}
}

func assertReadableStatusCategories(
	t *testing.T,
	sessionID string,
	categories factoryapi.StatusCategories,
) {
	t.Helper()

	for _, field := range []struct {
		name  string
		value int
	}{
		{"failed", categories.Failed},
		{"initial", categories.Initial},
		{"processing", categories.Processing},
		{"terminal", categories.Terminal},
	} {
		if field.value < 0 {
			t.Fatalf("live session %s categories.%s = %d, want >= 0", sessionID, field.name, field.value)
		}
	}
}

func assertReadableDurableProgressCounts(
	t *testing.T,
	sessionID string,
	progress factoryapi.FactorySessionDurableProgressCounts,
) {
	t.Helper()

	for _, field := range []struct {
		name  string
		value *int
	}{
		{"totalDispatches", progress.TotalDispatches},
		{"completedDispatches", progress.CompletedDispatches},
		{"failedDispatches", progress.FailedDispatches},
		{"inFlightDispatches", progress.InFlightDispatches},
		{"queuedDispatches", progress.QueuedDispatches},
		{"runningDispatches", progress.RunningDispatches},
		{"canceledDispatches", progress.CanceledDispatches},
		{"timedOutDispatches", progress.TimedOutDispatches},
		{"skippedDispatches", progress.SkippedDispatches},
		{"interruptedDispatches", progress.InterruptedDispatches},
		{"phaseCount", progress.PhaseCount},
	} {
		if field.value == nil {
			continue
		}
		if *field.value < 0 {
			t.Fatalf("durable session %s progress.%s = %d, want >= 0", sessionID, field.name, *field.value)
		}
	}
}

func readLiveSessionStatus(
	t *testing.T,
	baseURL string,
	sessionID string,
) factoryapi.StatusResponse {
	t.Helper()
	return support.GetJSON[factoryapi.StatusResponse](
		t,
		baseURL+"/factory-sessions/"+sessionID+"/status",
	)
}

func readLiveSession(
	t *testing.T,
	baseURL string,
	sessionID string,
) factoryapi.FactorySession {
	t.Helper()
	response := support.GetJSON[factoryapi.FactorySessionGetResponse](
		t,
		baseURL+"/factory-sessions/"+sessionID,
	)
	session, err := response.AsFactorySession()
	if err != nil {
		t.Fatalf("decode live session %s: %v", sessionID, err)
	}
	if session.Id != sessionID {
		t.Fatalf("live session id = %q, want %q", session.Id, sessionID)
	}
	return session
}

func assertLiveSessionLifecycleControlStatus(
	t *testing.T,
	baseURL string,
	sessionID string,
	want factoryapi.FactorySessionDurableLifecycleStatus,
) {
	t.Helper()

	session := readLiveSession(t, baseURL, sessionID)
	if session.Id != sessionID {
		t.Fatalf("live session id = %q, want %q", session.Id, sessionID)
	}
	if session.Runtime.LifecycleControlStatus == nil {
		t.Fatalf("live session %s lifecycleControlStatus = nil, want %q", sessionID, want)
	}
	if *session.Runtime.LifecycleControlStatus != want {
		t.Fatalf(
			"live session %s lifecycleControlStatus = %q, want %q",
			sessionID,
			*session.Runtime.LifecycleControlStatus,
			want,
		)
	}
}

func assertDurableSessionStatus(
	t *testing.T,
	baseURL string,
	sessionID string,
	want factoryapi.FactorySessionDurableLifecycleStatus,
) {
	t.Helper()

	session := readDurableSession(t, baseURL, sessionID)
	if session.Status != want {
		t.Fatalf("durable session %s status = %q, want %q", sessionID, session.Status, want)
	}
}

func readDurableSession(
	t testing.TB,
	baseURL string,
	sessionID string,
) factoryapi.FactorySessionDurableReadModel {
	t.Helper()

	response := support.GetJSON[factoryapi.FactorySessionGetResponse](
		t,
		baseURL+"/factory-sessions/"+sessionID,
	)
	session, err := response.AsFactorySessionDurableReadModel()
	if err != nil {
		t.Fatalf("decode durable session %s: %v", sessionID, err)
	}
	if session.SessionId != sessionID {
		t.Fatalf("durable session id = %q, want %q", session.SessionId, sessionID)
	}
	return session
}

func waitForDurableSessionStatus(
	t *testing.T,
	baseURL string,
	sessionID string,
	want factoryapi.FactorySessionDurableLifecycleStatus,
	timeout time.Duration,
) {
	t.Helper()

	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(15 * time.Millisecond)
	defer ticker.Stop()
	for {
		session := readDurableSession(t, baseURL, sessionID)
		if session.Status == want {
			return
		}
		// The public durable-session API has no readiness event for this
		// transition. This short bounded poll is an observation barrier, not
		// a workflow delay; it returns on the first matching projection.
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("durable session %s status = %q, want %q within %s", sessionID, session.Status, want, timeout)
		}
	}
}

func waitForDurableSessionTerminal(
	t testing.TB,
	baseURL string,
	sessionID string,
	timeout time.Duration,
) {
	t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(15 * time.Millisecond)
	defer ticker.Stop()
	for {
		session := readDurableSession(t, baseURL, sessionID)
		switch session.Status {
		case factoryapi.FactorySessionDurableLifecycleStatusCanceled,
			factoryapi.FactorySessionDurableLifecycleStatusFailed,
			factoryapi.FactorySessionDurableLifecycleStatusInterrupted,
			factoryapi.FactorySessionDurableLifecycleStatusSucceeded,
			factoryapi.FactorySessionDurableLifecycleStatusTerminated,
			factoryapi.FactorySessionDurableLifecycleStatusTimedOut:
			return
		}
		// Durable control completion is exposed through the public read model;
		// observe that projection with a bounded poll and return immediately
		// when it reaches a terminal state.
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("durable session %s status = %q, want terminal within %s", sessionID, session.Status, timeout)
		}
	}
}

func postJSON[T any](t *testing.T, endpoint string, request any, failurePrefix string) T {
	t.Helper()

	var body io.Reader
	if request != nil {
		encoded, err := json.Marshal(request)
		if err != nil {
			t.Fatalf("%s: marshal request: %v", failurePrefix, err)
		}
		body = bytes.NewReader(encoded)
	}
	response, err := http.Post(endpoint, "application/json", body)
	if err != nil {
		t.Fatalf("%s: POST %s: %v", failurePrefix, endpoint, err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf("%s: POST %s status = %d, want success: %s", failurePrefix, endpoint, response.StatusCode, payload)
	}
	var out T
	if err := json.NewDecoder(response.Body).Decode(&out); err != nil {
		t.Fatalf("%s: decode %s response: %v", failurePrefix, endpoint, err)
	}
	return out
}
