package root_composition_test

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func (fixture *concurrencySharedProcessFixture) runCapacityOne(t *testing.T) {
	t.Helper()
	session := fixture.openCase(t, "CC-01", 1, concurrencyRunnerHold, "cc01-first", "", 0)
	first := submitConcurrencyWork(t, session, "cc01-first")
	session.runner.waitStarted(t, concurrencySharedProcessTimeout)
	second := submitConcurrencyWork(t, session, "cc01-second")
	waitConcurrencyCategories(t, fixture.baseURL, session.id, func(status factoryapi.StatusResponse) bool {
		return status.Categories.Initial >= 1
	})
	if got := session.runner.callCount(); got != 1 {
		t.Fatalf("CC-01 command calls before release = %d, want one", got)
	}
	if got := session.runner.maxActive(); got != 1 {
		t.Fatalf("CC-01 max active calls = %d, want exactly one", got)
	}
	session.runner.releaseCall(1)
	session.runner.waitStarted(t, concurrencySharedProcessTimeout)
	if got := session.runner.callCount(); got != 2 {
		t.Fatalf("CC-01 command calls after first release = %d, want two", got)
	}
	if got := session.runner.maxActive(); got != 1 {
		t.Fatalf("CC-01 max active calls after handoff = %d, want exactly one", got)
	}
	session.runner.releaseCall(2)
	waitConcurrencyWorkSettled(t, fixture.baseURL, session.id, 2)
	assertConcurrencyCompletedWorks(t, session, []factoryapi.SubmitWorkResponse{first, second})
	if got := session.runner.activeCallCount(); got != 0 {
		t.Fatalf("CC-01 active calls after terminal status = %d, want zero", got)
	}
	session.closeAndAssertGone(t)
}

func (fixture *concurrencySharedProcessFixture) runCapacityTwo(t *testing.T) {
	t.Helper()
	session := fixture.openCase(t, "CC-02", 2, concurrencyRunnerHold, "cc02", "", 0)
	first := submitConcurrencyWork(t, session, "cc02-first")
	second := submitConcurrencyWork(t, session, "cc02-second")
	third := submitConcurrencyWork(t, session, "cc02-third")
	session.runner.waitStarted(t, concurrencySharedProcessTimeout)
	session.runner.waitStarted(t, concurrencySharedProcessTimeout)
	waitConcurrencyCategories(t, fixture.baseURL, session.id, func(status factoryapi.StatusResponse) bool {
		return status.Categories.Initial >= 1
	})
	if got := session.runner.activeCallCount(); got != 2 {
		t.Fatalf("CC-02 active calls before release = %d, want two", got)
	}
	if got := session.runner.callCount(); got != 2 {
		t.Fatalf("CC-02 command calls before release = %d, want two", got)
	}
	if got := session.runner.maxActive(); got != 2 {
		t.Fatalf("CC-02 max active calls = %d, want exactly two", got)
	}
	session.runner.releaseCall(1)
	session.runner.waitStarted(t, concurrencySharedProcessTimeout)
	if got := session.runner.callCount(); got != 3 {
		t.Fatalf("CC-02 calls after one release = %d, want three", got)
	}
	session.runner.releaseAll()
	waitConcurrencyWorkSettled(t, fixture.baseURL, session.id, 3)
	assertConcurrencyCompletedWorks(t, session, []factoryapi.SubmitWorkResponse{first, second, third})
	if got := session.runner.activeCallCount(); got != 0 {
		t.Fatalf("CC-02 active calls after terminal status = %d, want zero", got)
	}
	session.closeAndAssertGone(t)
}

func (fixture *concurrencySharedProcessFixture) runConcurrentSessionIsolation(t *testing.T) {
	t.Helper()
	first := fixture.openCase(t, "CC-03-A", 1, concurrencyRunnerHold, "cc03-A", "", 0)
	second := fixture.openCase(t, "CC-03-B", 1, concurrencyRunnerHold, "cc03-B", "", 0)
	firstWork := submitConcurrencyWork(t, first, first.marker)
	secondWork := submitConcurrencyWork(t, second, second.marker)
	first.runner.waitStarted(t, concurrencySharedProcessTimeout)
	second.runner.waitStarted(t, concurrencySharedProcessTimeout)
	if got := fixture.router.activeCount(); got != 2 {
		t.Fatalf("CC-03 router active calls while both are held = %d, want two", got)
	}
	if got := fixture.router.maxActive(); got < 2 {
		t.Fatalf("CC-03 router max active calls = %d, want at least two", got)
	}
	first.runner.releaseCall(1)
	waitConcurrencyWorkSettled(t, fixture.baseURL, first.id, 1)
	if got := second.runner.activeCallCount(); got != 1 {
		t.Fatalf("CC-03 surviving session active calls after first completion = %d, want one", got)
	}
	second.runner.releaseCall(1)
	waitConcurrencyWorkSettled(t, fixture.baseURL, second.id, 1)
	assertConcurrencyCompletedWorks(t, first, []factoryapi.SubmitWorkResponse{firstWork})
	assertConcurrencyCompletedWorks(t, second, []factoryapi.SubmitWorkResponse{secondWork})
	assertDistinctConcurrencyDispatches(t, first, second)
	first.closeAndAssertGone(t)
	second.closeAndAssertGone(t)
}

func (fixture *concurrencySharedProcessFixture) runSessionCancellationIsolation(t *testing.T) {
	t.Helper()
	canceled := fixture.openCase(t, "CC-04-canceled", 1, concurrencyRunnerHold, "cc04-canceled", "", 0)
	survivor := fixture.openCase(t, "CC-04-survivor", 1, concurrencyRunnerHold, "cc04-survivor", "", 0)
	canceledStream := support.OpenFactoryResponseEventStreamAt(t, support.SessionResponseEventsURL(fixture.baseURL, canceled.id))
	canceledDone := make(chan concurrencyInvocationResult, 1)
	go func() {
		response, err := postConcurrencyInvocation(t.Context(), fixture.baseURL, canceled.id, canceled.marker)
		canceledDone <- concurrencyInvocationResult{response: response, err: err}
	}()
	survivorDone := make(chan concurrencyInvocationResult, 1)
	go func() {
		response, err := postConcurrencyInvocation(t.Context(), fixture.baseURL, survivor.id, survivor.marker)
		survivorDone <- concurrencyInvocationResult{response: response, err: err}
	}()
	canceled.runner.waitStarted(t, concurrencySharedProcessTimeout)
	survivor.runner.waitStarted(t, concurrencySharedProcessTimeout)
	if err := cancelConcurrencySession(fixture.baseURL, canceled.id); err != nil {
		t.Fatalf("CC-04 cancel Factory Session: %v", err)
	}
	canceled.runner.waitCanceled(t, concurrencySharedProcessTimeout)
	if got := survivor.runner.activeCallCount(); got != 1 {
		t.Fatalf("CC-04 survivor active calls after canceled session = %d, want one", got)
	}
	support.WaitForSessionStopped(t, fixture.baseURL, canceled.id, concurrencySharedProcessTimeout)
	canceled.closeAndAssertGone(t)
	canceledEvents := readConcurrencyResponseEventsUntilTerminal(t, canceledStream, concurrencySharedProcessTimeout)
	assertConcurrencyCancellationResponseEvents(t, canceledEvents, canceled.id)
	canceledStream.Close()
	canceledStream.WaitClosed(concurrencySharedProcessTimeout)

	survivor.runner.releaseCall(1)
	survivorResult := awaitConcurrencyInvocation(t, survivorDone)
	if survivorResult.err != nil || survivorResult.response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("CC-04 survivor invocation result = %#v, error=%v, want COMPLETED", survivorResult.response, survivorResult.err)
	}
	assertConcurrencyInvocationPrimaryResult(t, survivorResult.response, survivor.marker)

	laterDone := make(chan concurrencyInvocationResult, 1)
	go func() {
		response, err := postConcurrencyInvocation(t.Context(), fixture.baseURL, survivor.id, "cc04-later")
		laterDone <- concurrencyInvocationResult{response: response, err: err}
	}()
	survivor.runner.waitStarted(t, concurrencySharedProcessTimeout)
	survivor.runner.releaseCall(2)
	laterResult := awaitConcurrencyInvocation(t, laterDone)
	if laterResult.err != nil || laterResult.response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("CC-04 later survivor result = %#v, error=%v, want COMPLETED", laterResult.response, laterResult.err)
	}
	assertConcurrencyInvocationPrimaryResult(t, laterResult.response, "cc04-later")
	if canceled.runner.callCount() != 1 || survivor.runner.callCount() != 2 || fixture.router.activeCount() != 0 {
		t.Fatalf("CC-04 calls/active = %d/%d/%d, want canceled=1 survivor=2 active=0", canceled.runner.callCount(), survivor.runner.callCount(), fixture.router.activeCount())
	}
	survivor.closeAndAssertGone(t)
}

func (fixture *concurrencySharedProcessFixture) runWorkerSessionCancellation(t *testing.T) {
	t.Helper()
	// The public Factory Session worker-session observation endpoint exposes
	// runtime-owned Worker Sessions, while the top-level worker-session control
	// endpoint owns a separate direct Worker Sessions service. There is no
	// public Factory Session-scoped cancellation route for this queued-work
	// witness. Keep the acceptance edge explicit until the public ownership
	// boundary is corrected; do not turn the misleading control response into
	// false cancellation evidence.
	t.Skip("CC-05 blocked: public Worker Session cancel does not control a Factory Session runtime Worker Session")
}

func (fixture *concurrencySharedProcessFixture) runIdempotentRequest(t *testing.T) {
	t.Helper()
	session := fixture.openCase(t, "CC-06", 1, concurrencyRunnerSuccess, "cc06", "", 0)
	request := concurrencyWorkRequest("cc06-request", "cc06", "cc06-name")
	first := upsertConcurrencyWorkRequest(t, fixture.baseURL, session.id, request)
	secondStatus, secondBody, second := upsertConcurrencyWorkRequestStatus(t, fixture.baseURL, session.id, request)
	if secondStatus < http.StatusOK || secondStatus >= http.StatusMultipleChoices {
		t.Fatalf("CC-06 duplicate status = %d: %s", secondStatus, secondBody)
	}
	if first.RequestId != second.RequestId || first.TraceId != second.TraceId || len(first.Works) != 1 || len(second.Works) != 1 || first.Works[0].WorkId != second.Works[0].WorkId {
		t.Fatalf("CC-06 duplicate responses differ: first=%#v second=%#v", first, second)
	}
	waitConcurrencyWorkSettled(t, fixture.baseURL, session.id, 1)
	assertConcurrencyCompletedWorks(t, session, []factoryapi.SubmitWorkResponse{{WorkId: stringPointer(first.Works[0].WorkId), RequestId: first.RequestId}})
	assertConcurrencyCounts(t, session, 1, 1)
	events := concurrencySessionEvents(t, fixture.baseURL, session.id)
	if countConcurrencyEvents(events, factoryapi.FactoryEventTypeWorkRequest) != 1 {
		t.Fatalf("CC-06 WORK_REQUEST events = %d, want one", countConcurrencyEvents(events, factoryapi.FactoryEventTypeWorkRequest))
	}
	session.closeAndAssertGone(t)
}

func (fixture *concurrencySharedProcessFixture) runDuplicateConflict(t *testing.T) {
	t.Helper()
	session := fixture.openCase(t, "CC-07", 1, concurrencyRunnerSuccess, "cc07-original", "", 0)
	request := concurrencyWorkRequest("cc07-request", "cc07-original", "cc07-name")
	first := upsertConcurrencyWorkRequest(t, fixture.baseURL, session.id, request)
	waitConcurrencyWorkSettled(t, fixture.baseURL, session.id, 1)
	beforeEvents := concurrencySessionEvents(t, fixture.baseURL, session.id)
	conflicting := concurrencyWorkRequest("cc07-request", "cc07-conflict", "cc07-name")
	status, body, second := upsertConcurrencyWorkRequestStatus(t, fixture.baseURL, session.id, conflicting)
	if status != http.StatusCreated || second.RequestId != first.RequestId || second.TraceId != first.TraceId || len(second.Works) != 1 || second.Works[0] != first.Works[0] {
		t.Fatalf("CC-07 duplicate request = status %d body %q response=%#v first=%#v, want current idempotent replay", status, body, second, first)
	}
	afterEvents := concurrencySessionEvents(t, fixture.baseURL, session.id)
	assertConcurrencyEventIDsUnchanged(t, beforeEvents, afterEvents, "CC-07 conflict")
	assertConcurrencyWorkCompleted(t, session, stringPointer(first.Works[0].WorkId), "cc07-original")
	if got := session.runner.callCount(); got != 1 {
		t.Fatalf("CC-07 provider calls = %d, want one", got)
	}
	session.closeAndAssertGone(t)
}

func (fixture *concurrencySharedProcessFixture) runEmptyRequest(t *testing.T) {
	t.Helper()
	session := fixture.openCase(t, "CC-08", 1, concurrencyRunnerSuccess, "cc08-valid", "", 0)
	empty := factoryapi.WorkRequest{RequestId: "cc08-empty", Type: factoryapi.WorkRequestTypeFactoryRequestBatch}
	status, body, _ := upsertConcurrencyWorkRequestStatus(t, fixture.baseURL, session.id, empty)
	if status != http.StatusBadRequest || strings.TrimSpace(body) == "" {
		t.Fatalf("CC-08 empty Work request = status %d body %q, want non-empty 400 validation", status, body)
	}
	if got := session.runner.callCount(); got != 0 {
		t.Fatalf("CC-08 provider calls after rejected empty request = %d, want zero", got)
	}
	valid := submitConcurrencyWork(t, session, "cc08-valid")
	waitConcurrencyWorkSettled(t, fixture.baseURL, session.id, 1)
	assertConcurrencyCompletedWorks(t, session, []factoryapi.SubmitWorkResponse{valid})
	session.closeAndAssertGone(t)
}

func (fixture *concurrencySharedProcessFixture) runMalformedConfigurationRecovery(t *testing.T) {
	t.Helper()
	runConcurrencyMalformedConfigurationProbe(t)
	session := fixture.openCase(t, "CC-09-valid-after-probe", 1, concurrencyRunnerSuccess, "cc09-valid", "", 0)
	response := submitConcurrencyWork(t, session, "cc09-valid")
	waitConcurrencyWorkSettled(t, fixture.baseURL, session.id, 1)
	assertConcurrencyCompletedWorks(t, session, []factoryapi.SubmitWorkResponse{response})
	session.closeAndAssertGone(t)
}

func (fixture *concurrencySharedProcessFixture) runPartialFailure(t *testing.T) {
	t.Helper()
	session := fixture.openCase(t, "CC-10", 2, concurrencyRunnerFailureHold, "cc10-success", "cc10-fail", 0)
	failing := submitConcurrencyWork(t, session, "cc10-fail")
	succeeding := submitConcurrencyWork(t, session, "cc10-success")
	session.runner.waitStarted(t, concurrencySharedProcessTimeout)
	session.runner.waitStarted(t, concurrencySharedProcessTimeout)
	if got := session.runner.maxActive(); got != 2 {
		t.Fatalf("CC-10 max active calls = %d, want exactly two", got)
	}
	session.runner.releaseAll()
	waitConcurrencyWorkSettled(t, fixture.baseURL, session.id, 2)
	assertConcurrencyWorkFailed(t, session, failing.WorkId, factoryapi.WorkFailureTypeAuthFailure, "Codex authentication failed.")
	assertConcurrencyWorkCompleted(t, session, succeeding.WorkId, "cc10-success")
	if got := session.runner.callCount(); got != 2 || session.runner.activeCallCount() != 0 {
		t.Fatalf("CC-10 calls/active = %d/%d, want two/zero", got, session.runner.activeCallCount())
	}
	assertConcurrencyCounts(t, session, 2, 2)
	session.closeAndAssertGone(t)
}

func (fixture *concurrencySharedProcessFixture) runSessionOrdering(t *testing.T) {
	t.Helper()
	first := fixture.openCase(t, "CC-12-A", 1, concurrencyRunnerSuccess, "cc12-A", "", 0)
	second := fixture.openCase(t, "CC-12-B", 1, concurrencyRunnerSuccess, "cc12-B", "", 0)
	firstWorks := []factoryapi.SubmitWorkResponse{
		submitConcurrencyWork(t, first, "cc12-A-1"),
		submitConcurrencyWork(t, first, "cc12-A-2"),
	}
	secondWorks := []factoryapi.SubmitWorkResponse{
		submitConcurrencyWork(t, second, "cc12-B-1"),
		submitConcurrencyWork(t, second, "cc12-B-2"),
	}
	waitConcurrencyWorkSettled(t, fixture.baseURL, first.id, 2)
	waitConcurrencyWorkSettled(t, fixture.baseURL, second.id, 2)
	assertConcurrencyCompletedWorks(t, first, firstWorks)
	assertConcurrencyCompletedWorks(t, second, secondWorks)
	assertConcurrencySessionOrdering(t, first)
	assertConcurrencySessionOrdering(t, second)
	if fixture.router.callsFor(first.dir) == nil || len(fixture.router.callsFor(first.dir)) != 2 || len(fixture.router.callsFor(second.dir)) != 2 {
		t.Fatalf("CC-12 per-session route calls = %d/%d, want two/two", len(fixture.router.callsFor(first.dir)), len(fixture.router.callsFor(second.dir)))
	}
	first.closeAndAssertGone(t)
	second.closeAndAssertGone(t)
}

func (fixture *concurrencySharedProcessFixture) runTimeoutRecovery(t *testing.T) {
	t.Helper()
	session := fixture.openCase(t, "CC-11", 1, concurrencyRunnerTimeoutMarker, "cc11-timeout", "cc11-timeout", 1)
	timedOut := submitConcurrencyWork(t, session, "cc11-timeout")
	session.runner.waitStarted(t, concurrencySharedProcessTimeout)
	successor := submitConcurrencyWork(t, session, "cc11-successor")
	waitConcurrencyCategories(t, fixture.baseURL, session.id, func(status factoryapi.StatusResponse) bool {
		return status.Categories.Initial >= 1
	})
	session.runner.waitStartedMarker(t, "cc11-successor", concurrencySharedProcessTimeout)
	waitConcurrencyWorkSettled(t, fixture.baseURL, session.id, 2)
	assertConcurrencyWorkFailed(t, session, timedOut.WorkId, factoryapi.WorkFailureTypeTimeout, "provider invocation timed out")
	assertConcurrencyWorkCompleted(t, session, successor.WorkId, "cc11-successor")
	if got := session.runner.callsForMarker("cc11-successor"); got != 1 || session.runner.activeCallCount() != 0 || session.runner.maxActive() != 1 {
		t.Fatalf("CC-11 successor calls/active/peak = %d/%d/%d, want one/zero/one", got, session.runner.activeCallCount(), session.runner.maxActive())
	}
	session.closeAndAssertGone(t)
}

func (fixture *concurrencySharedProcessFixture) runRecovery(t *testing.T) {
	t.Helper()
	old := fixture.openCase(t, "CC-13-old", 1, concurrencyRunnerHold, "cc13-old", "", 0)
	oldWork := submitConcurrencyWork(t, old, old.marker)
	old.runner.waitStarted(t, concurrencySharedProcessTimeout)
	oldEvents := concurrencySessionEvents(t, fixture.baseURL, old.id)
	oldDispatches := support.ObserveDispatchEvents(t, oldEvents)
	if len(oldDispatches) != 1 {
		t.Fatalf("CC-13 old dispatch observations = %#v, want one", oldDispatches)
	}
	if err := cancelConcurrencySession(fixture.baseURL, old.id); err != nil {
		t.Fatalf("CC-13 cancel old session: %v", err)
	}
	old.runner.waitCanceled(t, concurrencySharedProcessTimeout)
	support.WaitForSessionStopped(t, fixture.baseURL, old.id, concurrencySharedProcessTimeout)
	old.closeAndAssertGone(t)
	fixture.router.unregister(old.dir)

	fresh := fixture.openCase(t, "CC-13-fresh", 1, concurrencyRunnerSuccess, "cc13-fresh", "", 0)
	freshWork := submitConcurrencyWork(t, fresh, fresh.marker)
	waitConcurrencyWorkSettled(t, fixture.baseURL, fresh.id, 1)
	assertConcurrencyWorkCompleted(t, fresh, freshWork.WorkId, fresh.marker)
	if fresh.id == old.id || strings.Contains(workContentText(t, concurrencyWorkByID(t, fresh, freshWork.WorkId)), old.marker) {
		t.Fatalf("CC-13 fresh session identity/output reused canceled state: old=%q fresh=%q", old.id, fresh.id)
	}
	freshEvents := concurrencySessionEvents(t, fixture.baseURL, fresh.id)
	freshDispatches := support.ObserveDispatchEvents(t, freshEvents)
	if len(freshDispatches) != 1 || freshDispatches[0].DispatchID == oldDispatches[0].DispatchID || !support.DispatchObservationIncludesWork(freshDispatches[0], stringPointerValue(freshWork.WorkId)) {
		t.Fatalf("CC-13 fresh dispatch = %#v, want new identity for Work %q", freshDispatches, stringPointerValue(freshWork.WorkId))
	}
	if stringPointerValue(oldWork.WorkId) == stringPointerValue(freshWork.WorkId) {
		t.Fatalf("CC-13 fresh Work ID %q reused canceled Work", stringPointerValue(freshWork.WorkId))
	}
	fresh.closeAndAssertGone(t)
}

func (fixture *concurrencySharedProcessFixture) close(t testing.TB) {
	t.Helper()
	if fixture.process == nil {
		return
	}
	if fixture.command != nil {
		fixture.command.Stop(t)
	}
	closeCtx, cancel := context.WithTimeout(context.Background(), concurrencySharedProcessTimeout)
	defer cancel()
	if err := fixture.process.Close(closeCtx); err != nil {
		fixture.processCloseMu.Lock()
		fixture.processCloseErr = err.Error()
		fixture.processCloseMu.Unlock()
		t.Errorf("close shared concurrency application process: %v", err)
	} else {
		fixture.processClosed.Store(true)
	}
	if fixture.command != nil {
		select {
		case <-fixture.apiClosed:
		case <-closeCtx.Done():
			t.Errorf("shared concurrency API server did not close: %v", closeCtx.Err())
		}
	}
	if got := fixture.processBuilds.Load(); got != 1 {
		t.Errorf("shared concurrency process builds = %d, want exactly one", got)
	}
	if fixture.command != nil && fixture.apiStarts.Load() != 1 {
		t.Errorf("shared concurrency API starts = %d, want exactly one", fixture.apiStarts.Load())
	}
	fixture.sessionsMu.Lock()
	if len(fixture.opened) != len(fixture.closed) {
		t.Errorf("closed shared Factory Sessions = %d, opened = %d; opened=%#v closed=%#v", len(fixture.closed), len(fixture.opened), fixture.opened, fixture.closed)
	}
	sessions := make([]*concurrencySession, 0, len(fixture.sessions))
	for _, session := range fixture.sessions {
		sessions = append(sessions, session)
	}
	ownedDirs := append([]string(nil), fixture.ownedDirs...)
	fixture.sessionsMu.Unlock()
	for _, session := range sessions {
		if got := session.runner.activeCallCount(); got != 0 {
			t.Errorf("%s active command calls after process cleanup = %d, want zero", session.name, got)
		}
	}
	fixture.router.clearRoutes()
	if got := fixture.router.routeCount(); got != 0 {
		t.Errorf("shared concurrency routes after cleanup = %d, want zero", got)
	}
	for _, path := range ownedDirs {
		if err := os.RemoveAll(path); err != nil {
			t.Errorf("remove test-owned concurrency path %q: %v", path, err)
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("test-owned concurrency path %q remains after cleanup: %v", path, err)
		}
	}
}

func runConcurrencyMalformedConfigurationProbe(t *testing.T) {
	t.Helper()
	dir := support.ScaffoldFactory(t, map[string]any{
		"name": "concurrency-malformed-worker-reference",
		"workTypes": []any{map[string]any{
			"name": "task",
			"states": []any{
				map[string]any{"name": "init", "type": "INITIAL"},
				map[string]any{"name": "complete", "type": "TERMINAL"},
				map[string]any{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []any{map[string]any{"name": "processor"}},
		"workstations": []map[string]any{{
			"name":      "process",
			"worker":    "ghost-worker",
			"inputs":    []any{map[string]any{"workType": "task", "state": "init"}},
			"outputs":   []any{map[string]any{"workType": "task", "state": "complete"}},
			"onFailure": []any{map[string]any{"workType": "task", "state": "failed"}},
		}},
	})
	runner := support.NewRecordingCommandRunner("malformed concurrency configuration must not invoke a provider")
	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{ProviderCommandRunner: runner})
	if err != nil {
		t.Fatalf("CC-09 BuildProcess() = %v", err)
	}
	inputs := support.FakeInputs(context.Background(), []string{"you", "run", "--dir", dir, "--continuously", "--quiet", "--no-record"})
	inputs.Input.Env = append(os.Environ(), "HOME="+t.TempDir(), "USERPROFILE="+t.TempDir())
	inputs.Input.WorkingDirectory = dir
	err = process.Execute(inputs.Input)
	closeCtx, cancel := context.WithTimeout(context.Background(), concurrencySharedProcessTimeout)
	defer cancel()
	closeErr := process.Close(closeCtx)
	if err == nil {
		t.Fatal("CC-09 malformed worker configuration succeeded, want validation failure")
	}
	diagnostic := strings.ToLower(err.Error())
	if !strings.Contains(diagnostic, "validate factory config") || !strings.Contains(diagnostic, "ghost-worker") {
		t.Fatalf("CC-09 malformed validation error = %q, want actionable dangling-worker diagnostic", err)
	}
	if closeErr != nil {
		t.Fatalf("CC-09 malformed probe process close: %v", closeErr)
	}
	if got := runner.CallCount(); got != 0 {
		t.Fatalf("CC-09 malformed provider calls = %d, want zero", got)
	}
}
