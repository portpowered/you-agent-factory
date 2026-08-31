package codex

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	codexSharedFixtureTimeout          = 15 * time.Second
	codexSharedTrustedRouteSelector    = "codex-shared-trusted-work"
	codexSharedActionableRouteSelector = "codex-shared-actionable-refusal"
	codexSharedNeutralRouteSelector    = "codex-shared-neutral-refusal"
	codexSharedDuplicateSelector       = "codex-shared-duplicate-work"
	codexSharedTrustedWorkName         = "codex-shared-trusted-work"
	codexSharedActionableWorkName      = "codex-shared-actionable-refusal"
	codexSharedNeutralWorkName         = "codex-shared-neutral-refusal"
)

// codexSharedHTTPServer owns the one loopback server started by the shared
// root-built process. The starter count and completion signal make lifecycle
// ownership observable without changing process-global state.
type codexSharedHTTPServer struct {
	server *support.ProcessAPIServer

	mu       sync.Mutex
	starts   int
	done     chan struct{}
	doneOnce sync.Once
}

func newCodexSharedHTTPServer() *codexSharedHTTPServer {
	return &codexSharedHTTPServer{
		server: support.NewProcessAPIServer(),
		done:   make(chan struct{}),
	}
}

func (server *codexSharedHTTPServer) start(
	ctx context.Context,
	request platformhttpserver.StartRequest,
) error {
	server.mu.Lock()
	server.starts++
	server.mu.Unlock()
	defer server.doneOnce.Do(func() { close(server.done) })
	return server.server.Start(ctx, request)
}

func (server *codexSharedHTTPServer) startCount() int {
	server.mu.Lock()
	defer server.mu.Unlock()
	return server.starts
}

func (server *codexSharedHTTPServer) waitClosed(ctx context.Context) error {
	select {
	case <-server.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// codexProcessConstructor records successful root-built process constructions
// at the local construction boundary. The shared fixture uses this helper for
// every process it builds, so its topology assertion observes construction
// rather than comparing an initialized literal with the expected count.
type codexProcessConstructor struct {
	mu     sync.Mutex
	builds int
}

func (constructor *codexProcessConstructor) build(
	t testing.TB,
	edges serviceedges.Edges,
) support.ApplicationProcess {
	t.Helper()
	process, err := support.BuildProcessWithContext(context.Background(), edges)
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
	constructor.mu.Lock()
	constructor.builds++
	constructor.mu.Unlock()
	return process
}

func (constructor *codexProcessConstructor) count() int {
	constructor.mu.Lock()
	defer constructor.mu.Unlock()
	return constructor.builds
}

type codexSharedProcessFixture struct {
	rootDir                  string
	homeDir                  string
	trustedFactoryDir        string
	actionableFactoryDir     string
	neutralFactoryDir        string
	containmentOutsideDir    string
	containmentAvailable     bool
	containmentCapabilityErr error
	baseURL                  string

	process       support.ApplicationProcess
	command       *support.ProcessCommand
	api           *codexSharedHTTPServer
	commandRunner *codexSharedCommandRunner
	constructor   *codexProcessConstructor

	sessionMu         sync.Mutex
	openedSessionIDs  []string
	deletedSessionIDs []string

	closeOnce    sync.Once
	finalizeOnce sync.Once
}

func newCodexSharedProcessFixture(t *testing.T) *codexSharedProcessFixture {
	t.Helper()

	rootDir := t.TempDir()
	homeDir := filepath.Join(rootDir, "codex-home")
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		t.Fatalf("create shared Codex home: %v", err)
	}
	paths := prepareCodexSharedFactoryPaths(t)
	containment := prepareCodexSharedRolloutFixtures(t, homeDir)
	runner := prepareCodexSharedRoutes(t, paths, rootDir)

	api := newCodexSharedHTTPServer()
	constructor := &codexProcessConstructor{}
	process := constructor.build(t, serviceedges.Edges{
		APIServerStarter:                    api.start,
		ProviderCommandRunner:               runner,
		ProviderSessionResolveHomeDirectory: func() (string, error) { return homeDir, nil },
	})
	fixture := &codexSharedProcessFixture{
		rootDir: rootDir, homeDir: homeDir,
		trustedFactoryDir:        paths.trusted,
		actionableFactoryDir:     paths.actionable,
		neutralFactoryDir:        paths.neutral,
		containmentOutsideDir:    containment.outsideDir,
		containmentAvailable:     containment.available,
		containmentCapabilityErr: containment.err,
		process:                  process, api: api, commandRunner: runner, constructor: constructor,
	}

	inputs := support.FakeInputs(context.Background(), []string{
		"you", "run", "--dir", paths.trusted, "--continuously", "--with-server", "--quiet", "--no-record",
	})
	inputs.Input.Env = []string{"HOME=" + homeDir, "USERPROFILE=" + homeDir}
	inputs.Input.WorkingDirectory = paths.trusted
	fixture.command = support.StartProcessCommand(t, process, inputs.Input)
	t.Cleanup(func() { fixture.finalize(t) })

	fixture.baseURL = api.server.WaitForURL(t)
	support.WaitForStatus(t, fixture.baseURL, codexSharedFixtureTimeout, func(status factoryapi.StatusResponse) bool {
		return strings.TrimSpace(status.RuntimeStatus) != ""
	})
	return fixture
}

type codexSharedFactoryPaths struct {
	trusted    string
	actionable string
	neutral    string
}

func prepareCodexSharedFactoryPaths(t *testing.T) codexSharedFactoryPaths {
	t.Helper()
	return codexSharedFactoryPaths{
		trusted:    scaffoldCodexSharedFactory(t),
		actionable: scaffoldCodexSharedRefusalFactory(t),
		neutral:    scaffoldCodexSharedRefusalFactory(t),
	}
}

func prepareCodexSharedRolloutFixtures(
	t *testing.T,
	homeDir string,
) codexSharedContainmentFixture {
	t.Helper()
	containment := prepareCodexSharedContainmentFixture(t, homeDir, codexFunctionalOutsideSessionID)
	writeCodexRolloutFixture(
		t,
		codexSessionsRoot(homeDir),
		codexFunctionalSessionID,
		representativeCodexJSONL(),
	)
	writeCodexRolloutFixture(
		t,
		codexSessionsRoot(homeDir),
		codexFunctionalDetachedSessionID,
		representativeCodexJSONL(),
	)
	writeCodexRolloutFixture(
		t,
		codexSessionsRoot(homeDir),
		codexFunctionalMalformedSessionID,
		`{"type":"turn_context"}`+"\n"+
			`{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"partial`,
	)
	writeCodexRolloutFixture(
		t,
		codexSessionsRoot(homeDir),
		codexFunctionalOversizedSessionID,
		strings.Repeat("x", 1<<20+1)+"\n",
	)
	for i := 0; i < 65; i++ {
		writeCodexRolloutFixtureAt(
			t,
			codexSessionsRoot(homeDir),
			fmt.Sprintf("2026/05/%02d", i),
			codexFunctionalBoundedWalkSessionID,
			`{"type":"session_meta"}`+"\n",
		)
	}
	return containment
}

func prepareCodexSharedRoutes(
	t *testing.T,
	paths codexSharedFactoryPaths,
	rootDir string,
) *codexSharedCommandRunner {
	t.Helper()
	runner := newCodexSharedCommandRunner()
	if err := runner.register(
		codexSharedTrustedRouteSelector,
		paths.trusted,
		platformprocess.CommandResult{Stdout: []byte("trusted Git invocation COMPLETE")},
	); err != nil {
		t.Fatalf("register trusted Codex route: %v", err)
	}
	if err := runner.register(
		codexSharedActionableRouteSelector,
		paths.actionable,
		codexUntrustedWorkingDirectoryCommandResult(),
	); err != nil {
		t.Fatalf("register actionable Codex route: %v", err)
	}
	if err := runner.register(
		codexSharedNeutralRouteSelector,
		paths.neutral,
		codexNeutralRefusalCommandResult(),
	); err != nil {
		t.Fatalf("register neutral Codex route: %v", err)
	}
	assertCodexSharedDuplicateRouteRejected(t, runner, paths.trusted, 3)
	assertCodexSharedUnknownRouteRejected(t, runner, rootDir)
	return runner
}

func scaffoldCodexSharedFactory(t *testing.T) string {
	t.Helper()
	dir := scaffoldCodexWorkingDirectoryFactory(t)
	support.ClearSeedInputs(t, dir)
	initTrustedCodexRepository(t, dir)
	return dir
}

func scaffoldCodexSharedRefusalFactory(t *testing.T) string {
	t.Helper()
	dir := scaffoldCodexWorkingDirectoryFactory(t)
	support.ClearSeedInputs(t, dir)
	return dir
}

func (fixture *codexSharedProcessFixture) close(t testing.TB) {
	t.Helper()
	fixture.closeOnce.Do(func() {
		if fixture.command != nil {
			fixture.command.Stop(t)
		}
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := fixture.process.Close(closeCtx); err != nil {
			t.Errorf("close shared Codex application process: %v", err)
		}
		if err := fixture.api.waitClosed(closeCtx); err != nil {
			t.Errorf("wait for shared Codex API server shutdown: %v", err)
		}
	})
}

func (fixture *codexSharedProcessFixture) finalize(t testing.TB) {
	t.Helper()
	fixture.finalizeOnce.Do(func() {
		fixture.closeUnclosedSessions(t)
		fixture.close(t)
		assertCodexSharedListenerClosed(t, fixture.baseURL)
		for _, selector := range []string{
			codexSharedTrustedRouteSelector,
			codexSharedActionableRouteSelector,
			codexSharedNeutralRouteSelector,
		} {
			if err := fixture.commandRunner.unregister(selector); err != nil {
				t.Errorf("unregister shared Codex route %q: %v", selector, err)
			}
		}
		if got := fixture.commandRunner.routeCount(); got != 0 {
			t.Errorf("shared Codex route count after cleanup = %d, want zero", got)
		}
		removeCodexOwnedPath(t, fixture.trustedFactoryDir)
		removeCodexOwnedPath(t, fixture.actionableFactoryDir)
		removeCodexOwnedPath(t, fixture.neutralFactoryDir)
		removeCodexOwnedPath(t, fixture.containmentOutsideDir)
		removeCodexOwnedPath(t, fixture.rootDir)
	})
}

func (fixture *codexSharedProcessFixture) closeUnclosedSessions(t testing.TB) {
	t.Helper()
	fixture.sessionMu.Lock()
	opened := append([]string(nil), fixture.openedSessionIDs...)
	deleted := make(map[string]struct{}, len(fixture.deletedSessionIDs))
	for _, sessionID := range fixture.deletedSessionIDs {
		deleted[sessionID] = struct{}{}
	}
	fixture.sessionMu.Unlock()
	for _, sessionID := range opened {
		if _, ok := deleted[sessionID]; ok {
			continue
		}
		support.CloseFactorySessionAt(t, fixture.baseURL, sessionID)
		assertCodexFactorySessionDeleted(t, fixture.baseURL, sessionID)
		fixture.markSessionDeleted(sessionID)
	}
}

func (fixture *codexSharedProcessFixture) openSession(t testing.TB, factoryDir string) string {
	t.Helper()
	opened := support.OpenFactorySessionAt(t, fixture.baseURL, factoryDir)
	if opened.Session == nil || strings.TrimSpace(opened.Session.Id) == "" {
		t.Fatalf("shared Factory Session for %q = %#v, want identity", factoryDir, opened)
	}
	sessionID := opened.Session.Id
	if sessionID == factorysessions.DefaultSessionID {
		t.Fatalf("shared Factory Session for %q = %q, want explicit session", factoryDir, sessionID)
	}
	fixture.sessionMu.Lock()
	fixture.openedSessionIDs = append(fixture.openedSessionIDs, sessionID)
	fixture.sessionMu.Unlock()
	t.Cleanup(func() {
		support.CloseFactorySessionAt(t, fixture.baseURL, sessionID)
		assertCodexFactorySessionDeleted(t, fixture.baseURL, sessionID)
		fixture.markSessionDeleted(sessionID)
	})
	return sessionID
}

func (fixture *codexSharedProcessFixture) markSessionDeleted(sessionID string) {
	fixture.sessionMu.Lock()
	defer fixture.sessionMu.Unlock()
	for _, existing := range fixture.deletedSessionIDs {
		if existing == sessionID {
			return
		}
	}
	fixture.deletedSessionIDs = append(fixture.deletedSessionIDs, sessionID)
}

func assertCodexSharedListenerClosed(t testing.TB, baseURL string) {
	t.Helper()
	client := &http.Client{Timeout: 250 * time.Millisecond}
	defer client.CloseIdleConnections()
	response, err := client.Get(strings.TrimSuffix(baseURL, "/") + "/status")
	if err != nil {
		return
	}
	defer response.Body.Close()
	body, readErr := io.ReadAll(response.Body)
	t.Errorf(
		"shared Codex listener remains reachable after cleanup: status=%d body=%q readError=%v",
		response.StatusCode,
		strings.TrimSpace(string(body)),
		readErr,
	)
}

func removeCodexOwnedPath(t testing.TB, path string) {
	t.Helper()
	if strings.TrimSpace(path) == "" {
		return
	}
	if err := os.RemoveAll(path); err != nil {
		t.Errorf("remove Codex-owned path %q: %v", path, err)
		return
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("Codex-owned path %q remains after cleanup; stat error: %v", path, err)
	}
}

func TestCodexSharedTrustedWorkAndHistory(t *testing.T) {
	fixture := newCodexSharedProcessFixture(t)

	t.Run("trusted_work", func(t *testing.T) {
		runCodexSharedTrustedWork(t, fixture)
	})
	if t.Failed() {
		return
	}

	t.Run("actionable_refusal", func(t *testing.T) {
		assertCodexSharedActionableRefusal(t, fixture)
	})
	if t.Failed() {
		return
	}

	t.Run("neutral_refusal", func(t *testing.T) {
		assertCodexSharedNeutralRefusal(t, fixture)
	})
	if t.Failed() {
		return
	}

	t.Run("successful_history", func(t *testing.T) {
		assertCodexSharedSuccessfulHistory(t, fixture)
	})
	if t.Failed() {
		return
	}
	t.Run("detached_repeated_history", func(t *testing.T) {
		assertCodexSharedDetachedHistory(t, fixture)
	})
	t.Run("missing_history", func(t *testing.T) {
		assertCodexSharedMissingHistory(t, fixture)
	})
	t.Run("malformed_history", func(t *testing.T) {
		assertCodexSharedMalformedHistory(t, fixture)
	})
	t.Run("oversized_history", func(t *testing.T) {
		assertCodexSharedOversizedHistory(t, fixture)
	})
	t.Run("bounded_history", func(t *testing.T) {
		assertCodexSharedBoundedHistory(t, fixture)
	})
	t.Run("containment_history", func(t *testing.T) {
		assertCodexSharedContainmentHistory(t, fixture)
	})
	if t.Failed() {
		return
	}
	// Re-read a known-good rollout after every adverse history request. This
	// proves the shared HTTP/process spine remains healthy after safe failures.
	assertCodexSharedSuccessfulHistory(t, fixture)
	if t.Failed() {
		return
	}
	fixture.assertTopology(t)
	fixture.finalize(t)
}

func runCodexSharedTrustedWork(t *testing.T, fixture *codexSharedProcessFixture) string {
	t.Helper()
	sessionID := fixture.openSession(t, fixture.trustedFactoryDir)

	name := codexSharedTrustedWorkName
	submitted := support.SubmitSessionWorkAt(t, fixture.baseURL, sessionID, factoryapi.SubmitWorkRequest{
		Name:         &name,
		WorkTypeName: "task",
		Payload:      map[string]string{"title": "shared trusted Codex work"},
	})
	if submitted.SessionId == nil || *submitted.SessionId != sessionID {
		t.Fatalf("submitted Work session ID = %#v, want %q", submitted.SessionId, sessionID)
	}
	workID := support.StringPointerValue(submitted.WorkId)
	if workID == "" || strings.TrimSpace(submitted.RequestId) == "" {
		t.Fatalf("submitted Work identity = work:%q request:%q, want both identities", workID, submitted.RequestId)
	}
	support.WaitForSessionTerminalStatus(t, fixture.baseURL, sessionID, codexSharedFixtureTimeout)

	listed := listCodexSessionWork(t, fixture.baseURL, sessionID)
	if got := support.CountWorkAtCustomerState(listed, "task:complete"); got != 1 {
		t.Fatalf("shared trusted complete Work count = %d, want one; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 0 {
		t.Fatalf("shared trusted failed Work count = %d, want zero; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:init"); got != 0 {
		t.Fatalf("shared trusted initial Work count = %d, want zero; listed=%#v", got, listed)
	}

	requests := fixture.commandRunner.Requests()
	if len(requests) != 1 {
		t.Fatalf("shared Codex command requests = %d, want one", len(requests))
	}
	if requests[0].Command != string(modelprovider.ProviderCodex) || requests[0].WorkDir != fixture.trustedFactoryDir {
		t.Fatalf("shared Codex command request = %#v, want codex route for %q", requests[0], fixture.trustedFactoryDir)
	}

	events := support.GetFactoryEventsForSessionAt(t, fixture.baseURL, sessionID)
	support.AssertSingleWorkRequestEvent(t, events, submitted.RequestId, workID, "task")
	assertCodexSharedAcceptedDispatch(t, events, workID)
	return sessionID
}

func assertCodexSharedAcceptedDispatch(t testing.TB, events []factoryapi.FactoryEvent, workID string) {
	t.Helper()
	accepted := 0
	for _, dispatch := range support.ObserveDispatchEvents(t, events) {
		if dispatch.Request.TransitionId != "process" || !support.DispatchObservationIncludesWork(dispatch, workID) {
			continue
		}
		if dispatch.Response == nil {
			t.Fatalf("shared trusted process dispatch %q has no public response", dispatch.DispatchID)
		}
		if dispatch.Response.Outcome != factoryapi.WorkOutcomeAccepted || dispatch.Response.Error != nil {
			t.Fatalf("shared trusted process dispatch response = %#v, want ACCEPTED without error", dispatch.Response)
		}
		accepted++
	}
	if accepted != 1 {
		t.Fatalf("shared trusted accepted process dispatches = %d, want one", accepted)
	}
}

func assertCodexSharedSuccessfulHistory(t *testing.T, fixture *codexSharedProcessFixture) {
	t.Helper()
	detail := getCodexProviderSessionDetail(t, fixture.baseURL, codexFunctionalSessionID)
	if detail.ProviderSession.Id != codexFunctionalSessionID ||
		detail.ProviderSession.Provider != factoryapi.Codex ||
		detail.ProviderSession.Kind != factoryapi.LoadableProviderSessionKindSessionID {
		t.Fatalf("shared provider session = %#v, want codex session_id %s", detail.ProviderSession, codexFunctionalSessionID)
	}
	if detail.Source.RelativePath != "2026/07/27/rollout-"+codexFunctionalSessionID+".jsonl" {
		t.Fatalf("shared source path = %q, want contained rollout path", detail.Source.RelativePath)
	}
	if len(detail.Transcript) < 4 || len(detail.Parse.FunctionCalls) != 1 || len(detail.Parse.Reasoning) != 1 {
		t.Fatalf("shared provider detail = %#v, want transcript, tool, and reasoning facts", detail)
	}
	if detail.Parse.TokenUsage == nil || detail.Parse.TokenUsage.TotalTokens == nil ||
		*detail.Parse.TokenUsage.TotalTokens != 130 {
		t.Fatalf("shared provider token usage = %#v, want total 130", detail.Parse.TokenUsage)
	}
	if detail.Transcript[0].Text == nil || !strings.Contains(*detail.Transcript[0].Text, "Inspect the failing run") {
		t.Fatalf("shared provider transcript = %#v, want user message text", detail.Transcript)
	}
}

func (fixture *codexSharedProcessFixture) assertSessionTopology(t testing.TB) {
	t.Helper()
	fixture.sessionMu.Lock()
	opened := append([]string(nil), fixture.openedSessionIDs...)
	deleted := append([]string(nil), fixture.deletedSessionIDs...)
	fixture.sessionMu.Unlock()
	if len(opened) != 3 || len(deleted) != 3 {
		t.Fatalf("shared Factory Session topology = opened:%d deleted:%d, want three each", len(opened), len(deleted))
	}
	seen := make(map[string]struct{}, len(opened))
	for _, sessionID := range opened {
		if _, exists := seen[sessionID]; exists {
			t.Fatalf("shared Factory Session ID %q was reused", sessionID)
		}
		seen[sessionID] = struct{}{}
	}
	for _, sessionID := range deleted {
		if _, exists := seen[sessionID]; !exists {
			t.Fatalf("deleted shared Factory Session ID %q was not opened by this fixture", sessionID)
		}
	}
}

func (fixture *codexSharedProcessFixture) assertTopology(t testing.TB) {
	t.Helper()
	if got := fixture.constructor.count(); got != 1 || fixture.api.startCount() != 1 {
		t.Fatalf("shared Codex topology = root:%d http:%d, want one each", got, fixture.api.startCount())
	}
	if got := fixture.commandRunner.CallCount(); got != 3 {
		t.Fatalf("shared Codex command calls = %d, want one call for each Work route", got)
	}
	if got := fixture.commandRunner.routeCount(); got != 3 {
		t.Fatalf("shared Codex active route count = %d, want three immutable routes", got)
	}
	fixture.assertSessionTopology(t)
}

func assertCodexFactorySessionDeleted(t testing.TB, baseURL, sessionID string) {
	t.Helper()
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID)
	response, err := http.Get(endpoint)
	if err != nil {
		t.Fatalf("GET deleted shared Factory Session %q: %v", sessionID, err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("GET deleted shared Factory Session %q status = %d, want 404: %s", sessionID, response.StatusCode, strings.TrimSpace(string(body)))
	}
}

func listCodexSessionWork(t testing.TB, baseURL, sessionID string) factoryapi.ListWorkResponse {
	t.Helper()
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID) + "/work"
	return support.GetJSON[factoryapi.ListWorkResponse](t, endpoint)
}

type codexSharedCommandRoute struct {
	selector string
	workDir  string
	result   platformprocess.CommandResult
}

// codexSharedCommandRunner selects only by the immutable WorkDir carried by a
// ProviderCommandRunner request. Duplicate or unknown paths fail closed, and
// no route is selected from mutable session or invocation ordering.
type codexSharedCommandRunner struct {
	mu       sync.Mutex
	routes   map[string]codexSharedCommandRoute
	requests []platformprocess.CommandRequest
}

func newCodexSharedCommandRunner() *codexSharedCommandRunner {
	return &codexSharedCommandRunner{routes: make(map[string]codexSharedCommandRoute)}
}

func (runner *codexSharedCommandRunner) register(
	selector, workDir string,
	result platformprocess.CommandResult,
) error {
	selector = strings.TrimSpace(selector)
	workDir = strings.TrimSpace(workDir)
	if selector == "" || workDir == "" {
		return fmt.Errorf("Codex route selector and WorkDir are required")
	}
	runner.mu.Lock()
	defer runner.mu.Unlock()
	if _, exists := runner.routes[workDir]; exists {
		return fmt.Errorf("Codex WorkDir route %q is already registered", workDir)
	}
	for _, route := range runner.routes {
		if route.selector == selector {
			return fmt.Errorf("Codex route selector %q is already registered", selector)
		}
	}
	result.Stdout = append([]byte(nil), result.Stdout...)
	result.Stderr = append([]byte(nil), result.Stderr...)
	runner.routes[workDir] = codexSharedCommandRoute{selector: selector, workDir: workDir, result: result}
	return nil
}

func (runner *codexSharedCommandRunner) unregister(selector string) error {
	selector = strings.TrimSpace(selector)
	runner.mu.Lock()
	defer runner.mu.Unlock()
	for workDir, route := range runner.routes {
		if route.selector != selector {
			continue
		}
		delete(runner.routes, workDir)
		return nil
	}
	return fmt.Errorf("Codex route selector %q is not registered", selector)
}

func (runner *codexSharedCommandRunner) routeCount() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return len(runner.routes)
}

func (runner *codexSharedCommandRunner) CallCount() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return len(runner.requests)
}

func (runner *codexSharedCommandRunner) Requests() []platformprocess.CommandRequest {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	requests := make([]platformprocess.CommandRequest, len(runner.requests))
	for index := range runner.requests {
		requests[index] = cloneCodexCommandRequest(runner.requests[index])
	}
	return requests
}

func (runner *codexSharedCommandRunner) RequestsForWorkDir(workDir string) []platformprocess.CommandRequest {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	requests := make([]platformprocess.CommandRequest, 0)
	for _, request := range runner.requests {
		if request.WorkDir == workDir {
			requests = append(requests, cloneCodexCommandRequest(request))
		}
	}
	return requests
}

func (runner *codexSharedCommandRunner) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	runner.mu.Lock()
	route, ok := runner.routes[request.WorkDir]
	runner.mu.Unlock()
	if !ok {
		return platformprocess.CommandResult{}, fmt.Errorf("no Codex route matched WorkDir %q", request.WorkDir)
	}
	if request.Command != string(modelprovider.ProviderCodex) {
		return platformprocess.CommandResult{}, fmt.Errorf("Codex route %q received command %q", route.selector, request.Command)
	}
	if err := ctx.Err(); err != nil {
		return platformprocess.CommandResult{}, err
	}
	runner.mu.Lock()
	runner.requests = append(runner.requests, cloneCodexCommandRequest(request))
	runner.mu.Unlock()
	result := route.result
	result.Stdout = support.CodexSuccessStdout(string(result.Stdout))
	result.Stderr = append([]byte(nil), result.Stderr...)
	return result, nil
}

func cloneCodexCommandRequest(request platformprocess.CommandRequest) platformprocess.CommandRequest {
	request.Args = append([]string(nil), request.Args...)
	request.Stdin = append([]byte(nil), request.Stdin...)
	request.Env = append([]string(nil), request.Env...)
	return request
}

func assertCodexSharedDuplicateRouteRejected(
	t testing.TB,
	runner *codexSharedCommandRunner,
	factoryDir string,
	wantRouteCount int,
) {
	t.Helper()
	err := runner.register(
		codexSharedDuplicateSelector,
		factoryDir,
		platformprocess.CommandResult{Stdout: []byte("duplicate route must not run")},
	)
	if err == nil {
		t.Fatalf("duplicate Codex route for WorkDir %q was accepted", factoryDir)
	}
	if got := runner.routeCount(); got != wantRouteCount {
		t.Fatalf("Codex route count after duplicate rejection = %d, want %d", got, wantRouteCount)
	}
}

func assertCodexSharedUnknownRouteRejected(
	t testing.TB,
	runner *codexSharedCommandRunner,
	rootDir string,
) {
	t.Helper()
	unknownWorkDir := filepath.Join(rootDir, "unknown-workdir")
	_, err := runner.Run(context.Background(), platformprocess.CommandRequest{
		Command: "codex", WorkDir: unknownWorkDir,
		Stdin: []byte("secret work payload"), Env: []string{"CODEX_SECRET=secret"},
	})
	if err == nil {
		t.Fatalf("unknown Codex WorkDir %q was accepted", unknownWorkDir)
	}
	if strings.Contains(err.Error(), "secret") {
		t.Fatalf("unknown Codex route error leaked request payload or environment: %v", err)
	}
	if got := runner.CallCount(); got != 0 {
		t.Fatalf("Codex calls after unknown route rejection = %d, want zero", got)
	}
}

var _ platformprocess.CommandRunner = (*codexSharedCommandRunner)(nil)
