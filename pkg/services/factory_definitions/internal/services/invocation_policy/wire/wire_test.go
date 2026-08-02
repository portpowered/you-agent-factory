package wire_test

import (
	"context"
	"errors"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	invocationpolicywire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/invocation_policy/wire"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

func TestNewService_ResolvesDetachedInvocationFacts(t *testing.T) {
	t.Parallel()

	service, err := invocationpolicywire.NewService()
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	config := &factorydefinitions.FactoryConfig{
		Name:    "@you/example",
		Project: "builtin-example",
		WorkTypes: []factorydefinitions.WorkTypeConfig{{
			Name:             "task",
			HandlingBehavior: []string{factorydefinitions.WorkTypeHandlingBehaviorDefault},
		}},
		Workers: []factorydefinitions.FactoryWorkerConfig{{
			Name: "worker",
			Body: "${source}",
		}},
		Workstations: []factorydefinitions.FactoryWorkstationConfig{{
			Name:           factorydefinitions.PackagedGoalExecuteWorkstationName,
			Type:           factorydefinitions.WorkstationTypeModel,
			Timeout:        "${timeout}",
			PromptTemplate: "Use ${source}",
			OutcomeFormat:  factorydefinitions.WorkstationOutcomeFormatDecisionEnvelope,
			ClassificationRoutes: []factorydefinitions.ClassificationRouteConfig{{
				Label: "accepted",
			}},
			WorkPropagation: &factorydefinitions.WorkPropagationConfig{
				Mode: factorydefinitions.WorkPropagationModePreserveInput,
			},
		}},
	}
	arguments := factorydefinitions.InvocationArguments{Arguments: map[string]factorydefinitions.InvocationArgument{
		"source": {
			Values:    []string{"input.txt"},
			ValueMode: work.InvocationParameterValueModeFileContents,
		},
		"timeout": {Values: []string{"30s"}},
	}}
	resolved, err := service.ResolveInvocationDefinition(context.Background(), factorydefinitions.ResolveInvocationDefinitionRequest{
		Definition: factorydefinitions.EffectiveFactorySource{Factory: config},
		Arguments:  arguments,
		ResolvedFileInput: map[string][]byte{
			"input.txt": []byte("detached payload"),
		},
	})
	if err != nil {
		t.Fatalf("ResolveInvocationDefinition: %v", err)
	}
	if resolved.DefaultWorkType != "task" {
		t.Fatalf("DefaultWorkType = %q, want task", resolved.DefaultWorkType)
	}
	if resolved.FactoryKind != factorydefinitions.FactoryBehaviorKindPackaged {
		t.Fatalf("FactoryKind = %q, want packaged", resolved.FactoryKind)
	}
	if resolved.Factory.Workers[0].Body != "detached payload" {
		t.Fatalf("interpolated worker body = %q, want file contents", resolved.Factory.Workers[0].Body)
	}
	if resolved.Factory.Workstations[0].PromptTemplate != "Use detached payload" {
		t.Fatalf("interpolated prompt = %q, want detached payload", resolved.Factory.Workstations[0].PromptTemplate)
	}
	policy, ok := resolved.WorkstationPolicies[factorydefinitions.PackagedGoalExecuteWorkstationName]
	if !ok {
		t.Fatal("resolved workstation policy is missing")
	}
	if policy.ExecutionTimeout != 30*time.Second {
		t.Fatalf("ExecutionTimeout = %v, want 30s", policy.ExecutionTimeout)
	}
	if policy.PropagationMode != factorydefinitions.WorkPropagationModePreserveInput {
		t.Fatalf("PropagationMode = %q, want preserve input", policy.PropagationMode)
	}
	if policy.OutputMode != factorydefinitions.InvocationOutputModeSummary {
		t.Fatalf("OutputMode = %q, want summary", policy.OutputMode)
	}
	if policy.DecisionMode != factorydefinitions.DecisionEnvelopeModeGoalRouting {
		t.Fatalf("DecisionMode = %q, want goal routing", policy.DecisionMode)
	}

	config.Workers[0].Body = "mutated source"
	arguments.Arguments["timeout"] = factorydefinitions.InvocationArgument{Values: []string{"1s"}}
	if resolved.Factory.Workers[0].Body != "detached payload" || resolved.Factory.Workstations[0].Timeout != "" {
		t.Fatalf("resolved facts changed after caller mutation: %#v", resolved)
	}
}

func TestNewService_RejectsInvalidInvocationInputs(t *testing.T) {
	t.Parallel()

	service, err := invocationpolicywire.NewService()
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	base := factorydefinitions.FactoryConfig{
		WorkTypes: []factorydefinitions.WorkTypeConfig{{
			Name:             "task",
			HandlingBehavior: []string{factorydefinitions.WorkTypeHandlingBehaviorDefault},
		}},
	}

	_, err = service.ResolveInvocationDefinition(context.Background(), factorydefinitions.ResolveInvocationDefinitionRequest{})
	if !errors.Is(err, factorydefinitions.ErrInvalidInvocationDefinition) {
		t.Fatalf("missing Factory error = %v, want ErrInvalidInvocationDefinition", err)
	}

	withoutDefault := base
	withoutDefault.WorkTypes = []factorydefinitions.WorkTypeConfig{{Name: "task"}}
	_, err = service.ResolveInvocationDefinition(context.Background(), factorydefinitions.ResolveInvocationDefinitionRequest{
		Definition: factorydefinitions.EffectiveFactorySource{Factory: &withoutDefault},
	})
	if !errors.Is(err, factorydefinitions.ErrInvalidInvocationDefinition) {
		t.Fatalf("missing default Work type error = %v, want ErrInvalidInvocationDefinition", err)
	}

	missingFile := base
	missingFile.Workers = []factorydefinitions.FactoryWorkerConfig{{Body: "${source}"}}
	_, err = service.ResolveInvocationDefinition(context.Background(), factorydefinitions.ResolveInvocationDefinitionRequest{
		Definition: factorydefinitions.EffectiveFactorySource{Factory: &missingFile},
		Arguments: factorydefinitions.InvocationArguments{Arguments: map[string]factorydefinitions.InvocationArgument{
			"source": {Values: []string{"missing.txt"}, ValueMode: work.InvocationParameterValueModeFileContents},
		}},
	})
	if !errors.Is(err, factorydefinitions.ErrInvalidInvocationDefinition) {
		t.Fatalf("missing file error = %v, want ErrInvalidInvocationDefinition", err)
	}

	badTimeout := base
	badTimeout.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
		Name:    "broken",
		Timeout: "not-a-duration",
	}}
	_, err = service.ResolveInvocationDefinition(context.Background(), factorydefinitions.ResolveInvocationDefinitionRequest{
		Definition: factorydefinitions.EffectiveFactorySource{Factory: &badTimeout},
	})
	if !errors.Is(err, factorydefinitions.ErrInvalidInvocationDefinition) {
		t.Fatalf("invalid timeout error = %v, want ErrInvalidInvocationDefinition", err)
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = service.ResolveInvocationDefinition(canceled, factorydefinitions.ResolveInvocationDefinitionRequest{
		Definition: factorydefinitions.EffectiveFactorySource{Factory: &base},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled context error = %v, want context.Canceled", err)
	}
}
