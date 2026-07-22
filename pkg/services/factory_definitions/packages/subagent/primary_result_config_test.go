package subagent

import (
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
	"testing"
)

func TestBuiltInFactoryJSON_UsesSubmittedWorkTerminalInvocationReturnDefault(t *testing.T) {
	cfg, err := factorymapping.FactoryConfigFromOpenAPIJSON(BuiltInFactoryJSON)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}
	if cfg.InvocationReturn != nil {
		t.Fatalf("InvocationReturn = %#v, want nil so shared resolver uses SUBMITTED_WORK_TERMINAL", cfg.InvocationReturn)
	}
}
