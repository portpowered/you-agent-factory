package loop

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
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const loopSharedFixtureTimeout = 15 * time.Second

// loopSharedFixture owns one root-built process and one continuous API host
// for the package's compatible scheduler scenarios. Each child copies the
// packaged Factory into a private home and opens a non-default Factory Session.
type loopSharedFixture struct {
	process     support.ApplicationProcess
	provider    *loopProviderCommandRouter
	clock       *loopSchedulerClock
	baseURL     string
	factoryDir  string
	submissions chan work.FactorySubmissionRecord

	mu     sync.Mutex
	opened int
	closed int
}

// loopProviderCommandRouter keeps the process edge immutable while selecting
// each scenario's synchronized runner by Factory or work path.
type loopProviderCommandRouter struct {
	mu      sync.RWMutex
	runners map[string]platformprocess.CommandRunner
}

func newLoopProviderCommandRouter() *loopProviderCommandRouter {
	return &loopProviderCommandRouter{runners: make(map[string]platformprocess.CommandRunner)}
}

func (router *loopProviderCommandRouter) register(
	paths []string,
	runner platformprocess.CommandRunner,
) {
	router.mu.Lock()
	defer router.mu.Unlock()
	for _, path := range paths {
		router.runners[loopPathKey(path)] = runner
	}
}

func (router *loopProviderCommandRouter) unregister(paths []string) {
	router.mu.Lock()
	defer router.mu.Unlock()
	for _, path := range paths {
		delete(router.runners, loopPathKey(path))
	}
}

func (router *loopProviderCommandRouter) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	workDir := loopPathKey(request.WorkDir)
	router.mu.RLock()
	runner := router.runners[workDir]
	if runner == nil {
		for factoryDir, candidate := range router.runners {
			if loopPathContains(factoryDir, workDir) {
				runner = candidate
				break
			}
		}
	}
	router.mu.RUnlock()
	if runner == nil {
		return platformprocess.CommandResult{}, fmt.Errorf(
			"loop provider runner is not registered for work directory %q",
			request.WorkDir,
		)
	}
	return runner.Run(ctx, request)
}

func loopPathKey(path string) string {
	if absolute, err := filepath.Abs(path); err == nil {
		path = absolute
	}
	return filepath.Clean(path)
}

func loopPathContains(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))
}

type loopScenario struct {
	fixture          *loopSharedFixture
	factoryDir       string
	environment      []string
	workingDirectory string
	selectorPaths    []string
	sessionID        string
}

func newLoopSharedFixture(t *testing.T) *loopSharedFixture {
	t.Helper()
	rootDir := t.TempDir()
	homeDir := filepath.Join(rootDir, "home")
	workingDirectory := filepath.Join(rootDir, "work")
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		t.Fatalf("create shared loop home: %v", err)
	}
	if err := os.MkdirAll(workingDirectory, 0o755); err != nil {
		t.Fatalf("create shared loop working directory: %v", err)
	}

	api := support.NewProcessAPIServer()
	provider := newLoopProviderCommandRouter()
	clock := newLoopSchedulerClockAt(time.Date(2026, time.July, 29, 20, 0, 0, 0, time.UTC))
	submissions := make(chan work.FactorySubmissionRecord, 16)
	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		APIServerStarter:      api.Start,
		Clock:                 clock,
		ProviderCommandRunner: provider,
		SubmissionRecorder:    func(record work.FactorySubmissionRecord) { submissions <- record },
	})
	if err != nil {
		t.Fatalf("BuildProcess(loop): %v", err)
	}
	support.CleanupProcess(t, process)

	environment := loopCustomerEnvironment(homeDir)
	factoryDir := support.InstallPackagedFactoryWithProcess(
		t,
		process,
		environment,
		workingDirectory,
		factorydefinitions.PackagedLoopFactoryName,
	)
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "run", "--dir", factoryDir,
		"--continuously", "--with-server", "--quiet", "--no-record",
		"--provider", "CODEX", "--model", "operator-model",
	})
	inputs.Input.Env = environment
	inputs.Input.WorkingDirectory = factoryDir
	support.StartProcessCommand(t, process, inputs.Input)
	baseURL := api.WaitForURL(t)
	support.WaitForStatus(t, baseURL, loopSharedFixtureTimeout, func(status factoryapi.StatusResponse) bool {
		return strings.TrimSpace(status.RuntimeStatus) != ""
	})

	fixture := &loopSharedFixture{
		process:     process,
		provider:    provider,
		clock:       clock,
		baseURL:     baseURL,
		factoryDir:  factoryDir,
		submissions: submissions,
	}
	t.Cleanup(func() {
		fixture.mu.Lock()
		opened, closed := fixture.opened, fixture.closed
		fixture.mu.Unlock()
		t.Logf("loop shared fixture: process_starts=1 explicit_sessions_opened=%d explicit_sessions_closed=%d behavior_rows=2 isolated_rows=0", opened, closed)
	})
	return fixture
}

func loopCustomerEnvironment(homeDir string) []string {
	environment := make([]string, 0, len(os.Environ())+2)
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
	return append(environment, "HOME="+homeDir, "USERPROFILE="+homeDir)
}

func (fixture *loopSharedFixture) newScenario(
	t *testing.T,
	runner platformprocess.CommandRunner,
) *loopScenario {
	t.Helper()
	homeDir := t.TempDir()
	workingDirectory := t.TempDir()
	factoryDir := support.CopyFactoryAsNamed(
		t,
		fixture.factoryDir,
		homeDir,
		factorydefinitions.PackagedLoopFactoryName,
	)
	environment := loopCustomerEnvironment(homeDir)
	selectorPaths := []string{factoryDir, workingDirectory, fixture.factoryDir}
	fixture.provider.register(selectorPaths, runner)
	t.Cleanup(func() {
		fixture.provider.unregister(selectorPaths)
	})
	return &loopScenario{
		fixture:          fixture,
		factoryDir:       factoryDir,
		environment:      environment,
		workingDirectory: workingDirectory,
		selectorPaths:    selectorPaths,
	}
}

func (scenario *loopScenario) open(t *testing.T) {
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

var _ platformprocess.CommandRunner = (*loopProviderCommandRouter)(nil)
