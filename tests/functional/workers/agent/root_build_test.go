package agent_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	workerswire "github.com/portpowered/infinite-you/pkg/services/workers/wire"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestAgentRunnerDeliveryRemainsInertThroughRootBuildProcessConstruction proves
// root.BuildProcess stays inert while its public provider identity capability is
// available for composition.
func TestAgentRunnerDeliveryRemainsInertThroughRootBuildProcessConstruction(t *testing.T) {
	t.Parallel()

	process, err := root.BuildProcess(context.Background(), serviceedges.Edges{})
	if err != nil {
		t.Fatalf("root.BuildProcess() error = %v", err)
	}
	if process == nil || process.ProviderRegistry() == nil {
		t.Fatal("root-built process or provider registry = nil, want inert composition")
	}
	for _, providerID := range []string{"claude", "codex"} {
		if got, err := process.ProviderRegistry().CanonicalIdentity(providerID); err != nil || got != providerID {
			t.Fatalf("CanonicalIdentity(%q) = (%q, %v), want (%q, nil)", providerID, got, err, providerID)
		}
	}
	if _, err := process.ProviderRegistry().CanonicalIdentity("missing.provider"); err == nil {
		t.Fatal("CanonicalIdentity(missing.provider) error = nil, want unknown-provider failure")
	}
}

// TestBuildProcessExecutesModelWorkerThroughConvergedWorkersService proves
// the customer process reaches the shared Workers execution path through the
// public Process.Execute boundary. The provider override replaces only the
// external inference effect; terminal Work state remains observable through
// the public API projection.
func TestBuildProcessExecutesModelWorkerThroughConvergedWorkersService(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	testutil.WriteSeedFile(t, dir, "task", []byte("converged agent payload"))
	provider := testutil.NewMockProvider(workerexecution.InferenceResponse{
		Content: "converged agent response COMPLETE",
	})

	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:       dir,
		ProviderOverride: provider,
	})
	support.WaitForTerminalStatus(t, server.URL(), 10*time.Second)
	listed := support.ListDefaultSessionWork(t, server.URL())
	server.Stop(t)

	if provider.CallCount() != 1 {
		t.Fatalf("provider override calls = %d, want exactly one", provider.CallCount())
	}
	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("completed customer Work = %d, want one; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 0 {
		t.Fatalf("failed customer Work = %d, want zero; listed=%#v", got, listed)
	}
}

// TestBuildProcessResolvesRegisteredAgentThroughProviders proves the normal
// production composition selects the registered Agent runner and reaches the
// Providers command edge through the public Process.Execute path. This keeps
// the test distinct from the provider-override seam above: registry selection
// and provider resolution are part of the behavior under test.
func TestBuildProcessResolvesRegisteredAgentThroughProviders(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(
		modelprovider.ProviderCodex,
		"converged-agent-model",
	))
	testutil.WriteSeedFile(t, dir, "task", []byte("registered agent payload"))
	runner := support.NewShapedProviderCommandRunner(platformprocess.CommandResult{
		Stdout: []byte("registered agent response COMPLETE"),
	})

	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir,
		Edges: serviceedges.Edges{
			ProviderCommandRunner: runner,
		},
	})
	support.WaitForTerminalStatus(t, server.URL(), 10*time.Second)
	listed := support.ListDefaultSessionWork(t, server.URL())
	server.Stop(t)

	if runner.CallCount() != 1 {
		t.Fatalf("provider command runner calls = %d, want exactly one", runner.CallCount())
	}
	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("completed customer Work = %d, want one; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 0 {
		t.Fatalf("failed customer Work = %d, want zero; listed=%#v", got, listed)
	}
}

// TestMockWorkersWireCompositionExecutesSelectedMock proves the explicit mock
// feature graph is opt-in while still using the canonical Workers Execute
// contract. This is the one composition path that cannot be reached through a
// production root.BuildProcess, whose registry intentionally rejects mock.
func TestMockWorkersWireCompositionExecutesSelectedMock(t *testing.T) {
	service, err := workerswire.NewMockService(
		workerswire.AgentDependencies{
			Providers: functionalMockProviders{},
			Publish:   func(workerexecution.ProgressFragment) {},
		},
		workerswire.ScriptConfig{RequestSelected: true},
		workerswire.ScriptDependencies{
			CommandRunner: functionalMockCommandRunner{},
			FactoryDocs:   func(string) (map[string]string, error) { return nil, nil },
			Now:           time.Now,
			Publish:       func(workerexecution.ProgressFragment) {},
			Record:        func(workerexecution.ScriptEvent) {},
		},
		workerswire.InferenceConfig{
			Worker: modelprovider.LocalWorker{
				Name: "functional-inference",
			},
		},
		workerswire.InferenceDependencies{Models: functionalMockModels{}},
		&workerexecution.MockWorkersConfig{
			MockWorkers: []workerexecution.MockWorkerConfig{{
				WorkerName: "mock-worker",
				RunType:    workerexecution.MockWorkerRunTypeAccept,
			}},
		},
		workerswire.MockDependencies{},
		nil,
		nil,
		time.Now,
		nil,
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("workerswire.NewMockService() error = %v", err)
	}

	request := workerexecution.ExecuteRequest{
		Correlation: workerexecution.ExecutionCorrelation{
			FactorySessionID: "functional-mock-session",
			RuntimeID:        "functional-mock-runtime",
			GenerationID:     "functional-mock-generation",
			DispatchID:       "functional-mock-dispatch",
			AttemptID:        "functional-mock-attempt",
		},
		Target: workerexecution.ExecutionTarget{
			WorkerName: "mock-worker",
			RunnerID:   "mock",
		},
	}
	result, err := service.Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("mock Workers Execute() error = %v", err)
	}
	if result.Correlation != request.Correlation {
		t.Fatalf("mock result correlation = %#v, want %#v", result.Correlation, request.Correlation)
	}
	if result.Outcome != workerexecution.ExecutionOutcomeAccepted ||
		len(result.Output.Primary) != 1 || result.Output.Primary[0].Text != "mock worker accepted" {
		t.Fatalf("mock result = %#v, want accepted canonical output", result)
	}
}

// TestBuildStatelessWorkersRejectsMockSelectionInProduction proves the normal
// public root composition does not accidentally expose the opt-in mock
// registration when a caller requests the mock identity.
func TestBuildStatelessWorkersRejectsMockSelectionInProduction(t *testing.T) {
	service, err := root.BuildStatelessWorkers(t.Context(), serviceedges.Edges{
		ScriptCommandRunner: support.NewStaticSuccessCommandRunner("unused"),
	})
	if err != nil {
		t.Fatalf("root.BuildStatelessWorkers() error = %v", err)
	}

	result, err := service.Execute(context.Background(), workerexecution.ExecuteRequest{
		Correlation: workerexecution.ExecutionCorrelation{
			DispatchID: "production-mock-rejection",
		},
		Target: workerexecution.ExecutionTarget{
			WorkerName: "mock-worker",
			RunnerID:   "mock",
		},
		Input: workerexecution.ExecutionInput{
			MockWorkers: &workerexecution.MockWorkersConfig{
				MockWorkers: []workerexecution.MockWorkerConfig{{
					WorkerName: "mock-worker",
					RunType:    workerexecution.MockWorkerRunTypeAccept,
				}},
			},
		},
	})
	if !errors.Is(err, workerexecution.ErrInvalidExecuteRequest) {
		t.Fatalf("production mock Execute() error = %v, want invalid execute request", err)
	}
	if result.Outcome != "" || len(result.Output.Primary) != 0 {
		t.Fatalf("production mock result = %#v, want no executed outcome", result)
	}
}

type functionalMockProviders struct{ providers.Service }

type functionalMockModels struct{}

type functionalMockCommandRunner struct{}

func (functionalMockCommandRunner) Run(
	context.Context,
	workerexecution.CommandRequest,
) (workerexecution.CommandResult, error) {
	return workerexecution.CommandResult{Stdout: []byte("unused")}, nil
}

func (functionalMockCommandRunner) RunStreaming(
	ctx context.Context,
	request workerexecution.CommandRequest,
	observer workerexecution.OutputChunkObserver,
) (workerexecution.CommandResult, error) {
	result, err := functionalMockCommandRunner{}.Run(ctx, request)
	if observer != nil && len(result.Stdout) > 0 {
		observer(workerexecution.OutputStreamStdout, result.Stdout)
	}
	return result, err
}

func (functionalMockModels) InvokeLocal(
	context.Context,
	modelprovider.LocalInvocationRequest,
) (modelprovider.LocalInvocationResult, error) {
	return modelprovider.LocalInvocationResult{Handled: false}, nil
}
