package workflowruntime

import (
	"context"
	"fmt"
)

// ResumeContext carries durable checkpoint and completed child-dispatch facts used
// to reconstruct one interrupted JavaScript workflow session without rerunning
// already satisfied child work by default.
type ResumeContext struct {
	CompletedDispatchIDs  []string
	CompletedChildResults map[string]ChildExecutionResult
	CheckpointState       map[string]any
}

// ResumingChildExecutor wraps one child executor and replays completed dispatch
// results from resume context instead of re-executing satisfied child work.
type ResumingChildExecutor struct {
	base   ChildExecutor
	resume ResumeContext
	next   int
}

// NewResumingChildExecutor constructs one resume-aware child executor wrapper.
func NewResumingChildExecutor(base ChildExecutor, resume ResumeContext) *ResumingChildExecutor {
	if base == nil {
		base = NewFakeChildExecutor("", childRecordSinkFromCollector(newRecordCollector()))
	}
	return &ResumingChildExecutor{
		base:   base,
		resume: resume,
	}
}

// Execute returns cached completed child results or delegates to the wrapped executor.
func (e *ResumingChildExecutor) Execute(ctx context.Context, req ChildExecutionRequest) (ChildExecutionResult, error) {
	if e == nil {
		return ChildExecutionResult{}, fmt.Errorf("resuming child executor is nil")
	}
	e.next++
	dispatchID := fmt.Sprintf("dispatch-%d", e.next)
	if result, ok := e.resume.CompletedChildResults[dispatchID]; ok {
		return result, nil
	}
	req.ReservedIdentity = &ChildDispatchIdentity{
		DispatchID: dispatchID,
		ChildIndex: e.next,
	}
	return e.base.Execute(ctx, req)
}

// CompletedChildResultsFromRecords builds replayable child execution results from
// the latest completed child-dispatch runtime records.
func CompletedChildResultsFromRecords(records []RuntimeRecord) map[string]ChildExecutionResult {
	latest := make(map[string]ChildDispatchRecord)
	for _, record := range records {
		if record.Kind != RecordKindChildDispatch || record.ChildDispatch == nil {
			continue
		}
		child := *record.ChildDispatch
		if child.Status != ChildDispatchStatusCompleted {
			continue
		}
		latest[child.DispatchID] = child
	}
	if len(latest) == 0 {
		return nil
	}
	out := make(map[string]ChildExecutionResult, len(latest))
	for dispatchID, child := range latest {
		out[dispatchID] = childExecutionResultFromRecord(child)
	}
	return out
}

func childExecutionResultFromRecord(child ChildDispatchRecord) ChildExecutionResult {
	executionMode := child.ExecutionMode
	if executionMode == "" {
		executionMode = ChildExecutionModeFake
	}
	return ChildExecutionResult{
		DispatchID:         child.DispatchID,
		ChildIndex:         child.ChildIndex,
		Status:             child.Status,
		ExecutionMode:      executionMode,
		ArtifactRef:        child.ArtifactRef,
		ProviderSessionRef: child.ProviderSessionRef,
		Output: map[string]any{
			"text": fmt.Sprintf(
				"fake:%s:%s:%s",
				child.Label,
				child.DispatchID,
				childExecutionModeLabel(executionMode),
			),
			"label": child.Label,
		},
		Request: ChildExecutionRequest{
			Label: child.Label,
			Model: child.Model,
		},
	}
}

func childExecutionModeLabel(mode string) string {
	if mode == "" {
		return ChildExecutionModeFake
	}
	return mode
}

// ResumeContextFromCheckpointSummary builds runtime resume facts from one durable
// checkpoint summary and prior runtime records.
func ResumeContextFromCheckpointSummary(
	summary CompletedCheckpointSummary,
	records []RuntimeRecord,
) ResumeContext {
	completed := CompletedChildResultsFromRecords(records)
	if len(summary.CompletedDispatchIDs) > 0 && completed == nil {
		completed = make(map[string]ChildExecutionResult, len(summary.CompletedDispatchIDs))
	}
	for _, dispatchID := range summary.CompletedDispatchIDs {
		if _, ok := completed[dispatchID]; ok {
			continue
		}
		completed[dispatchID] = ChildExecutionResult{
			DispatchID:    dispatchID,
			Status:        ChildDispatchStatusCompleted,
			ExecutionMode: ChildExecutionModeFake,
			Output: map[string]any{
				"text": fmt.Sprintf("replayed:%s", dispatchID),
			},
		}
	}
	return ResumeContext{
		CompletedDispatchIDs:  append([]string(nil), summary.CompletedDispatchIDs...),
		CompletedChildResults: completed,
		CheckpointState:       cloneJSONMap(summary.CheckpointState),
	}
}

// CompletedCheckpointSummary is the minimal checkpoint summary shape consumed by
// the JavaScript runtime resume path.
type CompletedCheckpointSummary struct {
	CompletedDispatchIDs []string
	CheckpointState      map[string]any
}

func cloneJSONMap(value map[string]any) map[string]any {
	if len(value) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(value))
	for key, item := range value {
		cloned[key] = item
	}
	return cloned
}
