package workflowruntime_test

import (
	"testing"

	workflowruntime "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/runtime"
)

func assertParallelResultOrder(t *testing.T, sessionID string, outcome workflowruntime.Outcome, wantLabels []string) {
	t.Helper()
	results, ok := projectPrimaryJSON(t, sessionID, outcome.Value)["results"].([]any)
	if !ok || len(results) != len(wantLabels) {
		t.Fatalf("projected results = %#v, want %d entries", results, len(wantLabels))
	}
	for i, wantLabel := range wantLabels {
		child, ok := results[i].(map[string]any)
		if !ok || child["label"] != wantLabel || child["status"] != workflowruntime.ChildDispatchStatusCompleted {
			t.Fatalf("results[%d] = %#v, want completed child %q", i, results[i], wantLabel)
		}
	}
}

func peakConcurrentChildRunning(records []workflowruntime.RuntimeRecord) int {
	running, peak := 0, 0
	for _, record := range records {
		if record.Kind != workflowruntime.RecordKindChildDispatch || record.ChildDispatch == nil {
			continue
		}
		switch record.ChildDispatch.Status {
		case workflowruntime.ChildDispatchStatusRunning:
			running++
			if running > peak {
				peak = running
			}
		case workflowruntime.ChildDispatchStatusCompleted, workflowruntime.ChildDispatchStatusFailed:
			running--
		}
	}
	return peak
}

func childCompletionOrder(records []workflowruntime.RuntimeRecord) []string {
	var order []string
	for _, record := range records {
		if record.Kind == workflowruntime.RecordKindChildDispatch && record.ChildDispatch != nil && record.ChildDispatch.Status == workflowruntime.ChildDispatchStatusCompleted && record.ChildDispatch.Label != "" {
			order = append(order, record.ChildDispatch.Label)
		}
	}
	return order
}

func hasChildDispatchStatus(records []workflowruntime.RuntimeRecord, status string) bool {
	for _, record := range records {
		if record.Kind == workflowruntime.RecordKindChildDispatch && record.ChildDispatch != nil && record.ChildDispatch.Status == status {
			return true
		}
	}
	return false
}

func completedForLabel(records []workflowruntime.RuntimeRecord, label string) bool {
	for _, record := range records {
		if record.Kind == workflowruntime.RecordKindChildDispatch && record.ChildDispatch != nil && record.ChildDispatch.Label == label && record.ChildDispatch.Status == workflowruntime.ChildDispatchStatusCompleted {
			return true
		}
	}
	return false
}
