package workstationexecution

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

func TestResolveExecutionCatalogAcceptsProviderQualifiedModel(t *testing.T) {
	definition := &FactoryConfig{Workers: []FactoryWorkerConfig{{
		Name: "subagent", Type: "AGENT_WORKER", ModelProvider: "opencode",
		Model: "opencode/muse-spark-1.3-contributor-free",
	}}}
	resolved, err := ResolveExecutionCatalog(context.Background(), ResolveExecutionCatalogRequest{
		EffectiveDefinition: definition,
	})
	if err != nil {
		t.Fatalf("resolve qualified OpenCode model: %v", err)
	}
	if got := resolved.Workers["subagent"].Model; got != "opencode/muse-spark-1.3-contributor-free" {
		t.Fatalf("resolved model = %q", got)
	}
}

func TestResolveExecutionCatalogRejectsMalformedQualifiedModel(t *testing.T) {
	definition := &FactoryConfig{Workers: []FactoryWorkerConfig{{
		Name: "subagent", Type: "AGENT_WORKER", ModelProvider: "opencode",
		Model: "opencode//muse-spark-1.3-contributor-free",
	}}}
	if _, err := ResolveExecutionCatalog(context.Background(), ResolveExecutionCatalogRequest{
		EffectiveDefinition: definition,
	}); err == nil {
		t.Fatal("malformed qualified model unexpectedly resolved")
	}
}

func TestPolicy_WorkstationExecution(t *testing.T) {
	t.Parallel()

	execution := NewService()
	cfg := &factorydefinitions.FactoryWorkstationConfig{
		Limits: factorydefinitions.WorkstationLimits{MaxExecutionTime: "30s"},
	}
	timeout, err := execution.ExecutionTimeout(cfg)
	if err != nil {
		t.Fatalf("ExecutionTimeout: %v", err)
	}
	if timeout != 30*time.Second {
		t.Fatalf("ExecutionTimeout = %v, want 30s", timeout)
	}

	_, err = execution.ExecutionTimeout(&factorydefinitions.FactoryWorkstationConfig{
		Limits: factorydefinitions.WorkstationLimits{MaxExecutionTime: "not-a-duration"},
	})
	if err == nil {
		t.Fatal("ExecutionTimeout error = nil, want invalid duration parse failure")
	}
}

func TestResolveExecutionCatalog_DetachesInterpolatedPolicy(t *testing.T) {
	t.Parallel()
	resolver := ResolveExecutionCatalog
	request := detachedExecutionCatalogRequest()

	first := mustResolveExecutionCatalog(t, resolver, request)
	assertDetachedExecutionCatalog(t, first)
	mutateDetachedExecutionCatalog(first)

	second := mustResolveExecutionCatalog(t, resolver, request)
	assertDetachedExecutionCatalog(t, second)
	if reflect.DeepEqual(first, second) {
		t.Fatal("mutating the first detached result unexpectedly left it equal to a fresh result")
	}
}

func TestResolveExecutionCatalog_AppliesDetachedWorkerDefaults(t *testing.T) {
	t.Parallel()
	resolver := ResolveExecutionCatalog
	request := detachedExecutionCatalogRequest()
	worker := &request.EffectiveDefinition.Workers[0]
	worker.RuntimeDefaultModelProvider = "provider-a"
	worker.RuntimeDefaultModel = "model-a"
	delete(request.Invocation.Arguments.Arguments, "provider")
	delete(request.Invocation.Arguments.Arguments, "model")

	result := mustResolveExecutionCatalog(t, resolver, request)
	resolved := result.Workers["worker"]
	if resolved.ModelProvider != "provider-a" || resolved.Model != "model-a" {
		t.Fatalf("resolved worker defaults = %#v, want provider-a/model-a", resolved)
	}
}

func detachedExecutionCatalogRequest() factorydefinitions.ResolveExecutionCatalogRequest {
	return factorydefinitions.ResolveExecutionCatalogRequest{
		EffectiveDefinition: &factorydefinitions.FactoryConfig{
			Name:   "detached-factory",
			Runner: "codex",
			Version: &factorydefinitions.FactoryVersion{
				Logical:  7,
				Physical: time.Date(2026, 8, 12, 10, 11, 12, 0, time.UTC),
			},
			Workers: []factorydefinitions.FactoryWorkerConfig{{
				ID:              "worker-id",
				Name:            "worker",
				Type:            factorydefinitions.WorkerTypeModel,
				Provider:        "${provider}",
				Model:           "${model}",
				ModelProvider:   "${provider}",
				ReasoningEffort: "${effort}",
				Args:            []string{"--prompt=${prompt}"},
				Timeout:         "${timeout}",
				Body:            "worker body ${prompt}",
				SkipPermissions: true,
				AgentTools:      &factorydefinitions.AgentToolsConfig{Policy: "READ_ONLY"},
				Operations: []factorydefinitions.ModelOperation{{
					Name: "answer",
					Inputs: []factorydefinitions.ModelOperationSlot{{
						Name: "prompt", ContentTypes: []string{"TEXT"}, Required: true,
					}},
				}},
			}},
			Workstations: []factorydefinitions.FactoryWorkstationConfig{{
				ID:               "workstation-id",
				Name:             "run",
				Type:             factorydefinitions.WorkstationTypeModel,
				WorkerTypeName:   "worker",
				Runner:           "${runner}",
				PromptTemplate:   "run ${prompt}",
				Body:             "workstation body ${prompt}",
				WorkingDirectory: "${directory}",
				Worktree:         "${worktree}",
				Env:              map[string]string{"GREETING": "hello ${prompt}"},
				Timeout:          "${timeout}",
				Limits: factorydefinitions.WorkstationLimits{
					MaxExecutionTime: "${timeout}",
				},
				WorkPropagation: &factorydefinitions.WorkPropagationConfig{
					Mode: factorydefinitions.WorkPropagationModePreserveInput,
				},
				OutcomeFormat: factorydefinitions.DecisionEnvelopeOutcomeFormat,
				OperationBindings: []factorydefinitions.ModelOperationBinding{{
					Slot:     "prompt",
					Selector: &factorydefinitions.ModelOperationBindingSelector{Label: "${label}"},
					Config: []work.WorkContentPart{{
						Type: work.WorkContentPartTypeText, Text: "config ${prompt}",
					}},
				}},
				Inputs: []factorydefinitions.IOConfig{{
					WorkTypeName: "work", StateName: "ready",
					Guard: &factorydefinitions.InputGuardConfig{MatchInput: "${label}"},
				}},
				ClassificationRoutes: []factorydefinitions.ClassificationRouteConfig{{
					Label:   "accepted",
					Outputs: []factorydefinitions.IOConfig{{WorkTypeName: "work", StateName: "done"}},
				}},
			}},
		},
		Invocation: factorydefinitions.InvocationDefinitionContext{
			Arguments: &work.InvocationArguments{Arguments: map[string]work.InvocationArgument{
				"provider":  {Values: []string{"provider-a"}},
				"model":     {Values: []string{"model-a"}},
				"effort":    {Values: []string{"high"}},
				"prompt":    {Values: []string{"hello"}},
				"timeout":   {Values: []string{"2s"}},
				"runner":    {Values: []string{"codex"}},
				"directory": {Values: []string{"workspace"}},
				"worktree":  {Values: []string{"tree"}},
				"label":     {Values: []string{"input-label"}},
			}},
		},
		References: factorydefinitions.ExecutionCatalogReferenceCatalog{
			Runners:   map[string]struct{}{"codex": {}},
			Providers: map[string]struct{}{"provider-a": {}},
			Models:    map[string]struct{}{"model-a": {}},
		},
	}
}

func mustResolveExecutionCatalog(
	t *testing.T,
	resolver func(context.Context, factorydefinitions.ResolveExecutionCatalogRequest) (factorydefinitions.ResolveExecutionCatalogResult, error),
	request factorydefinitions.ResolveExecutionCatalogRequest,
) factorydefinitions.ResolveExecutionCatalogResult {
	t.Helper()
	result, err := resolver(context.Background(), request)
	if err != nil {
		t.Fatalf("ResolveExecutionCatalog: %v", err)
	}
	return result
}

func assertDetachedExecutionCatalog(
	t *testing.T,
	result factorydefinitions.ResolveExecutionCatalogResult,
) {
	t.Helper()
	if result.DefinitionVersion != "7@2026-08-12T10:11:12Z" ||
		result.Workers["worker"].Model != "model-a" ||
		result.Workstations["run"].PromptTemplate != "run hello" ||
		result.Workstations["run"].Limits.MaxExecutionTime != "2s" ||
		!result.Workstations["run"].DecisionEnvelope ||
		result.Workstations["run"].WorkPropagation != factorydefinitions.WorkPropagationModePreserveInput {
		t.Fatalf("resolved catalog has unexpected policy: %#v", result)
	}
	if result.Workers["worker"].Args[0] != "--prompt=hello" ||
		result.Workers["worker"].Operations[0].Inputs[0].ContentTypes[0] != "TEXT" ||
		result.Workstations["run"].Environment["GREETING"] != "hello hello" ||
		result.Workstations["run"].OperationBindings[0].Config[0].Text != "config hello" ||
		result.Workstations["run"].Inputs[0].Guard.MatchInput != "input-label" ||
		len(result.Diagnostics) != 0 {
		t.Fatalf("resolved catalog lost detached values: %#v", result)
	}
}

func mutateDetachedExecutionCatalog(result factorydefinitions.ResolveExecutionCatalogResult) {
	result.Workers["worker"].Args[0] = "mutated"
	result.Workers["worker"].Operations[0].Inputs[0].ContentTypes[0] = "MUTATED"
	result.Workstations["run"].Environment["GREETING"] = "mutated"
	result.Workstations["run"].OperationBindings[0].Config[0].Text = "mutated"
	result.Workstations["run"].Inputs[0].Guard.MatchInput = "mutated"
	result.Diagnostics = append(result.Diagnostics, factorydefinitions.ExecutionCatalogDiagnostic{
		Code: factorydefinitions.ExecutionCatalogDiagnosticInvalidDefinition,
	})
}

func TestResolveExecutionCatalog_ReportsTypedDetachedReferenceDiagnostics(t *testing.T) {
	t.Parallel()
	resolver := ResolveExecutionCatalog

	result, err := resolver(context.Background(), factorydefinitions.ResolveExecutionCatalogRequest{
		EffectiveDefinition: &factorydefinitions.FactoryConfig{
			Runner: "missing-runner",
			Workers: []factorydefinitions.FactoryWorkerConfig{{
				Name: "worker", Type: factorydefinitions.WorkerTypeModel,
				Provider: "missing-provider", Model: "missing-model",
			}},
			Workstations: []factorydefinitions.FactoryWorkstationConfig{{
				Name: "run", Type: factorydefinitions.WorkstationTypeModel,
				WorkerTypeName: "missing-worker", Runner: "missing-runner",
			}},
		},
		References: factorydefinitions.ExecutionCatalogReferenceCatalog{
			Runners:   map[string]struct{}{"codex": {}},
			Providers: map[string]struct{}{"provider-a": {}},
			Models:    map[string]struct{}{"model-a": {}},
		},
	})
	if err == nil {
		t.Fatal("ResolveExecutionCatalog error = nil, want typed diagnostics")
	}
	var catalogErr *factorydefinitions.ExecutionCatalogError
	if !errors.As(err, &catalogErr) {
		t.Fatalf("error = %T %v, want *ExecutionCatalogError", err, err)
	}
	if len(result.Diagnostics) == 0 || len(result.Diagnostics) != len(catalogErr.Diagnostics) {
		t.Fatalf("result diagnostics = %#v, error diagnostics = %#v", result.Diagnostics, catalogErr.Diagnostics)
	}
	wanted := map[factorydefinitions.ExecutionCatalogDiagnosticCode]bool{
		factorydefinitions.ExecutionCatalogDiagnosticUnknownRunner:   false,
		factorydefinitions.ExecutionCatalogDiagnosticUnknownProvider: false,
		factorydefinitions.ExecutionCatalogDiagnosticUnknownModel:    false,
		factorydefinitions.ExecutionCatalogDiagnosticUnknownWorker:   false,
	}
	for _, diagnostic := range result.Diagnostics {
		if _, ok := wanted[diagnostic.Code]; ok {
			wanted[diagnostic.Code] = true
		}
		if diagnostic.Message == "" || strings.Contains(diagnostic.Message, "missing-provider") {
			// The reference is available as a safe identity field; messages remain
			// stable diagnostic text rather than copied provider payloads.
			if diagnostic.Message == "" {
				t.Fatalf("diagnostic has empty message: %#v", diagnostic)
			}
		}
	}
	for code, found := range wanted {
		if !found {
			t.Fatalf("diagnostic code %q missing from %#v", code, result.Diagnostics)
		}
	}
}
