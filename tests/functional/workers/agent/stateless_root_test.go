package agent_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/providers"
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

func TestBuildStatelessWorkersPreservesDetachedAgentContractThroughRoot(t *testing.T) {
	t.Parallel()

	provider := testutil.NewNativeMockProvider(
		providers.ExecuteResult{
			Content: `{"decision":"ACCEPTED","feedback":"ready","output":"ship"}`,
			SessionRef: &providers.SessionRef{
				Provider: providers.IDCodex,
				Kind:     providers.SessionIDKind,
				ID:       "direct-root-agent-session",
			},
		},
	)
	service, err := root.BuildStatelessWorkers(t.Context(), serviceedges.Edges{
		ProviderOverride: provider,
	})
	if err != nil {
		t.Fatalf("root.BuildStatelessWorkers() error = %v", err)
	}

	request := workers.ExecuteRequest{
		Correlation: workers.ExecutionCorrelation{
			FactorySessionID: "session-root-agent",
			RuntimeID:        "runtime-root-agent",
			GenerationID:     "generation-root-agent",
			DispatchID:       "dispatch-root-agent",
			AttemptID:        "attempt-root-agent",
		},
		Target: workers.ExecutionTarget{
			WorkerName:      "agent-worker",
			WorkstationName: "execute-goal",
			RunnerID:        "agent",
			Provider:        workers.ProviderReference{ID: string(providers.IDCodex)},
			Prompt:          workers.PromptPolicy{UserMessage: "complete this goal"},
			Tools: workers.ToolPolicy{
				AgentLoop:       true,
				AgentToolPolicy: "DISABLED",
			},
			Output: workers.OutputPolicy{
				DecisionEnvelope:            true,
				GoalRoutingDecisionEnvelope: true,
			},
		},
	}
	result, err := service.Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("detached agent Execute() error = %v", err)
	}
	if result.Outcome != workers.ExecutionOutcomeAccepted {
		t.Fatalf("outcome = %q, failure = %#v, want ACCEPTED", result.Outcome, result.Failure)
	}
	if len(result.Output.Primary) != 1 || result.Output.Primary[0].Text != "ship" ||
		result.Output.Feedback != "ready" || result.Output.Classification != "accepted" {
		t.Fatalf("output = %#v, want normalized goal decision-envelope output", result.Output)
	}
	if result.Continuation == nil || result.Continuation.Provider != string(providers.IDCodex) ||
		result.Continuation.ProviderSessionID != "direct-root-agent-session" {
		t.Fatalf("continuation = %#v, want exact detached provider identity", result.Continuation)
	}
	if result.Diagnostics == nil ||
		result.Diagnostics.Metadata[workers.AgentRunMetadataExecutionBehavior] != workers.AgentRunExecutionBehavior ||
		result.Diagnostics.Metadata[workers.AgentRunMetadataToolPolicy] != "DISABLED" {
		t.Fatalf("diagnostics = %#v, want safe detached agent-run metadata", result.Diagnostics)
	}
	if provider.CallCount() != 1 {
		t.Fatalf("provider calls = %d, want one detached agent attempt", provider.CallCount())
	}
	call := provider.Calls()[0]
	if call.Correlation.DispatchID != request.Correlation.DispatchID ||
		call.Correlation.AttemptID != request.Correlation.AttemptID {
		t.Fatalf("provider correlation = %#v, want detached request correlation", call.Correlation)
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

// TestBuildStatelessWorkersSealsRuntimeAndPoolCapabilities proves supported
// production and mock roots execute through the same canonical request-scoped
// boundary and do not expose retired operational capabilities.
func TestBuildStatelessWorkersSealsRuntimeAndPoolCapabilities(t *testing.T) {
	production, err := root.BuildStatelessWorkers(t.Context(), serviceedges.Edges{
		ScriptCommandRunner: functionalStatelessCommandRunner{},
	})
	if err != nil {
		t.Fatalf("root.BuildStatelessWorkers() error = %v", err)
	}
	productionResult, err := production.Execute(context.Background(), workers.ExecuteRequest{
		Correlation: workers.ExecutionCorrelation{
			FactorySessionID: "sealed-production-session",
			RuntimeID:        "sealed-production-runtime",
			GenerationID:     "sealed-production-generation",
			DispatchID:       "sealed-production-dispatch",
			AttemptID:        "sealed-production-attempt",
		},
		Target: workers.ExecutionTarget{
			WorkerName: "sealed-script-worker",
			RunnerID:   "script",
			Command:    "sealed-script-command",
		},
	})
	if err != nil || productionResult.Outcome != workers.ExecutionOutcomeAccepted {
		t.Fatalf("production Execute() = %#v, error %v; want accepted canonical outcome", productionResult, err)
	}

	mock, err := root.BuildMockStatelessWorkers(t.Context(), serviceedges.Edges{}, &workers.MockWorkersConfig{
		MockWorkers: []workers.MockWorkerConfig{{
			WorkerName: "sealed-mock-worker",
			RunType:    workers.MockWorkerRunTypeAccept,
		}},
	})
	if err != nil {
		t.Fatalf("root.BuildMockStatelessWorkers() error = %v", err)
	}
	mockResult, err := mock.Execute(context.Background(), workers.ExecuteRequest{
		Correlation: workers.ExecutionCorrelation{
			FactorySessionID: "sealed-mock-session",
			RuntimeID:        "sealed-mock-runtime",
			GenerationID:     "sealed-mock-generation",
			DispatchID:       "sealed-mock-dispatch",
			AttemptID:        "sealed-mock-attempt",
		},
		Target: workers.ExecutionTarget{
			WorkerName: "sealed-mock-worker",
			RunnerID:   "mock",
		},
	})
	if err != nil || mockResult.Outcome != workers.ExecutionOutcomeAccepted {
		t.Fatalf("mock Execute() = %#v, error %v; want accepted canonical outcome", mockResult, err)
	}

	assertSealedWorkersCapabilities(t, production)
	assertSealedWorkersCapabilities(t, mock)
}

type functionalStatelessCommandRunner struct{}

// retiredWorkersDispatchCapability is deliberately local to this external
// package. If the old dispatch/pool operational surface is put back on a
// supported root, this assertion fails without inspecting source or routes.
type retiredWorkersDispatchCapability interface {
	DispatchWorkstation(context.Context, workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error)
	DispatchWorkstationWithAdmission(context.Context, workers.WorkstationDispatchRequest, workers.WorkstationDispatchAdmissionFunc) (workers.WorkstationDispatchResult, error)
	CancelWorkstationDispatch(context.Context, workers.WorkstationDispatchCancelRequest) (workers.WorkstationDispatchCancelResult, error)
}

// retiredWorkersAssemblyCapability names the alternate operational method
// family with detached value contracts so the test remains external to the
// Workers implementation after those contracts are removed.
type retiredWorkersAssemblyCapability interface {
	BuildRuntime(context.Context, workers.ExecuteRequest) (workers.ExecuteResult, error)
	StartWorkstationPool(context.Context, workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error)
	StopWorkstationPool(context.Context) (workers.WorkstationDispatchResult, error)
	WorkstationRoute(context.Context, workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error)
}

func assertSealedWorkersCapabilities(t *testing.T, service workers.Service) {
	t.Helper()
	if _, ok := service.(retiredWorkersDispatchCapability); ok {
		t.Fatal("Workers root exposes retired dispatch/pool operational capability")
	}
	if _, ok := service.(retiredWorkersAssemblyCapability); ok {
		t.Fatal("Workers root exposes retired runtime assembly capability")
	}
}

func (functionalStatelessCommandRunner) Run(
	context.Context,
	platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	return platformprocess.CommandResult{Stdout: []byte("functional-stateless-output")}, nil
}
