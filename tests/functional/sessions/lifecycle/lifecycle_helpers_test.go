package lifecycle_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// lifecycleClientProcess owns one reusable root-built client process for all
// sequential public CLI invocations of one lifecycle cell.
type lifecycleClientProcess struct {
	mu      sync.Mutex
	process support.ApplicationProcess
	env     []string
}

func sharedLifecycleClient(t *testing.T) *lifecycleClientProcess {
	t.Helper()
	if lifecycleFixture == nil || lifecycleFixture.client == nil {
		t.Fatal("shared lifecycle client process is unavailable")
	}
	return lifecycleFixture.client
}

func lifecycleWorkingDir(t *testing.T) string {
	t.Helper()
	if lifecycleFixture == nil || lifecycleFixture.clientWorkingDir == "" {
		t.Fatal("shared lifecycle client working directory is unavailable")
	}
	return lifecycleFixture.clientWorkingDir
}

// cleanupSessionViaAPI drives a created session to explicit terminal cleanup
// through the same public API boundary, tolerating an already-terminal
// session, so no cell leaves open-session residue for later cells.
func cleanupSessionViaAPI(t *testing.T, baseURL, sessionID string) {
	t.Helper()
	if strings.TrimSpace(sessionID) == "" {
		return
	}
	if err := terminateRemoteFunctionalSession(baseURL, sessionID); err != nil {
		t.Errorf("terminate lifecycle cell session %s: %v", sessionID, err)
	}
	closeSessionViaAPI(t, baseURL, sessionID)
}

func registerLifecycleSessionCleanup(t *testing.T, baseURL, sessionID string) func() {
	return registerLifecycleSessionCleanupAt(t, baseURL, sessionID, "")
}

func registerLifecycleSessionCleanupAt(
	t *testing.T,
	baseURL string,
	sessionID string,
	folderPath string,
) func() {
	t.Helper()

	ledger := lifecycleLedgerForTest(t)
	if err := ledger.registerSession(t.Name(), sessionID, folderPath); err != nil {
		t.Fatalf("register lifecycle cleanup census: %v", err)
	}

	var cleanupOnce sync.Once
	finish := func(cleanupAlreadyCompleted bool) {
		cleanupOnce.Do(func() {
			if !cleanupAlreadyCompleted {
				cleanupLifecycleSession(t, baseURL, sessionID)
			}

			publicAbsent := false
			terminalObserved := false
			if isLifecycleDurableSession(sessionID) {
				terminalObserved = assertDurableSessionTerminal(t, baseURL, sessionID)
			} else {
				assertAPISessionNotFound(t, baseURL, sessionID)
				publicAbsent = true
			}
			pathRemoved := removeLifecycleSessionPath(t, folderPath)
			if err := ledger.closeSession(sessionID, publicAbsent, terminalObserved, pathRemoved); err != nil {
				t.Errorf("record lifecycle session cleanup census: %v", err)
			}
		})
	}
	t.Cleanup(func() {
		finish(false)
	})
	return func() { finish(true) }
}

func cleanupLifecycleSession(t *testing.T, baseURL, sessionID string) {
	t.Helper()
	if isLifecycleDurableSession(sessionID) {
		if err := terminateRemoteFunctionalSession(baseURL, sessionID); err != nil {
			t.Errorf("terminate durable lifecycle cell session %s: %v", sessionID, err)
		}
		return
	}
	cleanupSessionViaAPI(t, baseURL, sessionID)
}

func (client *lifecycleClientProcess) executeCLI(
	ctx context.Context,
	workingDir string,
	serverURL string,
	args ...string,
) ([]byte, error) {
	client.mu.Lock()
	defer client.mu.Unlock()
	if client.process == nil {
		return nil, errors.New("shared lifecycle client process is unavailable")
	}
	cmdArgs := []string{"you"}
	if strings.TrimSpace(serverURL) != "" {
		cmdArgs = append(cmdArgs, "--server", serverURL)
	}
	cmdArgs = append(cmdArgs, args...)
	inputs := support.FakeInputs(ctx, cmdArgs)
	inputs.Input.Env = append([]string(nil), client.env...)
	inputs.Input.WorkingDirectory = workingDir
	var invocationID string
	if lifecycleFixture != nil && lifecycleFixture.ledger != nil {
		invocationID = lifecycleFixture.ledger.beginInvocation("shared lifecycle client Process.Execute")
	}
	err := client.process.Execute(inputs.Input)
	if invocationID != "" {
		err = errors.Join(err, lifecycleFixture.ledger.closeInvocation(invocationID))
	}
	combined := inputs.Stdout()
	if stderrText := inputs.Stderr(); strings.TrimSpace(stderrText) != "" {
		combined += "\n" + stderrText
	}
	return []byte(combined), err
}

func writeLifecycleFactory(dir string) error {
	rawConfig, err := json.Marshal(sessionLifecycleCRUDFactoryConfig())
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, interfaces.FactoryConfigFile), rawConfig, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", interfaces.FactoryConfigFile, err)
	}
	workstationPath := filepath.Join(
		dir,
		interfaces.WorkstationsDir,
		"process-task",
		interfaces.FactoryAgentsFileName,
	)
	if err := os.MkdirAll(filepath.Dir(workstationPath), 0o755); err != nil {
		return fmt.Errorf("create workstation directory: %w", err)
	}
	if err := os.WriteFile(
		workstationPath,
		[]byte("---\ntype: MODEL_WORKSTATION\n---\nDo the work.\n"),
		0o644,
	); err != nil {
		return fmt.Errorf("write workstation prompt: %w", err)
	}
	return nil
}

func writeLifecycleHostFactory(dir string) error {
	// The shared host only exercises the Factory Session control plane and
	// listener. Placement witnesses use their named JavaScript Factories, while
	// CRUD cells open their own authored Factory directories, so no host worker
	// or workstation is needed during the continuously-running idle command.
	config := sessionLifecycleCRUDFactoryConfig()
	config["workers"] = []map[string]string{}
	config["workstations"] = []map[string]any{}
	rawConfig, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, interfaces.FactoryConfigFile), rawConfig, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", interfaces.FactoryConfigFile, err)
	}
	return nil
}

func sessionListContains(
	sessions []factoryapi.FactorySessionSummary,
	sessionID string,
	folderPath string,
) bool {
	for _, session := range sessions {
		if session.Id == sessionID && session.FolderPath == folderPath {
			return true
		}
	}
	return false
}

func bytesTrimSpace(raw []byte) []byte {
	return []byte(strings.TrimSpace(string(raw)))
}

func postSessionLifecycleJSON[T any](t *testing.T, endpoint string, request any, failurePrefix string) T {
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

func terminateSessionViaAPI(t *testing.T, baseURL, sessionID string) {
	t.Helper()

	request, err := http.NewRequest(
		http.MethodPost,
		baseURL+"/factory-sessions/"+sessionID+"/terminate",
		bytes.NewReader([]byte(`{}`)),
	)
	if err != nil {
		t.Fatalf("construct terminate session request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("POST terminate Factory Session %q: %v", sessionID, err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf("POST terminate Factory Session %q status = %d, want success: %s", sessionID, response.StatusCode, payload)
	}
}

func closeSessionViaAPI(t *testing.T, baseURL, sessionID string) {
	t.Helper()

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

func assertAPISessionNotFound(t *testing.T, baseURL, sessionID string) {
	t.Helper()

	response, err := http.Get(baseURL + "/factory-sessions/" + sessionID)
	if err != nil {
		t.Fatalf("GET Factory Session %q: %v", sessionID, err)
	}
	defer response.Body.Close()
	assertAPISessionTypedNotFoundHTTPResponse(t, response, "GET", sessionID)
}

func assertAPIDeleteSessionTypedNotFound(t *testing.T, baseURL, sessionID string) {
	t.Helper()

	request, err := http.NewRequest(http.MethodDelete, baseURL+"/factory-sessions/"+sessionID, nil)
	if err != nil {
		t.Fatalf("construct delete Factory Session request: %v", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("DELETE Factory Session %q: %v", sessionID, err)
	}
	defer response.Body.Close()
	assertAPISessionTypedNotFoundHTTPResponse(t, response, "DELETE", sessionID)
}

func assertAPISessionTypedNotFoundHTTPResponse(
	t *testing.T,
	response *http.Response,
	operation string,
	sessionID string,
) {
	t.Helper()

	if response.StatusCode != http.StatusNotFound {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf("%s missing Factory Session %q status = %d, want 404: %s", operation, sessionID, response.StatusCode, payload)
	}

	contentType := response.Header.Get("Content-Type")
	if !strings.Contains(contentType, "application/json") {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf("%s missing Factory Session %q Content-Type = %q, want application/json structured error body: %s",
			operation, sessionID, contentType, payload)
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("%s missing Factory Session %q read body: %v", operation, sessionID, err)
	}
	var errResp factoryapi.ErrorResponse
	if err := json.Unmarshal(body, &errResp); err != nil {
		t.Fatalf("%s missing Factory Session %q decode structured error: %v\nbody: %s", operation, sessionID, err, body)
	}
	if errResp.Family != factoryapi.ErrorFamilyNotFound {
		t.Fatalf("%s missing Factory Session %q error family = %q, want %q: %#v",
			operation, sessionID, errResp.Family, factoryapi.ErrorFamilyNotFound, errResp)
	}
	if errResp.Code != factoryapi.ErrorResponseCodeNOTFOUND {
		t.Fatalf("%s missing Factory Session %q error code = %q, want %q: %#v",
			operation, sessionID, errResp.Code, factoryapi.ErrorResponseCodeNOTFOUND, errResp)
	}
	if strings.TrimSpace(errResp.Message) == "" {
		t.Fatalf("%s missing Factory Session %q error message is empty, want customer-readable not-found text: %#v",
			operation, sessionID, errResp)
	}
	if !strings.Contains(strings.ToLower(errResp.Message), "not found") {
		t.Fatalf("%s missing Factory Session %q error message = %q, want not-found guidance: %#v",
			operation, sessionID, errResp.Message, errResp)
	}

	if operation == "GET" {
		var shown factoryapi.FactorySession
		if json.Unmarshal(body, &shown) == nil && strings.TrimSpace(shown.Id) != "" {
			t.Fatalf("GET missing Factory Session %q must not emit a success session payload: %#v", sessionID, shown)
		}
		if strings.Contains(string(body), `"runtime"`) {
			t.Fatalf("GET missing Factory Session %q must not emit a success session payload: %s", sessionID, body)
		}
	}
}

func assertCLISessionNotFoundFailure(
	t *testing.T,
	operation string,
	output []byte,
	err error,
	sessionID string,
	expectDeleteConfirmation bool,
) {
	t.Helper()

	if err == nil {
		t.Fatalf("you session %s unexpectedly succeeded:\n%s", operation, output)
	}

	text := string(output)
	support.RequireNotFoundCLIDiagnostic(t, text)
	if strings.Contains(text, sessionID) {
		t.Fatalf("session %s leaked session id %q in safe diagnostic:\n%s", operation, sessionID, text)
	}

	if expectDeleteConfirmation {
		var deleted struct {
			SessionID string `json:"sessionId"`
		}
		if json.Unmarshal(bytesTrimSpace(output), &deleted) == nil && deleted.SessionID != "" {
			t.Fatalf("session delete must not emit success confirmation payload:\n%s", text)
		}
		return
	}

	var shown factoryapi.FactorySession
	if json.Unmarshal(bytesTrimSpace(output), &shown) == nil && strings.TrimSpace(shown.Id) != "" {
		t.Fatalf("session show must not emit a success session payload:\n%s", text)
	}
	if strings.Contains(text, `"id"`) && strings.Contains(text, `"runtime"`) {
		t.Fatalf("session show must not emit a success session payload:\n%s", text)
	}
}
