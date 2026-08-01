package current

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/transports/http/apitypes"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type currentFactoryServerSetup func(testing.TB, support.Process, root.Input)

func currentFactorySetup(
	t *testing.T,
	action func(support.Process, []string),
) currentFactoryServerSetup {
	t.Helper()
	return func(_ testing.TB, process support.Process, inputs root.Input) {
		action(process, append([]string(nil), inputs.Env...))
	}
}

func startCurrentFactoryServerWithSetup(
	t *testing.T,
	rootDir string,
	setup currentFactoryServerSetup,
) *support.FunctionalAPIServer {
	t.Helper()
	return support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                rootDir,
		UseMockWorkers:            true,
		WaitForServiceModeRuntime: true,
		BeforeStart:               setup,
	})
}

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
		Edges: serviceedges.Edges{
			ProviderCommandRunner: runner,
		},
		BeforeStart: setup,
	})
}

func seedNamedFactoryRootWithProcess(
	t *testing.T,
	process support.Process,
	env []string,
	rootDir, name, workType string,
) string {
	t.Helper()

	sourceDir := t.TempDir()
	sourcePath := filepath.Join(sourceDir, interfaces.FactoryConfigFile)
	if err := os.WriteFile(sourcePath, functionalNamedFactoryPayloadWithWorkType(t, name, workType), 0o600); err != nil {
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

func createNamedFactoryFixtureWithProcess(
	t *testing.T,
	process support.Process,
	env []string,
	rootDir, name string,
	payload []byte,
) string {
	t.Helper()

	sourceDir := t.TempDir()
	sourcePath := filepath.Join(sourceDir, interfaces.FactoryConfigFile)
	if err := os.WriteFile(sourcePath, payload, 0o600); err != nil {
		t.Fatalf("write customer Factory source %s: %v", name, err)
	}
	return support.CreateNamedFactoryAtRootWithProcess(
		t,
		process,
		env,
		sourceDir,
		rootDir,
		name,
		sourcePath,
	)
}

func openNamedFactorySession(t *testing.T, serverURL, folderPath, name string) string {
	t.Helper()
	body, err := json.Marshal(factoryapi.OpenFactorySessionRequest{
		FolderPath: folderPath,
		Target: &factoryapi.FactorySessionTargetRef{
			Kind: factoryapi.FactorySessionTargetRefKindNamed,
			Name: &name,
		},
	})
	if err != nil {
		t.Fatalf("marshal open factory session request: %v", err)
	}
	resp, err := http.Post(serverURL+"/factory-sessions", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /factory-sessions: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("POST /factory-sessions status = %d, want 200", resp.StatusCode)
	}
	var opened factoryapi.OpenFactorySessionResponse
	decodeJSONResponse(t, resp, &opened, "decode open factory session response")
	if opened.Session == nil || opened.Session.Id == "" {
		t.Fatalf("open factory session response = %#v, want session id", opened)
	}
	return opened.Session.Id
}

func getCurrentFactoryForSession(t *testing.T, serverURL, sessionID string) factoryapi.Factory {
	t.Helper()
	resp, err := http.Get(sessionFactoryURL(serverURL, sessionID))
	if err != nil {
		t.Fatalf("GET /factory-sessions/%s/factory: %v", sessionID, err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("GET /factory-sessions/%s/factory status = %d, want 200", sessionID, resp.StatusCode)
	}
	var current factoryapi.Factory
	decodeJSONResponse(t, resp, &current, "decode session current factory response")
	return current
}

func saveCurrentFactoryForSession(t *testing.T, serverURL, sessionID, body string) factoryapi.Factory {
	t.Helper()
	resp := putFactoryForSessionRequestExpectStatusWithClient(
		t,
		http.DefaultClient,
		serverURL,
		"/factory-sessions/"+sessionID+"/factory",
		saveFactoryForSessionRequestBody(body),
		http.StatusOK,
	)
	var saved factoryapi.Factory
	decodeJSONResponse(t, resp, &saved, "decode session current factory save response")
	return saved
}

func sessionFactoryURL(serverURL, sessionID string) string {
	return serverURL + "/factory-sessions/" + url.PathEscape(sessionID) + "/factory"
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

func getCurrentFactory(t *testing.T, serverURL string) factoryapi.Factory {
	t.Helper()
	resp, err := http.Get(serverURL + "/factory-sessions/~default/factory")
	if err != nil {
		t.Fatalf("GET /factory-sessions/~default/factory: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("GET /factory-sessions/~default/factory status = %d, want 200", resp.StatusCode)
	}
	var current factoryapi.Factory
	decodeJSONResponse(t, resp, &current, "decode current factory response")
	return current
}

func saveCurrentFactoryDefinition(t *testing.T, serverURL, body string) factoryapi.Factory {
	t.Helper()

	resp := saveCurrentFactoryDefinitionExpectStatus(t, serverURL, body, http.StatusOK)
	var saved factoryapi.Factory
	decodeJSONResponse(t, resp, &saved, "decode current factory save response")
	return saved
}

func saveCurrentFactoryDefinitionExpectStatus(t *testing.T, serverURL, body string, wantStatus int) *http.Response {
	t.Helper()
	return putFactoryForSessionRequestExpectStatusWithClient(
		t,
		http.DefaultClient,
		serverURL,
		"/factory-sessions/~default/factory",
		saveFactoryForSessionRequestBody(body),
		wantStatus,
	)
}

func saveFactoryForSessionRequestBody(factoryJSON string) string {
	return fmt.Sprintf(`{"factory":%s}`, factoryJSON)
}

func putFactoryForSessionRequestExpectStatusWithClient(
	t *testing.T,
	client *http.Client,
	serverURL,
	path string,
	body string,
	wantStatus int,
) *http.Response {
	t.Helper()

	req, err := http.NewRequest(http.MethodPut, serverURL+path, bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("new factory session save request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("PUT %s: %v", path, err)
	}
	if resp.StatusCode != wantStatus {
		payload, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("PUT %s status = %d, want %d: %s", path, resp.StatusCode, wantStatus, payload)
	}
	return resp
}

func decodeJSONResponse(t *testing.T, resp *http.Response, target any, message string) {
	t.Helper()
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		t.Fatalf("%s: %v", message, err)
	}
}

func assertFactoryWorkType(t *testing.T, factory factoryapi.Factory, want string, contextLabel string) {
	t.Helper()
	if factory.WorkTypes == nil || len(*factory.WorkTypes) != 1 || (*factory.WorkTypes)[0].Name != want {
		t.Fatalf("%s work types = %#v, want %s", contextLabel, factory.WorkTypes, want)
	}
}

func functionalNamedFactoryPayloadWithWorkType(t *testing.T, name, workType string) []byte {
	t.Helper()
	return []byte(functionalNamedFactoryPayloadJSON(name, workType))
}

func functionalNamedFactoryBody(name, workType string, version ...factoryapi.HybridLogicalTimestamp) string {
	if len(version) == 0 {
		return functionalNamedFactoryPayloadJSON(name, workType)
	}
	return currentFactorySaveDocument(nil, name, workType, versionDocument(version[0]))
}

func functionalNamedFactoryPayloadJSON(name, workType string) string {
	return `{
		"name":"` + name + `",
		"id":"` + name + `",
		"workTypes":[{"name":"` + workType + `","states":[{"name":"init","type":"INITIAL"},{"name":"done","type":"TERMINAL"},{"name":"failed","type":"FAILED"}]}],
		"workers":[{"name":"planner","type":"MODEL_WORKER","modelProvider":"CLAUDE","executorProvider":"SCRIPT_WRAP","model":"claude-sonnet-4-20250514"}],
		"workstations":[{"name":"plan-task","behavior":"STANDARD","type":"MODEL_WORKSTATION","worker":"planner","inputs":[{"workType":"` + workType + `","state":"init"}],"outputs":[{"workType":"` + workType + `","state":"done"}]}]
	}`
}

func currentFactorySaveDocument(t *testing.T, name, workType string, version any) string {
	if t != nil {
		t.Helper()
	}
	document := map[string]any{
		"name": name,
		"id":   name,
		"workTypes": []map[string]any{{
			"name": workType,
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "done", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]string{{
			"name":             "planner",
			"type":             "MODEL_WORKER",
			"modelProvider":    "CLAUDE",
			"executorProvider": "SCRIPT_WRAP",
			"model":            "claude-sonnet-4-20250514",
		}},
		"workstations": []map[string]any{{
			"name":     "plan-task",
			"behavior": "STANDARD",
			"type":     "MODEL_WORKSTATION",
			"worker":   "planner",
			"inputs":   []map[string]string{{"workType": workType, "state": "init"}},
			"outputs":  []map[string]string{{"workType": workType, "state": "done"}},
		}},
	}
	if version != nil {
		document["version"] = version
	}
	body, err := json.Marshal(document)
	if err != nil {
		if t != nil {
			t.Fatalf("marshal current factory save document: %v", err)
		}
		panic(err)
	}
	return string(body)
}

func advancedFactoryVersion(t *testing.T, version *factoryapi.HybridLogicalTimestamp) factoryapi.HybridLogicalTimestamp {
	t.Helper()
	if version == nil {
		t.Fatal("factory version = nil, want version metadata")
	}
	return factoryapi.HybridLogicalTimestamp{
		Logical:  version.Logical + 1,
		Physical: version.Physical.UTC().Add(time.Nanosecond),
	}
}

func versionDocument(version factoryapi.HybridLogicalTimestamp) map[string]string {
	return map[string]string{
		"logical":  strconv.FormatInt(version.Logical.Int64(), 10),
		"physical": version.Physical.UTC().Format(time.RFC3339Nano),
	}
}

const factoryValidationCodeDanglingPlaceReference = "factory.route.danglingPlaceReference"

func hasValidationTargetCode(targets []factoryapi.FactoryValidationTarget, code string) bool {
	for _, target := range targets {
		if target.Code == code {
			return true
		}
	}
	return false
}

const defaultFunctionalWorkstationName = "plan-task"

func promptTemplateContractURL(serverURL, sessionID, workstationName string) string {
	return serverURL + "/factory-sessions/" + url.PathEscape(sessionID) + "/factory/workstations/" + url.PathEscape(workstationName) + "/prompt-template-contract"
}

func promptTemplateValidationURL(serverURL, sessionID, workstationName string) string {
	return serverURL + "/factory-sessions/" + url.PathEscape(sessionID) + "/factory/workstations/" + url.PathEscape(workstationName) + "/prompt-template-validation"
}

func getPromptTemplateContract(t *testing.T, serverURL, sessionID, workstationName string) factoryapi.PromptTemplateContract {
	t.Helper()
	resp, err := http.Get(promptTemplateContractURL(serverURL, sessionID, workstationName))
	if err != nil {
		t.Fatalf("GET prompt-template-contract: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("GET prompt-template-contract status = %d, want 200: %s", resp.StatusCode, payload)
	}
	var contract factoryapi.PromptTemplateContract
	decodeJSONResponse(t, resp, &contract, "decode prompt-template contract response")
	return contract
}

func validatePromptTemplateForSession(
	t *testing.T,
	serverURL,
	sessionID,
	workstationName,
	prompt string,
) factoryapi.PromptTemplateValidationResult {
	t.Helper()
	body, err := json.Marshal(factoryapi.PromptTemplateValidationRequest{Prompt: prompt})
	if err != nil {
		t.Fatalf("marshal prompt-template validation request: %v", err)
	}
	resp, err := http.Post(
		promptTemplateValidationURL(serverURL, sessionID, workstationName),
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("POST prompt-template-validation: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("POST prompt-template-validation status = %d, want 200: %s", resp.StatusCode, payload)
	}
	var result factoryapi.PromptTemplateValidationResult
	decodeJSONResponse(t, resp, &result, "decode prompt-template validation response")
	return result
}

func promptTemplateContractHasVariablePath(contract factoryapi.PromptTemplateContract, want string) bool {
	for _, reference := range contract.AvailableVariables {
		if reference.Path == want {
			return true
		}
	}
	return false
}

func promptTemplateValidationHasDiagnosticKind(
	result factoryapi.PromptTemplateValidationResult,
	want factoryapi.PromptTemplateDiagnosticKind,
) bool {
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Kind == want {
			return true
		}
	}
	return false
}

func assertCurrentFactoryWorkCustomerStates(
	t *testing.T,
	listed factoryapi.ListWorkResponse,
	wants map[string]int,
) {
	t.Helper()
	for location, want := range wants {
		if got := support.CountWorkAtCustomerState(listed, location); got != want {
			t.Errorf("%s work count = %d, want %d", location, got, want)
		}
	}
}

func assertCompletedWorkName(
	t *testing.T,
	listed factoryapi.ListWorkResponse,
	workType,
	wantName string,
) {
	t.Helper()
	for _, item := range listed.Results {
		if item.WorkTypeName != nil && *item.WorkTypeName == workType &&
			item.State != nil && item.State.Name == "complete" {
			if item.Name != wantName {
				t.Errorf("%s:complete name = %q, want %q", workType, item.Name, wantName)
			}
			return
		}
	}
	t.Errorf("listed Work missing %s:complete", workType)
}

func firstDispatchInputToken(rawTokens any) workerexecution.Token {
	switch tokens := rawTokens.(type) {
	case []any:
		if len(tokens) == 0 {
			return workerexecution.Token{}
		}
		tok, ok := tokens[0].(workerexecution.Token)
		if !ok {
			return workerexecution.Token{}
		}
		return tok
	case []workerexecution.Token:
		if len(tokens) == 0 {
			return workerexecution.Token{}
		}
		return tokens[0]
	default:
		return workerexecution.Token{}
	}
}
