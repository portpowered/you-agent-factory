package construction

import (
	"os"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil/runtimefixtures"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners"
	workeragentrun "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/executor/agentrun"
)

func TestServiceCopiesRetainConvergedRunnerDependencies(t *testing.T) {
	t.Parallel()

	if got := (*Service)(nil).WithRunWorktree("worktree"); got != nil {
		t.Fatalf("nil Service.WithRunWorktree() = %#v, want nil", got)
	}
	if got := (*Service)(nil).WithRunReasoningEffort("high"); got != nil {
		t.Fatalf("nil Service.WithRunReasoningEffort() = %#v, want nil", got)
	}
	if got := (*Service)(nil).WithRunnerRegistry(nil); got != nil {
		t.Fatalf("nil Service.WithRunnerRegistry() = %#v, want nil", got)
	}

	service := New(
		nil,
		nil,
		nil,
		testFactoryDocs,
		nil,
		workeragentrun.NewLibraryHarnessAdapter(platformfilesystem.Local{}),
		testRetryRandom,
		platformfilesystem.Local{},
	)
	registry := &captureRunnerRegistry{}
	configured := service.
		WithRunWorktree("/tmp/worktree").
		WithRunReasoningEffort("high").
		WithRunnerRegistry(registry)
	if configured == service {
		t.Fatal("service copy mutated the original")
	}
	if configured.runWorktree != "/tmp/worktree" || configured.runReasoningEffort != "high" {
		t.Fatalf("run-scoped options = %q/%q, want retained options", configured.runWorktree, configured.runReasoningEffort)
	}
	if configured.runnerRegistry != registry {
		t.Fatal("WithRunnerRegistry() did not retain the injected registry")
	}
	rebuilt := configured.WithExecutionFactories(nil)
	if rebuilt.runnerRegistry != registry {
		t.Fatal("WithExecutionFactories() dropped the retained registry")
	}
}

func TestAgentRunnerUsesRetainedRegistryForInferenceSelection(t *testing.T) {
	registry := &captureRunnerRegistry{}
	service := New(
		nil,
		nil,
		nil,
		testFactoryDocs,
		nil,
		workeragentrun.NewLibraryHarnessAdapter(platformfilesystem.Local{}),
		testRetryRandom,
		platformfilesystem.Local{},
	).WithRunnerRegistry(registry)

	runner, err := service.agentRunner(
		runtimefixtures.RuntimeConfigLookupFixture{},
		&interfaces.FactoryWorkerConfig{Type: interfaces.WorkerTypeInference},
		logging.NoopLogger{},
		false,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("agentRunner() error = %v", err)
	}
	if _, err := runner.Execute(t.Context(), workers.RunnerExecutionRequest{
		RunnerID:       workers.RunnerIDCodex,
		ModelOperation: "transcribe",
	}); err != nil {
		t.Fatalf("retained registry Execute() error = %v", err)
	}
	if registry.request.Identity != runners.InferenceIdentity {
		t.Fatalf("registry identity = %q, want inference", registry.request.Identity)
	}
	if registry.request.Attempt.RunnerID != workers.RunnerIDCodex ||
		registry.request.Attempt.ModelOperation != "transcribe" {
		t.Fatalf("registry attempt = %#v, want caller request identity and operation", registry.request.Attempt)
	}
}

func TestServiceBuildLogicalUsesConfiguredConstructionDependencies(t *testing.T) {
	service := New(
		nil,
		nil,
		nil,
		testFactoryDocs,
		nil,
		workeragentrun.NewLibraryHarnessAdapter(platformfilesystem.Local{}),
		testRetryRandom,
		platformfilesystem.Local{},
	)
	result := service.BuildLogical(
		runtimefixtures.RuntimeConfigLookupFixture{},
		"logical",
		workers.RunnerIDCodex,
		nil,
		logging.NoopLogger{},
		testClock,
		os.Environ,
		os.Getwd,
	)
	if result.Dispatch == nil || result.Direct != nil {
		t.Fatalf("BuildLogical() = %#v, want dispatch-only result", result)
	}
}

func TestConstructionAdaptersPreserveExecutionBoundary(t *testing.T) {
	t.Parallel()

	registry := &captureRunnerRegistry{}
	runner := registryRunner{registry: registry, identity: runners.AgentIdentity}
	if _, err := runner.Execute(t.Context(), workers.RunnerExecutionRequest{RunnerID: workers.RunnerIDCodex}); err != nil {
		t.Fatalf("registryRunner.Execute() error = %v", err)
	}
	if registry.request.Identity != runners.AgentIdentity {
		t.Fatalf("registryRunner identity = %q, want agent", registry.request.Identity)
	}

	if _, err := (registryRunner{}).Execute(t.Context(), workers.RunnerExecutionRequest{}); err == nil {
		t.Fatal("nil registryRunner.Execute() error = nil, want missing registry error")
	}
}
