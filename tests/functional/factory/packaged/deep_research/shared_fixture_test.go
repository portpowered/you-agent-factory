package deep_research

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const deepResearchSharedFixtureTimeout = 15 * time.Second

// deepResearchSharedFixture owns one root-built process and one continuous
// API host for the package's compatible scenarios. Each child copies the
// packaged Factory into a private home and opens a non-default Factory Session.
type deepResearchSharedFixture struct {
	process    support.ApplicationProcess
	provider   *deepResearchProviderCommandRouter
	baseURL    string
	factoryDir string

	mu     sync.Mutex
	opened int
	closed int
}

// deepResearchProviderCommandRouter keeps the process edge immutable while
// selecting the scenario-owned command runner by the request's Factory path.
// The path selector also covers child work directories below that Factory.
type deepResearchProviderCommandRouter struct {
	mu      sync.RWMutex
	runners map[string]platformprocess.CommandRunner
}

func newDeepResearchProviderCommandRouter() *deepResearchProviderCommandRouter {
	return &deepResearchProviderCommandRouter{runners: make(map[string]platformprocess.CommandRunner)}
}

func (router *deepResearchProviderCommandRouter) register(
	paths []string,
	runner platformprocess.CommandRunner,
) {
	router.mu.Lock()
	defer router.mu.Unlock()
	for _, path := range paths {
		router.runners[deepResearchPathKey(path)] = runner
	}
}

func (router *deepResearchProviderCommandRouter) unregister(paths []string) {
	router.mu.Lock()
	defer router.mu.Unlock()
	for _, path := range paths {
		delete(router.runners, deepResearchPathKey(path))
	}
}

func (router *deepResearchProviderCommandRouter) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	workDir := deepResearchPathKey(request.WorkDir)
	router.mu.RLock()
	runner := router.runners[workDir]
	if runner == nil {
		for factoryDir, candidate := range router.runners {
			if deepResearchPathContains(factoryDir, workDir) {
				runner = candidate
				break
			}
		}
	}
	router.mu.RUnlock()
	if runner == nil {
		return platformprocess.CommandResult{}, fmt.Errorf(
			"deep research provider runner is not registered for work directory %q",
			request.WorkDir,
		)
	}
	return runner.Run(ctx, request)
}

func deepResearchPathKey(path string) string {
	if absolute, err := filepath.Abs(path); err == nil {
		path = absolute
	}
	return filepath.Clean(path)
}

func deepResearchPathContains(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))
}

type deepResearchScenario struct {
	fixture          *deepResearchSharedFixture
	factoryDir       string
	environment      []string
	workingDirectory string
	selectorPaths    []string
	sessionID        string
}

func newDeepResearchSharedFixture(t *testing.T) *deepResearchSharedFixture {
	t.Helper()
	rootDir := t.TempDir()
	homeDir := filepath.Join(rootDir, "home")
	workingDirectory := filepath.Join(rootDir, "work")
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		t.Fatalf("create shared deep-research home: %v", err)
	}
	if err := os.MkdirAll(workingDirectory, 0o755); err != nil {
		t.Fatalf("create shared deep-research working directory: %v", err)
	}

	api := support.NewProcessAPIServer()
	provider := newDeepResearchProviderCommandRouter()
	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		APIServerStarter:      api.Start,
		ProviderCommandRunner: provider,
	})
	if err != nil {
		t.Fatalf("BuildProcess(deep research): %v", err)
	}
	support.CleanupProcess(t, process)

	environment := deepResearchCustomerEnvironment(homeDir)
	factoryDir := support.InstallPackagedFactoryWithProcess(
		t,
		process,
		environment,
		workingDirectory,
		factorydefinitions.PackagedDeepResearchFactoryName,
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
	support.WaitForStatus(t, baseURL, deepResearchSharedFixtureTimeout, func(status factoryapi.StatusResponse) bool {
		return strings.TrimSpace(status.RuntimeStatus) != ""
	})

	fixture := &deepResearchSharedFixture{
		process:    process,
		provider:   provider,
		baseURL:    baseURL,
		factoryDir: factoryDir,
	}
	t.Cleanup(func() {
		fixture.mu.Lock()
		opened, closed := fixture.opened, fixture.closed
		fixture.mu.Unlock()
		t.Logf("deep-research shared fixture: process_starts=1 explicit_sessions_opened=%d explicit_sessions_closed=%d isolated_rows=0", opened, closed)
	})
	return fixture
}

func (fixture *deepResearchSharedFixture) newScenario(
	t *testing.T,
	runner platformprocess.CommandRunner,
) *deepResearchScenario {
	t.Helper()
	homeDir := t.TempDir()
	workingDirectory := t.TempDir()
	factoryDir := support.CopyFactoryAsNamed(
		t,
		fixture.factoryDir,
		homeDir,
		factorydefinitions.PackagedDeepResearchFactoryName,
	)
	environment := deepResearchCustomerEnvironment(homeDir)
	selectorPaths := []string{factoryDir, workingDirectory, fixture.factoryDir}
	fixture.provider.register(selectorPaths, runner)
	t.Cleanup(func() {
		fixture.provider.unregister(selectorPaths)
	})
	scenario := &deepResearchScenario{
		fixture:          fixture,
		factoryDir:       factoryDir,
		environment:      environment,
		workingDirectory: workingDirectory,
		selectorPaths:    selectorPaths,
	}
	return scenario
}

func (scenario *deepResearchScenario) open(t *testing.T) {
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

var _ platformprocess.CommandRunner = (*deepResearchProviderCommandRouter)(nil)
