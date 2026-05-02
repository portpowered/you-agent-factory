package submission

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestWorkRequestFromSubmitRequests_UsesSharedTraceFallback(t *testing.T) {
	workRequest := WorkRequestFromSubmitRequests([]interfaces.SubmitRequest{{
		Name:       "draft",
		WorkID:     "work-1",
		WorkTypeID: "task",
		TraceID:    "trace-legacy",
	}})

	if workRequest.CurrentChainingTraceID != "trace-legacy" {
		t.Fatalf("work request current chaining trace ID = %q, want trace-legacy", workRequest.CurrentChainingTraceID)
	}
	if len(workRequest.Works) != 1 {
		t.Fatalf("work count = %d, want 1", len(workRequest.Works))
	}
	if workRequest.Works[0].CurrentChainingTraceID != "trace-legacy" {
		t.Fatalf("work current chaining trace ID = %q, want trace-legacy", workRequest.Works[0].CurrentChainingTraceID)
	}
	if workRequest.Works[0].TraceID != "trace-legacy" {
		t.Fatalf("work trace ID = %q, want trace-legacy", workRequest.Works[0].TraceID)
	}
}
