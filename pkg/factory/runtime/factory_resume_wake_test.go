package runtime

import (
	"context"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
)

func TestResumeWakesOneBufferedSubmissionWhilePaused(t *testing.T) {
	f, err := New(
		factory.WithNet(buildSimpleNet()),
		factory.WithServiceMode(),
		factory.WithInlineDispatch(),
		factory.WithWorkerExecutor("mock", &passExecutor{}),
		factory.WithLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx := context.Background()
	runCtx, cancelRun := context.WithCancel(ctx)
	errCh := make(chan error, 1)
	go func() {
		errCh <- f.Run(runCtx)
	}()

	select {
	case err := <-errCh:
		t.Fatalf("Run returned before pause/resume scenario: %v", err)
	case <-time.After(150 * time.Millisecond):
	}

	if err := f.Pause(ctx); err != nil {
		t.Fatalf("Pause: %v", err)
	}

	if _, err := submitWorkRequests(ctx, f, []interfaces.SubmitRequest{{
		WorkID:     "work-resume-wake",
		WorkTypeID: "task",
		TraceID:    "trace-resume-wake",
	}}); err != nil {
		t.Fatalf("SubmitWorkRequest while paused: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	snapBeforeResume, err := f.GetEngineStateSnapshot(ctx)
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot before resume: %v", err)
	}
	if markingContainsWorkAtPlace(&snapBeforeResume.Marking, "work-resume-wake", "task:done") {
		t.Fatalf("marking = %#v, want buffered work to remain unprocessed before resume", snapBeforeResume.Marking.Tokens)
	}

	if err := f.Resume(ctx); err != nil {
		t.Fatalf("Resume: %v", err)
	}

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		snap, err := f.GetEngineStateSnapshot(ctx)
		if err != nil {
			t.Fatalf("GetEngineStateSnapshot after resume: %v", err)
		}
		if markingContainsWorkAtPlace(&snap.Marking, "work-resume-wake", "task:done") {
			if snap.FactoryState != string(interfaces.FactoryStateRunning) {
				t.Fatalf("factory state = %q, want %q after resume", snap.FactoryState, interfaces.FactoryStateRunning)
			}
			cancelRun()
			select {
			case err := <-errCh:
				if err != nil {
					t.Fatalf("Run after cancellation: %v", err)
				}
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for service-mode runtime to stop after resume drain")
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	cancelRun()
	<-errCh
	t.Fatal("buffered submission did not reach task:done after resume without a post-resume external signal")
}

func TestResumeWakesOneBufferedWorkerResultWhilePaused(t *testing.T) {
	executor := &blockingExecutor{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	f, err := New(
		factory.WithNet(buildSimpleNet()),
		factory.WithServiceMode(),
		factory.WithWorkerExecutor("mock", executor),
		factory.WithLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx := context.Background()
	runCtx, cancelRun := context.WithCancel(ctx)
	errCh := make(chan error, 1)
	go func() {
		errCh <- f.Run(runCtx)
	}()

	if _, err := submitWorkRequests(ctx, f, []interfaces.SubmitRequest{{
		WorkID:     "work-resume-result",
		WorkTypeID: "task",
		TraceID:    "trace-resume-result",
	}}); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}

	select {
	case <-executor.started:
	case err := <-errCh:
		t.Fatalf("Run returned before worker started: %v", err)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for worker to start")
	}

	if err := f.Pause(ctx); err != nil {
		t.Fatalf("Pause: %v", err)
	}

	close(executor.release)

	time.Sleep(100 * time.Millisecond)

	snapBeforeResume, err := f.GetEngineStateSnapshot(ctx)
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot before resume: %v", err)
	}
	if markingContainsWorkAtPlace(&snapBeforeResume.Marking, "work-resume-result", "task:done") {
		t.Fatalf("marking = %#v, want buffered worker result to remain unprocessed before resume", snapBeforeResume.Marking.Tokens)
	}

	if err := f.Resume(ctx); err != nil {
		t.Fatalf("Resume: %v", err)
	}

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		snap, err := f.GetEngineStateSnapshot(ctx)
		if err != nil {
			t.Fatalf("GetEngineStateSnapshot after resume: %v", err)
		}
		if markingContainsWorkAtPlace(&snap.Marking, "work-resume-result", "task:done") {
			cancelRun()
			select {
			case err := <-errCh:
				if err != nil {
					t.Fatalf("Run after cancellation: %v", err)
				}
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for service-mode runtime to stop after resume drain")
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	cancelRun()
	<-errCh
	t.Fatal("buffered worker result did not reach task:done after resume without a post-resume external signal")
}
