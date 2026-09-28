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

func runFSCP03SequentialDurableIdentity(t *testing.T, canonical factorysessions.Service, selections *factorysessions.SessionRuntimeSelection) {
	t.Helper()
	first := fscp03StartSynchronous(t, canonical, selections, "fscp03-sequential-first", fscp03ChildSource("sequential-first"))
	second := fscp03StartSynchronous(t, canonical, selections, "fscp03-sequential-second", fscp03ChildSource("sequential-second"))
	if first.SessionID == second.SessionID {
		t.Fatalf("sequential session ids = %q and %q, want distinct", first.SessionID, second.SessionID)
	}
	assertFSCP03SuccessfulStart(t, first)
	assertFSCP03SuccessfulStart(t, second)
	assertFSCP03DurableLineage(t, canonical, first.SessionID)
	assertFSCP03DurableLineage(t, canonical, second.SessionID)
	assertFSCP03DisjointDurableObservations(t, canonical, first.SessionID, second.SessionID)
}

func runFSCP03DurableDirection(t *testing.T, canonical factorysessions.Service, selections *factorysessions.SessionRuntimeSelection) {
	t.Helper()
	first := fscp03StartSynchronous(t, canonical, selections, "fscp03-canonical-direction-first", fscp03InlineSource(`return "fscp03 direction COMPLETE";`))
	second := fscp03StartSynchronous(t, canonical, selections, "fscp03-canonical-direction-second", fscp03InlineSource(`return "fscp03 direction COMPLETE";`))
	if first.SessionID == second.SessionID {
		t.Fatalf("canonical direction session id = %q, want distinct executions", first.SessionID)
	}
	assertFSCP03SuccessfulStart(t, first)
	assertFSCP03SuccessfulStart(t, second)
	firstView := fscp03GetDurableSession(t, canonical, first.SessionID)
	secondView := fscp03GetDurableSession(t, canonical, second.SessionID)
	for _, view := range []fscp03DurableSessionFacts{firstView, secondView} {
		if view.Status != string(factorysessions.LifecycleStatusSucceeded) || view.OrchestratorKind != "JAVASCRIPT" || view.SourceRef != "inline" || !view.HasResultSummary || view.ResultStatus != string(factorysessions.ResultStatusFinal) {
			t.Fatalf("canonical direction view = %#v, want durable success semantics", view)
		}
	}
	if firstView != secondView {
		// Both projections carry success semantics; only session-scoped
		// identity may differ, which the distinct-session check above covers.
		// Compare the semantic fields explicitly to keep the assertion stable.
		if firstView.Status != secondView.Status || firstView.OrchestratorKind != secondView.OrchestratorKind || firstView.SourceRef != secondView.SourceRef || firstView.ResultStatus != secondView.ResultStatus {
			t.Fatalf("canonical views first=%#v second=%#v, want equivalent public projections", firstView, secondView)
		}
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

func fscp03GetDurableSession(t *testing.T, canonical factorysessions.Service, sessionID string) fscp03DurableSessionFacts {
	t.Helper()
	view, err := canonical.Get(t.Context(), factorysessions.SessionGetRequest{
		SessionID: sessionID,
		Mode:      factorysessions.SessionOperationModeDurable,
	})
	if err != nil {
		t.Fatalf("canonical Get(%s) error = %v", sessionID, err)
	}
	facts := fscp03DurableSessionFacts{
		Status:           view.Session.Status,
		OrchestratorKind: view.Session.OrchestratorKind,
		SourceRef:        view.Session.SourceRef,
		HasResultSummary: view.Session.ResultStatus != "",
		ResultStatus:     view.Session.ResultStatus,
	}
	result, err := canonical.ReadResult(t.Context(), factorysessions.SessionResultReadRequest{
		SessionID: sessionID,
		Mode:      factorysessions.SessionOperationModeDurable,
		Request:   factorysessions.ResultRequest{Mode: factorysessions.ResultModeFinal},
	})
	if err != nil {
		t.Fatalf("canonical ReadResult(%s) error = %v", sessionID, err)
	}
	if result.Durable != nil && result.Durable.Failure != nil {
		facts.HasFailure = true
		facts.FailureMessage = result.Durable.Failure.Message
	}
	return facts
}

func runFSCP03ConcurrentDurableIdentity(t *testing.T, factoryDir, home string) {
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
	concurrentCanonical := openFSCP03Execution(t, concurrentProcess)
	selections := fscp03RuntimeSelections(factoryDir, home)
	runFSCP03ConcurrentStarts(t, concurrentCanonical, selections, barrier)
	runFSCP03FailedStartRecovery(t, concurrentCanonical, selections, barrier)
}

func runFSCP03ConcurrentStarts(t *testing.T, canonical factorysessions.Service, selections *factorysessions.SessionRuntimeSelection, barrier *fscp03BarrierRunner) {
	t.Helper()
	results := make(chan fscp03StartOutcome, 2)
	for _, label := range []string{"concurrent-a", "concurrent-b"} {
		label := label
		go func() {
			result, err := canonical.Start(t.Context(), factorysessions.SessionStartRequest{
				Mode:             factorysessions.SessionOperationModeDurable,
				FolderPath:       selections.ExecutionBaseDir,
				Persistence:      factorysessions.PersistencePolicyDisabled,
				Correlation:      factorysessions.SessionOperationCorrelation{RequestID: "fscp03-" + label},
				Source:           fscp03ChildSource(label),
				Synchronous:      true,
				RuntimeSelection: selections,
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

func runFSCP03FailedStartRecovery(t *testing.T, canonical factorysessions.Service, selections *factorysessions.SessionRuntimeSelection, barrier *fscp03BarrierRunner) {
	t.Helper()
	barrier.Reset()
	activeResults := make(chan fscp03StartOutcome, 1)
	go func() {
		result, err := canonical.Start(t.Context(), factorysessions.SessionStartRequest{
			Mode:             factorysessions.SessionOperationModeDurable,
			FolderPath:       selections.ExecutionBaseDir,
			Persistence:      factorysessions.PersistencePolicyDisabled,
			Correlation:      factorysessions.SessionOperationCorrelation{RequestID: "fscp03-failure-peer"},
			Source:           fscp03ChildSource("failure-peer"),
			Synchronous:      true,
			RuntimeSelection: selections,
		})
		activeResults <- fscp03StartOutcome{result: result, err: err}
	}()
	if err := barrier.WaitStarted(t.Context(), 1); err != nil {
		t.Fatalf("active peer did not reach controlled provider: %v", err)
	}
	failed, err := canonical.Start(t.Context(), factorysessions.SessionStartRequest{
		Mode:             factorysessions.SessionOperationModeDurable,
		FolderPath:       selections.ExecutionBaseDir,
		Persistence:      factorysessions.PersistencePolicyDisabled,
		Correlation:      factorysessions.SessionOperationCorrelation{RequestID: "fscp03-controlled-failure"},
		Source:           fscp03InlineSource(`throw new Error("fscp03 controlled failure");`),
		Synchronous:      true,
		RuntimeSelection: selections,
	})
	if err != nil {
		t.Fatalf("controlled failed Start() error = %v, want terminal failed projection", err)
	}
	if failed.Status != string(factorysessions.LifecycleStatusFailed) {
		t.Fatalf("failed start status = %q, want FAILED", failed.Status)
	}
	failedView := fscp03GetDurableSession(t, canonical, failed.SessionID)
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
	recovery := fscp03StartSynchronous(t, canonical, selections, "fscp03-after-failure", fscp03ChildSource("after-failure"))
	assertFSCP03SuccessfulStart(t, recovery)
	if recovery.SessionID == failed.SessionID || recovery.SessionID == active.result.SessionID {
		t.Fatalf("recovery session id = %q, want a fresh usable session", recovery.SessionID)
	}
	assertFSCP03DurableLineage(t, canonical, recovery.SessionID)
}

func runFSCP03Cancel(t *testing.T, canonical factorysessions.Service, selections *factorysessions.SessionRuntimeSelection, runner *fscp03ControlRunner) {
	t.Helper()
	canceled := fscp03StartBlockedAsync(t, canonical, selections, runner.started, "fscp03-cancel")
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

func runFSCP03Terminate(t *testing.T, canonical factorysessions.Service, selections *factorysessions.SessionRuntimeSelection, runner *fscp03ControlRunner) {
	t.Helper()
	terminated := fscp03StartBlockedAsync(t, canonical, selections, runner.started, "fscp03-terminate")
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

func runFSCP03Close(t *testing.T, canonical factorysessions.Service, selections *factorysessions.SessionRuntimeSelection, factoryDir string) {
	t.Helper()
	live, err := canonical.Start(t.Context(), factorysessions.SessionStartRequest{
		Mode:             factorysessions.SessionOperationModeLive,
		FolderPath:       factoryDir,
		RuntimeSelection: selections,
	})
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

func runFSCP03TimeoutBranches(t *testing.T, canonical factorysessions.Service, selections *factorysessions.SessionRuntimeSelection, runner *fscp03ControlRunner) {
	t.Helper()
	for _, cancelOnTimeout := range []bool{false, true} {
		requestID := fmt.Sprintf("fscp03-timeout-%t", cancelOnTimeout)
		started := make(chan fscp03StartOutcome, 1)
		go func() {
			result, err := canonical.Start(t.Context(), factorysessions.SessionStartRequest{
				Mode:             factorysessions.SessionOperationModeDurable,
				FolderPath:       selections.ExecutionBaseDir,
				Persistence:      factorysessions.PersistencePolicyDisabled,
				Correlation:      factorysessions.SessionOperationCorrelation{RequestID: requestID},
				Source:           fscp03ChildSource(requestID),
				Synchronous:      true,
				RuntimeSelection: selections,
				Wait:             factorysessions.SessionOperationWait{TimeoutMillis: 25, CancelOnTimeout: cancelOnTimeout},
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
