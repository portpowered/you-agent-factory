package runtime

import (
	"context"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/state"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

func TestServiceMode_WorkerResultWhilePaused_BuffersUntilResume(t *testing.T) {
	executor := &blockingExecutor{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	h := startServiceModeRunHarness(t,
		withNet(buildSimpleNet()),
		withServiceMode(),
		withWorkerExecutor("mock", executor),
		withLogger(logging.NoopLogger{}),
	)
	defer h.stop()
	buffered := observeNextBufferedResult(t, h.Factory)

	if _, err := submitWorkRequests(context.Background(), h.Factory, []work.SubmitRequest{{
		RequestID:  "request-runtime-paused-result-001",
		WorkTypeID: "task",
		TraceID:    "trace-runtime-paused-result",
	}}); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}

	waitForBlockingWorkerStart(t, executor, h.errCh)
	waitForAggregateSnapshot(t, h.Factory, func(snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) bool {
		return snapshot.InFlightCount > 0 && len(snapshot.Dispatches) > 0
	})

	h.pauseAndWait()
	close(executor.release)
	waitForBufferedResult(t, buffered)
	assertPausedWorkerResultBuffered(t, h.Factory)

	h.resumeAndWait()
	waitForWorkAtPlace(t, h.Factory, "task:done", time.Second)
	assertNoInFlightDispatches(t, h.Factory)
	assertTaskDoneOnce(t, h.Factory)
}
