package commands_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	sessionPauseWiringRequestID   = "cli-session-pause-wiring"
	sessionPauseWiringWorkName    = "paused-task"
	sessionWiringMissingSessionID = "dur-sess-missing-999"
)

// TestCLISessionCreateListShowDelete proves you session create, list, show, and
// delete work as a thin CLI lifecycle against a running Factory Session server,
// yielding observable session identity and success or failure exit behavior.
func TestCLISessionCreateListShowDelete(t *testing.T) {
	primaryFactoryDir := support.ScaffoldFactory(t, sessionWiringFactoryConfig())
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:     primaryFactoryDir,
		UseMockWorkers: true,
	})
	defer server.Stop(t)

	baseURL := server.URL()
	serverPort := portFromServerURL(t, baseURL)
	binaryPath := buildYouCLIBinary(t)

	newFactoryDir := filepath.Join(t.TempDir(), "cli-session-wiring-factory")
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
	if !sessionWiringListContains(listed.Sessions, sessionID, newFactoryDir) {
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

// TestCLISessionPauseBuffersAndResumeDispatches proves you session pause keeps
// accepted work buffered and you session resume restores dispatch through the
// public CLI against a running Factory Session server.
func TestCLISessionPauseBuffersAndResumeDispatches(t *testing.T) {
	factoryDir := support.ScaffoldFactory(t, sessionWiringFactoryConfig())
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:     factoryDir,
		UseMockWorkers: true,
	})
	defer server.Stop(t)

	baseURL := server.URL()
	binaryPath := buildYouCLIBinary(t)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	pauseResponse := runSessionLifecycleCLIJSON(
		t, ctx, binaryPath, factoryDir, baseURL, "pause", factorysessions.DefaultSessionID,
	)
	if pauseResponse.Operation != factoryapi.FactorySessionLifecycleControlKindPause ||
		pauseResponse.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("session pause response = %#v, want accepted pause", pauseResponse)
	}
	if pauseResponse.Status != factoryapi.FactorySessionDurableLifecycleStatusPaused {
		t.Fatalf("session pause status = %q, want PAUSED", pauseResponse.Status)
	}

	pausedSession := runSessionShowCLIJSON(t, ctx, binaryPath, factoryDir, baseURL, factorysessions.DefaultSessionID)
	if pausedSession.Runtime.LifecycleControlStatus == nil ||
		*pausedSession.Runtime.LifecycleControlStatus != factoryapi.FactorySessionDurableLifecycleStatusPaused {
		t.Fatalf("session show after pause missing paused lifecycle marker: %#v", pausedSession.Runtime)
	}

	inlineBatch := fmt.Sprintf(
		`{"requestId":%q,"type":"FACTORY_REQUEST_BATCH","works":[{"name":%q,"workTypeName":"task","payload":{"title":"Paused session wiring"}}]}`,
		sessionPauseWiringRequestID,
		sessionPauseWiringWorkName,
	)
	submitOut, err := runYouCLI(ctx, binaryPath, factoryDir, baseURL,
		"--json",
		"submit", "batch",
		inlineBatch,
	)
	if err != nil {
		t.Fatalf("you submit batch while paused: %v\noutput:\n%s", err, submitOut)
	}
	var submitted sessionWiringBatchSubmitJSON
	if err := json.Unmarshal(bytesTrimSpace(submitOut), &submitted); err != nil {
		t.Fatalf("decode submit batch JSON: %v\noutput:\n%s", err, submitOut)
	}
	if submitted.WorkCount != 1 || len(submitted.Works) != 1 || strings.TrimSpace(submitted.Works[0].WorkID) == "" {
		t.Fatalf("submit batch response missing accepted work identity: %#v", submitted)
	}
	workID := submitted.Works[0].WorkID

	assertWorkNotDispatchedViaCLI(t, ctx, binaryPath, factoryDir, baseURL, workID, sessionPauseWiringWorkName)

	resumeResponse := runSessionLifecycleCLIJSON(
		t, ctx, binaryPath, factoryDir, baseURL, "resume", factorysessions.DefaultSessionID,
	)
	if resumeResponse.Operation != factoryapi.FactorySessionLifecycleControlKindResume ||
		resumeResponse.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("session resume response = %#v, want accepted resume", resumeResponse)
	}
	if resumeResponse.Status != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("session resume status = %q, want RUNNING", resumeResponse.Status)
	}

	resumedSession := runSessionShowCLIJSON(t, ctx, binaryPath, factoryDir, baseURL, factorysessions.DefaultSessionID)
	if resumedSession.Runtime.LifecycleControlStatus == nil ||
		*resumedSession.Runtime.LifecycleControlStatus != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("session show after resume missing running lifecycle marker: %#v", resumedSession.Runtime)
	}

	waitForWorkStateViaCLI(t, ctx, binaryPath, factoryDir, baseURL, workID, "complete", 30*time.Second)
}

// TestCLISessionMissingIDReturnsNotFound proves you session show and delete against
// an unknown session ID exit non-success with actionable not-found diagnostics and
// no false success session payload through the public CLI wiring boundary.
func TestCLISessionMissingIDReturnsNotFound(t *testing.T) {
	factoryDir := support.ScaffoldFactory(t, sessionWiringFactoryConfig())
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:     factoryDir,
		UseMockWorkers: true,
	})
	defer server.Stop(t)

	baseURL := server.URL()
	serverPort := portFromServerURL(t, baseURL)
	binaryPath := buildYouCLIBinary(t)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	showOut, err := runYouCLI(ctx, binaryPath, factoryDir, baseURL,
		"--json",
		"session", "show", sessionWiringMissingSessionID,
	)
	assertCLISessionNotFoundFailure(t, "show", showOut, err, sessionWiringMissingSessionID, false)

	deleteOut, err := runYouCLI(ctx, binaryPath, factoryDir, "",
		"--json",
		"session", "delete", sessionWiringMissingSessionID,
		"--port", fmt.Sprintf("%d", serverPort),
	)
	assertCLISessionNotFoundFailure(t, "delete", deleteOut, err, sessionWiringMissingSessionID, true)
}

func sessionWiringFactoryConfig() map[string]any {
	return map[string]any{
		"name": "cli-session-wiring",
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

func sessionWiringListContains(
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

type sessionWiringBatchSubmitJSON struct {
	WorkCount int `json:"workCount"`
	Works     []struct {
		Name   string `json:"name"`
		WorkID string `json:"workId"`
	} `json:"works"`
}

func runSessionLifecycleCLIJSON(
	t *testing.T,
	ctx context.Context,
	binaryPath string,
	workingDir string,
	serverURL string,
	operation string,
	sessionID string,
) factoryapi.FactorySessionLifecycleControlResponse {
	t.Helper()

	out, err := runYouCLI(ctx, binaryPath, workingDir, serverURL,
		"--json",
		"session", operation, sessionID,
	)
	if err != nil {
		t.Fatalf("you session %s: %v\noutput:\n%s", operation, err, out)
	}
	var response factoryapi.FactorySessionLifecycleControlResponse
	if err := json.Unmarshal(bytesTrimSpace(out), &response); err != nil {
		t.Fatalf("decode session %s JSON: %v\noutput:\n%s", operation, err, out)
	}
	return response
}

func runSessionShowCLIJSON(
	t *testing.T,
	ctx context.Context,
	binaryPath string,
	workingDir string,
	serverURL string,
	sessionID string,
) factoryapi.FactorySession {
	t.Helper()

	out, err := runYouCLI(ctx, binaryPath, workingDir, serverURL,
		"--json",
		"session", "show", sessionID,
	)
	if err != nil {
		t.Fatalf("you session show: %v\noutput:\n%s", err, out)
	}
	var session factoryapi.FactorySession
	if err := json.Unmarshal(bytesTrimSpace(out), &session); err != nil {
		t.Fatalf("decode session show JSON: %v\noutput:\n%s", err, out)
	}
	return session
}

func runWorkShowCLIJSON(
	t *testing.T,
	ctx context.Context,
	binaryPath string,
	workingDir string,
	serverURL string,
	workID string,
) (factoryapi.Work, error) {
	t.Helper()

	out, err := runYouCLI(ctx, binaryPath, workingDir, serverURL,
		"--json",
		"work", "show", workID,
	)
	if err != nil {
		return factoryapi.Work{}, err
	}
	var work factoryapi.Work
	if err := json.Unmarshal(bytesTrimSpace(out), &work); err != nil {
		t.Fatalf("decode work show JSON: %v\noutput:\n%s", err, out)
	}
	return work, nil
}

func runWorkListCLIJSON(
	t *testing.T,
	ctx context.Context,
	binaryPath string,
	workingDir string,
	serverURL string,
	workName string,
) factoryapi.ListWorkResponse {
	t.Helper()

	args := []string{"--json", "work", "list"}
	if strings.TrimSpace(workName) != "" {
		args = append(args, "--name", workName)
	}
	out, err := runYouCLI(ctx, binaryPath, workingDir, serverURL, args...)
	if err != nil {
		t.Fatalf("you work list: %v\noutput:\n%s", err, out)
	}
	var listed factoryapi.ListWorkResponse
	if err := json.Unmarshal(bytesTrimSpace(out), &listed); err != nil {
		t.Fatalf("decode work list JSON: %v\noutput:\n%s", err, out)
	}
	return listed
}

func assertWorkNotDispatchedViaCLI(
	t *testing.T,
	ctx context.Context,
	binaryPath string,
	workingDir string,
	serverURL string,
	workID string,
	workName string,
) {
	t.Helper()

	if work, err := runWorkShowCLIJSON(t, ctx, binaryPath, workingDir, serverURL, workID); err == nil {
		if work.State != nil && work.State.Name == "complete" {
			t.Fatalf("work %q reached complete while session was paused: %#v", workID, work)
		}
	} else {
		exitErr, ok := err.(*exec.ExitError)
		if !ok || exitErr.ExitCode() == 0 {
			t.Fatalf("you work show %s while paused: %v", workID, err)
		}
	}

	listed := runWorkListCLIJSON(t, ctx, binaryPath, workingDir, serverURL, workName)
	if support.HasWorkAtCustomerState(listed, workID, support.WorkCustomerLocation("task", "complete")) {
		t.Fatalf("work %q reached task:complete before resume: %#v", workID, listed.Results)
	}
	if support.HasWorkAtCustomerState(listed, workID, support.WorkCustomerLocation("task", "init")) {
		t.Fatalf("work %q reached task:init while session was paused: %#v", workID, listed.Results)
	}
}

func waitForWorkStateViaCLI(
	t *testing.T,
	ctx context.Context,
	binaryPath string,
	workingDir string,
	serverURL string,
	workID string,
	wantState string,
	timeout time.Duration,
) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		work, err := runWorkShowCLIJSON(t, ctx, binaryPath, workingDir, serverURL, workID)
		if err == nil && work.State != nil && work.State.Name == wantState {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("wait for work %q state %q: %v", workID, wantState, ctx.Err())
		case <-time.After(200 * time.Millisecond):
		}
	}
	work, err := runWorkShowCLIJSON(t, ctx, binaryPath, workingDir, serverURL, workID)
	if err != nil {
		t.Fatalf("work %q missing after resume: %v", workID, err)
	}
	t.Fatalf("work %q state = %#v, want %q within %s", workID, work.State, wantState, timeout)
}
