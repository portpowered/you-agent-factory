package runtime_api

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestSubmitMultipleRuntimeWorkItemsCompletes exercises customer-facing
// multi-submit admission and dispatch planning publication for concurrent Work.
func TestSubmitMultipleRuntimeWorkItemsCompletes(t *testing.T) {
	dir := scaffoldInvocationFactory(t, nil)
	server := startFunctionalServerWithArgs(
		t,
		dir,
		false,
		nil,
		withWorkerCommands(support.NewStaticSuccessCommandRunner("Done. COMPLETE"), nil),
	)

	submitted := server.SubmitRuntimeWork(t,
		work.SubmitRequest{
			WorkTypeID: "task",
			TraceID:    "trace-multi-1",
			Payload:    []byte(`{"title":"first concurrent item"}`),
		},
		work.SubmitRequest{
			WorkTypeID: "task",
			TraceID:    "trace-multi-2",
			Payload:    []byte(`{"title":"second concurrent item"}`),
		},
	)
	if len(submitted) != 2 {
		t.Fatalf("submitted work count = %d, want 2", len(submitted))
	}

	workIDs := make([]string, len(submitted))
	for index, item := range submitted {
		workIDs[index] = item.WorkID
	}
	waitForGeneratedWorkIDsComplete(t, server.URL(), workIDs, 15*time.Second)

	listed := server.ListWork(t)
	if len(listed.Results) != 2 {
		t.Fatalf("listed work count = %d, want 2", len(listed.Results))
	}
}
