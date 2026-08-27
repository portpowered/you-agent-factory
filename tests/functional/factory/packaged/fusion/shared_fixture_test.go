package fusion

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

const fusionSharedFixtureTimeout = 15 * time.Second

// fusionSharedFixture owns one root-built process and one continuous API host
// for the package's compatible scenarios. Each child copies the packaged
// Factory into a private home and opens a non-default Factory Session.
type fusionSharedFixture struct {
	process    support.ApplicationProcess
	provider   *fusionProviderCommandRouter
	baseURL    string
	factoryDir string

	mu     sync.Mutex
	opened int
	closed int
}

// fusionProviderCommandRouter keeps the process edge immutable while
// selecting the scenario-owned command runner by the request's Factory path.
// The path selector also covers child work directories below that Factory.
type fusionProviderCommandRouter struct {
	mu      sync.RWMutex
	runners map[string]platformprocess.CommandRunner
}

func newFusionProviderCommandRouter() *fusionProviderCommandRouter {
	return &fusionProviderCommandRouter{runners: make(map[string]platformprocess.CommandRunner)}
}

func (router *fusionProviderCommandRouter) register(
	paths []string,
	runner platformprocess.CommandRunner,
) {
	router.mu.Lock()
	defer router.mu.Unlock()
	for _, path := range paths {
		router.runners[fusionPathKey(path)] = runner
	}
}

func (router *fusionProviderCommandRouter) unregister(paths []string) {
	router.mu.Lock()
	defer router.mu.Unlock()
	for _, path := range paths {
		delete(router.runners, fusionPathKey(path))
	}
}

func (router *fusionProviderCommandRouter) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	workDir := fusionPathKey(request.WorkDir)
	router.mu.RLock()
	runner := router.runners[workDir]
	if runner == nil {
		for factoryDir, candidate := range router.runners {
			if fusionPathContains(factoryDir, workDir) {
				runner = candidate
				break
			}
		}
	}
	router.mu.RUnlock()
	if runner == nil {
		return platformprocess.CommandResult{}, fmt.Errorf(
			"fusion provider runner is not registered for work directory %q",
			request.WorkDir,
		)
	}
	return runner.Run(ctx, request)
}

func fusionPathKey(path string) string {
	if absolute, err := filepath.Abs(path); err == nil {
		path = absolute
	}
	return filepath.Clean(path)
}

func fusionPathContains(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))
}

type fusionScenario struct {
	fixture          *fusionSharedFixture
	factoryDir       string
	environment      []string
	workingDirectory string
	selectorPaths    []string
	sessionID        string
}

func newFusionSharedFixture(t *testing.T) *fusionSharedFixture {
	t.Helper()
	rootDir := t.TempDir()
	homeDir := filepath.Join(rootDir, "home")
	workingDirectory := filepath.Join(rootDir, "work")
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		t.Fatalf("create shared fusion home: %v", err)
	}
	if err := os.MkdirAll(workingDirectory, 0o755); err != nil {
		t.Fatalf("create shared fusion working directory: %v", err)
	}

	api := support.NewProcessAPIServer()
	provider := newFusionProviderCommandRouter()
	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		APIServerStarter:      api.Start,
		ProviderCommandRunner: provider,
	})
	if err != nil {
		t.Fatalf("BuildProcess(fusion): %v", err)
	}
	support.CleanupProcess(t, process)

	environment := fusionCustomerEnvironment(homeDir)
	factoryDir := support.InstallPackagedFactoryWithProcess(
		t,
		process,
		environment,
		workingDirectory,
		factorydefinitions.PackagedFusionFactoryName,
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
	support.WaitForStatus(t, baseURL, fusionSharedFixtureTimeout, func(status factoryapi.StatusResponse) bool {
		return strings.TrimSpace(status.RuntimeStatus) != ""
	})

	fixture := &fusionSharedFixture{
		process:    process,
		provider:   provider,
		baseURL:    baseURL,
		factoryDir: factoryDir,
	}
	t.Cleanup(func() {
		fixture.mu.Lock()
		opened, closed := fixture.opened, fixture.closed
		fixture.mu.Unlock()
		t.Logf("fusion shared fixture: process_starts=1 explicit_sessions_opened=%d explicit_sessions_closed=%d isolated_rows=0", opened, closed)
	})
	return fixture
}

func fusionCustomerEnvironment(homeDir string) []string {
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

func (fixture *fusionSharedFixture) newScenario(
	t *testing.T,
	runner platformprocess.CommandRunner,
) *fusionScenario {
	t.Helper()
	homeDir := t.TempDir()
	workingDirectory := t.TempDir()
	factoryDir := support.CopyFactoryAsNamed(
		t,
		fixture.factoryDir,
		homeDir,
		factorydefinitions.PackagedFusionFactoryName,
	)
	environment := fusionCustomerEnvironment(homeDir)
	selectorPaths := []string{factoryDir, workingDirectory}
	fixture.provider.register(selectorPaths, runner)
	t.Cleanup(func() {
		fixture.provider.unregister(selectorPaths)
	})
	return &fusionScenario{
		fixture:          fixture,
		factoryDir:       factoryDir,
		environment:      environment,
		workingDirectory: workingDirectory,
		selectorPaths:    selectorPaths,
	}
}

func (scenario *fusionScenario) open(t *testing.T) {
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

var _ platformprocess.CommandRunner = (*fusionProviderCommandRouter)(nil)
