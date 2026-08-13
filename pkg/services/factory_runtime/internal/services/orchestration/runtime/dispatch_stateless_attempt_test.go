package runtime

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestStartThroughStatelessWorkersBuildsCorrelatedDetachedRequest(t *testing.T) {
	var observed workers.ExecuteRequest
	service := attemptExecuteFunc(func(_ context.Context, request workers.ExecuteRequest) (workers.ExecuteResult, error) {
		observed = request
		return workers.ExecuteResult{
			Correlation: request.Correlation,
			Outcome:     workers.ExecutionOutcomeAccepted,
			Output: workers.ProposedOutput{
				Primary: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "done"}},
				ProposedWork: []workers.ProposedWork{{
					WorkTypeID: "review",
					Name:       "review-1",
					State:      "init",
				}},
			},
		}, nil
	})
	cfg := &runtimeConfig{
		executeService: service,
		newID:          func() string { return "attempt-1" },
		attempts:       newAttemptLifecycle(service, func() string { return "attempt-1" }, 1),
		inlineDispatch: true,
	}
	request := workers.WorkstationDispatchRequest{
		WorkstationName: "workstation-a",
		Execution: workers.WorkstationExecutionRequest{
			WorkerName:       "worker-a",
			WorkerType:       "agent",
			RunnerID:         workers.RunnerIDCodex,
			FactorySessionID: "session-1",
			RecordingID:      "runtime-1",
			Model:            "model-a",
			ModelProvider:    "provider-a",
			Dispatch: work.WorkDispatch{
				DispatchID:      "dispatch-1",
				TransitionID:    "transition-a",
				WorkerType:      "agent",
				WorkstationName: "workstation-a",
				Execution: work.ExecutionMetadata{
					RequestID: "request-1",
					TraceID:   "trace-1",
					WorkIDs:   []string{"work-1"},
				},
				InputTokens: workers.InputTokens(workers.Token{Color: workers.Color{
					WorkID:     "work-1",
					WorkTypeID: "task",
					RequestID:  "request-1",
					DataType:   workers.DataTypeWork,
					TraceID:    "trace-1",
					Content: []work.WorkContentPart{{
						Type: work.WorkContentPartTypeText,
						Text: "input",
					}},
				}}),
			},
		},
	}
	var got workers.WorkstationDispatchResult
	var gotErr error
	if err := startThroughStatelessWorkers(
		context.Background(),
		cfg,
		request,
		func(_ context.Context, _ workers.WorkstationDispatchRequest, result workers.WorkstationDispatchResult, err error) {
			got = result
			gotErr = err
		},
	); err != nil {
		t.Fatalf("startThroughStatelessWorkers() error = %v", err)
	}
	if gotErr != nil {
		t.Fatalf("dispatch callback error = %v", gotErr)
	}
	if got.TerminalOutcome != workers.WorkstationDispatchTerminalOutcomeCompleted || got.Result.Output != "done" {
		t.Fatalf("dispatch result = %#v", got)
	}
	if got.ProposedOutput == nil || len(got.ProposedOutput.ProposedWork) != 1 ||
		got.ProposedOutput.ProposedWork[0].Name != "review-1" {
		t.Fatalf("detached proposed output = %#v", got.ProposedOutput)
	}
	if observed.Correlation.DispatchID != "dispatch-1" || observed.Correlation.AttemptID != "attempt-1" {
		t.Fatalf("correlation = %#v", observed.Correlation)
	}
	if observed.Correlation.FactorySessionID != "session-1" || observed.Correlation.RequestID != "request-1" || observed.Correlation.TraceID != "trace-1" {
		t.Fatalf("correlation lineage = %#v", observed.Correlation)
	}
	if len(observed.Input.Work) != 1 || observed.Input.Work[0].WorkID != "work-1" || observed.Input.Work[0].Content[0].Text != "input" {
		t.Fatalf("work input = %#v", observed.Input.Work)
	}
}

func TestStartThroughStatelessWorkersPreservesDetachedDispatchFacts(t *testing.T) {
	var observed workers.ExecuteRequest
	service := attemptExecuteFunc(func(_ context.Context, request workers.ExecuteRequest) (workers.ExecuteResult, error) {
		observed = request
		return workers.ExecuteResult{
			Correlation: request.Correlation,
			Outcome:     workers.ExecutionOutcomeAccepted,
		}, nil
	})
	cfg := &runtimeConfig{
		executeService: service,
		newID:          func() string { return "attempt-facts" },
		attempts:       newAttemptLifecycle(service, func() string { return "attempt-facts" }, 1),
		inlineDispatch: true,
	}
	dispatch := work.WorkDispatch{
		DispatchID:      "dispatch-facts",
		TransitionID:    "transition-facts",
		WorkerType:      "agent",
		WorkstationName: "workstation-facts",
		ProjectID:       "project-facts",
		Execution: work.ExecutionMetadata{
			RequestID: "request-facts",
			TraceID:   "trace-facts",
			ReplayKey: "replay-facts",
			WorkIDs:   []string{"work-facts"},
		},
	}
	request := workers.WorkstationDispatchRequest{
		WorkstationName: "workstation-facts",
		Execution: workers.WorkstationExecutionRequest{
			Dispatch:         dispatch,
			WorkerType:       "agent",
			RunnerID:         workers.RunnerIDCodex,
			FactorySessionID: "session-facts",
			RecordingID:      "runtime-facts",
		},
	}
	if err := startThroughStatelessWorkers(context.Background(), cfg, request, nil); err != nil {
		t.Fatalf("startThroughStatelessWorkers() error = %v", err)
	}
	if observed.Input.Dispatch.ProjectID != "project-facts" ||
		observed.Input.Dispatch.Execution.ReplayKey != "replay-facts" ||
		observed.Input.Dispatch.TransitionID != "transition-facts" {
		t.Fatalf("detached dispatch facts = %#v", observed.Input.Dispatch)
	}
}
