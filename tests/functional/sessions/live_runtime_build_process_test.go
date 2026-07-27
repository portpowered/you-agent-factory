package sessions_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestBuildProcessRoutesLiveOpenListControlAndCloseThroughFactorySessionsRoot proves
// live open, ordered registry reads, lifecycle control, and close resolve through
// the Factory Sessions root composed by root.BuildProcess without constructing the
// private live_runtime capability outside Sessions composition.
func TestBuildProcessRoutesLiveOpenListControlAndCloseThroughFactorySessionsRoot(t *testing.T) {
	dir := support.ScaffoldFactory(t, liveRuntimePipelineConfig())
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		UseMockWorkers:            true,
		WaitForServiceModeRuntime: true,
	})
	baseURL := server.URL()

	defaultSession := support.GetDefaultSession(t, baseURL)
	if !defaultSession.IsDefault || defaultSession.Id == "" {
		t.Fatalf("default live session = %#v, want non-empty default session identity", defaultSession)
	}

	listed := support.GetJSON[factoryapi.ListFactorySessionsResponse](t, baseURL+"/factory-sessions")
	if !liveRuntimeSummaryContains(listed.Sessions, defaultSession.Id) {
		t.Fatalf("list sessions = %#v, want default session %q", listed.Sessions, defaultSession.Id)
	}

	pause := postLiveRuntimeLifecycleControl(
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

	resume := postLiveRuntimeLifecycleControl(
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

	opened := postLiveRuntimeOpen(t, baseURL, dir)
	if opened.Session == nil || opened.Session.Id == "" {
		t.Fatalf("opened live session = %#v, want canonical session identity", opened)
	}
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

	afterOpen := support.GetJSON[factoryapi.ListFactorySessionsResponse](t, baseURL+"/factory-sessions")
	if !liveRuntimeSummaryContains(afterOpen.Sessions, opened.Session.Id) {
		t.Fatalf("list sessions after open = %#v, want opened session %q", afterOpen.Sessions, opened.Session.Id)
	}

	closeLiveRuntimeSession(t, baseURL, opened.Session.Id)
	assertLiveRuntimeSessionNotFound(t, baseURL, opened.Session.Id)

	afterClose := support.GetJSON[factoryapi.ListFactorySessionsResponse](t, baseURL+"/factory-sessions")
	if liveRuntimeSummaryContains(afterClose.Sessions, opened.Session.Id) {
		t.Fatalf("list sessions after close = %#v, want opened session %q retired", afterClose.Sessions, opened.Session.Id)
	}

	closeLiveRuntimeSession(t, baseURL, defaultSession.Id)
}

func liveRuntimePipelineConfig() map[string]any {
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

func liveRuntimeSummaryContains(summaries []factoryapi.FactorySessionSummary, sessionID string) bool {
	for _, summary := range summaries {
		if summary.Id == sessionID {
			return true
		}
	}
	return false
}

func postLiveRuntimeOpen(t *testing.T, baseURL, folderPath string) factoryapi.OpenFactorySessionResponse {
	t.Helper()
	return postLiveRuntimeJSON[factoryapi.OpenFactorySessionResponse](
		t,
		baseURL+"/factory-sessions",
		factoryapi.OpenFactorySessionRequest{FolderPath: folderPath},
		"open live Factory Session",
	)
}

func postLiveRuntimeLifecycleControl(
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
	return postLiveRuntimeJSON[factoryapi.FactorySessionLifecycleControlResponse](
		t,
		baseURL+"/factory-sessions/"+sessionID+"/"+pathSegment,
		factoryapi.FactorySessionLifecycleControlRequest{},
		"apply live Factory Session lifecycle control",
	)
}

func postLiveRuntimeJSON[T any](t *testing.T, endpoint string, request any, failurePrefix string) T {
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

func closeLiveRuntimeSession(t *testing.T, baseURL, sessionID string) {
	t.Helper()
	request, err := http.NewRequest(http.MethodDelete, baseURL+"/factory-sessions/"+sessionID, nil)
	if err != nil {
		t.Fatalf("construct close live session request: %v", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("DELETE live Factory Session %q: %v", sessionID, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf("DELETE live Factory Session %q status = %d, want 204: %s", sessionID, response.StatusCode, payload)
	}
}

func assertLiveRuntimeSessionNotFound(t *testing.T, baseURL, sessionID string) {
	t.Helper()
	response, err := http.Get(baseURL + "/factory-sessions/" + sessionID)
	if err != nil {
		t.Fatalf("GET closed live Factory Session %q: %v", sessionID, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf("GET closed live Factory Session %q status = %d, want 404: %s", sessionID, response.StatusCode, payload)
	}
}
