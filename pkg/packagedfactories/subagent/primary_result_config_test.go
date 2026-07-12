package subagent

import (
	"testing"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
)

func TestBuiltInFactoryJSON_UsesSubmittedWorkTerminalInvocationReturnDefault(t *testing.T) {
	cfg, err := factoryconfig.FactoryConfigFromOpenAPIJSON(factoryconfig.BuiltInSubagentFactoryJSON)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}
	if cfg.InvocationReturn != nil {
		t.Fatalf("InvocationReturn = %#v, want nil so shared resolver uses SUBMITTED_WORK_TERMINAL", cfg.InvocationReturn)
	}
}
