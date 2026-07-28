package lifecycle_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const sessionLifecycleCRUDMissingSessionID = "dur-sess-missing-999"

// TestFactorySessionCreateListShowDelete proves the public CLI Factory Session
// boundary supports a full create → list → show → delete lifecycle: a newly
// created session returns a stable public session ID, appears in session list
// and session show with matching folder identity, and session delete removes it
// so subsequent list/show no longer treat it as an open session.
func TestFactorySessionCreateListShowDelete(t *testing.T) {
	primaryFactoryDir := support.ScaffoldFactory(t, sessionLifecycleCRUDFactoryConfig())
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:     primaryFactoryDir,
		UseMockWorkers: true,
	})
	defer server.Stop(t)

	baseURL := server.URL()
	serverPort := portFromServerURL(t, baseURL)
	binaryPath := buildYouCLIBinary(t)

	newFactoryDir := filepath.Join(t.TempDir(), "session-lifecycle-crud-factory")
	if err := os.Mkdir(newFactoryDir, 0o755); err != nil {
		t.Fatalf("create factory directory: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	createOut, err := runYouCLI(ctx, binaryPath, primaryFactoryDir, baseURL,
		"--json",
		"session", "create",
		"--dir", newFactoryDir,
		"--init-new-factory",
	)
	if err != nil {
		t.Fatalf("you session create: %v\noutput:\n%s", err, createOut)
	}

	var created factoryapi.OpenFactorySessionResponse
	if err := json.Unmarshal(bytesTrimSpace(createOut), &created); err != nil {
		t.Fatalf("decode session create JSON: %v\noutput:\n%s", err, createOut)
	}
	if created.Session == nil || strings.TrimSpace(created.Session.Id) == "" {
		t.Fatalf("session create response missing session id: %#v", created)
	}
	if created.Session.FolderPath != newFactoryDir {
		t.Fatalf("session folder path = %q, want %q", created.Session.FolderPath, newFactoryDir)
	}
	sessionID := created.Session.Id

	listOut, err := runYouCLI(ctx, binaryPath, primaryFactoryDir, baseURL, "session", "list")
	if err != nil {
		t.Fatalf("you session list: %v\noutput:\n%s", err, listOut)
	}
	listHuman := string(listOut)
	for _, marker := range []string{sessionID, newFactoryDir} {
		if !strings.Contains(listHuman, marker) {
			t.Fatalf("session list output missing %q:\n%s", marker, listHuman)
		}
	}

	listJSONOut, err := runYouCLI(ctx, binaryPath, primaryFactoryDir, baseURL,
		"--json",
		"session", "list",
	)
	if err != nil {
		t.Fatalf("you session list --json: %v\noutput:\n%s", err, listJSONOut)
	}
	var listed factoryapi.ListFactorySessionsResponse
	if err := json.Unmarshal(bytesTrimSpace(listJSONOut), &listed); err != nil {
		t.Fatalf("decode session list JSON: %v\noutput:\n%s", err, listJSONOut)
	}
	if !sessionListContains(listed.Sessions, sessionID, newFactoryDir) {
		t.Fatalf("session list JSON missing created session %q at %q: %#v", sessionID, newFactoryDir, listed.Sessions)
	}

	showOut, err := runYouCLI(ctx, binaryPath, primaryFactoryDir, baseURL,
		"--json",
		"session", "show", sessionID,
	)
	if err != nil {
		t.Fatalf("you session show: %v\noutput:\n%s", err, showOut)
	}
	var shown factoryapi.FactorySession
	if err := json.Unmarshal(bytesTrimSpace(showOut), &shown); err != nil {
		t.Fatalf("decode session show JSON: %v\noutput:\n%s", err, showOut)
	}
	if shown.Id != sessionID {
		t.Fatalf("session show id = %q, want %q", shown.Id, sessionID)
	}
	if shown.FolderPath != newFactoryDir {
		t.Fatalf("session show folder path = %q, want %q", shown.FolderPath, newFactoryDir)
	}
	if shown.Runtime.Status == "" {
		t.Fatalf("session show missing runtime status markers: %#v", shown)
	}

	deleteOut, err := runYouCLI(ctx, binaryPath, primaryFactoryDir, "",
		"--json",
		"session", "delete", sessionID,
		"--port", fmt.Sprintf("%d", serverPort),
	)
	if err != nil {
		t.Fatalf("you session delete: %v\noutput:\n%s", err, deleteOut)
	}
	var deleted struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(bytesTrimSpace(deleteOut), &deleted); err != nil {
		t.Fatalf("decode session delete JSON: %v\noutput:\n%s", err, deleteOut)
	}
	if deleted.SessionID != sessionID {
		t.Fatalf("session delete confirmation = %q, want %q", deleted.SessionID, sessionID)
	}

	showAfterDeleteOut, err := runYouCLI(ctx, binaryPath, primaryFactoryDir, baseURL,
		"--json",
		"session", "show", sessionID,
	)
	if err == nil {
		t.Fatalf("you session show after delete unexpectedly succeeded:\n%s", showAfterDeleteOut)
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("you session show after delete error = %v, want *exec.ExitError", err)
	}
	if exitErr.ExitCode() == 0 {
		t.Fatalf("you session show after delete exit code = 0, want non-zero")
	}
	showAfterDelete := string(showAfterDeleteOut)
	if !strings.Contains(showAfterDelete, "not found") {
		t.Fatalf("session show after delete missing not-found diagnostic:\n%s", showAfterDelete)
	}
	if strings.Contains(showAfterDelete, sessionID) && strings.Contains(showAfterDelete, `"id"`) {
		t.Fatalf("session show after delete must not emit a success session payload:\n%s", showAfterDelete)
	}
}

// TestFactorySessionListMultipleSessions proves the public CLI Factory Session
// boundary lists every open session as a distinct entry: creating at least two
// sessions and running session list returns both session IDs without collapsing
// them into one row or omitting either expected identity.
func TestFactorySessionListMultipleSessions(t *testing.T) {
	primaryFactoryDir := support.ScaffoldFactory(t, sessionLifecycleCRUDFactoryConfig())
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:     primaryFactoryDir,
		UseMockWorkers: true,
	})
	defer server.Stop(t)

	baseURL := server.URL()
	binaryPath := buildYouCLIBinary(t)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	firstFactoryDir := filepath.Join(t.TempDir(), "session-lifecycle-crud-factory-a")
	if err := os.Mkdir(firstFactoryDir, 0o755); err != nil {
		t.Fatalf("create first factory directory: %v", err)
	}
	firstSession := createSessionViaCLI(t, ctx, binaryPath, primaryFactoryDir, baseURL, firstFactoryDir)

	secondFactoryDir := filepath.Join(t.TempDir(), "session-lifecycle-crud-factory-b")
	if err := os.Mkdir(secondFactoryDir, 0o755); err != nil {
		t.Fatalf("create second factory directory: %v", err)
	}
	secondSession := createSessionViaCLI(t, ctx, binaryPath, primaryFactoryDir, baseURL, secondFactoryDir)

	if firstSession.id == secondSession.id {
		t.Fatalf("session ids must be distinct: first=%q second=%q", firstSession.id, secondSession.id)
	}

	listOut, err := runYouCLI(ctx, binaryPath, primaryFactoryDir, baseURL, "session", "list")
	if err != nil {
		t.Fatalf("you session list: %v\noutput:\n%s", err, listOut)
	}
	listHuman := string(listOut)
	for _, marker := range []string{
		firstSession.id, firstFactoryDir,
		secondSession.id, secondFactoryDir,
	} {
		if !strings.Contains(listHuman, marker) {
			t.Fatalf("session list output missing %q:\n%s", marker, listHuman)
		}
	}

	listJSONOut, err := runYouCLI(ctx, binaryPath, primaryFactoryDir, baseURL,
		"--json",
		"session", "list",
	)
	if err != nil {
		t.Fatalf("you session list --json: %v\noutput:\n%s", err, listJSONOut)
	}
	var listed factoryapi.ListFactorySessionsResponse
	if err := json.Unmarshal(bytesTrimSpace(listJSONOut), &listed); err != nil {
		t.Fatalf("decode session list JSON: %v\noutput:\n%s", err, listJSONOut)
	}
	if !sessionListContains(listed.Sessions, firstSession.id, firstFactoryDir) {
		t.Fatalf("session list JSON missing first session %q at %q: %#v", firstSession.id, firstFactoryDir, listed.Sessions)
	}
	if !sessionListContains(listed.Sessions, secondSession.id, secondFactoryDir) {
		t.Fatalf("session list JSON missing second session %q at %q: %#v", secondSession.id, secondFactoryDir, listed.Sessions)
	}
	assertDistinctSessionListEntries(t, listed.Sessions, firstSession.id, secondSession.id)
}

// TestFactorySessionMissingShowAndDeleteFail proves the public CLI Factory Session
// boundary rejects session show and delete for a missing session ID with a
// customer-visible not-found outcome, without removing or corrupting any still-open
// session created for isolation proof.
func TestFactorySessionMissingShowAndDeleteFail(t *testing.T) {
	primaryFactoryDir := support.ScaffoldFactory(t, sessionLifecycleCRUDFactoryConfig())
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:     primaryFactoryDir,
		UseMockWorkers: true,
	})
	defer server.Stop(t)

	baseURL := server.URL()
	serverPort := portFromServerURL(t, baseURL)
	binaryPath := buildYouCLIBinary(t)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	isolationFactoryDir := filepath.Join(t.TempDir(), "session-lifecycle-crud-isolation-factory")
	if err := os.Mkdir(isolationFactoryDir, 0o755); err != nil {
		t.Fatalf("create isolation factory directory: %v", err)
	}
	openSession := createSessionViaCLI(t, ctx, binaryPath, primaryFactoryDir, baseURL, isolationFactoryDir)

	showOut, err := runYouCLI(ctx, binaryPath, primaryFactoryDir, baseURL,
		"--json",
		"session", "show", sessionLifecycleCRUDMissingSessionID,
	)
	assertCLISessionNotFoundFailure(t, "show", showOut, err, sessionLifecycleCRUDMissingSessionID, false)

	deleteOut, err := runYouCLI(ctx, binaryPath, primaryFactoryDir, "",
		"--json",
		"session", "delete", sessionLifecycleCRUDMissingSessionID,
		"--port", fmt.Sprintf("%d", serverPort),
	)
	assertCLISessionNotFoundFailure(t, "delete", deleteOut, err, sessionLifecycleCRUDMissingSessionID, true)

	showOut, err = runYouCLI(ctx, binaryPath, primaryFactoryDir, baseURL,
		"--json",
		"session", "show", openSession.id,
	)
	if err != nil {
		t.Fatalf("you session show for isolation session: %v\noutput:\n%s", err, showOut)
	}
	var shown factoryapi.FactorySession
	if err := json.Unmarshal(bytesTrimSpace(showOut), &shown); err != nil {
		t.Fatalf("decode isolation session show JSON: %v\noutput:\n%s", err, showOut)
	}
	if shown.Id != openSession.id {
		t.Fatalf("isolation session show id = %q, want %q", shown.Id, openSession.id)
	}
	if shown.FolderPath != isolationFactoryDir {
		t.Fatalf("isolation session show folder path = %q, want %q", shown.FolderPath, isolationFactoryDir)
	}

	listJSONOut, err := runYouCLI(ctx, binaryPath, primaryFactoryDir, baseURL,
		"--json",
		"session", "list",
	)
	if err != nil {
		t.Fatalf("you session list --json after missing delete: %v\noutput:\n%s", err, listJSONOut)
	}
	var listed factoryapi.ListFactorySessionsResponse
	if err := json.Unmarshal(bytesTrimSpace(listJSONOut), &listed); err != nil {
		t.Fatalf("decode session list JSON after missing delete: %v\noutput:\n%s", err, listJSONOut)
	}
	if !sessionListContains(listed.Sessions, openSession.id, isolationFactoryDir) {
		t.Fatalf("isolation session %q at %q missing from list after missing delete attempt: %#v",
			openSession.id, isolationFactoryDir, listed.Sessions)
	}
}

// TestAPIOpenListGetAndCloseFactorySession proves the public HTTP Factory Session
// boundary supports a full open → list → get → close lifecycle: POST /factory-sessions
// opens a live session with a public ID, GET /factory-sessions lists it, GET
// /factory-sessions/{session_id} returns the matching inspection read model, and
// DELETE /factory-sessions/{session_id} closes it so subsequent get/list no longer
// treat it as an open live session.
func TestAPIOpenListGetAndCloseFactorySession(t *testing.T) {
	primaryFactoryDir := support.ScaffoldFactory(t, sessionLifecycleCRUDFactoryConfig())
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:     primaryFactoryDir,
		UseMockWorkers: true,
	})
	defer server.Stop(t)

	baseURL := strings.TrimSuffix(server.URL(), "/")
	newFactoryDir := filepath.Join(t.TempDir(), "session-lifecycle-crud-api-factory")
	if err := os.Mkdir(newFactoryDir, 0o755); err != nil {
		t.Fatalf("create factory directory: %v", err)
	}

	initNewFactory := true
	opened := postSessionLifecycleJSON[factoryapi.OpenFactorySessionResponse](
		t,
		baseURL+"/factory-sessions",
		factoryapi.OpenFactorySessionRequest{
			FolderPath:     newFactoryDir,
			InitNewFactory: &initNewFactory,
		},
		"open Factory Session",
	)
	if opened.Session == nil || strings.TrimSpace(opened.Session.Id) == "" {
		t.Fatalf("open session response missing session id: %#v", opened)
	}
	if opened.Session.FolderPath != newFactoryDir {
		t.Fatalf("open session folder path = %q, want %q", opened.Session.FolderPath, newFactoryDir)
	}
	sessionID := opened.Session.Id

	listed := support.GetJSON[factoryapi.ListFactorySessionsResponse](t, baseURL+"/factory-sessions")
	if !sessionListContains(listed.Sessions, sessionID, newFactoryDir) {
		t.Fatalf("list sessions missing opened session %q at %q: %#v", sessionID, newFactoryDir, listed.Sessions)
	}

	selected := support.GetJSON[factoryapi.FactorySessionGetResponse](
		t,
		baseURL+"/factory-sessions/"+sessionID,
	)
	resolved, err := selected.AsFactorySession()
	if err != nil {
		t.Fatalf("decode opened session get response: %v", err)
	}
	if resolved.Id != sessionID {
		t.Fatalf("get session id = %q, want %q", resolved.Id, sessionID)
	}
	if resolved.FolderPath != newFactoryDir {
		t.Fatalf("get session folder path = %q, want %q", resolved.FolderPath, newFactoryDir)
	}
	if resolved.Runtime.Status == "" {
		t.Fatalf("get session missing runtime status markers: %#v", resolved)
	}

	closeSessionViaAPI(t, baseURL, sessionID)
	assertAPISessionNotFound(t, baseURL, sessionID)

	afterClose := support.GetJSON[factoryapi.ListFactorySessionsResponse](t, baseURL+"/factory-sessions")
	if sessionListContains(afterClose.Sessions, sessionID, newFactoryDir) {
		t.Fatalf("list sessions after close still contains %q at %q: %#v", sessionID, newFactoryDir, afterClose.Sessions)
	}
}

// TestAPIFactorySessionNotFoundUsesTypedError proves the public HTTP Factory Session
// boundary returns a typed not-found error (HTTP 404 with stable NOT_FOUND family/code)
// when get or close targets a missing session ID, without silently resolving to the
// default or another live session.
func TestAPIFactorySessionNotFoundUsesTypedError(t *testing.T) {
	primaryFactoryDir := support.ScaffoldFactory(t, sessionLifecycleCRUDFactoryConfig())
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:     primaryFactoryDir,
		UseMockWorkers: true,
	})
	defer server.Stop(t)

	baseURL := strings.TrimSuffix(server.URL(), "/")
	openFactoryDir := filepath.Join(t.TempDir(), "session-lifecycle-crud-api-isolation-factory")
	if err := os.Mkdir(openFactoryDir, 0o755); err != nil {
		t.Fatalf("create factory directory: %v", err)
	}

	initNewFactory := true
	opened := postSessionLifecycleJSON[factoryapi.OpenFactorySessionResponse](
		t,
		baseURL+"/factory-sessions",
		factoryapi.OpenFactorySessionRequest{
			FolderPath:     openFactoryDir,
			InitNewFactory: &initNewFactory,
		},
		"open isolation Factory Session",
	)
	if opened.Session == nil || strings.TrimSpace(opened.Session.Id) == "" {
		t.Fatalf("open isolation session response missing session id: %#v", opened)
	}
	openSessionID := opened.Session.Id
	if openSessionID == sessionLifecycleCRUDMissingSessionID {
		t.Fatalf("open session id = %q, want distinct from missing probe id %q", openSessionID, sessionLifecycleCRUDMissingSessionID)
	}

	assertAPISessionNotFound(t, baseURL, sessionLifecycleCRUDMissingSessionID)

	selected := support.GetJSON[factoryapi.FactorySessionGetResponse](
		t,
		baseURL+"/factory-sessions/"+openSessionID,
	)
	resolved, err := selected.AsFactorySession()
	if err != nil {
		t.Fatalf("decode isolation session get response: %v", err)
	}
	if resolved.Id != openSessionID {
		t.Fatalf("isolation session get id = %q, want %q", resolved.Id, openSessionID)
	}
	if resolved.Id == sessionLifecycleCRUDMissingSessionID {
		t.Fatalf("missing-session get must not resolve to another live session id %q", openSessionID)
	}

	assertAPIDeleteSessionTypedNotFound(t, baseURL, sessionLifecycleCRUDMissingSessionID)

	selectedAfterDelete, err := http.Get(baseURL + "/factory-sessions/" + openSessionID)
	if err != nil {
		t.Fatalf("GET isolation Factory Session %q after missing delete: %v", openSessionID, err)
	}
	defer selectedAfterDelete.Body.Close()
	if selectedAfterDelete.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(selectedAfterDelete.Body)
		t.Fatalf("GET isolation Factory Session %q after missing delete status = %d, want 200: %s",
			openSessionID, selectedAfterDelete.StatusCode, payload)
	}
}

type cliCreatedSession struct {
	id         string
	folderPath string
}

func createSessionViaCLI(
	t *testing.T,
	ctx context.Context,
	binaryPath string,
	workingDir string,
	serverURL string,
	factoryDir string,
) cliCreatedSession {
	t.Helper()

	createOut, err := runYouCLI(ctx, binaryPath, workingDir, serverURL,
		"--json",
		"session", "create",
		"--dir", factoryDir,
		"--init-new-factory",
	)
	if err != nil {
		t.Fatalf("you session create for %q: %v\noutput:\n%s", factoryDir, err, createOut)
	}

	var created factoryapi.OpenFactorySessionResponse
	if err := json.Unmarshal(bytesTrimSpace(createOut), &created); err != nil {
		t.Fatalf("decode session create JSON for %q: %v\noutput:\n%s", factoryDir, err, createOut)
	}
	if created.Session == nil || strings.TrimSpace(created.Session.Id) == "" {
		t.Fatalf("session create response for %q missing session id: %#v", factoryDir, created)
	}
	if created.Session.FolderPath != factoryDir {
		t.Fatalf("session folder path for %q = %q, want %q", factoryDir, created.Session.FolderPath, factoryDir)
	}
	return cliCreatedSession{
		id:         created.Session.Id,
		folderPath: factoryDir,
	}
}

func assertDistinctSessionListEntries(
	t *testing.T,
	sessions []factoryapi.FactorySessionSummary,
	firstSessionID string,
	secondSessionID string,
) {
	t.Helper()

	firstCount := 0
	secondCount := 0
	for _, session := range sessions {
		switch session.Id {
		case firstSessionID:
			firstCount++
		case secondSessionID:
			secondCount++
		}
	}
	if firstCount != 1 {
		t.Fatalf("session list JSON has %d entries for first session %q, want exactly 1: %#v", firstCount, firstSessionID, sessions)
	}
	if secondCount != 1 {
		t.Fatalf("session list JSON has %d entries for second session %q, want exactly 1: %#v", secondCount, secondSessionID, sessions)
	}
}

func sessionLifecycleCRUDFactoryConfig() map[string]any {
	return map[string]any{
		"name": "session-lifecycle-crud",
		"workTypes": []map[string]any{
			{
				"name": "task",
				"states": []map[string]any{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
		},
		"workers": []map[string]string{
			{"name": "mock-worker"},
		},
		"workstations": []map[string]any{
			{
				"name":      "process-task",
				"worker":    "mock-worker",
				"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
				"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
				"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
			},
		},
	}
}

func buildYouCLIBinary(t *testing.T) string {
	t.Helper()

	binaryName := "you"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	binaryPath := filepath.Join(t.TempDir(), binaryName)
	build := exec.Command("go", "build", "-o", binaryPath, "./cmd/factory")
	build.Dir = testutil.MustRepoRoot(t)
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build you CLI: %v\n%s", err, string(output))
	}
	return binaryPath
}

func runYouCLI(
	ctx context.Context,
	binaryPath string,
	workingDir string,
	serverURL string,
	args ...string,
) ([]byte, error) {
	cmdArgs := []string{}
	if strings.TrimSpace(serverURL) != "" {
		cmdArgs = append(cmdArgs, "--server", serverURL)
	}
	cmdArgs = append(cmdArgs, args...)
	cmd := exec.CommandContext(ctx, binaryPath, cmdArgs...)
	cmd.Dir = workingDir
	return cmd.CombinedOutput()
}

func portFromServerURL(t *testing.T, serverURL string) int {
	t.Helper()

	parsed, err := url.Parse(serverURL)
	if err != nil {
		t.Fatalf("parse server URL %q: %v", serverURL, err)
	}
	port := parsed.Port()
	if port == "" {
		t.Fatalf("server URL %q missing port", serverURL)
	}
	var portNumber int
	if _, err := fmt.Sscanf(port, "%d", &portNumber); err != nil {
		t.Fatalf("parse server port %q: %v", port, err)
	}
	return portNumber
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
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("you session %s error = %v, want *exec.ExitError", operation, err)
	}
	if exitErr.ExitCode() == 0 {
		t.Fatalf("you session %s exit code = 0, want non-zero", operation)
	}

	text := string(output)
	if !strings.Contains(strings.ToLower(text), "not found") {
		t.Fatalf("session %s missing not-found diagnostic:\n%s", operation, text)
	}
	if !strings.Contains(text, sessionID) {
		t.Fatalf("session %s missing session id %q in diagnostic:\n%s", operation, sessionID, text)
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
