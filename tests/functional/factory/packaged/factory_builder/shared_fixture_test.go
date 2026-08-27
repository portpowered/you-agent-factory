package factory_builder

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

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const factoryBuilderSharedFixtureTimeout = 15 * time.Second

// factoryBuilderSharedFixture owns one root-built process and one continuous
// API host for the two session-safe greeting scenarios. Each child copies the
// packaged Factory into a private home and opens a non-default Factory Session.
type factoryBuilderSharedFixture struct {
	process    support.ApplicationProcess
	provider   *factoryBuilderProviderCommandRouter
	baseURL    string
	factoryDir string
	requestID  atomic.Uint64

	mu     sync.Mutex
	opened int
	closed int
}

// factoryBuilderProviderCommandRouter keeps process wiring immutable while
// selecting each scenario's synchronized command runner by its owned path.
type factoryBuilderProviderCommandRouter struct {
	mu      sync.RWMutex
	runners map[string]platformprocess.CommandRunner
}

func newFactoryBuilderProviderCommandRouter() *factoryBuilderProviderCommandRouter {
	return &factoryBuilderProviderCommandRouter{runners: make(map[string]platformprocess.CommandRunner)}
}

func (router *factoryBuilderProviderCommandRouter) register(
	paths []string,
	runner platformprocess.CommandRunner,
) {
	router.mu.Lock()
	defer router.mu.Unlock()
	for _, path := range paths {
		router.runners[factoryBuilderPathKey(path)] = runner
	}
}

func (router *factoryBuilderProviderCommandRouter) unregister(paths []string) {
	router.mu.Lock()
	defer router.mu.Unlock()
	for _, path := range paths {
		delete(router.runners, factoryBuilderPathKey(path))
	}
}

func (router *factoryBuilderProviderCommandRouter) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	workDir := factoryBuilderPathKey(request.WorkDir)
	router.mu.RLock()
	runner := router.runners[workDir]
	if runner == nil {
		for factoryDir, candidate := range router.runners {
			if factoryBuilderPathContains(factoryDir, workDir) {
				runner = candidate
				break
			}
		}
	}
	router.mu.RUnlock()
	if runner == nil {
		return platformprocess.CommandResult{}, fmt.Errorf(
			"Factory Builder provider runner is not registered for work directory %q",
			request.WorkDir,
		)
	}
	return runner.Run(ctx, request)
}

func factoryBuilderPathKey(path string) string {
	if absolute, err := filepath.Abs(path); err == nil {
		path = absolute
	}
	return filepath.Clean(path)
}

func factoryBuilderPathContains(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))
}

type factoryBuilderScenario struct {
	fixture          *factoryBuilderSharedFixture
	factoryDir       string
	environment      []string
	workingDirectory string
	selectorPaths    []string
	sessionID        string
}

func newFactoryBuilderSharedFixture(t *testing.T) *factoryBuilderSharedFixture {
	t.Helper()
	rootDir := t.TempDir()
	homeDir := filepath.Join(rootDir, "home")
	workingDirectory := filepath.Join(rootDir, "work")
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		t.Fatalf("create shared Factory Builder home: %v", err)
	}
	if err := os.MkdirAll(workingDirectory, 0o755); err != nil {
		t.Fatalf("create shared Factory Builder working directory: %v", err)
	}

	api := support.NewProcessAPIServer()
	provider := newFactoryBuilderProviderCommandRouter()
	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		APIServerStarter:      api.Start,
		ProviderCommandRunner: provider,
	})
	if err != nil {
		t.Fatalf("BuildProcess(Factory Builder): %v", err)
	}
	support.CleanupProcess(t, process)

	environment := factoryBuilderCustomerEnvironment(homeDir)
	factoryDir := support.InstallPackagedFactoryWithProcess(
		t,
		process,
		environment,
		workingDirectory,
		factoryBuilderName,
	)
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "run", "--dir", factoryDir,
		"--continuously", "--with-server", "--quiet", "--no-record",
		"--provider", "CODEX", "--model", "gpt-5",
	})
	inputs.Input.Env = environment
	inputs.Input.WorkingDirectory = factoryDir
	support.StartProcessCommand(t, process, inputs.Input)
	baseURL := api.WaitForURL(t)
	support.WaitForStatus(t, baseURL, factoryBuilderSharedFixtureTimeout, func(status factoryapi.StatusResponse) bool {
		return strings.TrimSpace(status.RuntimeStatus) != ""
	})

	fixture := &factoryBuilderSharedFixture{
		process:    process,
		provider:   provider,
		baseURL:    baseURL,
		factoryDir: factoryDir,
	}
	t.Cleanup(func() {
		fixture.mu.Lock()
		opened, closed := fixture.opened, fixture.closed
		fixture.mu.Unlock()
		t.Logf("Factory Builder shared fixture: process_starts=1 explicit_sessions_opened=%d explicit_sessions_closed=%d behavior_rows=2 isolated_rows=3", opened, closed)
	})
	return fixture
}

func factoryBuilderCustomerEnvironment(homeDir string) []string {
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
		"YOU_DEFAULT_WORKER_MODEL=gpt-5",
	)
}

func (fixture *factoryBuilderSharedFixture) nextRequestID() uint64 {
	return fixture.requestID.Add(1)
}

func (fixture *factoryBuilderSharedFixture) newScenario(
	t *testing.T,
	runner platformprocess.CommandRunner,
) *factoryBuilderScenario {
	t.Helper()
	homeDir := t.TempDir()
	workingDirectory := t.TempDir()
	factoryDir := support.CopyFactoryAsNamed(t, fixture.factoryDir, homeDir, factoryBuilderName)
	environment := factoryBuilderCustomerEnvironment(homeDir)
	selectorPaths := []string{factoryDir, workingDirectory, fixture.factoryDir}
	fixture.provider.register(selectorPaths, runner)
	t.Cleanup(func() {
		fixture.provider.unregister(selectorPaths)
	})
	return &factoryBuilderScenario{
		fixture:          fixture,
		factoryDir:       factoryDir,
		environment:      environment,
		workingDirectory: workingDirectory,
		selectorPaths:    selectorPaths,
	}
}

func (scenario *factoryBuilderScenario) open(t *testing.T) {
	t.Helper()
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

var _ platformprocess.CommandRunner = (*factoryBuilderProviderCommandRouter)(nil)
