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
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/transports/http/apitypes"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func startCurrentFactoryServer(t *testing.T, rootDir string) *support.FunctionalAPIServer {
	t.Helper()
	runner := support.NewRecordingCommandRunner("runtime must not execute during current-factory read/save")
	return startCurrentFactoryServerWithProviderRunner(t, rootDir, runner)
}

func startCurrentFactoryServerWithProviderRunner(
	t *testing.T,
	rootDir string,
	runner platformprocess.CommandRunner,
) *support.FunctionalAPIServer {
	t.Helper()
	return support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                rootDir,
		WaitForServiceModeRuntime: true,
		Edges: serviceedges.Edges{
			ProviderCommandRunner: runner,
		},
	})
}

func seedNamedFactoryRootWithTerminalState(t *testing.T, rootDir, name, terminalState string) string {
	t.Helper()

	sourceDir := t.TempDir()
	sourcePath := filepath.Join(sourceDir, interfaces.FactoryConfigFile)
	payload := functionalNamedFactoryPayloadWithTerminalState(t, name, terminalState)
	if err := os.WriteFile(sourcePath, payload, 0o600); err != nil {
		t.Fatalf("write customer Factory source %s: %v", name, err)
	}
	return support.CreateAndActivateNamedFactoryAtRoot(t, sourceDir, rootDir, name, sourcePath)
}

func seedFilewatcherNamedFactoryRoot(t *testing.T, rootDir, name string, activate bool) string {
	t.Helper()

	srcDir := support.LegacyFixtureDir(t, "filewatcher_flow")
	sourcePath := filepath.Join(srcDir, interfaces.FactoryConfigFile)
	if activate {
		return support.CreateAndActivateNamedFactoryAtRoot(t, srcDir, rootDir, name, sourcePath)
	}
	return support.CreateNamedFactoryAtRoot(t, srcDir, rootDir, name, sourcePath)
}

func namedFilewatcherFactoryPayload(t *testing.T, name string) []byte {
	t.Helper()

	sourcePath := filepath.Join(support.LegacyFixtureDir(t, "filewatcher_flow"), interfaces.FactoryConfigFile)
	factory, err := support.LoadedFactory(t, sourcePath)
	if err != nil {
		t.Fatalf("LoadedFactory: %v", err)
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

func seedNamedFactoryRoot(t *testing.T, rootDir, name, workType string) string {
	t.Helper()

	sourceDir := t.TempDir()
	sourcePath := filepath.Join(sourceDir, interfaces.FactoryConfigFile)
	if err := os.WriteFile(sourcePath, functionalNamedFactoryPayloadWithWorkType(t, name, workType), 0o600); err != nil {
		t.Fatalf("write customer Factory source %s: %v", name, err)
	}
	return support.CreateAndActivateNamedFactoryAtRoot(t, sourceDir, rootDir, name, sourcePath)
}

func createNamedFactoryFixture(t *testing.T, rootDir, name string, payload []byte) string {
	t.Helper()

	sourceDir := t.TempDir()
	sourcePath := filepath.Join(sourceDir, interfaces.FactoryConfigFile)
	if err := os.WriteFile(sourcePath, payload, 0o600); err != nil {
		t.Fatalf("write customer Factory source %s: %v", name, err)
	}
	return support.CreateNamedFactoryAtRoot(t, sourceDir, rootDir, name, sourcePath)
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

const (
	factoryValidationCodeDanglingPlaceReference = "factory.route.danglingPlaceReference"
	factoryValidationCodePayloadInvalid         = "factory.payload.invalid"
)

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

func assertProviderRunnerIdle(t *testing.T, runner *support.RecordingCommandRunner) {
	t.Helper()
	if runner.CallCount() != 0 {
		t.Fatalf("provider command runner calls = %d, want 0", runner.CallCount())
	}
}

func requireInitialStructurePayload(t *testing.T, events []factoryapi.FactoryEvent) factoryapi.InitialStructureRequestEventPayload {
	t.Helper()
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeInitialStructureRequest {
			continue
		}
		payload, err := event.Payload.AsInitialStructureRequestEventPayload()
		if err != nil {
			t.Fatalf("decode initial-structure payload: %v", err)
		}
		return payload
	}
	t.Fatalf("initial-structure event not found in %d events", len(events))
	return factoryapi.InitialStructureRequestEventPayload{}
}

func requireFactoryChangeAfter(t *testing.T, before []factoryapi.FactoryEvent, after []factoryapi.FactoryEvent) factoryapi.FactoryEvent {
	t.Helper()
	minSequence := -1
	for _, event := range before {
		if event.Context.Sequence > minSequence {
			minSequence = event.Context.Sequence
		}
	}
	for _, event := range after {
		if event.Context.Sequence > minSequence && event.Type == factoryapi.FactoryEventTypeFactoryChange {
			return event
		}
	}
	t.Fatalf("factory-change event not found after save; before=%d after=%d", len(before), len(after))
	return factoryapi.FactoryEvent{}
}

func requireLatestFactoryChange(t *testing.T, events []factoryapi.FactoryEvent) factoryapi.FactoryEvent {
	t.Helper()
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Type == factoryapi.FactoryEventTypeFactoryChange {
			return events[i]
		}
	}
	t.Fatalf("factory-change event not found; events=%d", len(events))
	return factoryapi.FactoryEvent{}
}

func assertFactoryChangeVersion(t *testing.T, eventFactory factoryapi.Factory, saved factoryapi.Factory) {
	t.Helper()
	if saved.Version == nil {
		t.Fatal("saved factory version = nil, want version metadata")
	}
	if eventFactory.Version == nil {
		t.Fatal("factory-change payload version = nil, want saved version metadata")
	}
	if eventFactory.Version.Logical != saved.Version.Logical || !eventFactory.Version.Physical.Equal(saved.Version.Physical) {
		t.Fatalf("factory-change payload version = %#v, want saved version %#v", eventFactory.Version, saved.Version)
	}
}

func docBundledFileEntry(targetPath, inline string) map[string]any {
	return map[string]any{
		"id":         targetPath,
		"type":       "DOC",
		"targetPath": targetPath,
		"content": map[string]string{
			"encoding": "utf-8",
			"inline":   inline,
		},
	}
}

func scriptBundledFileEntry(targetPath, inline string) map[string]any {
	return map[string]any{
		"id":         targetPath,
		"type":       "SCRIPT",
		"targetPath": targetPath,
		"content": map[string]string{
			"encoding": "utf-8",
			"inline":   inline,
		},
	}
}

func currentFactoryDocumentWithBundledDocs(t *testing.T, current factoryapi.Factory, bundledFiles []map[string]any) string {
	t.Helper()

	body, err := json.Marshal(current)
	if err != nil {
		t.Fatalf("marshal current factory document: %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(body, &document); err != nil {
		t.Fatalf("decode current factory document: %v", err)
	}
	document["version"] = versionDocument(advancedFactoryVersion(t, current.Version))
	document["supportingFiles"] = map[string]any{
		"bundledFiles": bundledFiles,
	}
	body, err = json.Marshal(document)
	if err != nil {
		t.Fatalf("marshal current factory document with bundled docs: %v", err)
	}
	return string(body)
}

func currentFactoryEventDocumentWithBundledFiles(
	t *testing.T,
	name string,
	workType string,
	bundledFiles []map[string]any,
) []byte {
	t.Helper()

	var document map[string]any
	if err := json.Unmarshal([]byte(functionalNamedFactoryPayloadJSON(name, workType)), &document); err != nil {
		t.Fatalf("decode factory event bundled-file document: %v", err)
	}
	document["supportingFiles"] = map[string]any{
		"bundledFiles": bundledFiles,
	}
	body, err := json.Marshal(document)
	if err != nil {
		t.Fatalf("marshal factory event bundled-file document: %v", err)
	}
	return body
}

func findBundledFile(t *testing.T, factory factoryapi.Factory, targetPath string) *factoryapi.BundledFile {
	t.Helper()
	if factory.SupportingFiles == nil || factory.SupportingFiles.BundledFiles == nil {
		return nil
	}
	for _, bundledFile := range *factory.SupportingFiles.BundledFiles {
		if bundledFile.TargetPath == targetPath {
			copied := bundledFile
			return &copied
		}
	}
	return nil
}

func assertDocBundledFileInline(t *testing.T, factory factoryapi.Factory, targetPath, wantInline string) {
	t.Helper()
	bundledFile := findBundledFile(t, factory, targetPath)
	if bundledFile == nil {
		t.Fatalf("doc bundled file %q missing from %#v", targetPath, factory.SupportingFiles)
	}
	if bundledFile.Type != factoryapi.BundledFileTypeDOC {
		t.Fatalf("bundled file %q type = %q, want DOC", targetPath, bundledFile.Type)
	}
	if bundledFile.Id == nil || *bundledFile.Id != targetPath {
		t.Fatalf("doc bundled file %q id = %#v, want %q", targetPath, bundledFile.Id, targetPath)
	}
	if bundledFile.Content.Inline != wantInline {
		t.Fatalf("doc bundled file %q inline = %q, want %q", targetPath, bundledFile.Content.Inline, wantInline)
	}
}

func assertScriptBundledFileInline(t *testing.T, factory factoryapi.Factory, targetPath, wantInline string) {
	t.Helper()
	bundledFile := findBundledFile(t, factory, targetPath)
	if bundledFile == nil {
		t.Fatalf("script bundled file %q missing from %#v", targetPath, factory.SupportingFiles)
	}
	if bundledFile.Type != factoryapi.BundledFileTypeSCRIPT {
		t.Fatalf("bundled file %q type = %q, want SCRIPT", targetPath, bundledFile.Type)
	}
	if bundledFile.Id == nil || *bundledFile.Id != targetPath {
		t.Fatalf("script bundled file %q id = %#v, want %q", targetPath, bundledFile.Id, targetPath)
	}
	if bundledFile.Content.Inline != wantInline {
		t.Fatalf("script bundled file %q inline = %q, want %q", targetPath, bundledFile.Content.Inline, wantInline)
	}
}

func assertPortableFile(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	if string(data) != want {
		t.Fatalf("file %s = %q, want %q", path, string(data), want)
	}
}

func assertPersistedFactoryJSONStripsInlineBundledContent(t *testing.T, path string, forbidden ...string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	payload := string(data)
	for _, snippet := range forbidden {
		if bytes.Contains(data, []byte(snippet)) {
			t.Fatalf("persisted factory payload %s still contains inline portable content %q: %s", path, snippet, payload)
		}
	}
}

func functionalDefaultFactorySaveBody(id, workType string, version factoryapi.HybridLogicalTimestamp) string {
	return `{
		"name":"UNDEFINED",
		"id":"` + id + `",
		"version":{"physical":"` + version.Physical.UTC().Format(time.RFC3339Nano) + `","logical":"` + strconv.FormatInt(version.Logical.Int64(), 10) + `"},
		"workTypes":[{"name":"` + workType + `","states":[{"name":"init","type":"INITIAL"},{"name":"done","type":"TERMINAL"},{"name":"failed","type":"FAILED"}]}],
		"workers":[{"name":"planner","type":"MODEL_WORKER","modelProvider":"CLAUDE","executorProvider":"SCRIPT_WRAP","model":"claude-sonnet-4-20250514","body":"You are the planner."}],
		"workstations":[{"name":"plan-task","behavior":"STANDARD","type":"MODEL_WORKSTATION","worker":"planner","body":"Plan the work.","inputs":[{"workType":"` + workType + `","state":"init"}],"outputs":[{"workType":"` + workType + `","state":"done"}]}]
	}`
}

func assertRawCurrentFactoryLogicalVersionIsString(t *testing.T, serverURL string) {
	t.Helper()
	resp, err := http.Get(serverURL + "/factory-sessions/~default/factory")
	if err != nil {
		t.Fatalf("GET /factory-sessions/~default/factory: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("GET /factory-sessions/~default/factory status = %d, want 200", resp.StatusCode)
	}
	var document map[string]any
	decodeJSONResponse(t, resp, &document, "decode raw current factory response")
	version, ok := document["version"].(map[string]any)
	if !ok {
		t.Fatalf("raw current factory version = %#v, want object", document["version"])
	}
	if _, ok := version["logical"].(string); !ok {
		t.Fatalf("raw current factory version.logical = %#v, want string", version["logical"])
	}
}

func assertFunctionalSplitLayoutAtRoot(t *testing.T, rootDir, project string) {
	t.Helper()

	factoryJSON, err := os.ReadFile(filepath.Join(rootDir, interfaces.FactoryConfigFile))
	if err != nil {
		t.Fatalf("ReadFile(factory.json): %v", err)
	}
	if strings.Contains(string(factoryJSON), "You are the planner.") {
		t.Fatalf("factory.json should omit inlined planner body after split save, got %s", factoryJSON)
	}

	agentsPath := filepath.Join(rootDir, interfaces.WorkersDir, "planner", interfaces.FactoryAgentsFileName)
	if _, err := os.Stat(agentsPath); err != nil {
		t.Fatalf("expected planner AGENTS.md at %s: %v", agentsPath, err)
	}

	workstationPath := filepath.Join(rootDir, interfaces.WorkstationsDir, "plan-task", interfaces.FactoryAgentsFileName)
	if _, err := os.Stat(workstationPath); err != nil {
		t.Fatalf("expected plan-task AGENTS.md at %s: %v", workstationPath, err)
	}

	loaded, err := support.LoadedCurrentFactory(t, rootDir)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig: %v", err)
	}
	if loaded.Id == nil || *loaded.Id != project {
		t.Fatalf("factory id = %v, want %q", loaded.Id, project)
	}
}

func saveCurrentFactoryDefinitionWithClient(t *testing.T, client *http.Client, serverURL, body string) factoryapi.Factory {
	t.Helper()

	resp := saveCurrentFactoryDefinitionExpectStatusWithClient(t, client, serverURL, body, http.StatusOK)
	var saved factoryapi.Factory
	decodeJSONResponse(t, resp, &saved, "decode current factory save response")
	return saved
}

func saveCurrentFactoryDefinitionExpectStatusWithClient(
	t *testing.T,
	client *http.Client,
	serverURL,
	body string,
	wantStatus int,
) *http.Response {
	t.Helper()

	return putFactoryForSessionRequestExpectStatusWithClient(
		t,
		client,
		serverURL,
		"/factory-sessions/~default/factory",
		saveFactoryForSessionRequestBody(body),
		wantStatus,
	)
}

func submitWorkAndExpectStatus(t *testing.T, serverURL, workType, title string, wantStatus int) *http.Response {
	t.Helper()
	resp, err := http.Post(
		support.DefaultSessionWorkURL(serverURL, "/work"),
		"application/json",
		bytes.NewBufferString(`{"name":"current-factory-read-save-submit","workTypeName":"`+workType+`","payload":{"title":"`+title+`"}}`),
	)
	if err != nil {
		t.Fatalf("POST /work %s: %v", workType, err)
	}
	if resp.StatusCode != wantStatus {
		resp.Body.Close()
		t.Fatalf("POST /work %s status = %d, want %d", workType, resp.StatusCode, wantStatus)
	}
	return resp
}

func submitWorkForSessionAndExpectStatus(t *testing.T, serverURL, sessionID, workType, title string, wantStatus int) *http.Response {
	t.Helper()
	endpoint := serverURL + "/factory-sessions/" + url.PathEscape(sessionID) + "/work"
	resp, err := http.Post(
		endpoint,
		"application/json",
		bytes.NewBufferString(`{"name":"current-factory-session-read-save-submit","workTypeName":"`+workType+`","payload":{"title":"`+title+`"}}`),
	)
	if err != nil {
		t.Fatalf("POST /factory-sessions/%s/work %s: %v", sessionID, workType, err)
	}
	if resp.StatusCode != wantStatus {
		resp.Body.Close()
		t.Fatalf("POST /factory-sessions/%s/work %s status = %d, want %d", sessionID, workType, resp.StatusCode, wantStatus)
	}
	return resp
}

func upsertNamedFactoryFromBody(t *testing.T, serverURL, factoryBody string) factoryapi.Factory {
	t.Helper()
	requestBody := upsertNamedFactoryRequestBody(factoryBody)
	resp := putFactoryForSessionRequestExpectStatusWithClient(
		t,
		http.DefaultClient,
		serverURL,
		"/factory-sessions/~default/factory",
		requestBody,
		http.StatusOK,
	)
	var created factoryapi.Factory
	decodeJSONResponse(t, resp, &created, "decode upsert named factory response")
	return created
}

func upsertNamedFactoryRequestBody(factoryJSON string) string {
	return fmt.Sprintf(`{"mode":"UPSERT_NAMED_AND_ACTIVATE","factory":%s}`, factoryJSON)
}

func nonDefaultSessionImportBodyWithBundledFiles(
	t *testing.T,
	sessionCurrent factoryapi.Factory,
	workType string,
) string {
	t.Helper()
	if sessionCurrent.Version == nil {
		t.Fatal("session current factory version = nil, want version metadata for import")
	}

	body, err := json.Marshal(sessionCurrent)
	if err != nil {
		t.Fatalf("marshal session current factory document: %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(body, &document); err != nil {
		t.Fatalf("decode session current factory document: %v", err)
	}

	document["version"] = versionDocument(advancedFactoryVersion(t, sessionCurrent.Version))
	document["workTypes"] = []map[string]any{{
		"name": workType,
		"states": []map[string]string{
			{"name": "init", "type": "INITIAL"},
			{"name": "done", "type": "TERMINAL"},
			{"name": "failed", "type": "FAILED"},
		},
	}}
	document["workers"] = []map[string]any{{
		"body":             "Plan imported work.",
		"executorProvider": "SCRIPT_WRAP",
		"model":            "claude-sonnet-4-20250514",
		"modelProvider":    "CLAUDE",
		"name":             "planner",
		"type":             "MODEL_WORKER",
	}}
	document["workstations"] = []map[string]any{{
		"behavior": "STANDARD",
		"body":     "Plan the imported work.",
		"inputs":   []map[string]string{{"state": "init", "workType": workType}},
		"name":     "plan-task",
		"outputs":  []map[string]string{{"state": "done", "workType": workType}},
		"type":     "MODEL_WORKSTATION",
		"worker":   "planner",
	}}
	document["supportingFiles"] = map[string]any{
		"bundledFiles": []map[string]any{
			{
				"type":       "ROOT_HELPER",
				"targetPath": "Makefile",
				"content": map[string]string{
					"encoding": "utf-8",
					"inline":   "test:\n\tgo test ./...\n",
				},
			},
			{
				"type":       "DOC",
				"targetPath": "factory/docs/README.md",
				"content": map[string]string{
					"encoding": "utf-8",
					"inline":   "# Session import factory\n",
				},
			},
			{
				"type":       "SCRIPT",
				"targetPath": "factory/scripts/session-import.py",
				"content": map[string]string{
					"encoding": "utf-8",
					"inline":   "print('session import script')\n",
				},
			},
		},
	}

	body, err = json.Marshal(document)
	if err != nil {
		t.Fatalf("marshal non-default session import document: %v", err)
	}
	return string(body)
}

type advancedSaveVersionCase struct {
	name      string
	version   func(t *testing.T, current factoryapi.HybridLogicalTimestamp) any
	wantCode  factoryapi.ErrorResponseCode
	wantState string
}

func runAdvancedSaveVersionCase(t *testing.T, tc advancedSaveVersionCase) {
	t.Helper()
	rootDir := t.TempDir()
	seedNamedFactoryRoot(t, rootDir, "alpha", "task")
	server := startCurrentFactoryServer(t, rootDir)
	defer server.Stop(t)
	current := getCurrentFactory(t, server.URL())
	if current.Version == nil {
		t.Fatal("current factory version = nil, want version metadata")
	}

	body := currentFactorySaveDocument(t, "alpha", "story", tc.version(t, *current.Version))
	if tc.wantCode != "" {
		resp := saveCurrentFactoryDefinitionExpectStatus(t, server.URL(), body, http.StatusConflict)
		var errResp factoryapi.ErrorResponse
		decodeJSONResponse(t, resp, &errResp, "decode stale current factory save response")
		if errResp.Code != tc.wantCode {
			t.Fatalf("error code = %q, want %q", errResp.Code, tc.wantCode)
		}
	} else {
		saved := saveCurrentFactoryDefinition(t, server.URL(), body)
		assertFactoryWorkType(t, saved, "story", "saved current factory")
	}

	reloaded := getCurrentFactory(t, server.URL())
	assertFactoryWorkType(t, reloaded, tc.wantState, "reloaded current factory after version save")
}

func advancedSaveVersionCases() []advancedSaveVersionCase {
	return []advancedSaveVersionCase{
		staleVersionCase("lower logical lower physical fails", -1, -time.Nanosecond),
		staleVersionCase("same logical equal physical fails", 0, 0),
		staleVersionCase("lower logical greater physical fails", -1, time.Second),
		staleVersionCase("same logical greater physical fails", 0, time.Second),
		missingVersionCase(),
		missingLogicalVersionCase(),
		missingPhysicalVersionCase(),
		advancedVersionPassCase(),
	}
}

func staleVersionCase(name string, logicalDelta int64, physicalDelta time.Duration) advancedSaveVersionCase {
	return advancedSaveVersionCase{
		name: name,
		version: func(t *testing.T, current factoryapi.HybridLogicalTimestamp) any {
			return versionDocument(factoryapi.HybridLogicalTimestamp{
				Logical:  apitypes.Int64String(current.Logical.Int64() + logicalDelta),
				Physical: current.Physical.Add(physicalDelta),
			})
		},
		wantCode:  factoryapi.ErrorResponseCodeSTALEFACTORYVERSION,
		wantState: "task",
	}
}

func missingVersionCase() advancedSaveVersionCase {
	return advancedSaveVersionCase{
		name: "missing version fails",
		version: func(t *testing.T, current factoryapi.HybridLogicalTimestamp) any {
			return nil
		},
		wantCode:  factoryapi.ErrorResponseCodeSTALEFACTORYVERSION,
		wantState: "task",
	}
}

func missingLogicalVersionCase() advancedSaveVersionCase {
	return advancedSaveVersionCase{
		name: "missing logical fails",
		version: func(t *testing.T, current factoryapi.HybridLogicalTimestamp) any {
			return map[string]any{"physical": current.Physical.Add(time.Second).UTC().Format(time.RFC3339Nano)}
		},
		wantCode:  factoryapi.ErrorResponseCodeSTALEFACTORYVERSION,
		wantState: "task",
	}
}

func missingPhysicalVersionCase() advancedSaveVersionCase {
	return advancedSaveVersionCase{
		name: "missing physical fails",
		version: func(t *testing.T, current factoryapi.HybridLogicalTimestamp) any {
			return map[string]any{"logical": strconv.FormatInt(current.Logical.Int64()+1, 10)}
		},
		wantCode:  factoryapi.ErrorResponseCodeSTALEFACTORYVERSION,
		wantState: "task",
	}
}

func advancedVersionPassCase() advancedSaveVersionCase {
	return advancedSaveVersionCase{
		name: "greater logical and physical passes",
		version: func(t *testing.T, current factoryapi.HybridLogicalTimestamp) any {
			return versionDocument(advancedFactoryVersion(t, &current))
		},
		wantState: "story",
	}
}
