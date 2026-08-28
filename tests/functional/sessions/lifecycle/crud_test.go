package lifecycle_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const sessionLifecycleCRUDMissingSessionID = "dur-sess-missing-999"

const lifecycleFixtureShutdownTimeout = 5 * time.Second

// sharedLifecycleFixture owns one package-local API server reused by every
// CRUD lifecycle cell and one reusable CLI client. All cells clean up the
// sessions they create, so public list/get observations show no unintended
// state across cells.
type sharedLifecycleFixture struct {
	rootDir          string
	baseURL          string
	factoryDir       string
	clientWorkingDir string
	homeDir          string
	cancel           context.CancelFunc
	done             chan error
	process          support.ApplicationProcess
	client           *lifecycleClientProcess
	api              *lifecycleHTTPServer
	constructor      *lifecycleProcessConstructor
}

var lifecycleFixture *sharedLifecycleFixture

func TestMain(m *testing.M) {
	fixture, err := startSharedLifecycleServer()
	if err != nil {
		fmt.Fprintf(os.Stderr, "start shared lifecycle API server: %v\n", err)
		os.Exit(1)
	}
	lifecycleFixture = fixture
	code := m.Run()
	if err := fixture.stop(); err != nil {
		fmt.Fprintf(os.Stderr, "stop shared lifecycle fixture: %v\n", err)
		if code == 0 {
			code = 1
		}
	}
	os.Exit(code)
}

func startSharedLifecycleServer() (*sharedLifecycleFixture, error) {
	ctx, cancel := context.WithCancel(context.Background())
	rootDir, err := os.MkdirTemp("", "session-lifecycle-shared-")
	if err != nil {
		cancel()
		return nil, fmt.Errorf("create shared fixture root: %w", err)
	}
	cleanupRoot := func() {
		_ = os.RemoveAll(rootDir)
	}
	homeDir := filepath.Join(rootDir, "home")
	factoryDir := filepath.Join(rootDir, "server-factory")
	clientWorkingDir := filepath.Join(rootDir, "client-working")
	for name, dir := range map[string]string{
		"home":           homeDir,
		"server factory": factoryDir,
		"client working": clientWorkingDir,
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			cleanupRoot()
			cancel()
			return nil, fmt.Errorf("create shared %s directory: %w", name, err)
		}
	}
	if err := writeLifecycleHostFactory(factoryDir); err != nil {
		cleanupRoot()
		cancel()
		return nil, fmt.Errorf("write shared server factory: %w", err)
	}
	if err := writeLifecycleFactory(clientWorkingDir); err != nil {
		cleanupRoot()
		cancel()
		return nil, fmt.Errorf("write shared client factory: %w", err)
	}
	if err := writeSharedRemoteLifecycleFactory(homeDir, factoryDir); err != nil {
		cleanupRoot()
		cancel()
		return nil, fmt.Errorf("write shared remote lifecycle factory: %w", err)
	}

	api := newLifecycleHTTPServer()
	constructor := &lifecycleProcessConstructor{}
	resolveHome := func() (string, error) { return homeDir, nil }
	process, err := constructor.build(ctx, "server", serviceedges.Edges{
		BrowserOpener:                      func(context.Context, string) error { return nil },
		APIServerStarter:                   api.start,
		FactorySessionResolveHomeDirectory: resolveHome,
		FactoryRuntimeWorkflowHome:         resolveHome,
	})
	if err != nil {
		stopFailedFixture(cancel, rootDir, process, nil)
		return nil, fmt.Errorf("build shared server root process: %w", err)
	}
	inputs := support.FakeInputs(ctx, []string{
		"you", "run", "--continuously", "--with-server", "--quiet", "--dir", factoryDir, "--no-record",
	})
	inputs.Input.Env = lifecycleEnvironment(homeDir)
	inputs.Input.WorkingDirectory = factoryDir
	done := make(chan error, 1)
	go func() { done <- process.Execute(inputs.Input) }()
	baseURL, err := api.waitForBaseURL(15 * time.Second)
	if err != nil {
		stopFailedFixture(cancel, rootDir, process, done)
		return nil, fmt.Errorf("wait for shared server base URL: %w", err)
	}
	clientProcess, err := constructor.build(context.Background(), "client", serviceedges.Edges{
		BrowserOpener:                      func(context.Context, string) error { return nil },
		FactorySessionResolveHomeDirectory: resolveHome,
		FactoryRuntimeWorkflowHome:         resolveHome,
	})
	if err != nil {
		stopFailedFixture(cancel, rootDir, process, done)
		return nil, fmt.Errorf("build shared client root process: %w", err)
	}
	return &sharedLifecycleFixture{
		rootDir:          rootDir,
		baseURL:          baseURL,
		factoryDir:       factoryDir,
		clientWorkingDir: clientWorkingDir,
		homeDir:          homeDir,
		cancel:           cancel,
		done:             done,
		process:          process,
		api:              api,
		constructor:      constructor,
		client: &lifecycleClientProcess{
			process: clientProcess,
			env:     lifecycleEnvironment(homeDir),
		},
	}, nil
}

func stopFailedFixture(
	cancel context.CancelFunc,
	rootDir string,
	process support.ApplicationProcess,
	done chan error,
) {
	cancel()
	_ = waitForLifecycleProcess(done)
	_ = closeLifecycleProcess(process)
	_ = os.RemoveAll(rootDir)
}

func (fixture *sharedLifecycleFixture) stop() error {
	if fixture == nil {
		return nil
	}
	var cleanupErrors []error
	fixture.cancel()
	if err := waitForLifecycleProcess(fixture.done); err != nil && !errors.Is(err, context.Canceled) {
		cleanupErrors = append(cleanupErrors, fmt.Errorf("wait for shared server: %w", err))
	}
	if err := closeLifecycleProcess(fixture.process); err != nil {
		cleanupErrors = append(cleanupErrors, fmt.Errorf("close shared server process: %w", err))
	}
	if fixture.client != nil {
		if err := closeLifecycleProcess(fixture.client.process); err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("close shared client process: %w", err))
		}
	}
	listenerClosed := false
	if fixture.api != nil {
		if err := fixture.api.waitClosed(); err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("wait for shared listener: %w", err))
		} else if err := fixture.api.probeClosed(); err != nil {
			cleanupErrors = append(cleanupErrors, err)
		} else {
			listenerClosed = true
		}
	}
	roles := []string(nil)
	if fixture.constructor != nil {
		roles = fixture.constructor.roles()
		if len(roles) != 2 {
			cleanupErrors = append(cleanupErrors,
				fmt.Errorf("shared lifecycle root-built process roles = %d (%v), want exactly 2", len(roles), roles),
			)
		}
	}
	listenerStarts := 0
	if fixture.api != nil {
		listenerStarts = fixture.api.startCount()
		if listenerStarts != 1 {
			cleanupErrors = append(cleanupErrors,
				fmt.Errorf("shared lifecycle HTTP listener starts = %d, want exactly 1", listenerStarts),
			)
		}
	}
	rootRemoved := false
	if err := os.RemoveAll(fixture.rootDir); err != nil {
		cleanupErrors = append(cleanupErrors, fmt.Errorf("remove shared fixture root: %w", err))
	} else if _, err := os.Stat(fixture.rootDir); !os.IsNotExist(err) {
		cleanupErrors = append(cleanupErrors, fmt.Errorf("shared fixture root %q remains after cleanup: %v", fixture.rootDir, err))
	} else {
		rootRemoved = true
	}
	fmt.Fprintf(
		os.Stderr,
		"LIFECYCLE-005 evidence: root-built-roles=%d roles=%v http-listener-starts=%d listener-closed=%t fixture-root-removed=%t\n",
		len(roles), roles, listenerStarts, listenerClosed, rootRemoved,
	)
	return errors.Join(cleanupErrors...)
}

func waitForLifecycleProcess(done chan error) error {
	if done == nil {
		return nil
	}
	timer := time.NewTimer(lifecycleFixtureShutdownTimeout)
	defer timer.Stop()
	select {
	case err := <-done:
		return err
	case <-timer.C:
		return fmt.Errorf("timed out after %s", lifecycleFixtureShutdownTimeout)
	}
}

func closeLifecycleProcess(process support.ApplicationProcess) error {
	if process == nil {
		return nil
	}
	closeCtx, cancelClose := context.WithTimeout(context.Background(), lifecycleFixtureShutdownTimeout)
	defer cancelClose()
	return process.Close(closeCtx)
}

func lifecycleEnvironment(homeDir string) []string {
	environment := append([]string(nil), os.Environ()...)
	return append(environment, "HOME="+homeDir, "USERPROFILE="+homeDir)
}

func sharedLifecycleServerURL(t *testing.T) string {
	t.Helper()
	if lifecycleFixture == nil {
		t.Fatal("shared lifecycle API server is unavailable")
	}
	return lifecycleFixture.baseURL
}

// TestFactorySessionCreateListShowDelete proves the public CLI Factory Session
// boundary supports a full create → list → show → terminate → delete lifecycle: a newly
// created session returns a stable public session ID, appears in session list
// and session show with matching folder identity and live runtime status markers,
// and session delete removes it so subsequent list/show no longer treat it as
// an open session participating in the owned runtime lifecycle.
// backendsizecheck:ignore-function pre-existing baseline debt recorded 2026-08-08; split this oversized code into focused units and remove this exemption
func TestFactorySessionCreateListShowDelete(t *testing.T) {
	baseURL := sharedLifecycleServerURL(t)
	client := sharedLifecycleClient(t)

	newFactoryDir := filepath.Join(t.TempDir(), "session-lifecycle-crud-factory")
	if err := os.Mkdir(newFactoryDir, 0o755); err != nil {
		t.Fatalf("create factory directory: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	createOut, err := client.executeCLI(ctx, lifecycleWorkingDir(t), baseURL,
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
	markSessionClean := registerLifecycleSessionCleanup(t, baseURL, sessionID)

	listOut, err := client.executeCLI(ctx, lifecycleWorkingDir(t), baseURL, "session", "list")
	if err != nil {
		t.Fatalf("you session list: %v\noutput:\n%s", err, listOut)
	}
	listHuman := string(listOut)
	for _, marker := range []string{sessionID, newFactoryDir} {
		if !strings.Contains(listHuman, marker) {
			t.Fatalf("session list output missing %q:\n%s", marker, listHuman)
		}
	}

	listJSONOut, err := client.executeCLI(ctx, lifecycleWorkingDir(t), baseURL,
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

	showOut, err := client.executeCLI(ctx, lifecycleWorkingDir(t), baseURL,
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

	terminateOut, err := client.executeCLI(ctx, lifecycleWorkingDir(t), baseURL,
		"--json",
		"session", "terminate", sessionID,
	)
	if err != nil {
		t.Fatalf("you session terminate: %v\noutput:\n%s", err, terminateOut)
	}

	deleteOut, err := client.executeCLI(ctx, lifecycleWorkingDir(t), baseURL,
		"--json",
		"session", "delete", sessionID,
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
	markSessionClean()

	showAfterDeleteOut, err := client.executeCLI(ctx, lifecycleWorkingDir(t), baseURL,
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

// TestFactorySessionListMultipleSessions proves the public CLI Factory Session
// boundary lists every open session as a distinct entry and preserves a sibling
// after one session is terminated and deleted.
func TestFactorySessionListMultipleSessions(t *testing.T) {
	baseURL := sharedLifecycleServerURL(t)
	client := sharedLifecycleClient(t)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	firstFactoryDir := filepath.Join(t.TempDir(), "session-lifecycle-crud-factory-a")
	if err := os.Mkdir(firstFactoryDir, 0o755); err != nil {
		t.Fatalf("create first factory directory: %v", err)
	}
	firstSession := createSessionViaCLI(t, ctx, client, lifecycleWorkingDir(t), baseURL, firstFactoryDir)
	markFirstSessionClean := registerLifecycleSessionCleanup(t, baseURL, firstSession.id)

	secondFactoryDir := filepath.Join(t.TempDir(), "session-lifecycle-crud-factory-b")
	if err := os.Mkdir(secondFactoryDir, 0o755); err != nil {
		t.Fatalf("create second factory directory: %v", err)
	}
	secondSession := createSessionViaCLI(t, ctx, client, lifecycleWorkingDir(t), baseURL, secondFactoryDir)
	registerLifecycleSessionCleanup(t, baseURL, secondSession.id)

	if firstSession.id == secondSession.id {
		t.Fatalf("session ids must be distinct: first=%q second=%q", firstSession.id, secondSession.id)
	}

	listOut, err := client.executeCLI(ctx, lifecycleWorkingDir(t), baseURL, "session", "list")
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

	listJSONOut, err := client.executeCLI(ctx, lifecycleWorkingDir(t), baseURL,
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

	deleteSessionViaCLI(t, ctx, client, lifecycleWorkingDir(t), baseURL, firstSession.id)
	markFirstSessionClean()
	assertCLISessionDeletionPreservesSurvivor(t, ctx, client, lifecycleWorkingDir(t), baseURL,
		firstSession, secondSession)
}

// TestFactorySessionMissingShowAndDeleteFail proves the public CLI Factory Session
// boundary rejects session show and delete for a missing session ID with a
// customer-visible not-found outcome, without removing or corrupting any still-open
// session created for isolation proof.
func TestFactorySessionMissingShowAndDeleteFail(t *testing.T) {
	baseURL := sharedLifecycleServerURL(t)
	client := sharedLifecycleClient(t)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	isolationFactoryDir := filepath.Join(t.TempDir(), "session-lifecycle-crud-isolation-factory")
	if err := os.Mkdir(isolationFactoryDir, 0o755); err != nil {
		t.Fatalf("create isolation factory directory: %v", err)
	}
	openSession := createSessionViaCLI(t, ctx, client, lifecycleWorkingDir(t), baseURL, isolationFactoryDir)
	t.Cleanup(func() { cleanupSessionViaAPI(t, baseURL, openSession.id) })

	showOut, err := client.executeCLI(ctx, lifecycleWorkingDir(t), baseURL,
		"--json",
		"session", "show", sessionLifecycleCRUDMissingSessionID,
	)
	assertCLISessionNotFoundFailure(t, "show", showOut, err, sessionLifecycleCRUDMissingSessionID, false)

	deleteOut, err := client.executeCLI(ctx, lifecycleWorkingDir(t), baseURL,
		"--json",
		"session", "delete", sessionLifecycleCRUDMissingSessionID,
	)
	assertCLISessionNotFoundFailure(t, "delete", deleteOut, err, sessionLifecycleCRUDMissingSessionID, true)

	showOut, err = client.executeCLI(ctx, lifecycleWorkingDir(t), baseURL,
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

	listJSONOut, err := client.executeCLI(ctx, lifecycleWorkingDir(t), baseURL,
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
	baseURL := sharedLifecycleServerURL(t)
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
	markSessionClean := registerLifecycleSessionCleanup(t, baseURL, sessionID)

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

	terminateSessionViaAPI(t, baseURL, sessionID)
	closeSessionViaAPI(t, baseURL, sessionID)
	markSessionClean()
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
	baseURL := sharedLifecycleServerURL(t)
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
	registerLifecycleSessionCleanup(t, baseURL, openSessionID)

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

// TestAPIMultipleFactorySessionsRemainIsolated proves the public HTTP Factory Session
// boundary keeps concurrently open sessions isolated: opening at least two live sessions
// lists both as distinct entries, and stopping then deleting one session leaves the other
// gettable and listable with its original public identity without adopting stopped state.
func TestAPIMultipleFactorySessionsRemainIsolated(t *testing.T) {
	baseURL := sharedLifecycleServerURL(t)

	firstFactoryDir := filepath.Join(t.TempDir(), "session-lifecycle-crud-api-factory-a")
	if err := os.Mkdir(firstFactoryDir, 0o755); err != nil {
		t.Fatalf("create first factory directory: %v", err)
	}
	firstSession := openSessionViaAPI(t, baseURL, firstFactoryDir)
	markFirstSessionClean := registerLifecycleSessionCleanup(t, baseURL, firstSession.id)

	secondFactoryDir := filepath.Join(t.TempDir(), "session-lifecycle-crud-api-factory-b")
	if err := os.Mkdir(secondFactoryDir, 0o755); err != nil {
		t.Fatalf("create second factory directory: %v", err)
	}
	secondSession := openSessionViaAPI(t, baseURL, secondFactoryDir)
	registerLifecycleSessionCleanup(t, baseURL, secondSession.id)

	if firstSession.id == secondSession.id {
		t.Fatalf("session ids must be distinct: first=%q second=%q", firstSession.id, secondSession.id)
	}

	listed := support.GetJSON[factoryapi.ListFactorySessionsResponse](t, baseURL+"/factory-sessions")
	if !sessionListContains(listed.Sessions, firstSession.id, firstFactoryDir) {
		t.Fatalf("list sessions missing first session %q at %q: %#v", firstSession.id, firstFactoryDir, listed.Sessions)
	}
	if !sessionListContains(listed.Sessions, secondSession.id, secondFactoryDir) {
		t.Fatalf("list sessions missing second session %q at %q: %#v", secondSession.id, secondFactoryDir, listed.Sessions)
	}
	assertDistinctSessionListEntries(t, listed.Sessions, firstSession.id, secondSession.id)

	terminateSessionViaAPI(t, baseURL, firstSession.id)
	closeSessionViaAPI(t, baseURL, firstSession.id)
	markFirstSessionClean()
	assertAPISessionNotFound(t, baseURL, firstSession.id)

	afterClose := support.GetJSON[factoryapi.ListFactorySessionsResponse](t, baseURL+"/factory-sessions")
	if sessionListContains(afterClose.Sessions, firstSession.id, firstFactoryDir) {
		t.Fatalf("list sessions after close still contains closed session %q at %q: %#v",
			firstSession.id, firstFactoryDir, afterClose.Sessions)
	}
	if !sessionListContains(afterClose.Sessions, secondSession.id, secondFactoryDir) {
		t.Fatalf("list sessions after close missing surviving session %q at %q: %#v",
			secondSession.id, secondFactoryDir, afterClose.Sessions)
	}

	selected := support.GetJSON[factoryapi.FactorySessionGetResponse](
		t,
		baseURL+"/factory-sessions/"+secondSession.id,
	)
	resolved, err := selected.AsFactorySession()
	if err != nil {
		t.Fatalf("decode surviving session get response: %v", err)
	}
	if resolved.Id != secondSession.id {
		t.Fatalf("surviving session get id = %q, want %q", resolved.Id, secondSession.id)
	}
	if resolved.Id == firstSession.id {
		t.Fatalf("surviving session must not adopt closed session id %q", firstSession.id)
	}
	if resolved.FolderPath != secondFactoryDir {
		t.Fatalf("surviving session folder path = %q, want %q", resolved.FolderPath, secondFactoryDir)
	}
	if resolved.FolderPath == firstFactoryDir {
		t.Fatalf("surviving session must not adopt closed session folder %q", firstFactoryDir)
	}
	if resolved.Runtime.Status == "" {
		t.Fatalf("surviving session missing runtime status markers: %#v", resolved)
	}
}

type apiOpenedSession struct {
	id         string
	folderPath string
}

func openSessionViaAPI(t *testing.T, baseURL, factoryDir string) apiOpenedSession {
	t.Helper()

	initNewFactory := true
	opened := postSessionLifecycleJSON[factoryapi.OpenFactorySessionResponse](
		t,
		baseURL+"/factory-sessions",
		factoryapi.OpenFactorySessionRequest{
			FolderPath:     factoryDir,
			InitNewFactory: &initNewFactory,
		},
		fmt.Sprintf("open Factory Session at %q", factoryDir),
	)
	if opened.Session == nil || strings.TrimSpace(opened.Session.Id) == "" {
		t.Fatalf("open session response for %q missing session id: %#v", factoryDir, opened)
	}
	if opened.Session.FolderPath != factoryDir {
		t.Fatalf("open session folder path for %q = %q, want %q", factoryDir, opened.Session.FolderPath, factoryDir)
	}
	return apiOpenedSession{
		id:         opened.Session.Id,
		folderPath: factoryDir,
	}
}

type cliCreatedSession struct {
	id         string
	folderPath string
}

func deleteSessionViaCLI(
	t *testing.T,
	ctx context.Context,
	client *lifecycleClientProcess,
	workingDir string,
	serverURL string,
	sessionID string,
) {
	t.Helper()

	terminateOut, err := client.executeCLI(ctx, workingDir, serverURL,
		"--json", "session", "terminate", sessionID)
	if err != nil {
		t.Fatalf("you session terminate for %q: %v\noutput:\n%s", sessionID, err, terminateOut)
	}
	deleteOut, err := client.executeCLI(ctx, workingDir, serverURL,
		"--json", "session", "delete", sessionID)
	if err != nil {
		t.Fatalf("you session delete for %q: %v\noutput:\n%s", sessionID, err, deleteOut)
	}
	var deleted struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(bytesTrimSpace(deleteOut), &deleted); err != nil {
		t.Fatalf("decode session delete JSON for %q: %v\noutput:\n%s", sessionID, err, deleteOut)
	}
	if deleted.SessionID != sessionID {
		t.Fatalf("session delete confirmation = %q, want %q", deleted.SessionID, sessionID)
	}
}

func assertCLISessionDeletionPreservesSurvivor(
	t *testing.T,
	ctx context.Context,
	client *lifecycleClientProcess,
	workingDir string,
	serverURL string,
	deletedSession cliCreatedSession,
	survivorSession cliCreatedSession,
) {
	t.Helper()

	showDeletedOut, err := client.executeCLI(ctx, workingDir, serverURL,
		"--json", "session", "show", deletedSession.id)
	if err == nil {
		t.Fatalf("you session show for deleted session unexpectedly succeeded:\n%s", showDeletedOut)
	}
	assertCLISessionNotFoundFailure(t, "show", showDeletedOut, err, deletedSession.id, false)

	afterDeleteOut, err := client.executeCLI(ctx, workingDir, serverURL,
		"--json", "session", "list")
	if err != nil {
		t.Fatalf("you session list after deleting %q: %v\noutput:\n%s", deletedSession.id, err, afterDeleteOut)
	}
	var afterDelete factoryapi.ListFactorySessionsResponse
	if err := json.Unmarshal(bytesTrimSpace(afterDeleteOut), &afterDelete); err != nil {
		t.Fatalf("decode session list after deleting %q: %v\noutput:\n%s", deletedSession.id, err, afterDeleteOut)
	}
	if sessionListContains(afterDelete.Sessions, deletedSession.id, deletedSession.folderPath) {
		t.Fatalf("session list after delete still contains deleted session %q at %q: %#v",
			deletedSession.id, deletedSession.folderPath, afterDelete.Sessions)
	}
	if !sessionListContains(afterDelete.Sessions, survivorSession.id, survivorSession.folderPath) {
		t.Fatalf("session list after deleting %q missing survivor %q at %q: %#v",
			deletedSession.id, survivorSession.id, survivorSession.folderPath, afterDelete.Sessions)
	}
	assertSessionListEntryExactlyOnce(t, afterDelete.Sessions, survivorSession.id)

	showSurvivorOut, err := client.executeCLI(ctx, workingDir, serverURL,
		"--json", "session", "show", survivorSession.id)
	if err != nil {
		t.Fatalf("you session show for surviving session: %v\noutput:\n%s", err, showSurvivorOut)
	}
	var survivor factoryapi.FactorySession
	if err := json.Unmarshal(bytesTrimSpace(showSurvivorOut), &survivor); err != nil {
		t.Fatalf("decode surviving session show JSON: %v\noutput:\n%s", err, showSurvivorOut)
	}
	if survivor.Id != survivorSession.id || survivor.FolderPath != survivorSession.folderPath {
		t.Fatalf("surviving session identity = %q at %q, want %q at %q",
			survivor.Id, survivor.FolderPath, survivorSession.id, survivorSession.folderPath)
	}
	if survivor.Runtime.Status == "" {
		t.Fatalf("surviving session missing runtime status markers: %#v", survivor)
	}
}

func createSessionViaCLI(
	t *testing.T,
	ctx context.Context,
	client *lifecycleClientProcess,
	workingDir string,
	serverURL string,
	factoryDir string,
) cliCreatedSession {
	t.Helper()

	createOut, err := client.executeCLI(ctx, workingDir, serverURL,
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

func assertSessionListEntryExactlyOnce(
	t *testing.T,
	sessions []factoryapi.FactorySessionSummary,
	sessionID string,
) {
	t.Helper()

	count := 0
	for _, session := range sessions {
		if session.Id == sessionID {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("session list JSON has %d entries for session %q, want exactly 1: %#v", count, sessionID, sessions)
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
