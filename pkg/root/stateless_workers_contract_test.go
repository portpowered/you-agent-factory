package root

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// TestBuildStatelessWorkersExecutesDetachedAttemptThroughRoot proves the
// standalone Workers root composes and executes without opening Factory Runtime
// or a Factory Session. This is a root contract, not an application workflow.
func TestBuildStatelessWorkersExecutesDetachedAttemptThroughRoot(t *testing.T) {
	t.Parallel()

	service, err := BuildStatelessWorkers(t.Context(), serviceedges.Edges{
		ScriptCommandRunner: statelessCommandRunner{},
	})
	if err != nil {
		t.Fatalf("BuildStatelessWorkers() error = %v", err)
	}

	result, err := service.Execute(context.Background(), workers.ExecuteRequest{
		Correlation: workers.ExecutionCorrelation{
			FactorySessionID: "session-contract",
			RuntimeID:        "runtime-contract",
			GenerationID:     "generation-contract",
			DispatchID:       "contract-stateless-dispatch",
			AttemptID:        "contract-stateless-attempt",
		},
		Target: workers.ExecutionTarget{
			WorkerName: "script-worker",
			RunnerID:   "script",
			Command:    "contract-stateless-script",
		},
	})
	if err != nil {
		t.Fatalf("stateless Execute() error = %v", err)
	}
	if result.Outcome != workers.ExecutionOutcomeAccepted ||
		len(result.Output.Primary) != 1 || result.Output.Primary[0].Text != "stateless-contract-output" {
		t.Fatalf("stateless result = %#v, want accepted contract output", result)
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

	if service, err := BuildStatelessWorkers(t.Context(), serviceedges.Edges{
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
		Stdout: statelessCodexSuccessStdout("stateless-provider-output"),
	})
	service, err := BuildStatelessWorkers(t.Context(), serviceedges.Edges{
		ProviderCommandRunner: runner,
	})
	if err != nil {
		t.Fatalf("BuildStatelessWorkers() error = %v", err)
	}

	result, err := service.Execute(context.Background(), workers.ExecuteRequest{
		Correlation: workers.ExecutionCorrelation{
			FactorySessionID: "session-contract-provider",
			RuntimeID:        "runtime-contract-provider",
			GenerationID:     "generation-contract-provider",
			DispatchID:       "contract-provider-dispatch",
			AttemptID:        "contract-provider-attempt",
		},
		Target: workers.ExecutionTarget{
			WorkerName: "agent-worker",
			RunnerID:   "agent",
			Provider:   workers.ProviderReference{ID: "codex"},
			Prompt:     workers.PromptPolicy{UserMessage: "answer the contract provider test"},
		},
	})
	if err != nil {
		t.Fatalf("stateless provider Execute() error = %v", err)
	}
	if result.Outcome != workers.ExecutionOutcomeAccepted ||
		len(result.Output.Primary) != 1 || result.Output.Primary[0].Text != "stateless-provider-output" {
		t.Fatalf("stateless provider result = %#v, want accepted contract output", result)
	}
	if runner.CallCount() != 1 {
		t.Fatalf("provider command calls = %d, want one", runner.CallCount())
	}
}

func TestBuildStatelessWorkersPreservesDetachedAgentContractThroughRoot(t *testing.T) {
	t.Parallel()

	provider := testutil.NewNativeMockProvider(providers.ExecuteResult{
		Content: `{"decision":"ACCEPTED","feedback":"ready","output":"ship"}`,
		SessionRef: &providers.SessionRef{
			Provider: providers.IDCodex,
			Kind:     providers.SessionIDKind,
			ID:       "contract-agent-session",
		},
	})
	service, err := BuildStatelessWorkers(t.Context(), serviceedges.Edges{
		ProviderOverride: provider,
	})
	if err != nil {
		t.Fatalf("BuildStatelessWorkers() error = %v", err)
	}

	request := workers.ExecuteRequest{
		Correlation: workers.ExecutionCorrelation{
			FactorySessionID: "session-contract-agent",
			RuntimeID:        "runtime-contract-agent",
			GenerationID:     "generation-contract-agent",
			DispatchID:       "contract-agent-dispatch",
			AttemptID:        "contract-agent-attempt",
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
		result.Continuation.ProviderSessionID != "contract-agent-session" {
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

// TestBuildStatelessWorkersExposesDetachedRuntimeAndPoolContracts proves the
// public root keeps runtime assembly and workstation-pool lifecycle separate
// from one detached Execute attempt.
func TestBuildStatelessWorkersExposesDetachedRuntimeAndPoolContracts(t *testing.T) {
	service, err := BuildStatelessWorkers(t.Context(), serviceedges.Edges{})
	if err != nil {
		t.Fatalf("BuildStatelessWorkers() error = %v", err)
	}

	built, err := service.BuildRuntime(context.Background(), workers.RuntimeBuildRequest{
		RunnerID: "agent",
		Roles: []workers.RuntimeBuildRoleRequest{{
			Name: "contract-agent",
			Kind: workers.RuntimeBuildRoleKindWorker,
		}},
	})
	if err != nil {
		t.Fatalf("BuildRuntime() error = %v", err)
	}
	if built.RunnerSelection.RunnerID != "agent" || len(built.Bindings) != 1 ||
		built.Bindings[0].RoleName != "contract-agent" {
		t.Fatalf("BuildRuntime() = %#v, want one agent binding", built)
	}

	started, err := service.StartWorkstationPool(context.Background(), workers.WorkstationPoolStartRequest{
		Bindings: []workers.AssembledRuntimeBinding{{
			RoleName:        "contract-route",
			RoleKind:        workers.RuntimeBuildRoleKindWorkstation,
			RunnerSelection: workers.ResolvedRunnerSelection{RunnerID: "agent", Source: workers.RunnerSelectionSourceFactory},
			Executor:        rootContractExecutor{},
		}},
	})
	if err != nil || started.Outcome != workers.WorkstationPoolLifecycleOutcomeStarted {
		t.Fatalf("StartWorkstationPool() = %#v, error %v; want STARTED", started, err)
	}
	route, err := service.WorkstationRoute(context.Background(), workers.WorkstationRouteRequest{WorkstationName: "contract-route"})
	if err != nil || !route.Available {
		t.Fatalf("WorkstationRoute() = %#v, error %v; want available route", route, err)
	}
	stopped, err := service.StopWorkstationPool(context.Background())
	if err != nil || stopped.Outcome != workers.WorkstationPoolLifecycleOutcomeStopped {
		t.Fatalf("StopWorkstationPool() = %#v, error %v; want STOPPED", stopped, err)
	}
	if _, err := service.WorkstationRoute(context.Background(), workers.WorkstationRouteRequest{WorkstationName: "contract-route"}); !errors.Is(err, workers.ErrWorkstationPoolStopped) {
		t.Fatalf("WorkstationRoute() after stop error = %v, want stopped error", err)
	}
}

type statelessCommandRunner struct{}

func (statelessCommandRunner) Run(context.Context, platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	return platformprocess.CommandResult{Stdout: []byte("stateless-contract-output")}, nil
}

type rootContractExecutor struct{}

func (rootContractExecutor) Execute(context.Context, workers.WorkstationExecutionRequest) (workers.WorkResult, error) {
	return workers.WorkResult{Outcome: workers.OutcomeAccepted}, nil
}

func statelessCodexSuccessStdout(result string) []byte {
	item, err := json.Marshal(map[string]any{
		"type": "item.completed",
		"item": map[string]any{
			"id":   "root-contract-message",
			"type": "agent_message",
			"text": result,
		},
	})
	if err != nil {
		panic(err)
	}
	return append([]byte(`{"type":"turn.started"}`+"\n"), append(item, []byte("\n{\"type\":\"turn.completed\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}\n")...)...)
}
