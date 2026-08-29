package guards

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryinterfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const sharedGuardFixtureShutdownTimeout = 15 * time.Second

var (
	sharedGuardFixtureOnce sync.Once
	sharedGuardFixture     *sharedGuardProcessFixture
	sharedGuardFixtureErr  error
)

// TestMain owns the one production-composed process and loopback API for this
// package. Scenarios own only their Factory directories, routes, and explicit
// Factory Sessions.
func TestMain(m *testing.M) {
	code := m.Run()
	if sharedGuardFixture != nil {
		if err := sharedGuardFixture.close(); err != nil {
			fmt.Fprintf(os.Stderr, "close shared Petri guard fixture: %v\n", err)
			if code == 0 {
				code = 1
			}
		}
	}
	os.Exit(code)
}

type sharedGuardProcessFixture struct {
	rootDir      string
	homeDir      string
	bootstrapDir string
	baseURL      string

	process    support.ApplicationProcess
	command    *sharedGuardHostedCommand
	api        *support.ProcessAPIServer
	apiStarter *sharedGuardAPIServerStarter
	router     *sharedGuardCommandRouter
	dispatches *sharedGuardDispatchRecorder

	requestSequence atomic.Uint64
	sessionMu       sync.Mutex
	openedSessions  map[string]struct{}
	closedSessions  map[string]struct{}
}

type sharedGuardAPIServerStarter struct {
	api    *support.ProcessAPIServer
	starts atomic.Int32
}

type sharedGuardHostedCommand struct {
	cancel context.CancelFunc
	done   chan struct{}

	mu  sync.Mutex
	err error
}

func sharedGuardProcess(t testing.TB) *sharedGuardProcessFixture {
	t.Helper()
	sharedGuardFixtureOnce.Do(func() {
		sharedGuardFixture, sharedGuardFixtureErr = newSharedGuardProcessFixture(t)
	})
	if sharedGuardFixtureErr != nil {
		t.Fatalf("start shared Petri guard fixture: %v", sharedGuardFixtureErr)
	}
	if sharedGuardFixture == nil {
		t.Fatal("shared Petri guard fixture is unavailable")
	}
	return sharedGuardFixture
}

func newSharedGuardProcessFixture(t testing.TB) (*sharedGuardProcessFixture, error) {
	t.Helper()

	rootDir, err := os.MkdirTemp("", "you-functional-petri-guards-")
	if err != nil {
		return nil, fmt.Errorf("create fixture root: %w", err)
	}
	cleanupRoot := func() { _ = os.RemoveAll(rootDir) }
	homeDir := filepath.Join(rootDir, "home")
	bootstrapDir := filepath.Join(rootDir, "bootstrap")
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		cleanupRoot()
		return nil, fmt.Errorf("create fixture home: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(rootDir, "scenarios"), 0o755); err != nil {
		cleanupRoot()
		return nil, fmt.Errorf("create scenario root: %w", err)
	}
	if err := writeSharedGuardFactory(bootstrapDir, sharedGuardBootstrapFactoryConfig()); err != nil {
		cleanupRoot()
		return nil, fmt.Errorf("write bootstrap Factory: %w", err)
	}

	api := support.NewProcessAPIServer()
	apiStarter := &sharedGuardAPIServerStarter{api: api}
	router := newSharedGuardCommandRouter()
	dispatches := newSharedGuardDispatchRecorder()
	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		APIServerStarter:      apiStarter.start,
		ProviderCommandRunner: router,
		ScriptCommandRunner:   router,
		DispatchRecorder:      dispatches.Record,
	})
	if err != nil {
		cleanupRoot()
		return nil, fmt.Errorf("build root process: %w", err)
	}

	fixture := &sharedGuardProcessFixture{
		rootDir:        rootDir,
		homeDir:        homeDir,
		bootstrapDir:   bootstrapDir,
		process:        process,
		api:            api,
		apiStarter:     apiStarter,
		router:         router,
		dispatches:     dispatches,
		openedSessions: make(map[string]struct{}),
		closedSessions: make(map[string]struct{}),
	}
	inputs := support.FakeInputs(context.Background(), []string{
		"you", "run",
		"--dir", bootstrapDir,
		"--continuously",
		"--with-server",
		"--server", "http://127.0.0.1:1",
		"--quiet",
		"--no-record",
	})
	inputs.Input.Env = []string{"HOME=" + homeDir, "USERPROFILE=" + homeDir}
	inputs.Input.WorkingDirectory = bootstrapDir
	fixture.command = startSharedGuardHostedCommand(process, inputs.Input)

	baseURL, err := api.WaitForBaseURL(sharedGuardFixtureShutdownTimeout)
	if err != nil {
		_ = fixture.close()
		cleanupRoot()
		return nil, fmt.Errorf("wait for loopback API: %w", err)
	}
	fixture.baseURL = baseURL
	support.WaitForStatus(t, baseURL, sharedGuardFixtureShutdownTimeout, func(status factoryapi.StatusResponse) bool {
		return strings.TrimSpace(status.RuntimeStatus) != ""
	})
	return fixture, nil
}

func (starter *sharedGuardAPIServerStarter) start(
	ctx context.Context,
	request platformhttpserver.StartRequest,
) error {
	starter.starts.Add(1)
	return starter.api.Start(ctx, request)
}

func startSharedGuardHostedCommand(process support.Process, input root.Input) *sharedGuardHostedCommand {
	parent := input.Context
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	input.Context = ctx
	command := &sharedGuardHostedCommand{cancel: cancel, done: make(chan struct{})}
	go func() {
		err := process.Execute(input)
		command.mu.Lock()
		command.err = err
		command.mu.Unlock()
		close(command.done)
	}()
	return command
}

func (fixture *sharedGuardProcessFixture) close() error {
	if fixture == nil {
		return nil
	}
	var closeErr error
	if fixture.command != nil {
		closeErr = fixture.command.stop()
	}
	if fixture.process != nil {
		ctx, cancel := context.WithTimeout(context.Background(), sharedGuardFixtureShutdownTimeout)
		closeErr = errors.Join(closeErr, fixture.process.Close(ctx))
		cancel()
	}
	if fixture.baseURL != "" {
		client := http.Client{Timeout: time.Second}
		response, err := client.Get(fixture.baseURL + "/status")
		if err == nil {
			_ = response.Body.Close()
			closeErr = errors.Join(closeErr, errors.New("shared guard API port remains reachable after process close"))
		}
	}
	if fixture.apiStarter != nil && fixture.apiStarter.starts.Load() != 1 {
		closeErr = errors.Join(closeErr, fmt.Errorf(
			"guard fixture API server starts = %d, want exactly one",
			fixture.apiStarter.starts.Load(),
		))
	}
	if fixture.router != nil && fixture.router.routeCount() != 0 {
		closeErr = errors.Join(closeErr, fmt.Errorf(
			"shared guard command routes remaining after cleanup = %d",
			fixture.router.routeCount(),
		))
	}
	closeErr = errors.Join(closeErr, fixture.sessionLifecycleError())
	if fixture.rootDir != "" {
		if err := os.RemoveAll(fixture.rootDir); err != nil {
			closeErr = errors.Join(closeErr, fmt.Errorf("remove fixture root: %w", err))
		}
		if _, err := os.Stat(fixture.rootDir); err == nil {
			closeErr = errors.Join(closeErr, fmt.Errorf("guard fixture root still exists: %s", fixture.rootDir))
		} else if !os.IsNotExist(err) {
			closeErr = errors.Join(closeErr, fmt.Errorf("probe removed guard fixture root: %w", err))
		}
	}
	fmt.Fprintf(os.Stdout, "PETRI_GUARD_SHARED_RUNTIME processStarts=1 apiStarts=%d sessionsOpened=%d sessionsClosed=%d routes=0\n",
		fixture.apiStarter.starts.Load(), len(fixture.openedSessions), len(fixture.closedSessions))
	return closeErr
}

func (command *sharedGuardHostedCommand) stop() error {
	if command == nil {
		return nil
	}
	command.cancel()
	// Hosted shutdown has no completion channel exposed by the public process
	// contract; this bounded wait is cleanup protection for a missing exit, not
	// a workflow delay or polling-based readiness assertion.
	select {
	case <-command.done:
		command.mu.Lock()
		err := command.err
		command.mu.Unlock()
		if err != nil && !errors.Is(err, context.Canceled) {
			return err
		}
		return nil
	case <-time.After(sharedGuardFixtureShutdownTimeout):
		return errors.New("timed out waiting for shared guard host shutdown")
	}
}

func (fixture *sharedGuardProcessFixture) sessionLifecycleError() error {
	fixture.sessionMu.Lock()
	defer fixture.sessionMu.Unlock()
	if len(fixture.openedSessions) != len(fixture.closedSessions) {
		return fmt.Errorf(
			"shared guard Factory Session lifecycle opened %d sessions but closed %d",
			len(fixture.openedSessions), len(fixture.closedSessions),
		)
	}
	for sessionID := range fixture.openedSessions {
		if _, closed := fixture.closedSessions[sessionID]; !closed {
			return fmt.Errorf("shared guard Factory Session %q was not closed", sessionID)
		}
	}
	return nil
}

func (fixture *sharedGuardProcessFixture) recordSessionOpened(sessionID string) error {
	fixture.sessionMu.Lock()
	defer fixture.sessionMu.Unlock()
	if _, exists := fixture.openedSessions[sessionID]; exists {
		return fmt.Errorf("Factory Session %q was opened twice", sessionID)
	}
	fixture.openedSessions[sessionID] = struct{}{}
	return nil
}

func (fixture *sharedGuardProcessFixture) recordSessionClosed(sessionID string) {
	fixture.sessionMu.Lock()
	defer fixture.sessionMu.Unlock()
	fixture.closedSessions[sessionID] = struct{}{}
}

func (fixture *sharedGuardProcessFixture) nextRequestID() string {
	return fmt.Sprintf("guard-session-request-%d", fixture.requestSequence.Add(1))
}

type sharedGuardCommandResponder func(
	context.Context,
	platformprocess.CommandRequest,
) (platformprocess.CommandResult, error)

type sharedGuardRouteConfig struct {
	provider sharedGuardCommandResponder
	script   sharedGuardCommandResponder
}

type sharedGuardCommandRoute struct {
	factoryDir    string
	calls         int
	providerCalls int
	scriptCalls   int
	requests      []platformprocess.CommandRequest
	provider      sharedGuardCommandResponder
	script        sharedGuardCommandResponder
}

type sharedGuardCommandRouter struct {
	mu      sync.Mutex
	routes  map[string]*sharedGuardCommandRoute
	history map[string]sharedGuardCommandRoute
}

func newSharedGuardCommandRouter() *sharedGuardCommandRouter {
	return &sharedGuardCommandRouter{
		routes:  make(map[string]*sharedGuardCommandRoute),
		history: make(map[string]sharedGuardCommandRoute),
	}
}

func (router *sharedGuardCommandRouter) register(factoryDir string, config sharedGuardRouteConfig) error {
	factoryDir = filepath.Clean(factoryDir)
	if factoryDir == "." || strings.TrimSpace(factoryDir) == "" {
		return errors.New("shared guard route Factory directory is required")
	}
	router.mu.Lock()
	defer router.mu.Unlock()
	if _, exists := router.routes[factoryDir]; exists {
		return fmt.Errorf("shared guard route for %q is already registered", factoryDir)
	}
	router.routes[factoryDir] = &sharedGuardCommandRoute{
		factoryDir: factoryDir,
		provider:   config.provider,
		script:     config.script,
	}
	return nil
}

func (router *sharedGuardCommandRouter) unregister(factoryDir string) {
	factoryDir = filepath.Clean(factoryDir)
	router.mu.Lock()
	if route := router.routes[factoryDir]; route != nil {
		router.history[factoryDir] = cloneSharedGuardRoute(*route)
	}
	delete(router.routes, factoryDir)
	router.mu.Unlock()
}

func (router *sharedGuardCommandRouter) routeCount() int {
	router.mu.Lock()
	defer router.mu.Unlock()
	return len(router.routes)
}

func (router *sharedGuardCommandRouter) callsFor(factoryDir string) int {
	router.mu.Lock()
	defer router.mu.Unlock()
	if route := router.routes[filepath.Clean(factoryDir)]; route != nil {
		return route.calls
	}
	return router.history[filepath.Clean(factoryDir)].calls
}

func (router *sharedGuardCommandRouter) providerCallsFor(factoryDir string) int {
	router.mu.Lock()
	defer router.mu.Unlock()
	if route := router.routes[filepath.Clean(factoryDir)]; route != nil {
		return route.providerCalls
	}
	return router.history[filepath.Clean(factoryDir)].providerCalls
}

func (router *sharedGuardCommandRouter) requestsFor(factoryDir string) []platformprocess.CommandRequest {
	router.mu.Lock()
	defer router.mu.Unlock()
	route := router.routes[filepath.Clean(factoryDir)]
	if route == nil {
		archived := router.history[filepath.Clean(factoryDir)]
		route = &archived
	}
	requests := make([]platformprocess.CommandRequest, len(route.requests))
	for index, request := range route.requests {
		requests[index] = cloneGuardCommandRequest(request)
	}
	return requests
}

func (router *sharedGuardCommandRouter) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	if err := ctx.Err(); err != nil {
		return platformprocess.CommandResult{}, err
	}
	router.mu.Lock()
	route := router.routeForRequest(request)
	var responder sharedGuardCommandResponder
	if route != nil {
		route.calls++
		route.requests = append(route.requests, cloneGuardCommandRequest(request))
		if isSharedGuardProviderCommand(request.Command) {
			route.providerCalls++
			responder = route.provider
		} else {
			route.scriptCalls++
			responder = route.script
		}
	}
	router.mu.Unlock()
	if route == nil {
		return platformprocess.CommandResult{}, errors.New("no shared guard command route matched the request")
	}
	if responder != nil {
		return responder(ctx, request)
	}
	if isSharedGuardProviderCommand(request.Command) {
		return platformprocess.CommandResult{
			Stdout: sharedGuardProviderStdout(request.Command, "Done. <COMPLETE> COMPLETE ACCEPTED"),
		}, nil
	}
	return platformprocess.CommandResult{Stdout: []byte("guard-script-output-ok")}, nil
}

func (router *sharedGuardCommandRouter) routeForRequest(request platformprocess.CommandRequest) *sharedGuardCommandRoute {
	var best *sharedGuardCommandRoute
	for factoryDir, route := range router.routes {
		if !sharedGuardPathBelongsTo(factoryDir, request.WorkDir) &&
			!sharedGuardPathBelongsTo(factoryDir, request.Command) {
			continue
		}
		if best == nil || len(factoryDir) > len(best.factoryDir) {
			best = route
		}
	}
	return best
}

func sharedGuardPathBelongsTo(factoryDir, candidate string) bool {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" || !filepath.IsAbs(candidate) {
		return false
	}
	relative, err := filepath.Rel(factoryDir, filepath.Clean(candidate))
	if err != nil {
		return false
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))
}

func cloneSharedGuardRoute(route sharedGuardCommandRoute) sharedGuardCommandRoute {
	route.requests = make([]platformprocess.CommandRequest, len(route.requests))
	for index, request := range route.requests {
		route.requests[index] = cloneGuardCommandRequest(request)
	}
	return route
}

func cloneGuardCommandRequest(request platformprocess.CommandRequest) platformprocess.CommandRequest {
	request.Args = append([]string(nil), request.Args...)
	request.Stdin = append([]byte(nil), request.Stdin...)
	request.Env = append([]string(nil), request.Env...)
	return request
}

func isSharedGuardProviderCommand(command string) bool {
	switch strings.ToLower(filepath.Base(strings.TrimSpace(command))) {
	case "codex", "claude", "agy":
		return true
	default:
		return false
	}
}

func sharedGuardProviderStdout(command, result string) []byte {
	if strings.EqualFold(filepath.Base(strings.TrimSpace(command)), "claude") {
		return support.ClaudeSuccessStdout(result)
	}
	return support.CodexSuccessStdout(result)
}

type sharedGuardCommandResponse struct {
	result              platformprocess.CommandResult
	err                 error
	providerOutput      string
	shapeProviderOutput bool
}

func sharedGuardProviderOutput(content string) sharedGuardCommandResponse {
	return sharedGuardCommandResponse{providerOutput: content, shapeProviderOutput: true}
}

func sharedGuardProviderSequence(responses ...sharedGuardCommandResponse) sharedGuardCommandResponder {
	var mu sync.Mutex
	next := 0
	return func(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
		if err := ctx.Err(); err != nil {
			return platformprocess.CommandResult{}, err
		}
		mu.Lock()
		if next >= len(responses) {
			mu.Unlock()
			return platformprocess.CommandResult{}, fmt.Errorf("shared guard response sequence exhausted for %q", filepath.Base(request.Command))
		}
		response := responses[next]
		next++
		mu.Unlock()
		result := response.result
		result.Stdout = append([]byte(nil), result.Stdout...)
		result.Stderr = append([]byte(nil), result.Stderr...)
		if response.shapeProviderOutput {
			result.Stdout = sharedGuardProviderStdout(request.Command, response.providerOutput)
		}
		return result, response.err
	}
}

func sharedGuardFixedProviderOutput(content string) sharedGuardCommandResponder {
	return func(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
		if err := ctx.Err(); err != nil {
			return platformprocess.CommandResult{}, err
		}
		return platformprocess.CommandResult{Stdout: sharedGuardProviderStdout(request.Command, content)}, nil
	}
}

type sharedGuardCommandGate struct {
	started  chan struct{}
	release  chan struct{}
	canceled chan struct{}

	startOnce   sync.Once
	releaseOnce sync.Once
	cancelOnce  sync.Once
}

func newSharedGuardCommandGate() *sharedGuardCommandGate {
	return &sharedGuardCommandGate{
		started:  make(chan struct{}),
		release:  make(chan struct{}),
		canceled: make(chan struct{}),
	}
}

func (gate *sharedGuardCommandGate) responder(content string) sharedGuardCommandResponder {
	return func(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
		if err := gate.wait(ctx); err != nil {
			return platformprocess.CommandResult{}, err
		}
		return platformprocess.CommandResult{Stdout: sharedGuardProviderStdout(request.Command, content)}, nil
	}
}

func (gate *sharedGuardCommandGate) responderResult(result platformprocess.CommandResult) sharedGuardCommandResponder {
	return func(ctx context.Context, _ platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
		if err := gate.wait(ctx); err != nil {
			return platformprocess.CommandResult{}, err
		}
		result.Stdout = append([]byte(nil), result.Stdout...)
		result.Stderr = append([]byte(nil), result.Stderr...)
		return result, nil
	}
}

func (gate *sharedGuardCommandGate) wait(ctx context.Context) error {
	gate.startOnce.Do(func() { close(gate.started) })
	select {
	case <-gate.release:
		return nil
	case <-ctx.Done():
		gate.cancelOnce.Do(func() { close(gate.canceled) })
		return ctx.Err()
	}
}

func (gate *sharedGuardCommandGate) releaseResponse() {
	gate.releaseOnce.Do(func() { close(gate.release) })
}

type sharedGuardSession struct {
	fixture    *sharedGuardProcessFixture
	factoryDir string
	sessionID  string

	closeOnce sync.Once
}

func supportWaitForGuardTerminal(t *testing.T, session *sharedGuardSession) {
	t.Helper()
	// Terminal completion is exposed by the public Factory Session status
	// projection, not by the controlled Worker edge. The shared helper's
	// bounded observation is therefore a readiness barrier, not a workflow
	// delay.
	support.WaitForSessionTerminalStatus(
		t,
		session.fixture.baseURL,
		session.sessionID,
		sharedGuardFixtureShutdownTimeout,
	)
}

func readSharedGuardSession(
	t testing.TB,
	session *sharedGuardSession,
) (factoryapi.FactorySession, factoryapi.ListWorkResponse, []factoryapi.FactoryEvent) {
	t.Helper()
	response := support.GetJSON[factoryapi.FactorySessionGetResponse](
		t,
		strings.TrimSuffix(session.fixture.baseURL, "/")+"/factory-sessions/"+url.PathEscape(session.sessionID),
	)
	publicSession, err := response.AsFactorySession()
	if err != nil {
		t.Fatalf("decode guard Factory Session %q: %v", session.sessionID, err)
	}
	workList := support.GetJSON[factoryapi.ListWorkResponse](
		t,
		strings.TrimSuffix(session.fixture.baseURL, "/")+"/factory-sessions/"+url.PathEscape(session.sessionID)+"/work",
	)
	events := support.GetFactoryEventsForSessionAt(t, session.fixture.baseURL, session.sessionID)
	return publicSession, workList, events
}

func openSharedGuardSession(t *testing.T, factoryDir string, config sharedGuardRouteConfig) *sharedGuardSession {
	t.Helper()
	fixture := sharedGuardProcess(t)
	if err := fixture.router.register(factoryDir, config); err != nil {
		t.Fatalf("register shared guard command route: %v", err)
	}
	opened := support.OpenFactorySessionAt(t, fixture.baseURL, factoryDir)
	sessionID := opened.Session.Id
	if sessionID == "" {
		fixture.router.unregister(factoryDir)
		t.Fatal("opened shared guard Factory Session has empty ID")
	}
	if err := fixture.recordSessionOpened(sessionID); err != nil {
		support.CloseFactorySessionAt(t, fixture.baseURL, sessionID)
		fixture.router.unregister(factoryDir)
		t.Fatalf("record opened guard Factory Session: %v", err)
	}
	session := &sharedGuardSession{
		fixture:    fixture,
		factoryDir: filepath.Clean(factoryDir),
		sessionID:  sessionID,
	}
	t.Cleanup(func() { session.close(t) })
	return session
}

func (session *sharedGuardSession) close(t testing.TB) {
	if session == nil {
		return
	}
	session.closeOnce.Do(func() {
		support.CloseFactorySessionAt(t, session.fixture.baseURL, session.sessionID)
		session.fixture.router.unregister(session.factoryDir)
		if err := os.RemoveAll(session.factoryDir); err != nil {
			t.Errorf("remove guard scenario Factory %s: %v", session.factoryDir, err)
		}
		session.fixture.recordSessionClosed(session.sessionID)
	})
}

func newSharedGuardScenario(t *testing.T, config map[string]any) string {
	t.Helper()
	fixture := sharedGuardProcess(t)
	dir, err := os.MkdirTemp(filepath.Join(fixture.rootDir, "scenarios"), "factory-")
	if err != nil {
		t.Fatalf("create guard scenario Factory: %v", err)
	}
	if err := writeSharedGuardFactory(dir, config); err != nil {
		_ = os.RemoveAll(dir)
		t.Fatalf("write guard scenario Factory: %v", err)
	}
	workers, _ := config["workers"].([]map[string]string)
	for _, worker := range workers {
		name := worker["name"]
		if name != "" {
			support.WriteAgentConfig(t, dir, name, support.BuildModelWorkerConfig("codex", "gpt-5-codex"))
		}
	}
	workstations, _ := config["workstations"].([]map[string]any)
	for _, workstation := range workstations {
		name, _ := workstation["name"].(string)
		kind, _ := workstation["type"].(string)
		if name == "" || strings.EqualFold(kind, string(factoryinterfaces.WorkstationTypeLogical)) {
			continue
		}
		support.WriteWorkstationConfig(t, dir, name, "---\ntype: MODEL_WORKSTATION\n---\nDo the work.\n")
	}
	return dir
}

func writeSharedGuardFactory(dir string, config map[string]any) error {
	raw, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("marshal Factory config: %w", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create Factory directory: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, factoryinterfaces.FactoryConfigFile), raw, 0o644); err != nil {
		return fmt.Errorf("write factory.json: %w", err)
	}
	return nil
}

func sharedGuardBootstrapFactoryConfig() map[string]any {
	return map[string]any{
		"name": "shared-petri-guard-bootstrap",
		"workTypes": []map[string]any{{
			"name":   "bootstrap",
			"states": []map[string]string{{"name": "ready", "type": "INITIAL"}},
		}},
		"workers":      []map[string]string{},
		"workstations": []map[string]any{},
	}
}

type sharedGuardDispatchRecorder struct {
	mu      sync.Mutex
	records []recordings.FactoryDispatchRecord
}

func newSharedGuardDispatchRecorder() *sharedGuardDispatchRecorder {
	return &sharedGuardDispatchRecorder{}
}

func (recorder *sharedGuardDispatchRecorder) Record(record recordings.FactoryDispatchRecord) {
	record.Dispatch = work.CloneWorkDispatch(record.Dispatch)
	record.ConsumedTokens = append([]string(nil), record.ConsumedTokens...)
	recorder.mu.Lock()
	recorder.records = append(recorder.records, record)
	recorder.mu.Unlock()
}

func (recorder *sharedGuardDispatchRecorder) Snapshot() []recordings.FactoryDispatchRecord {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	result := make([]recordings.FactoryDispatchRecord, len(recorder.records))
	for index, record := range recorder.records {
		result[index] = record
		result[index].Dispatch = work.CloneWorkDispatch(record.Dispatch)
		result[index].ConsumedTokens = append([]string(nil), record.ConsumedTokens...)
	}
	return result
}

func (recorder *sharedGuardDispatchRecorder) forTransition(transitionID string) []recordings.FactoryDispatchRecord {
	var matches []recordings.FactoryDispatchRecord
	for _, record := range recorder.Snapshot() {
		if record.Dispatch.TransitionID == transitionID {
			matches = append(matches, record)
		}
	}
	return matches
}
