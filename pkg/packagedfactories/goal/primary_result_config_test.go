package goal

import (
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
)

func TestBuiltInFactoryJSON_ExposesExplicitGoalSummaryInvocationReturn(t *testing.T) {
	cfg, err := factoryconfig.FactoryConfigFromOpenAPIJSON(factoryconfig.BuiltInGoalFactoryJSON)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}
	if cfg.InvocationReturn == nil {
		t.Fatal("InvocationReturn = nil, want explicit goal summary policy")
	}
	if cfg.InvocationReturn.Policy != string(factoryapi.InvocationReturnPolicyExplicit) {
		t.Fatalf("policy = %q, want %q", cfg.InvocationReturn.Policy, factoryapi.InvocationReturnPolicyExplicit)
	}
	if cfg.InvocationReturn.WorkTypeName != PackagedInvocationReturnWorkTypeName {
		t.Fatalf("workTypeName = %q, want %q", cfg.InvocationReturn.WorkTypeName, PackagedInvocationReturnWorkTypeName)
	}
	if cfg.InvocationReturn.TerminalState != PackagedInvocationReturnTerminalState {
		t.Fatalf("terminalState = %q, want %q", cfg.InvocationReturn.TerminalState, PackagedInvocationReturnTerminalState)
	}

	for _, target := range factoryvalidation.Validate(cfg).Targets {
		if strings.HasPrefix(target.Code, "factory.invocationReturn.") {
			t.Fatalf("validation targets = %#v, want valid invocationReturn policy", target)
		}
	}
}
