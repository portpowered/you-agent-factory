package fullflow

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
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const fullFlowSharedFixtureTimeout = 15 * time.Second

// fullFlowSharedFixture owns one root-built process and one continuous API
// host for all compatible Full Flow scenarios. Each child keeps its Factory,
// repository, worktrees, and Factory Session private while only the immutable
// process wiring and API listener are reused.
type fullFlowSharedFixture struct {
	process    support.ApplicationProcess
	provider   *fullFlowProviderCommandRouter
	baseURL    string
	factoryDir string
	requestID  atomic.Uint64
}

// fullFlowProviderCommandRouter keeps the shared process edge immutable while
// selecting a scenario-owned runner by repository or worktree path.
type fullFlowProviderCommandRouter struct {
	mu      sync.RWMutex
	runners map[string]platformprocess.CommandRunner
}

func newFullFlowProviderCommandRouter() *fullFlowProviderCommandRouter {
	return &fullFlowProviderCommandRouter{runners: make(map[string]platformprocess.CommandRunner)}
}

func (router *fullFlowProviderCommandRouter) register(
	paths []string,
	runner platformprocess.CommandRunner,
) {
	router.mu.Lock()
	defer router.mu.Unlock()
	for _, path := range paths {
		router.runners[fullFlowPathKey(path)] = runner
	}
}

func (router *fullFlowProviderCommandRouter) unregister(paths []string) {
	router.mu.Lock()
	defer router.mu.Unlock()
	for _, path := range paths {
		delete(router.runners, fullFlowPathKey(path))
	}
}

func (router *fullFlowProviderCommandRouter) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	workDir := fullFlowPathKey(request.WorkDir)
	router.mu.RLock()
	runner := router.runners[workDir]
	if runner == nil {
		for root, candidate := range router.runners {
			if fullFlowPathContains(root, workDir) {
				runner = candidate
				break
			}
		}
	}
	router.mu.RUnlock()
	if runner == nil {
		return platformprocess.CommandResult{}, fmt.Errorf(
			"Full Flow provider runner is not registered for work directory %q",
			request.WorkDir,
		)
	}
	return runner.Run(ctx, request)
}

func fullFlowPathKey(path string) string {
	if absolute, err := filepath.Abs(path); err == nil {
		path = absolute
	}
	return filepath.Clean(path)
}

func fullFlowPathContains(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))
}

type fullFlowScenario struct {
	fixture       *fullFlowSharedFixture
	factoryDir    string
	repository    string
	environment   []string
	selectorPaths []string
	sessionID     string
}

func newFullFlowSharedFixture(t *testing.T) *fullFlowSharedFixture {
	t.Helper()
	rootDir := t.TempDir()
	homeDir := filepath.Join(rootDir, "home")
	workingDirectory := filepath.Join(rootDir, "work")
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		t.Fatalf("create shared Full Flow home: %v", err)
	}
	if err := os.MkdirAll(workingDirectory, 0o755); err != nil {
		t.Fatalf("create shared Full Flow working directory: %v", err)
	}

	api := support.NewProcessAPIServer()
	provider := newFullFlowProviderCommandRouter()
	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		APIServerStarter:      api.Start,
		ProviderCommandRunner: provider,
	})
	if err != nil {
		t.Fatalf("BuildProcess(Full Flow): %v", err)
	}
	fixture := &fullFlowSharedFixture{
		process:  process,
		provider: provider,
	}
	support.CleanupProcess(t, process)

	environment := fullFlowCustomerEnvironment(homeDir)
	factoryDir := support.InstallPackagedFactoryWithProcess(
		t,
		process,
		environment,
		workingDirectory,
		factorydefinitions.PackagedFullFlowFactoryName,
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
	support.WaitForStatus(t, baseURL, fullFlowSharedFixtureTimeout, func(status factoryapi.StatusResponse) bool {
		return strings.TrimSpace(status.RuntimeStatus) != ""
	})

	fixture.baseURL = baseURL
	fixture.factoryDir = factoryDir
	return fixture
}

func fullFlowCustomerEnvironment(homeDir string) []string {
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

func (fixture *fullFlowSharedFixture) nextRequestID() uint64 {
	return fixture.requestID.Add(1)
}

func (fixture *fullFlowSharedFixture) newScenario(
	t *testing.T,
	runner platformprocess.CommandRunner,
) *fullFlowScenario {
	t.Helper()
	rootDir := t.TempDir()
	homeDir := filepath.Join(rootDir, "home")
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		t.Fatalf("create Full Flow scenario home: %v", err)
	}
	factoryDir := support.CopyFactoryAsNamed(
		t,
		fixture.factoryDir,
		homeDir,
		factorydefinitions.PackagedFullFlowFactoryName,
	)
	repository := initializeFullFlowRepositoryAt(t, factoryDir)
	environment := fullFlowCustomerEnvironment(homeDir)
	selectorPaths := []string{factoryDir, repository}
	fixture.provider.register(selectorPaths, runner)
	t.Cleanup(func() {
		fixture.provider.unregister(selectorPaths)
	})
	return &fullFlowScenario{
		fixture:       fixture,
		factoryDir:    factoryDir,
		repository:    repository,
		environment:   environment,
		selectorPaths: selectorPaths,
	}
}

func (scenario *fullFlowScenario) open(t *testing.T) {
	t.Helper()
	opened := support.OpenFactorySessionAt(t, scenario.fixture.baseURL, scenario.factoryDir)
	scenario.sessionID = opened.Session.Id
	t.Cleanup(func() {
		scenario.close(t)
	})
}

func (scenario *fullFlowScenario) close(t testing.TB) {
	t.Helper()
	if scenario == nil || scenario.sessionID == "" {
		return
	}
	support.CloseFactorySessionAt(t, scenario.fixture.baseURL, scenario.sessionID)
}

var _ platformprocess.CommandRunner = (*fullFlowProviderCommandRouter)(nil)
