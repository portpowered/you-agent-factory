package wire_test

import (
	"testing"

	factorydefinitionswire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire"
)

func TestInvocationPolicyPortsFromNestedOwner(t *testing.T) {
	t.Parallel()

	ports, err := factorydefinitionswire.InvocationPolicyPortsFromNestedOwner()
	if err != nil {
		t.Fatalf("InvocationPolicyPortsFromNestedOwner() error = %v", err)
	}
	if ports.DecisionEnvelope == nil {
		t.Fatal("DecisionEnvelope is nil")
	}
	if ports.InvocationInterpolation == nil {
		t.Fatal("InvocationInterpolation is nil")
	}
	if ports.InvocationOutput == nil {
		t.Fatal("InvocationOutput is nil")
	}
	if ports.InvocationWorkType == nil {
		t.Fatal("InvocationWorkType is nil")
	}
	if ports.QuorumPolicy == nil {
		t.Fatal("QuorumPolicy is nil")
	}
	if ports.WorkPropagation == nil {
		t.Fatal("WorkPropagation is nil")
	}
	if ports.WorkstationExecution == nil {
		t.Fatal("WorkstationExecution is nil")
	}
	if ports.TTSObservability == nil {
		t.Fatal("TTSObservability is nil")
	}
}
