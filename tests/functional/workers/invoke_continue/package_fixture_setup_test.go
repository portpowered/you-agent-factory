package acceptance

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryinterfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type invokeContinueScenarioSetup struct {
	scenarios            []invokeContinueScenario
	routes               []invokeContinueStaticCommandRouteEntry
	managerRunner        *s8RemoteProviderRunner
	managerRepositoryA   s8Repository
	managerRepositoryB   s8Repository
	interruptRunner      *s8InterruptProviderRunner
	interruptRepositoryA s8Repository
	interruptRepositoryB s8Repository
}

type invokeContinueStartedProcess struct {
	process       support.ApplicationProcess
	command       *invokeContinuePackageCommand
	baseURL       string
	apiStopped    <-chan struct{}
	apiStarts     *atomic.Int32
	processBuilds *atomic.Int32
}

func prepareInvokeContinuePackageRoot(t *testing.T, rootDir string) (string, string, error) {
	t.Helper()
	hostDir := filepath.Join(rootDir, "host-factory")
	homeDir := filepath.Join(rootDir, "home")
	if err := copyInvokeContinueDirectory(support.LegacyFixtureDir(t, "executor_success"), hostDir); err != nil {
		return "", "", fmt.Errorf("copy host Factory: %w", err)
	}
	// The copied fixture is used only to provide a valid Factory definition for
	// the explicit session. Removing its seed inputs prevents the hosted
	// default session from consuming the CASE-01 provider results at startup.
	if err := os.RemoveAll(filepath.Join(hostDir, factoryinterfaces.InputsDir)); err != nil {
		return "", "", fmt.Errorf("clear host Factory seed inputs: %w", err)
	}
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		return "", "", fmt.Errorf("create package fixture home %q: %w", homeDir, err)
	}
	return hostDir, homeDir, nil
}

func appendInvokeContinueScenario(
	rootDir string,
	scenarios *[]invokeContinueScenario,
	routes *[]invokeContinueStaticCommandRouteEntry,
	name string,
	runner platformprocess.CommandRunner,
	providerRunner invokeContinueProviderCommandRunner,
	streamingRunner *wsrFT015StreamingProviderRunner,
	blockingRunner *invokeContinueBlockingProviderRunner,
	unsupportedProvider providers.Service,
	reset func(),
) error {
	workingDirectory := filepath.Join(rootDir, "routes", name)
	scenarioHome := filepath.Join(rootDir, "scenario-homes", name)
	for _, dir := range []string{workingDirectory, scenarioHome} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create %s scenario directory %q: %w", name, dir, err)
		}
	}
	*scenarios = append(*scenarios, invokeContinueScenario{
		name:                name,
		workingDirectory:    workingDirectory,
		homeDirectory:       scenarioHome,
		providerRunner:      providerRunner,
		streamingRunner:     streamingRunner,
		blockingRunner:      blockingRunner,
		unsupportedProvider: unsupportedProvider,
		reset:               reset,
	})
	*routes = append(*routes, invokeContinueStaticCommandRouteEntry{
		workingDirectory: workingDirectory,
		runner:           runner,
	})
	return nil
}

func newInvokeContinueScenarioSetup(t *testing.T, rootDir, homeDir string) (invokeContinueScenarioSetup, error) {
	t.Helper()
	direct, err := newInvokeContinueDirectScenarioSetup(t, rootDir)
	if err != nil {
		return invokeContinueScenarioSetup{}, err
	}
	manager, err := newInvokeContinueManagerScenarioSetup(t, rootDir, homeDir)
	if err != nil {
		return invokeContinueScenarioSetup{}, err
	}
	return invokeContinueScenarioSetup{
		scenarios:            append(direct.scenarios, manager.scenarios...),
		routes:               append(direct.routes, manager.routes...),
		managerRunner:        manager.managerRunner,
		managerRepositoryA:   manager.managerRepositoryA,
		managerRepositoryB:   manager.managerRepositoryB,
		interruptRunner:      manager.interruptRunner,
		interruptRepositoryA: manager.interruptRepositoryA,
		interruptRepositoryB: manager.interruptRepositoryB,
	}, nil
}

func newInvokeContinueDirectScenarioSetup(t *testing.T, rootDir string) (invokeContinueScenarioSetup, error) {
	t.Helper()
	setup := invokeContinueScenarioSetup{
		scenarios: make([]invokeContinueScenario, 0, 16),
		routes:    make([]invokeContinueStaticCommandRouteEntry, 0, 16),
	}
	localRunner := testutil.NewProviderCommandRunner(
		platformprocess.CommandResult{Stdout: directCodexSessionOutput("local-source-thread", "initial direct output COMPLETE")},
		platformprocess.CommandResult{Stdout: directCodexSessionOutput("local-source-thread", "continued direct output COMPLETE")},
	)
	if err := appendInvokeContinueScenario(rootDir, &setup.scenarios, &setup.routes, "local", localRunner, localRunner, nil, nil, nil, nil); err != nil {
		return invokeContinueScenarioSetup{}, err
	}
	futureRunner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stdout: directCodexSessionOutput("future-file-thread", "future-file output COMPLETE"),
	})
	if err := appendInvokeContinueScenario(rootDir, &setup.scenarios, &setup.routes, "future-fields", futureRunner, futureRunner, nil, nil, nil, nil); err != nil {
		return invokeContinueScenarioSetup{}, err
	}
	streamingRunner := newWSRFT015StreamingProviderRunner()
	if err := appendInvokeContinueScenario(rootDir, &setup.scenarios, &setup.routes, "recorded-provider-session", streamingRunner, nil, streamingRunner, nil, nil, nil); err != nil {
		return invokeContinueScenarioSetup{}, err
	}
	unassociatedRunner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stdout: directCodexOutputWithoutSession("completed without a Provider Session"),
	})
	if err := appendInvokeContinueScenario(rootDir, &setup.scenarios, &setup.routes, "unassociated-source", unassociatedRunner, unassociatedRunner, nil, nil, nil, nil); err != nil {
		return invokeContinueScenarioSetup{}, err
	}
	staleRunner := testutil.NewProviderCommandRunner(
		platformprocess.CommandResult{Stdout: directCodexSessionOutput("stale-source-thread", "initial output")},
		platformprocess.CommandResult{Stderr: []byte("Error: thread/resume failed: no rollout found for thread id stale-source-thread"), ExitCode: 1},
	)
	if err := appendInvokeContinueScenario(rootDir, &setup.scenarios, &setup.routes, "stale-provider-session", staleRunner, staleRunner, nil, nil, nil, nil); err != nil {
		return invokeContinueScenarioSetup{}, err
	}
	duplicateRunner := newInvokeContinueResettableProviderCommandRunner(
		platformprocess.CommandResult{Stdout: directCodexSessionOutput("duplicate-source-thread", "duplicate initial output")},
		platformprocess.CommandResult{Stdout: directCodexSessionOutput("duplicate-source-thread", "duplicate continued output")},
	)
	if err := appendInvokeContinueScenario(rootDir, &setup.scenarios, &setup.routes, "duplicate-continuation", duplicateRunner, duplicateRunner, nil, nil, nil, duplicateRunner.Reset); err != nil {
		return invokeContinueScenarioSetup{}, err
	}
	blockingRunner := newInvokeContinueBlockingProviderRunner()
	if err := appendInvokeContinueScenario(rootDir, &setup.scenarios, &setup.routes, "dependency-cancellation", blockingRunner, nil, nil, blockingRunner, nil, blockingRunner.reset); err != nil {
		return invokeContinueScenarioSetup{}, err
	}
	recoveryRunner := newInvokeContinueResettableProviderCommandRunner(platformprocess.CommandResult{
		Stdout: directCodexSessionOutput("timeout-recovery-thread", "timeout recovery output COMPLETE"),
	})
	if err := appendInvokeContinueScenario(rootDir, &setup.scenarios, &setup.routes, "cancellation-recovery", recoveryRunner, recoveryRunner, nil, nil, nil, recoveryRunner.Reset); err != nil {
		return invokeContinueScenarioSetup{}, err
	}
	unsupportedRunner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stdout: directCodexSessionOutput("unsupported-source-thread", "initial provider output"),
	})
	if err := appendInvokeContinueScenario(rootDir, &setup.scenarios, &setup.routes, "unsupported-provider", unsupportedRunner, unsupportedRunner, nil, nil, nil, nil); err != nil {
		return invokeContinueScenarioSetup{}, err
	}
	for _, name := range []string{"unknown-source", "empty-input", "remote-interrupt", "remote-controls", "remote-continue-failures", "remote-stream-failure", "remote-cancellation"} {
		runner := testutil.NewProviderCommandRunner()
		if err := appendInvokeContinueScenario(rootDir, &setup.scenarios, &setup.routes, name, runner, runner, nil, nil, nil, nil); err != nil {
			return invokeContinueScenarioSetup{}, err
		}
	}
	return setup, nil
}

func newInvokeContinueManagerScenarioSetup(t *testing.T, rootDir, homeDir string) (invokeContinueScenarioSetup, error) {
	t.Helper()
	managerRepositoryA, err := newS8RepositoryAt(filepath.Join(rootDir, "routes", "manager-isolation-a"), s8RepositoryAMarker)
	if err != nil {
		return invokeContinueScenarioSetup{}, fmt.Errorf("create manager isolation repository A: %w", err)
	}
	managerRepositoryB, err := newS8RepositoryAt(filepath.Join(rootDir, "routes", "manager-isolation-b"), s8RepositoryBMarker)
	if err != nil {
		return invokeContinueScenarioSetup{}, fmt.Errorf("create manager isolation repository B: %w", err)
	}
	stdout := readS8ProviderFixture(t, "stdout.jsonl")
	rollout := readS8ProviderFixture(t, "rollout.jsonl")
	managerRunner := newS8RemoteProviderRunner(stdout,
		s8RemoteProviderCase{repository: managerRepositoryA.path, marker: managerRepositoryA.marker, sessionID: s8ProviderSessionA, output: s8OutputA, release: make(chan struct{}), started: make(chan struct{})},
		s8RemoteProviderCase{repository: managerRepositoryB.path, marker: managerRepositoryB.marker, sessionID: s8ProviderSessionB, output: s8OutputB, release: make(chan struct{}), started: make(chan struct{})},
	)
	writeS8CodexRollout(t, homeDir, s8ProviderSessionA, rollout, s8OutputA)
	writeS8CodexRollout(t, homeDir, s8ProviderSessionB, rollout, s8OutputB)
	setup := invokeContinueScenarioSetup{
		scenarios:          make([]invokeContinueScenario, 0, 2),
		routes:             make([]invokeContinueStaticCommandRouteEntry, 0, 6),
		managerRunner:      managerRunner,
		managerRepositoryA: managerRepositoryA,
		managerRepositoryB: managerRepositoryB,
	}
	if err := appendInvokeContinueScenario(rootDir, &setup.scenarios, &setup.routes, "manager-isolation", managerRunner, managerRunner, nil, nil, nil, managerRunner.reset); err != nil {
		return invokeContinueScenarioSetup{}, err
	}
	setup.routes = append(setup.routes,
		invokeContinueStaticCommandRouteEntry{workingDirectory: managerRepositoryA.path, runner: managerRunner},
		invokeContinueStaticCommandRouteEntry{workingDirectory: managerRepositoryB.path, runner: managerRunner},
	)
	interruptRepositoryA, err := newS8RepositoryAt(filepath.Join(rootDir, "routes", "manager-interrupt-a"), s8RepositoryAMarker)
	if err != nil {
		return invokeContinueScenarioSetup{}, fmt.Errorf("create manager interrupt repository A: %w", err)
	}
	interruptRepositoryB, err := newS8RepositoryAt(filepath.Join(rootDir, "routes", "manager-interrupt-b"), s8RepositoryBMarker)
	if err != nil {
		return invokeContinueScenarioSetup{}, fmt.Errorf("create manager interrupt repository B: %w", err)
	}
	interruptRunner := newS8InterruptProviderRunner(stdout, interruptRepositoryA, interruptRepositoryB)
	writeS8CodexRollout(t, homeDir, s8InterruptProviderSessionA, rollout, s8ReplacementOutput)
	writeS8CodexRollout(t, homeDir, s8InterruptProviderSessionB, rollout, s8OutputB)
	if err := appendInvokeContinueScenario(rootDir, &setup.scenarios, &setup.routes, "manager-interrupt", interruptRunner, interruptRunner, nil, nil, nil, interruptRunner.reset); err != nil {
		return invokeContinueScenarioSetup{}, err
	}
	setup.routes = append(setup.routes,
		invokeContinueStaticCommandRouteEntry{workingDirectory: interruptRepositoryA.path, runner: interruptRunner},
		invokeContinueStaticCommandRouteEntry{workingDirectory: interruptRepositoryB.path, runner: interruptRunner},
	)
	setup.interruptRunner = interruptRunner
	setup.interruptRepositoryA = interruptRepositoryA
	setup.interruptRepositoryB = interruptRepositoryB
	return setup, nil
}

func startInvokeContinuePackageProcess(
	t *testing.T,
	rootDir, hostDir, homeDir string,
	route *invokeContinueStaticCommandRoute,
	unsupportedProvider providers.Service,
) (invokeContinueStartedProcess, error) {
	t.Helper()
	fallbackProvider, err := providerswire.NewService(providerswire.WithCommandRunner(route))
	if err != nil {
		return invokeContinueStartedProcess{}, fmt.Errorf("build fixture provider fallback: %w", err)
	}
	providerOverride := &invokeContinueProviderRouter{
		fallback:                    fallbackProvider,
		unsupported:                 unsupportedProvider,
		unsupportedWorkingDirectory: filepath.Join(rootDir, "routes", "unsupported-provider"),
	}
	api := support.NewProcessAPIServer()
	apiStopped := make(chan struct{})
	var apiStopOnce sync.Once
	apiStarts := &atomic.Int32{}
	processBuilds := &atomic.Int32{}
	processBuilds.Add(1)
	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		// This route is complete before root construction and has no registration
		// or session-based fallback after the process starts.
		ProviderCommandRunner: route,
		ProviderOverride:      providerOverride,
		ProviderSessionResolveHomeDirectory: func() (string, error) {
			return homeDir, nil
		},
		WorkerSessionResolveHomeDirectory: func() (string, error) {
			return homeDir, nil
		},
		APIServerStarter: func(ctx context.Context, request platformhttpserver.StartRequest) error {
			apiStarts.Add(1)
			err := api.Start(ctx, request)
			apiStopOnce.Do(func() { close(apiStopped) })
			return err
		},
	})
	if err != nil {
		return invokeContinueStartedProcess{}, fmt.Errorf("BuildProcess: %w", err)
	}
	inputs := support.FakeInputs(context.Background(), []string{
		"you", "run", "--dir", hostDir, "--continuously", "--with-server", "--server", "http://127.0.0.1:1", "--quiet", "--no-record",
	})
	inputs.Input.Env = invokeContinueEnvironment(homeDir)
	inputs.Input.WorkingDirectory = hostDir
	command := startInvokeContinuePackageCommand(process, inputs)
	baseURL, err := api.WaitForBaseURL(invokeContinuePackageFixtureTimeout)
	if err != nil {
		_ = command.stop()
		closeCtx, cancel := context.WithTimeout(context.Background(), invokeContinuePackageFixtureTimeout)
		_ = process.Close(closeCtx)
		cancel()
		return invokeContinueStartedProcess{}, fmt.Errorf("wait for package fixture API: %w", err)
	}
	return invokeContinueStartedProcess{process: process, command: command, baseURL: baseURL, apiStopped: apiStopped, apiStarts: apiStarts, processBuilds: processBuilds}, nil
}
