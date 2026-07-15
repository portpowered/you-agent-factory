package runtime

import (
	"context"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/work"

	"github.com/portpowered/infinite-you/pkg/factory"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
)

func TestPause_AlreadyPausedIsNoOp(t *testing.T) {
	h := startServiceModeRunHarness(t,
		factory.WithNet(buildSimpleNet()),
		factory.WithServiceMode(),
		factory.WithInlineDispatch(),
		factory.WithWorkerExecutor("mock", &passExecutor{}),
		factory.WithLogger(logging.NoopLogger{}),
	)
	defer h.stop()

	h.pauseAndWait()

	eventsAfterFirstPause := runtimeGeneratedEvents(t, h.Factory)
	pauseEventsAfterFirst := countFactoryStateEventsTo(t, eventsAfterFirstPause, factoryapi.FactoryStatePaused)

	if err := h.Factory.Pause(context.Background()); err != nil {
		t.Fatalf("second Pause: %v", err)
	}
	if err := h.Factory.Pause(context.Background()); err != nil {
		t.Fatalf("third Pause: %v", err)
	}
	waitForFactoryState(t, h.Factory, interfaces.FactoryStatePaused, time.Second)

	eventsAfterRepeatedPause := runtimeGeneratedEvents(t, h.Factory)
	if got := countFactoryStateEventsTo(t, eventsAfterRepeatedPause, factoryapi.FactoryStatePaused); got != pauseEventsAfterFirst {
		t.Fatalf("pause state events = %d, want %d after repeated pause", got, pauseEventsAfterFirst)
	}
}

func TestServiceMode_RepeatedPausePreservesBufferedSubmission(t *testing.T) {
	h := startServiceModeRunHarness(t,
		factory.WithNet(buildSimpleNet()),
		factory.WithServiceMode(),
		factory.WithInlineDispatch(),
		factory.WithWorkerExecutor("mock", &passExecutor{}),
		factory.WithLogger(logging.NoopLogger{}),
	)
	defer h.stop()

	h.pauseAndWait()
	submitPausedBufferTask(t, h.Factory, "request-repeated-pause-submit-001", "trace-repeated-pause-submit")

	time.Sleep(200 * time.Millisecond)
	if err := h.Factory.Pause(context.Background()); err != nil {
		t.Fatalf("second Pause: %v", err)
	}
	if err := h.Factory.Pause(context.Background()); err != nil {
		t.Fatalf("third Pause: %v", err)
	}
	waitForFactoryState(t, h.Factory, interfaces.FactoryStatePaused, time.Second)
	assertPausedSubmissionNotDone(t, h.Factory)

	h.resumeAndWait()
	waitForWorkAtPlace(t, h.Factory, "task:done", time.Second)
}

func TestServiceMode_RepeatedPausePreservesBufferedWorkerResult(t *testing.T) {
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
		RequestID:  "request-repeated-pause-result-001",
		WorkTypeID: "task",
		TraceID:    "trace-repeated-pause-result",
	}}); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}

	waitForBlockingWorkerStart(t, executor, h.errCh)
	waitForAggregateSnapshot(t, h.Factory, func(snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) bool {
		return snapshot.InFlightCount > 0
	})

	h.pauseAndWait()
	close(executor.release)
	time.Sleep(200 * time.Millisecond)

	if err := h.Factory.Pause(context.Background()); err != nil {
		t.Fatalf("second Pause: %v", err)
	}
	if err := h.Factory.Pause(context.Background()); err != nil {
		t.Fatalf("third Pause: %v", err)
	}
	waitForFactoryState(t, h.Factory, interfaces.FactoryStatePaused, time.Second)
	assertPausedWorkerResultNotDone(t, h.Factory)

	h.resumeAndWait()
	waitForWorkAtPlace(t, h.Factory, "task:done", time.Second)
	assertNoInFlightDispatches(t, h.Factory)
}

func countFactoryStateEventsTo(t *testing.T, events []factoryapi.FactoryEvent, want factoryapi.FactoryState) int {
	t.Helper()
	count := 0
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeFactoryStateResponse {
			continue
		}
		payload, err := event.Payload.AsFactoryStateResponseEventPayload()
		if err != nil {
			t.Fatalf("decode factory state event: %v", err)
		}
		if payload.State == want {
			count++
		}
	}
	return count
}
