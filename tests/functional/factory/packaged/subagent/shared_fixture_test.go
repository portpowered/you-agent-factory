package subagent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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

const subagentSharedFixtureTimeout = 15 * time.Second

// subagentSharedFixture owns one root-built process and one continuous API
// host for the package's compatible child-result scenarios. Each child copies
// the packaged Factory into a private home and opens a non-default session.
type subagentSharedFixture struct {
	owner       testing.TB
	process     support.ApplicationProcess
	provider    *subagentProviderCommandRouter
	apiStarter  *subagentAPIServerStarter
	environment []string
	host        *support.ProcessCommand
	baseURL     string
	factoryDir  string

	mu     sync.Mutex
	opened int
	closed int
}

// subagentAPIServerStarter records listener requests so the no-server CLI
// witness can prove that its invocation did not request a second listener.
type subagentAPIServerStarter struct {
	api   *support.ProcessAPIServer
	calls atomic.Int32
}

func (starter *subagentAPIServerStarter) Start(
	ctx context.Context,
	request platformhttpserver.StartRequest,
) error {
	starter.calls.Add(1)
	return starter.api.Start(ctx, request)
}

// subagentProviderCommandRouter keeps the process edge immutable while
// selecting each scenario's synchronized runner by Factory or work path.
type subagentProviderCommandRouter struct {
	mu      sync.RWMutex
	runners map[string]platformprocess.CommandRunner
}

func newSubagentProviderCommandRouter() *subagentProviderCommandRouter {
	return &subagentProviderCommandRouter{runners: make(map[string]platformprocess.CommandRunner)}
}

func (router *subagentProviderCommandRouter) register(
	paths []string,
	runner platformprocess.CommandRunner,
) {
	router.mu.Lock()
	defer router.mu.Unlock()
	for _, path := range paths {
		router.runners[subagentPathKey(path)] = runner
	}
}

func (router *subagentProviderCommandRouter) unregister(paths []string) {
	router.mu.Lock()
	defer router.mu.Unlock()
	for _, path := range paths {
		delete(router.runners, subagentPathKey(path))
	}
}

func (router *subagentProviderCommandRouter) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	workDir := subagentPathKey(request.WorkDir)
	router.mu.RLock()
	runner := router.runners[workDir]
	if runner == nil {
		for factoryDir, candidate := range router.runners {
			if subagentPathContains(factoryDir, workDir) {
				runner = candidate
				break
			}
		}
	}
	router.mu.RUnlock()
	if runner == nil {
		return platformprocess.CommandResult{}, fmt.Errorf(
			"subagent provider runner is not registered for work directory %q",
			request.WorkDir,
		)
	}
	return runner.Run(ctx, request)
}

func subagentPathKey(path string) string {
	if absolute, err := filepath.Abs(path); err == nil {
		path = absolute
	}
	return filepath.Clean(path)
}

func subagentPathContains(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))
}

type subagentScenario struct {
	fixture          *subagentSharedFixture
	factoryDir       string
	environment      []string
	workingDirectory string
	selectorPaths    []string
	sessionID        string
}

func newSubagentSharedFixture(t *testing.T) *subagentSharedFixture {
	t.Helper()
	rootDir := t.TempDir()
	homeDir := filepath.Join(rootDir, "home")
	workingDirectory := filepath.Join(rootDir, "work")
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		t.Fatalf("create shared subagent home: %v", err)
	}
	if err := os.MkdirAll(workingDirectory, 0o755); err != nil {
		t.Fatalf("create shared subagent working directory: %v", err)
	}

	api := support.NewProcessAPIServer()
	apiStarter := &subagentAPIServerStarter{api: api}
	provider := newSubagentProviderCommandRouter()
	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		APIServerStarter:      apiStarter.Start,
		ProviderCommandRunner: provider,
	})
	if err != nil {
		t.Fatalf("BuildProcess(subagent): %v", err)
	}
	support.CleanupProcess(t, process)

	environment := subagentCustomerEnvironment(homeDir)
	factoryDir := support.InstallPackagedFactoryWithProcess(
		t,
		process,
		environment,
		workingDirectory,
		"@you/subagent",
	)

	fixture := &subagentSharedFixture{
		owner:       t,
		process:     process,
		provider:    provider,
		apiStarter:  apiStarter,
		environment: environment,
		factoryDir:  factoryDir,
	}
	t.Cleanup(func() {
		fixture.mu.Lock()
		opened, closed := fixture.opened, fixture.closed
		fixture.mu.Unlock()
		t.Logf("subagent shared fixture: process_starts=1 explicit_sessions_opened=%d explicit_sessions_closed=%d behavior_rows=5 isolated_rows=0", opened, closed)
	})
	return fixture
}

func (fixture *subagentSharedFixture) startHost(t testing.TB) {
	t.Helper()
	if fixture.host != nil {
		return
	}
	inputs := support.FakeInputs(context.Background(), []string{
		"you", "run", "--dir", fixture.factoryDir,
		"--continuously", "--with-server", "--quiet", "--no-record",
	})
	inputs.Input.Env = fixture.environment
	inputs.Input.WorkingDirectory = fixture.factoryDir
	fixture.host = support.StartProcessCommand(fixture.owner, fixture.process, inputs.Input)
	fixture.baseURL = fixture.apiStarter.api.WaitForURL(t)
	support.WaitForStatus(t, fixture.baseURL, subagentSharedFixtureTimeout, func(status factoryapi.StatusResponse) bool {
		return strings.TrimSpace(status.RuntimeStatus) != ""
	})
	// The continuous host is the one expected listener. Reset the observation
	// baseline so a no-server invocation can assert that it requested none.
	fixture.apiStarter.calls.Store(0)
}

func subagentCustomerEnvironment(homeDir string) []string {
	environment := make([]string, 0, len(os.Environ())+4)
	for _, entry := range os.Environ() {
		name := strings.SplitN(entry, "=", 2)[0]
		switch {
		case strings.EqualFold(name, "HOME"), strings.EqualFold(name, "USERPROFILE"),
			strings.EqualFold(name, "YOU_DEFAULT_WORKER_MODEL_PROVIDER"), strings.EqualFold(name, "YOU_DEFAULT_WORKER_MODEL"):
			continue
		default:
			environment = append(environment, entry)
		}
	}
	return append(
		environment,
		"HOME="+homeDir,
		"USERPROFILE="+homeDir,
		"YOU_DEFAULT_WORKER_MODEL_PROVIDER=CODEX",
		"YOU_DEFAULT_WORKER_MODEL=operator-model",
	)
}

func (fixture *subagentSharedFixture) newScenario(
	t *testing.T,
	runner platformprocess.CommandRunner,
) *subagentScenario {
	t.Helper()
	homeDir := t.TempDir()
	workingDirectory := t.TempDir()
	factoryDir := support.CopyFactoryAsNamed(t, fixture.factoryDir, homeDir, "@you/subagent")
	environment := subagentCustomerEnvironment(homeDir)
	selectorPaths := []string{factoryDir, workingDirectory, fixture.factoryDir}
	fixture.provider.register(selectorPaths, runner)
	t.Cleanup(func() {
		fixture.provider.unregister(selectorPaths)
	})
	return &subagentScenario{
		fixture:          fixture,
		factoryDir:       factoryDir,
		environment:      environment,
		workingDirectory: workingDirectory,
		selectorPaths:    selectorPaths,
	}
}

func (scenario *subagentScenario) open(t *testing.T) {
	t.Helper()
	scenario.fixture.startHost(t)
	opened := support.OpenFactorySessionAt(t, scenario.fixture.baseURL, scenario.factoryDir)
	scenario.sessionID = opened.Session.Id
	scenario.fixture.mu.Lock()
	scenario.fixture.opened++
	scenario.fixture.mu.Unlock()
	t.Cleanup(func() {
		support.CloseFactorySessionAt(t, scenario.fixture.baseURL, scenario.sessionID)
		scenario.fixture.mu.Lock()
		scenario.fixture.closed++
		scenario.fixture.mu.Unlock()
	})
}

var _ platformprocess.CommandRunner = (*subagentProviderCommandRouter)(nil)
