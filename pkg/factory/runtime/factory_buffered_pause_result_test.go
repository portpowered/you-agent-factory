package runtime

import (
	"context"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/factory"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/work"
)

func TestServiceMode_WorkerResultWhilePaused_BuffersUntilResume(t *testing.T) {
	executor := &blockingExecutor{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	h := startServiceModeRunHarness(t,
		factory.WithNet(buildSimpleNet()),
		factory.WithServiceMode(),
		factory.WithWorkerExecutor("mock", executor),
		factory.WithLogger(logging.NoopLogger{}),
	)
	defer h.stop()

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
	assertPausedWorkerResultBuffered(t, h.Factory, 500*time.Millisecond)

	h.resumeAndWait()
	waitForWorkAtPlace(t, h.Factory, "task:done", time.Second)
	assertNoInFlightDispatches(t, h.Factory)
	assertTaskDoneOnce(t, h.Factory)
}
