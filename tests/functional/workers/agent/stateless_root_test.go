package agent_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestBuildStatelessWorkersExecutesDetachedAttemptThroughRoot proves the
// standalone Workers root composes and executes without opening Factory Runtime
// or a Factory Session. The direct Execute boundary has no Process.Execute
// transport representation, so this is intentionally a public root test.
func TestBuildStatelessWorkersExecutesDetachedAttemptThroughRoot(t *testing.T) {
	t.Parallel()

	service, err := root.BuildStatelessWorkers(t.Context(), serviceedges.Edges{
		ScriptCommandRunner: functionalStatelessCommandRunner{},
	})
	if err != nil {
		t.Fatalf("root.BuildStatelessWorkers() error = %v", err)
	}

	result, err := service.Execute(context.Background(), workers.ExecuteRequest{
		Correlation: workers.ExecutionCorrelation{
			FactorySessionID: "session-functional",
			RuntimeID:        "runtime-functional",
			GenerationID:     "generation-functional",
			DispatchID:       "functional-stateless-dispatch",
			AttemptID:        "functional-stateless-attempt",
		},
		Target: workers.ExecutionTarget{
			WorkerName: "script-worker",
			RunnerID:   "script",
			Command:    "functional-stateless-script",
		},
	})
	if err != nil {
		t.Fatalf("stateless Execute() error = %v", err)
	}
	if result.Outcome != workers.ExecutionOutcomeAccepted ||
		len(result.Output.Primary) != 1 || result.Output.Primary[0].Text != "functional-stateless-output" {
		t.Fatalf("stateless result = %#v, want accepted functional output", result)
	}

	promptTemplates, ok := service.(workers.PromptTemplates)
	if !ok {
		t.Fatal("stateless Workers root does not expose prompt template contracts")
	}
	contract := promptTemplates.BuildPromptTemplateContract(1, []string{"factory/docs/guide.md"})
	if contract.InputCount != 1 || len(contract.AvailableVariables) == 0 {
		t.Fatalf("prompt contract = %#v, want selected input variables", contract)
	}
	validation := promptTemplates.ValidatePromptTemplate("{{ .Context.Project }}", 1, nil)
	if !validation.Valid || len(validation.Diagnostics) != 0 {
		t.Fatalf("prompt validation = %#v, want valid detached template", validation)
	}
	fieldResolver, ok := service.(interface {
		ResolveTemplateFields(
			string,
			map[string]string,
			[]workers.Token,
			*workers.Context,
			string,
		) (*workers.ResolvedTemplateFields, error)
	})
	if !ok {
		t.Fatal("stateless Workers root does not expose template field resolution")
	}
	fields, err := fieldResolver.ResolveTemplateFields(
		"{{.Context.WorkDir}}",
		map[string]string{"TOKEN": "{{.Context.Project}}"},
		nil,
		&workers.Context{WorkDirectory: "/workspace", ProjectID: "project-1"},
		"",
	)
	if err != nil || fields.WorkingDirectory != "/workspace" || fields.Env["TOKEN"] != "project-1" {
		t.Fatalf("resolved fields = %#v, error = %v, want detached context values", fields, err)
	}
	if recorder, ok := service.(interface{ RuntimeOwnsModelEventRecording() bool }); !ok || !recorder.RuntimeOwnsModelEventRecording() {
		t.Fatal("stateless Workers root does not own model event recording")
	}

	if service, err := root.BuildStatelessWorkers(t.Context(), serviceedges.Edges{
		ProviderRegistrations: []providerswire.Registration{{
			Manifest:    providerswire.Manifest{ID: "codex"},
			Integration: providerswire.ProgressingExternalIntegration("codex", "unused"),
		}},
	}); service != nil || err == nil || !strings.Contains(err.Error(), "provider registry validation failed") {
		t.Fatalf("invalid stateless provider registration = (%#v, %v), want provider registry validation failure", service, err)
	}
}

// TestBuildStatelessWorkersExecutesProviderAttemptThroughRoot proves the
// standalone Workers root reaches the Providers-owned command boundary for a
// detached agent attempt without opening Factory Runtime or a Factory Session.
func TestBuildStatelessWorkersExecutesProviderAttemptThroughRoot(t *testing.T) {
	t.Parallel()

	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stdout: support.CodexSuccessStdout("functional-provider-output"),
	})
	service, err := root.BuildStatelessWorkers(t.Context(), serviceedges.Edges{
		ProviderCommandRunner: runner,
	})
	if err != nil {
		t.Fatalf("root.BuildStatelessWorkers() error = %v", err)
	}

	result, err := service.Execute(context.Background(), workers.ExecuteRequest{
		Correlation: workers.ExecutionCorrelation{
			FactorySessionID: "session-functional-provider",
			RuntimeID:        "runtime-functional-provider",
			GenerationID:     "generation-functional-provider",
			DispatchID:       "functional-provider-dispatch",
			AttemptID:        "functional-provider-attempt",
		},
		Target: workers.ExecutionTarget{
			WorkerName: "agent-worker",
			RunnerID:   "agent",
			Provider:   workers.ProviderReference{ID: "codex"},
			Prompt:     workers.PromptPolicy{UserMessage: "answer the functional provider test"},
		},
	})
	if err != nil {
		t.Fatalf("stateless provider Execute() error = %v", err)
	}
	if result.Outcome != workers.ExecutionOutcomeAccepted ||
		len(result.Output.Primary) != 1 || result.Output.Primary[0].Text != "functional-provider-output" {
		t.Fatalf("stateless provider result = %#v, want accepted functional provider output", result)
	}
	if runner.CallCount() != 1 {
		t.Fatalf("provider command calls = %d, want one", runner.CallCount())
	}
}

// TestBuildProcessExecutesProviderAttemptThroughRuntimeRoot proves the
// application process still composes the canonical Workers runtime around the
// Providers root and reaches the injected provider command edge.
func TestBuildProcessExecutesProviderAttemptThroughRuntimeRoot(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"runtime provider root"}`))
	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stdout: support.CodexSuccessStdout("functional-runtime-provider-output COMPLETE"),
	})

	_, listed := support.RunFactoryToCompletionWithEdgesAndWork(
		t,
		dir,
		serviceedges.Edges{ProviderCommandRunner: runner},
		20*time.Second,
	)
	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("completed work = %d, want 1; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 0 {
		t.Fatalf("failed work = %d, want 0; listed=%#v", got, listed)
	}
	if runner.CallCount() != 1 {
		t.Fatalf("provider command calls = %d, want one", runner.CallCount())
	}
}

// TestBuildStatelessWorkersExposesDetachedRuntimeAndPoolContracts proves the
// public root keeps runtime assembly and workstation-pool lifecycle separate
// from one detached Execute attempt.
func TestBuildStatelessWorkersExposesDetachedRuntimeAndPoolContracts(t *testing.T) {
	service, err := root.BuildStatelessWorkers(t.Context(), serviceedges.Edges{})
	if err != nil {
		t.Fatalf("root.BuildStatelessWorkers() error = %v", err)
	}

	built, err := service.BuildRuntime(context.Background(), workers.RuntimeBuildRequest{
		RunnerID: "agent",
		Roles: []workers.RuntimeBuildRoleRequest{{
			Name: "functional-agent",
			Kind: workers.RuntimeBuildRoleKindWorker,
		}},
	})
	if err != nil {
		t.Fatalf("BuildRuntime() error = %v", err)
	}
	if built.RunnerSelection.RunnerID != "agent" || len(built.Bindings) != 1 ||
		built.Bindings[0].RoleName != "functional-agent" {
		t.Fatalf("BuildRuntime() = %#v, want one agent binding", built)
	}

	started, err := service.StartWorkstationPool(context.Background(), workers.WorkstationPoolStartRequest{
		Bindings: []workers.AssembledRuntimeBinding{{
			RoleName:        "functional-route",
			RoleKind:        workers.RuntimeBuildRoleKindWorkstation,
			RunnerSelection: workers.ResolvedRunnerSelection{RunnerID: "agent", Source: workers.RunnerSelectionSourceFactory},
			Executor:        functionalRootExecutor{},
		}},
	})
	if err != nil || started.Outcome != workers.WorkstationPoolLifecycleOutcomeStarted {
		t.Fatalf("StartWorkstationPool() = %#v, error %v; want STARTED", started, err)
	}
	route, err := service.WorkstationRoute(context.Background(), workers.WorkstationRouteRequest{WorkstationName: "functional-route"})
	if err != nil || !route.Available {
		t.Fatalf("WorkstationRoute() = %#v, error %v; want available route", route, err)
	}
	stopped, err := service.StopWorkstationPool(context.Background())
	if err != nil || stopped.Outcome != workers.WorkstationPoolLifecycleOutcomeStopped {
		t.Fatalf("StopWorkstationPool() = %#v, error %v; want STOPPED", stopped, err)
	}
	if _, err := service.WorkstationRoute(context.Background(), workers.WorkstationRouteRequest{WorkstationName: "functional-route"}); !errors.Is(err, workers.ErrWorkstationPoolStopped) {
		t.Fatalf("WorkstationRoute() after stop error = %v, want stopped error", err)
	}
}

type functionalStatelessCommandRunner struct{}

type functionalRootExecutor struct{}

func (functionalRootExecutor) Execute(context.Context, workers.WorkstationExecutionRequest) (workers.WorkResult, error) {
	return workers.WorkResult{Outcome: workers.OutcomeAccepted}, nil
}

func (functionalStatelessCommandRunner) Run(
	context.Context,
	platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	return platformprocess.CommandResult{Stdout: []byte("functional-stateless-output")}, nil
}
