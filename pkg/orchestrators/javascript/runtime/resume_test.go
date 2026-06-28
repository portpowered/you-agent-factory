package workflowruntime

import (
	"context"
	"testing"
)

func TestResumingChildExecutor_ReplaysCompletedDispatchWithoutCallingBase(t *testing.T) {
	base := &countingChildExecutor{}
	resume := ResumeContext{
		CompletedDispatchIDs: []string{"dispatch-1"},
		CompletedChildResults: map[string]ChildExecutionResult{
			"dispatch-1": {
				DispatchID:    "dispatch-1",
				ChildIndex:    1,
				Status:        ChildDispatchStatusCompleted,
				ExecutionMode: ChildExecutionModeFake,
				Output: map[string]any{
					"text": "cached:first",
				},
			},
		},
	}
	executor := NewResumingChildExecutor(base, resume)

	first, err := executor.Execute(context.Background(), ChildExecutionRequest{Label: "step-one"})
	if err != nil {
		t.Fatalf("first Execute() error = %v", err)
	}
	if first.Output["text"] != "cached:first" {
		t.Fatalf("first output = %#v, want cached:first", first.Output)
	}
	if base.calls != 0 {
		t.Fatalf("base calls after replay = %d, want 0", base.calls)
	}

	second, err := executor.Execute(context.Background(), ChildExecutionRequest{Label: "step-two"})
	if err != nil {
		t.Fatalf("second Execute() error = %v", err)
	}
	if second.DispatchID != "dispatch-2" {
		t.Fatalf("second dispatchId = %q, want dispatch-2", second.DispatchID)
	}
	if base.calls != 1 {
		t.Fatalf("base calls after second dispatch = %d, want 1", base.calls)
	}
}

type countingChildExecutor struct {
	calls int
}

func (c *countingChildExecutor) Execute(_ context.Context, req ChildExecutionRequest) (ChildExecutionResult, error) {
	c.calls++
	dispatchID := "dispatch-2"
	if req.ReservedIdentity != nil && req.ReservedIdentity.DispatchID != "" {
		dispatchID = req.ReservedIdentity.DispatchID
	}
	return ChildExecutionResult{
		DispatchID:    dispatchID,
		ChildIndex:    2,
		Status:        ChildDispatchStatusCompleted,
		ExecutionMode: ChildExecutionModeFake,
		Output: map[string]any{
			"text":  "fresh:second",
			"label": req.Label,
		},
	}, nil
}
