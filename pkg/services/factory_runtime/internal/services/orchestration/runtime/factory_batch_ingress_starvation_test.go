package runtime

import (
	"context"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	"github.com/portpowered/infinite-you/internal/testutil/recordingfixtures"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

func TestConcurrentBatchIngressProjectsWhileDispatchBlocked_ServiceModeWorkerPool(t *testing.T) {
	t.Helper()

	executor := &blockingExecutor{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	history := &recordingfixtures.ScriptedRuntimeLedger{}

	opts := []testFactoryOption{
		withNet(buildSimpleNet()),
		withServiceMode(),
		withWorkerExecutor("mock", executor),
		withFactoryEventHistory(history),
		withLogger(logging.NoopLogger{}),
	}

	h := startServiceModeRunHarness(t, opts...)

	if _, err := submitWorkRequests(context.Background(), h.Factory, []work.SubmitRequest{{
		WorkID:     "work-blocked-dispatch",
		WorkTypeID: "task",
		TraceID:    "trace-blocked-dispatch",
	}}); err != nil {
		t.Fatalf("submit initial work: %v", err)
	}

	waitForAggregateSnapshot(t, h.Factory, func(snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) bool {
		return snapshot.InFlightCount > 0
	})

	select {
	case <-executor.started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for blocking dispatch to start")
	}

	batchRequest := work.WorkRequest{
		RequestID: "request-concurrent-ingress",
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		Works: []work.Work{{
			Name:       "concurrent-ingress",
			WorkID:     "work-concurrent-ingress",
			WorkTypeID: "task",
			TraceID:    "trace-concurrent-ingress",
		}},
	}
	if _, err := h.Factory.SubmitWorkRequest(context.Background(), batchRequest); err != nil {
		t.Fatalf("SubmitWorkRequest concurrent batch: %v", err)
	}

	if !workRequestRecorded(history, batchRequest.RequestID) {
		t.Fatalf("WORK_REQUEST not recorded before submit returned; work requests=%#v", history.WorkRequests)
	}
	snap, err := h.Factory.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if !snapshotObservesWork(snap, "work-concurrent-ingress") {
		t.Fatalf(
			"submit returned before work became observable; work requests=%#v marking tokens=%#v",
			history.WorkRequests,
			snap.Marking.Tokens,
		)
	}

	close(executor.release)
	h.cancel()
	select {
	case <-h.errCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for service-mode runtime to stop")
	}
}

func workRequestRecorded(history *recordingfixtures.ScriptedRuntimeLedger, requestID string) bool {
	for _, record := range history.WorkRequests {
		if record.RequestID == requestID {
			return true
		}
	}
	return false
}

func snapshotObservesWork(snap *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], workID string) bool {
	if snap == nil {
		return false
	}
	if markingContainsWorkAtPlace(&snap.Marking, workID, "task:init") {
		return true
	}
	for _, entry := range snap.Dispatches {
		for _, token := range entry.ConsumedTokens {
			if token.Color.WorkID == workID {
				return true
			}
		}
	}
	return false
}
