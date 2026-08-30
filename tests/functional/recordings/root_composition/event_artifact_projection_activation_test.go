package root_composition_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const recordingsSurfaceActivationArtifactWorkflow = `return (async function () {
  const artifactRef = workflow.artifact({
    kind: "log",
    label: "fun-recordings-surface-activation",
    content: { message: "recordings artifact activation" },
  });
  return { artifactRef };
})();`

// TestRecordingsEventArtifactProjectionSurfacesActivateThroughRootBuildProcessAfterLifecycle
// proves canonical event, artifact, and projection-query surfaces activate only after
// runtime lifecycle on a process constructed only through root.BuildProcess. Deeper
// event/artifact/projection coverage remains under tests/functional/replay_contracts,
// runtime_api, orchestration/javascript/contracts, and session parity proofs; this
// test closes the explicit public-process activation gap for the Recordings FUN suite
// home.
func TestRecordingsEventArtifactProjectionSurfacesActivateThroughRootBuildProcessAfterLifecycle(
	t *testing.T,
) {
	t.Parallel()

	dir := support.ScaffoldFactory(t, recordingsSurfaceActivationFactoryConfig())
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"FUN Recordings event/artifact/projection activation"}`))

	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		UseMockWorkers:            true,
		WaitForServiceModeRuntime: true,
	})
	t.Cleanup(func() { server.Stop(t) })

	baseURL := server.URL()
	terminalObservation := support.OpenDefaultSessionTerminalFactoryEventObservation(t, baseURL)
	support.WaitForSessionWorkTerminalFromFactoryEvents(t, baseURL, "~default", 15*time.Second)

	events := support.GetFactoryEventsAt(t, baseURL)
	if recordingsActivationLiveEventCount(events, factoryapi.FactoryEventTypeDispatchRequest) == 0 {
		t.Fatal("canonical event list missing dispatch request events after lifecycle")
	}
	if recordingsActivationLiveEventCount(events, factoryapi.FactoryEventTypeDispatchResponse) == 0 {
		t.Fatal("canonical event list missing dispatch response events after lifecycle")
	}

	defaultSession := support.GetDefaultSession(t, baseURL)
	if defaultSession.Runtime.Progress.Categories.Terminal != 1 {
		t.Fatalf(
			"default session projection terminal count = %d, want 1 after lifecycle",
			defaultSession.Runtime.Progress.Categories.Terminal,
		)
	}

	globalStatus := support.GetJSON[factoryapi.StatusResponse](t, baseURL+"/status")
	if globalStatus.Categories.Terminal != 1 {
		t.Fatalf(
			"global projection terminal count = %d, want 1 after lifecycle",
			globalStatus.Categories.Terminal,
		)
	}
	if globalStatus.FactoryState != string(interfaces.FactoryStateRunning) {
		t.Fatalf(
			"global projection factoryState = %q, want %q",
			globalStatus.FactoryState,
			interfaces.FactoryStateRunning,
		)
	}

	durableSession := startRecordingsSurfaceActivationDurableSession(t, baseURL)
	artifactList := support.GetJSON[factoryapi.ListFactorySessionArtifactsResponse](
		t,
		recordingsSurfaceActivationSessionEndpoint(baseURL, durableSession.SessionId)+"/artifacts",
	)
	if artifactList.SessionId != durableSession.SessionId {
		t.Fatalf(
			"artifact list sessionId = %q, want %q",
			artifactList.SessionId,
			durableSession.SessionId,
		)
	}
	if len(artifactList.Artifacts) == 0 {
		t.Fatalf("artifact list = %#v, want at least one artifact after lifecycle", artifactList.Artifacts)
	}

	artifactDetail := support.GetJSON[factoryapi.FactorySessionArtifactDetail](
		t,
		recordingsSurfaceActivationSessionEndpoint(baseURL, durableSession.SessionId)+"/artifacts/"+artifactList.Artifacts[0].Id,
	)
	if artifactDetail.Id != artifactList.Artifacts[0].Id {
		t.Fatalf(
			"artifact detail id = %q, want %q",
			artifactDetail.Id,
			artifactList.Artifacts[0].Id,
		)
	}
	if artifactDetail.Label == nil || strings.TrimSpace(*artifactDetail.Label) == "" {
		t.Fatalf("artifact detail label = %#v, want non-empty label after lifecycle", artifactDetail.Label)
	}
	if artifactDetail.ContentHash == nil || strings.TrimSpace(*artifactDetail.ContentHash) == "" {
		t.Fatalf("artifact detail contentHash = %#v, want non-empty hash after lifecycle", artifactDetail.ContentHash)
	}
	server.Stop(t)
	terminalObservation.Wait(15 * time.Second)
}

func startRecordingsSurfaceActivationDurableSession(
	t *testing.T,
	baseURL string,
) factoryapi.FactorySessionSyncExecutionResponse {
	t.Helper()

	dialect := "you-workflow-v1"
	return postRecordingsSurfaceActivationJSON[factoryapi.FactorySessionSyncExecutionResponse](
		t,
		baseURL+"/factory-sessions/sync",
		factoryapi.FactorySessionExecutionRequest{
			RequestId: "fun-recordings-surface-activation-sync",
			Source: factoryapi.FactorySessionExecutionSource{
				Kind: factoryapi.FactorySessionExecutionSourceKindInlineWorkflow,
				InlineWorkflow: &factoryapi.FactorySessionExecutionInlineWorkflow{
					Dialect: &dialect,
					InlineSource: factoryapi.FactoryOrchestratorJavaScriptInlineSource{
						Encoding: factoryapi.FactoryOrchestratorJavaScriptInlineSourceEncodingUtf8,
						Inline:   recordingsSurfaceActivationArtifactWorkflow,
					},
				},
			},
		},
		"start Recordings surface activation durable session",
	)
}

func recordingsSurfaceActivationSessionEndpoint(baseURL, sessionID string) string {
	return strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + sessionID
}

func postRecordingsSurfaceActivationJSON[T any](
	t *testing.T,
	endpoint string,
	request any,
	label string,
) T {
	t.Helper()

	body, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("%s: marshal request: %v", label, err)
	}
	response, err := http.Post(endpoint, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("%s: POST %s: %v", label, endpoint, err)
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("%s: read response: %v", label, err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		t.Fatalf("%s: POST %s status = %d, want success: %s", label, endpoint, response.StatusCode, payload)
	}
	var decoded T
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("%s: decode response: %v\n%s", label, err, payload)
	}
	if sync, ok := any(decoded).(factoryapi.FactorySessionSyncExecutionResponse); ok {
		if sync.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
			t.Fatalf("%s durable session status = %q, want SUCCEEDED", label, sync.Status)
		}
		if strings.TrimSpace(sync.SessionId) == "" {
			t.Fatalf("%s durable session id is empty", label)
		}
	}
	return decoded
}

func recordingsSurfaceActivationFactoryConfig() map[string]any {
	return map[string]any{
		"name": "recordings-surface-activation",
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]string{{"name": "mock-worker"}},
		"workstations": []map[string]any{{
			"name":      "process-task",
			"worker":    "mock-worker",
			"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
			"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
			"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
		}},
	}
}
