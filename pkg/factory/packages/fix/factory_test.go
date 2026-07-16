package fix

import (
	"testing"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestBuiltInFactoryJSON_ModelsIsolatedPlanImplementReviewLoop(t *testing.T) {
	cfg, err := factoryconfig.FactoryConfigFromOpenAPIJSON(BuiltInFactoryJSON)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}
	if cfg.Name != PackagedFactoryName || cfg.Project != PackagedFactoryProject {
		t.Fatalf("factory identity = %q/%q, want %q/%q", cfg.Name, cfg.Project, PackagedFactoryName, PackagedFactoryProject)
	}
	if len(cfg.WorkTypes) != 1 || cfg.WorkTypes[0].Name != PackagedWorkTypeName {
		t.Fatalf("workTypes = %#v, want one %q work type", cfg.WorkTypes, PackagedWorkTypeName)
	}
	if cfg.InvocationReturn == nil || cfg.InvocationReturn.WorkTypeName != PackagedWorkTypeName || cfg.InvocationReturn.TerminalState != "complete" {
		t.Fatalf("invocationReturn = %#v, want explicit fix:complete", cfg.InvocationReturn)
	}

	workstations := map[string]interfaces.FactoryWorkstationConfig{}
	for _, workstation := range cfg.Workstations {
		workstations[workstation.Name] = workstation
	}
	for _, name := range []string{PackagedPlanWorkstationName, PackagedImplementWorkstationName, PackagedReviewWorkstationName} {
		workstation, ok := workstations[name]
		if !ok {
			t.Fatalf("missing workstation %q", name)
		}
		if workstation.Worktree != "fix-{{ (index .Inputs 0).TraceID }}" {
			t.Fatalf("%s worktree = %q, want stable isolated invocation worktree", name, workstation.Worktree)
		}
	}
	if workstations[PackagedImplementWorkstationName].Kind != interfaces.WorkstationKindRepeater {
		t.Fatalf("implement workstation kind = %q, want repeater", workstations[PackagedImplementWorkstationName].Kind)
	}
	assertFixRoute(t, workstations[PackagedReviewWorkstationName].OnRejection, "implement")
	assertFixRoute(t, workstations[PackagedReviewWorkstationName].Outputs, "complete")

	for _, target := range factoryvalidation.Validate(cfg).Targets {
		if target.Severity == factoryvalidation.SeverityError {
			t.Fatalf("validation target = %#v", target)
		}
	}
}

func assertFixRoute(t *testing.T, routes []interfaces.IOConfig, state string) {
	t.Helper()
	for _, route := range routes {
		if route.WorkTypeName == PackagedWorkTypeName && route.StateName == state {
			return
		}
	}
	t.Fatalf("routes = %#v, want %s:%s", routes, PackagedWorkTypeName, state)
}
