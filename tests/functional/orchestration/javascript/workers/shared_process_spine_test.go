package workers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	sharedJavaScriptSuccessPrompt = "shared process spine permission success"
	sharedJavaScriptSuccessOutput = "shared process spine success output"

	sharedJavaScriptPermissionOmittedFactory = "shared-javascript-permissions-omitted"
	sharedJavaScriptPermissionDefaultFactory = "shared-javascript-permissions-default"
	sharedJavaScriptPermissionSkipFactory    = "shared-javascript-permissions-skip"
	sharedJavaScriptDisallowedFactory        = "named-factory"

	sharedJavaScriptSentinelCredentialName  = "SHARED_JAVASCRIPT_SENTINEL_CREDENTIAL"
	sharedJavaScriptSentinelCredentialValue = "sentinel-not-for-customer-process"
)

// TestJavaScriptSharedWorkerBehavior is the lexical owner for the complete
// JavaScript worker behavior lane. Every child uses this one root-built
// process and its one continuous API listener; only the external provider
// command outcome is selected per request.
func TestJavaScriptSharedWorkerBehavior(t *testing.T) {
	t.Setenv(
		sharedJavaScriptSentinelCredentialName,
		sharedJavaScriptSentinelCredentialValue,
	)
	fixture := newJavaScriptSharedProcessFixture(t)
	// API-owned cells do not carry an invocation environment and intentionally
	// exercise the host default. Remove the planted credential before those
	// cells run, after the shared customer environment has been constructed and
	// verified as the CLI boundary under review.
	if environmentContainsName(fixture.environment, sharedJavaScriptSentinelCredentialName) {
		t.Fatalf("customer environment propagated sentinel credential %q", sharedJavaScriptSentinelCredentialName)
	}
	if err := os.Unsetenv(sharedJavaScriptSentinelCredentialName); err != nil {
		t.Fatalf("remove shared JavaScript sentinel credential: %v", err)
	}

	tests := []struct {
		name string
		run  func(*testing.T, *javascriptSharedProcessFixture)
	}{
		{"spine/permission-matrix-cli-success", runJavaScriptSharedSuccess},
		{"spine/invalid-permission-pre-dispatch-failure", runJavaScriptSharedFailure},
		{"spine/reverse-order", runJavaScriptSharedReverseOrder},
		{"permissions/command-shaping", runJavaScriptPermissionMatrixCharacterization},
		{"permissions/disallowed", runJavaScriptDisallowedPermission},
		{"antigravity/model-embedded-effort", runJavaScriptAntigravitySuccess},
		{"antigravity/typed-rejection", runJavaScriptAntigravityRejection},
		{"providers/live-command-edge", runJavaScriptLiveProvider},
		{"providers/permission-flags", runJavaScriptProviderPermissionFlags},
		{"providers/invalid-permissions", runJavaScriptInvalidPermissions},
		{"providers/distinct-provider-model", runJavaScriptDistinctProviderModels},
		{"mock-workers/partial-passthrough", runJavaScriptPartialMockWorkers},
		{"overrides/unknown-provider", runJavaScriptUnknownProviderOverride},
		{"isolation/concurrent-success-failure", runJavaScriptConcurrentIsolation},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) { test.run(t, fixture) })
	}
	assertJavaScriptSharedCustomerEnvironmentSanitized(t, fixture)
}

func runJavaScriptSharedReverseOrder(t *testing.T, fixture *javascriptSharedProcessFixture) {
	runJavaScriptSharedFailureWithPrompt(t, fixture, "shared invalid permissions reverse")
	runJavaScriptSharedSuccessWithPrompt(t, fixture, "shared process spine permission success reverse")
}

func runJavaScriptSharedSuccess(t *testing.T, fixture *javascriptSharedProcessFixture) {
	runJavaScriptSharedSuccessWithPrompt(t, fixture, sharedJavaScriptSuccessPrompt)
}

func runJavaScriptSharedSuccessWithPrompt(t *testing.T, fixture *javascriptSharedProcessFixture, prompt string) {
	t.Helper()
	runner := support.NewRecordingCommandRunner(sharedJavaScriptSuccessOutput)
	if err := fixture.router.register(sharedJavaScriptSuccessPrompt, runner); err != nil {
		t.Fatalf("register success route: %v", err)
	}
	t.Cleanup(func() {
		if err := fixture.router.unregister(sharedJavaScriptSuccessPrompt); err != nil {
			t.Errorf("unregister success route: %v", err)
		}
	})

	before := fixture.persistentSessionIDs(t)
	beforeRecords := fixture.router.callCount()
	inputs, err := fixture.executeRemote(t, fixture.successFactoryName, prompt)
	if err != nil {
		t.Fatalf("shared remote success Process.Execute() error = %v\nstdout:\n%s\nstderr:\n%s", err, inputs.Stdout(), inputs.Stderr())
	}
	assertSharedRemoteCommandPlacement(t, inputs.Input.Args, fixture.baseURL)
	if !strings.Contains(inputs.Stdout(), sharedJavaScriptSuccessOutput) {
		t.Fatalf("shared remote success output = %q, want %q", inputs.Stdout(), sharedJavaScriptSuccessOutput)
	}
	if runner.CallCount() != 1 {
		t.Fatalf("owning Codex runner calls = %d, want one", runner.CallCount())
	}
	requests := fixture.router.requestRecords()
	if len(requests) != beforeRecords+1 {
		t.Fatalf("shared provider requests = %d, want %d; requests=%#v", len(requests), beforeRecords+1, requests)
	}
	request := requests[len(requests)-1]
	if request.Command != "codex" || !reflect.DeepEqual(request.Args, []string{"exec", "--json", "-"}) {
		t.Fatalf("shared provider command = %q %#v, want codex %#v", request.Command, request.Args, []string{"exec", "--json", "-"})
	}
	if !bytes.Contains(request.Stdin, []byte(sharedJavaScriptSuccessPrompt)) {
		t.Fatalf("shared provider stdin = %q, want owning request selector %q", request.Stdin, sharedJavaScriptSuccessPrompt)
	}
	if strings.TrimSpace(request.WorkDir) == "" {
		t.Fatal("shared provider request has empty working directory")
	}
	after := fixture.persistentSessionIDs(t)
	newSessions := differenceJavaScriptSessionIDs(before, after)
	if len(newSessions) != 1 || strings.TrimSpace(newSessions[0]) == "" {
		t.Fatalf("persisted session IDs before=%v after=%v new=%v, want one unique host-owned session", before, after, newSessions)
	}
	assertJavaScriptSharedCompletedDispatch(t, fixture, newSessions[0], "codex", "", "permission-matrix-child")
	fixture.trackSession(t, newSessions[0])
	t.Logf("ROUTE-001 success: root_process_builds=1 api_server_starts=%d request_records=%d session_id=%s command=%s args=%v", fixture.apiStarter.starts.Load(), len(requests), newSessions[0], request.Command, request.Args)
}

func runJavaScriptSharedFailure(t *testing.T, fixture *javascriptSharedProcessFixture) {
	runJavaScriptSharedFailureWithPrompt(t, fixture, "shared invalid permissions")
}

func runJavaScriptSharedFailureWithPrompt(t *testing.T, fixture *javascriptSharedProcessFixture, prompt string) {
	t.Helper()
	beforeCalls := fixture.router.callCount()
	beforeSessions := fixture.persistentSessionIDs(t)
	inputs, err := fixture.executeRemote(t, fixture.failureFactoryName, prompt)
	if err == nil {
		t.Fatalf("shared remote invalid-permission Process.Execute() error = nil\nstdout:\n%s\nstderr:\n%s", inputs.Stdout(), inputs.Stderr())
	}
	assertSharedRemoteCommandPlacement(t, inputs.Input.Args, fixture.baseURL)
	var response factoryapi.InvocationResponse
	if decodeErr := json.Unmarshal([]byte(strings.TrimSpace(inputs.Stdout())), &response); decodeErr != nil {
		t.Fatalf("decode shared invalid-permission invocation response: %v; stdout=%q", decodeErr, inputs.Stdout())
	}
	if response.SessionId == nil || strings.TrimSpace(*response.SessionId) == "" {
		t.Fatalf("shared invalid-permission response = %#v, want failed session identity", response)
	}
	session := readJavaScriptSharedDurableSession(t, fixture.baseURL, *response.SessionId)
	if session.FailureDetail == nil {
		t.Fatalf("shared invalid-permission durable session = %#v, want failure detail", session)
	}
	diagnostic := strings.ToLower(strings.Join([]string{err.Error(), inputs.Stdout(), inputs.Stderr(), session.FailureDetail.Message}, "\n"))
	if !strings.Contains(diagnostic, "permissions") {
		t.Fatalf("shared invalid-permission diagnostic = %q, want permissions detail", diagnostic)
	}
	if strings.Contains(diagnostic, strings.ToLower(sharedJavaScriptSuccessOutput)) {
		t.Fatalf("shared invalid-permission diagnostic cross-observed success output: %q", diagnostic)
	}
	if got := fixture.router.callCount(); got != beforeCalls {
		t.Fatalf("provider command calls after invalid permission = %d, want unchanged %d", got, beforeCalls)
	}
	after := fixture.persistentSessionIDs(t)
	if duplicate := duplicateJavaScriptSessionID(after); duplicate != "" {
		t.Fatalf("persisted session ID %q was reused across shared cells: %v", duplicate, after)
	}
	if !containsJavaScriptSessionID(after, *response.SessionId) {
		t.Fatalf("persisted session IDs after=%v omit failed session %q", after, *response.SessionId)
	}
	newSessions := differenceJavaScriptSessionIDs(beforeSessions, after)
	if len(newSessions) != 1 || newSessions[0] != *response.SessionId {
		t.Fatalf("persisted session IDs before=%v after=%v new=%v, want exactly the failed cell's unique session", beforeSessions, after, newSessions)
	}
	assertJavaScriptSharedNoDispatch(t, fixture, *response.SessionId)
	fixture.trackSession(t, *response.SessionId)
	t.Logf("ROUTE-001 failure: request_records=%d provider_calls=%d persisted_session_ids=%v diagnostic_contains_permissions=true", fixture.router.callCount(), fixture.router.callCount(), after)
}

type javascriptSharedProcessFixture struct {
	process            support.ApplicationProcess
	router             *javascriptSharedCommandRouter
	apiStarter         *javascriptSharedAPIServerStarter
	environment        []string
	baseURL            string
	hostDir            string
	successFactoryName string
	failureFactoryName string
	disallowedFactory  string
	requestSequence    atomic.Uint64
	sessionMu          sync.Mutex
	trackedSessions    map[string]struct{}
	closedSessions     map[string]struct{}
}

type javascriptSharedAPIServerStarter struct {
	api    *support.ProcessAPIServer
	starts atomic.Int32
}

func (starter *javascriptSharedAPIServerStarter) Start(
	ctx context.Context,
	request platformhttpserver.StartRequest,
) error {
	starter.starts.Add(1)
	return starter.api.Start(ctx, request)
}

// javascriptSharedCommandRouter is the only mutable part of the fixture. Its
// route registry is keyed by a request-content selector and rejects zero or
// ambiguous matches, so a sibling invocation cannot receive another cell's
// provider result accidentally.
type javascriptSharedCommandRouter struct {
	mu         sync.Mutex
	routes     map[string]platformprocess.CommandRunner
	requestLog []platformprocess.CommandRequest
	calls      chan int
}

func newJavaScriptSharedCommandRouter() *javascriptSharedCommandRouter {
	return &javascriptSharedCommandRouter{
		routes: make(map[string]platformprocess.CommandRunner),
		calls:  make(chan int, 64),
	}
}

func (router *javascriptSharedCommandRouter) register(
	selector string,
	runner platformprocess.CommandRunner,
) error {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return fmt.Errorf("JavaScript route selector is required")
	}
	if runner == nil {
		return fmt.Errorf("JavaScript route %q has no command runner", selector)
	}
	router.mu.Lock()
	defer router.mu.Unlock()
	if _, exists := router.routes[selector]; exists {
		return fmt.Errorf("JavaScript route %q is already registered", selector)
	}
	router.routes[selector] = runner
	return nil
}

func (router *javascriptSharedCommandRouter) unregister(selector string) error {
	selector = strings.TrimSpace(selector)
	router.mu.Lock()
	defer router.mu.Unlock()
	if _, exists := router.routes[selector]; !exists {
		return fmt.Errorf("JavaScript route %q is not registered", selector)
	}
	delete(router.routes, selector)
	return nil
}

func (router *javascriptSharedCommandRouter) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	router.mu.Lock()
	router.requestLog = append(router.requestLog, cloneJavaScriptCommandRequest(request))
	requestCount := len(router.requestLog)
	matched := make([]platformprocess.CommandRunner, 0, 1)
	requestContent := append([]byte(nil), request.Stdin...)
	requestContent = append(requestContent, []byte("\n")...)
	requestContent = append(requestContent, []byte(strings.Join(request.Args, " "))...)
	for selector, runner := range router.routes {
		if bytes.Contains(requestContent, []byte(selector)) {
			matched = append(matched, runner)
		}
	}
	router.mu.Unlock()
	select {
	case router.calls <- requestCount:
	default:
	}

	switch len(matched) {
	case 0:
		return platformprocess.CommandResult{}, fmt.Errorf("JavaScript request did not match an immutable provider route")
	case 1:
		return matched[0].Run(ctx, request)
	default:
		return platformprocess.CommandResult{}, fmt.Errorf("JavaScript request matched %d provider routes; want exactly one", len(matched))
	}
}

func (router *javascriptSharedCommandRouter) requestRecords() []platformprocess.CommandRequest {
	router.mu.Lock()
	defer router.mu.Unlock()
	requests := make([]platformprocess.CommandRequest, len(router.requestLog))
	for index := range router.requestLog {
		requests[index] = cloneJavaScriptCommandRequest(router.requestLog[index])
	}
	return requests
}

func (router *javascriptSharedCommandRouter) callCount() int {
	router.mu.Lock()
	defer router.mu.Unlock()
	return len(router.requestLog)
}

func (router *javascriptSharedCommandRouter) waitForCall(ctx context.Context, want int) error {
	// This is an event-driven barrier for the concurrent behavior test. The
	// provider command edge is the only deterministic observation that proves
	// the success Process.Execute call is in flight before the failure call is
	// released; a sleep or mocked API response would skip the shared
	// Process/HTTP/session path under test. The context is only a fail-fast bound
	// for a missing request or a deadlocked server.
	if router.callCount() >= want {
		return nil
	}
	for {
		select {
		case requestCount := <-router.calls:
			if requestCount >= want {
				return nil
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (router *javascriptSharedCommandRouter) routeCount() int {
	router.mu.Lock()
	defer router.mu.Unlock()
	return len(router.routes)
}

func newJavaScriptSharedProcessFixture(t *testing.T) *javascriptSharedProcessFixture {
	t.Helper()
	rootDir := t.TempDir()
	homeDir := filepath.Join(rootDir, "home")
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		t.Fatalf("create shared JavaScript worker home: %v", err)
	}
	hostDir := support.ScaffoldFactory(t, permissionMatrixFactoryConfig(
		permissionMatrixWorkflowWithPrompt("DEFAULT", "shared process spine host"),
	))
	successFactoryName := "shared-javascript-success"
	failureFactoryName := "shared-javascript-invalid"
	disallowedFactory := sharedJavaScriptDisallowedFactory
	successConfig := permissionMatrixFactoryConfig(
		permissionMatrixWorkflowWithPrompt("DEFAULT", sharedJavaScriptSuccessPrompt),
	)
	successConfig["name"] = successFactoryName
	failureConfig := permissionsOverrideFactoryConfig(invalidPermissionsOverrideWorkflow())
	failureConfig["name"] = failureFactoryName
	successSourceDir := support.ScaffoldFactory(t, successConfig)
	failureSourceDir := support.ScaffoldFactory(t, failureConfig)
	factoryDirs := map[string]string{
		sharedJavaScriptPermissionOmittedFactory: support.ScaffoldFactory(t, permissionMatrixFactoryConfig(
			permissionMatrixWorkflowWithPrompt("omitted", "shared permissions omitted"),
		)),
		sharedJavaScriptPermissionDefaultFactory: support.ScaffoldFactory(t, permissionMatrixFactoryConfig(
			permissionMatrixWorkflowWithPrompt("DEFAULT", "shared permissions default"),
		)),
		sharedJavaScriptPermissionSkipFactory: support.ScaffoldFactory(t, permissionMatrixFactoryConfig(
			permissionMatrixWorkflowWithPrompt("SKIP_PERMISSIONS", "shared permissions skip"),
		)),
	}
	disallowedSourceDir := support.ScaffoldFactory(t, disallowedPermissionFactoryConfig())
	workflowDir := filepath.Join(disallowedSourceDir, "workflows")
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatalf("create disallowed permission workflow directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workflowDir, "review.js"), []byte(disallowedPermissionWorkflow), 0o600); err != nil {
		t.Fatalf("write disallowed permission workflow: %v", err)
	}
	api := support.NewProcessAPIServer()
	apiStarter := &javascriptSharedAPIServerStarter{api: api}
	router := newJavaScriptSharedCommandRouter()
	writeJavaScriptSharedWorkerPresetConfig(t, homeDir)
	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		APIServerStarter:                   apiStarter.Start,
		FactoryRuntimeWorkflowHome:         func() (string, error) { return homeDir, nil },
		FactorySessionResolveHomeDirectory: func() (string, error) { return homeDir, nil },
		ProviderCommandRunner:              router,
	})
	if err != nil {
		t.Fatalf("BuildProcess(shared JavaScript spine): %v", err)
	}
	fixture := &javascriptSharedProcessFixture{
		process:            process,
		router:             router,
		apiStarter:         apiStarter,
		environment:        javascriptSharedCustomerEnvironment(homeDir),
		hostDir:            hostDir,
		successFactoryName: successFactoryName,
		failureFactoryName: failureFactoryName,
		disallowedFactory:  disallowedFactory,
		trackedSessions:    make(map[string]struct{}),
		closedSessions:     make(map[string]struct{}),
	}
	// Cleanup is registered before the process and host cleanups so the probe
	// runs after the hosted command stops and the one reusable process closes.
	t.Cleanup(func() { fixture.assertCleanup(t) })
	support.CleanupProcess(t, process)
	support.CopyFactoryAsNamed(t, successSourceDir, homeDir, successFactoryName)
	support.CopyFactoryAsNamed(t, failureSourceDir, homeDir, failureFactoryName)
	for name, sourceDir := range factoryDirs {
		factoryDirs[name] = support.CopyFactoryAsNamed(t, sourceDir, homeDir, name)
	}
	support.CopyFactoryAsNamed(t, disallowedSourceDir, homeDir, disallowedFactory)

	mockWorkersPath := support.WriteMockWorkersConfig(t, partialNamedJavaScriptMockWorkersConfig())
	inputs := support.FakeInputs(context.Background(), []string{
		"you", "run", "--dir", hostDir,
		"--continuously", "--with-server", "--quiet", "--no-record",
		"--with-mock-workers", mockWorkersPath,
	})
	inputs.Input.Env = append([]string(nil), fixture.environment...)
	inputs.Input.WorkingDirectory = hostDir
	support.StartProcessCommand(t, process, inputs.Input)
	fixture.baseURL = api.WaitForURL(t)
	support.WaitForStatus(t, fixture.baseURL, 15*time.Second, func(status factoryapi.StatusResponse) bool {
		return strings.TrimSpace(status.RuntimeStatus) != ""
	})
	if got := fixture.apiStarter.starts.Load(); got != 1 {
		t.Fatalf("shared API server starts = %d, want one host-owned listener", got)
	}
	return fixture
}

func writeJavaScriptSharedWorkerPresetConfig(t testing.TB, homeDir string) {
	t.Helper()
	configDir := filepath.Join(homeDir, ".you-agent-factory")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("create shared worker configuration directory: %v", err)
	}
	config := []byte(`{
  "workerPresets": [{
    "id": "` + mockedWorkerPresetName + `",
    "modelProvider": "codex",
    "model": "mocked-child-model"
  }]
}`)
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), config, 0o600); err != nil {
		t.Fatalf("write shared worker configuration: %v", err)
	}
}

func (fixture *javascriptSharedProcessFixture) executeRemote(
	t testing.TB,
	namedFactory string,
	prompt string,
) (*support.CapturedInputs, error) {
	t.Helper()
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--remote", "--server", fixture.baseURL, "--json", "run",
		"--named", namedFactory,
		"--output", "primary", "--no-record", prompt,
	})
	inputs.Input.Env = append([]string(nil), fixture.environment...)
	inputs.Input.WorkingDirectory = fixture.hostDir
	return inputs, fixture.process.Execute(inputs.Input)
}

func (fixture *javascriptSharedProcessFixture) executeInline(
	t *testing.T,
	name string,
	workflow string,
) factoryapi.FactorySessionSyncExecutionResponse {
	t.Helper()
	requestID := fmt.Sprintf(
		"shared-javascript-%s-%d",
		strings.NewReplacer(" ", "-", "/", "-", "_", "-").Replace(name),
		fixture.requestSequence.Add(1),
	)
	response := startOverridesWorkflow(t, fixture.baseURL, requestID, workflow)
	fixture.trackSession(t, response.SessionId)
	return response
}

func (fixture *javascriptSharedProcessFixture) trackSession(t testing.TB, sessionID string) {
	t.Helper()
	if strings.TrimSpace(sessionID) == "" {
		t.Fatal("shared JavaScript workflow returned empty Factory Session ID")
	}
	fixture.sessionMu.Lock()
	if _, exists := fixture.trackedSessions[sessionID]; exists {
		fixture.sessionMu.Unlock()
		t.Fatalf("shared JavaScript workflow reused Factory Session ID %q", sessionID)
	}
	fixture.trackedSessions[sessionID] = struct{}{}
	fixture.sessionMu.Unlock()
	t.Cleanup(func() {
		// A synchronous workflow already owns a terminal session. Requesting
		// termination is the bounded public cleanup operation; waiting for the
		// delete-only stopped transition here can outlive the shared host.
		support.TerminateFactorySessionAt(t, fixture.baseURL, sessionID)
		fixture.sessionMu.Lock()
		fixture.closedSessions[sessionID] = struct{}{}
		fixture.sessionMu.Unlock()
	})
}

func (fixture *javascriptSharedProcessFixture) persistentSessionIDs(t testing.TB) []string {
	t.Helper()
	response := support.GetJSON[factoryapi.ListFactorySessionsResponse](
		t,
		strings.TrimSuffix(fixture.baseURL, "/")+"/factory-sessions?scope=persisted",
	)
	if response.DurableSessions == nil {
		return nil
	}
	ids := make([]string, 0, len(*response.DurableSessions))
	for _, session := range *response.DurableSessions {
		ids = append(ids, session.SessionId)
	}
	return ids
}

func (fixture *javascriptSharedProcessFixture) assertCleanup(t testing.TB) {
	t.Helper()
	if got := fixture.router.routeCount(); got != 0 {
		t.Errorf("ROUTE-001 routes remaining after subtest cleanup = %d, want 0", got)
	}
	if got := fixture.apiStarter.starts.Load(); got != 1 {
		t.Errorf("ROUTE-001 API server starts = %d, want one", got)
	}
	fixture.sessionMu.Lock()
	tracked := len(fixture.trackedSessions)
	closed := len(fixture.closedSessions)
	fixture.sessionMu.Unlock()
	if tracked != closed {
		t.Errorf("CLEAN-001 tracked sessions closed = %d/%d, want all closed", closed, tracked)
	}
	if fixture.baseURL == "" {
		return
	}
	client := http.Client{Timeout: time.Second}
	response, err := client.Get(strings.TrimSuffix(fixture.baseURL, "/") + "/status")
	if err == nil {
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		t.Errorf("ROUTE-001 shared API listener remained available after process close: status=%d body=%q", response.StatusCode, strings.TrimSpace(string(body)))
	}
}

func javascriptSharedCustomerEnvironment(homeDir string) []string {
	return []string{"HOME=" + homeDir, "USERPROFILE=" + homeDir}
}

func assertJavaScriptSharedCustomerEnvironmentSanitized(
	t testing.TB,
	fixture *javascriptSharedProcessFixture,
) {
	t.Helper()
	if environmentContainsName(fixture.environment, sharedJavaScriptSentinelCredentialName) {
		t.Fatalf(
			"customer environment propagated sentinel credential %q",
			sharedJavaScriptSentinelCredentialName,
		)
	}
	requestLog, err := json.Marshal(fixture.router.requestRecords())
	if err != nil {
		t.Fatalf("marshal shared JavaScript request log: %v", err)
	}
	for _, forbidden := range []string{
		sharedJavaScriptSentinelCredentialName,
		sharedJavaScriptSentinelCredentialValue,
	} {
		if bytes.Contains(requestLog, []byte(forbidden)) {
			t.Fatalf("shared JavaScript request log contains forbidden credential data %q", forbidden)
		}
	}
}

func environmentContainsName(environment []string, want string) bool {
	for _, entry := range environment {
		name := strings.SplitN(entry, "=", 2)[0]
		if strings.EqualFold(name, want) {
			return true
		}
	}
	return false
}

func readJavaScriptSharedDurableSession(
	t testing.TB,
	baseURL string,
	sessionID string,
) factoryapi.FactorySessionDurableReadModel {
	t.Helper()
	response := support.GetJSON[factoryapi.FactorySessionGetResponse](
		t,
		strings.TrimSuffix(baseURL, "/")+"/factory-sessions/"+url.PathEscape(sessionID),
	)
	session, err := response.AsFactorySessionDurableReadModel()
	if err != nil {
		t.Fatalf("decode shared durable session: %v", err)
	}
	return session
}

func assertSharedRemoteCommandPlacement(t testing.TB, args []string, serverURL string) {
	t.Helper()
	wantPrefix := []string{"you", "--remote", "--server", serverURL}
	if len(args) < len(wantPrefix) || !reflect.DeepEqual(args[:len(wantPrefix)], wantPrefix) {
		t.Fatalf("public remote command prefix = %#v, want %#v", args, wantPrefix)
	}
	if len(args) < len(wantPrefix)+2 || !reflect.DeepEqual(args[len(wantPrefix):len(wantPrefix)+2], []string{"--json", "run"}) {
		t.Fatalf("public remote command mode = %#v, want --json run after --remote --server", args)
	}
}

func assertJavaScriptSharedCompletedDispatch(
	t testing.TB,
	fixture *javascriptSharedProcessFixture,
	sessionID, wantProvider, wantModel, wantLabel string,
) {
	t.Helper()
	dispatches := support.GetJSON[factoryapi.ListFactorySessionDispatchesResponse](
		t,
		strings.TrimSuffix(fixture.baseURL, "/")+"/factory-sessions/"+url.PathEscape(sessionID)+"/dispatches",
	)
	if len(dispatches.Dispatches) != 1 {
		t.Fatalf("shared session %q dispatch count = %d, want one; dispatches=%#v", sessionID, len(dispatches.Dispatches), dispatches.Dispatches)
	}
	dispatch := dispatches.Dispatches[0]
	if dispatch.Status != factoryapi.FactoryDispatchStatusCOMPLETED {
		t.Fatalf("shared session %q dispatch status = %q, want COMPLETED", sessionID, dispatch.Status)
	}
	if wantLabel != "" && (dispatch.Label == nil || *dispatch.Label != wantLabel) {
		t.Fatalf("shared session %q dispatch label = %#v, want %q", sessionID, dispatch.Label, wantLabel)
	}
	if wantProvider != "" && (dispatch.ModelProvider == nil || *dispatch.ModelProvider != wantProvider) {
		t.Fatalf("shared session %q dispatch provider = %#v, want %q", sessionID, dispatch.ModelProvider, wantProvider)
	}
	if wantModel != "" && (dispatch.Model == nil || *dispatch.Model != wantModel) {
		t.Fatalf("shared session %q dispatch model = %#v, want %q", sessionID, dispatch.Model, wantModel)
	}
}

func assertJavaScriptSharedNoDispatch(t testing.TB, fixture *javascriptSharedProcessFixture, sessionID string) {
	t.Helper()
	dispatches := support.GetJSON[factoryapi.ListFactorySessionDispatchesResponse](
		t,
		strings.TrimSuffix(fixture.baseURL, "/")+"/factory-sessions/"+url.PathEscape(sessionID)+"/dispatches",
	)
	if len(dispatches.Dispatches) != 0 {
		t.Fatalf("shared session %q dispatches = %#v, want none", sessionID, dispatches.Dispatches)
	}
}

func cloneJavaScriptCommandRequest(request platformprocess.CommandRequest) platformprocess.CommandRequest {
	request.Args = append([]string(nil), request.Args...)
	request.Stdin = append([]byte(nil), request.Stdin...)
	request.Env = append([]string(nil), request.Env...)
	return request
}

func differenceJavaScriptSessionIDs(before, after []string) []string {
	seen := make(map[string]struct{}, len(before))
	for _, id := range before {
		seen[id] = struct{}{}
	}
	var difference []string
	for _, id := range after {
		if _, exists := seen[id]; !exists {
			difference = append(difference, id)
		}
	}
	return difference
}

func duplicateJavaScriptSessionID(ids []string) string {
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if _, exists := seen[id]; exists {
			return id
		}
		seen[id] = struct{}{}
	}
	return ""
}

func containsJavaScriptSessionID(ids []string, want string) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}

var _ platformprocess.CommandRunner = (*javascriptSharedCommandRouter)(nil)
