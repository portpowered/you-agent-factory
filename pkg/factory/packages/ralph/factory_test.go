package ralph

import (
	"strings"
	"testing"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
	"github.com/portpowered/infinite-you/pkg/interfaces"
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
	assertRoute(t, executor.Inputs, "execute")
	assertRoute(t, executor.Outputs, "complete")
	assertRoute(t, executor.OnContinue, "execute")
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
