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
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/root"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
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

func startCurrentFactoryServerWithoutMockWorkersWithSetup(
	t *testing.T,
	rootDir string,
	setup currentFactoryServerSetup,
) *support.FunctionalAPIServer {
	t.Helper()
	return support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                rootDir,
		WaitForServiceModeRuntime: true,
		BeforeStart:               setup,
	})
}

// sharedCurrentFactoryAPI owns the one process and API listener used by the
// compatible Current Factory read/save scenarios. Named Factory definitions
// are authored before the continuous server invocation starts; each scenario
// then gets its own explicit Factory Session and uniquely named fixture so a
// save cannot affect another witness's initial state.
type sharedCurrentFactoryAPI struct {
	rootDir string
	server  *support.FunctionalAPIServer
}

func startSharedCurrentFactoryAPI(t *testing.T) *sharedCurrentFactoryAPI {
	t.Helper()

	fixture := &sharedCurrentFactoryAPI{rootDir: t.TempDir()}
	fixture.server = startCurrentFactoryServerWithoutMockWorkersWithSetup(
		t,
		fixture.rootDir,
		currentFactorySetup(t, func(process support.Process, env []string) {
			seedNamedFactoryRootWithProcess(t, process, env, fixture.rootDir, "alpha", "alpha-task")
			createNamedFactoryFixtureWithProcess(
				t,
				process,
				env,
				fixture.rootDir,
				"alpha-valid",
				functionalNamedFactoryPayloadWithWorkType(t, "alpha-valid", "alpha-valid-task"),
			)
			createNamedFactoryFixtureWithProcess(
				t,
				process,
				env,
				fixture.rootDir,
				"alpha-invalid",
				functionalNamedFactoryPayloadWithWorkType(t, "alpha-invalid", "alpha-invalid-task"),
			)
			createNamedFactoryFixtureWithProcess(
				t,
				process,
				env,
				fixture.rootDir,
				"alpha-isolation",
				functionalNamedFactoryPayloadWithWorkType(t, "alpha-isolation", "alpha-isolation-task"),
			)
			createNamedFactoryFixtureWithProcess(
				t,
				process,
				env,
				fixture.rootDir,
				"beta-isolation",
				functionalNamedFactoryPayloadWithWorkType(t, "beta-isolation", "beta-isolation-task"),
			)
			createNamedFactoryFixtureWithProcess(
				t,
				process,
				env,
				fixture.rootDir,
				"alpha-prompt-contract",
				functionalNamedFactoryPayloadWithWorkType(t, "alpha-prompt-contract", "alpha-prompt-contract-task"),
			)
			createNamedFactoryFixtureWithProcess(
				t,
				process,
				env,
				fixture.rootDir,
				"alpha-prompt-invalid",
				functionalNamedFactoryPayloadWithWorkType(t, "alpha-prompt-invalid", "alpha-prompt-invalid-task"),
			)
			createNamedFactoryFixtureWithProcess(
				t,
				process,
				env,
				fixture.rootDir,
				"alpha-prompt-nonmutation",
				functionalNamedFactoryPayloadWithWorkType(t, "alpha-prompt-nonmutation", "alpha-prompt-nonmutation-task"),
			)
		}),
	)
	return fixture
}

type sharedCurrentFactorySession struct {
	serverURL string
	id        string
	closed    bool
}

func (fixture *sharedCurrentFactoryAPI) openSession(t *testing.T, name string) *sharedCurrentFactorySession {
	t.Helper()
	if fixture == nil || fixture.server == nil {
		t.Fatal("shared Current Factory API fixture is unavailable")
	}
	session := &sharedCurrentFactorySession{
		serverURL: fixture.server.URL(),
		id:        openNamedFactorySession(t, fixture.server.URL(), fixture.rootDir, name),
	}
	if session.id == "~default" {
		t.Fatalf("named Factory Session %q unexpectedly used the default session", name)
	}
	t.Cleanup(func() {
		if session.closed {
			return
		}
		support.CloseFactorySessionAt(t, session.serverURL, session.id)
	})
	return session
}

func (session *sharedCurrentFactorySession) close(t *testing.T) {
	t.Helper()
	if session == nil || session.closed {
		return
	}
	support.CloseFactorySessionAt(t, session.serverURL, session.id)
	session.closed = true
}

func (fixture *sharedCurrentFactoryAPI) requireServerRunning(t *testing.T) {
	t.Helper()
	if fixture == nil || fixture.server == nil {
		t.Fatal("shared Current Factory API fixture is unavailable")
	}
	select {
	case <-fixture.server.Done():
		t.Fatal("shared Current Factory process exited before the scenario completed")
	default:
	}
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
	resp := saveCurrentFactoryForSessionExpectStatus(t, serverURL, sessionID, body, http.StatusOK)
	var saved factoryapi.Factory
	decodeJSONResponse(t, resp, &saved, "decode session current factory save response")
	return saved
}

func saveCurrentFactoryForSessionExpectStatus(
	t *testing.T,
	serverURL, sessionID, body string,
	wantStatus int,
) *http.Response {
	t.Helper()
	return putFactoryForSessionRequestExpectStatusWithClient(
		t,
		http.DefaultClient,
		serverURL,
		"/factory-sessions/"+url.PathEscape(sessionID)+"/factory",
		saveFactoryForSessionRequestBody(body),
		wantStatus,
	)
}

func sessionFactoryURL(serverURL, sessionID string) string {
	return serverURL + "/factory-sessions/" + url.PathEscape(sessionID) + "/factory"
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
	return saveCurrentFactoryForSessionExpectStatus(t, serverURL, "~default", body, wantStatus)
}

func getCurrentFactoryForSessionExpectStatus(
	t *testing.T,
	serverURL, sessionID string,
	wantStatus int,
) factoryapi.ErrorResponse {
	t.Helper()
	resp, err := http.Get(sessionFactoryURL(serverURL, sessionID))
	if err != nil {
		t.Fatalf("GET /factory-sessions/%s/factory: %v", sessionID, err)
	}
	body, readErr := io.ReadAll(resp.Body)
	resp.Body.Close()
	if readErr != nil {
		t.Fatalf("read GET /factory-sessions/%s/factory response: %v", sessionID, readErr)
	}
	if resp.StatusCode != wantStatus {
		t.Fatalf(
			"GET /factory-sessions/%s/factory status = %d, want %d: %s",
			sessionID,
			resp.StatusCode,
			wantStatus,
			body,
		)
	}
	var errResp factoryapi.ErrorResponse
	if err := json.Unmarshal(body, &errResp); err != nil {
		t.Fatalf("decode GET /factory-sessions/%s/factory error response: %v: %s", sessionID, err, body)
	}
	return errResp
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

func assertFactoryVersionAtLeast(
	t *testing.T,
	got *factoryapi.HybridLogicalTimestamp,
	want factoryapi.HybridLogicalTimestamp,
	contextLabel string,
) {
	t.Helper()
	if got == nil || got.Logical.Int64() < want.Logical.Int64() ||
		(got.Logical.Int64() == want.Logical.Int64() && got.Physical.Before(want.Physical)) {
		t.Fatalf("%s version = %#v, want at least %#v", contextLabel, got, want)
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
