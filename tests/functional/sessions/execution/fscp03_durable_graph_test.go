package execution_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func runFSCP03SequentialDurableIdentity(t *testing.T, canonical fscp03CanonicalOperations) {
	t.Helper()
	first := fscp03StartSynchronous(t, canonical, "fscp03-sequential-first", fscp03ChildSource("sequential-first"))
	second := fscp03StartSynchronous(t, canonical, "fscp03-sequential-second", fscp03ChildSource("sequential-second"))
	if first.SessionID == second.SessionID {
		t.Fatalf("sequential session ids = %q and %q, want distinct", first.SessionID, second.SessionID)
	}
	assertFSCP03SuccessfulStart(t, first)
	assertFSCP03SuccessfulStart(t, second)
	assertFSCP03DurableLineage(t, canonical, first.SessionID)
	assertFSCP03DurableLineage(t, canonical, second.SessionID)
	assertFSCP03DisjointDurableObservations(t, canonical, first.SessionID, second.SessionID)
}

func runFSCP03DurableDirection(t *testing.T, canonical fscp03CanonicalOperations, legacy factorysessions.DurableExecutionService) {
	t.Helper()
	canonicalDirection := fscp03StartSynchronous(t, canonical, "fscp03-canonical-direction", fscp03InlineSource(`return "fscp03 direction COMPLETE";`))
	legacyDirection, err := legacy.StartSync(t.Context(), factorysessions.StartRequest{
		RequestID: "fscp03-legacy-direction",
		Source:    fscp03InlineSource(`return "fscp03 direction COMPLETE";`),
	})
	if err != nil {
		t.Fatalf("legacy durable StartSync() error = %v", err)
	}
	if canonicalDirection.SessionID == legacyDirection.SessionID {
		t.Fatalf("canonical/legacy session id = %q, want distinct executions", canonicalDirection.SessionID)
	}
	assertFSCP03SuccessfulStart(t, canonicalDirection)
	if legacyDirection.Status != string(factorysessions.LifecycleStatusSucceeded) || legacyDirection.SyncOutcome != factorysessions.SyncOutcome("COMPLETED") {
		t.Fatalf("legacy direction result = %#v, want SUCCEEDED/COMPLETED", legacyDirection)
	}
	legacyView := fscp03GetDurableSession(t, legacy, legacyDirection.SessionID)
	if legacyView.Status != string(factorysessions.LifecycleStatusSucceeded) || legacyView.OrchestratorKind != "JAVASCRIPT" || legacyView.SourceRef != "inline" || !legacyView.HasResultSummary || legacyView.ResultStatus != string(factorysessions.ResultStatusFinal) {
		t.Fatalf("legacy direction view = %#v, want equivalent durable success semantics", legacyView)
	}
	canonicalView := fscp03GetDurableSession(t, legacy, canonicalDirection.SessionID)
	if canonicalView.Status != legacyView.Status || canonicalView.OrchestratorKind != legacyView.OrchestratorKind || canonicalView.SourceRef != legacyView.SourceRef || !canonicalView.HasResultSummary || !legacyView.HasResultSummary || canonicalView.ResultStatus != legacyView.ResultStatus {
		t.Fatalf("canonical view = %#v, legacy view = %#v, want equivalent public projections", canonicalView, legacyView)
	}
}

type fscp03DurableSessionFacts struct {
	Status           string
	OrchestratorKind string
	SourceRef        string
	HasResultSummary bool
	ResultStatus     string
	HasFailure       bool
	FailureMessage   string
}

func fscp03GetDurableSession(t *testing.T, legacy factorysessions.DurableExecutionService, sessionID string) fscp03DurableSessionFacts {
	t.Helper()
	view, err := legacy.GetSession(t.Context(), sessionID)
	if err != nil {
		t.Fatalf("durable GetSession(%s) error = %v", sessionID, err)
	}
	facts := fscp03DurableSessionFacts{
		Status:           string(view.Status),
		OrchestratorKind: view.OrchestratorKind,
		SourceRef:        view.ResolvedSource.SourceRef,
		HasResultSummary: view.ResultSummary != nil,
		HasFailure:       view.Failure != nil,
	}
	if view.ResultSummary != nil {
		facts.ResultStatus = view.ResultSummary.ResultStatus
	}
	if view.Failure != nil {
		facts.FailureMessage = view.Failure.Message
	}
	return facts
}

func runFSCP03ConcurrentDurableIdentity(t *testing.T, factoryDir string) {
	t.Helper()
	barrier := newFSCP03BarrierRunner()
	concurrentProcess, err := root.BuildProcess(t.Context(), serviceedges.Edges{
		BrowserOpener:         func(context.Context, string) error { return nil },
		ProviderCommandRunner: barrier,
	})
	if err != nil {
		t.Fatalf("concurrent root.BuildProcess() error = %v", err)
	}
	support.CleanupProcess(t, concurrentProcess)
	concurrentOpened, concurrentCanonical := openFSCP03Execution(t, concurrentProcess, concurrentProcess.ExecutionRuntimeOpening(), factoryDir, t.TempDir(), "fscp03-concurrent-owner")
	runFSCP03ConcurrentStarts(t, concurrentCanonical, barrier)
	runFSCP03FailedStartRecovery(t, concurrentOpened, concurrentCanonical, barrier)
}

func runFSCP03ConcurrentStarts(t *testing.T, canonical fscp03CanonicalOperations, barrier *fscp03BarrierRunner) {
	t.Helper()
	results := make(chan fscp03StartOutcome, 2)
	for _, label := range []string{"concurrent-a", "concurrent-b"} {
		label := label
		go func() {
			result, err := canonical.Start(t.Context(), factorysessions.SessionStartRequest{
				Mode:        factorysessions.SessionOperationModeDurable,
				Correlation: factorysessions.SessionOperationCorrelation{RequestID: "fscp03-" + label},
				Source:      fscp03ChildSource(label),
				Synchronous: true,
			})
			results <- fscp03StartOutcome{result: result, err: err}
		}()
	}
	if err := barrier.WaitStarted(t.Context(), 2); err != nil {
		t.Fatalf("concurrent starts did not overlap at provider edge: %v", err)
	}
	barrier.Release()
	concurrent := make([]factorysessions.SessionStartResult, 0, 2)
	for range 2 {
		select {
		case outcome := <-results:
			if outcome.err != nil {
				t.Fatalf("concurrent canonical Start() error = %v", outcome.err)
			}
			concurrent = append(concurrent, outcome.result)
		case <-time.After(fscp03ObservationTimeout):
			t.Fatal("concurrent canonical starts did not complete after provider release")
		}
	}
	if concurrent[0].SessionID == concurrent[1].SessionID {
		t.Fatalf("concurrent session ids = %q and %q, want distinct", concurrent[0].SessionID, concurrent[1].SessionID)
	}
	for _, result := range concurrent {
		assertFSCP03SuccessfulStart(t, result)
		assertFSCP03DurableLineage(t, canonical, result.SessionID)
	}
	assertFSCP03DisjointDurableObservations(t, canonical, concurrent[0].SessionID, concurrent[1].SessionID)
}

func runFSCP03FailedStartRecovery(t *testing.T, opened factorysessions.OpenedExecutionRuntime, canonical fscp03CanonicalOperations, barrier *fscp03BarrierRunner) {
	t.Helper()
	barrier.Reset()
	activeResults := make(chan fscp03StartOutcome, 1)
	go func() {
		result, err := canonical.Start(t.Context(), factorysessions.SessionStartRequest{
			Mode:        factorysessions.SessionOperationModeDurable,
			Correlation: factorysessions.SessionOperationCorrelation{RequestID: "fscp03-failure-peer"},
			Source:      fscp03ChildSource("failure-peer"),
			Synchronous: true,
		})
		activeResults <- fscp03StartOutcome{result: result, err: err}
	}()
	if err := barrier.WaitStarted(t.Context(), 1); err != nil {
		t.Fatalf("active peer did not reach controlled provider: %v", err)
	}
	failed, err := canonical.Start(t.Context(), factorysessions.SessionStartRequest{
		Mode:        factorysessions.SessionOperationModeDurable,
		Correlation: factorysessions.SessionOperationCorrelation{RequestID: "fscp03-controlled-failure"},
		Source:      fscp03InlineSource(`throw new Error("fscp03 controlled failure");`),
		Synchronous: true,
	})
	if err != nil {
		t.Fatalf("controlled failed Start() error = %v, want terminal failed projection", err)
	}
	if failed.Status != string(factorysessions.LifecycleStatusFailed) {
		t.Fatalf("failed start status = %q, want FAILED", failed.Status)
	}
	failedView := fscp03GetDurableSession(t, opened.Execution.(factorysessions.DurableExecutionService), failed.SessionID)
	if failedView.Status != string(factorysessions.LifecycleStatusFailed) || !failedView.HasFailure || !strings.Contains(failedView.FailureMessage, "fscp03 controlled failure") {
		t.Fatalf("failed durable view = %#v, want original typed failure", failedView)
	}
	failedResult, err := canonical.ReadResult(t.Context(), factorysessions.SessionResultReadRequest{
		SessionID: failed.SessionID,
		Mode:      factorysessions.SessionOperationModeDurable,
		Request:   factorysessions.ResultRequest{Mode: factorysessions.ResultModeFinal},
	})
	if err != nil {
		t.Fatalf("failed ReadResult() error = %v", err)
	}
	if failedResult.Status != string(factorysessions.ResultStatusUnavailable) || failedResult.Durable == nil || failedResult.Durable.SessionStatus != factorysessions.LifecycleStatusFailed {
		t.Fatalf("failed result = %#v durable=%#v, want unavailable result with original failure", failedResult, failedResult.Durable)
	}
	barrier.Release()
	active := <-activeResults
	if active.err != nil {
		t.Fatalf("active peer after failed start error = %v", active.err)
	}
	assertFSCP03SuccessfulStart(t, active.result)
	recovery := fscp03StartSynchronous(t, canonical, "fscp03-after-failure", fscp03ChildSource("after-failure"))
	assertFSCP03SuccessfulStart(t, recovery)
	if recovery.SessionID == failed.SessionID || recovery.SessionID == active.result.SessionID {
		t.Fatalf("recovery session id = %q, want a fresh usable session", recovery.SessionID)
	}
	assertFSCP03DurableLineage(t, canonical, recovery.SessionID)
}

func runFSCP03Cancel(t *testing.T, canonical fscp03CanonicalOperations, runner *fscp03ControlRunner) {
	t.Helper()
	canceled := fscp03StartBlockedAsync(t, canonical, runner.started, "fscp03-cancel")
	control, err := canonical.Control(t.Context(), factorysessions.SessionControlRequest{
		SessionID: canceled.SessionID, Mode: factorysessions.SessionOperationModeDurable,
		Operation: factorysessions.SessionControlCancel, Control: factorysessions.ControlRequest{RequestID: "fscp03-cancel-control"},
	})
	if err != nil {
		t.Fatalf("canonical CANCEL error = %v", err)
	}
	if control.Operation != factorysessions.SessionControlCancel || control.Outcome != factorysessions.LifecycleControlOutcomeAccepted || control.Status != factorysessions.LifecycleStatusCanceling {
		t.Fatalf("CANCEL result = %#v, want accepted CANCEL/CANCELING", control)
	}
	assertFSCP03DurableStatus(t, canonical, canceled.SessionID, factorysessions.LifecycleStatusCanceled)
}

func runFSCP03Terminate(t *testing.T, canonical fscp03CanonicalOperations, runner *fscp03ControlRunner) {
	t.Helper()
	terminated := fscp03StartBlockedAsync(t, canonical, runner.started, "fscp03-terminate")
	control, err := canonical.Control(t.Context(), factorysessions.SessionControlRequest{
		SessionID: terminated.SessionID, Mode: factorysessions.SessionOperationModeDurable,
		Operation: factorysessions.SessionControlTerminate, Control: factorysessions.ControlRequest{RequestID: "fscp03-terminate-control"},
	})
	if err != nil {
		t.Fatalf("canonical TERMINATE error = %v", err)
	}
	if control.Operation != factorysessions.SessionControlTerminate || control.Outcome != factorysessions.LifecycleControlOutcomeAccepted || control.Status != factorysessions.LifecycleStatusTerminated {
		t.Fatalf("TERMINATE result = %#v, want accepted TERMINATE/TERMINATED", control)
	}
	assertFSCP03DurableTerminalStatus(t, canonical, terminated.SessionID)
}

func runFSCP03Close(t *testing.T, canonical fscp03CanonicalOperations, factoryDir string) {
	t.Helper()
	live, err := canonical.Start(t.Context(), factorysessions.SessionStartRequest{Mode: factorysessions.SessionOperationModeLive, FolderPath: factoryDir})
	if err != nil {
		t.Fatalf("live Start() for CLOSE error = %v", err)
	}
	closed, err := canonical.Control(t.Context(), factorysessions.SessionControlRequest{
		SessionID: live.SessionID, Mode: factorysessions.SessionOperationModeLive,
		Operation: factorysessions.SessionControlClose, Control: factorysessions.ControlRequest{RequestID: "fscp03-close-control"},
	})
	if err != nil {
		t.Fatalf("canonical CLOSE error = %v", err)
	}
	if closed.Operation != factorysessions.SessionControlClose || !closed.Closed {
		t.Fatalf("CLOSE result = %#v, want closed live session", closed)
	}
}

func runFSCP03TimeoutBranches(t *testing.T, canonical fscp03CanonicalOperations, runner *fscp03ControlRunner) {
	t.Helper()
	for _, cancelOnTimeout := range []bool{false, true} {
		requestID := fmt.Sprintf("fscp03-timeout-%t", cancelOnTimeout)
		started := make(chan fscp03StartOutcome, 1)
		go func() {
			result, err := canonical.Start(t.Context(), factorysessions.SessionStartRequest{
				Mode:        factorysessions.SessionOperationModeDurable,
				Correlation: factorysessions.SessionOperationCorrelation{RequestID: requestID},
				Source:      fscp03ChildSource(requestID), Synchronous: true,
				Wait: factorysessions.SessionOperationWait{TimeoutMillis: 25, CancelOnTimeout: cancelOnTimeout},
			})
			started <- fscp03StartOutcome{result: result, err: err}
		}()
		select {
		case <-runner.started:
		case <-time.After(fscp03ObservationTimeout):
			t.Fatalf("timeout branch cancel=%t did not reach provider", cancelOnTimeout)
		}
		var outcome fscp03StartOutcome
		select {
		case outcome = <-started:
		case <-time.After(fscp03ObservationTimeout):
			t.Fatalf("timeout branch cancel=%t did not return", cancelOnTimeout)
		}
		if outcome.err != nil || outcome.result.Sync == nil || outcome.result.Sync.SyncOutcome != factorysessions.SyncOutcome("TIMED_OUT") || !outcome.result.Sync.TimedOut || outcome.result.Sync.SessionCanceledByTimeout != cancelOnTimeout {
			t.Fatalf("timeout branch cancel=%t result = %#v error=%v, want matching timed-out sync outcome", cancelOnTimeout, outcome.result, outcome.err)
		}
		if cancelOnTimeout {
			assertFSCP03DurableStatus(t, canonical, outcome.result.SessionID, factorysessions.LifecycleStatusCanceled)
			continue
		}
		if outcome.result.Status != string(factorysessions.LifecycleStatusRunning) {
			t.Fatalf("timeout branch cancel=false status = %q, want RUNNING", outcome.result.Status)
		}
		control, err := canonical.Control(t.Context(), factorysessions.SessionControlRequest{
			SessionID: outcome.result.SessionID, Mode: factorysessions.SessionOperationModeDurable,
			Operation: factorysessions.SessionControlCancel, Control: factorysessions.ControlRequest{RequestID: requestID + "-cleanup"},
		})
		if err != nil || control.Operation != factorysessions.SessionControlCancel {
			t.Fatalf("timeout false cleanup control = %#v error=%v, want CANCEL", control, err)
		}
		assertFSCP03DurableStatus(t, canonical, outcome.result.SessionID, factorysessions.LifecycleStatusCanceled)
	}
}
