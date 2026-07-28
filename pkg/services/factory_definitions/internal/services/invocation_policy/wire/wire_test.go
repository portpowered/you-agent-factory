package wire_test

import (
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	invocationpolicyservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/invocation_policy"
	invocationpolicywire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/invocation_policy/wire"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestNewService_ConstructsPublishedRootPolicyContracts(t *testing.T) {
	t.Parallel()

	svc, err := invocationpolicywire.NewService(invocationpolicyservice.Dependencies{})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	if svc == nil {
		t.Fatal("NewService returned nil service")
	}
	var _ invocationpolicyservice.Service = svc

	if svc.DecisionEnvelope() == nil {
		t.Fatal("DecisionEnvelope() returned nil")
	}
	if svc.InvocationInterpolation() == nil {
		t.Fatal("InvocationInterpolation() returned nil")
	}
	if svc.InvocationOutput() == nil {
		t.Fatal("InvocationOutput() returned nil")
	}
	if svc.InvocationWorkType() == nil {
		t.Fatal("InvocationWorkType() returned nil")
	}
	if svc.QuorumPolicy() == nil {
		t.Fatal("QuorumPolicy() returned nil")
	}
	if svc.WorkPropagation() == nil {
		t.Fatal("WorkPropagation() returned nil")
	}
	if svc.WorkstationExecution() == nil {
		t.Fatal("WorkstationExecution() returned nil")
	}
	if svc.TTSObservability() == nil {
		t.Fatal("TTSObservability() returned nil")
	}
}

func TestNewService_DecisionEnvelopeContractThroughNestedOwner(t *testing.T) {
	t.Parallel()

	svc, err := invocationpolicywire.NewService(invocationpolicyservice.Dependencies{})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	decisionEnvelopes := svc.DecisionEnvelope()
	workstation := &factorydefinitions.FactoryWorkstationConfig{
		OutcomeFormat: factorydefinitions.DecisionEnvelopeOutcomeFormat,
	}
	if !decisionEnvelopes.UsesDecisionEnvelopeOutcome(workstation) {
		t.Fatal("UsesDecisionEnvelopeOutcome() = false, want true for decision-envelope workstation")
	}

	raw := `{"decision":"ACCEPTED","feedback":"Ship it.","output":"done"}`
	result := decisionEnvelopes.WorkResultFromDecisionEnvelopeJSONOrFailed(
		"dispatch-1",
		"transition-1",
		raw,
	)
	if result.Outcome != workerexecution.OutcomeAccepted {
		t.Fatalf("WorkResultFromDecisionEnvelopeJSONOrFailed() outcome = %q, want %q", result.Outcome, workerexecution.OutcomeAccepted)
	}
	if result.Feedback != "Ship it." {
		t.Fatalf("WorkResultFromDecisionEnvelopeJSONOrFailed() feedback = %q, want %q", result.Feedback, "Ship it.")
	}
}
