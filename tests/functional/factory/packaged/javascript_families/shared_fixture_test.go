package javascript_families

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

const javascriptFamiliesSharedFixtureTimeout = 15 * time.Second

// javascriptFamiliesSharedFixture owns one root-built process and one
// continuous API host for the tournament and spawn scenarios. Each child
// copies its packaged Factory into a private home and opens a non-default
// Factory Session.
type javascriptFamiliesSharedFixture struct {
	process     support.ApplicationProcess
	provider    *javascriptFamiliesProviderCommandRouter
	baseURL     string
	factoryDirs map[string]string

	mu     sync.Mutex
	opened int
	closed int
}

// javascriptFamiliesProviderCommandRouter keeps the process edge immutable
// while selecting the scenario-owned command runner by Factory path. The path
// selector also covers child work directories below that Factory.
type javascriptFamiliesProviderCommandRouter struct {
	mu      sync.RWMutex
	runners map[string]platformprocess.CommandRunner
}

func newJavascriptFamiliesProviderCommandRouter() *javascriptFamiliesProviderCommandRouter {
	return &javascriptFamiliesProviderCommandRouter{runners: make(map[string]platformprocess.CommandRunner)}
}

func (router *javascriptFamiliesProviderCommandRouter) register(
	paths []string,
	runner platformprocess.CommandRunner,
) {
	router.mu.Lock()
	defer router.mu.Unlock()
	for _, path := range paths {
		router.runners[javascriptFamiliesPathKey(path)] = runner
	}
}

func (router *javascriptFamiliesProviderCommandRouter) unregister(paths []string) {
	router.mu.Lock()
	defer router.mu.Unlock()
	for _, path := range paths {
		delete(router.runners, javascriptFamiliesPathKey(path))
	}
}

func (router *javascriptFamiliesProviderCommandRouter) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	workDir := javascriptFamiliesPathKey(request.WorkDir)
	router.mu.RLock()
	runner := router.runners[workDir]
	if runner == nil {
		for factoryDir, candidate := range router.runners {
			if javascriptFamiliesPathContains(factoryDir, workDir) {
				runner = candidate
				break
			}
		}
	}
	router.mu.RUnlock()
	if runner == nil {
		return platformprocess.CommandResult{}, fmt.Errorf(
			"JavaScript families provider runner is not registered for work directory %q",
			request.WorkDir,
		)
	}
	return runner.Run(ctx, request)
}

func javascriptFamiliesPathKey(path string) string {
	if absolute, err := filepath.Abs(path); err == nil {
		path = absolute
	}
	return filepath.Clean(path)
}

func javascriptFamiliesPathContains(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))
}

type javascriptFamiliesScenario struct {
	fixture          *javascriptFamiliesSharedFixture
	factoryName      string
	factoryDir       string
	environment      []string
	workingDirectory string
	selectorPaths    []string
	sessionID        string
}

func newJavascriptFamiliesSharedFixture(t *testing.T) *javascriptFamiliesSharedFixture {
	t.Helper()
	rootDir := t.TempDir()
	homeDir := filepath.Join(rootDir, "home")
	workingDirectory := filepath.Join(rootDir, "work")
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		t.Fatalf("create shared JavaScript families home: %v", err)
	}
	if err := os.MkdirAll(workingDirectory, 0o755); err != nil {
		t.Fatalf("create shared JavaScript families working directory: %v", err)
	}

	api := support.NewProcessAPIServer()
	provider := newJavascriptFamiliesProviderCommandRouter()
	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		APIServerStarter:      api.Start,
		ProviderCommandRunner: provider,
	})
	if err != nil {
		t.Fatalf("BuildProcess(JavaScript families): %v", err)
	}
	support.CleanupProcess(t, process)

	environment := javascriptFamiliesCustomerEnvironment(homeDir)
	factoryDirs := make(map[string]string, 2)
	for _, factoryName := range []string{
		factorydefinitions.PackagedTournamentFactoryName,
		factorydefinitions.PackagedSpawnFactoryName,
	} {
		factoryDirs[factoryName] = support.InstallPackagedFactoryWithProcess(
			t,
			process,
			environment,
			workingDirectory,
			factoryName,
		)
	}
	baseFactoryDir := factoryDirs[factorydefinitions.PackagedTournamentFactoryName]
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "run", "--dir", baseFactoryDir,
		"--continuously", "--with-server", "--quiet", "--no-record",
		"--provider", "CODEX", "--model", "operator-js-model",
	})
	inputs.Input.Env = environment
	inputs.Input.WorkingDirectory = baseFactoryDir
	support.StartProcessCommand(t, process, inputs.Input)
	baseURL := api.WaitForURL(t)
	support.WaitForStatus(t, baseURL, javascriptFamiliesSharedFixtureTimeout, func(status factoryapi.StatusResponse) bool {
		return strings.TrimSpace(status.RuntimeStatus) != ""
	})

	fixture := &javascriptFamiliesSharedFixture{
		process:     process,
		provider:    provider,
		baseURL:     baseURL,
		factoryDirs: factoryDirs,
	}
	t.Cleanup(func() {
		fixture.mu.Lock()
		opened, closed := fixture.opened, fixture.closed
		fixture.mu.Unlock()
		t.Logf("JavaScript families shared fixture: process_starts=1 explicit_sessions_opened=%d explicit_sessions_closed=%d behavior_rows=3 isolated_rows=0", opened, closed)
	})
	return fixture
}

func javascriptFamiliesCustomerEnvironment(homeDir string) []string {
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
		"YOU_DEFAULT_WORKER_MODEL=operator-js-model",
	)
}

func (fixture *javascriptFamiliesSharedFixture) newScenario(
	t *testing.T,
	factoryName string,
	runner platformprocess.CommandRunner,
) *javascriptFamiliesScenario {
	t.Helper()
	sourceDir, ok := fixture.factoryDirs[factoryName]
	if !ok {
		t.Fatalf("JavaScript families Factory %q was not installed in the shared fixture", factoryName)
	}
	homeDir := t.TempDir()
	workingDirectory := t.TempDir()
	factoryDir := support.CopyFactoryAsNamed(t, sourceDir, homeDir, factoryName)
	environment := javascriptFamiliesCustomerEnvironment(homeDir)
	selectorPaths := []string{
		factoryDir,
		workingDirectory,
		fixture.factoryDirs[factorydefinitions.PackagedTournamentFactoryName],
	}
	fixture.provider.register(selectorPaths, runner)
	t.Cleanup(func() {
		fixture.provider.unregister(selectorPaths)
	})
	return &javascriptFamiliesScenario{
		fixture:          fixture,
		factoryName:      factoryName,
		factoryDir:       factoryDir,
		environment:      environment,
		workingDirectory: workingDirectory,
		selectorPaths:    selectorPaths,
	}
}

func (scenario *javascriptFamiliesScenario) open(t *testing.T) {
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

var _ platformprocess.CommandRunner = (*javascriptFamiliesProviderCommandRouter)(nil)
