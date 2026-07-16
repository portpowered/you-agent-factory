package ralph

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	invocations "github.com/portpowered/infinite-you/pkg/work/invocation"
)

func TestBuiltInFactoryJSON_LoadsRunnablePlanThenExecuteFactory(t *testing.T) {
	cfg, err := factoryconfig.FactoryConfigFromOpenAPIJSON(BuiltInFactoryJSON)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}
	if cfg.Name != PackagedFactoryName || cfg.Project != PackagedFactoryProject {
		t.Fatalf("factory identity = %q/%q, want %q/%q", cfg.Name, cfg.Project, PackagedFactoryName, PackagedFactoryProject)
	}
	if cfg.InvocationSignature == nil || cfg.InvocationReturn == nil {
		t.Fatal("packaged Ralph invocation contract is incomplete")
	}
	if cfg.InvocationReturn.WorkTypeName != PackagedWorkTypeName || cfg.InvocationReturn.TerminalState != "complete" {
		t.Fatalf("invocation return = %#v, want ralph:complete", cfg.InvocationReturn)
	}

	planner := findWorkstation(t, cfg.Workstations, PackagedPlanWorkstationName)
	executor := findWorkstation(t, cfg.Workstations, PackagedExecuteWorkstationName)
	assertRoute(t, planner.Inputs, "init")
	assertRoute(t, planner.Outputs, "execute")
	assertRoute(t, planner.OnFailure, "failed")
	assertRoute(t, executor.Inputs, "execute")
	assertRoute(t, executor.Outputs, "complete")
	assertRoute(t, executor.OnContinue, "execute")
	assertRoute(t, executor.OnFailure, "failed")
	assertLoopBreaker(t, cfg.Workstations)
	if executor.Kind != interfaces.WorkstationKindRepeater {
		t.Fatalf("executor kind = %q, want %q", executor.Kind, interfaces.WorkstationKindRepeater)
	}
	for _, workstation := range []interfaces.FactoryWorkstationConfig{planner, executor} {
		if workstation.WorkPropagation == nil || workstation.WorkPropagation.Mode != interfaces.WorkPropagationModeOutputAsPayload {
			t.Fatalf("workstation %q propagation = %#v, want OUTPUT_AS_PAYLOAD", workstation.Name, workstation.WorkPropagation)
		}
		if strings.TrimSpace(workstation.Body) == "" {
			t.Fatalf("workstation %q prompt is empty", workstation.Name)
		}
	}
	for _, target := range factoryvalidation.Validate(cfg).Targets {
		if target.Severity == factoryvalidation.SeverityError {
			t.Fatalf("validation target = %#v", target)
		}
	}
}

func TestBuiltInRalphFactory_AppliesValidatedPlanningAndExecutionParameters(t *testing.T) {
	cfg, err := factoryconfig.FactoryConfigFromOpenAPIJSON(BuiltInFactoryJSON)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}

	normalized, err := invocations.NormalizeArguments(invocations.NormalizeArgumentsInput{
		Signature:      cfg.InvocationSignature,
		PositionalArgs: []string{"Ship the login fix"},
		NamedArgs: []invocations.NamedArgumentInput{
			{Key: "planning-detail", Values: []string{"brief"}},
			{Key: "execution-style", Values: []string{"direct"}},
		},
	})
	if err != nil {
		t.Fatalf("NormalizeArguments: %v", err)
	}
	assertNormalizedArgument(t, &normalized, "planningDetail", []string{"brief"})
	assertNormalizedArgument(t, &normalized, "executionStyle", []string{"direct"})

	args := invocations.RuntimeInvocationArguments(cfg.InvocationSignature, &normalized)
	if err := invocations.ValidateInvocationInterpolation(cfg, args, nil); err != nil {
		t.Fatalf("ValidateInvocationInterpolation: %v", err)
	}
	planner, err := invocations.InterpolateWorkstationConfig(findWorkstation(t, cfg.Workstations, PackagedPlanWorkstationName), args, nil)
	if err != nil {
		t.Fatalf("InterpolateWorkstationConfig(planner): %v", err)
	}
	executor, err := invocations.InterpolateWorkstationConfig(findWorkstation(t, cfg.Workstations, PackagedExecuteWorkstationName), args, nil)
	if err != nil {
		t.Fatalf("InterpolateWorkstationConfig(executor): %v", err)
	}
	if !strings.Contains(planner.Body, "Planning detail: brief") {
		t.Fatalf("planner prompt = %q, want configured planning detail", planner.Body)
	}
	if !strings.Contains(executor.Body, "Execution style: direct") {
		t.Fatalf("executor prompt = %q, want configured execution style", executor.Body)
	}

	defaults, err := invocations.NormalizeArguments(invocations.NormalizeArgumentsInput{
		Signature:      cfg.InvocationSignature,
		PositionalArgs: []string{"Ship the next fix"},
	})
	if err != nil {
		t.Fatalf("NormalizeArguments(defaults after configured invocation): %v", err)
	}
	defaultArgs := invocations.RuntimeInvocationArguments(cfg.InvocationSignature, &defaults)
	defaultPlanner, err := invocations.InterpolateWorkstationConfig(findWorkstation(t, cfg.Workstations, PackagedPlanWorkstationName), defaultArgs, nil)
	if err != nil {
		t.Fatalf("InterpolateWorkstationConfig(default planner): %v", err)
	}
	if !strings.Contains(defaultPlanner.Body, "Planning detail: detailed") {
		t.Fatalf("default planner prompt = %q, want unchanged planning default", defaultPlanner.Body)
	}
}

func TestBuiltInRalphFactory_ParameterDefaultsAndInvalidValuesAreHandledBeforeDispatch(t *testing.T) {
	cfg, err := factoryconfig.FactoryConfigFromOpenAPIJSON(BuiltInFactoryJSON)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}

	_, err = invocations.NormalizeArguments(invocations.NormalizeArgumentsInput{
		Signature: cfg.InvocationSignature,
		NamedArgs: []invocations.NamedArgumentInput{{Key: "planning-detail", Values: []string{"verbose"}}},
	})
	var argumentErr *invocations.ArgumentError
	if !errors.As(err, &argumentErr) {
		t.Fatalf("NormalizeArguments(invalid planning detail) error = %v, want ArgumentError", err)
	}
	if argumentErr.Parameter != "planningDetail" || !strings.Contains(argumentErr.Error(), "declared choices") {
		t.Fatalf("invalid parameter diagnostic = %#v, want actionable planningDetail choices error", argumentErr)
	}
}

func assertNormalizedArgument(t *testing.T, normalized *invocations.NormalizedArguments, name string, want []string) {
	t.Helper()
	got, ok := normalized.Arguments[name]
	if !ok || !reflect.DeepEqual(got.Values, want) {
		t.Fatalf("argument %q = %#v, want %#v", name, got.Values, want)
	}
}

func assertLoopBreaker(t *testing.T, workstations []interfaces.FactoryWorkstationConfig) {
	t.Helper()

	loopBreaker := findWorkstation(t, workstations, PackagedLoopBreakerName)
	assertRoute(t, loopBreaker.Inputs, "execute")
	assertRoute(t, loopBreaker.Outputs, "failed")
	if loopBreaker.Type != interfaces.WorkstationTypeLogical {
		t.Fatalf("loop breaker type = %q, want %q", loopBreaker.Type, interfaces.WorkstationTypeLogical)
	}
	if len(loopBreaker.Guards) != 1 || loopBreaker.Guards[0].Type != interfaces.GuardTypeVisitCount || loopBreaker.Guards[0].Workstation != PackagedExecuteWorkstationName || loopBreaker.Guards[0].MaxVisits != 8 {
		t.Fatalf("loop breaker guards = %#v, want visit count for %q at 8", loopBreaker.Guards, PackagedExecuteWorkstationName)
	}
}

func findWorkstation(t *testing.T, workstations []interfaces.FactoryWorkstationConfig, name string) interfaces.FactoryWorkstationConfig {
	t.Helper()
	for _, workstation := range workstations {
		if workstation.Name == name {
			return workstation
		}
	}
	t.Fatalf("missing workstation %q", name)
	return interfaces.FactoryWorkstationConfig{}
}

func assertRoute(t *testing.T, routes []interfaces.IOConfig, state string) {
	t.Helper()
	for _, route := range routes {
		if route.WorkTypeName == PackagedWorkTypeName && route.StateName == state {
			return
		}
	}
	t.Fatalf("routes = %#v, want %s:%s", routes, PackagedWorkTypeName, state)
}
