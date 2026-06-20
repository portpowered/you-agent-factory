package runtime

import (
	"context"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
	"github.com/portpowered/infinite-you/pkg/petri"
)

type gatedMultiWorkerExecutor struct {
	started chan struct{}
	release chan struct{}
}

func (e *gatedMultiWorkerExecutor) Execute(_ context.Context, dispatch interfaces.WorkDispatch) (interfaces.WorkResult, error) {
	e.started <- struct{}{}
	<-e.release
	return interfaces.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      interfaces.OutcomeAccepted,
		Output:       "done",
	}, nil
}

func waitForWorkerStarts(t *testing.T, started <-chan struct{}, count int) {
	t.Helper()
	for i := 0; i < count; i++ {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for worker start %d/%d", i+1, count)
		}
	}
}

func workIDsFromDispatchHistory(history []interfaces.CompletedDispatch) []string {
	workIDs := make([]string, 0, len(history))
	for _, completed := range history {
		for _, token := range completed.ConsumedTokens {
			if token.Color.WorkID != "" {
				workIDs = append(workIDs, token.Color.WorkID)
				break
			}
		}
	}
	return workIDs
}

func dispatchHistoryContainsWorkIDs(t *testing.T, history []interfaces.CompletedDispatch, wantWorkIDs []string) {
	t.Helper()
	got := workIDsFromDispatchHistory(history)
	if len(got) != len(wantWorkIDs) {
		t.Fatalf("dispatch history count = %d, want %d: %#v", len(got), len(wantWorkIDs), history)
	}
	seen := make(map[string]int, len(got))
	for _, workID := range got {
		seen[workID]++
	}
	for _, workID := range wantWorkIDs {
		if seen[workID] != 1 {
			t.Fatalf("dispatch history work IDs = %v, want each of %v exactly once", got, wantWorkIDs)
		}
	}
}

func allWorksAtDonePlace(marking *petri.MarkingSnapshot, workIDs []string) bool {
	for _, workID := range workIDs {
		if !markingContainsWorkAtPlace(marking, workID, "task:done") {
			return false
		}
	}
	return true
}

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

func TestResumeDrainsMultipleBufferedSubmissionsToQuiescenceWhilePaused(t *testing.T) {
	workIDs := []string{"work-resume-drain-a", "work-resume-drain-b", "work-resume-drain-c"}
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

	for _, workID := range workIDs {
		if _, err := submitWorkRequests(ctx, f, []interfaces.SubmitRequest{{
			WorkID:     workID,
			WorkTypeID: "task",
			TraceID:    "trace-" + workID,
		}}); err != nil {
			t.Fatalf("SubmitWorkRequest for %q while paused: %v", workID, err)
		}
	}

	time.Sleep(100 * time.Millisecond)

	snapBeforeResume, err := f.GetEngineStateSnapshot(ctx)
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot before resume: %v", err)
	}
	for _, workID := range workIDs {
		if markingContainsWorkAtPlace(&snapBeforeResume.Marking, workID, "task:done") {
			t.Fatalf("marking = %#v, want buffered work %q to remain unprocessed before resume", snapBeforeResume.Marking.Tokens, workID)
		}
	}

	if err := f.Resume(ctx); err != nil {
		t.Fatalf("Resume: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		snap, err := f.GetEngineStateSnapshot(ctx)
		if err != nil {
			t.Fatalf("GetEngineStateSnapshot after resume: %v", err)
		}
		if allWorksAtDonePlace(&snap.Marking, workIDs) && snap.InFlightCount == 0 {
			gotOrder := workIDsFromDispatchHistory(snap.DispatchHistory)
			for i, wantWorkID := range workIDs {
				if gotOrder[i] != wantWorkID {
					t.Fatalf("dispatch history order = %v, want %v", gotOrder, workIDs)
				}
			}
			if snap.FactoryState != string(interfaces.FactoryStateRunning) {
				t.Fatalf("factory state = %q, want %q after resume drain", snap.FactoryState, interfaces.FactoryStateRunning)
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
	t.Fatal("buffered submissions did not drain to quiescence after resume without a post-resume external signal")
}

func TestResumeDrainsMultipleBufferedWorkerResultsToQuiescenceWhilePaused(t *testing.T) {
	workIDs := []string{"work-resume-result-a", "work-resume-result-b", "work-resume-result-c"}
	executor := &gatedMultiWorkerExecutor{
		started: make(chan struct{}, len(workIDs)),
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

	for _, workID := range workIDs {
		if _, err := submitWorkRequests(ctx, f, []interfaces.SubmitRequest{{
			WorkID:     workID,
			WorkTypeID: "task",
			TraceID:    "trace-" + workID,
		}}); err != nil {
			t.Fatalf("SubmitWorkRequest for %q: %v", workID, err)
		}
	}

	waitForWorkerStarts(t, executor.started, len(workIDs))

	if err := f.Pause(ctx); err != nil {
		t.Fatalf("Pause: %v", err)
	}

	close(executor.release)

	time.Sleep(100 * time.Millisecond)

	snapBeforeResume, err := f.GetEngineStateSnapshot(ctx)
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot before resume: %v", err)
	}
	for _, workID := range workIDs {
		if markingContainsWorkAtPlace(&snapBeforeResume.Marking, workID, "task:done") {
			t.Fatalf("marking = %#v, want buffered worker result for %q to remain unprocessed before resume", snapBeforeResume.Marking.Tokens, workID)
		}
	}

	if err := f.Resume(ctx); err != nil {
		t.Fatalf("Resume: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		snap, err := f.GetEngineStateSnapshot(ctx)
		if err != nil {
			t.Fatalf("GetEngineStateSnapshot after resume: %v", err)
		}
		if allWorksAtDonePlace(&snap.Marking, workIDs) && snap.InFlightCount == 0 {
			dispatchHistoryContainsWorkIDs(t, snap.DispatchHistory, workIDs)
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
	t.Fatal("buffered worker results did not drain to quiescence after resume without a post-resume external signal")
}

func TestResumeOnRunningFactoryIsAcceptedNoOp(t *testing.T) {
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
		t.Fatalf("Run returned before resume no-op scenario: %v", err)
	case <-time.After(150 * time.Millisecond):
	}

	snapBefore, err := f.GetEngineStateSnapshot(ctx)
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot before resume: %v", err)
	}
	if snapBefore.FactoryState != string(interfaces.FactoryStateRunning) {
		t.Fatalf("factory state = %q, want %q before resume no-op", snapBefore.FactoryState, interfaces.FactoryStateRunning)
	}

	if err := f.Resume(ctx); err != nil {
		t.Fatalf("Resume on running factory: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	snapAfter, err := f.GetEngineStateSnapshot(ctx)
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot after resume: %v", err)
	}
	if snapAfter.FactoryState != string(interfaces.FactoryStateRunning) {
		t.Fatalf("factory state = %q, want %q after resume no-op", snapAfter.FactoryState, interfaces.FactoryStateRunning)
	}
	if len(snapAfter.DispatchHistory) != len(snapBefore.DispatchHistory) {
		t.Fatalf("dispatch history count = %d, want unchanged %d after resume no-op", len(snapAfter.DispatchHistory), len(snapBefore.DispatchHistory))
	}
	if snapAfter.InFlightCount != snapBefore.InFlightCount {
		t.Fatalf("in-flight count = %d, want unchanged %d after resume no-op", snapAfter.InFlightCount, snapBefore.InFlightCount)
	}

	cancelRun()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Run after cancellation: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for service-mode runtime to stop after resume no-op")
	}
}

func TestResumeRepeatedAfterBufferedDrainDoesNotReprocessWork(t *testing.T) {
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
		WorkID:     "work-resume-repeat",
		WorkTypeID: "task",
		TraceID:    "trace-resume-repeat",
	}}); err != nil {
		t.Fatalf("SubmitWorkRequest while paused: %v", err)
	}

	if err := f.Resume(ctx); err != nil {
		t.Fatalf("Resume: %v", err)
	}

	deadline := time.Now().Add(time.Second)
	var drainedDispatchCount int
	for time.Now().Before(deadline) {
		snap, err := f.GetEngineStateSnapshot(ctx)
		if err != nil {
			t.Fatalf("GetEngineStateSnapshot after first resume: %v", err)
		}
		if markingContainsWorkAtPlace(&snap.Marking, "work-resume-repeat", "task:done") && snap.InFlightCount == 0 {
			drainedDispatchCount = len(snap.DispatchHistory)
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if drainedDispatchCount == 0 {
		cancelRun()
		<-errCh
		t.Fatal("buffered submission did not drain after first resume")
	}

	if err := f.Resume(ctx); err != nil {
		t.Fatalf("second Resume after drain: %v", err)
	}
	if err := f.Resume(ctx); err != nil {
		t.Fatalf("third Resume after drain: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	snapAfter, err := f.GetEngineStateSnapshot(ctx)
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot after repeated resume: %v", err)
	}
	if snapAfter.FactoryState != string(interfaces.FactoryStateRunning) {
		t.Fatalf("factory state = %q, want %q after repeated resume", snapAfter.FactoryState, interfaces.FactoryStateRunning)
	}
	if len(snapAfter.DispatchHistory) != drainedDispatchCount {
		t.Fatalf("dispatch history count = %d, want unchanged %d after repeated resume", len(snapAfter.DispatchHistory), drainedDispatchCount)
	}
	dispatchHistoryContainsWorkIDs(t, snapAfter.DispatchHistory, []string{"work-resume-repeat"})
	if !markingContainsWorkAtPlace(&snapAfter.Marking, "work-resume-repeat", "task:done") {
		t.Fatalf("marking = %#v, want work-resume-repeat at task:done after repeated resume", snapAfter.Marking.Tokens)
	}
	if snapAfter.InFlightCount != 0 {
		t.Fatalf("in-flight count = %d, want 0 after repeated resume", snapAfter.InFlightCount)
	}

	cancelRun()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Run after cancellation: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for service-mode runtime to stop after repeated resume")
	}
}

func TestResumeRepeatedWhilePausedWakesBufferedWorkOnce(t *testing.T) {
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
		WorkID:     "work-resume-double",
		WorkTypeID: "task",
		TraceID:    "trace-resume-double",
	}}); err != nil {
		t.Fatalf("SubmitWorkRequest while paused: %v", err)
	}

	if err := f.Resume(ctx); err != nil {
		t.Fatalf("first Resume: %v", err)
	}
	if err := f.Resume(ctx); err != nil {
		t.Fatalf("second Resume: %v", err)
	}

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		snap, err := f.GetEngineStateSnapshot(ctx)
		if err != nil {
			t.Fatalf("GetEngineStateSnapshot after repeated resume: %v", err)
		}
		if markingContainsWorkAtPlace(&snap.Marking, "work-resume-double", "task:done") && snap.InFlightCount == 0 {
			dispatchHistoryContainsWorkIDs(t, snap.DispatchHistory, []string{"work-resume-double"})
			if snap.FactoryState != string(interfaces.FactoryStateRunning) {
				t.Fatalf("factory state = %q, want %q after repeated resume", snap.FactoryState, interfaces.FactoryStateRunning)
			}
			cancelRun()
			select {
			case err := <-errCh:
				if err != nil {
					t.Fatalf("Run after cancellation: %v", err)
				}
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for service-mode runtime to stop after repeated resume")
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	cancelRun()
	<-errCh
	t.Fatal("buffered submission was not processed exactly once after repeated resume while paused")
}
