//go:build functionallong

package current

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/transports/http/apitypes"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func startCurrentFactoryServerWithProviderRunnerAndSetup(
	t *testing.T,
	rootDir string,
	runner platformprocess.CommandRunner,
	setup currentFactoryServerSetup,
) *support.FunctionalAPIServer {
	t.Helper()
	return support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                rootDir,
		WaitForServiceModeRuntime: true,
		Edges: edges.Edges{
			ProviderCommandRunner: runner,
		},
		BeforeStart: setup,
	})
}

func seedNamedFactoryRootWithTerminalStateAndProcess(
	t *testing.T,
	process support.Process,
	env []string,
	rootDir, name, terminalState string,
) string {
	t.Helper()

	sourceDir := t.TempDir()
	sourcePath := filepath.Join(sourceDir, interfaces.FactoryConfigFile)
	if err := os.WriteFile(sourcePath, functionalNamedFactoryPayloadWithTerminalState(t, name, terminalState), 0o600); err != nil {
		t.Fatalf("write customer Factory source %s: %v", name, err)
	}
	return support.CreateAndActivateNamedFactoryAtRootWithProcess(
		t,
		process,
		env,
		sourceDir,
		rootDir,
		name,
		sourcePath,
	)
}

func seedFilewatcherNamedFactoryRootWithProcess(
	t *testing.T,
	process support.Process,
	env []string,
	rootDir, name string,
	activate bool,
) string {
	t.Helper()

	srcDir := support.LegacyFixtureDir(t, "filewatcher_flow")
	sourcePath := filepath.Join(srcDir, interfaces.FactoryConfigFile)
	if activate {
		return support.CreateAndActivateNamedFactoryAtRootWithProcess(
			t,
			process,
			env,
			srcDir,
			rootDir,
			name,
			sourcePath,
		)
	}
	return support.CreateNamedFactoryAtRootWithProcess(
		t,
		process,
		env,
		srcDir,
		rootDir,
		name,
		sourcePath,
	)
}

func namedFilewatcherFactoryPayloadWithProcess(
	t *testing.T,
	process support.Process,
	env []string,
	name string,
) []byte {
	t.Helper()

	sourcePath := filepath.Join(support.LegacyFixtureDir(t, "filewatcher_flow"), interfaces.FactoryConfigFile)
	factory, err := support.LoadedFactoryWithProcessAndEnv(t, process, env, sourcePath)
	if err != nil {
		t.Fatalf("LoadedFactoryWithProcess: %v", err)
	}
	factory.Name = factoryapi.FactoryName(name)
	id := name
	factory.Id = &id
	payload, err := json.Marshal(factory)
	if err != nil {
		t.Fatalf("marshal named filewatcher factory payload: %v", err)
	}
	return payload
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

func waitForActivatedFactoryRuntime(
	t *testing.T,
	serverURL string,
	wantPlaceID string,
	timeout time.Duration,
) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	var lastFactoryState string
	var lastRuntimeStatus interfaces.RuntimeStatus
	var sawPlace bool
	for time.Now().Before(deadline) {
		status := support.GetJSON[factoryapi.StatusResponse](t, serverURL+"/status")
		factory := getCurrentFactory(t, serverURL)
		lastFactoryState = status.FactoryState
		lastRuntimeStatus = interfaces.RuntimeStatus(status.RuntimeStatus)
		sawPlace = factoryHasCustomerPlace(factory, wantPlaceID)
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

func waitForSubmittedWorkAtPlace(
	t *testing.T,
	serverURL string,
	traceID string,
	placeID string,
	timeout time.Duration,
) factoryapi.ListWorkResponse {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		listed := support.ListDefaultSessionWork(t, serverURL)
		for _, item := range listed.Results {
			if workTraceID(item) == traceID && workCustomerPlaceID(item) == placeID {
				return listed
			}
		}
		time.Sleep(100 * time.Millisecond)
	}

	return support.ListDefaultSessionWork(t, serverURL)
}

func waitForProviderCommandWorkSettlement(
	t *testing.T,
	serverURL string,
	runner *support.ShapedProviderCommandRunner,
	wantCalls int,
	timeout time.Duration,
) {
	t.Helper()

	support.WaitForStatus(t, serverURL, timeout, func(status factoryapi.StatusResponse) bool {
		if status.RuntimeStatus != string(interfaces.RuntimeStatusIdle) {
			return false
		}
		if runner.CallCount() != wantCalls {
			return false
		}
		listed := support.ListDefaultSessionWork(t, serverURL)
		return len(listed.Results) == wantCalls &&
			support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("task", "complete")) == wantCalls &&
			support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("task", "init")) == 0 &&
			support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("task", "processing")) == 0
	})
}

func assertNoAdditionalProviderCommandWork(
	t *testing.T,
	serverURL string,
	runner *support.ShapedProviderCommandRunner,
	timeout time.Duration,
) {
	t.Helper()

	const stableWindow = 300 * time.Millisecond
	deadline := time.Now().Add(timeout)
	var stableSince time.Time

	for time.Now().Before(deadline) {
		if runner.CallCount() > 1 {
			t.Fatalf(
				"deactivated factory watch path triggered additional work: provider command calls = %d, want 1",
				runner.CallCount(),
			)
		}

		listed := support.ListDefaultSessionWork(t, serverURL)
		status := support.GetJSON[factoryapi.StatusResponse](t, serverURL+"/status")
		stable := runner.CallCount() == 1 &&
			len(listed.Results) == 1 &&
			support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("task", "complete")) == 1 &&
			support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("task", "init")) == 0 &&
			support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("task", "processing")) == 0 &&
			status.RuntimeStatus == string(interfaces.RuntimeStatusIdle)

		if stable {
			if stableSince.IsZero() {
				stableSince = time.Now()
			} else if time.Since(stableSince) >= stableWindow {
				return
			}
		} else {
			stableSince = time.Time{}
		}
		time.Sleep(25 * time.Millisecond)
	}

	listed := support.ListDefaultSessionWork(t, serverURL)
	t.Fatalf(
		"timed out waiting for stable no-additional-work observation: provider_calls=%d listed=%#v",
		runner.CallCount(),
		listed.Results,
	)
}

func factoryHasCustomerPlace(factory factoryapi.Factory, placeID string) bool {
	if factory.WorkTypes == nil {
		return false
	}
	workType, state, ok := strings.Cut(placeID, ":")
	if !ok {
		return false
	}
	for _, candidate := range *factory.WorkTypes {
		if candidate.Name != workType {
			continue
		}
		for _, candidateState := range candidate.States {
			if candidateState.Name == state {
				return true
			}
		}
	}
	return false
}

func workCustomerPlaceID(work factoryapi.Work) string {
	if work.State == nil {
		return workTraceID(work) + ":"
	}
	workType := ""
	if work.WorkTypeName != nil {
		workType = *work.WorkTypeName
	}
	return workType + ":" + work.State.Name
}

func workTraceID(work factoryapi.Work) string {
	if work.TraceId == nil {
		return ""
	}
	return *work.TraceId
}

func assertCurrentFactoryNameAndDirectory(t *testing.T, serverURL, wantName, wantDir string) {
	t.Helper()

	current := getCurrentFactory(t, serverURL)
	if current.Name != factoryapi.FactoryName(wantName) {
		t.Fatalf("current factory name = %q, want %q", current.Name, wantName)
	}
	if current.FactoryDirectory == nil || *current.FactoryDirectory != wantDir {
		t.Fatalf("current factory directory = %#v, want %q", current.FactoryDirectory, wantDir)
	}
}

func activateNamedPersistedFactoryOverHTTP(t *testing.T, serverURL string, payload []byte) {
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
		serverURL+"/factory-sessions/~default/factory",
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
