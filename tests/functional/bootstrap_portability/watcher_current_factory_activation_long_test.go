//go:build functionallong

package bootstrap_portability

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/transports/http/apitypes"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestCurrentFactoryActivationFixture_ActivatesSecondPersistedFactoryAndResolvesCurrentFactory(t *testing.T) {
	support.SkipLongFunctional(t, "slow current-factory activation persistence smoke")

	rootDir := t.TempDir()

	alphaDir := createAndActivateFunctionalNamedFactoryFixture(t, rootDir, "alpha", functionalNamedFactoryPayload(t, "alpha"))
	betaDir := createFunctionalNamedFactoryFixture(t, rootDir, "beta", functionalNamedFactoryPayload(t, "beta"))

	server := startFunctionalServer(t, rootDir, true)
	assertCurrentFactoryReadback(t, server.URL(), "alpha", alphaDir)

	activateNamedFactoryOverHTTP(t, server.URL(), functionalNamedFactoryPayload(t, "beta"))

	wantDir := betaDir
	assertCurrentFactoryReadback(t, server.URL(), "beta", wantDir)
}

func createFunctionalNamedFactoryFixture(
	t *testing.T,
	rootDir string,
	name string,
	payload []byte,
) string {
	return createFunctionalNamedFactoryFixtureAtBoundary(t, rootDir, name, payload, false)
}

func createAndActivateFunctionalNamedFactoryFixture(
	t *testing.T,
	rootDir string,
	name string,
	payload []byte,
) string {
	return createFunctionalNamedFactoryFixtureAtBoundary(t, rootDir, name, payload, true)
}

func createFunctionalNamedFactoryFixtureAtBoundary(
	t *testing.T,
	rootDir string,
	name string,
	payload []byte,
	activate bool,
) string {
	t.Helper()

	sourceDir := t.TempDir()
	sourcePath := filepath.Join(sourceDir, interfaces.FactoryConfigFile)
	if err := os.WriteFile(sourcePath, payload, 0o600); err != nil {
		t.Fatalf("write customer Factory source %s: %v", name, err)
	}
	if activate {
		return support.CreateAndActivateNamedFactoryAtRoot(t, sourceDir, rootDir, name, sourcePath)
	}
	return support.CreateNamedFactoryAtRoot(t, sourceDir, rootDir, name, sourcePath)
}

func TestCurrentFactoryActivationFixture_WatchedFileExecutionFollowsActivatedFactory(t *testing.T) {
	support.SkipLongFunctional(t, "slow current-factory watcher activation smoke")

	rootDir := t.TempDir()
	alphaDir := copyAndActivateCurrentFactoryFixture(t, rootDir, "alpha")
	betaDir := copyCurrentFactoryFixture(t, rootDir, "beta")
	createCurrentFactoryWatchChannel(t, alphaDir, "task", "activated")
	createCurrentFactoryWatchChannel(t, betaDir, "task", "activated")

	provider := testutil.NewMockProvider(
		support.AcceptedProviderResponse(),
		support.AcceptedProviderResponse(),
		support.AcceptedProviderResponse(),
	)
	server := startFunctionalServer(t, rootDir, false, serviceedges.Edges{
		ProviderOverride: provider,
	})
	support.WaitForRuntimeIdle(t, server.URL(), 5*time.Second)
	activateNamedFactoryOverHTTP(t, server.URL(), functionalNamedFactoryPayload(t, "beta"))
	assertCurrentFactoryReadback(t, server.URL(), "beta", betaDir)
	createCurrentFactoryWatchChannel(t, betaDir, "task", "activated")

	writeCurrentFactoryWatchedInput(t, betaDir, "task", "activated", "beta-work.json", []byte(`{"title":"beta watched work"}`))
	waitForCurrentFactoryWatchedCompletion(t, betaDir, server, provider, 1, 5*time.Second)

	writeCurrentFactoryWatchedInput(t, alphaDir, "task", "activated", "alpha-work.json", []byte(`{"title":"alpha watched work"}`))
	assertNoAdditionalCurrentFactoryWork(t, betaDir, server, provider, 750*time.Millisecond)
}

func TestCurrentFactoryActivationFixture_LiveAPIReadsFollowActivatedFactory(t *testing.T) {
	support.SkipLongFunctional(t, "slow current-factory live API activation smoke")

	rootDir := t.TempDir()

	createAndActivateFunctionalNamedFactoryFixture(
		t,
		rootDir,
		"alpha",
		functionalNamedFactoryPayloadWithTerminalState(t, "alpha", "alpha-complete"),
	)
	createFunctionalNamedFactoryFixture(
		t,
		rootDir,
		"beta",
		functionalNamedFactoryPayloadWithTerminalState(t, "beta", "beta-complete"),
	)
	provider := testutil.NewMockProvider(
		support.AcceptedProviderResponse(),
		support.AcceptedProviderResponse(),
	)
	server := startFunctionalServer(t, rootDir, false, serviceedges.Edges{
		ProviderOverride: provider,
	})

	support.WaitForRuntimeIdle(t, server.URL(), 5*time.Second)
	activateNamedFactoryOverHTTP(
		t,
		server.URL(),
		functionalNamedFactoryPayloadWithTerminalState(t, "beta", "beta-complete"),
	)
	assertCurrentFactoryReadback(t, server.URL(), "beta", filepath.Join(rootDir, "beta"))
	waitForCurrentFactoryActivatedRuntime(t, server, "task:beta-complete", 5*time.Second)

	traceID := server.SubmitWork(t, "task", json.RawMessage(`{"title":"beta api work"}`))
	if traceID == "" {
		t.Fatal("POST /work returned an empty trace ID after activation")
	}
	work := waitForGeneratedWorkAtPlace(t, server.URL(), traceID, "task:beta-complete", 5*time.Second)
	if len(work.Results) != 1 {
		status := support.GetJSON[factoryapi.StatusResponse](t, server.URL()+"/status")
		t.Fatalf(
			"GET /work result count after activation = %d, want 1; provider_calls=%d factory_state=%q runtime_status=%q total_tokens=%d",
			len(work.Results),
			provider.CallCount(),
			status.FactoryState,
			status.RuntimeStatus,
			status.TotalTokens,
		)
	}
	if generatedWorkPlaceID(work.Results[0]) != "task:beta-complete" {
		t.Fatalf("GET /work place_id after activation = %q, want task:beta-complete", generatedWorkPlaceID(work.Results[0]))
	}

	status := getGeneratedJSON[factoryapi.StatusResponse](t, server.URL()+"/status")
	if status.RuntimeStatus != string(interfaces.RuntimeStatusIdle) {
		t.Fatalf("GET /status runtime_status after activation = %q, want %q", status.RuntimeStatus, interfaces.RuntimeStatusIdle)
	}
	if status.TotalTokens != 1 {
		t.Fatalf("GET /status total_tokens after activation = %d, want 1", status.TotalTokens)
	}
	if status.Categories.Terminal != 1 {
		t.Fatalf("GET /status terminal count after activation = %d, want 1", status.Categories.Terminal)
	}

	if lastCall := provider.LastCall(); lastCall.ProjectID != "beta" {
		t.Fatalf("API submit project = %q, want beta", lastCall.ProjectID)
	}
}

func copyCurrentFactoryFixture(t *testing.T, rootDir, name string) string {
	return copyCurrentFactoryFixtureAtBoundary(t, rootDir, name, false)
}

func copyAndActivateCurrentFactoryFixture(t *testing.T, rootDir, name string) string {
	return copyCurrentFactoryFixtureAtBoundary(t, rootDir, name, true)
}

func copyCurrentFactoryFixtureAtBoundary(t *testing.T, rootDir, name string, activate bool) string {
	t.Helper()

	srcDir := support.LegacyFixtureDir(t, "filewatcher_flow")
	sourcePath := filepath.Join(srcDir, interfaces.FactoryConfigFile)
	if activate {
		return support.CreateAndActivateNamedFactoryAtRoot(t, srcDir, rootDir, name, sourcePath)
	}
	return support.CreateNamedFactoryAtRoot(t, srcDir, rootDir, name, sourcePath)
}

func createCurrentFactoryWatchChannel(t *testing.T, factoryDir, workType, channel string) {
	t.Helper()

	inputDir := filepath.Join(factoryDir, interfaces.InputsDir, workType, channel)
	if err := os.MkdirAll(inputDir, 0o755); err != nil {
		t.Fatalf("create watched input dir %q: %v", inputDir, err)
	}
}

func writeCurrentFactoryWatchedInput(t *testing.T, factoryDir, workType, channel, name string, payload []byte) {
	t.Helper()

	inputDir := filepath.Join(factoryDir, interfaces.InputsDir, workType, channel)
	if err := os.WriteFile(filepath.Join(inputDir, name), payload, 0o644); err != nil {
		t.Fatalf("write watched input %q: %v", name, err)
	}
}

func waitForCurrentFactoryActivatedRuntime(
	t *testing.T,
	server *functionalAPIServer,
	wantPlaceID string,
	timeout time.Duration,
) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	var lastFactoryState string
	var lastRuntimeStatus interfaces.RuntimeStatus
	var sawPlace bool
	for time.Now().Before(deadline) {
		status := support.GetJSON[factoryapi.StatusResponse](t, server.URL()+"/status")
		factory := support.GetJSON[factoryapi.Factory](
			t,
			server.URL()+"/factory-sessions/~default/factory",
		)
		lastFactoryState = status.FactoryState
		lastRuntimeStatus = interfaces.RuntimeStatus(status.RuntimeStatus)
		sawPlace = generatedFactoryHasPlace(factory, wantPlaceID)
		if lastRuntimeStatus == interfaces.RuntimeStatusIdle && sawPlace {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}

	t.Fatalf(
		"timed out waiting for activated runtime %q; factory_state=%q runtime_status=%q saw_place=%t",
		wantPlaceID,
		lastFactoryState,
		lastRuntimeStatus,
		sawPlace,
	)
}

func waitForCurrentFactoryWatchedCompletion(
	t *testing.T,
	wantDir string,
	server *functionalAPIServer,
	provider *testutil.MockProvider,
	wantCalls int,
	timeout time.Duration,
) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		assertCurrentFactoryReadback(t, server.URL(), "beta", wantDir)

		status := support.GetJSON[factoryapi.StatusResponse](t, server.URL()+"/status")
		work := support.ListDefaultSessionWork(t, server.URL())
		if status.RuntimeStatus == string(interfaces.RuntimeStatusIdle) &&
			provider.CallCount() == wantCalls &&
			len(work.Results) == wantCalls &&
			allWorkInState(work.Results, "complete") {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	status := support.GetJSON[factoryapi.StatusResponse](t, server.URL()+"/status")
	work := support.ListDefaultSessionWork(t, server.URL())
	t.Fatalf(
		"timed out waiting for activated-factory watched completion: provider_calls=%d runtime_status=%q work=%#v",
		provider.CallCount(),
		status.RuntimeStatus,
		work.Results,
	)
}

func assertNoAdditionalCurrentFactoryWork(
	t *testing.T,
	wantDir string,
	server *functionalAPIServer,
	provider *testutil.MockProvider,
	stableFor time.Duration,
) {
	t.Helper()

	deadline := time.Now().Add(stableFor)
	for time.Now().Before(deadline) {
		assertCurrentFactoryReadback(t, server.URL(), "beta", wantDir)

		status := support.GetJSON[factoryapi.StatusResponse](t, server.URL()+"/status")
		work := support.ListDefaultSessionWork(t, server.URL())
		if provider.CallCount() != 1 {
			t.Fatalf("old factory directory still triggered work: provider call count = %d, want 1", provider.CallCount())
		}
		if status.RuntimeStatus != string(interfaces.RuntimeStatusIdle) {
			t.Fatalf("runtime status after old-factory write = %q, want %q", status.RuntimeStatus, interfaces.RuntimeStatusIdle)
		}
		if len(work.Results) != 1 || !allWorkInState(work.Results, "complete") {
			t.Fatalf("GET /work after old-factory write = %#v, want one complete work", work.Results)
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func assertCurrentFactoryReadback(t *testing.T, serverURL, wantName, wantDir string) {
	t.Helper()

	current := support.GetJSON[factoryapi.Factory](t, serverURL+"/factory-sessions/~default/factory")
	if current.Name != factoryapi.FactoryName(wantName) {
		t.Fatalf("current factory name = %q, want %q", current.Name, wantName)
	}
	if current.FactoryDirectory == nil || *current.FactoryDirectory != wantDir {
		t.Fatalf("current factory directory = %#v, want %q", current.FactoryDirectory, wantDir)
	}
}

func activateNamedFactoryOverHTTP(t *testing.T, baseURL string, payload []byte) {
	t.Helper()
	var factory factoryapi.Factory
	if err := json.Unmarshal(payload, &factory); err != nil {
		t.Fatalf("decode named factory API payload: %v", err)
	}
	// UPSERT_NAMED_AND_ACTIVATE is an optimistic-concurrency write. These
	// fixtures are persisted before the process starts, so emulate a client
	// editing that stored definition by submitting an advanced version.
	factory.Version = &factoryapi.HybridLogicalTimestamp{
		Logical:  apitypes.Int64String(1<<62 - 1),
		Physical: time.Now().UTC().Add(time.Hour),
	}
	mode := factoryapi.FactorySaveModeUpsertNamedAndActivate
	body, err := json.Marshal(factoryapi.SaveFactoryForSessionRequest{
		Factory: factory,
		Mode:    &mode,
	})
	if err != nil {
		t.Fatalf("encode named factory activation: %v", err)
	}
	request, err := http.NewRequest(
		http.MethodPut,
		baseURL+"/factory-sessions/~default/factory",
		bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("build named factory activation request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("activate named factory over HTTP: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		responseBody, _ := io.ReadAll(response.Body)
		t.Fatalf(
			"activate named factory status = %d, want 200: %s",
			response.StatusCode,
			string(responseBody),
		)
	}
}

func generatedFactoryHasPlace(factory factoryapi.Factory, placeID string) bool {
	if factory.WorkTypes == nil {
		return false
	}
	for _, workType := range *factory.WorkTypes {
		for _, state := range workType.States {
			if workType.Name+":"+state.Name == placeID {
				return true
			}
		}
	}
	return false
}

func allWorkInState(work []factoryapi.Work, stateName string) bool {
	if len(work) == 0 {
		return false
	}
	for _, item := range work {
		if item.State == nil || item.State.Name != stateName {
			return false
		}
	}
	return true
}

func functionalNamedFactoryPayload(t *testing.T, project string) []byte {
	return functionalNamedFactoryPayloadWithTerminalState(t, project, "complete")
}

func functionalNamedFactoryPayloadWithTerminalState(t *testing.T, project, terminalState string) []byte {
	t.Helper()

	payload, err := json.Marshal(map[string]any{
		"name": project,
		"id":   project,
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": terminalState, "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]any{{
			"name":             "worker-a",
			"type":             "MODEL_WORKER",
			"body":             "You are worker " + project + ".",
			"modelProvider":    "CLAUDE",
			"executorProvider": "SCRIPT_WRAP",
			"model":            "claude-sonnet-4-20250514",
		}},
		"workstations": []map[string]any{{
			"name":      "process",
			"behavior":  "STANDARD",
			"worker":    "worker-a",
			"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
			"outputs":   []map[string]string{{"workType": "task", "state": terminalState}},
			"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
			"type":      "MODEL_WORKSTATION",
			"body":      "Do the " + project + " work.",
		}},
	})
	if err != nil {
		t.Fatalf("marshal functional named factory payload: %v", err)
	}
	return payload
}
