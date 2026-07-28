package wire_test

import (
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	invocationpolicyservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/invocation_policy"
	invocationpolicywire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/invocation_policy/wire"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/work"
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

func TestNewService_GoalRoutingDecisionEnvelopeThroughNestedOwner(t *testing.T) {
	t.Parallel()

	svc, err := invocationpolicywire.NewService(invocationpolicyservice.Dependencies{})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	decisionEnvelopes := svc.DecisionEnvelope()
	workstation := &factorydefinitions.FactoryWorkstationConfig{
		OutcomeFormat: factorydefinitions.DecisionEnvelopeOutcomeFormat,
		ClassificationRoutes: []factorydefinitions.ClassificationRouteConfig{
			{Label: "accepted", Outputs: []factorydefinitions.IOConfig{{WorkTypeName: "goal", StateName: "complete"}}},
		},
	}
	if !decisionEnvelopes.UsesGoalRoutingDecisionEnvelope(workstation) {
		t.Fatal("UsesGoalRoutingDecisionEnvelope() = false, want true for goal routing workstation")
	}

	acceptedRaw := `{"decision":"accepted","feedback":"Approved.","output":"done"}`
	accepted := decisionEnvelopes.WorkResultFromGoalRoutingDecisionEnvelopeJSONOrFailed(
		"dispatch-goal",
		"review-goal",
		acceptedRaw,
	)
	if accepted.Outcome != workerexecution.OutcomeAccepted {
		t.Fatalf("accepted envelope outcome = %q, want %q", accepted.Outcome, workerexecution.OutcomeAccepted)
	}
	if accepted.SelectedClassificationLabel != factorydefinitions.GoalRoutingDecisionAccepted {
		t.Fatalf("accepted envelope label = %q, want %q", accepted.SelectedClassificationLabel, factorydefinitions.GoalRoutingDecisionAccepted)
	}

	needsChangesRaw := `{"decision":"needs-changes","feedback":"Rework required."}`
	needsChanges := decisionEnvelopes.WorkResultFromGoalRoutingDecisionEnvelopeJSONOrFailed(
		"dispatch-goal",
		"review-goal",
		needsChangesRaw,
	)
	if needsChanges.SelectedClassificationLabel != factorydefinitions.GoalRoutingDecisionNeedsChanges {
		t.Fatalf("needs-changes envelope label = %q, want %q", needsChanges.SelectedClassificationLabel, factorydefinitions.GoalRoutingDecisionNeedsChanges)
	}

	malformed := decisionEnvelopes.WorkResultFromGoalRoutingDecisionEnvelopeJSONOrFailed(
		"dispatch-goal",
		"review-goal",
		`not-json`,
	)
	if malformed.Outcome != factorydefinitions.MalformedEnvelopeFailureOutcome {
		t.Fatalf("malformed envelope outcome = %q, want %q", malformed.Outcome, factorydefinitions.MalformedEnvelopeFailureOutcome)
	}
	if malformed.Error == "" {
		t.Fatal("malformed envelope error is empty, want actionable failure text")
	}
}

func TestNewService_InvocationInterpolationThroughNestedOwner(t *testing.T) {
	t.Parallel()

	svc, err := invocationpolicywire.NewService(invocationpolicyservice.Dependencies{})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	interpolation := svc.InvocationInterpolation()
	workstation, err := interpolation.InterpolateWorkstationConfig(
		factorydefinitions.FactoryWorkstationConfig{
			PromptTemplate: "Use ${input} now",
		},
		&work.InvocationArguments{
			Arguments: map[string]work.InvocationArgument{
				"input": {Values: []string{"draft"}},
			},
		},
		nil,
	)
	if err != nil {
		t.Fatalf("InterpolateWorkstationConfig: %v", err)
	}
	if workstation.PromptTemplate != "Use draft now" {
		t.Fatalf("PromptTemplate = %q, want interpolated draft", workstation.PromptTemplate)
	}

	_, err = interpolation.InterpolateWorkstationConfig(
		factorydefinitions.FactoryWorkstationConfig{
			PromptTemplate: "Use ${missing} now",
		},
		&work.InvocationArguments{},
		nil,
	)
	if err == nil {
		t.Fatal("InterpolateWorkstationConfig error = nil, want invalid interpolation")
	}
}

func TestNewService_InvocationOutputThroughNestedOwner(t *testing.T) {
	t.Parallel()

	svc, err := invocationpolicywire.NewService(invocationpolicyservice.Dependencies{})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	output := svc.InvocationOutput()
	workstation := &factorydefinitions.FactoryWorkstationConfig{
		Name: "execute-goal",
		Type: factorydefinitions.WorkstationTypeModel,
	}
	if !output.ShouldFormatInvocationSummary(workstation) {
		t.Fatal("ShouldFormatInvocationSummary() = false, want true for goal workstation")
	}

	content, err := output.SummaryContentFromWorkerOutput("Final goal summary.\nCOMPLETE", "COMPLETE")
	if err != nil {
		t.Fatalf("SummaryContentFromWorkerOutput: %v", err)
	}
	if len(content) != 1 || content[0].Text != "Final goal summary." {
		t.Fatalf("summary content = %#v, want normalized goal summary text", content)
	}
}

func TestNewService_InvocationWorkTypeThroughNestedOwner(t *testing.T) {
	t.Parallel()

	svc, err := invocationpolicywire.NewService(invocationpolicyservice.Dependencies{})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	workType, err := svc.InvocationWorkType().DefaultWorkType(&factorydefinitions.FactoryConfig{
		WorkTypes: []factorydefinitions.WorkTypeConfig{
			{Name: "task"},
			{Name: "story", HandlingBehavior: []string{factorydefinitions.WorkTypeHandlingBehaviorDefault}},
		},
	})
	if err != nil {
		t.Fatalf("DefaultWorkType: %v", err)
	}
	if workType != "story" {
		t.Fatalf("DefaultWorkType = %q, want story", workType)
	}
}
