package root_composition_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestSessionsLifecycleAndRuntimeOpeningActivateThroughRootBuildProcessAfterLifecycle
// proves session lifecycle and runtime-opening activate through public Sessions
// HTTP surfaces after runtime lifecycle on a process constructed only through
// root.BuildProcess with Sessions effects replaced via published edges.Edges
// fields.
// backendsizecheck:ignore-function pre-existing baseline debt recorded 2026-08-08; split this oversized code into focused units and remove this exemption
func TestSessionsLifecycleAndRuntimeOpeningActivateThroughRootBuildProcessAfterLifecycle(t *testing.T) {
	t.Parallel()
	acquireRootCompositionFixtureSlot(t)

	fixture := rootCompositionSharedProcess(t)
	dir := support.ScaffoldFactory(t, sessionsLifecycleRuntimeOpeningFactoryConfig())
	baseURL := fixture.baseURL
	lifecycleBefore := fixture.effects.lifecycleCount()

	defaultSession := support.GetDefaultSession(t, baseURL)
	if !defaultSession.IsDefault || defaultSession.Id == "" {
		t.Fatalf("default live session = %#v, want non-empty default session identity", defaultSession)
	}

	listed := support.GetJSON[factoryapi.ListFactorySessionsResponse](t, baseURL+"/factory-sessions")
	if !sessionSummaryContains(listed.Sessions, defaultSession.Id) {
		t.Fatalf("list sessions = %#v, want default session %q", listed.Sessions, defaultSession.Id)
	}

	pause := postSessionsLifecycleControl(
		t,
		baseURL,
		factorysessions.DefaultSessionID,
		factoryapi.FactorySessionLifecycleControlKindPause,
	)
	if pause.Operation != factoryapi.FactorySessionLifecycleControlKindPause ||
		pause.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("pause response = %#v, want accepted pause", pause)
	}
	paused := support.GetDefaultSession(t, baseURL)
	if paused.Runtime.LifecycleControlStatus == nil ||
		*paused.Runtime.LifecycleControlStatus != factoryapi.FactorySessionDurableLifecycleStatusPaused {
		t.Fatalf(
			"paused live session lifecycleControlStatus = %#v, want %q",
			paused.Runtime.LifecycleControlStatus,
			factoryapi.FactorySessionDurableLifecycleStatusPaused,
		)
	}

	resume := postSessionsLifecycleControl(
		t,
		baseURL,
		factorysessions.DefaultSessionID,
		factoryapi.FactorySessionLifecycleControlKindResume,
	)
	if resume.Operation != factoryapi.FactorySessionLifecycleControlKindResume ||
		resume.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("resume response = %#v, want accepted resume", resume)
	}
	running := support.GetDefaultSession(t, baseURL)
	if running.Runtime.LifecycleControlStatus == nil ||
		*running.Runtime.LifecycleControlStatus != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf(
			"resumed live session lifecycleControlStatus = %#v, want %q",
			running.Runtime.LifecycleControlStatus,
			factoryapi.FactorySessionDurableLifecycleStatusRunning,
		)
	}

	opened := postSessionsOpen(t, baseURL, dir)
	if opened.Session == nil || opened.Session.Id == "" {
		t.Fatalf("opened live session = %#v, want canonical session identity", opened)
	}
	fixture.trackSession(t, opened.Session.Id)
	if opened.Session.Id == defaultSession.Id {
		t.Fatalf("opened session id = %q, want distinct live identity from default", opened.Session.Id)
	}

	selected := support.GetJSON[factoryapi.FactorySessionGetResponse](
		t,
		baseURL+"/factory-sessions/"+opened.Session.Id,
	)
	resolved, err := selected.AsFactorySession()
	if err != nil {
		t.Fatalf("decode opened live session: %v", err)
	}
	if resolved.Id != opened.Session.Id {
		t.Fatalf("selected session id = %q, want %q", resolved.Id, opened.Session.Id)
	}
	if resolved.Runtime.Status == "" {
		t.Fatalf("opened session missing runtime-opening status markers: %#v", resolved)
	}

	afterOpen := support.GetJSON[factoryapi.ListFactorySessionsResponse](t, baseURL+"/factory-sessions")
	if !sessionSummaryContains(afterOpen.Sessions, opened.Session.Id) {
		t.Fatalf("list sessions after open = %#v, want opened session %q", afterOpen.Sessions, opened.Session.Id)
	}

	fixture.closeSession(t, opened.Session.Id)
	assertSessionsSessionNotFound(t, baseURL, opened.Session.Id)

	if got := fixture.effects.lifecycleCount() - lifecycleBefore; got <= 0 {
		t.Fatalf("lifecycle effect calls after public session operations = %d, want > 0 via edges", got)
	}
	if got := fixture.effects.runtimeID.Load(); got <= 0 {
		t.Fatalf("runtime instance id generations = %d, want > 0 via edges during runtime opening", got)
	}
}

func sessionsLifecycleRuntimeOpeningFactoryConfig() map[string]any {
	return map[string]any{
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]string{{"name": "worker-a"}},
		"workstations": []map[string]any{{
			"name":      "process",
			"worker":    "worker-a",
			"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
			"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
			"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
		}},
	}
}

func sessionSummaryContains(summaries []factoryapi.FactorySessionSummary, sessionID string) bool {
	for _, summary := range summaries {
		if summary.Id == sessionID {
			return true
		}
	}
	return false
}

func postSessionsOpen(t *testing.T, baseURL, folderPath string) factoryapi.OpenFactorySessionResponse {
	t.Helper()
	return postSessionsJSON[factoryapi.OpenFactorySessionResponse](
		t,
		baseURL+"/factory-sessions",
		factoryapi.OpenFactorySessionRequest{FolderPath: folderPath},
		"open Factory Session through public HTTP surface",
	)
}

func postSessionsLifecycleControl(
	t *testing.T,
	baseURL string,
	sessionID string,
	operation factoryapi.FactorySessionLifecycleControlKind,
) factoryapi.FactorySessionLifecycleControlResponse {
	t.Helper()
	pathSegment := "pause"
	if operation == factoryapi.FactorySessionLifecycleControlKindResume {
		pathSegment = "resume"
	}
	return postSessionsJSON[factoryapi.FactorySessionLifecycleControlResponse](
		t,
		baseURL+"/factory-sessions/"+sessionID+"/"+pathSegment,
		factoryapi.FactorySessionLifecycleControlRequest{},
		"apply Factory Session lifecycle control through public HTTP surface",
	)
}

func postSessionsJSON[T any](t *testing.T, endpoint string, request any, failurePrefix string) T {
	t.Helper()
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("%s: marshal request: %v", failurePrefix, err)
	}
	response, err := http.Post(endpoint, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("%s: POST %s: %v", failurePrefix, endpoint, err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf("%s: POST %s status = %d, want success: %s", failurePrefix, endpoint, response.StatusCode, payload)
	}
	var decoded T
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		t.Fatalf("%s: decode %s response: %v", failurePrefix, endpoint, err)
	}
	return decoded
}

func closeSessionsSession(t *testing.T, baseURL, sessionID string) {
	t.Helper()
	support.TerminateFactorySessionAt(t, baseURL, sessionID)
	support.WaitForSessionStopped(t, baseURL, sessionID, 10*time.Second)
	request, err := http.NewRequest(http.MethodDelete, baseURL+"/factory-sessions/"+sessionID, nil)
	if err != nil {
		t.Fatalf("construct close session request: %v", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("DELETE Factory Session %q: %v", sessionID, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf("DELETE Factory Session %q status = %d, want 204: %s", sessionID, response.StatusCode, payload)
	}
}

func assertSessionsSessionNotFound(t *testing.T, baseURL, sessionID string) {
	t.Helper()
	response, err := http.Get(baseURL + "/factory-sessions/" + sessionID)
	if err != nil {
		t.Fatalf("GET closed Factory Session %q: %v", sessionID, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf("GET closed Factory Session %q status = %d, want 404: %s", sessionID, response.StatusCode, payload)
	}
}
