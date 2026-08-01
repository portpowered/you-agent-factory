package wire_test

import (
	"testing"

	factorydefinitionswire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire"
)

func TestNewInvocationPolicySharesOneOwnerProjection(t *testing.T) {
	t.Parallel()

	policy, err := factorydefinitionswire.NewInvocationPolicy()
	if err != nil {
		t.Fatalf("NewInvocationPolicy() error = %v", err)
	}
	if policy.DecisionEnvelope == nil {
		t.Fatal("DecisionEnvelope is nil")
	}
	if policy.InvocationInterpolation == nil {
		t.Fatal("InvocationInterpolation is nil")
	}
	if policy.InvocationOutput == nil {
		t.Fatal("InvocationOutput is nil")
	}
	if policy.InvocationWorkType == nil {
		t.Fatal("InvocationWorkType is nil")
	}
	if policy.QuorumPolicy == nil {
		t.Fatal("QuorumPolicy is nil")
	}
	if policy.WorkPropagation == nil {
		t.Fatal("WorkPropagation is nil")
	}
	if policy.WorkstationExecution == nil {
		t.Fatal("WorkstationExecution is nil")
	}
	if policy.TTSObservability == nil {
		t.Fatal("TTSObservability is nil")
	}
}
