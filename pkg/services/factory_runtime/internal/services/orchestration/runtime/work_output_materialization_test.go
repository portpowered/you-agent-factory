package runtime

import (
	"context"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workwire "github.com/portpowered/infinite-you/pkg/services/work/wire"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestMaterializeWorkerOutputForDispatchAssignsWorkOwnedIDs(t *testing.T) {
	t.Parallel()

	ids := 0
	result := materializeWorkerOutputForDispatch(
		context.Background(),
		testMaterializationService(),
		&state.Net{
			WorkTypes: map[string]*state.WorkType{
				"review": {
					ID: "review",
					States: []state.StateDefinition{{
						Value:    "init",
						Category: state.StateCategoryInitial,
					}},
				},
			},
		},
		func() string {
			ids++
			return "owned"
		},
		workerexecution.WorkstationDispatchRequest{
			Execution: workerexecution.WorkstationExecutionRequest{
				Dispatch: work.WorkDispatch{
					DispatchID: "dispatch-1",
					Execution: work.ExecutionMetadata{
						RequestID: "request-1",
						TraceID:   "trace-1",
						WorkIDs:   []string{"work-source"},
					},
					CurrentChainingTraceID: "chain-1",
				},
			},
		},
		workerexecution.WorkResult{
			DispatchID:   "dispatch-1",
			TransitionID: "t1",
			Outcome:      workerexecution.OutcomeAccepted,
			Output:       "reviewer output",
			RecordedOutputWork: []work.FactoryWorkItem{{
				ID:          "agent-chosen-id",
				WorkTypeID:  "review",
				DisplayName: "review-1",
				State:       "init",
				Content: []work.WorkContentPart{{
					Type: work.WorkContentPartTypeText,
					Text: "body",
				}},
			}},
		},
	)
	if result.Outcome != workerexecution.OutcomeAccepted {
		t.Fatalf("outcome = %q, want ACCEPTED", result.Outcome)
	}
	if len(result.RecordedOutputWork) != 1 {
		t.Fatalf("RecordedOutputWork = %#v", result.RecordedOutputWork)
	}
	item := result.RecordedOutputWork[0]
	if item.ID == "agent-chosen-id" || !strings.HasPrefix(item.ID, "work-") {
		t.Fatalf("canonical ID = %q, want Work-owned work-* identity", item.ID)
	}
	if item.ParentID != "work-source" {
		t.Fatalf("ParentID = %q, want work-source", item.ParentID)
	}
	if item.DisplayName != "review-1" || item.WorkTypeID != "review" {
		t.Fatalf("item = %#v", item)
	}
}

func TestMaterializeWorkerOutputForDispatchRejectsUnknownTypeAsFailed(t *testing.T) {
	t.Parallel()

	result := materializeWorkerOutputForDispatch(
		context.Background(),
		testMaterializationService(),
		&state.Net{WorkTypes: map[string]*state.WorkType{"task": {ID: "task"}}},
		func() string { return "1" },
		workerexecution.WorkstationDispatchRequest{
			Execution: workerexecution.WorkstationExecutionRequest{
				Dispatch: work.WorkDispatch{
					DispatchID: "dispatch-1",
					Execution:  work.ExecutionMetadata{WorkIDs: []string{"work-1"}},
				},
			},
		},
		workerexecution.WorkResult{
			Outcome: workerexecution.OutcomeAccepted,
			RecordedOutputWork: []work.FactoryWorkItem{{
				ID:          "agent-id",
				WorkTypeID:  "missing",
				DisplayName: "bad",
			}},
		},
	)
	if result.Outcome != workerexecution.OutcomeFailed {
		t.Fatalf("outcome = %q, want FAILED", result.Outcome)
	}
	if len(result.RecordedOutputWork) != 0 {
		t.Fatalf("invalid proposals must not enter Runtime state: %#v", result.RecordedOutputWork)
	}
	if !strings.Contains(result.Error, "worker output materialization") {
		t.Fatalf("error = %q, want materialization detail", result.Error)
	}
}

func TestMaterializeWorkerOutputForDispatchNoopWithoutProposals(t *testing.T) {
	t.Parallel()

	original := workerexecution.WorkResult{
		Outcome: workerexecution.OutcomeContinue,
		Output:  "partial",
	}
	result := materializeWorkerOutputForDispatch(
		context.Background(),
		nil,
		nil,
		nil,
		workerexecution.WorkstationDispatchRequest{},
		original,
	)
	if result.Outcome != workerexecution.OutcomeContinue || result.Output != "partial" {
		t.Fatalf("result = %#v, want unchanged", result)
	}
}

func TestMaterializeExecuteResultPreservesCustomerOutcomes(t *testing.T) {
	t.Parallel()

	for _, outcome := range []workerexecution.WorkOutcome{
		workerexecution.OutcomeAccepted,
		workerexecution.OutcomeContinue,
		workerexecution.OutcomeRejected,
	} {
		outcome := outcome
		t.Run(string(outcome), func(t *testing.T) {
			t.Parallel()
			result := materializeExecuteResultForDispatch(
				context.Background(),
				testMaterializationService(),
				&state.Net{WorkTypes: map[string]*state.WorkType{"task": {ID: "task"}}},
				func() string { return "id" },
				work.WorkDispatch{
					DispatchID: "dispatch-1",
					Execution:  work.ExecutionMetadata{WorkIDs: []string{"work-1"}},
				},
				workerexecution.WorkResult{Outcome: outcome, Output: "body"},
				workerexecution.ExecuteResult{
					Outcome: workerexecution.ExecutionOutcome(outcome),
					Output: workerexecution.ProposedOutput{
						Primary: []work.WorkContentPart{{
							Type: work.WorkContentPartTypeText,
							Text: "body",
						}},
						ProposedWork: []workerexecution.ProposedWork{{
							WorkTypeID: "task",
							Name:       "next",
						}},
					},
				},
			)
			if result.Outcome != outcome {
				t.Fatalf("outcome = %q, want %q", result.Outcome, outcome)
			}
			if len(result.RecordedOutputWork) != 1 || result.RecordedOutputWork[0].ID == "" {
				t.Fatalf("RecordedOutputWork = %#v", result.RecordedOutputWork)
			}
		})
	}
}

func testMaterializationService() work.Service {
	return workwire.NewRuntimeService(nil, nil, nil, nil)
}
