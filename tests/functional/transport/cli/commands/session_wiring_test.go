package commands_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
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
// backendsizecheck:ignore-function pre-existing baseline debt recorded 2026-08-08; split this oversized code into focused units and remove this exemption
func testCLISessionCreateListShowDelete(t *testing.T, remote *sharedRemoteCLI) {
	primaryFactoryDir := remote.hostFactoryDir
	processHarness := remote.process
	baseURL := remote.baseURL

	newFactoryDir := filepath.Join(t.TempDir(), "cli-session-wiring-factory")
	if err := os.Mkdir(newFactoryDir, 0o755); err != nil {
		t.Fatalf("create factory directory: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	createOut, err := runYouCLI(ctx, processHarness, primaryFactoryDir, baseURL,
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
	sessionDeleted := false
	t.Cleanup(func() {
		if !sessionDeleted {
			remote.closeSession(t, primaryFactoryDir, sessionID)
		}
	})

	listOut, err := runYouCLI(ctx, processHarness, primaryFactoryDir, baseURL, "session", "list")
	if err != nil {
		t.Fatalf("you session list: %v\noutput:\n%s", err, listOut)
	}
	listHuman := string(listOut)
	for _, marker := range []string{sessionID, newFactoryDir} {
		if !strings.Contains(listHuman, marker) {
			t.Fatalf("session list output missing %q:\n%s", marker, listHuman)
		}
	}

	listJSONOut, err := runYouCLI(ctx, processHarness, primaryFactoryDir, baseURL,
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

	showOut, err := runYouCLI(ctx, processHarness, primaryFactoryDir, baseURL,
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

	terminateResponse := runSessionLifecycleCLIJSON(
		t, ctx, processHarness, primaryFactoryDir, baseURL, "terminate", sessionID,
	)
	if terminateResponse.Operation != factoryapi.FactorySessionLifecycleControlKindTerminate ||
		terminateResponse.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("session terminate response = %#v, want accepted terminate", terminateResponse)
	}

	deleteOut, err := runYouCLI(ctx, processHarness, primaryFactoryDir, baseURL,
		"--json",
		"session", "delete", sessionID,
	)
	if err != nil {
		t.Fatalf("you session delete: %v\noutput:\n%s", err, deleteOut)
	}
	var deletedResponse struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(bytesTrimSpace(deleteOut), &deletedResponse); err != nil {
		t.Fatalf("decode session delete JSON: %v\noutput:\n%s", err, deleteOut)
	}
	if deletedResponse.SessionID != sessionID {
		t.Fatalf("session delete confirmation = %q, want %q", deletedResponse.SessionID, sessionID)
	}
	sessionDeleted = true

	showAfterDeleteOut, err := runYouCLI(ctx, processHarness, primaryFactoryDir, baseURL,
		"--json",
		"session", "show", sessionID,
	)
	if err == nil {
		t.Fatalf("you session show after delete unexpectedly succeeded:\n%s", showAfterDeleteOut)
	}
	showAfterDelete := string(showAfterDeleteOut)
	support.RequireNotFoundCLIDiagnostic(t, showAfterDelete)
	if strings.Contains(showAfterDelete, sessionID) && strings.Contains(showAfterDelete, `"id"`) {
		t.Fatalf("session show after delete must not emit a success session payload:\n%s", showAfterDelete)
	}
}

// testCLISessionListUsesIsolatedRecordingHome keeps the real CLI process
// pointed at a host-like home containing one malformed dated recording
// artifact, while the server's composed recording inventory uses its own
// package-owned home. The public history-only response proves that the
// external home is not consulted.
func testCLISessionListUsesIsolatedRecordingHome(t *testing.T, remote *sharedRemoteCLI) {
	t.Helper()

	const artifactReference = "2026/08/28/c07-external-home-malformed.json"
	externalHome := t.TempDir()
	isolatedRecordingHome := t.TempDir()
	artifactPath := filepath.Join(
		externalHome,
		".you-agent-factory",
		"recordings",
		filepath.FromSlash(artifactReference),
	)
	if err := os.MkdirAll(filepath.Dir(artifactPath), 0o755); err != nil {
		t.Fatalf("create external recording directory: %v", err)
	}
	const malformedArtifact = `{"c07":"malformed recording"}`
	if err := os.WriteFile(artifactPath, []byte(malformedArtifact), 0o600); err != nil {
		t.Fatalf("write malformed external recording artifact: %v", err)
	}
	// Keep the CLI process on the same path a real operator invocation uses,
	// while t.Setenv restores both variables after this serialized scenario.
	t.Setenv("HOME", externalHome)
	t.Setenv("USERPROFILE", externalHome)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	characterizationFactoryDir := support.ScaffoldFactory(t, sessionHistoryOnlyFactoryConfig())
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                characterizationFactoryDir,
		WaitForServiceModeRuntime: true,
		Edges: serviceedges.Edges{
			// Bind the durable listing to test-owned state at root composition;
			// invocation-local environment must not select an operator home.
			FactorySessionResolveHomeDirectory: func() (string, error) { return isolatedRecordingHome, nil },
		},
	})
	defer server.Stop(t)

	command := remote.process.CommandContext(ctx,
		"--server", server.URL(),
		"--json", "session", "list", "--history-only",
	)
	command.Dir = characterizationFactoryDir
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("you session list consulted malformed external-home recording artifact: %v\n%s", err, output)
	}

	var listed factoryapi.ListFactorySessionsResponse
	if err := json.Unmarshal(bytesTrimSpace(output), &listed); err != nil {
		t.Fatalf("decode isolated history-only session list JSON: %v\noutput:\n%s", err, output)
	}
	if listed.Scope == nil || string(*listed.Scope) != "history" {
		t.Fatalf("history-only session list scope = %#v, want history", listed.Scope)
	}
	if len(listed.Sessions) != 0 {
		t.Fatalf("history-only session list returned live sessions: %#v", listed.Sessions)
	}
	if listed.RecordedSessions != nil && len(*listed.RecordedSessions) != 0 {
		t.Fatalf("history-only session list consulted external recording state: %#v", listed.RecordedSessions)
	}
	if strings.Contains(string(output), artifactReference) || strings.Contains(string(output), malformedArtifact) {
		t.Fatalf("history-only session list leaked external recording state:\n%s", output)
	}
	t.Logf("CLI history-only listing used isolated recording home %q and ignored external home %q containing %q", isolatedRecordingHome, externalHome, artifactReference)
}

// TestCLISessionPauseBuffersAndResumeDispatches proves you session pause keeps
// accepted work buffered and you session resume restores dispatch through the
// public CLI against a running Factory Session server.
func testCLISessionPauseBuffersAndResumeDispatches(t *testing.T, remote *sharedRemoteCLI) {
	factoryDir := support.ScaffoldFactory(t, sessionWiringFactoryConfig())
	sessionID := remote.openSession(t, factoryDir)
	baseURL := remote.baseURL
	processHarness := remote.process

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	pauseResponse := runSessionLifecycleCLIJSON(
		t, ctx, processHarness, factoryDir, baseURL, "pause", sessionID,
	)
	if pauseResponse.Operation != factoryapi.FactorySessionLifecycleControlKindPause ||
		pauseResponse.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("session pause response = %#v, want accepted pause", pauseResponse)
	}
	if pauseResponse.Status != factoryapi.FactorySessionDurableLifecycleStatusPaused {
		t.Fatalf("session pause status = %q, want PAUSED", pauseResponse.Status)
	}

	pausedSession := runSessionShowCLIJSON(t, ctx, processHarness, factoryDir, baseURL, sessionID)
	if pausedSession.Runtime.LifecycleControlStatus == nil ||
		*pausedSession.Runtime.LifecycleControlStatus != factoryapi.FactorySessionDurableLifecycleStatusPaused {
		t.Fatalf("session show after pause missing paused lifecycle marker: %#v", pausedSession.Runtime)
	}

	inlineBatch := fmt.Sprintf(
		`{"requestId":%q,"type":"FACTORY_REQUEST_BATCH","works":[{"name":%q,"workTypeName":"task","payload":{"title":"Paused session wiring"}}]}`,
		sessionPauseWiringRequestID,
		sessionPauseWiringWorkName,
	)
	submitOut, err := remote.run(ctx, factoryDir, sessionID,
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

	assertWorkNotDispatchedViaCLI(t, ctx, processHarness, factoryDir, baseURL, sessionID, workID, sessionPauseWiringWorkName)

	resumeResponse := runSessionLifecycleCLIJSON(
		t, ctx, processHarness, factoryDir, baseURL, "resume", sessionID,
	)
	if resumeResponse.Operation != factoryapi.FactorySessionLifecycleControlKindResume ||
		resumeResponse.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("session resume response = %#v, want accepted resume", resumeResponse)
	}
	if resumeResponse.Status != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("session resume status = %q, want RUNNING", resumeResponse.Status)
	}

	resumedSession := runSessionShowCLIJSON(t, ctx, processHarness, factoryDir, baseURL, sessionID)
	if resumedSession.Runtime.LifecycleControlStatus == nil ||
		*resumedSession.Runtime.LifecycleControlStatus != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("session show after resume missing running lifecycle marker: %#v", resumedSession.Runtime)
	}

	waitForWorkStateViaCLI(t, ctx, processHarness, factoryDir, baseURL, sessionID, workID, "complete", 30*time.Second)
}

// TestCLISessionMissingIDReturnsNotFound proves you session show and delete against
// an unknown session ID exit non-success with actionable not-found diagnostics and
// no false success session payload through the public CLI wiring boundary.
func testCLISessionMissingIDReturnsNotFound(t *testing.T, remote *sharedRemoteCLI) {
	factoryDir := remote.hostFactoryDir
	baseURL := remote.baseURL
	processHarness := remote.process

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	showOut, err := runYouCLI(ctx, processHarness, factoryDir, baseURL,
		"--json",
		"session", "show", sessionWiringMissingSessionID,
	)
	assertCLISessionNotFoundFailure(t, "show", showOut, err, sessionWiringMissingSessionID, false)

	deleteOut, err := runYouCLI(ctx, processHarness, factoryDir, baseURL,
		"--json",
		"session", "delete", sessionWiringMissingSessionID,
	)
	assertCLISessionNotFoundFailure(t, "delete", deleteOut, err, sessionWiringMissingSessionID, true)

	for _, operation := range []string{"cancel", "terminate"} {
		controlOut, controlErr := runYouCLI(ctx, processHarness, factoryDir, baseURL,
			"--remote", "--json", "session", operation, sessionWiringMissingSessionID,
		)
		assertCLISessionNotFoundFailure(t, operation, controlOut, controlErr, sessionWiringMissingSessionID, false)
	}
	remote.assertHealthy(t, factoryDir)
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
	processHarness *commandRuntime,
	workingDir string,
	serverURL string,
	args ...string,
) ([]byte, error) {
	cmdArgs := []string{}
	if strings.TrimSpace(serverURL) != "" {
		cmdArgs = append(cmdArgs, "--server", serverURL)
	}
	cmdArgs = append(cmdArgs, args...)
	cmd, cleanup, err := newEphemeralCommandForScenario(processHarness, ctx, workingDir, cmdArgs...)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	return cmd.CombinedOutput()
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
	processHarness *commandRuntime,
	workingDir string,
	serverURL string,
	operation string,
	sessionID string,
) factoryapi.FactorySessionLifecycleControlResponse {
	t.Helper()

	out, err := runYouCLI(ctx, processHarness, workingDir, serverURL,
		"--remote",
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
	processHarness *commandRuntime,
	workingDir string,
	serverURL string,
	sessionID string,
) factoryapi.FactorySession {
	t.Helper()

	out, err := runYouCLI(ctx, processHarness, workingDir, serverURL,
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
	processHarness *commandRuntime,
	workingDir string,
	serverURL string,
	sessionID string,
	workID string,
) (factoryapi.Work, error) {
	t.Helper()

	out, err := runYouCLI(ctx, processHarness, workingDir, serverURL,
		"--json",
		"work", "show", workID,
		"--session", sessionID,
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
	processHarness *commandRuntime,
	workingDir string,
	serverURL string,
	sessionID string,
	workName string,
) factoryapi.ListWorkResponse {
	t.Helper()

	args := []string{"--json", "work", "list"}
	if strings.TrimSpace(workName) != "" {
		args = append(args, "--name", workName)
	}
	args = append(args, "--session", sessionID)
	out, err := runYouCLI(ctx, processHarness, workingDir, serverURL, args...)
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
	processHarness *commandRuntime,
	workingDir string,
	serverURL string,
	sessionID string,
	workID string,
	workName string,
) {
	t.Helper()

	if work, err := runWorkShowCLIJSON(t, ctx, processHarness, workingDir, serverURL, sessionID, workID); err == nil {
		if work.State != nil && work.State.Name == "complete" {
			t.Fatalf("work %q reached complete while session was paused: %#v", workID, work)
		}
	}

	listed := runWorkListCLIJSON(t, ctx, processHarness, workingDir, serverURL, sessionID, workName)
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
	processHarness *commandRuntime,
	workingDir string,
	serverURL string,
	sessionID string,
	workID string,
	wantState string,
	timeout time.Duration,
) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		work, err := runWorkShowCLIJSON(t, ctx, processHarness, workingDir, serverURL, sessionID, workID)
		if err == nil && work.State != nil && work.State.Name == wantState {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("wait for work %q state %q: %v", workID, wantState, ctx.Err())
		case <-time.After(200 * time.Millisecond):
		}
	}
	work, err := runWorkShowCLIJSON(t, ctx, processHarness, workingDir, serverURL, sessionID, workID)
	if err != nil {
		t.Fatalf("work %q missing after resume: %v", workID, err)
	}
	t.Fatalf("work %q state = %#v, want %q within %s", workID, work.State, wantState, timeout)
}
