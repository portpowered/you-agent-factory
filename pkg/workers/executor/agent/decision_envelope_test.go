package agent_test

import (
	"context"
	"strings"
	"testing"
	"time"

	factorytoken "github.com/portpowered/infinite-you/pkg/factory/token"
	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"

	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"

	"github.com/portpowered/infinite-you/internal/testutil/runtimefixtures"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/factory/packages/goal"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/factory/token_transformer"
	"github.com/portpowered/infinite-you/pkg/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/work"
	executorpkg "github.com/portpowered/infinite-you/pkg/workers/executor"
	"github.com/portpowered/infinite-you/pkg/workers/prompting"
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
		Workers: map[string]*workerconfig.Config{
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
	now := time.Date(2026, time.June, 20, 10, 0, 0, 0, time.UTC)
	transformer := token_transformer.New(
		map[string]*petri.Place{
			"task:init": {ID: "task:init", TypeID: "task", State: "init"},
		},
		map[string]*state.WorkType{
			"task": {
				ID: "task",
				States: []state.StateDefinition{
					{Value: "init", Category: state.StateCategoryInitial},
				},
			},
		},
	)
	consumed := factorytoken.Token{
		ID: "work-task-1",
		Color: factorytoken.Color{
			WorkID:     "work-task-1",
			WorkTypeID: "task",
			Payload:    []byte(priorDraft),
			Tags: map[string]string{
				"_last_output": priorDraft,
			},
		},
	}
	outputToken, err := transformer.OutputToken(token_transformer.OutputTokenInput{
		ArcIndex: 0,
		Arcs: []petri.Arc{
			{PlaceID: "task:init", Direction: petri.ArcOutput},
		},
		ConsumedTokens: []factorytoken.Token{consumed},
		InputColors:    []factorytoken.Color{consumed.Color},
		Output:         result.Output,
		Outcome:        result.Outcome,
		Feedback:       result.Feedback,
		Now:            now,
		History:        factorytoken.History{TotalVisits: map[string]int{"review": 1}},
	})
	if err != nil {
		t.Fatalf("OutputToken: %v", err)
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
		[]factorytoken.Token{*outputToken},
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
		response: workerexecution.InferenceResponse{Content: `{"decision":"ACCEPTED","feedback":"ignored"}`},
	}
	executor := executorpkg.NewAgentExecutor(runtimefixtures.RuntimeConfigLookupFixture{
		Workers: map[string]*workerconfig.Config{
			"processor": {Model: "test-model", StopToken: "<COMPLETE>"},
		},
	}, provider)

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
