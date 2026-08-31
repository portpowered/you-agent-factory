package claude

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const claudeSharedInvocationTimeout = 30 * time.Second

const claudeSharedRouteCount = 6

type haikuGoldenReplayCase struct {
	golden                 haikuGoldenCase
	factoryDir             string
	stdout                 []byte
	blockUntilCancellation bool
	started                chan struct{}
}

func prepareHaikuGoldenReplayCases(t *testing.T, manifest haikuGoldenManifest) []haikuGoldenReplayCase {
	t.Helper()
	cases := make([]haikuGoldenReplayCase, 0, len(manifest.Cases))
	for _, golden := range manifest.Cases {
		cases = append(cases, prepareHaikuGoldenReplayCase(t, golden))
	}
	return cases
}

func prepareHaikuGoldenReplayCase(t *testing.T, golden haikuGoldenCase) haikuGoldenReplayCase {
	t.Helper()
	stdout := loadHaikuGoldenStdout(t, golden)
	assertHaikuGoldenNativeShape(t, stdout, golden)
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	return configureHaikuGoldenReplayCase(t, golden, dir, stdout)
}

func prepareHaikuGoldenReplayCaseAt(
	t *testing.T,
	golden haikuGoldenCase,
	dir string,
) haikuGoldenReplayCase {
	t.Helper()
	stdout := loadHaikuGoldenStdout(t, golden)
	assertHaikuGoldenNativeShape(t, stdout, golden)
	copyClaudeDirectory(t, support.LegacyFixtureDir(t, "executor_success"), dir)
	support.ClearSeedInputs(t, dir)
	return configureHaikuGoldenReplayCase(t, golden, dir, stdout)
}

func configureHaikuGoldenReplayCase(
	t *testing.T,
	golden haikuGoldenCase,
	dir string,
	stdout []byte,
) haikuGoldenReplayCase {
	t.Helper()
	support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(
		modelprovider.ProviderClaude,
		golden.Selector,
	))
	support.WriteWorkstationConfig(t, dir, "process", haikuGoldenWorkstationConfig(dir))
	return haikuGoldenReplayCase{
		golden:     golden,
		factoryDir: normalizeHaikuGoldenRouteDirectory(dir),
		stdout:     append([]byte(nil), stdout...),
	}
}

func haikuGoldenWorkstationConfig(factoryDir string) string {
	return fmt.Sprintf(`---
type: MODEL_WORKSTATION
workingDirectory: %s
---

Test workstation.
`, strconv.Quote(filepath.ToSlash(factoryDir)))
}

// haikuGoldenCommandRouter is constructed completely before the application
// process starts. Its directory map is immutable during dispatch; the mutex
// only protects request witnesses because the runtime owns asynchronous work.
type haikuGoldenCommandRouter struct {
	mu       sync.Mutex
	routes   map[string]haikuGoldenRoute
	requests []platformprocess.CommandRequest
	active   int
	closed   bool
}

type haikuGoldenRoute struct {
	selector               string
	result                 platformprocess.CommandResult
	blockUntilCancellation bool
	started                chan struct{}
	startedOnce            *sync.Once
}

func newHaikuGoldenCommandRouter(t *testing.T, cases []haikuGoldenReplayCase) *haikuGoldenCommandRouter {
	t.Helper()
	router, err := buildHaikuGoldenCommandRouter(cases)
	if err != nil {
		t.Fatal(err)
	}
	return router
}

func buildHaikuGoldenCommandRouter(cases []haikuGoldenReplayCase) (*haikuGoldenCommandRouter, error) {
	return buildHaikuCommandRouter(cases, true)
}

func buildHaikuSharedCommandRouter(cases []haikuGoldenReplayCase) (*haikuGoldenCommandRouter, error) {
	return buildHaikuCommandRouter(cases, false)
}

func buildHaikuCommandRouter(
	cases []haikuGoldenReplayCase,
	rejectDuplicateSelectors bool,
) (*haikuGoldenCommandRouter, error) {
	routes := make(map[string]haikuGoldenRoute, len(cases))
	selectors := make(map[string]struct{}, len(cases))
	for _, replayCase := range cases {
		if _, exists := routes[replayCase.factoryDir]; exists {
			return nil, errors.New("duplicate Claude golden Factory directory route")
		}
		if replayCase.golden.Selector == "" {
			return nil, errors.New("Claude golden selector is required")
		}
		if rejectDuplicateSelectors {
			if _, exists := selectors[replayCase.golden.Selector]; exists {
				return nil, errors.New("duplicate Claude golden selector route")
			}
			selectors[replayCase.golden.Selector] = struct{}{}
		}
		var startedOnce *sync.Once
		if replayCase.started != nil {
			startedOnce = &sync.Once{}
		}
		routes[replayCase.factoryDir] = haikuGoldenRoute{
			selector: replayCase.golden.Selector,
			result: platformprocess.CommandResult{
				Stdout: append([]byte(nil), replayCase.stdout...),
			},
			blockUntilCancellation: replayCase.blockUntilCancellation,
			started:                replayCase.started,
			startedOnce:            startedOnce,
		}
	}
	return &haikuGoldenCommandRouter{routes: routes}, nil
}

func (r *haikuGoldenCommandRouter) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return platformprocess.CommandResult{}, errors.New("Claude golden route is closed")
	}
	if request.Command != string(modelprovider.ProviderClaude) {
		r.mu.Unlock()
		return platformprocess.CommandResult{}, errors.New("Claude golden route rejected unexpected provider command")
	}
	route, ok := r.routes[normalizeHaikuGoldenRouteDirectory(request.WorkDir)]
	if !ok {
		r.mu.Unlock()
		return platformprocess.CommandResult{}, errors.New("Claude golden route unavailable")
	}
	if !haikuGoldenRequestSelects(request.Args, route.selector) {
		r.mu.Unlock()
		return platformprocess.CommandResult{}, errors.New("Claude golden route rejected selector")
	}
	r.requests = append(r.requests, cloneHaikuCommandRequest(request))
	r.active++
	result := cloneHaikuCommandResult(route.result)
	if route.started != nil {
		if route.startedOnce != nil {
			route.startedOnce.Do(func() { close(route.started) })
		} else {
			close(route.started)
		}
	}
	r.mu.Unlock()
	defer r.finishCall()
	if !route.blockUntilCancellation {
		return result, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	<-ctx.Done()
	result.Stdout = nil
	return result, ctx.Err()
}

func (r *haikuGoldenCommandRouter) finishCall() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.active--
}

func haikuGoldenRequestSelects(args []string, selector string) bool {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == "--model" && args[index+1] == selector {
			return true
		}
	}
	return false
}

func (r *haikuGoldenCommandRouter) Requests() []platformprocess.CommandRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	requests := make([]platformprocess.CommandRequest, len(r.requests))
	for index, request := range r.requests {
		requests[index] = cloneHaikuCommandRequest(request)
	}
	return requests
}

func (r *haikuGoldenCommandRouter) CallsFor(factoryDir string) int {
	want := normalizeHaikuGoldenRouteDirectory(factoryDir)
	r.mu.Lock()
	defer r.mu.Unlock()
	count := 0
	for _, request := range r.requests {
		if normalizeHaikuGoldenRouteDirectory(request.WorkDir) == want {
			count++
		}
	}
	return count
}

func (r *haikuGoldenCommandRouter) RequestFor(factoryDir string) (platformprocess.CommandRequest, bool) {
	want := normalizeHaikuGoldenRouteDirectory(factoryDir)
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, request := range r.requests {
		if normalizeHaikuGoldenRouteDirectory(request.WorkDir) == want {
			return cloneHaikuCommandRequest(request), true
		}
	}
	return platformprocess.CommandRequest{}, false
}

func (r *haikuGoldenCommandRouter) RouteCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.routes)
}

func (r *haikuGoldenCommandRouter) ActiveCallCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.active
}

func (r *haikuGoldenCommandRouter) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed = true
	r.routes = nil
}

func normalizeHaikuGoldenRouteDirectory(directory string) string {
	absolute, err := filepath.Abs(directory)
	if err == nil {
		directory = absolute
	}
	return filepath.Clean(directory)
}

func cloneHaikuCommandRequest(request platformprocess.CommandRequest) platformprocess.CommandRequest {
	request.Args = append([]string(nil), request.Args...)
	request.Stdin = append([]byte(nil), request.Stdin...)
	request.Env = append([]string(nil), request.Env...)
	return request
}

func cloneHaikuCommandResult(result platformprocess.CommandResult) platformprocess.CommandResult {
	result.Stdout = append([]byte(nil), result.Stdout...)
	result.Stderr = append([]byte(nil), result.Stderr...)
	return result
}

func sessionWorkURL(baseURL, sessionID string) string {
	return strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID) + "/work"
}

var _ platformprocess.CommandRunner = (*haikuGoldenCommandRouter)(nil)

// claudeSharedHTTPServer owns the one listener started by the package-scoped
// Claude process. Its wrapper records the listener lifecycle so package
// finalization can prove that the shared server closed after the host stopped.
type claudeSharedHTTPServer struct {
	server *support.ProcessAPIServer

	mu       sync.Mutex
	starts   int
	done     chan struct{}
	doneOnce sync.Once
}

func newClaudeSharedHTTPServer() *claudeSharedHTTPServer {
	return &claudeSharedHTTPServer{
		server: support.NewProcessAPIServer(),
		done:   make(chan struct{}),
	}
}

func (server *claudeSharedHTTPServer) start(
	ctx context.Context,
	request platformhttpserver.StartRequest,
) error {
	server.mu.Lock()
	server.starts++
	server.mu.Unlock()
	defer server.doneOnce.Do(func() { close(server.done) })
	return server.server.Start(ctx, request)
}

func (server *claudeSharedHTTPServer) startCount() int {
	server.mu.Lock()
	defer server.mu.Unlock()
	return server.starts
}

func (server *claudeSharedHTTPServer) waitClosed(ctx context.Context) error {
	select {
	case <-server.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// claudeSharedProcessCommand owns the long-lived host invocation without
// tying cleanup to the first test that initializes the package fixture.
type claudeSharedProcessCommand struct {
	cancel context.CancelFunc
	done   chan struct{}

	mu  sync.Mutex
	err error

	stopOnce sync.Once
}

func startClaudeSharedProcessCommand(
	process support.Process,
	inputs *support.CapturedInputs,
) *claudeSharedProcessCommand {
	parent := inputs.Input.Context
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	inputs.Input.Context = ctx
	command := &claudeSharedProcessCommand{cancel: cancel, done: make(chan struct{})}
	go func() {
		err := process.Execute(inputs.Input)
		command.mu.Lock()
		command.err = err
		command.mu.Unlock()
		close(command.done)
	}()
	return command
}

func (command *claudeSharedProcessCommand) stop(ctx context.Context) error {
	if command == nil {
		return nil
	}
	command.stopOnce.Do(command.cancel)
	select {
	case <-command.done:
		command.mu.Lock()
		err := command.err
		command.mu.Unlock()
		if errors.Is(err, context.Canceled) {
			return nil
		}
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

type claudeSharedProcessFixture struct {
	rootDir     string
	homeDir     string
	hostWorkDir string
	baseURL     string
	process     support.ApplicationProcess
	command     *claudeSharedProcessCommand
	api         *claudeSharedHTTPServer
	runner      *haikuGoldenCommandRouter
	routes      map[string]haikuGoldenReplayCase
	goldenCases []haikuGoldenReplayCase

	processBuilds int
	processCloses int

	sessionMu        sync.Mutex
	openedSessionIDs map[string]struct{}
	activeSessionIDs map[string]struct{}
	hostMu           sync.Mutex

	closeOnce sync.Once
	closeErr  error
}

var claudeSharedFixtureState struct {
	sync.Mutex
	fixture *claudeSharedProcessFixture
}

func claudeSharedProcess(t *testing.T) *claudeSharedProcessFixture {
	t.Helper()
	claudeSharedFixtureState.Lock()
	defer claudeSharedFixtureState.Unlock()
	if claudeSharedFixtureState.fixture != nil {
		return claudeSharedFixtureState.fixture
	}
	fixture := newClaudeSharedProcessFixture(t)
	claudeSharedFixtureState.fixture = fixture
	return fixture
}

func TestMain(m *testing.M) {
	code := m.Run()

	claudeSharedFixtureState.Lock()
	fixture := claudeSharedFixtureState.fixture
	claudeSharedFixtureState.Unlock()
	if fixture != nil {
		if err := fixture.close(); err != nil {
			fmt.Fprintf(os.Stderr, "close shared Claude process fixture: %v\n", err)
			code = 1
		}
	}
	os.Exit(code)
}

func newClaudeSharedProcessFixture(t *testing.T) *claudeSharedProcessFixture {
	t.Helper()
	rootDir, err := os.MkdirTemp("", "you-claude-shared-")
	if err != nil {
		t.Fatalf("create shared Claude fixture root: %v", err)
	}
	fixture := &claudeSharedProcessFixture{
		rootDir:          rootDir,
		homeDir:          filepath.Join(rootDir, "home"),
		hostWorkDir:      filepath.Join(rootDir, "host"),
		api:              newClaudeSharedHTTPServer(),
		routes:           make(map[string]haikuGoldenReplayCase),
		openedSessionIDs: make(map[string]struct{}),
		activeSessionIDs: make(map[string]struct{}),
	}
	defer func() {
		if fixture.process == nil {
			_ = os.RemoveAll(rootDir)
		}
	}()
	if err := os.MkdirAll(fixture.homeDir, 0o755); err != nil {
		t.Fatalf("create shared Claude home %q: %v", fixture.homeDir, err)
	}
	if err := os.MkdirAll(fixture.hostWorkDir, 0o755); err != nil {
		t.Fatalf("create shared Claude host workspace %q: %v", fixture.hostWorkDir, err)
	}

	manifest := loadHaikuGoldenManifest(t)
	allCases := make([]haikuGoldenReplayCase, 0, claudeSharedRouteCount)
	for _, golden := range manifest.Cases {
		name := "golden-" + golden.Name
		replayCase := prepareHaikuGoldenReplayCaseAt(
			t, golden, filepath.Join(rootDir, name),
		)
		fixture.addRoute(name, replayCase)
		fixture.goldenCases = append(fixture.goldenCases, replayCase)
		allCases = append(allCases, replayCase)
	}

	standalone := newClaudeStandaloneReplayCase(t, filepath.Join(rootDir, "standalone"))
	fixture.addRoute("standalone", standalone)
	allCases = append(allCases, standalone)

	partial := prepareHaikuGoldenReplayCaseAt(
		t, manifest.Cases[0], filepath.Join(rootDir, "adverse-partial"),
	)
	partial.stdout = []byte(`{"type":"system","subtype":"init","session_id":"partial-session"}` + "\n")
	fixture.addRoute("adverse-partial", partial)
	allCases = append(allCases, partial)

	cancellation := prepareHaikuGoldenReplayCaseAt(
		t, manifest.Cases[0], filepath.Join(rootDir, "adverse-cancellation"),
	)
	cancellation.blockUntilCancellation = true
	cancellation.started = make(chan struct{})
	fixture.addRoute("adverse-cancellation", cancellation)
	allCases = append(allCases, cancellation)

	prepareClaudeSharedHostFactory(t, fixture.hostWorkDir)
	runner, err := buildHaikuSharedCommandRouter(allCases)
	if err != nil {
		t.Fatalf("build shared Claude command router: %v", err)
	}
	fixture.runner = runner
	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		APIServerStarter:      fixture.api.start,
		ProviderCommandRunner: fixture.runner,
	})
	if err != nil {
		t.Fatalf("BuildProcess(shared Claude fixture): %v", err)
	}
	fixture.process = process
	fixture.processBuilds++
	return fixture
}

func (fixture *claudeSharedProcessFixture) addRoute(
	name string,
	replayCase haikuGoldenReplayCase,
) {
	fixture.routes[name] = replayCase
}

func newClaudeStandaloneReplayCase(t *testing.T, dir string) haikuGoldenReplayCase {
	t.Helper()
	copyClaudeDirectory(t, support.LegacyFixtureDir(t, "executor_success"), dir)
	support.ClearSeedInputs(t, dir)
	support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(
		modelprovider.ProviderClaude,
		claudeFunctionalModel,
	))
	support.WriteWorkstationConfig(t, dir, "process", haikuGoldenWorkstationConfig(dir))
	return haikuGoldenReplayCase{
		golden: haikuGoldenCase{
			Name:     "standalone",
			Selector: claudeFunctionalModel,
		},
		factoryDir: normalizeHaikuGoldenRouteDirectory(dir),
		stdout:     support.ClaudeSuccessStdout("Done. COMPLETE"),
	}
}

func prepareClaudeSharedHostFactory(t *testing.T, dir string) {
	t.Helper()
	copyClaudeDirectory(t, support.LegacyFixtureDir(t, "executor_success"), dir)
	support.ClearSeedInputs(t, dir)
	support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(
		modelprovider.ProviderClaude,
		claudeFunctionalModel,
	))
	support.WriteWorkstationConfig(t, dir, "process", haikuGoldenWorkstationConfig(dir))
}

func copyClaudeDirectory(t *testing.T, sourceDir, targetDir string) {
	t.Helper()
	err := filepath.WalkDir(sourceDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return os.MkdirAll(targetDir, 0o755)
		}
		targetPath := filepath.Join(targetDir, relative)
		if entry.IsDir() {
			return os.MkdirAll(targetPath, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(targetPath, data, 0o644)
	})
	if err != nil {
		t.Fatalf("copy shared Claude fixture %q to %q: %v", sourceDir, targetDir, err)
	}
}

type claudeSharedSession struct {
	routeName string
	id        string
	closed    bool
}

func (fixture *claudeSharedProcessFixture) ensureHosted(t *testing.T) {
	t.Helper()
	fixture.hostMu.Lock()
	defer fixture.hostMu.Unlock()
	if fixture.command != nil {
		return
	}
	inputs := support.FakeInputs(context.Background(), []string{
		"you", "run", "--dir", fixture.hostWorkDir,
		"--continuously", "--with-server", "--server", "http://127.0.0.1:1",
		"--quiet", "--no-record",
	})
	inputs.Input.Env = claudeSharedEnvironment(fixture.homeDir)
	inputs.Input.WorkingDirectory = fixture.hostWorkDir
	fixture.command = startClaudeSharedProcessCommand(fixture.process, inputs)
	fixture.baseURL = fixture.api.server.WaitForURL(t)
	support.WaitForStatus(t, fixture.baseURL, claudeSharedInvocationTimeout, func(status factoryapi.StatusResponse) bool {
		return strings.TrimSpace(status.RuntimeStatus) != ""
	})
}

func claudeSharedEnvironment(homeDir string) []string {
	environment := append([]string(nil), os.Environ()...)
	environment = append(environment, "HOME="+homeDir, "USERPROFILE="+homeDir)
	return environment
}

func (fixture *claudeSharedProcessFixture) route(
	t *testing.T,
	name string,
) haikuGoldenReplayCase {
	t.Helper()
	route, ok := fixture.routes[name]
	if !ok {
		t.Fatalf("shared Claude route %q is missing", name)
	}
	return route
}

func (fixture *claudeSharedProcessFixture) openSession(
	t *testing.T,
	name string,
) *claudeSharedSession {
	t.Helper()
	fixture.ensureHosted(t)
	route := fixture.route(t, name)
	opened := support.OpenFactorySessionAt(t, fixture.baseURL, route.factoryDir)
	if opened.Session == nil || strings.TrimSpace(opened.Session.Id) == "" {
		t.Fatalf("shared Claude Factory Session for %q = %#v, want identity", name, opened)
	}
	sessionID := strings.TrimSpace(opened.Session.Id)
	if sessionID == "~default" {
		t.Fatalf("shared Claude Factory Session for %q = %q, want explicit session", name, sessionID)
	}
	fixture.sessionMu.Lock()
	if _, exists := fixture.openedSessionIDs[sessionID]; exists {
		fixture.sessionMu.Unlock()
		t.Fatalf("duplicate shared Claude Factory Session identity %q", sessionID)
	}
	fixture.openedSessionIDs[sessionID] = struct{}{}
	fixture.activeSessionIDs[sessionID] = struct{}{}
	fixture.sessionMu.Unlock()
	session := &claudeSharedSession{routeName: name, id: sessionID}
	t.Cleanup(func() { fixture.closeSession(t, session) })
	return session
}

func (fixture *claudeSharedProcessFixture) closeSession(t testing.TB, session *claudeSharedSession) {
	t.Helper()
	if session == nil || session.closed {
		return
	}
	support.CloseFactorySessionAt(t, fixture.baseURL, session.id)
	session.closed = true
	fixture.sessionMu.Lock()
	delete(fixture.activeSessionIDs, session.id)
	fixture.sessionMu.Unlock()
}

func (fixture *claudeSharedProcessFixture) submitWork(
	t testing.TB,
	session *claudeSharedSession,
	title string,
) {
	t.Helper()
	if session == nil {
		t.Fatal("submit shared Claude Work: session is nil")
	}
	name := session.routeName
	submitted := support.SubmitSessionWorkAt(t, fixture.baseURL, session.id, factoryapi.SubmitWorkRequest{
		Name:         &name,
		WorkTypeName: "task",
		Payload:      map[string]string{"title": title},
	})
	if submitted.SessionId == nil || *submitted.SessionId != session.id {
		t.Fatalf("shared Claude submitted Work session ID = %#v, want %q", submitted.SessionId, session.id)
	}
	if strings.TrimSpace(submitted.RequestId) == "" || support.StringPointerValue(submitted.WorkId) == "" {
		t.Fatalf("shared Claude submitted Work identity = request:%q work:%q, want both identities", submitted.RequestId, support.StringPointerValue(submitted.WorkId))
	}
}

func (fixture *claudeSharedProcessFixture) close() error {
	fixture.closeOnce.Do(func() {
		var closeErrors []error
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if fixture.processBuilds != 1 {
			closeErrors = append(closeErrors, fmt.Errorf("shared Claude process builds = %d, want 1", fixture.processBuilds))
		}
		fixture.hostMu.Lock()
		command := fixture.command
		fixture.hostMu.Unlock()
		if command != nil {
			if err := command.stop(closeCtx); err != nil {
				closeErrors = append(closeErrors, fmt.Errorf("stop shared Claude host command: %w", err))
			}
			if err := fixture.api.waitClosed(closeCtx); err != nil {
				closeErrors = append(closeErrors, fmt.Errorf("wait for shared Claude listener: %w", err))
			}
		}
		if fixture.api != nil && fixture.api.startCount() != 1 {
			closeErrors = append(closeErrors, fmt.Errorf("shared Claude listener starts = %d, want 1", fixture.api.startCount()))
		}
		if fixture.process != nil {
			fixture.processCloses++
			if err := fixture.process.Close(closeCtx); err != nil {
				closeErrors = append(closeErrors, fmt.Errorf("close shared Claude process: %w", err))
			}
		}
		if fixture.runner != nil {
			if got := fixture.runner.ActiveCallCount(); got != 0 {
				closeErrors = append(closeErrors, fmt.Errorf("shared Claude active command calls = %d, want 0", got))
			}
			fixture.sessionMu.Lock()
			activeSessions := len(fixture.activeSessionIDs)
			fixture.sessionMu.Unlock()
			if activeSessions != 0 {
				closeErrors = append(closeErrors, fmt.Errorf("shared Claude active Factory Sessions = %d, want 0", activeSessions))
			}
			if got := fixture.runner.RouteCount(); got != claudeSharedRouteCount {
				closeErrors = append(closeErrors, fmt.Errorf("shared Claude routes before clear = %d, want %d", got, claudeSharedRouteCount))
			}
			fixture.runner.Close()
			if got := fixture.runner.RouteCount(); got != 0 {
				closeErrors = append(closeErrors, fmt.Errorf("shared Claude routes after clear = %d, want 0", got))
			}
		}
		if fixture.processCloses != 1 {
			closeErrors = append(closeErrors, fmt.Errorf("shared Claude process closes = %d, want 1", fixture.processCloses))
		}
		if fixture.rootDir != "" {
			if err := os.RemoveAll(fixture.rootDir); err != nil {
				closeErrors = append(closeErrors, err)
			} else if _, err := os.Stat(fixture.rootDir); !errors.Is(err, os.ErrNotExist) {
				closeErrors = append(closeErrors, fmt.Errorf("shared Claude fixture root remains: %w", err))
			}
		}
		fixture.routes = nil
		fixture.closeErr = errors.Join(closeErrors...)
	})
	return fixture.closeErr
}
