package service

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/api"
	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factorysessions"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"go.uber.org/zap"
)

func TestFactoryService_LiveSessionPauseResume_HTTPReturnsTypedLifecycleControl(t *testing.T) {
	t.Parallel()

	harness := startRunningSessionService(t, runningSessionServiceOptions{
		rootConfig: minimalFactoryConfig(),
	})
	server := httptest.NewServer(api.NewServer(harness.svc, 0, zap.NewNop()).Handler())
	defer server.Close()

	sessionID := factorysessions.DefaultSessionID
	sessionPath := "/factory-sessions/" + sessionID

	pauseResp, pauseStatus := postLiveSessionLifecycleControl(t, server.URL, sessionID, "pause")
	if pauseStatus != http.StatusOK {
		t.Fatalf("pause status = %d, want 200", pauseStatus)
	}
	if pauseResp.Operation != factoryapi.FactorySessionLifecycleControlKindPause {
		t.Fatalf("operation = %q, want PAUSE", pauseResp.Operation)
	}
	if pauseResp.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("outcome = %q, want ACCEPTED", pauseResp.Outcome)
	}
	if pauseResp.Status != factoryapi.FactorySessionDurableLifecycleStatusPaused {
		t.Fatalf("status = %q, want PAUSED", pauseResp.Status)
	}

	pausedSession := getLiveFactorySession(t, server.URL, sessionID)
	if pausedSession.Runtime.Progress.FactoryState != string(interfaces.FactoryStatePaused) {
		t.Fatalf("factory state after pause = %q, want PAUSED", pausedSession.Runtime.Progress.FactoryState)
	}

	resumeResp, resumeStatus := postLiveSessionLifecycleControl(t, server.URL, sessionID, "resume")
	if resumeStatus != http.StatusOK {
		t.Fatalf("resume status = %d, want 200", resumeStatus)
	}
	if resumeResp.Operation != factoryapi.FactorySessionLifecycleControlKindResume {
		t.Fatalf("operation = %q, want RESUME", resumeResp.Operation)
	}
	if resumeResp.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("outcome = %q, want ACCEPTED", resumeResp.Outcome)
	}
	if resumeResp.Status != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("status = %q, want RUNNING", resumeResp.Status)
	}

	runningSession := getLiveFactorySession(t, server.URL, sessionID)
	if runningSession.Runtime.Progress.FactoryState == string(interfaces.FactoryStatePaused) {
		t.Fatalf("factory state after resume = %q, want not PAUSED", runningSession.Runtime.Progress.FactoryState)
	}
	if pauseResp.Links == nil || pauseResp.Links.Session == nil || *pauseResp.Links.Session != sessionPath {
		t.Fatalf("pause links = %#v, want session %q", pauseResp.Links, sessionPath)
	}
}

func TestFactoryService_LiveSessionResume_HTTPNoOpWhenAlreadyRunning(t *testing.T) {
	t.Parallel()

	harness := startRunningSessionService(t, runningSessionServiceOptions{
		rootConfig: minimalFactoryConfig(),
	})
	server := httptest.NewServer(api.NewServer(harness.svc, 0, zap.NewNop()).Handler())
	defer server.Close()

	sessionID := factorysessions.DefaultSessionID

	if _, status := postLiveSessionLifecycleControl(t, server.URL, sessionID, "pause"); status != http.StatusOK {
		t.Fatalf("pause status = %d, want 200", status)
	}
	if _, status := postLiveSessionLifecycleControl(t, server.URL, sessionID, "resume"); status != http.StatusOK {
		t.Fatalf("first resume status = %d, want 200", status)
	}

	resumeResp, resumeStatus := postLiveSessionLifecycleControl(t, server.URL, sessionID, "resume")
	if resumeStatus != http.StatusOK {
		t.Fatalf("second resume status = %d, want 200", resumeStatus)
	}
	if resumeResp.Operation != factoryapi.FactorySessionLifecycleControlKindResume {
		t.Fatalf("operation = %q, want RESUME", resumeResp.Operation)
	}
	if resumeResp.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeNoOp {
		t.Fatalf("outcome = %q, want NO_OP", resumeResp.Outcome)
	}
	if resumeResp.Status != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("status = %q, want RUNNING", resumeResp.Status)
	}
}

func TestFactoryService_LiveSessionPauseResume_HTTPEmitsSessionLifecycleControlEvents(t *testing.T) {
	t.Parallel()

	harness := startRunningSessionService(t, runningSessionServiceOptions{
		rootConfig: minimalFactoryConfig(),
	})
	server := httptest.NewServer(api.NewServer(harness.svc, 0, zap.NewNop()).Handler())
	defer server.Close()

	sessionID := factorysessions.DefaultSessionID

	if _, status := postLiveSessionLifecycleControl(t, server.URL, sessionID, "pause"); status != http.StatusOK {
		t.Fatalf("pause status = %d, want 200", status)
	}
	if _, status := postLiveSessionLifecycleControl(t, server.URL, sessionID, "resume"); status != http.StatusOK {
		t.Fatalf("resume status = %d, want 200", status)
	}

	events, err := harness.svc.GetFactoryEvents(context.Background())
	if err != nil {
		t.Fatalf("GetFactoryEvents: %v", err)
	}
	lifecycleControls := filterSessionLifecycleControlEvents(events)
	if len(lifecycleControls) != 2 {
		t.Fatalf("SESSION_LIFECYCLE_CONTROL events = %d, want pause and resume", len(lifecycleControls))
	}
	assertAcceptedSessionLifecycleControlPayload(
		t,
		lifecycleControls[0],
		factoryapi.FactorySessionLifecycleControlKindPause,
		factoryapi.FactorySessionDurableLifecycleStatusRunning,
		factoryapi.FactorySessionDurableLifecycleStatusPaused,
	)
	assertAcceptedSessionLifecycleControlPayload(
		t,
		lifecycleControls[1],
		factoryapi.FactorySessionLifecycleControlKindResume,
		factoryapi.FactorySessionDurableLifecycleStatusPaused,
		factoryapi.FactorySessionDurableLifecycleStatusRunning,
	)
}

func filterSessionLifecycleControlEvents(events []factoryapi.FactoryEvent) []factoryapi.FactoryEvent {
	var lifecycleControls []factoryapi.FactoryEvent
	for _, event := range events {
		if event.Type == factoryapi.FactoryEventTypeSessionLifecycleControl {
			lifecycleControls = append(lifecycleControls, event)
		}
	}
	return lifecycleControls
}

func assertAcceptedSessionLifecycleControlPayload(
	t *testing.T,
	event factoryapi.FactoryEvent,
	operation factoryapi.FactorySessionLifecycleControlKind,
	previousStatus factoryapi.FactorySessionDurableLifecycleStatus,
	newStatus factoryapi.FactorySessionDurableLifecycleStatus,
) {
	t.Helper()
	payload, err := event.Payload.AsSessionLifecycleControlEventPayload()
	if err != nil {
		t.Fatalf("lifecycle payload: %v", err)
	}
	if payload.Operation != operation ||
		payload.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted ||
		payload.PreviousStatus != previousStatus ||
		payload.NewStatus != newStatus {
		t.Fatalf("lifecycle payload = %#v, want %s %s->%s ACCEPTED", payload, operation, previousStatus, newStatus)
	}
}

func TestFactoryService_LiveSessionPauseResume_HTTPDrainsBufferedSubmissionWithoutExternalSignal(t *testing.T) {
	t.Parallel()

	harness := startRunningSessionService(t, runningSessionServiceOptions{
		rootConfig: minimalFactoryConfig(),
	})
	server := httptest.NewServer(api.NewServer(harness.svc, 0, zap.NewNop()).Handler())
	defer server.Close()

	sessionID := factorysessions.DefaultSessionID

	if _, status := postLiveSessionLifecycleControl(t, server.URL, sessionID, "pause"); status != http.StatusOK {
		t.Fatalf("pause status = %d, want 200", status)
	}
	waitForSessionFactoryState(t, harness.svc, sessionID, interfaces.FactoryStatePaused, time.Second, "live session paused")

	submitStatus := postLiveSessionWork(t, server.URL, sessionID, `{"name":"api-paused-submit","workTypeName":"task","traceId":"trace-api-paused-submit"}`)
	if submitStatus != http.StatusOK && submitStatus != http.StatusCreated {
		t.Fatalf("submit status = %d, want 200 or 201", submitStatus)
	}

	assertSessionWorkNotAtPlace(t, harness.svc, sessionID, "task:complete", 300*time.Millisecond)

	if _, status := postLiveSessionLifecycleControl(t, server.URL, sessionID, "resume"); status != http.StatusOK {
		t.Fatalf("resume status = %d, want 200", status)
	}
	waitForSessionFactoryState(t, harness.svc, sessionID, interfaces.FactoryStateRunning, time.Second, "live session resumed")
	waitForSessionWorkAtPlace(t, harness.svc, sessionID, "task:complete", 2*time.Second)

	resumedSession := getLiveFactorySession(t, server.URL, sessionID)
	if resumedSession.Runtime.Progress.FactoryState == string(interfaces.FactoryStatePaused) {
		t.Fatalf("factory state after drain = %q, want not PAUSED", resumedSession.Runtime.Progress.FactoryState)
	}
}

func postLiveSessionLifecycleControl(
	t *testing.T,
	serverURL, sessionID, operation string,
) (factoryapi.FactorySessionLifecycleControlResponse, int) {
	t.Helper()
	resp, err := http.Post(serverURL+"/factory-sessions/"+sessionID+"/"+operation, "application/json", nil)
	if err != nil {
		t.Fatalf("POST /factory-sessions/%s/%s: %v", sessionID, operation, err)
	}
	defer resp.Body.Close()
	var response factoryapi.FactorySessionLifecycleControlResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatalf("decode lifecycle control response: %v", err)
	}
	return response, resp.StatusCode
}

func postLiveSessionWork(t *testing.T, serverURL, sessionID, body string) int {
	t.Helper()
	resp, err := http.Post(
		serverURL+"/factory-sessions/"+sessionID+"/work",
		"application/json",
		bytes.NewReader([]byte(body)),
	)
	if err != nil {
		t.Fatalf("POST /factory-sessions/%s/work: %v", sessionID, err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

func getLiveFactorySession(t *testing.T, serverURL, sessionID string) factoryapi.FactorySession {
	t.Helper()
	resp, err := http.Get(serverURL + "/factory-sessions/" + sessionID)
	if err != nil {
		t.Fatalf("GET /factory-sessions/%s: %v", sessionID, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /factory-sessions/%s status = %d, want 200", sessionID, resp.StatusCode)
	}
	var session factoryapi.FactorySession
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		t.Fatalf("decode factory session: %v", err)
	}
	return session
}

func assertSessionWorkNotAtPlace(t *testing.T, svc *FactoryService, sessionID, placeID string, duration time.Duration) {
	t.Helper()
	deadline := time.Now().Add(duration)
	for time.Now().Before(deadline) {
		if sessionHasWorkAtPlace(t, svc, sessionID, placeID) {
			t.Fatalf("work reached %s while session %s remained paused", placeID, sessionID)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func waitForSessionWorkAtPlace(t *testing.T, svc *FactoryService, sessionID, placeID string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if sessionHasWorkAtPlace(t, svc, sessionID, placeID) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for work at %s in session %s", placeID, sessionID)
}

func sessionHasWorkAtPlace(t *testing.T, svc *FactoryService, sessionID, placeID string) bool {
	t.Helper()
	snapshot, err := svc.GetEngineStateSnapshotForSession(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("GetEngineStateSnapshotForSession(%s): %v", sessionID, err)
	}
	for _, token := range snapshot.Marking.Tokens {
		if token.PlaceID == placeID {
			return true
		}
	}
	return false
}
