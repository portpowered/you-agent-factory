package root_composition_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	internaltestutil "github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const sharedBTRCBatchFixtureShutdownTimeout = 15 * time.Second

var (
	sharedBTRCBatchFixtureOnce  sync.Once
	sharedBTRCBatchFixtureValue *sharedBTRCBatchProcessFixture
	sharedBTRCBatchFixtureErr   error
)

func TestMain(m *testing.M) {
	code := m.Run()
	if sharedBTRCBatchFixtureValue != nil {
		if err := sharedBTRCBatchFixtureValue.close(); err != nil {
			fmt.Fprintf(os.Stderr, "close shared batch fixture: %v\n", err)
			code = 1
		}
	}
	if sharedBTRCBatchFixtureErr != nil {
		fmt.Fprintf(os.Stderr, "build shared batch fixture: %v\n", sharedBTRCBatchFixtureErr)
		code = 1
	}
	os.Exit(code)
}

type sharedBTRCBatchProcessFixture struct {
	rootDir    string
	homeDir    string
	recordings string
	process    support.ApplicationProcess
	router     *sharedBTRCBatchCommandRouter
}

func sharedBTRCBatchProcess(t *testing.T) *sharedBTRCBatchProcessFixture {
	t.Helper()
	sharedBTRCBatchFixtureOnce.Do(func() {
		sharedBTRCBatchFixtureValue, sharedBTRCBatchFixtureErr = newSharedBTRCBatchProcessFixture(t)
	})
	if sharedBTRCBatchFixtureErr != nil {
		t.Fatalf("build shared batch process: %v", sharedBTRCBatchFixtureErr)
	}
	if sharedBTRCBatchFixtureValue == nil {
		t.Fatal("build shared batch process returned nil fixture")
	}
	return sharedBTRCBatchFixtureValue
}

func newSharedBTRCBatchProcessFixture(t *testing.T) (*sharedBTRCBatchProcessFixture, error) {
	t.Helper()
	rootDir, err := os.MkdirTemp("", "you-functional-factory-batch-")
	if err != nil {
		return nil, fmt.Errorf("create fixture root: %w", err)
	}
	removeRoot := func() {
		_ = os.RemoveAll(rootDir)
	}
	homeDir := filepath.Join(rootDir, "home")
	recordings := filepath.Join(rootDir, "recordings")
	for _, dir := range []string{homeDir, recordings} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			removeRoot()
			return nil, fmt.Errorf("create fixture directory %q: %w", dir, err)
		}
	}

	router := newSharedBTRCBatchCommandRouter()
	process, err := support.BuildProcessWithContext(
		context.Background(),
		serviceedges.Edges{ProviderCommandRunner: router},
	)
	if err != nil {
		removeRoot()
		return nil, fmt.Errorf("build root process: %w", err)
	}
	return &sharedBTRCBatchProcessFixture{
		rootDir:    rootDir,
		homeDir:    homeDir,
		recordings: recordings,
		process:    process,
		router:     router,
	}, nil
}

func (fixture *sharedBTRCBatchProcessFixture) close() error {
	if fixture == nil {
		return nil
	}
	var closeErr error
	if fixture.process != nil {
		ctx, cancel := context.WithTimeout(context.Background(), sharedBTRCBatchFixtureShutdownTimeout)
		closeErr = fixture.process.Close(ctx)
		cancel()
	}
	if got := fixture.router.routeCount(); got != 0 && closeErr == nil {
		closeErr = fmt.Errorf("shared batch command routes remaining after close: %d", got)
	}
	if err := os.RemoveAll(fixture.rootDir); err != nil && closeErr == nil {
		closeErr = fmt.Errorf("remove shared batch fixture root: %w", err)
	}
	if _, err := os.Stat(fixture.rootDir); !errors.Is(err, os.ErrNotExist) && closeErr == nil {
		closeErr = fmt.Errorf("shared batch fixture root still exists: %w", err)
	}
	return closeErr
}

type sharedBTRCBatchSession struct {
	fixture       *sharedBTRCBatchProcessFixture
	factoryDir    string
	batchPath     string
	recordingPath string
	closeOnce     sync.Once
}

// The finite `you run --work` command owns the terminal run/lifecycle events
// asserted by the batch characterization. A live explicit-session Work
// Request remains hosted after terminal Work and does not publish that same
// finite-run suffix, so each case keeps this process-scoped CLI boundary while
// receiving an independent runtime, fixture root, route, and recording.
func openBTRCBatchSession(
	t *testing.T,
	provider platformprocess.CommandRunner,
) *sharedBTRCBatchSession {
	t.Helper()
	fixture := sharedBTRCBatchProcess(t)
	factoryDir := newBTRCBatchScenario(t, fixture.rootDir)
	fixture.router.register(factoryDir, provider)
	session := &sharedBTRCBatchSession{
		fixture:    fixture,
		factoryDir: factoryDir,
	}
	t.Cleanup(func() { session.close(t) })
	return session
}

func (session *sharedBTRCBatchSession) close(t testing.TB) {
	t.Helper()
	session.closeOnce.Do(func() {
		session.fixture.router.unregister(session.factoryDir)
		for _, path := range []string{session.factoryDir, session.recordingPath} {
			if path == "" {
				continue
			}
			if err := os.RemoveAll(path); err != nil {
				t.Errorf("remove batch session path %q: %v", path, err)
			}
			if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("batch session path %q still exists: %v", path, err)
			}
		}
	})
}

func executeBTRCBatch(
	t *testing.T,
	session *sharedBTRCBatchSession,
	request work.WorkRequest,
) (*interfaces.ReplayArtifact, error) {
	t.Helper()
	payload, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal batch request: %v", err)
	}
	session.batchPath = filepath.Join(session.factoryDir, "batch.json")
	session.recordingPath = filepath.Join(
		session.fixture.recordings,
		filepath.Base(session.factoryDir)+".replay.json",
	)
	if err := os.WriteFile(session.batchPath, payload, 0o600); err != nil {
		t.Fatalf("write batch request: %v", err)
	}
	inputs := support.FakeInputs(t.Context(), []string{
		"you",
		"run",
		"--dir",
		session.factoryDir,
		"--work",
		session.batchPath,
		"--record",
		session.recordingPath,
	})
	inputs.Input.Env = append(os.Environ(),
		"HOME="+session.fixture.homeDir,
		"USERPROFILE="+session.fixture.homeDir,
	)
	inputs.Input.WorkingDirectory = session.factoryDir
	executeErr := session.fixture.process.Execute(inputs.Input)
	artifact := internaltestutil.LoadReplayArtifact(t, session.recordingPath)
	reloaded := internaltestutil.LoadReplayArtifact(t, session.recordingPath)
	if !reflect.DeepEqual(artifact, reloaded) {
		t.Fatal("repeated batch recording observation changed the retained artifact")
	}
	return artifact, executeErr
}

func newBTRCBatchScenario(t *testing.T, rootDir string) string {
	t.Helper()
	scenarioDir := filepath.Join(rootDir, "scenario-"+strings.ReplaceAll(t.Name(), "/", "-"))
	if err := os.MkdirAll(scenarioDir, 0o755); err != nil {
		t.Fatalf("create batch scenario: %v", err)
	}
	copyBTRCBatchDirectory(t, support.LegacyFixtureDir(t, "resource_contention"), scenarioDir)
	return scenarioDir
}

func copyBTRCBatchDirectory(t *testing.T, sourceDir, destinationDir string) {
	t.Helper()
	if err := filepath.WalkDir(sourceDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		destination := destinationDir
		if relative != "." {
			destination = filepath.Join(destinationDir, relative)
		}
		if entry.IsDir() {
			return os.MkdirAll(destination, 0o755)
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(destination, contents, entry.Type().Perm())
	}); err != nil {
		t.Fatalf("copy batch fixture: %v", err)
	}
}

func assertBTRCBatchRouteRequests(t *testing.T, session *sharedBTRCBatchSession) {
	t.Helper()
	requests := session.fixture.router.requestsFor(session.factoryDir)
	if len(requests) != 2 {
		t.Fatalf("provider command requests = %d, want 2", len(requests))
	}
	for index, request := range requests {
		if !pathWithinBTRCBatchDir(request.WorkDir, session.factoryDir) {
			t.Fatalf("provider request[%d] work dir = %q, want inside %q", index, request.WorkDir, session.factoryDir)
		}
	}
}

func pathWithinBTRCBatchDir(path, root string) bool {
	path = filepath.Clean(path)
	root = filepath.Clean(root)
	if path == root {
		return true
	}
	separator := string(filepath.Separator)
	return strings.HasPrefix(path, root+separator)
}

type sharedBTRCBatchCommandRouter struct {
	mu      sync.Mutex
	routes  map[string]*sharedBTRCBatchRoute
	history map[string]*sharedBTRCBatchRoute
}

type sharedBTRCBatchRoute struct {
	factoryDir    string
	provider      platformprocess.CommandRunner
	providerCalls int
	requests      []platformprocess.CommandRequest
}

func newSharedBTRCBatchCommandRouter() *sharedBTRCBatchCommandRouter {
	return &sharedBTRCBatchCommandRouter{
		routes:  make(map[string]*sharedBTRCBatchRoute),
		history: make(map[string]*sharedBTRCBatchRoute),
	}
}

func (router *sharedBTRCBatchCommandRouter) register(
	factoryDir string,
	provider platformprocess.CommandRunner,
) {
	router.mu.Lock()
	defer router.mu.Unlock()
	if _, exists := router.routes[factoryDir]; exists {
		panic("batch command route already registered")
	}
	router.routes[factoryDir] = &sharedBTRCBatchRoute{
		factoryDir: factoryDir,
		provider:   provider,
	}
}

func (router *sharedBTRCBatchCommandRouter) unregister(factoryDir string) {
	router.mu.Lock()
	defer router.mu.Unlock()
	route, exists := router.routes[factoryDir]
	if !exists {
		return
	}
	router.history[factoryDir] = route
	delete(router.routes, factoryDir)
}

func (router *sharedBTRCBatchCommandRouter) routeCount() int {
	router.mu.Lock()
	defer router.mu.Unlock()
	return len(router.routes)
}

func (router *sharedBTRCBatchCommandRouter) requestsFor(factoryDir string) []platformprocess.CommandRequest {
	router.mu.Lock()
	defer router.mu.Unlock()
	route := router.history[factoryDir]
	if route == nil {
		route = router.routes[factoryDir]
	}
	if route == nil {
		return nil
	}
	return append([]platformprocess.CommandRequest(nil), route.requests...)
}

func (router *sharedBTRCBatchCommandRouter) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	router.mu.Lock()
	route := router.routeForRequest(request)
	if route == nil {
		router.mu.Unlock()
		return platformprocess.CommandResult{}, fmt.Errorf("no batch command route for %q in %q", request.Command, request.WorkDir)
	}
	route.providerCalls++
	route.requests = append(route.requests, request)
	provider := route.provider
	router.mu.Unlock()
	return provider.Run(ctx, request)
}

func (router *sharedBTRCBatchCommandRouter) routeForRequest(
	request platformprocess.CommandRequest,
) *sharedBTRCBatchRoute {
	var selected *sharedBTRCBatchRoute
	for _, route := range router.routes {
		if !pathWithinBTRCBatchDir(request.WorkDir, route.factoryDir) &&
			!pathWithinBTRCBatchDir(request.Command, route.factoryDir) {
			continue
		}
		if selected == nil || len(route.factoryDir) > len(selected.factoryDir) {
			selected = route
		}
	}
	return selected
}

var _ platformprocess.CommandRunner = (*sharedBTRCBatchCommandRouter)(nil)
