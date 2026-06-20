package agent_test

import (
	"context"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/packagedfactories/goal"
	"github.com/portpowered/infinite-you/pkg/testutil/runtimefixtures"
	executorpkg "github.com/portpowered/infinite-you/pkg/workers/executor"
)

func reviewAgentRequest(dispatchID string, content string) (interfaces.WorkstationExecutionRequest, *agentMockProvider) {
	provider := &agentMockProvider{
		response: interfaces.InferenceResponse{Content: content},
	}
	request := testAgentRequest(
		interfaces.WorkDispatch{
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
		Workers: map[string]*interfaces.WorkerConfig{
			"planner": {Model: "test-model", StopToken: "<COMPLETE>"},
		},
		Workstations: map[string]*interfaces.FactoryWorkstationConfig{
			"review": {OutcomeFormat: goal.DecisionEnvelopeOutcomeFormat},
		},
	}, provider)
}

func TestAgentExecutor_ReviewWorkstation_ParsesDecisionEnvelopeAccepted(t *testing.T) {
	raw := `{"decision":"ACCEPTED","feedback":"All criteria pass."}`
	request, provider := reviewAgentRequest("d-review-accepted", raw)
	executor := reviewAgentExecutor(provider)

	result, err := executor.Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Outcome != interfaces.OutcomeAccepted {
		t.Fatalf("Outcome = %q, want ACCEPTED", result.Outcome)
	}
	if result.Feedback != "All criteria pass." {
		t.Fatalf("Feedback = %q, want reviewer feedback", result.Feedback)
	}
	if result.Output != raw {
		t.Fatalf("Output = %q, want raw envelope JSON", result.Output)
	}
}

func TestAgentExecutor_ReviewWorkstation_ParsesDecisionEnvelopeContinueAndRejected(t *testing.T) {
	cases := []struct {
		name     string
		decision string
		want     interfaces.WorkOutcome
	}{
		{name: "continue", decision: "CONTINUE", want: interfaces.OutcomeContinue},
		{name: "rejected", decision: "REJECTED", want: interfaces.OutcomeRejected},
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
			if result.Outcome != goal.MalformedEnvelopeFailureOutcome {
				t.Fatalf("Outcome = %q, want %q", result.Outcome, goal.MalformedEnvelopeFailureOutcome)
			}
			if !strings.Contains(result.Error, tc.wantErr) {
				t.Fatalf("Error = %q, want %q", result.Error, tc.wantErr)
			}
		})
	}
}

func TestAgentExecutor_ProcessWorkstation_StillUsesStopTokenWhenNotReview(t *testing.T) {
	provider := &agentMockProvider{
		response: interfaces.InferenceResponse{Content: `{"decision":"ACCEPTED","feedback":"ignored"}`},
	}
	executor := executorpkg.NewAgentExecutor(runtimefixtures.RuntimeConfigLookupFixture{
		Workers: map[string]*interfaces.WorkerConfig{
			"processor": {Model: "test-model", StopToken: "<COMPLETE>"},
		},
	}, provider)

	result, err := executor.Execute(context.Background(), testAgentRequest(
		interfaces.WorkDispatch{
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
	if result.Outcome != interfaces.OutcomeRejected {
		t.Fatalf("Outcome = %q, want REJECTED from stop-token path", result.Outcome)
	}
}
