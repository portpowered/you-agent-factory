package agent_test

import (
	"context"
	"strings"
	"testing"
	"time"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"

	"github.com/portpowered/infinite-you/internal/testutil/runtimefixtures"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/work"
	executorpkg "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/executor"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/prompting"
)

func reviewAgentRequest(dispatchID string, content string) (workerexecution.WorkstationExecutionRequest, *agentMockProvider) {
	provider := &agentMockProvider{
		response: workerexecution.InferenceResponse{Content: content},
	}
	request := testAgentRequest(
		work.WorkDispatch{
			DispatchID:      dispatchID,
			TransitionID:    "review",
			WorkerType:      "planner",
			WorkstationName: "review",
		},
		withAgentPrompts("review system", "review user"),
	)
	request.WorkstationType = "review"
	return request, provider
}

func reviewAgentExecutor(provider *agentMockProvider) *executorpkg.AgentExecutor {
	return executorpkg.NewAgentExecutor(runtimefixtures.RuntimeConfigLookupFixture{
		Workers: map[string]*interfaces.FactoryWorkerConfig{
			"planner": {Model: "test-model", StopToken: "<COMPLETE>"},
		},
		Workstations: map[string]*interfaces.FactoryWorkstationConfig{
			"review": {OutcomeFormat: interfaces.DecisionEnvelopeOutcomeFormat},
		},
	}, provider, nil, time.Now, agentDecisionEnvelopeFake{})

}

type agentDecisionEnvelopeFake struct{}

func (agentDecisionEnvelopeFake) UsesDecisionEnvelopeOutcome(workstation *interfaces.FactoryWorkstationConfig) bool {
	return workstation != nil && workstation.OutcomeFormat == interfaces.DecisionEnvelopeOutcomeFormat
}

func (agentDecisionEnvelopeFake) UsesGoalRoutingDecisionEnvelope(*interfaces.FactoryWorkstationConfig) bool {
	return false
}

func (agentDecisionEnvelopeFake) WorkResultFromDecisionEnvelopeJSONOrFailed(
	dispatchID string,
	transitionID string,
	raw string,
) workerexecution.WorkResult {
	result := workerexecution.WorkResult{DispatchID: dispatchID, TransitionID: transitionID}
	switch raw {
	case `{"decision":"ACCEPTED","feedback":"All criteria pass."}`:
		result.Outcome = workerexecution.OutcomeAccepted
		result.Feedback = "All criteria pass."
	case `{"decision":"ACCEPTED","feedback":"All criteria pass.","output":"Ship-ready summary."}`:
		result.Outcome = workerexecution.OutcomeAccepted
		result.Feedback = "All criteria pass."
		result.Output = "Ship-ready summary."
	case `{"decision":"REJECTED","feedback":"Add tests."}`:
		result.Outcome = workerexecution.OutcomeRejected
		result.Feedback = "Add tests."
	case `{"decision":"CONTINUE","feedback":"review notes"}`:
		result.Outcome = workerexecution.OutcomeContinue
		result.Feedback = "review notes"
	case `{"decision":"REJECTED","feedback":"review notes"}`:
		result.Outcome = workerexecution.OutcomeRejected
		result.Feedback = "review notes"
	case "not-json":
		result.Outcome = workerexecution.OutcomeFailed
		result.Error = "invalid JSON"
	default:
		result.Outcome = workerexecution.OutcomeFailed
		result.Error = `unknown decision "MAYBE"`
	}
	return result
}

func (agentDecisionEnvelopeFake) WorkResultFromGoalRoutingDecisionEnvelopeJSONOrFailed(
	dispatchID string,
	transitionID string,
	raw string,
) workerexecution.WorkResult {
	return agentDecisionEnvelopeFake{}.WorkResultFromDecisionEnvelopeJSONOrFailed(dispatchID, transitionID, raw)
}

func TestAgentExecutor_ReviewWorkstation_ParsesDecisionEnvelopeAccepted(t *testing.T) {
	raw := `{"decision":"ACCEPTED","feedback":"All criteria pass."}`
	request, provider := reviewAgentRequest("d-review-accepted", raw)
	executor := reviewAgentExecutor(provider)

	result, err := executor.Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Outcome != workerexecution.OutcomeAccepted {
		t.Fatalf("Outcome = %q, want ACCEPTED", result.Outcome)
	}
	if result.Feedback != "All criteria pass." {
		t.Fatalf("Feedback = %q, want reviewer feedback", result.Feedback)
	}
	if result.Output != "" {
		t.Fatalf("Output = %q, want empty when envelope omits output", result.Output)
	}
}

func TestAgentExecutor_ReviewWorkstation_ParsesDecisionEnvelopeExplicitOutput(t *testing.T) {
	raw := `{"decision":"ACCEPTED","feedback":"All criteria pass.","output":"Ship-ready summary."}`
	request, provider := reviewAgentRequest("d-review-explicit-output", raw)
	executor := reviewAgentExecutor(provider)

	result, err := executor.Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Output != "Ship-ready summary." {
		t.Fatalf("Output = %q, want explicit envelope output text", result.Output)
	}
}

func TestAgentExecutor_ReviewWorkstation_OmittedOutputDoesNotLeakIntoRetryPromptState(t *testing.T) {
	raw := `{"decision":"REJECTED","feedback":"Add tests."}`
	request, provider := reviewAgentRequest("d-review-retry-prompt", raw)
	executor := reviewAgentExecutor(provider)

	result, err := executor.Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Output != "" {
		t.Fatalf("Output = %q, want empty when envelope omits output", result.Output)
	}

	const priorDraft = "user-authored draft content"
	outputToken := factoryruntime.RuntimeToken{
		ID: "work-task-1",
		Color: factoryruntime.RuntimeTokenColor{
			WorkID:     "work-task-1",
			WorkTypeID: "task",
			Payload:    []byte(priorDraft),
			Tags: map[string]string{
				"_last_output": priorDraft,
			},
		},
	}
	payload := string(outputToken.Color.Payload)
	if payload != priorDraft {
		t.Fatalf("downstream payload = %q, want preserved draft %q", payload, priorDraft)
	}
	if strings.Contains(payload, `"decision"`) {
		t.Fatalf("downstream payload leaked decision envelope JSON: %q", payload)
	}

	renderer := &prompting.DefaultPromptRenderer{}
	rendered, err := renderer.Render(
		`Previous output: {{ (index .Inputs 0).PreviousOutput }}`,
		[]factoryruntime.RuntimeToken{outputToken},
		nil,
	)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if strings.Contains(rendered, `"decision"`) {
		t.Fatalf("PreviousOutput leaked decision envelope JSON: %q", rendered)
	}
	if !strings.Contains(rendered, priorDraft) {
		t.Fatalf("PreviousOutput = %q, want preserved draft content", rendered)
	}
}

func TestAgentExecutor_ReviewWorkstation_ParsesDecisionEnvelopeContinueAndRejected(t *testing.T) {
	cases := []struct {
		name     string
		decision string
		want     workerexecution.WorkOutcome
	}{
		{name: "continue", decision: "CONTINUE", want: workerexecution.OutcomeContinue},
		{name: "rejected", decision: "REJECTED", want: workerexecution.OutcomeRejected},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := `{"decision":"` + tc.decision + `","feedback":"review notes"}`
			request, provider := reviewAgentRequest("d-review-"+tc.name, raw)
			executor := reviewAgentExecutor(provider)

			result, err := executor.Execute(context.Background(), request)
			if err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if result.Outcome != tc.want {
				t.Fatalf("Outcome = %q, want %q", result.Outcome, tc.want)
			}
		})
	}
}

func TestAgentExecutor_ReviewWorkstation_MalformedEnvelopeUsesFailedOutcome(t *testing.T) {
	cases := []struct {
		name    string
		content string
		wantErr string
	}{
		{
			name:    "invalid json",
			content: "not-json",
			wantErr: "invalid JSON",
		},
		{
			name:    "unknown decision",
			content: `{"decision":"MAYBE","feedback":"unclear"}`,
			wantErr: `unknown decision "MAYBE"`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			request, provider := reviewAgentRequest("d-review-"+tc.name, tc.content)
			executor := reviewAgentExecutor(provider)

			result, err := executor.Execute(context.Background(), request)
			if err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if result.Outcome != workerexecution.OutcomeFailed {
				t.Fatalf("Outcome = %q, want %q", result.Outcome, workerexecution.OutcomeFailed)
			}
			if !strings.Contains(result.Error, tc.wantErr) {
				t.Fatalf("Error = %q, want %q", result.Error, tc.wantErr)
			}
		})
	}
}

func TestAgentExecutor_ProcessWorkstation_StillUsesStopTokenWhenNotReview(t *testing.T) {
	provider := &agentMockProvider{
		response: workerexecution.InferenceResponse{Content: `{"decision":"ACCEPTED","feedback":"ignored"}`},
	}
	executor := executorpkg.NewAgentExecutor(runtimefixtures.RuntimeConfigLookupFixture{
		Workers: map[string]*interfaces.FactoryWorkerConfig{
			"processor": {Model: "test-model", StopToken: "<COMPLETE>"},
		},
	}, provider, nil, time.Now)

	result, err := executor.Execute(context.Background(), testAgentRequest(
		work.WorkDispatch{
			DispatchID:      "d-process-1",
			TransitionID:    "process",
			WorkerType:      "processor",
			WorkstationName: "process",
		},
		withAgentPrompts("process system", "process user"),
	))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Outcome != workerexecution.OutcomeRejected {
		t.Fatalf("Outcome = %q, want REJECTED from stop-token path", result.Outcome)
	}
}
