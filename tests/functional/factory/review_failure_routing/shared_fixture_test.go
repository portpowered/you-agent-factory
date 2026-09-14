package review_failure_routing

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const reviewFailureFixtureShutdownTimeout = 60 * time.Second

var (
	reviewFailureFixtureOnce sync.Once
	reviewFailureFixture     *reviewFailureProcessFixture
	reviewFailureFixtureErr  error
)

// TestMain owns one production-composed process and one loopback API for all
// scenarios in this behavior slice. Each test owns its copied Factory,
// explicit Factory Session, route, and public event stream.
func TestMain(m *testing.M) {
	code := m.Run()
	if reviewFailureFixture != nil {
		if err := reviewFailureFixture.close(); err != nil {
			fmt.Fprintf(os.Stderr, "close shared review-failure fixture: %v\n", err)
			if code == 0 {
				code = 1
			}
		}
	}
	os.Exit(code)
}

type reviewFailureProcessFixture struct {
	rootDir       string
	sourceFactory string
	bootstrapDir  string
	baseURL       string
	process       support.ApplicationProcess
	command       *reviewFailureHostedCommand
	api           *support.ProcessAPIServer
	apiStarter    *reviewFailureAPIServerStarter
	router        *reviewFailureCommandRouter

	nextScenario atomic.Uint64
	sessionMu    sync.Mutex
	opened       map[string]struct{}
	closed       map[string]struct{}
}

type reviewFailureAPIServerStarter struct {
	api    *support.ProcessAPIServer
	starts atomic.Int32
}

type reviewFailureHostedCommand struct {
	cancel context.CancelFunc
	done   chan struct{}

	mu  sync.Mutex
	err error
}

func sharedReviewFailureFixture(t testing.TB) *reviewFailureProcessFixture {
	t.Helper()
	reviewFailureFixtureOnce.Do(func() {
		reviewFailureFixture, reviewFailureFixtureErr = newReviewFailureProcessFixture(t)
	})
	if reviewFailureFixtureErr != nil {
		t.Fatalf("start shared review-failure fixture: %v", reviewFailureFixtureErr)
	}
	if reviewFailureFixture == nil {
		t.Fatal("shared review-failure fixture is unavailable")
	}
	return reviewFailureFixture
}

func newReviewFailureProcessFixture(t testing.TB) (*reviewFailureProcessFixture, error) {
	t.Helper()

	rootDir, err := os.MkdirTemp("", "you-functional-review-failure-")
	if err != nil {
		return nil, fmt.Errorf("create fixture root: %w", err)
	}
	cleanupRoot := func() { _ = os.RemoveAll(rootDir) }
	bootstrapDir := filepath.Join(rootDir, "bootstrap")
	if err := writeReviewFailureBootstrapFactory(bootstrapDir); err != nil {
		cleanupRoot()
		return nil, fmt.Errorf("write bootstrap Factory: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(rootDir, "scenarios"), 0o755); err != nil {
		cleanupRoot()
		return nil, fmt.Errorf("create scenario root: %w", err)
	}

	api := support.NewProcessAPIServer()
	apiStarter := &reviewFailureAPIServerStarter{api: api}
	router := newReviewFailureCommandRouter()
	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		APIServerStarter:      apiStarter.start,
		ProviderCommandRunner: router,
		ScriptCommandRunner:   router,
	})
	if err != nil {
		cleanupRoot()
		return nil, fmt.Errorf("build root process: %w", err)
	}

	fixture := &reviewFailureProcessFixture{
		rootDir:       rootDir,
		sourceFactory: testutil.MustRepoPath(t, "factory"),
		bootstrapDir:  bootstrapDir,
		process:       process,
		api:           api,
		apiStarter:    apiStarter,
		router:        router,
		opened:        make(map[string]struct{}),
		closed:        make(map[string]struct{}),
	}
	inputs := support.FakeInputs(context.Background(), []string{
		"you", "run",
		"--dir", bootstrapDir,
		"--continuously",
		"--with-server",
		"--quiet",
	})
	inputs.Input.WorkingDirectory = bootstrapDir
	fixture.command = startReviewFailureHostedCommand(process, inputs.Input)

	baseURL, err := api.WaitForBaseURL(reviewFailureFixtureShutdownTimeout)
	if err != nil {
		_ = fixture.close()
		cleanupRoot()
		return nil, fmt.Errorf("wait for loopback API: %w", err)
	}
	fixture.baseURL = baseURL
	return fixture, nil
}

func (starter *reviewFailureAPIServerStarter) start(
	ctx context.Context,
	request platformhttpserver.StartRequest,
) error {
	starter.starts.Add(1)
	return starter.api.Start(ctx, request)
}

func startReviewFailureHostedCommand(process support.Process, input root.Input) *reviewFailureHostedCommand {
	parent := input.Context
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	input.Context = ctx
	command := &reviewFailureHostedCommand{cancel: cancel, done: make(chan struct{})}
	go func() {
		err := process.Execute(input)
		command.mu.Lock()
		command.err = err
		command.mu.Unlock()
		close(command.done)
	}()
	return command
}

func (fixture *reviewFailureProcessFixture) close() error {
	if fixture == nil {
		return nil
	}
	var closeErr error
	if fixture.command != nil {
		closeErr = fixture.command.stop()
	}
	if fixture.process != nil {
		ctx, cancel := context.WithTimeout(context.Background(), reviewFailureFixtureShutdownTimeout)
		closeErr = errors.Join(closeErr, fixture.process.Close(ctx))
		cancel()
	}
	if fixture.baseURL != "" {
		client := http.Client{Timeout: time.Second}
		response, err := client.Get(fixture.baseURL + "/status")
		if err == nil {
			_ = response.Body.Close()
			closeErr = errors.Join(closeErr, errors.New("review-failure API port remains reachable after process close"))
		}
	}
	if fixture.apiStarter != nil && fixture.apiStarter.starts.Load() != 1 {
		closeErr = errors.Join(closeErr, fmt.Errorf(
			"review-failure API server starts = %d, want exactly one",
			fixture.apiStarter.starts.Load(),
		))
	}
	if fixture.router != nil && fixture.router.routeCount() != 0 {
		closeErr = errors.Join(closeErr, fmt.Errorf(
			"review-failure command routes remaining after cleanup = %d",
			fixture.router.routeCount(),
		))
	}
	closeErr = errors.Join(closeErr, fixture.sessionLifecycleError())
	if fixture.rootDir != "" {
		if err := os.RemoveAll(fixture.rootDir); err != nil {
			closeErr = errors.Join(closeErr, fmt.Errorf("remove review-failure fixture root: %w", err))
		}
		if _, err := os.Stat(fixture.rootDir); err == nil {
			closeErr = errors.Join(closeErr, fmt.Errorf("review-failure fixture root still exists: %s", fixture.rootDir))
		} else if !os.IsNotExist(err) {
			closeErr = errors.Join(closeErr, fmt.Errorf("probe removed review-failure fixture root: %w", err))
		}
	}
	fmt.Fprintf(os.Stdout, "REVIEW_FAILURE_SHARED_RUNTIME processStarts=1 apiStarts=%d sessionsOpened=%d sessionsClosed=%d routes=0\n",
		fixture.apiStarter.starts.Load(), len(fixture.opened), len(fixture.closed))
	return closeErr
}

func (command *reviewFailureHostedCommand) stop() error {
	if command == nil {
		return nil
	}
	command.cancel()
	select {
	case <-command.done:
		command.mu.Lock()
		err := command.err
		command.mu.Unlock()
		if err != nil && !errors.Is(err, context.Canceled) {
			return err
		}
		return nil
	case <-time.After(reviewFailureFixtureShutdownTimeout):
		return errors.New("timed out waiting for review-failure host shutdown")
	}
}

func (fixture *reviewFailureProcessFixture) sessionOpened(sessionID string) error {
	fixture.sessionMu.Lock()
	defer fixture.sessionMu.Unlock()
	if _, exists := fixture.opened[sessionID]; exists {
		return fmt.Errorf("Factory Session %q was opened twice", sessionID)
	}
	fixture.opened[sessionID] = struct{}{}
	return nil
}

func (fixture *reviewFailureProcessFixture) sessionClosed(sessionID string) {
	fixture.sessionMu.Lock()
	defer fixture.sessionMu.Unlock()
	fixture.closed[sessionID] = struct{}{}
}

func (fixture *reviewFailureProcessFixture) sessionLifecycleError() error {
	fixture.sessionMu.Lock()
	defer fixture.sessionMu.Unlock()
	if len(fixture.opened) != len(fixture.closed) {
		return fmt.Errorf("review-failure Factory Sessions opened %d but closed %d", len(fixture.opened), len(fixture.closed))
	}
	for sessionID := range fixture.opened {
		if _, ok := fixture.closed[sessionID]; !ok {
			return fmt.Errorf("review-failure Factory Session %q was not closed", sessionID)
		}
	}
	return nil
}

func (fixture *reviewFailureProcessFixture) nextScenarioID() string {
	return fmt.Sprintf("rf-%d", fixture.nextScenario.Add(1))
}

func writeReviewFailureBootstrapFactory(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	const config = `{"name":"review-failure-bootstrap","workTypes":[{"name":"bootstrap","states":[{"name":"ready","type":"INITIAL"}]}],"workers":[],"workstations":[]}`
	return os.WriteFile(filepath.Join(dir, "factory.json"), []byte(config), 0o644)
}

type reviewFailureCommandResponder func(
	context.Context,
	platformprocess.CommandRequest,
	int,
) (platformprocess.CommandResult, error)

type reviewFailureRouteConfig struct {
	marker   string
	provider reviewFailureCommandResponder
	script   reviewFailureCommandResponder
}

type reviewFailureCommandRoute struct {
	factoryDir string
	marker     string
	provider   reviewFailureCommandResponder
	script     reviewFailureCommandResponder
	requests   []platformprocess.CommandRequest
	providerN  int
	scriptN    int
}

type reviewFailureCommandRouter struct {
	mu      sync.Mutex
	routes  map[string]*reviewFailureCommandRoute
	history map[string]reviewFailureCommandRoute
}

func newReviewFailureCommandRouter() *reviewFailureCommandRouter {
	return &reviewFailureCommandRouter{
		routes:  make(map[string]*reviewFailureCommandRoute),
		history: make(map[string]reviewFailureCommandRoute),
	}
}

func (router *reviewFailureCommandRouter) register(factoryDir string, config reviewFailureRouteConfig) error {
	factoryDir = filepath.Clean(factoryDir)
	if factoryDir == "." || strings.TrimSpace(config.marker) == "" {
		return errors.New("review-failure route Factory directory and marker are required")
	}
	router.mu.Lock()
	defer router.mu.Unlock()
	if _, exists := router.routes[factoryDir]; exists {
		return fmt.Errorf("review-failure route for %q is already registered", factoryDir)
	}
	router.routes[factoryDir] = &reviewFailureCommandRoute{
		factoryDir: factoryDir,
		marker:     config.marker,
		provider:   config.provider,
		script:     config.script,
	}
	return nil
}

func (router *reviewFailureCommandRouter) unregister(factoryDir string) {
	factoryDir = filepath.Clean(factoryDir)
	router.mu.Lock()
	if route := router.routes[factoryDir]; route != nil {
		router.history[factoryDir] = cloneReviewFailureRoute(*route)
	}
	delete(router.routes, factoryDir)
	router.mu.Unlock()
}

func (router *reviewFailureCommandRouter) routeCount() int {
	router.mu.Lock()
	defer router.mu.Unlock()
	return len(router.routes)
}

func (router *reviewFailureCommandRouter) routeForRequest(request platformprocess.CommandRequest) *reviewFailureCommandRoute {
	var best *reviewFailureCommandRoute
	for factoryDir, route := range router.routes {
		if !reviewFailurePathBelongsTo(factoryDir, request.WorkDir) &&
			!reviewFailurePathBelongsTo(factoryDir, request.Command) &&
			!reviewFailureRequestContainsMarker(request, route.marker) {
			continue
		}
		if best == nil || len(factoryDir) > len(best.factoryDir) {
			best = route
		}
	}
	return best
}

func (router *reviewFailureCommandRouter) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	if err := ctx.Err(); err != nil {
		return platformprocess.CommandResult{}, err
	}
	router.mu.Lock()
	route := router.routeForRequest(request)
	if route == nil {
		router.mu.Unlock()
		return platformprocess.CommandResult{}, fmt.Errorf("no review-failure command route matched command=%q workdir=%q args=%q", request.Command, request.WorkDir, request.Args)
	}
	request = cloneReviewFailureCommandRequest(request)
	route.requests = append(route.requests, request)
	provider := isReviewFailureProviderCommand(request.Command)
	var responder reviewFailureCommandResponder
	var call int
	if provider {
		route.providerN++
		call = route.providerN
		responder = route.provider
	} else {
		route.scriptN++
		call = route.scriptN
		responder = route.script
	}
	router.mu.Unlock()
	if responder != nil {
		return responder(ctx, request, call)
	}
	if provider {
		return reviewFailureAccepted("fixture accepted"), nil
	}
	return platformprocess.CommandResult{Stdout: []byte("review-failure-script-ok"), ExitCode: 0}, nil
}

func (router *reviewFailureCommandRouter) requestsFor(factoryDir string) []platformprocess.CommandRequest {
	router.mu.Lock()
	defer router.mu.Unlock()
	route := router.routes[filepath.Clean(factoryDir)]
	if route == nil {
		archived := router.history[filepath.Clean(factoryDir)]
		route = &archived
	}
	requests := make([]platformprocess.CommandRequest, len(route.requests))
	for index, request := range route.requests {
		requests[index] = cloneReviewFailureCommandRequest(request)
	}
	return requests
}

func reviewFailurePathBelongsTo(factoryDir, candidate string) bool {
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

func reviewFailureRequestContainsMarker(request platformprocess.CommandRequest, marker string) bool {
	values := append([]string{request.Command, request.WorkDir, string(request.Stdin)}, request.Args...)
	for _, value := range values {
		if strings.Contains(value, marker) {
			return true
		}
	}
	return false
}

func isReviewFailureProviderCommand(command string) bool {
	switch strings.ToLower(filepath.Base(strings.TrimSpace(command))) {
	case "codex", "claude", "agy":
		return true
	default:
		return false
	}
}

func cloneReviewFailureRoute(route reviewFailureCommandRoute) reviewFailureCommandRoute {
	route.requests = make([]platformprocess.CommandRequest, len(route.requests))
	for index, request := range route.requests {
		route.requests[index] = cloneReviewFailureCommandRequest(request)
	}
	return route
}

func cloneReviewFailureCommandRequest(request platformprocess.CommandRequest) platformprocess.CommandRequest {
	request.Args = append([]string(nil), request.Args...)
	request.Stdin = append([]byte(nil), request.Stdin...)
	request.Env = append([]string(nil), request.Env...)
	return request
}
